package arena

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type armoryEnv struct {
	t     *testing.T
	w     *wallet.Wallet
	a     *Armory
	uid   int64
	token string
	next  int // số quay của lần cường hoá tới (0..99)
}

func newArmory(t *testing.T, balance int64) *armoryEnv {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "arena.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	a, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	e := &armoryEnv{t: t, w: w, a: a}
	a.roll = func() int { return e.next }
	u, token, err := w.Register(ctx, "xathu", "matkhau123", wallet.Profile{Name: "Xạ thủ"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid, e.token = u.ID, token
	e.setBalance(balance)
	return e
}

func (e *armoryEnv) setBalance(b int64) {
	e.t.Helper()
	cur, err := e.w.Balance(ctx, e.uid)
	if err != nil {
		e.t.Fatal(err)
	}
	tx, _ := e.w.Begin(ctx)
	defer tx.Rollback()
	if b > cur {
		err = tx.Credit(e.uid, b-cur, "test", "nạp")
	} else if b < cur {
		err = tx.Charge(e.uid, cur-b, "test", "rút")
	}
	if err != nil {
		e.t.Fatal(err)
	}
	tx.Commit()
}

func (e *armoryEnv) balance() int64 {
	b, _ := e.w.Balance(ctx, e.uid)
	return b
}

func (e *armoryEnv) setLevel(slot string, lv int) {
	tx, _ := e.w.Begin(ctx)
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO arena_gear (user_id, slot, level) VALUES (?, ?, ?)
		ON CONFLICT (user_id, slot) DO UPDATE SET level = excluded.level`, e.uid, slot, lv); err != nil {
		e.t.Fatal(err)
	}
	tx.Commit()
}

func TestRateFormula(t *testing.T) {
	cases := [][3]int{{0, 1, 95}, {0, 2, 100}, {3, 1, 65}, {3, 2, 77}, {9, 1, 5}, {9, 5, 53}, {8, 1, 15}, {7, 3, 49}}
	for _, c := range cases {
		if got := Rate(c[0], c[1]); got != c[2] {
			t.Fatalf("Rate(+%d, đá %d) = %d, phải %d", c[0], c[1], got, c[2])
		}
	}
	if Fee(0) != 50 || Fee(9) != 500 {
		t.Fatal("phí 50·(L+1)")
	}
}

func TestBuyStonesAndUpgrade(t *testing.T) {
	e := newArmory(t, 1000)
	if _, err := e.a.BuyStones(ctx, e.uid, 6, 1); !errors.Is(err, ErrBadStone) {
		t.Fatalf("đá cấp 6: %v", err)
	}
	if _, err := e.a.BuyStones(ctx, e.uid, 1, 0); !errors.Is(err, ErrBadQty) {
		t.Fatalf("số lượng 0: %v", err)
	}
	if _, err := e.a.BuyStones(ctx, e.uid, 3, 2); !errors.Is(err, wallet.ErrNotEnoughCoins) {
		t.Fatalf("thiếu xu: %v", err)
	}
	inv, err := e.a.BuyStones(ctx, e.uid, 2, 3)
	if err != nil || inv.Stones[2] != 3 || e.balance() != 100 {
		t.Fatalf("mua 3 Đá Lam: %+v %v, ví %d", inv.Stones, err, e.balance())
	}
	if _, _, err := e.a.Upgrade(ctx, e.uid, "giay", 2, false, false); !errors.Is(err, ErrBadSlot) {
		t.Fatalf("ô sai: %v", err)
	}
	if _, _, err := e.a.Upgrade(ctx, e.uid, SlotGun, 1, false, false); !errors.Is(err, ErrNoStone) {
		t.Fatalf("hết Đá Lục: %v", err)
	}
	// Thành công: lên +1, mất 1 đá + phí 50.
	e.next = 0
	inv, out, err := e.a.Upgrade(ctx, e.uid, SlotGun, 2, false, false)
	if err != nil || !out.OK || out.Level != 1 || out.From != 0 || out.Rate != 100 || inv.Gear.Gun != 1 || inv.Stones[2] != 2 || e.balance() != 50 {
		t.Fatalf("cường hoá thành công: %+v %+v %v ví %d", out, inv, err, e.balance())
	}
	// Thất bại ở +1 (tỉ lệ 85+12=97, quay 99): tụt về +0, vẫn mất đá + phí.
	e.next = 99
	e.setBalance(100)
	inv, out, err = e.a.Upgrade(ctx, e.uid, SlotGun, 2, false, false)
	if err != nil || out.OK || !out.Dropped || out.Level != 0 || out.From != 1 || inv.Gear.Gun != 0 || inv.Stones[2] != 1 || e.balance() != 0 {
		t.Fatalf("thất bại tụt cấp: %+v %+v %v ví %d", out, inv, err, e.balance())
	}
	// Thất bại ở +0 (Đá Lục 95%): giữ 0, không tụt.
	e.setBalance(150)
	e.a.BuyStones(ctx, e.uid, 1, 1)
	_, out, err = e.a.Upgrade(ctx, e.uid, SlotGun, 1, false, false)
	if err != nil || out.OK || out.Dropped || out.Level != 0 {
		t.Fatalf("cấp 0 không tụt: %+v %v", out, err)
	}
	// Thiếu xu phí: không mất đá.
	e.setBalance(100)
	inv, _ = e.a.BuyStones(ctx, e.uid, 1, 1)
	e.setBalance(10)
	if _, _, err := e.a.Upgrade(ctx, e.uid, SlotHat, 1, false, false); !errors.Is(err, wallet.ErrNotEnoughCoins) {
		t.Fatalf("thiếu phí: %v", err)
	}
	if inv, _ := e.a.Inventory(ctx, e.uid); inv.Stones[1] != 1 {
		t.Fatalf("thiếu phí thì không mất đá: %+v", inv.Stones)
	}
	// Cấp tối đa: 409.
	e.setLevel(SlotArmor, MaxLevel)
	e.setBalance(1000)
	if _, _, err := e.a.Upgrade(ctx, e.uid, SlotArmor, 1, false, false); !errors.Is(err, ErrMaxed) {
		t.Fatalf("đã max: %v", err)
	}
	if r := reply(inv, 0); len(r.Rates[SlotGun]) != 5 || r.Fee[SlotGun] != 50 {
		t.Fatalf("reply: %+v", r)
	}
}

func TestCharmsLuckProtectEvolve(t *testing.T) {
	e := newArmory(t, 100000)
	if _, err := e.a.BuyCharms(ctx, e.uid, "x", 1); !errors.Is(err, ErrBadCharm) {
		t.Fatalf("bùa sai: %v", err)
	}
	e.a.BuyStones(ctx, e.uid, 1, 10)
	e.setLevel(SlotHat, 5) // tỉ lệ đá Lục ở +5: 45%
	if _, _, err := e.a.Upgrade(ctx, e.uid, SlotHat, 1, true, false); !errors.Is(err, ErrNoCharm) {
		t.Fatalf("chưa có bùa may mắn: %v", err)
	}
	if _, _, err := e.a.Upgrade(ctx, e.uid, SlotHat, 1, false, true); !errors.Is(err, ErrNoCharm) {
		t.Fatalf("chưa có bùa giữ cấp: %v", err)
	}
	before := e.balance()
	inv, err := e.a.BuyCharms(ctx, e.uid, CharmLuck, 2)
	if err != nil || inv.Charms[CharmLuck] != 2 || e.balance() != before-3000 {
		t.Fatalf("mua bùa may mắn: %+v %v", inv.Charms, err)
	}
	e.a.BuyCharms(ctx, e.uid, CharmProtect, 1)
	// Bùa may mắn +15%: quay 50 → 50 < 45+15 thành công; bùa bị tiêu.
	e.next = 50
	inv, out, err := e.a.Upgrade(ctx, e.uid, SlotHat, 1, true, false)
	if err != nil || !out.OK || out.Rate != 60 || out.Level != 6 || inv.Charms[CharmLuck] != 1 {
		t.Fatalf("bùa may mắn: %+v %+v %v", out, inv.Charms, err)
	}
	// Lên +7: biến đổi.
	e.next = 0
	_, out, err = e.a.Upgrade(ctx, e.uid, SlotHat, 1, false, false)
	if err != nil || !out.OK || out.Level != 7 || !out.Evolved {
		t.Fatalf("lên +7 phải biến đổi: %+v %v", out, err)
	}
	// Thất bại có bùa giữ cấp: giữ +7, tiêu bùa.
	e.next = 99
	inv, out, err = e.a.Upgrade(ctx, e.uid, SlotHat, 1, false, true)
	if err != nil || out.OK || out.Dropped || !out.Protected || out.Level != 7 || inv.Gear.Hat != 7 || inv.Charms[CharmProtect] != 0 {
		t.Fatalf("giữ cấp nhờ bùa: %+v %+v %v", out, inv, err)
	}
	// Bùa giữ cấp dùng cả khi thành công.
	e.a.BuyCharms(ctx, e.uid, CharmProtect, 1)
	e.next = 0
	inv, out, _ = e.a.Upgrade(ctx, e.uid, SlotHat, 1, false, true)
	if !out.OK || out.Evolved || inv.Charms[CharmProtect] != 0 {
		t.Fatalf("thành công vẫn tiêu bùa: %+v %+v", out, inv.Charms)
	}
	// +9 → +10: biến đổi lần hai; thất bại từ +7 không bùa tụt +6.
	e.setLevel(SlotGun, 9)
	_, out, _ = e.a.Upgrade(ctx, e.uid, SlotGun, 1, false, false)
	if !out.OK || out.Level != 10 || !out.Evolved {
		t.Fatalf("lên +10: %+v", out)
	}
	e.setLevel(SlotArmor, 7)
	e.next = 99
	_, out, _ = e.a.Upgrade(ctx, e.uid, SlotArmor, 1, false, false)
	if out.OK || !out.Dropped || out.Level != 6 {
		t.Fatalf("+7 thất bại tụt +6: %+v", out)
	}
}

func TestGearAPI(t *testing.T) {
	e := newArmory(t, 5000)
	srv := httptest.NewServer(e.a.Handler())
	defer srv.Close()
	call := func(method, path, token string, body any, out any) int {
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
	var maps []MapInfo
	if code := call("GET", "/api/arena/maps", "", nil, &maps); code != 200 || len(maps) != 5 {
		t.Fatalf("maps: %d %v", code, maps)
	}
	if code := call("GET", "/api/arena/gear", "", nil, nil); code != 401 {
		t.Fatalf("chưa đăng nhập: %d", code)
	}
	var g GearReply
	if code := call("GET", "/api/arena/gear", e.token, nil, &g); code != 200 || g.Power != 1000 || g.Stats.MaxHP != 200 ||
		g.Balance != 5000 || len(g.StoneShop) != 5 || len(g.CharmShop) != 2 || g.LuckBonus != 15 || g.Stones["1"] != 0 ||
		g.Rates[SlotGun][0] != 95 || g.Fee[SlotHat] != 50 {
		t.Fatalf("GET gear: %d %+v", code, g)
	}
	if code := call("POST", "/api/arena/stones/buy", e.token, map[string]int{"tier": 9, "qty": 1}, nil); code != 400 {
		t.Fatalf("đá sai: %d", code)
	}
	if code := call("POST", "/api/arena/stones/buy", e.token, map[string]int{"tier": 5, "qty": 2}, nil); code != 402 {
		t.Fatalf("thiếu xu: %d", code)
	}
	if code := call("POST", "/api/arena/stones/buy", e.token, map[string]int{"tier": 1, "qty": 2}, &g); code != 200 || g.Stones["1"] != 2 || g.Balance != 4800 {
		t.Fatalf("mua đá: %d %+v", code, g.Stones)
	}
	if code := call("POST", "/api/arena/charms/buy", e.token, map[string]any{"id": "luck", "qty": 1}, &g); code != 200 || g.Charms["luck"] != 1 {
		t.Fatalf("mua bùa: %d %+v", code, g.Charms)
	}
	if code := call("POST", "/api/arena/gear/upgrade", e.token, map[string]any{"slot": "gun", "stone": 1, "protect": true}, nil); code != 409 {
		t.Fatalf("hết bùa giữ cấp: %d", code)
	}
	e.next = 0
	if code := call("POST", "/api/arena/gear/upgrade", e.token, map[string]any{"slot": "gun", "stone": 1, "luck": true}, &g); code != 200 ||
		g.Result == nil || !g.Result.OK || g.Result.Rate != 100 || g.Gear.Gun != 1 || g.Power != 1120 || g.Charms["luck"] != 0 {
		t.Fatalf("cường hoá: %d %+v %+v", code, g.Result, g.Gear)
	}
	if code := call("POST", "/api/arena/gear/upgrade", e.token, map[string]any{"slot": "x", "stone": 1}, nil); code != 400 {
		t.Fatalf("ô sai: %d", code)
	}
}
