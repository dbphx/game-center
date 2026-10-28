package fame

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type env struct {
	t     *testing.T
	w     *wallet.Wallet
	f     *Fame
	clock time.Time
	n     int // số tài khoản đã tạo, để đặt tên đăng nhập
}

func newEnv(t *testing.T) *env {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "fame.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	fm, err := farm.New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, w: w, f: New(w, fm), clock: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)}
	e.f.now = func() time.Time { return e.clock }
	return e
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

// user tạo người chơi có đúng balance xu; trả id và token.
func (e *env) user(name string, balance int64) (int64, string) {
	e.t.Helper()
	e.n++
	u, token, err := e.w.Register(ctx, fmt.Sprintf("nguoi%d", e.n), "matkhau123", wallet.Profile{Name: name})
	if err != nil {
		e.t.Fatal(err)
	}
	e.exec(`UPDATE users SET balance = ? WHERE id = ?`, balance, u.ID)
	return u.ID, token
}

func (e *env) board(uid int64) Board {
	e.t.Helper()
	b, err := e.f.Board(ctx, uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func TestWorthParts(t *testing.T) {
	e := newEnv(t)
	id, _ := e.user("An", 1000)
	// Đồ thời trang: vest 1500 + áo thường ngày (starter, 0) + tóc mohican 1200 + cánh thiên thần 2500.
	e.exec(`INSERT INTO user_outfits (user_id, outfit, created_at) VALUES (?, 'suit', 0)`, id)
	e.exec(`INSERT INTO user_items (user_id, kind, item, created_at) VALUES (?, 'hair', 'mohawk', 0), (?, 'wings', 'angel', 0), (?, 'hair', 'da_xoa', 0)`, id, id, id)
	// Nông trại: 8 ô (2 ô mua: 10000 + 12000), 1 ô đang trồng ngô (hạt 80);
	// chuồng mở rộng 2 lần (3000 + 20000 + 40000); sân gia cầm mở rộng 1 lần (10000); kho giống mở rộng 1 lần (10000);
	// một con bò (mua 1500), một cá rô phi (100); kho: 3 hạt cà rốt (50), 2 cám (50), 4 cà chua (bán 260).
	for i := range 8 {
		crop := ""
		if i == 0 {
			crop = "corn"
		}
		e.exec(`INSERT INTO farm_plots (user_id, idx, crop) VALUES (?, ?, ?)`, id, i, crop)
	}
	e.exec(`INSERT INTO farm_buildings (user_id, place, capacity, expansions) VALUES (?, 'barn', 15, 2), (?, 'yard', 15, 1)`, id, id)
	e.exec(`INSERT INTO farm_storage (user_id, store, capacity, expansions) VALUES (?, 'seeds', 75, 1)`, id)
	e.exec(`INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at) VALUES (?, 'barn', 'cow', 0, 0), (?, 'pond', 'tilapia', 0, 0)`, id, id)
	e.exec(`INSERT INTO farm_items (user_id, store, item, qty) VALUES (?, 'seeds', 'carrot', 3), (?, 'seeds', 'feed_livestock', 2), (?, 'products', 'tomato', 4)`, id, id, id)

	b := e.board(id)
	wantItems := int64(1500 + 1200 + 2500)
	wantFarm := int64(10000 + 12000 + 80 + 3000 + 20000 + 40000 + 10000 + 10000 + 1500 + 100 + 3*50 + 2*50 + 4*260)
	got := b.Top[0]
	if got.Coins != 1000 || got.Items != wantItems || got.Farm != wantFarm || got.Total != 1000+wantItems+wantFarm {
		t.Fatalf("tài sản = %+v, muốn items %d farm %d", got, wantItems, wantFarm)
	}
	if b.Me == nil || b.Me.Rank != 1 || b.Me.Total != got.Total {
		t.Fatalf("me = %+v", b.Me)
	}
}

func TestOrderTiesAndLimit(t *testing.T) {
	e := newEnv(t)
	var ids []int64
	for i := range 12 {
		id, _ := e.user(fmt.Sprintf("P%02d", i), int64(100*(i%6))) // xu 0,100,..500 lặp hai lần → có hoà
		ids = append(ids, id)
	}
	b := e.board(ids[0])
	if len(b.Top) != TopN {
		t.Fatalf("top có %d người", len(b.Top))
	}
	// 500 xu: ids[5] rồi ids[11] (hoà thì id nhỏ trước); 400: ids[4], ids[10]...
	want := []int64{ids[5], ids[11], ids[4], ids[10], ids[3], ids[9], ids[2], ids[8], ids[1], ids[7]}
	for i, e := range b.Top {
		if e.ID != want[i] || e.Rank != i+1 {
			t.Fatalf("hạng %d = id %d rank %d, muốn id %d", i+1, e.ID, e.Rank, want[i])
		}
	}
	// ids[0] có 0 xu, hoà với ids[6] nhưng id nhỏ hơn → hạng 11.
	if b.Me == nil || b.Me.Rank != 11 {
		t.Fatalf("me = %+v, muốn hạng 11", b.Me)
	}
	if e.board(0).Me != nil {
		t.Fatal("khách xem không có me")
	}
}

func TestEmptyAndCache(t *testing.T) {
	e := newEnv(t)
	b := e.board(0)
	if b.Top == nil || len(b.Top) != 0 {
		t.Fatalf("bảng rỗng phải là [], được %#v", b.Top)
	}
	id, _ := e.user("An", 100)
	// Người mới chưa có trong bảng đã tính → tính lại ngay để có hạng.
	if b := e.board(id); b.Me == nil || b.Me.Rank != 1 || len(b.Top) != 1 {
		t.Fatalf("người mới: %+v", b)
	}
	at := e.board(0).UpdatedAt
	e.exec(`UPDATE users SET balance = 999 WHERE id = ?`, id)
	e.clock = e.clock.Add(CacheTTL - time.Second)
	if b := e.board(0); b.Top[0].Coins != 100 || b.UpdatedAt != at {
		t.Fatalf("trong %v phải dùng bảng cũ: %+v", CacheTTL, b)
	}
	e.clock = e.clock.Add(time.Second)
	if b := e.board(0); b.Top[0].Coins != 999 || b.UpdatedAt != e.clock.UnixMilli() {
		t.Fatalf("hết hạn phải tính lại: %+v", b)
	}
}

func TestHandler(t *testing.T) {
	e := newEnv(t)
	_, token := e.user("An", 500)
	e.user("Bình", 900)
	srv := httptest.NewServer(e.f.Handler())
	defer srv.Close()
	get := func(token string) map[string]json.RawMessage {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/fame", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status %d", res.StatusCode)
		}
		var out map[string]json.RawMessage
		json.NewDecoder(res.Body).Decode(&out)
		return out
	}
	if out := get(""); out["me"] != nil || out["top"] == nil || out["updatedAt"] == nil {
		t.Fatalf("không token: %v", out)
	}
	if out := get("token-sai"); out["me"] != nil {
		t.Fatal("token sai không có me")
	}
	var me Entry
	json.Unmarshal(get(token)["me"], &me)
	if me.Name != "An" || me.Rank != 2 || me.Total != 500 {
		t.Fatalf("me = %+v", me)
	}
	var top []map[string]any
	json.Unmarshal(get("")["top"], &top)
	if _, ok := top[0]["username"]; ok {
		t.Fatal("không được lộ tên đăng nhập")
	}
}
