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
	_, err = e.f.BuyStock(ctx, e.uid, placeBarn, "pig")
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
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))
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
	if g.Coins != 0 || g.XP != 12 || g.Items["meat_pig"] != 1 || after.Balance != before.Balance || after.XP != before.XP+12 ||
		len(s.Barn.Animals) != 0 || s.Barn.Used != 0 || s.Storage.Products.Items["meat_pig"] != 1 {
		t.Fatalf("thu heo: %+v, chuồng %+v, kho %+v", g, s.Barn, s.Storage.Products)
	}
	s, g, err = e.f.SellProduct(ctx, e.uid, "meat_pig", 1)
	if err != nil || g.Coins != 1300 {
		t.Fatalf("bán thịt heo từ kho: %+v %v", g, err)
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
	_, _, err = e.f.FeedStock(ctx, e.uid, placeBarn, a.ID)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "Hết Cám gia súc, mua thêm ở cửa hàng") {
		t.Fatalf("hết thức ăn: %v", err)
	}
	s = e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_livestock", 3))
	if s.Storage.Seeds.Items["feed_livestock"] != 3 || s.Storage.Seeds.Used != 3 {
		t.Fatalf("mua cám: %+v", s.Storage.Seeds)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Mua 3 Cám gia súc" || h[0].Delta != -150 {
		t.Fatalf("sổ cái: %+v", h[0])
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
	if g.XP != 1 || g.Used["feed_livestock"] != 1 || after.Balance != before.Balance || after.XP != before.XP+1 ||
		fed.ReadyAt != wantReady || fed.ReadyAt != a.ReadyAt-750 || fed.FedAt != now || fed.Hungry ||
		s.Storage.Seeds.Items["feed_livestock"] != 2 {
		t.Fatalf("cho heo ăn: %+v, con %+v, số dư %d→%d", g, fed, before.Balance, after.Balance)
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

func TestFeedPortionsAndStore(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(5))
	_, err := e.f.BuyFeeds(ctx, e.uid, "feed_cat", 1)
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.f.BuyFeeds(ctx, e.uid, "feed_fish", 101)
	wantStatus(t, err, http.StatusBadRequest)
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placePond))
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "cow"))
	e.ok(e.f.BuyStock(ctx, e.uid, placePond, "koi"))
	e.ok(e.f.BuyStock(ctx, e.uid, placePond, "catfish")) // rô phi lớn sau 600 s nên dùng cá trê
	s := e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "duck"))
	cow, koi, tilapia, duck := s.Barn.Animals[0].ID, s.Pond.Fish[0].ID, s.Pond.Fish[1].ID, s.Yard.Animals[0].ID
	e.clock = e.clock.Add(FeedCooldown * time.Second)

	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_livestock", 1))
	_, _, err = e.f.FeedStock(ctx, e.uid, placeBarn, cow)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "Hết Cám gia súc") {
		t.Fatalf("bò ăn 2 phần, có 1: %v", err)
	}
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_livestock", 1))
	if s, g, err := e.f.FeedStock(ctx, e.uid, placeBarn, cow); err != nil || g.Used["feed_livestock"] != 2 ||
		s.Storage.Seeds.Items["feed_livestock"] != 0 {
		t.Fatalf("bò ăn 2 phần: %+v %v", g, err)
	}

	bal := e.user().Balance
	s = e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_fish", 3))
	if e.user().Balance != bal-30 {
		t.Fatalf("mua 3 phần thức ăn cá: số dư %d→%d", bal, e.user().Balance)
	}
	if _, g, err := e.f.FeedStock(ctx, e.uid, placePond, koi); err != nil || g.Used["feed_fish"] != 2 {
		t.Fatalf("koi ăn 2 phần: %+v %v", g, err)
	}
	if s, g, err := e.f.FeedStock(ctx, e.uid, placePond, tilapia); err != nil || g.Used["feed_fish"] != 1 ||
		s.Storage.Seeds.Items["feed_fish"] != 0 {
		t.Fatalf("cá trê ăn 1 phần: %+v %v", g, err)
	}
	// Thức ăn gà vịt riêng: có cám / thức ăn cá cũng không cho vịt ăn được.
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_livestock", 5))
	_, _, err = e.f.FeedStock(ctx, e.uid, placeYard, duck)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "Hết Thóc cho gà vịt") {
		t.Fatalf("vịt: %v", err)
	}
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_poultry", 0)) // qty thiếu = 1
	if _, g, err := e.f.FeedStock(ctx, e.uid, placeYard, duck); err != nil || g.Used["feed_poultry"] != 1 {
		t.Fatalf("vịt ăn thóc: %+v %v", g, err)
	}

	// Thức ăn chiếm chỗ kho giống: kho 50, đang có 5 cám.
	_, err = e.f.BuyFeeds(ctx, e.uid, "feed_fish", 46)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "kho giống đầy") {
		t.Fatalf("kho đầy: %v", err)
	}
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_fish", 45))
	_, err = e.f.BuySeeds(ctx, e.uid, "carrot", 1)
	wantStatus(t, err, http.StatusConflict)
	e.set(0, e.user().XP)
	e.exec(`DELETE FROM farm_items WHERE user_id = ? AND item = 'feed_fish'`, e.uid)
	_, err = e.f.BuyFeeds(ctx, e.uid, "feed_fish", 1)
	wantStatus(t, err, http.StatusPaymentRequired)
}

