package server

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// staticFiles phục vụ giao diện nhúng trong binary với bộ nhớ đệm của trình duyệt:
//   - mỗi file có ETag theo nội dung, trình duyệt hỏi lại thì nhận 304 (không tải lại);
//   - file chữ (html, js, css, json) được nén gzip sẵn lúc khởi động;
//   - /sw.js là service worker giữ ảnh (/img/) trong Cache Storage theo phiên bản bản build,
//     nên lần sau mở trang không phải tải lại ảnh; cập nhật server đổi phiên bản thì bộ đệm cũ bị xoá;
//   - trang HTML gắn ?v=<ETag> vào link js / css: Cloudflare ghi đè Cache-Control thành max-age 12 giờ,
//     nên không có phiên bản trong URL thì sau khi cập nhật trình duyệt vẫn chạy code cũ.
type staticFiles struct {
	fs      fs.FS
	files   http.Handler
	etags   map[string]string // đường dẫn (không có "/" đầu) → ETag
	gz      map[string][]byte // bản nén gzip của file chữ
	pages   map[string][]byte // trang HTML và module JS đã gắn phiên bản vào link / import
	version string            // băm của toàn bộ file, đổi mỗi khi giao diện đổi
	started time.Time
}

func newStaticFiles(static fs.FS) *staticFiles {
	s := &staticFiles{fs: static, files: http.FileServer(http.FS(static)), etags: map[string]string{}, gz: map[string][]byte{},
		pages: map[string][]byte{}, started: time.Now()}
	datas := map[string][]byte{}
	var names []string
	fs.WalkDir(static, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(static, p)
		if err != nil {
			return nil
		}
		datas[p] = data
		s.etags[p] = etagOf(data)
		names = append(names, p)
		return nil
	})
	// Module JS: gắn ?v=<phiên bản> vào import tương đối (đệ quy theo đồ thị import) trước, vì phiên bản
	// của file cha phải tính trên nội dung đã viết lại — file con đổi thì URL của file cha cũng đổi.
	state := map[string]int{}
	for _, p := range names {
		if path.Ext(p) == ".js" {
			s.resolveJS(p, datas, state)
		}
	}
	// Trang HTML: đã biết ETag của mọi js / css thì mới gắn phiên bản được.
	for _, p := range names {
		if path.Ext(p) == ".html" {
			page := s.versioned(datas[p])
			s.pages[p] = page
			datas[p] = page
			s.etags[p] = etagOf(page)
		}
	}
	for _, p := range names {
		data := datas[p]
		if compressible(p) && len(data) > 512 {
			var buf bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
			zw.Write(data)
			zw.Close()
			if buf.Len() < len(data) {
				s.gz[p] = buf.Bytes()
			}
		}
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		fmt.Fprintf(h, "%s %s\n", n, s.etags[n])
	}
	s.version = hex.EncodeToString(h.Sum(nil)[:6])
	return s
}

