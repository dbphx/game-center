package farm

import (
	"context"
	"encoding/json"
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
	// Thu hoạch không cộng xu nữa: cà rốt vào kho sản phẩm, chỉ cộng XP.
	if g.Coins != 0 || g.XP != 2 || g.Items["carrot"] != 1 || u.Balance != 950 || u.XP != 2 || s.Plots[0].Crop != "" ||
		s.Storage.Products.Items["carrot"] != 1 || s.Storage.Products.Used != 1 {
		t.Fatalf("thu hoạch: %+v, số dư %d, XP %d, ô %+v, kho %+v", g, u.Balance, u.XP, s.Plots[0], s.Storage.Products)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Mua hạt Cà rốt" || h[0].Delta != -50 {
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
	if g.Count != 2 || g.Coins != 0 || g.XP != 4 || g.Items["carrot"] != 2 || s.Plots[2].Crop != "corn" ||
		s.Storage.Products.Items["carrot"] != 2 || e.user().XP != 4 {
		t.Fatalf("thu hoạch tất cả: %+v, ô ngô %+v", g, s.Plots[2])
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
	s := e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn)) // chuồng mở từ level 1
	_, err := e.f.BuyStock(ctx, e.uid, placeBarn, "pig")
	wantStatus(t, err, http.StatusForbidden) // heo cần level 2
	e.set(100_000, levelXP(2))
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

func TestSeedStore(t *testing.T) {
	e := newEnv(t, 10_000, 0)
	s := e.ok(e.f.State(ctx, e.uid))
	if sd := s.Storage.Seeds; sd.Capacity != 50 || sd.Used != 0 || sd.MaxCapacity != 300 || sd.NextExpandPrice != 1000 || sd.Items == nil {
		t.Fatalf("kho giống mới: %+v", sd)
	}
	if p := s.Storage.Products; p.Capacity != 50 || p.MaxCapacity != 500 || p.NextExpandPrice != 1500 || p.Items == nil {
		t.Fatalf("kho sản phẩm mới: %+v", p)
	}
	s = e.ok(e.f.BuySeeds(ctx, e.uid, "carrot", 5))
	if s.Storage.Seeds.Items["carrot"] != 5 || s.Storage.Seeds.Used != 5 || e.user().Balance != 10_000-250 {
		t.Fatalf("mua 5 hạt: %+v, số dư %d", s.Storage.Seeds, e.user().Balance)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Mua 5 hạt Cà rốt" || h[0].Delta != -250 {
		t.Fatalf("sổ cái: %+v", h[0])
	}
	s = e.ok(e.f.BuySeeds(ctx, e.uid, "corn", 0)) // qty thiếu = 1
	if s.Storage.Seeds.Items["corn"] != 1 {
		t.Fatalf("mua 1 hạt ngô: %+v", s.Storage.Seeds)
	}
	_, err := e.f.BuySeeds(ctx, e.uid, "carrot", 101)
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.BuySeeds(ctx, e.uid, "durian", 1)
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.BuySeeds(ctx, e.uid, "tomato", 1)
	wantStatus(t, err, http.StatusForbidden) // cà chua cần level 2
	_, err = e.f.BuySeeds(ctx, e.uid, "carrot", 45)
	wantStatus(t, err, http.StatusConflict) // 6 + 45 > 50

	// Gieo dùng hạt trong kho, không mất xu.
	bal := e.user().Balance
	s = e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	if s.Storage.Seeds.Items["carrot"] != 4 || e.user().Balance != bal {
		t.Fatalf("gieo từ kho: %+v, số dư %d→%d", s.Storage.Seeds, bal, e.user().Balance)
	}
	// Hết hạt ngô sau lần gieo đầu; lần sau tự mua một hạt.
	s = e.ok(e.f.Plant(ctx, e.uid, 1, "corn"))
	if _, ok := s.Storage.Seeds.Items["corn"]; ok || e.user().Balance != bal {
		t.Fatalf("gieo hạt ngô cuối: %+v", s.Storage.Seeds)
	}
	e.ok(e.f.Plant(ctx, e.uid, 2, "corn"))
	if e.user().Balance != bal-80 {
		t.Fatalf("hết hạt thì tự mua: số dư %d, muốn %d", e.user().Balance, bal-80)
	}
}

func TestProductStoreFullAndSell(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 2, "corn"))
	// Kho sản phẩm còn đúng 2 chỗ.
	tx, err := e.w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE farm_storage SET capacity = 2 WHERE user_id = ? AND store = 'products'`, e.uid); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(time.Hour)
	s, g, err := e.f.HarvestAll(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if g.Count != 2 || s.Storage.Products.Used != 2 {
		t.Fatalf("thu hoạch tới khi đầy: %+v, kho %+v", g, s.Storage.Products)
	}
	ripeLeft := 0
	for _, p := range s.Plots[:3] {
		if p.Crop != "" {
			ripeLeft++
		}
	}
	if ripeLeft != 1 {
		t.Fatalf("muốn còn 1 ô chín chưa thu, còn %d", ripeLeft)
	}
	_, _, err = e.f.HarvestAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // kho đầy
	for _, p := range s.Plots[:3] {
		if p.Crop != "" {
			_, _, err = e.f.Harvest(ctx, e.uid, p.Index)
			wantStatus(t, err, http.StatusConflict)
		}
	}

	_, _, err = e.f.SellProduct(ctx, e.uid, "durian", 1)
	wantStatus(t, err, http.StatusBadRequest)
	_, _, err = e.f.SellProduct(ctx, e.uid, "watermelon", 0)
	wantStatus(t, err, http.StatusConflict) // không có trong kho
	_, _, err = e.f.SellProduct(ctx, e.uid, "carrot", 99)
	wantStatus(t, err, http.StatusConflict) // không đủ

	items := s.Storage.Products.Items
	item := "carrot"
	if items["carrot"] == 0 {
		item = "corn"
	}
	_, price, _ := product(item)
	bal := e.user().Balance
	s, g, err = e.f.SellProduct(ctx, e.uid, item, 1)
	if err != nil || g.Coins != price || g.Count != 1 || e.user().Balance != bal+price || s.Storage.Products.Used != 1 {
		t.Fatalf("bán 1 %s: %+v %v, kho %+v", item, g, err, s.Storage.Products)
	}
	xp := e.user().XP
	s, g, err = e.f.SellAllProducts(ctx, e.uid)
	if err != nil || g.Count != 1 || s.Storage.Products.Used != 0 || len(s.Storage.Products.Items) != 0 || e.user().XP != xp {
		t.Fatalf("bán hết: %+v %v, kho %+v", g, err, s.Storage.Products)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if !strings.HasPrefix(h[0].Note, "Bán 1 ") || h[0].Delta != g.Coins {
		t.Fatalf("sổ cái: %+v", h[0])
	}
	_, _, err = e.f.SellAllProducts(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // kho trống
}

func TestSellProductAllOfOneItem(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	for i := range 3 {
		e.ok(e.f.Plant(ctx, e.uid, i, "carrot"))
	}
	e.clock = e.clock.Add(time.Hour)
	if _, _, err := e.f.HarvestAll(ctx, e.uid); err != nil {
		t.Fatal(err)
	}
	s, g, err := e.f.SellProduct(ctx, e.uid, "carrot", 0) // qty 0 = bán hết
	if err != nil || g.Count != 3 || g.Coins != 270 || s.Storage.Products.Used != 0 {
		t.Fatalf("bán hết cà rốt: %+v %v", g, err)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Bán 3 Cà rốt" || h[0].Delta != 270 {
		t.Fatalf("sổ cái: %+v", h[0])
	}
}

func TestCollectStock(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(2))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	s := e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))
	id := s.Barn.Animals[0].ID
	_, _, err := e.f.CollectStock(ctx, e.uid, placeBarn, id)
	wantStatus(t, err, http.StatusConflict) // chưa lớn
	_, _, err = e.f.CollectStock(ctx, e.uid, placeBarn, 99999)
	wantStatus(t, err, http.StatusNotFound)
	_, _, err = e.f.CollectStock(ctx, e.uid, placePond, id)
	wantStatus(t, err, http.StatusConflict) // chưa có ao

	e.clock = e.clock.Add(time.Hour)
	before := e.user()
	s, g, err := e.f.CollectStock(ctx, e.uid, placeBarn, id)
	if err != nil {
		t.Fatal(err)
	}
	after := e.user()
	if g.Coins != 0 || g.XP != 12 || g.Items["pig"] != 1 || after.Balance != before.Balance || after.XP != before.XP+12 ||
		len(s.Barn.Animals) != 0 || s.Barn.Used != 0 || s.Storage.Products.Items["pig"] != 1 {
		t.Fatalf("thu heo: %+v, chuồng %+v, kho %+v", g, s.Barn, s.Storage.Products)
	}
	s, g, err = e.f.SellProduct(ctx, e.uid, "pig", 1)
	if err != nil || g.Coins != 1300 {
		t.Fatalf("bán heo từ kho: %+v %v", g, err)
	}
}

func TestExpandStores(t *testing.T) {
	e := newEnv(t, 10_000_000, 0)
	for _, c := range []struct {
		store      string
		max, steps int
		base       int64
	}{{storeSeeds, 300, 10, 1000}, {storeProducts, 500, 18, 1500}} {
		s := e.ok(e.f.State(ctx, e.uid))
		spent, steps := int64(0), 0
		for st := storeOf(s, c.store); st.Capacity < c.max; st = storeOf(s, c.store) {
			steps++
			if want := c.base * int64(steps); st.NextExpandPrice != want {
				t.Fatalf("%s lần %d: muốn giá %d, được %d", c.store, steps, want, st.NextExpandPrice)
			}
			spent += st.NextExpandPrice
			bal := e.user().Balance
			s = e.ok(e.f.ExpandStore(ctx, e.uid, c.store))
			if e.user().Balance != bal-st.NextExpandPrice {
				t.Fatalf("%s: trừ sai xu", c.store)
			}
		}
		if st := storeOf(s, c.store); steps != c.steps || st.Capacity != c.max || st.NextExpandPrice != 0 {
			t.Fatalf("%s: %d lần, kho %+v", c.store, steps, st)
		}
		_, err := e.f.ExpandStore(ctx, e.uid, c.store)
		wantStatus(t, err, http.StatusConflict)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Mở rộng kho sản phẩm" {
		t.Fatalf("sổ cái: %+v", h[0])
	}
}

func TestFeedStock(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(2))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	s := e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig")) // heo: 600 xu, lớn sau 3600 s
	a := s.Barn.Animals[0]
	if a.Hungry || a.FedAt != 0 {
		t.Fatalf("heo mới mua chưa đói: %+v", a)
	}
	_, _, err := e.f.FeedStock(ctx, e.uid, placeBarn, a.ID)
	wantStatus(t, err, http.StatusConflict) // chưa đói
	if !strings.Contains(err.Error(), "Heo chưa đói") || !strings.Contains(err.Error(), "10 phút") {
		t.Fatalf("thông báo: %v", err)
	}
	_, _, err = e.f.FeedStock(ctx, e.uid, placeBarn, 99999)
	wantStatus(t, err, http.StatusNotFound)

	e.clock = e.clock.Add(FeedCooldown * time.Second)
	s = e.ok(e.f.State(ctx, e.uid))
	if !s.Barn.Animals[0].Hungry {
		t.Fatalf("sau %d s heo phải đói: %+v", FeedCooldown, s.Barn.Animals[0])
	}
	before := e.user()
	now := e.clock.Unix()
	s, g, err := e.f.FeedStock(ctx, e.uid, placeBarn, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := e.user()
	fed := s.Barn.Animals[0]
	wantReady := a.ReadyAt - (a.ReadyAt-now)/4 // còn 3000 s → bớt 750 s
	if g.XP != 1 || after.Balance != before.Balance-60 || after.XP != before.XP+1 || fed.ReadyAt != wantReady ||
		fed.ReadyAt != a.ReadyAt-750 || fed.FedAt != now || fed.Hungry {
		t.Fatalf("cho heo ăn: %+v, con %+v, số dư %d→%d", g, fed, before.Balance, after.Balance)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Cho Heo ăn" || h[0].Delta != -60 {
		t.Fatalf("sổ cái: %+v", h[0])
	}
	e.clock = e.clock.Add(4 * time.Minute)
	_, _, err = e.f.FeedStock(ctx, e.uid, placeBarn, a.ID)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "6 phút") {
		t.Fatalf("chưa đói lại: %v", err)
	}

	// Đã lớn thì không cho ăn nữa.
	e.clock = e.clock.Add(time.Hour)
	_, _, err = e.f.FeedStock(ctx, e.uid, placeBarn, a.ID)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "đã lớn") {
		t.Fatalf("con đã lớn: %v", err)
	}
	if s = e.ok(e.f.State(ctx, e.uid)); s.Barn.Animals[0].Hungry {
		t.Fatal("con đã lớn không còn đói")
	}
}

func TestFeedFishNotEnoughCoins(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(2))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placePond))
	s := e.ok(e.f.BuyStock(ctx, e.uid, placePond, "catfish")) // 250 xu → thức ăn 25 xu, lớn sau 1800 s
	id := s.Pond.Fish[0].ID
	e.clock = e.clock.Add(FeedCooldown * time.Second)
	e.set(24, e.user().XP)
	_, _, err := e.f.FeedStock(ctx, e.uid, placePond, id)
	wantStatus(t, err, http.StatusPaymentRequired)
	if s = e.ok(e.f.State(ctx, e.uid)); s.Pond.Fish[0].FedAt != 0 || e.user().Balance != 24 {
		t.Fatalf("thiếu xu mà vẫn cho ăn: %+v", s.Pond.Fish[0])
	}
	e.set(25, e.user().XP)
	if _, _, err = e.f.FeedStock(ctx, e.uid, placePond, id); err != nil || e.user().Balance != 0 {
		t.Fatalf("cho cá ăn 25 xu: %v, số dư %d", err, e.user().Balance)
	}
	_, _, err = e.f.FeedStock(ctx, e.uid, placeBarn, id)
	wantStatus(t, err, http.StatusConflict) // chưa có chuồng
}

func TestCatalogFeed(t *testing.T) {
	c := Catalog()
	if c["feedCooldown"] != FeedCooldown {
		t.Fatalf("feedCooldown: %v", c["feedCooldown"])
	}
	b, _ := json.Marshal(c["animals"])
	if !strings.Contains(string(b), `"id":"cow"`) || !strings.Contains(string(b), `"feedPrice":150`) {
		t.Fatalf("catalog vật nuôi: %s", b)
	}
	b, _ = json.Marshal(c["fish"])
	if !strings.Contains(string(b), `"feedPrice":10`) || (Stock{Price: 30}).feedPrice() != 5 {
		t.Fatalf("catalog cá: %s", b)
	}
}

// Database từ bản cũ: farm_stock chưa có cột fed_at → New thêm cột, giữ dữ liệu cũ.
func TestMigrateFedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	w, err := wallet.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := w.Register(ctx, "nongdancu", "matkhau123", wallet.Profile{Name: "Cũ"})
	if err != nil {
		t.Fatal(err)
	}
	old := `CREATE TABLE farm_stock (
		id INTEGER PRIMARY KEY AUTOINCREMENT, user_id INTEGER NOT NULL REFERENCES users(id),
		place TEXT NOT NULL, kind TEXT NOT NULL, bought_at INTEGER NOT NULL, ready_at INTEGER NOT NULL);
	CREATE TABLE farm_buildings (user_id INTEGER NOT NULL, place TEXT NOT NULL, capacity INTEGER NOT NULL,
		expansions INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (user_id, place));`
	if err := w.ExecSchema(ctx, old); err != nil {
		t.Fatal(err)
	}
	tx, err := w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx.ExecContext(ctx, `INSERT INTO farm_buildings (user_id, place, capacity) VALUES (?, 'barn', 5)`, u.ID)
	tx.ExecContext(ctx, `INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at) VALUES (?, 'barn', 'pig', 100, 9999999999)`, u.ID)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w.Close()

	for range 2 { // chạy hai lần: lần sau không thêm cột trùng
		w, err = wallet.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := New(ctx, w)
		if err != nil {
			t.Fatal(err)
		}
		s, err := f.State(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if a := s.Barn.Animals; len(a) != 1 || a[0].Kind != "pig" || a[0].FedAt != 0 || !a[0].Hungry {
			t.Fatalf("heo cũ sau nâng cấp: %+v", a)
		}
		w.Close()
	}
}