func TestCatalogFeed(t *testing.T) {
	c := Catalog()
	if c["feedCooldown"] != FeedCooldown {
		t.Fatalf("feedCooldown: %v", c["feedCooldown"])
	}
	b, _ := json.Marshal(c["animals"])
	if !strings.Contains(string(b), `"id":"cow","name":"Bò","level":4,"units":2,"price":1500,"growSeconds":10800,"sellPrice":3400,"xp":35,"feed":"feed_livestock","portions":2,"feedPrice":100`) ||
		!strings.Contains(string(b), `"feed":"feed_livestock","portions":1,"feedPrice":50`) ||
		strings.Contains(string(b), `"chicken"`) ||
		!strings.Contains(string(b), `"product":null,"meat":{"id":"meat_pig","name":"Thịt heo","sellPrice":1300,"xp":12}`) ||
		!strings.Contains(string(b), `"product":{"id":"milk","name":"Sữa bò","every":1200,"sellPrice":300,"xp":5}`) {
		t.Fatalf("catalog vật nuôi: %s", b)
	}
	b, _ = json.Marshal(c["poultry"])
	if !strings.Contains(string(b), `{"id":"duck","name":"Vịt","level":1,"units":1,"price":250,"growSeconds":1200,"sellPrice":520,"xp":6,"feed":"feed_poultry","portions":1,"feedPrice":15,`+
		`"product":{"id":"duck_egg","name":"Trứng vịt","every":900,"sellPrice":80,"xp":2},"meat":{"id":"meat_duck","name":"Thịt vịt","sellPrice":520,"xp":6}}`) ||
		!strings.Contains(string(b), `"id":"chicken"`) {
		t.Fatalf("catalog gia cầm: %s", b)
	}
	if y := c["yard"].(Building); y.Capacity != 10 || y.ExpandUnits != 5 || y.MaxCapacity != 40 || y.Price != 0 {
		t.Fatalf("catalog sân: %+v", y)
	}
	b, _ = json.Marshal(c["fish"])
	if strings.Contains(string(b), `"meat"`) || strings.Contains(string(b), `"product"`) {
		t.Fatalf("cá không có sản phẩm / thịt: %s", b)
	}
	if !strings.Contains(string(b), `"feed":"feed_fish","portions":1,"feedPrice":10`) ||
		!strings.Contains(string(b), `"id":"koi","name":"Cá Koi","level":5,"units":3,"price":2000,"growSeconds":14400,"sellPrice":4800,"xp":50,"feed":"feed_fish","portions":2,"feedPrice":20`) {
		t.Fatalf("catalog cá: %s", b)
	}
	b, _ = json.Marshal(c["feeds"])
	if string(b) != `[{"id":"feed_poultry","name":"Thóc cho gà vịt","price":15,"for":"poultry"},`+
		`{"id":"feed_livestock","name":"Cám gia súc","price":50,"for":"barn"},{"id":"feed_fish","name":"Thức ăn cá","price":10,"for":"pond"}]` {
		t.Fatalf("catalog thức ăn: %s", b)
	}
}

