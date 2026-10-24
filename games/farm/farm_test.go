package farm

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type env struct {
	t     *testing.T
	w     *wallet.Wallet
	f     *Farm
	clock time.Time
	uid   int64
}

// newEnv tạo ví + nông trại với đồng hồ giả và một người chơi có xu / XP cho trước.
func newEnv(t *testing.T, balance, xp int64) *env {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "farm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	f, err := New(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, w: w, f: f, clock: time.Unix(1_800_000_000, 0)}
	f.now = func() time.Time { return e.clock }
	u, _, err := w.Register(ctx, "nongdan", "matkhau123", wallet.Profile{Name: "Nông dân"})
	if err != nil {
		t.Fatal(err)
	}
	e.uid = u.ID
	e.set(balance, xp)
	return e
}

func (e *env) set(balance, xp int64) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = ?, xp = ? WHERE id = ?`, balance, xp, e.uid); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) user() wallet.User {
	e.t.Helper()
	u, err := e.w.User(ctx, e.uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) ok(s State, err error) State {
	e.t.Helper()
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

// wantStatus kiểm tra lỗi mang đúng mã HTTP.
func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if err == nil {
		t.Fatalf("muốn lỗi %d, không có lỗi", status)
	}
	if got := errStatus(err); got != status {
		t.Fatalf("muốn mã %d, được %d (%v)", status, got, err)
	}
}

func levelXP(level int) int64 { return wallet.LevelStart(level) }

func TestPlotsPriceGrowsAndStopsAt48(t *testing.T) {
	e := newEnv(t, 10_000_000, 0)
	s := e.ok(e.f.State(ctx, e.uid))
	if len(s.Plots) != FreePlots || s.NextPlotPrice != 10000 || s.MaxPlots != 48 {
		t.Fatalf("nông trại mới: %d ô, giá ô tiếp %d", len(s.Plots), s.NextPlotPrice)
	}
	spent := int64(0)
	for n := FreePlots; n < MaxPlots; n++ {
		want := int64(10000 + 2000*(n-FreePlots))
		if s.NextPlotPrice != want {
			t.Fatalf("ô số %d: muốn giá %d, được %d", n+1, want, s.NextPlotPrice)
		}
		spent += want
		s = e.ok(e.f.BuyPlot(ctx, e.uid))
	}
	if len(s.Plots) != MaxPlots || s.NextPlotPrice != 0 {
		t.Fatalf("sau khi mua hết: %d ô, giá tiếp %d", len(s.Plots), s.NextPlotPrice)
	}
	_, err := e.f.BuyPlot(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	if b := e.user().Balance; b != 10_000_000-spent {
		t.Fatalf("số dư %d, muốn %d", b, 10_000_000-spent)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Game != GameID || h[0].Note != "Mua ô đất số 48" {
		t.Fatalf("sổ cái: %+v", h[0])
	}
}

func TestPlantAndHarvest(t *testing.T) {
	e := newEnv(t, 1000, 0)
	s := e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	if p := s.Plots[0]; p.Crop != "carrot" || p.ReadyAt != p.PlantedAt+120 || e.user().Balance != 950 {
		t.Fatalf("trồng cà rốt: %+v, số dư %d", p, e.user().Balance)
	}
	_, err := e.f.Plant(ctx, e.uid, 0, "corn")
	wantStatus(t, err, http.StatusConflict) // ô đang có cây
	_, err = e.f.Plant(ctx, e.uid, 99, "corn")
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.Plant(ctx, e.uid, 1, "durian")
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.Plant(ctx, e.uid, 1, "tomato")
	wantStatus(t, err, http.StatusForbidden) // cà chua cần level 2

	_, _, err = e.f.Harvest(ctx, e.uid, 0)
	wantStatus(t, err, http.StatusConflict) // chưa chín
	_, _, err = e.f.Harvest(ctx, e.uid, 1)
	wantStatus(t, err, http.StatusConflict) // ô trống

	e.clock = e.clock.Add(120 * time.Second)
	s, g, err := e.f.Harvest(ctx, e.uid, 0)
	if err != nil {
		t.Fatal(err)
	}
	u := e.user()
	if g.Coins != 90 || g.XP != 2 || u.Balance != 1040 || u.XP != 2 || s.Plots[0].Crop != "" {
		t.Fatalf("thu hoạch: %+v, số dư %d, XP %d, ô %+v", g, u.Balance, u.XP, s.Plots[0])
	}
	h, _ := e.w.History(ctx, e.uid, 2)
	if h[0].Note != "Thu hoạch Cà rốt" || h[0].Delta != 90 || h[1].Note != "Mua hạt Cà rốt" || h[1].Delta != -50 {
		t.Fatalf("sổ cái: %+v", h)
	}
}

func TestHarvestAll(t *testing.T) {
	e := newEnv(t, 1000, 0)
	_, _, err := e.f.HarvestAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 2, "corn"))
	e.clock = e.clock.Add(200 * time.Second) // cà rốt chín, ngô chưa
	s, g, err := e.f.HarvestAll(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if g.Count != 2 || g.Coins != 180 || g.XP != 4 || s.Plots[2].Crop != "corn" {
		t.Fatalf("thu hoạch tất cả: %+v, ô ngô %+v", g, s.Plots[2])
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Thu hoạch 2 Cà rốt" || h[0].Delta != 180 {
		t.Fatalf("sổ cái: %+v", h[0])
	}
}

func TestNotEnoughCoinsChangesNothing(t *testing.T) {
	e := newEnv(t, 40, 0)
	_, err := e.f.Plant(ctx, e.uid, 0, "carrot")
	wantStatus(t, err, http.StatusPaymentRequired)
	_, err = e.f.BuyPlot(ctx, e.uid)
	wantStatus(t, err, http.StatusPaymentRequired)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.Plots[0].Crop != "" || len(s.Plots) != FreePlots || e.user().Balance != 40 {
		t.Fatalf("thiếu xu mà vẫn đổi: %+v, số dư %d", s.Plots[0], e.user().Balance)
	}
}

func TestBarnUnitsAndExpand(t *testing.T) {
	e := newEnv(t, 1_000_000, levelXP(4))
	_, err := e.f.BuyStock(ctx, e.uid, placeBarn, "pig")
	wantStatus(t, err, http.StatusConflict) // chưa có chuồng
	_, err = e.f.ExpandBuilding(ctx, e.uid, placeBarn)
	wantStatus(t, err, http.StatusConflict)

	s := e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	if s.Barn == nil || s.Barn.Capacity != 5 || s.Barn.Used != 0 || s.Barn.NextExpandPrice != 2000 {
		t.Fatalf("chuồng mới: %+v", s.Barn)
	}
	_, err = e.f.BuyBuilding(ctx, e.uid, placeBarn)
	wantStatus(t, err, http.StatusConflict) // đã có
	_, err = e.f.BuyStock(ctx, e.uid, placeBarn, "koi")
	wantStatus(t, err, http.StatusBadRequest) // cá không nuôi trong chuồng

	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "cow"))
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "cow"))
	s = e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))
	if s.Barn.Used != 5 || len(s.Barn.Animals) != 3 {
		t.Fatalf("bò 2 đơn vị × 2 + heo 1: used %d, %d con", s.Barn.Used, len(s.Barn.Animals))
	}
	_, err = e.f.BuyStock(ctx, e.uid, placeBarn, "chicken")
	wantStatus(t, err, http.StatusConflict) // đầy

	prices := []int64{}
	for s.Barn.Capacity < 30 {
		prices = append(prices, s.Barn.NextExpandPrice)
		s = e.ok(e.f.ExpandBuilding(ctx, e.uid, placeBarn))
	}
	if len(prices) != 5 || prices[0] != 2000 || prices[4] != 10000 || s.Barn.NextExpandPrice != 0 {
		t.Fatalf("giá mở rộng: %v, giá tiếp %d", prices, s.Barn.NextExpandPrice)
	}
	_, err = e.f.ExpandBuilding(ctx, e.uid, placeBarn)
	wantStatus(t, err, http.StatusConflict)
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "chicken"))
}

func TestSellAnimalAndLevel(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	_, err := e.f.BuyBuilding(ctx, e.uid, placeBarn)
	wantStatus(t, err, http.StatusForbidden) // chuồng cần level 2
	e.set(100_000, levelXP(2))
	s := e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	_, err = e.f.BuyStock(ctx, e.uid, placeBarn, "cow")
	wantStatus(t, err, http.StatusForbidden) // bò cần level 4
	s = e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))
	id := s.Barn.Animals[0].ID

	_, _, err = e.f.SellStock(ctx, e.uid, placeBarn, id)
	wantStatus(t, err, http.StatusConflict) // chưa lớn
	_, _, err = e.f.SellStock(ctx, e.uid, placeBarn, 99999)
	wantStatus(t, err, http.StatusNotFound)

	e.clock = e.clock.Add(time.Hour)
	before := e.user()
	s, g, err := e.f.SellStock(ctx, e.uid, placeBarn, id)
	if err != nil {
		t.Fatal(err)
	}
	after := e.user()
	if g.Coins != 1300 || g.XP != 12 || after.Balance != before.Balance+1300 || after.XP != before.XP+12 || len(s.Barn.Animals) != 0 {
		t.Fatalf("bán heo: %+v, số dư %d→%d", g, before.Balance, after.Balance)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Bán Heo" {
		t.Fatalf("sổ cái: %+v", h[0])
	}
}

func TestPond(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(5))
	_, err := e.f.BuyStock(ctx, e.uid, placePond, "tilapia")
	wantStatus(t, err, http.StatusConflict) // chưa có ao
	s := e.ok(e.f.BuyBuilding(ctx, e.uid, placePond))
	if s.Pond.Capacity != 5 || s.Pond.NextExpandPrice != 1500 || s.Pond.Fish == nil {
		t.Fatalf("ao mới: %+v", s.Pond)
	}
	e.ok(e.f.BuyStock(ctx, e.uid, placePond, "koi"))
	s = e.ok(e.f.BuyStock(ctx, e.uid, placePond, "carp"))
	if s.Pond.Used != 5 {
		t.Fatalf("koi 3 + chép 2: used %d", s.Pond.Used)
	}
	_, err = e.f.BuyStock(ctx, e.uid, placePond, "tilapia")
	wantStatus(t, err, http.StatusConflict)
	s = e.ok(e.f.ExpandBuilding(ctx, e.uid, placePond))
	if s.Pond.Capacity != 10 || s.Pond.NextExpandPrice != 3000 {
		t.Fatalf("ao sau mở rộng: %+v", s.Pond)
	}
	e.clock = e.clock.Add(4 * time.Hour)
	_, g, err := e.f.SellStock(ctx, e.uid, placePond, s.Pond.Fish[0].ID)
	if err != nil || g.Coins != 4800 || g.XP != 50 {
		t.Fatalf("bán cá koi: %+v %v", g, err)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if !strings.HasPrefix(h[0].Note, "Bán Cá Koi") {
		t.Fatalf("sổ cái: %+v", h[0])
	}
}
