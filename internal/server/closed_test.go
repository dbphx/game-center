package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/dbphx/game-center/internal/wallet"
)

// Game có Closed: trang, API, WebSocket, danh sách bàn đều bị chặn; /api/games báo closed để giao diện khoá nút vào.
func TestClosedGameBlocked(t *testing.T) {
	w, err := wallet.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	reached := false
	ok := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { reached = true })
	static := fstest.MapFS{"index.html": {Data: []byte("hub")}, "arena/index.html": {Data: []byte("arena")}}
	srv := httptest.NewServer(New(w, static, []Game{{
		ID: "arena", Name: "Đấu trường", Path: "/arena/", Closed: "Đấu trường tạm đóng cửa",
		WS: ok, Rooms: ok, API: ok,
	}}))
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	for _, path := range []string{"/ws/arena", "/api/arena/rooms", "/api/arena", "/api/arena/shop"} {
		res, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]string
		json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if res.StatusCode != http.StatusServiceUnavailable || body["error"] != "Đấu trường tạm đóng cửa" {
			t.Fatalf("%s: muốn 503 kèm thông báo, được %d %v", path, res.StatusCode, body)
		}
	}
	if reached {
		t.Fatal("handler của game đóng cửa không được chạy")
	}
	for _, path := range []string{"/arena/", "/arena/arena.js"} {
		res, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
			t.Fatalf("%s: muốn chuyển về bản đồ, được %d %q", path, res.StatusCode, res.Header.Get("Location"))
		}
	}

	var games []map[string]any
	res, err := client.Get(srv.URL + "/api/games")
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(res.Body).Decode(&games)
	res.Body.Close()
	if len(games) != 1 || games[0]["closed"] != "Đấu trường tạm đóng cửa" {
		t.Fatalf("/api/games phải báo closed: %v", games)
	}
}