// Database từ bản cũ: farm_stock chưa có cột fed_at / produced_at → New thêm cột, giữ dữ liệu cũ.
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
		if a := s.Barn.Animals; len(a) != 1 || a[0].Kind != "pig" || a[0].FedAt != 0 || !a[0].Hungry || a[0].ProductReadyAt != 0 {
			t.Fatalf("heo cũ sau nâng cấp: %+v", a)
		}
		w.Close()
	}
}

func TestPoultryInYard(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	s := e.ok(e.f.State(ctx, e.uid))
	if y := s.Yard; y == nil || y.Capacity != 10 || y.MaxCapacity != 40 || y.NextExpandPrice != 1000 || y.Animals == nil || s.Barn != nil {
		t.Fatalf("sân gia cầm có sẵn: %+v, chuồng %+v", s.Yard, s.Barn)
	}
	_, err := e.f.BuyBuilding(ctx, e.uid, placeYard)
	wantStatus(t, err, http.StatusConflict) // đã có sẵn
	e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "chicken"))
	s = e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "duck")) // gọi kiểu cũ /animals/buy → vào sân
	if s.Yard.Used != 2 || len(s.Yard.Animals) != 2 || s.Yard.Animals[1].Kind != "duck" ||
		s.Yard.Animals[1].ReadyAt != s.Yard.Animals[1].BoughtAt+1200 || s.Barn != nil {
		t.Fatalf("gà + vịt: %+v", s.Yard)
	}
	_, err = e.f.BuyStock(ctx, e.uid, placeYard, "pig")
	wantStatus(t, err, http.StatusBadRequest) // heo không thả sân
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	_, err = e.f.BuyStock(ctx, e.uid, placeBarn, "koi")
	wantStatus(t, err, http.StatusBadRequest)
	for range 8 {
		e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "chicken"))
	}
	_, err = e.f.BuyStock(ctx, e.uid, placeYard, "chicken")
	wantStatus(t, err, http.StatusConflict) // sân đầy 10/10
	prices := []int64{}
	for s = e.ok(e.f.State(ctx, e.uid)); s.Yard.Capacity < 40; s = e.ok(e.f.ExpandBuilding(ctx, e.uid, placeYard)) {
		prices = append(prices, s.Yard.NextExpandPrice)
	}
	if len(prices) != 6 || prices[0] != 1000 || prices[5] != 6000 || s.Yard.NextExpandPrice != 0 {
		t.Fatalf("giá mở rộng sân: %v", prices)
	}

	e.clock = e.clock.Add(1200 * time.Second)
	duck := s.Yard.Animals[1].ID
	s, g, err := e.f.CollectStock(ctx, e.uid, placeBarn, duck) // id gia cầm qua /animals/collect cũng tìm thấy
	if err != nil || g.XP != 6 || s.Storage.Products.Items["meat_duck"] != 1 {
		t.Fatalf("thu vịt: %+v %v", g, err)
	}
	if _, g, err = e.f.SellProduct(ctx, e.uid, "meat_duck", 1); err != nil || g.Coins != 520 {
		t.Fatalf("bán vịt: %+v %v", g, err)
	}
	_, _, err = e.f.FeedStock(ctx, e.uid, placeYard, 99999)
	wantStatus(t, err, http.StatusNotFound)
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func TestProduceCycle(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	s := e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "chicken")) // gà thả sân (không cần chuồng), lớn sau 900 s, trứng mỗi 600 s
	a := s.Yard.Animals[0]
	if a.ProductReadyAt != a.ReadyAt+600 || a.ProductReady {
		t.Fatalf("gà mới: %+v", a)
	}
	_, _, err := e.f.ProduceStock(ctx, e.uid, placeYard, a.ID)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "chưa lớn") {
		t.Fatalf("gà chưa lớn: %v", err)
	}
	e.clock = e.clock.Add(900 * time.Second) // vừa lớn, trứng chưa có
	_, _, err = e.f.ProduceStock(ctx, e.uid, placeYard, a.ID)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "Trứng gà chưa có, chờ thêm 10 phút") {
		t.Fatalf("trứng chưa có: %v", err)
	}
	e.clock = e.clock.Add(3 * 600 * time.Second) // để lâu vẫn chỉ 1 trứng
	s = e.ok(e.f.State(ctx, e.uid))
	if !s.Yard.Animals[0].ProductReady {
		t.Fatalf("trứng phải sẵn sàng: %+v", s.Yard.Animals[0])
	}
	xp := e.user().XP
	s, g, err := e.f.ProduceStock(ctx, e.uid, placeYard, a.ID)
	if err != nil || g.XP != 2 || g.Items["egg"] != 1 || e.user().XP != xp+2 || s.Storage.Products.Items["egg"] != 1 {
		t.Fatalf("lấy trứng: %+v %v", g, err)
	}
	now := e.clock.Unix()
	if b := s.Yard.Animals[0]; b.ProductReady || b.ProductReadyAt != now+600 {
		t.Fatalf("sau khi lấy chờ lại 600 s: %+v", b)
	}
	_, _, err = e.f.ProduceStock(ctx, e.uid, placeYard, a.ID)
	wantStatus(t, err, http.StatusConflict) // không cộng dồn
	e.clock = e.clock.Add(600 * time.Second)
	s, _, err = e.f.ProduceStock(ctx, e.uid, placeYard, a.ID)
	if err != nil || s.Storage.Products.Items["egg"] != 2 {
		t.Fatalf("trứng thứ hai: %v %+v", err, s.Storage.Products)
	}

	// Kho đầy thì không lấy được.
	e.clock = e.clock.Add(600 * time.Second)
	e.exec(`UPDATE farm_storage SET capacity = 2 WHERE user_id = ? AND store = 'products'`, e.uid)
	_, _, err = e.f.ProduceStock(ctx, e.uid, placeYard, a.ID)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "kho sản phẩm đầy") {
		t.Fatalf("kho đầy: %v", err)
	}
	_, _, err = e.f.ProduceStock(ctx, e.uid, placeYard, 99999)
	wantStatus(t, err, http.StatusNotFound)

	// Bán trứng, rồi lấy thịt gà.
	if _, g, err = e.f.SellProduct(ctx, e.uid, "egg", 0); err != nil || g.Coins != 120 || g.Count != 2 {
		t.Fatalf("bán 2 trứng: %+v %v", g, err)
	}
	s, g, err = e.f.CollectStock(ctx, e.uid, placeYard, a.ID)
	if err != nil || g.XP != 5 || g.Items["meat_chicken"] != 1 || len(s.Yard.Animals) != 0 {
		t.Fatalf("lấy thịt gà: %+v %v", g, err)
	}
	if _, g, err = e.f.SellProduct(ctx, e.uid, "meat_chicken", 1); err != nil || g.Coins != 400 {
		t.Fatalf("bán thịt gà: %+v %v", g, err)
	}
}

