package server

import (
	"context"
	"database/sql"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/games/tienlen/online"
	"github.com/dbphx/game-center/internal/wallet"
)

// newHPServer dựng hub có Casino (chặn theo máu) và Nông trại, trả kèm đường dẫn database để test chỉnh máu.
func newHPServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hp.db")
	w, err := wallet.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	auth := func(ctx context.Context, token string) (online.User, error) {
		u, err := w.UserByToken(ctx, token)
		return online.User{ID: u.ID, Name: u.Name, HP: u.HP, HospitalUntil: u.HospitalUntil}, err
	}
	tl := online.NewHub(w, online.Options{BotDelay: time.Millisecond, TurnTimeout: time.Minute, Grace: time.Minute,
		MinHP: wallet.ZoneMinHP["casino"]})
	f, err := farm.New(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(w, fstest.MapFS{}, []Game{
		{ID: online.GameID, Name: "Tiến Lên", Path: "/tienlen/", WS: tl.Handler(auth), Rooms: tl.RoomsHandler()},
		{ID: farm.GameID, Name: "Nông trại", Path: "/farm/", API: f.Handler()},
	}))
	t.Cleanup(srv.Close)
	return srv, path
}

// setHP chỉnh thẳng máu trong database (hp_at = bây giờ nên chưa kịp hồi).
func setHP(t *testing.T, path, username string, hp int, hospitalUntil int64) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE users SET hp = ?, hp_at = ?, hospital_until = ? WHERE username = ?`,
		hp, time.Now().Unix(), hospitalUntil, username); err != nil {
		t.Fatal(err)
	}
}

func TestHPAndHospitalAPI(t *testing.T) {
	srv, path := newHPServer(t)
	a := newAccount(t, srv, "binh")

	var cfg struct {
		ZoneMinHP map[string]int `json:"zoneMinHP"`
		Hospital  map[string]int `json:"hospital"`
	}
	call(t, srv, "GET", "/api/config", "", nil, &cfg)
	if cfg.ZoneMinHP["casino"] != 30 || cfg.ZoneMinHP["farm"] != 40 || cfg.ZoneMinHP["shop"] != 10 ||
		cfg.Hospital["dischargePrice"] != 200 || cfg.Hospital["carDamage"] != 20 || cfg.Hospital["stayMinutes"] != 5 {
		t.Fatalf("config máu / bệnh viện sai: %+v", cfg)
	}

	var u wallet.User
	if code := call(t, srv, "POST", "/api/hp/hit", a.Token, map[string]string{"source": "rock"}, nil); code != 400 {
		t.Fatalf("nguồn lạ phải 400, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/hp/hit", a.Token, map[string]string{"source": "car"}, &u); code != 200 || u.HP != 80 || u.MaxHP != 100 || u.Now == 0 {
		t.Fatalf("bị xe đụng: %d %+v", code, u)
	}
	if code := call(t, srv, "POST", "/api/hospital/heal", a.Token, nil, &u); code != 200 || u.HP != 100 || u.Balance != wallet.StartBalance-40 {
		t.Fatalf("chữa 20 HP: %d %+v", code, u)
	}
	if code := call(t, srv, "POST", "/api/hospital/heal", a.Token, nil, nil); code != 409 {
		t.Fatalf("đầy máu phải 409, được %d", code)
	}

	// Đang nằm viện: không bị đụng nữa, xuất viện mất 200 xu; thiếu xu thì 402.
	setHP(t, path, "binh", 0, time.Now().Unix()+300)
	if code := call(t, srv, "POST", "/api/hp/hit", a.Token, map[string]string{"source": "car"}, nil); code != 423 {
		t.Fatalf("nằm viện bị đụng phải 423, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/hospital/heal", a.Token, nil, &u); code != 200 || u.HP != 100 || u.HospitalUntil != 0 ||
		u.Balance != wallet.StartBalance-40-200 {
		t.Fatalf("xuất viện: %d %+v", code, u)
	}
	b := newAccount(t, srv, "ngheo")
	setHP(t, path, "ngheo", 0, time.Now().Unix()+300)
	db, _ := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	db.Exec(`UPDATE users SET balance = 50 WHERE username = 'ngheo'`)
	db.Close()
	if code := call(t, srv, "POST", "/api/hospital/heal", b.Token, nil, nil); code != 402 {
		t.Fatalf("thiếu xu viện phí phải 402, được %d", code)
	}
}

func TestZoneHPGates(t *testing.T) {
	srv, path := newHPServer(t)
	a := newAccount(t, srv, "binh")

	// Shop thời trang cần 10 HP.
	setHP(t, path, "binh", 5, 0)
	var e map[string]string
	if code := call(t, srv, "POST", "/api/character", a.Token, map[string]string{"outfit": "sport"}, &e); code != 403 ||
		e["error"] != "cần ít nhất 10 HP để vào Shop thời trang" {
		t.Fatalf("shop thiếu máu: %d %v", code, e)
	}
	if code := call(t, srv, "POST", "/api/outfits/buy", a.Token, map[string]string{"outfit": "hoodie"}, nil); code != 403 {
		t.Fatalf("mua đồ thiếu máu phải 403, được %d", code)
	}

	// Nông trại: xem được, làm nông cần 40 HP; nằm viện → 423.
	setHP(t, path, "binh", 35, 0)
	if code := call(t, srv, "GET", "/api/farm", a.Token, nil, nil); code != 200 {
		t.Fatalf("xem nông trại luôn được, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/farm/plant", a.Token, map[string]any{"plot": 0, "crop": "carrot"}, &e); code != 403 ||
		e["error"] != "cần ít nhất 40 HP để làm nông" {
		t.Fatalf("làm nông thiếu máu: %d %v", code, e)
	}
	setHP(t, path, "binh", 90, time.Now().Unix()+120)
	if code := call(t, srv, "POST", "/api/farm/plant", a.Token, map[string]any{"plot": 0, "crop": "carrot"}, nil); code != 423 {
		t.Fatalf("nằm viện làm nông phải 423, được %d", code)
	}
	setHP(t, path, "binh", 40, 0)
	if code := call(t, srv, "POST", "/api/farm/plant", a.Token, map[string]any{"plot": 0, "crop": "carrot"}, nil); code != 200 {
		t.Fatalf("đủ 40 HP phải làm nông được, được %d", code)
	}

	// Casino: tạo / vào bàn cần 30 HP, nằm viện thì không vào được.
	setHP(t, path, "binh", 20, 0)
	c := dial(t, srv)
	c.send(online.Message{T: "create", Token: a.Token})
	if msg := c.next("error")["msg"]; msg != "cần ít nhất 30 HP để vào Casino" {
		t.Fatalf("tạo bàn thiếu máu: %v", msg)
	}
	setHP(t, path, "binh", 100, 0)
	c.send(online.Message{T: "create", Token: a.Token})
	code := c.next("session")["code"].(string)

	b := newAccount(t, srv, "chi")
	setHP(t, path, "chi", 100, time.Now().Unix()+200)
	c2 := dial(t, srv)
	c2.send(online.Message{T: "join", Code: code, Token: b.Token})
	if msg := c2.next("error")["msg"]; msg != "bạn đang nằm viện" {
		t.Fatalf("nằm viện vào bàn: %v", msg)
	}
}

// Ngồi ghế: POST /api/me/sit {sit} trả user có sitting / regenSeconds; bị tông thì đứng; nằm viện 423.
func TestSitAPI(t *testing.T) {
	srv, path := newHPServer(t)
	a := newAccount(t, srv, "ngoi")
	var u wallet.User
	if code := call(t, srv, "POST", "/api/me/sit", a.Token, map[string]bool{"sit": true}, &u); code != 200 || !u.Sitting || u.RegenSeconds != 30 {
		t.Fatalf("ngồi: %d %+v", code, u)
	}
	if code := call(t, srv, "POST", "/api/me/sit", a.Token, map[string]bool{"sit": false}, &u); code != 200 || u.Sitting || u.RegenSeconds != 60 {
		t.Fatalf("đứng dậy: %d %+v", code, u)
	}
	call(t, srv, "POST", "/api/me/sit", a.Token, map[string]bool{"sit": true}, nil)
	if code := call(t, srv, "POST", "/api/hp/hit", a.Token, map[string]string{"source": "car"}, &u); code != 200 || u.Sitting {
		t.Fatalf("bị tông phải đứng dậy: %d %+v", code, u)
	}
	setHP(t, path, "ngoi", 0, time.Now().Unix()+wallet.HospitalStay)
	if code := call(t, srv, "POST", "/api/me/sit", a.Token, map[string]bool{"sit": true}, nil); code != 423 {
		t.Fatalf("nằm viện ngồi phải 423, được %d", code)
	}
	if code := call(t, srv, "POST", "/api/me/sit", "", map[string]bool{"sit": true}, nil); code != 401 {
		t.Fatalf("chưa đăng nhập phải 401, được %d", code)
	}
	var cfg struct {
		Sit int `json:"sitRegenMultiplier"`
	}
	if call(t, srv, "GET", "/api/config", "", nil, &cfg); cfg.Sit != 2 {
		t.Fatalf("config sitRegenMultiplier: %+v", cfg)
	}
}

// API riêng của game (nông trại) trả 401 cho token bị đá ra cũng có code "elsewhere".
func TestGameAPIElsewhere(t *testing.T) {
	srv, _ := newHPServer(t)
	first := newAccount(t, srv, "nongdan")
	call(t, srv, "POST", "/api/login", "", creds("nongdan", "matkhau123"), nil)
	var e map[string]string
	if code := call(t, srv, "GET", "/api/farm", first.Token, nil, &e); code != 401 || e["code"] != "elsewhere" {
		t.Fatalf("API nông trại với token cũ: %d %v", code, e)
	}
	e = nil
	if code := call(t, srv, "GET", "/api/farm", "khong-co", nil, &e); code != 401 || e["code"] == "elsewhere" {
		t.Fatalf("API nông trại với token lạ: %d %v", code, e)
	}
}
