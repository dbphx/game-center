package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticCaching(t *testing.T) {
	big := strings.Repeat("console.log('dcity');\n", 100)
	st := newStaticFiles(fstest.MapFS{
		"index.html":   {Data: []byte("<h1>hub</h1>")},
		"app.js":       {Data: []byte(big)},
		"img/logo.png": {Data: []byte{0x89, 'P', 'N', 'G', 1, 2, 3}},
	})
	srv := newHTTPTest(t, st)
	tr := &http.Transport{DisableCompression: true}
	cl := &http.Client{Transport: tr}
	get := func(p string, hdr map[string]string) *http.Response {
		req, _ := http.NewRequest("GET", srv+p, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		res, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// ETag theo nội dung, hỏi lại với ETag thì 304.
	for _, p := range []string{"/", "/app.js", "/img/logo.png"} {
		res := get(p, nil)
		res.Body.Close()
		etag := res.Header.Get("ETag")
		want := map[string]string{"/": "no-cache", "/app.js": "no-cache", "/img/logo.png": "public, max-age=86400"}[p]
		if res.StatusCode != 200 || etag == "" || res.Header.Get("Cache-Control") != want {
			t.Fatalf("%s: %d etag=%q cc=%q", p, res.StatusCode, etag, res.Header.Get("Cache-Control"))
		}
		res = get(p, map[string]string{"If-None-Match": etag, "Accept-Encoding": "gzip"})
		res.Body.Close()
		if res.StatusCode != http.StatusNotModified {
			t.Fatalf("%s: hỏi lại phải 304, được %d", p, res.StatusCode)
		}
	}

	// JS / CSS có ?v= đúng phiên bản: giữ 1 năm, không hỏi lại; sai phiên bản (bản cũ) thì vẫn phải hỏi lại.
	v := strings.Trim(st.etags["app.js"], `"`)[:10]
	if res := get("/app.js?v="+v, nil); res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("js có phiên bản: cc=%q", res.Header.Get("Cache-Control"))
	}
	if res := get("/app.js?v=deadbeef00", nil); res.Header.Get("Cache-Control") != "no-cache" {
		t.Fatalf("js sai phiên bản: cc=%q", res.Header.Get("Cache-Control"))
	}

	// JS được nén gzip khi trình duyệt nhận gzip.
	res := get("/app.js", map[string]string{"Accept-Encoding": "gzip"})
	if res.Header.Get("Content-Encoding") != "gzip" || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") {
		t.Fatalf("js phải gzip: %v", res.Header)
	}
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(zr)
	res.Body.Close()
	if string(body) != big {
		t.Fatal("giải nén không ra đúng nội dung")
	}

	// Service worker gắn phiên bản; đổi nội dung thì phiên bản đổi.
	res = get("/sw.js", nil)
	sw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.Contains(string(sw), "dcity-"+st.version) {
		t.Fatalf("sw.js thiếu phiên bản: %s", sw)
	}
	st2 := newStaticFiles(fstest.MapFS{"index.html": {Data: []byte("<h1>hub 2</h1>")}})
	if st2.version == st.version {
		t.Fatal("đổi giao diện thì phiên bản phải đổi")
	}
	if res := get("/khong-co.png", nil); res.StatusCode != 404 {
		t.Fatalf("file không có phải 404, được %d", res.StatusCode)
	}
}

func newHTTPTest(t *testing.T, h http.Handler) string {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

// Trang HTML gắn ?v= theo nội dung js / css: Cloudflare giữ js / css 12 giờ, URL phải đổi khi file đổi.
func TestStaticVersionedAssets(t *testing.T) {
	page := `<link rel="stylesheet" href="/app.css"><script src="/app.js"></script><script src="/khong-co.js"></script><a href="/farm/">x</a>`
	mk := func(js string) (*staticFiles, string) {
		st := newStaticFiles(fstest.MapFS{
			"index.html":      {Data: []byte(page)},
			"farm/index.html": {Data: []byte(page)},
			"app.js":          {Data: []byte(js)},
			"app.css":         {Data: []byte("body{}")},
		})
		srv := newHTTPTest(t, st)
		res, err := http.Get(srv + "/farm/")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/html") || res.Header.Get("ETag") != st.etags["farm/index.html"] {
			t.Fatalf("trang: %v", res.Header)
		}
		// Hỏi lại với ETag của trang đã gắn phiên bản thì 304.
		req, _ := http.NewRequest("GET", srv+"/farm/", nil)
		req.Header.Set("If-None-Match", res.Header.Get("ETag"))
		res2, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res2.Body.Close()
		if res2.StatusCode != http.StatusNotModified {
			t.Fatalf("hỏi lại trang phải 304, được %d", res2.StatusCode)
		}
		return st, string(body)
	}
	st, a := mk("console.log(1)")
	want := `src="/app.js?v=` + strings.Trim(st.etags["app.js"], `"`)[:10] + `"`
	if !strings.Contains(a, want) || !strings.Contains(a, `href="/app.css?v=`) {
		t.Fatalf("thiếu phiên bản: %s", a)
	}
	if !strings.Contains(a, `src="/khong-co.js"`) || !strings.Contains(a, `href="/farm/"`) {
		t.Fatalf("link không phải file js / css có thật phải giữ nguyên: %s", a)
	}
	_, b := mk("console.log(2)")
	if a == b {
		t.Fatal("đổi js thì link trong trang phải đổi")
	}
}