func TestPigHasNoProduct(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(2))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	s := e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))
	if a := s.Barn.Animals[0]; a.ProductReadyAt != 0 || a.ProductReady {
		t.Fatalf("heo không có sản phẩm: %+v", a)
	}
	e.clock = e.clock.Add(10 * time.Hour)
	_, _, err := e.f.ProduceStock(ctx, e.uid, placeBarn, s.Barn.Animals[0].ID)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "không cho sản phẩm") {
		t.Fatalf("heo: %v", err)
	}
}

func TestSellNewAndOldProducts(t *testing.T) {
	e := newEnv(t, 0, 0)
	e.ok(e.f.State(ctx, e.uid))
	// Kho từ bản cũ còn con nguyên con ("pig", "chicken"); món mới: trứng, sữa, lông, thịt.
	want := map[string]int64{
		"egg": 60, "duck_egg": 80, "milk": 300, "wool": 400,
		"meat_chicken": 400, "meat_duck": 520, "meat_pig": 1300, "meat_sheep": 2000, "meat_cow": 3400,
		"pig": 1300, "chicken": 400, "tilapia": 220, "carrot": 90,
	}
	for item := range want {
		e.exec(`INSERT INTO farm_items (user_id, store, item, qty) VALUES (?, 'products', ?, 1)`, e.uid, item)
	}
	for item, price := range want {
		bal := e.user().Balance
		if _, g, err := e.f.SellProduct(ctx, e.uid, item, 1); err != nil || g.Coins != price || e.user().Balance != bal+price {
			t.Fatalf("bán %s: %+v %v, muốn %d xu", item, g, err, price)
		}
	}
	h, _ := e.w.History(ctx, e.uid, 20)
	notes := ""
	for _, x := range h {
		notes += x.Note + ";"
	}
	for _, n := range []string{"Bán 1 Sữa bò", "Bán 1 Lông cừu", "Bán 1 Thịt bò", "Bán 1 Trứng vịt"} {
		if !strings.Contains(notes, n) {
			t.Fatalf("sổ cái thiếu %q: %s", n, notes)
		}
	}
}

