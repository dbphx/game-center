package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"

	"github.com/dbphx/game-center/games/tienlen/online"
	"github.com/dbphx/game-center/internal/wallet"
)

func newTestServer(t *testing.T) (*httptest.Server, *wallet.Wallet) {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	auth := func(ctx context.Context, token string) (online.User, error) {
		u, err := w.UserByToken(ctx, token)
		return online.User{ID: u.ID, Name: u.Name}, err
	}
	tl := online.NewHub(w, online.Options{BotDelay: time.Millisecond, TurnTimeout: time.Minute, Grace: time.Minute})
	static := fstest.MapFS{"index.html": {Data: []byte("hub")}}
	srv := httptest.NewServer(New(w, static, []Game{{
		ID: online.GameID, Name: "Tiến Lên", Path: "/tienlen/",
		Stats: func() any { return tl.Stats() }, WS: tl.Handler(auth),
	}}))
	t.Cleanup(srv.Close)
	return srv, w
}

func call(t *testing.T, srv *httptest.Server, method, path, token string, body any, out any) int {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, srv.URL+path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		json.NewDecoder(res.Body).Decode(out)
	}
	return res.StatusCode
}

type guest struct {
	Token string      `json:"token"`
	User  wallet.User `json:"user"`
}

func newGuest(t *testing.T, srv *httptest.Server, name string) guest {
	t.Helper()
	var g guest
	if code := call(t, srv, "POST", "/api/guest", "", map[string]string{"name": name}, &g); code != 200 || g.Token == "" {
		t.Fatalf("tạo khách: %d %+v", code, g)
	}
	return g
}

func TestAccountAPI(t *testing.T) {
	srv, _ := newTestServer(t)
	if code := call(t, srv, "POST", "/api/guest", "", map[string]string{"name": " "}, nil); code != 400 {
		t.Fatalf("tên trống phải 400, được %d", code)
	}
	g := newGuest(t, srv, "An")
	if g.User.Balance != wallet.StartBalance {
		t.Fatalf("số dư ban đầu %d", g.User.Balance)
	}
	if code := call(t, srv, "GET", "/api/me", "sai", nil, nil); code != 401 {
		t.Fatalf("token sai phải 401, được %d", code)
	}
	var u wallet.User
	call(t, srv, "POST", "/api/me", g.Token, map[string]string{"name": "An Béo"}, &u)
	if u.Name != "An Béo" {
		t.Fatalf("đổi tên: %+v", u)
	}
	if code := call(t, srv, "POST", "/api/daily", g.Token, nil, &u); code != 200 || u.Balance != wallet.StartBalance+wallet.DailyBonus {
		t.Fatalf("thưởng ngày: %d %+v", code, u)
	}
	if code := call(t, srv, "POST", "/api/daily", g.Token, nil, nil); code != 409 {
		t.Fatalf("nhận 2 lần phải 409, được %d", code)
	}
	var hist []wallet.Entry
	call(t, srv, "GET", "/api/history", g.Token, nil, &hist)
	if len(hist) != 2 || hist[0].Note != "Thưởng hằng ngày" {
		t.Fatalf("lịch sử: %+v", hist)
	}
	var games []map[string]any
	call(t, srv, "GET", "/api/games", "", nil, &games)
	if len(games) != 1 || games[0]["id"] != "tienlen" || games[0]["online"] == nil {
		t.Fatalf("danh sách game: %+v", games)
	}
}

type wsClient struct {
	t  *testing.T
	ws *websocket.Conn
}

func dial(t *testing.T, srv *httptest.Server) *wsClient {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/tienlen", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	return &wsClient{t: t, ws: ws}
}

func (c *wsClient) send(m any) {
	c.t.Helper()
	if err := c.ws.WriteJSON(m); err != nil {
		c.t.Fatal(err)
	}
}

// next đọc tới khi gặp tin nhắn loại want.
func (c *wsClient) next(want string) map[string]any {
	c.t.Helper()
	c.ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var m map[string]any
		if err := c.ws.ReadJSON(&m); err != nil {
			c.t.Fatalf("đợi %q: %v", want, err)
		}
		if m["t"] == want {
			return m
		}
		if m["t"] == "error" && want != "error" {
			c.t.Fatalf("server báo lỗi: %v", m["msg"])
		}
	}
}

func (c *wsClient) state() online.State {
	c.t.Helper()
	b, _ := json.Marshal(c.next("state"))
	var s online.State
	json.Unmarshal(b, &s)
	return s
}

func TestWebSocketRequiresAccount(t *testing.T) {
	srv, _ := newTestServer(t)
	c := dial(t, srv)
	c.send(online.Message{T: "create", Token: "sai"})
	c.next("auth")
}

// Hai tài khoản + một máy chơi trọn một ván qua WebSocket thật; xu được ghi vào ví SQLite.
func TestTienLenGameSettlesIntoWallet(t *testing.T) {
	srv, w := newTestServer(t)
	a, b := newGuest(t, srv, "An"), newGuest(t, srv, "Bình")

	host := dial(t, srv)
	host.send(online.Message{T: "create", Token: a.Token})
	code := host.next("session")["code"].(string)

	other := dial(t, srv)
	other.send(online.Message{T: "join", Code: strings.ToLower(code), Token: b.Token})
	other.next("session")

	host.send(online.Message{T: "addBot"})
	host.send(online.Message{T: "setBet", Bet: 100})
	host.send(online.Message{T: "start"})

	s := host.state()
	for s.Phase == online.PhaseLobby {
		s = host.state()
	}
	clients := map[string]*wsClient{s.Players[0].ID: host, s.Players[1].ID: other}

	for moves := 0; ; moves, s = moves+1, host.state() {
		if moves > 300 {
			t.Fatal("ván không kết thúc")
		}
		if s.Phase == online.PhaseOver {
			break
		}
		cur := clients[s.Players[s.Turn].ID]
		if cur == nil {
			continue // lượt máy, đợi trạng thái tiếp theo
		}
		cur.send(online.Message{T: "hint"})
		hint := cur.next("hint")
		if hint["cards"] == nil {
			cur.send(online.Message{T: "pass"})
			continue
		}
		var cards []int
		for _, v := range hint["cards"].([]any) {
			cards = append(cards, int(v.(float64)))
		}
		cur.send(online.Message{T: "play", Cards: cards})
	}

	for i, g := range []guest{a, b} {
		want := max(0, wallet.StartBalance+int64(s.Money[i]))
		u, _ := w.User(context.Background(), g.User.ID)
		if u.Balance != want || s.Players[i].Coins != want {
			t.Fatalf("%s: ví %d, hiển thị %d, muốn %d", g.User.Name, u.Balance, s.Players[i].Coins, want)
		}
		hist, _ := w.History(context.Background(), g.User.ID, 5)
		if s.Money[i] != 0 && (hist[0].Game != "tienlen" || !strings.Contains(hist[0].Note, code)) {
			t.Fatalf("%s: sổ cái thiếu ván Tiến Lên: %+v", g.User.Name, hist)
		}
	}
}
