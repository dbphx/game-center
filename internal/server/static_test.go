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
		if res.StatusCode != 200 || etag == "" || res.Header.Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: %d etag=%q cc=%q", p, res.StatusCode, etag, res.Header.Get("Cache-Control"))
		}
		res = get(p, map[string]string{"If-None-Match": etag, "Accept-Encoding": "gzip"})
		res.Body.Close()
		if res.StatusCode != http.StatusNotModified {
			t.Fatalf("%s: hỏi lại phải 304, được %d", p, res.StatusCode)
		}
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
