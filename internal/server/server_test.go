package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
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
		return online.User{ID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Level: u.Level}, err
	}
	tl := online.NewHub(w, online.Options{BotDelay: time.Millisecond, TurnTimeout: time.Minute, Grace: time.Minute})
	static := fstest.MapFS{"index.html": {Data: []byte("hub")}}
	srv := httptest.NewServer(New(w, static, []Game{{
		ID: online.GameID, Name: "Tiến Lên", Path: "/tienlen/",
		Stats: func() any { return tl.Stats() }, WS: tl.Handler(auth), Rooms: tl.RoomsHandler(),
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

type session struct {
	Token string      `json:"token"`
	User  wallet.User `json:"user"`
}

func creds(username, password string) map[string]string {
	return map[string]string{"username": username, "password": password, "name": username}
}

func newAccount(t *testing.T, srv *httptest.Server, username string) session {
	t.Helper()
	var g session
	if code := call(t, srv, "POST", "/api/register", "", creds(username, "matkhau123"), &g); code != 200 || g.Token == "" {
		t.Fatalf("đăng ký %s: %d %+v", username, code, g)
	}
	return g
}

func TestRegisterLoginLogout(t *testing.T) {
	srv, _ := newTestServer(t)
	cases := []struct {
		body map[string]string
		want int
	}{
		{creds("a", "matkhau123"), 400}, // tên quá ngắn
		{creds("binh", "123"), 400},     // mật khẩu yếu
		{map[string]string{}, 400},      // thiếu hết
	}
	for _, tc := range cases {
		if code := call(t, srv, "POST", "/api/register", "", tc.body, nil); code != tc.want {
			t.Errorf("đăng ký %v: muốn %d, được %d", tc.body, tc.want, code)
		}
	}
	a := newAccount(t, srv, "binh")
	if code := call(t, srv, "POST", "/api/register", "", creds("BINH", "matkhau123"), nil); code != 409 {
		t.Fatalf("trùng tên phải 409, được %d", code)
	}

	var s session
	if code := call(t, srv, "POST", "/api/login", "", creds("Binh", "matkhau123"), &s); code != 200 || s.User.ID != a.User.ID {
		t.Fatalf("đăng nhập: %d %+v", code, s)
	}
	if code := call(t, srv, "POST", "/api/logout", s.Token, nil, nil); code != 200 {
		t.Fatalf("đăng xuất: %d", code)
	}
	if code := call(t, srv, "GET", "/api/me", s.Token, nil, nil); code != 401 {
		t.Fatalf("token đã đăng xuất phải 401, được %d", code)
	}
	if code := call(t, srv, "GET", "/api/me", a.Token, nil, nil); code != 200 {
		t.Fatalf("phiên khác vẫn phải dùng được, được %d", code)
	}

	var u wallet.User
	// Giới tính gửi kèm bị bỏ qua: không đổi được sau khi tạo.
	if code := call(t, srv, "POST", "/api/character", a.Token, map[string]string{"gender": "female", "outfit": "sport"}, &u); code != 200 ||
		u.Outfit != "sport" || u.Gender != a.User.Gender {
		t.Fatalf("mặc trang phục: %d %+v", code, u)
	}
	if code := call(t, srv, "POST", "/api/character", a.Token, map[string]string{"outfit": "suit"}, nil); code != 403 {
		t.Fatalf("mặc trang phục chưa có phải 403, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/character", a.Token, map[string]string{"outfit": "bikini"}, nil); code != 400 {
		t.Fatalf("trang phục lạ phải 400, được %d", code)
	}

	if code := call(t, srv, "POST", "/api/password", a.Token, map[string]string{"old": "sai", "new": "moi123456"}, nil); code != 401 {
		t.Fatalf("đổi mật khẩu với mật khẩu cũ sai phải 401, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/password", a.Token, map[string]string{"old": "matkhau123", "new": "moi123456"}, nil); code != 200 {
		t.Fatalf("đổi mật khẩu: %d", code)
	}
	if code := call(t, srv, "POST", "/api/login", "", creds("binh", "moi123456"), nil); code != 200 {
		t.Fatalf("đăng nhập bằng mật khẩu mới: %d", code)
	}
}

func TestOutfitShopAPI(t *testing.T) {
	srv, w := newTestServer(t)
	if code := call(t, srv, "POST", "/api/register", "", map[string]string{"username": "binh", "password": "matkhau123", "outfit": "aodai"}, nil); code != 400 {
		t.Fatalf("đăng ký với trang phục phải mua phải 400, được %d", code)
	}
	a := newAccount(t, srv, "binh")
	buy := func(outfit string, out any) int {
		return call(t, srv, "POST", "/api/outfits/buy", a.Token, map[string]string{"outfit": outfit}, out)
	}
	var e map[string]string
	if code := buy("hoodie", &e); code != 403 || e["error"] != "cần level 2 để mua" {
		t.Fatalf("thiếu level: %d %v", code, e)
	}
	if code := buy("bikini", nil); code != 400 {
		t.Fatalf("trang phục lạ phải 400, được %d", code)
	}
	w.AwardXP(context.Background(), map[int64]int64{a.User.ID: wallet.LevelStart(6)})
	if code := buy("royal", nil); code != 402 {
		t.Fatalf("thiếu xu phải 402, được %d", code)
	}
	var u wallet.User
	if code := buy("suit", nil); code != 402 { // 1.500 xu > 1.000 xu ban đầu
		t.Fatalf("suit khi chỉ có 1.000 xu phải 402, được %d", code)
	}
	if code := buy("hoodie", &u); code != 200 || u.Outfit != "hoodie" || u.Balance != wallet.StartBalance-800 || !slices.Contains(u.Owned, "hoodie") {
		t.Fatalf("mua hoodie: %d %+v", code, u)
	}
	if code := buy("hoodie", nil); code != 409 {
		t.Fatalf("mua lại phải 409, được %d", code)
	}

	var cfg struct {
		Catalog []wallet.OutfitInfo `json:"catalog"`
		Outfits []string            `json:"outfits"`
	}
	call(t, srv, "GET", "/api/config", "", nil, &cfg)
	if len(cfg.Catalog) != 6 || cfg.Catalog[5].ID != "royal" || cfg.Catalog[5].Level != 6 || cfg.Catalog[5].Price != 5000 || !cfg.Catalog[0].Starter || len(cfg.Outfits) != 6 {
		t.Fatalf("catalog trong config: %+v", cfg)
	}
}

func TestLoginLockoutAfterFailures(t *testing.T) {
	srv, _ := newTestServer(t)
	newAccount(t, srv, "binh")
	for i := range 5 {
		if code := call(t, srv, "POST", "/api/login", "", creds("binh", "sai"), nil); code != 401 {
			t.Fatalf("lần sai %d: muốn 401, được %d", i+1, code)
		}
	}
	// Bị khoá tạm thời, kể cả khi nhập đúng.
	if code := call(t, srv, "POST", "/api/login", "", creds("binh", "matkhau123"), nil); code != 429 {
		t.Fatalf("sau 5 lần sai phải 429, được %d", code)
	}
}

func TestAccountAPI(t *testing.T) {
	srv, _ := newTestServer(t)
	g := newAccount(t, srv, "an.nguyen")
	if g.User.Balance != wallet.StartBalance {
		t.Fatalf("số dư ban đầu %d", g.User.Balance)
	}
	if code := call(t, srv, "GET", "/api/me", "sai", nil, nil); code != 401 {
		t.Fatalf("token sai phải 401, được %d", code)
	}
	var u wallet.User
	call(t, srv, "POST", "/api/me", g.Token, map[string]string{"name": "An Béo"}, &u)
	if u.Name != "An Béo" || u.Username != "an.nguyen" {
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
	a, b := newAccount(t, srv, "an.nguyen"), newAccount(t, srv, "binh")

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

	for i, g := range []session{a, b} {
		want := max(0, wallet.StartBalance+int64(s.Money[i]))
		u, _ := w.User(context.Background(), g.User.ID)
		if u.Balance != want || s.Players[i].Coins != want {
			t.Fatalf("%s: ví %d, hiển thị %d, muốn %d", g.User.Name, u.Balance, s.Players[i].Coins, want)
		}
		if u.XP < 10 || u.XP > 30 {
			t.Fatalf("%s: XP sau một ván phải trong 10–30, được %d", g.User.Name, u.XP)
		}
		if u.XP < 10 || u.XP > 30 {
			t.Fatalf("%s: XP sau một ván phải trong 10–30, được %d", g.User.Name, u.XP)
		}
		hist, _ := w.History(context.Background(), g.User.ID, 5)
		if s.Money[i] != 0 && (hist[0].Game != "tienlen" || !strings.Contains(hist[0].Note, code)) {
			t.Fatalf("%s: sổ cái thiếu ván Tiến Lên: %+v", g.User.Name, hist)
		}
	}
}

func TestTienLenRoomsAPI(t *testing.T) {
	srv, _ := newTestServer(t)
	var rooms []online.RoomSummary
	if code := call(t, srv, "GET", "/api/tienlen/rooms", "", nil, &rooms); code != 200 || len(rooms) != 0 {
		t.Fatalf("chưa có bàn: %d %+v", code, rooms)
	}
	a := newAccount(t, srv, "an.nguyen")
	host := dial(t, srv)
	host.send(online.Message{T: "create", Token: a.Token, Bet: 200})
	code := host.next("session")["code"].(string)

	if status := call(t, srv, "GET", "/api/tienlen/rooms", "", nil, &rooms); status != 200 || len(rooms) != 1 {
		t.Fatalf("danh sách bàn: %d %+v", status, rooms)
	}
	r := rooms[0]
	if r.Code != code || r.Bet != 200 || r.Phase != online.PhaseLobby || len(r.Players) != 1 || !r.Players[0].Host || r.Players[0].Name != "an.nguyen" {
		t.Fatalf("bàn sai: %+v", r)
	}
}