// Module JS: import tương đối được gắn ?v=<phiên bản của file con>; đổi file con thì ?v của file cha
// và của thẻ <script> trong trang đều đổi (Cloudflare giữ js không có ?v tới 12 giờ).
func TestStaticModuleImportsVersioned(t *testing.T) {
	build := func(physics string) *staticFiles {
		return newStaticFiles(fstest.MapFS{
			"race/index.html": {Data: []byte(`<script type="importmap">{"imports":{"three":"/vendor/three.js"}}</script>` +
				`<script type="module" src="/race/race.js"></script>`)},
			"race/race.js": {Data: []byte("import { step } from './physics.js';\n" +
				"import \"./scene.js?v=old\";\n" +
				"export { a } from '../lib/util.js';\n" +
				"const m = import( './lazy.js' );\n" +
				"import * as THREE from 'three';\n" +
				"import x from '/race/abs.js';\n" +
				"import y from 'https://cdn.example.com/z.js';\n" + strings.Repeat("// đệm cho đủ dài để nén gzip\n", 40))},
			"race/physics.js": {Data: []byte(physics)},
			"race/scene.js":   {Data: []byte("import './race.js';\nexport const s = 1;\n")}, // vòng import
			"race/lazy.js":    {Data: []byte("export default 1;\n")},
			"race/abs.js":     {Data: []byte("export default 2;\n")},
			"lib/util.js":     {Data: []byte("export const a = 1;\n")},
			"vendor/three.js": {Data: []byte("export const V = 1;\n")},
		})
	}
	st := build("export const step = 1;\n")
	ver := func(p string) string { return shortVer(st.etags[p]) }
	race := string(st.pages["race/race.js"])
	for _, want := range []string{
		"from './physics.js?v=" + ver("race/physics.js") + "'",
		`import "./scene.js?v=` + ver("race/scene.js") + `"`,
		"from '../lib/util.js?v=" + ver("lib/util.js") + "'",
		"import( './lazy.js?v=" + ver("race/lazy.js") + "' )",
		"from '/race/abs.js?v=" + ver("race/abs.js") + "'",
		"from 'three'",
		"from 'https://cdn.example.com/z.js'",
	} {
		if !strings.Contains(race, want) {
			t.Fatalf("race.js thiếu %q:\n%s", want, race)
		}
	}
	if st.etags["race/race.js"] != etagOf(st.pages["race/race.js"]) {
		t.Fatal("ETag phải tính trên nội dung đã viết lại")
	}
	page := string(st.pages["race/index.html"])
	if !strings.Contains(page, `src="/race/race.js?v=`+ver("race/race.js")+`"`) || !strings.Contains(page, `"/vendor/three.js?v=`+ver("vendor/three.js")+`"`) {
		t.Fatalf("trang chưa gắn phiên bản đúng: %s", page)
	}

	// Đổi file con → phiên bản file cha và thẻ trong trang đổi theo.
	st2 := build("export const step = 2;\n")
	if st2.etags["race/race.js"] == st.etags["race/race.js"] || string(st2.pages["race/index.html"]) == page {
		t.Fatal("đổi physics.js phải đổi phiên bản race.js và trang")
	}

	// Phục vụ đúng nội dung đã viết lại, cả bản gzip (Content-Length khớp).
	srv := newHTTPTest(t, st)
	cl := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	for _, enc := range []string{"", "gzip"} {
		req, _ := http.NewRequest("GET", srv+"/race/race.js?v="+ver("race/race.js"), nil)
		if enc != "" {
			req.Header.Set("Accept-Encoding", enc)
		}
		res, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var r io.Reader = res.Body
		if res.Header.Get("Content-Encoding") == "gzip" {
			zr, err := gzip.NewReader(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			r = zr
		}
		body, _ := io.ReadAll(r)
		res.Body.Close()
		if string(body) != race || res.Header.Get("Cache-Control") != "public, max-age=31536000, immutable" || res.Header.Get("ETag") != st.etags["race/race.js"] {
			t.Fatalf("enc=%q: nội dung/headers sai: cc=%q etag=%q\n%s", enc, res.Header.Get("Cache-Control"), res.Header.Get("ETag"), body)
		}
	}
}