// Bản cũ nuôi gà vịt trong chuồng: khởi động lại thì chuyển ra sân, sân đủ chỗ cho cả đàn.
func TestMigratePoultryToYard(t *testing.T) {
	e := newEnv(t, 0, 0)
	e.exec(`INSERT INTO farm_buildings (user_id, place, capacity) VALUES (?, 'barn', 30)`, e.uid)
	for i := range 12 {
		kind := "chicken"
		if i%3 == 0 {
			kind = "duck"
		}
		e.exec(`INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at) VALUES (?, 'barn', ?, 0, 0)`, e.uid, kind)
	}
	e.exec(`INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at) VALUES (?, 'barn', 'pig', 0, 0)`, e.uid)
	for range 2 { // chạy lại không đổi gì thêm
		f, err := New(ctx, e.w)
		if err != nil {
			t.Fatal(err)
		}
		f.now = e.f.now
		s := e.ok(f.State(ctx, e.uid))
		if len(s.Yard.Animals) != 12 || s.Yard.Used != 12 || s.Yard.Capacity != 12 ||
			len(s.Barn.Animals) != 1 || s.Barn.Animals[0].Kind != "pig" || s.Barn.Used != 1 {
			t.Fatalf("sau nâng cấp: sân %+v, chuồng %+v", s.Yard, s.Barn)
		}
	}
}

func TestPlantAll(t *testing.T) {
	e := newEnv(t, 230, 0)
	_, _, err := e.f.PlantAll(ctx, e.uid, "tomato")
	wantStatus(t, err, http.StatusForbidden) // cà chua cần level 2
	_, _, err = e.f.PlantAll(ctx, e.uid, "durian")
	wantStatus(t, err, http.StatusBadRequest)
	e.ok(e.f.BuySeeds(ctx, e.uid, "carrot", 2)) // còn 130 xu
	e.ok(e.f.Plant(ctx, e.uid, 0, "corn"))      // 80 xu → còn 50
	// 5 ô trống: 2 hạt trong kho + 1 hạt mua được (50 xu) = gieo 3 ô.
	s, g, err := e.f.PlantAll(ctx, e.uid, "carrot")
	if err != nil {
		t.Fatal(err)
	}
	planted := 0
	for _, p := range s.Plots {
		if p.Crop == "carrot" {
			planted++
		}
	}
	if g.Count != 3 || g.Spent != 50 || planted != 3 || e.user().Balance != 0 || len(s.Storage.Seeds.Items) != 0 {
		t.Fatalf("gieo nhanh: %+v, %d ô cà rốt, số dư %d, kho %+v", g, planted, e.user().Balance, s.Storage.Seeds)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Mua 1 hạt Cà rốt" || h[0].Delta != -50 {
		t.Fatalf("sổ cái: %+v", h[0])
	}
	_, _, err = e.f.PlantAll(ctx, e.uid, "carrot")
	wantStatus(t, err, http.StatusConflict) // hết hạt, hết xu
	e.set(10_000, 0)
	if _, g, err = e.f.PlantAll(ctx, e.uid, "carrot"); err != nil || g.Count != 2 {
		t.Fatalf("gieo nốt 2 ô: %+v %v", g, err)
	}
	_, _, err = e.f.PlantAll(ctx, e.uid, "carrot")
	wantStatus(t, err, http.StatusConflict) // không còn ô trống
}