func etagOf(data []byte) string {
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// Link js / css tuyệt đối trong trang, vd src="/city.js" hay href="/hub.css" (chưa có ?query).
var assetRef = regexp.MustCompile(`((?:src|href)=")(/[^"?#]+\.(?:js|css))"`)

// versioned gắn ?v=<ETag rút gọn> vào link js / css có trong bộ file; file đổi nội dung thì URL đổi theo.
// Đường dẫn js trong <script type="importmap"> cũng được gắn phiên bản.
func (s *staticFiles) versioned(page []byte) []byte {
	page = assetRef.ReplaceAllFunc(page, func(m []byte) []byte {
		sub := assetRef.FindSubmatch(m)
		etag, ok := s.etags[strings.TrimPrefix(string(sub[2]), "/")]
		if !ok {
			return m
		}
		return []byte(fmt.Sprintf(`%s%s?v=%s"`, sub[1], sub[2], shortVer(etag)))
	})
	return importMap.ReplaceAllFunc(page, func(block []byte) []byte {
		return mapRef.ReplaceAllFunc(block, func(m []byte) []byte {
			p := string(m[2 : len(m)-1])
			etag, ok := s.etags[p]
			if !ok {
				return m
			}
			return []byte(fmt.Sprintf(`"/%s?v=%s"`, p, shortVer(etag)))
		})
	})
}

func shortVer(etag string) string { return strings.Trim(etag, `"`)[:10] }

var (
	importMap = regexp.MustCompile(`(?s)<script type="importmap">.*?</script>`)
	mapRef    = regexp.MustCompile(`"/[^"?#]+\.js"`)
	// jsRef: chỗ nạp module tĩnh trong JS — from '…', import '…', import('…') (cả nháy đơn và kép).
	jsRef = regexp.MustCompile(`(\bfrom\s*|\bimport\s*\(\s*|\bimport\s+)(['"])([^'"\n]+)(['"])`)
)

// localModule đổi đường dẫn import trong file from thành đường dẫn file trong bộ (không có "/" đầu).
// Chỉ nhận đường dẫn tương đối (./, ../) và tuyệt đối trong site (/…); bare specifier ("three") và URL ngoài thì bỏ qua.
func localModule(from, spec string) (string, bool) {
	base := spec
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	var p string
	switch {
	case strings.HasPrefix(base, "./") || strings.HasPrefix(base, "../"):
		p = path.Join(path.Dir(from), base)
	case strings.HasPrefix(base, "/") && !strings.HasPrefix(base, "//"):
		p = strings.TrimPrefix(path.Clean(base), "/")
	default:
		return "", false
	}
	if path.Ext(p) != ".js" || strings.HasPrefix(p, "..") {
		return "", false
	}
	return p, true
}

// resolveJS viết lại import tương đối của file p thành ./x.js?v=<phiên bản>, viết file con trước (đệ quy).
// state: 1 đang xử lý, 2 xong. Gặp vòng import thì file đang xử lý dùng ETag theo nội dung gốc (vẫn ổn định, không treo).
func (s *staticFiles) resolveJS(p string, datas map[string][]byte, state map[string]int) {
	if state[p] != 0 {
		return
	}
	state[p] = 1
	src := datas[p]
	out := jsRef.ReplaceAllFunc(src, func(m []byte) []byte {
		sub := jsRef.FindSubmatch(m)
		spec := string(sub[3])
		target, ok := localModule(p, spec)
		if !ok {
			return m
		}
		if _, ok := datas[target]; !ok {
			return m
		}
		s.resolveJS(target, datas, state)
		if i := strings.IndexAny(spec, "?#"); i >= 0 {
			spec = spec[:i]
		}
		return []byte(fmt.Sprintf("%s%s%s?v=%s%s", sub[1], sub[2], spec, shortVer(s.etags[target]), sub[4]))
	})
	if !bytes.Equal(out, src) {
		datas[p] = out
		s.pages[p] = out
		s.etags[p] = etagOf(out)
	}
	state[p] = 2
}

func compressible(p string) bool {
	switch path.Ext(p) {
	case ".html", ".js", ".css", ".json", ".svg", ".txt", ".csv", ".md":
		return true
	}
	return false
}

func (s *staticFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/sw.js" {
		s.serviceWorker(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || strings.HasSuffix(r.URL.Path, "/") {
		name = path.Join(name, "index.html")
	}
	etag, ok := s.etags[name]
	if !ok {
		s.files.ServeHTTP(w, r) // 404 hoặc chuyển hướng thư mục → để FileServer xử lý
		return
	}
	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Cache-Control", cacheControl(name, etag, r.URL.Query().Get("v")))
	h.Add("Vary", "Accept-Encoding")
	if gz, ok := s.gz[name]; ok && r.Header.Get("Range") == "" && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if ct := contentType(name); ct != "" {
			h.Set("Content-Type", ct)
		}
		h.Set("Content-Encoding", "gzip")
		http.ServeContent(w, r, name, s.started, bytes.NewReader(gz))
		return
	}
	if page, ok := s.pages[name]; ok {
		h.Set("Content-Type", contentType(name))
		http.ServeContent(w, r, name, s.started, bytes.NewReader(page))
		return
	}
	s.files.ServeHTTP(w, r) // FileServer tự trả 304 khi If-None-Match khớp ETag đã đặt
}

// cacheControl chọn thời gian trình duyệt (và Cloudflare) được giữ file:
//   - js / css có ?v= đúng phiên bản hiện tại: giữ 1 năm, không hỏi lại (đổi nội dung thì URL đổi theo);
//   - ảnh, font, âm thanh: giữ 1 ngày rồi hỏi lại bằng ETag (đổi ảnh mà giữ tên thì có thể thấy ảnh cũ tối đa 1 ngày);
//   - HTML, js / css không có hoặc sai phiên bản, json và file chữ khác: luôn hỏi lại (304 khi không đổi).
func cacheControl(name, etag, v string) string {
	switch path.Ext(name) {
	case ".js", ".css":
		if v != "" && strings.HasPrefix(strings.Trim(etag, `"`), v) {
			return "public, max-age=31536000, immutable"
		}
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".woff", ".woff2", ".ttf", ".mp3", ".ogg", ".wav", ".glb", ".gltf", ".bin":
		return "public, max-age=86400"
	}
	return "no-cache"
}

func contentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".json":
		return "application/json"
	case ".svg":
		return "image/svg+xml"
	case ".txt", ".md":
		return "text/plain; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	}
	return ""
}

// serviceWorker trả /sw.js gắn phiên bản bản build. Trang HTML và API luôn đi thẳng tới server;
// ảnh lấy từ bộ đệm nếu có, chưa có thì tải một lần rồi cất vào.
func (s *staticFiles) serviceWorker(w http.ResponseWriter, _ *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/javascript; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	fmt.Fprintf(w, `// DCity service worker — phiên bản %[1]s
const CACHE = 'dcity-%[1]s';
self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => {
  e.waitUntil(caches.keys()
    .then((keys) => Promise.all(keys.filter((k) => k.startsWith('dcity-') && k !== CACHE).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});
self.addEventListener('fetch', (e) => {
  const req = e.request;
  if (req.method !== 'GET' || req.mode === 'navigate') return;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;
  const p = url.pathname;
  // Chỉ giữ ảnh (phần nặng) theo phiên bản; js / css / html đi qua bộ đệm HTTP thường (ETag → 304)
  // để ngay lần tải đầu sau khi cập nhật, trang mới không bị ghép với code cũ.
  if (!p.startsWith('/img/')) return;
  e.respondWith(caches.open(CACHE).then((cache) => cache.match(req).then((hit) => hit || fetch(req).then((res) => {
    if (res.ok && res.type === 'basic') cache.put(req, res.clone());
    return res;
  }))));
});
`, s.version)
}
