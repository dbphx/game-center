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