func TestFeedAll(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(4))
	_, _, err := e.f.FeedAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // chưa có con nào
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placePond))
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "cow"))     // 2 phần cám
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))     // 1 phần cám
	e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "chicken")) // 1 phần thóc
	e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "duck"))    // 1 phần thóc
	e.ok(e.f.BuyStock(ctx, e.uid, placePond, "catfish")) // 1 phần thức ăn cá
	_, _, err = e.f.FeedAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // chưa con nào đói
	e.clock = e.clock.Add(FeedCooldown * time.Second)
	_, _, err = e.f.FeedAll(ctx, e.uid)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "Hết thức ăn") {
		t.Fatalf("chưa có thức ăn: %v", err)
	}
	// 1 phần cám: bò (2 phần) không đủ, heo ăn được; 1 phần thóc: chỉ gà; không có thức ăn cá.
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_livestock", 1))
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_poultry", 1))
	bal, xp := e.user().Balance, e.user().XP
	s, g, err := e.f.FeedAll(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if g.Count != 2 || g.XP != 2 || g.Used["feed_livestock"] != 1 || g.Used["feed_poultry"] != 1 || g.Spent != 0 ||
		e.user().Balance != bal || e.user().XP != xp+2 || len(s.Storage.Seeds.Items) != 0 ||
		!s.Barn.Animals[0].Hungry || s.Barn.Animals[1].Hungry || s.Yard.Animals[0].Hungry || !s.Yard.Animals[1].Hungry ||
		!s.Pond.Fish[0].Hungry {
		t.Fatalf("cho ăn hàng loạt: %+v, chuồng %+v, sân %+v", g, s.Barn.Animals, s.Yard.Animals)
	}
	_, _, err = e.f.FeedAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // hết thức ăn
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_livestock", 2))
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_poultry", 1))
	e.ok(e.f.BuyFeeds(ctx, e.uid, "feed_fish", 5))
	if s, g, err = e.f.FeedAll(ctx, e.uid); err != nil || g.Count != 3 || g.Used["feed_fish"] != 1 ||
		s.Pond.Fish[0].Hungry || s.Barn.Animals[0].Hungry || s.Storage.Seeds.Items["feed_fish"] != 4 {
		t.Fatalf("cho nốt bò, vịt, cá: %+v %v", g, err)
	}
}

func TestProduceAllAndCollectAllFish(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(4))
	_, _, err := e.f.ProduceAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	_, _, err = e.f.CollectAllFish(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // chưa có ao
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placePond))
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "cow"))
	e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))
	e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "chicken"))
	e.ok(e.f.BuyStock(ctx, e.uid, placeYard, "chicken"))
	e.ok(e.f.BuyStock(ctx, e.uid, placePond, "tilapia"))
	e.ok(e.f.BuyStock(ctx, e.uid, placePond, "tilapia"))
	e.ok(e.f.BuyStock(ctx, e.uid, placePond, "carp"))
	e.clock = e.clock.Add(4 * time.Hour) // bò lớn sau 3 giờ + sữa 20 phút; gà có trứng; cá đều đã lớn
	s, g, err := e.f.ProduceAll(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if g.Count != 3 || g.Items["egg"] != 2 || g.Items["milk"] != 1 || g.XP != 2+2+5 || s.Storage.Products.Used != 3 {
		t.Fatalf("lấy sản phẩm hàng loạt: %+v", g)
	}
	_, _, err = e.f.ProduceAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict) // vừa lấy, chưa có mới

	// Kho còn 2 chỗ: thu được 2 trong 3 con cá.
	e.exec(`UPDATE farm_storage SET capacity = 5 WHERE user_id = ? AND store = 'products'`, e.uid)
	s, g, err = e.f.CollectAllFish(ctx, e.uid)
	if err != nil || g.Count != 2 || len(s.Pond.Fish) != 1 || s.Storage.Products.Used != 5 {
		t.Fatalf("thu cá hàng loạt: %+v %v, ao %+v", g, err, s.Pond.Fish)
	}
	_, _, err = e.f.CollectAllFish(ctx, e.uid)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "kho sản phẩm đầy") {
		t.Fatalf("kho đầy: %v", err)
	}
	e.clock = e.clock.Add(time.Hour)
	_, _, err = e.f.ProduceAll(ctx, e.uid)
	if wantStatus(t, err, http.StatusConflict); !strings.Contains(err.Error(), "kho sản phẩm đầy") {
		t.Fatalf("kho đầy: %v", err)
	}
}
