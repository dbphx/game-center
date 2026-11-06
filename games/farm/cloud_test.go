package farm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

// fullFarm đưa nông trại của uid lên đủ 48 ô, không tốn xu.
func (e *env) fullFarm(uid int64) {
	e.t.Helper()
	e.ok(e.f.State(ctx, uid))
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	for i := FreePlots; i < MaxPlots; i++ {
		if _, err := tx.ExecContext(ctx, `INSERT INTO farm_plots (user_id, idx) VALUES (?, ?)`, uid, i); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

// openFloors mở thẳng n tầng mây cho người chơi chính.
func (e *env) openFloors(n int) {
	e.t.Helper()
	e.fullFarm(e.uid)
	for range n {
		f := e.ok(e.f.State(ctx, e.uid)).Cloud.Floors
		e.ok(e.f.PlantBean(ctx, e.uid))
		e.clock = e.clock.Add(time.Duration(CloudFloors[f].BeanSeconds) * time.Second)
	}
	e.ok(e.f.State(ctx, e.uid))
}

func (e *env) worth() int64 {
	e.t.Helper()
	w, err := e.f.Worth(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	return w[e.uid]
}

func TestCloudFloorsTable(t *testing.T) {
	want := []struct {
		bean, sec, pot int64
		crop           string
		seed, sell, xp int64
	}{
		{200_000, 24 * 3600, 100_000, "cloud_berry", 5_000, 20_000, 200},
		{1_000_000, 48 * 3600, 500_000, "mist_pear", 20_000, 70_000, 300},
		{3_000_000, 72 * 3600, 2_000_000, "star_grape", 50_000, 200_000, 450},
		{10_000_000, 96 * 3600, 10_000_000, "fairy_peach", 100_000, 600_000, 700},
	}
	if len(CloudFloors) != len(want) {
		t.Fatalf("số tầng: %d", len(CloudFloors))
	}
	for i, w := range want {
		c := CloudFloors[i]
		if c.Floor != i+1 || c.BeanPrice != w.bean || c.BeanSeconds != w.sec || c.PotPrice != w.pot || c.Slots != 6 {
			t.Fatalf("tầng %d: %+v", i+1, c)
		}
		if c.Crop.ID != w.crop || c.Crop.SeedPrice != w.seed || c.Crop.SellPrice != w.sell || c.Crop.XP != w.xp || c.Crop.GrowSeconds != 43200 {
			t.Fatalf("cây tầng %d: %+v", i+1, c.Crop)
		}
	}
	if MaxPlots != 48 {
		t.Fatalf("MaxPlots đổi: %d", MaxPlots)
	}
}

func TestBeanNeeds48Plots(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.Cloud.Available || s.Cloud.Floors != 0 || s.Cloud.Pots == nil {
		t.Fatalf("chưa đủ ô: %+v", s.Cloud)
	}
	_, err := e.f.PlantBean(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	if b := e.user().Balance; b != 100_000_000 {
		t.Fatalf("bị trừ xu: %d", b)
	}

	e.fullFarm(e.uid)
	now := e.clock.Unix()
	s = e.ok(e.f.PlantBean(ctx, e.uid))
	if !s.Cloud.Available || s.Cloud.BeanPlantedAt != now || s.Cloud.BeanReadyAt != now+24*3600 {
		t.Fatalf("sau khi trồng: %+v", s.Cloud)
	}
	if b := e.user().Balance; b != 100_000_000-200_000 {
		t.Fatalf("xu sau khi trồng: %d", b)
	}
}

func TestSecondBeanRefusedWithoutCharge(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.fullFarm(e.uid)
	e.ok(e.f.PlantBean(ctx, e.uid))
	before := e.user().Balance
	_, err := e.f.PlantBean(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	if b := e.user().Balance; b != before {
		t.Fatalf("hạt thứ hai trừ xu: %d", before-b)
	}
}

func TestBeanNotEnoughCoins(t *testing.T) {
	e := newEnv(t, 199_999, 0)
	e.fullFarm(e.uid)
	_, err := e.f.PlantBean(ctx, e.uid)
	wantStatus(t, err, http.StatusPaymentRequired)
}

func TestBeanOpensFloorExactlyWhenReady(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.fullFarm(e.uid)
	e.ok(e.f.PlantBean(ctx, e.uid))

	e.clock = e.clock.Add(24*time.Hour - time.Second)
	if s := e.ok(e.f.State(ctx, e.uid)); s.Cloud.Floors != 0 || s.Cloud.BeanReadyAt == 0 {
		t.Fatalf("trước giây chín: %+v", s.Cloud)
	}
	e.clock = e.clock.Add(time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.Cloud.Floors != 1 || s.Cloud.BeanPlantedAt != 0 || s.Cloud.BeanReadyAt != 0 {
		t.Fatalf("đúng giây chín qua State: %+v", s.Cloud)
	}

	// Qua do: thao tác đầu tiên sau khi chín thấy ngay tầng mới và mua được chậu của tầng đó.
	e.ok(e.f.PlantBean(ctx, e.uid))
	e.clock = e.clock.Add(48 * time.Hour)
	s = e.ok(e.f.BuyPot(ctx, e.uid, 2))
	if s.Cloud.Floors != 2 || len(s.Cloud.Pots) != 1 || s.Cloud.Pots[0].Floor != 2 {
		t.Fatalf("đúng giây chín qua do: %+v", s.Cloud)
	}
}

func TestBeanFloorsAreSequentialAndStopAtFour(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(3)
	if f := e.ok(e.f.State(ctx, e.uid)).Cloud.Floors; f != 3 {
		t.Fatalf("số tầng: %d", f)
	}
	before := e.user().Balance
	e.ok(e.f.PlantBean(ctx, e.uid)) // hạt tầng 4
	if got := before - e.user().Balance; got != 10_000_000 {
		t.Fatalf("hạt tầng 4: %d", got)
	}
	// Hạt vừa chín: cùng yêu cầu chốt tầng 4 rồi mới báo đã đủ, không trồng hạt thứ năm.
	e.clock = e.clock.Add(96 * time.Hour)
	before = e.user().Balance
	_, err := e.f.PlantBean(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	if err.Error() != "đã mở đủ 4 tầng mây" {
		t.Fatalf("thông báo: %v", err)
	}
	if b := e.user().Balance; b != before {
		t.Fatalf("hạt thứ năm trừ xu: %d", before-b)
	}
	if f := e.ok(e.f.State(ctx, e.uid)).Cloud.Floors; f != 4 {
		t.Fatalf("số tầng cuối: %d", f)
	}
}

func TestBuyPotRules(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	start := e.user().Balance
	for _, floor := range []int{0, 5, -1} {
		_, err := e.f.BuyPot(ctx, e.uid, floor)
		wantStatus(t, err, http.StatusBadRequest)
	}
	_, err := e.f.BuyPot(ctx, e.uid, 2) // chưa mở
	wantStatus(t, err, http.StatusConflict)
	if b := e.user().Balance; b != start {
		t.Fatalf("bị trừ xu khi bị từ chối: %d", start-b)
	}
	for i := range 6 {
		s := e.ok(e.f.BuyPot(ctx, e.uid, 1))
		if len(s.Cloud.Pots) != i+1 || s.Cloud.Pots[i].Slot != i {
			t.Fatalf("chậu %d: %+v", i, s.Cloud.Pots)
		}
	}
	if got := start - e.user().Balance; got != 600_000 {
		t.Fatalf("6 chậu tầng 1: %d", got)
	}
	_, err = e.f.BuyPot(ctx, e.uid, 1) // chậu thứ 7
	wantStatus(t, err, http.StatusConflict)
	if got := start - e.user().Balance; got != 600_000 {
		t.Fatalf("chậu thứ 7 trừ xu: %d", got)
	}
}

func TestBuyPotNotEnoughCoins(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	e.set(99_999, 0)
	_, err := e.f.BuyPot(ctx, e.uid, 1)
	wantStatus(t, err, http.StatusPaymentRequired)
	if n := len(e.ok(e.f.State(ctx, e.uid)).Cloud.Pots); n != 0 {
		t.Fatalf("không đủ xu vẫn có chậu: %d", n)
	}
}

// Bấm nhiều lần cùng lúc: chỉ trừ tiền đúng số chậu tạo được, không vượt 6.
func TestBuyPotConcurrent(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	start := e.user().Balance
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.f.BuyPot(ctx, e.uid, 1); err != nil && errStatus(err) != http.StatusConflict {
				t.Errorf("lỗi lạ: %v", err)
			}
		}()
	}
	wg.Wait()
	s := e.ok(e.f.State(ctx, e.uid))
	if len(s.Cloud.Pots) != 6 {
		t.Fatalf("số chậu: %d", len(s.Cloud.Pots))
	}
	if got := start - e.user().Balance; got != 600_000 {
		t.Fatalf("tiền trừ: %d", got)
	}
}

func TestPlantAndHarvestPot(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	before := e.user().Balance
	xp := e.user().XP
	now := e.clock.Unix()

	s := e.ok(e.f.PlantPot(ctx, e.uid, 1, 0))
	if p := s.Cloud.Pots[0]; p.PlantedAt != now || p.ReadyAt != now+43200 {
		t.Fatalf("sau khi trồng: %+v", p)
	}
	if got := before - e.user().Balance; got != 5_000 {
		t.Fatalf("tiền hạt: %d", got)
	}
	if n := s.Storage.Seeds.Items["cloud_berry"]; n != 0 {
		t.Fatalf("không dùng kho giống: %d", n)
	}
	_, err := e.f.PlantPot(ctx, e.uid, 1, 0) // đang có cây
	wantStatus(t, err, http.StatusConflict)
	if got := before - e.user().Balance; got != 5_000 {
		t.Fatalf("trồng chồng trừ xu: %d", got)
	}

	e.clock = e.clock.Add(43200*time.Second - time.Second)
	_, _, err = e.f.HarvestPot(ctx, e.uid, 1, 0)
	wantStatus(t, err, http.StatusConflict)

	e.clock = e.clock.Add(time.Second)
	s, g, err := e.f.HarvestPot(ctx, e.uid, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if g.XP != 200 || g.Count != 1 || g.Items["cloud_berry"] != 1 {
		t.Fatalf("phần thưởng: %+v", g)
	}
	if s.Storage.Products.Items["cloud_berry"] != 1 {
		t.Fatalf("kho: %+v", s.Storage.Products.Items)
	}
	if got := e.user().XP - xp; got != 200 {
		t.Fatalf("XP: %d", got)
	}
	if p := s.Cloud.Pots[0]; p.PlantedAt != 0 || p.ReadyAt != 0 {
		t.Fatalf("chậu phải trống lại: %+v", p)
	}
	_, _, err = e.f.HarvestPot(ctx, e.uid, 1, 0) // chậu trống
	wantStatus(t, err, http.StatusConflict)

	// Chín để lâu không hỏng.
	e.ok(e.f.PlantPot(ctx, e.uid, 1, 0))
	e.clock = e.clock.Add(30 * 24 * time.Hour)
	if _, _, err := e.f.HarvestPot(ctx, e.uid, 1, 0); err != nil {
		t.Fatalf("chín lâu vẫn thu được: %v", err)
	}
}

func TestPotBadSlotAndMissingPot(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	for _, c := range [][2]int{{0, 0}, {5, 0}, {1, -1}, {1, 6}} {
		_, err := e.f.PlantPot(ctx, e.uid, c[0], c[1])
		wantStatus(t, err, http.StatusBadRequest)
		_, _, err = e.f.HarvestPot(ctx, e.uid, c[0], c[1])
		wantStatus(t, err, http.StatusBadRequest)
	}
	_, err := e.f.PlantPot(ctx, e.uid, 1, 1) // chưa có chậu ở chỗ 1
	wantStatus(t, err, http.StatusConflict)
	_, _, err = e.f.HarvestPot(ctx, e.uid, 1, 1)
	wantStatus(t, err, http.StatusConflict)
	_, err = e.f.PlantPot(ctx, e.uid, 2, 0) // tầng chưa mở
	wantStatus(t, err, http.StatusConflict)
}

func TestPlantPotNotEnoughCoinsLeavesPotEmpty(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.set(4_999, 0)
	_, err := e.f.PlantPot(ctx, e.uid, 1, 0)
	wantStatus(t, err, http.StatusPaymentRequired)
	if p := e.ok(e.f.State(ctx, e.uid)).Cloud.Pots[0]; p.PlantedAt != 0 {
		t.Fatalf("chậu phải trống: %+v", p)
	}
	if b := e.user().Balance; b != 4_999 {
		t.Fatalf("bị trừ xu: %d", b)
	}
}

func TestHarvestPotFullStoreKeepsCrop(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.ok(e.f.PlantPot(ctx, e.uid, 1, 0))
	e.clock = e.clock.Add(43200 * time.Second)
	cap := e.ok(e.f.State(ctx, e.uid)).Storage.Products.Capacity
	e.giveItem(e.uid, storeProducts, "egg", cap)
	_, _, err := e.f.HarvestPot(ctx, e.uid, 1, 0)
	wantStatus(t, err, http.StatusConflict)
	if p := e.ok(e.f.State(ctx, e.uid)).Cloud.Pots[0]; p.ReadyAt == 0 {
		t.Fatal("kho đầy thì cây phải còn nguyên")
	}
}

// Nông sản mây bán đúng giá; sau khi bán hết, lần có hàng sau vẫn đúng tên và giá.
func TestSellCloudProduct(t *testing.T) {
	e := newEnv(t, 0, 0)
	if name, price, ok := product("fairy_peach"); !ok || price != 600_000 || name != "Đào tiên" {
		t.Fatalf("product: %q %d %v", name, price, ok)
	}
	for round := range 2 {
		e.giveItem(e.uid, storeProducts, "fairy_peach", 1)
		before := e.user().Balance
		_, g, err := e.f.SellProduct(ctx, e.uid, "fairy_peach", 1)
		if err != nil {
			t.Fatalf("lần %d: %v", round, err)
		}
		if g.Coins != 540_000 || g.Tax != 60_000 || e.user().Balance-before != 540_000 { // thuế nông sản 10%
			t.Fatalf("lần %d: %+v", round, g)
		}
	}
}

// Cây mây không lọt xuống đất: không có trong danh sách cây, không gieo được, không mua hạt được.
func TestCloudCropsStayOffGround(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	for _, f := range CloudFloors {
		if _, ok := findCrop(f.Crop.ID); ok {
			t.Fatalf("%s nằm trong Crops", f.Crop.ID)
		}
		if c, ok := findCloudCrop(f.Crop.ID); !ok || c.ID != f.Crop.ID {
			t.Fatalf("findCloudCrop(%s)", f.Crop.ID)
		}
		_, err := e.f.Plant(ctx, e.uid, 0, f.Crop.ID)
		wantStatus(t, err, http.StatusBadRequest)
		_, err = e.f.BuySeeds(ctx, e.uid, f.Crop.ID, 1)
		wantStatus(t, err, http.StatusBadRequest)
	}
	if _, ok := Catalog()["cloudFloors"].([]CloudFloor); !ok {
		t.Fatal("catalog thiếu cloudFloors")
	}
}

// Zombie, thu hoạch tất cả và gieo tất cả chỉ làm việc với ô đất, không đụng tới đậu và chậu.
func TestZombiesAndBulkLeaveCloudAlone(t *testing.T) {
	e := newZombieEnv(t)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.ok(e.f.PlantPot(ctx, e.uid, 1, 0))
	e.ok(e.f.PlantBean(ctx, e.uid))
	before := e.ok(e.f.State(ctx, e.uid)).Cloud

	if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil {
		t.Fatal(err)
	}
	// Cà rốt chín rồi thu hoạch tất cả: chậu chưa chín và đậu vẫn nguyên.
	e.clock = e.clock.Add(121 * time.Second)
	if _, _, err := e.f.HarvestAll(ctx, e.uid); err != nil {
		t.Fatal(err)
	}
	s := e.ok(e.f.State(ctx, e.uid))
	if s.Cloud.Pots[0] != before.Pots[0] || s.Cloud.BeanReadyAt != before.BeanReadyAt {
		t.Fatalf("hàng loạt đụng tới mây: %+v", s.Cloud)
	}

	// Chậu chín rồi mà chưa thu: thu hoạch tất cả dưới đất cũng không lấy.
	e.clock = e.clock.Add(43200 * time.Second)
	if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(121 * time.Second)
	if _, _, err := e.f.HarvestAll(ctx, e.uid); err != nil && errStatus(err) != http.StatusConflict {
		t.Fatal(err)
	}
	if s := e.ok(e.f.State(ctx, e.uid)); s.Cloud.Pots[0].ReadyAt == 0 || s.Storage.Products.Items["cloud_berry"] != 0 {
		t.Fatalf("thu hoạch tất cả lấy cây mây: %+v", s.Cloud.Pots[0])
	}

	// Không canh nhiều ngày: cây dưới đất bị ăn, đậu vẫn chín, cây mây còn nguyên.
	if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil && errStatus(err) != http.StatusConflict {
		t.Fatal(err)
	}
	crops := 0
	for _, p := range e.ok(e.f.State(ctx, e.uid)).Plots {
		if p.Crop != "" {
			crops++
		}
	}
	e.clock = e.clock.Add(5 * 24 * time.Hour)
	s = e.ok(e.f.State(ctx, e.uid))
	if s.Cloud.Floors != 2 || s.Cloud.Pots[0].ReadyAt == 0 {
		t.Fatalf("zombie đụng tới mây: %+v", s.Cloud)
	}
	if len(s.Plots) != MaxPlots {
		t.Fatalf("ô 49 không nằm trong Plots: %d", len(s.Plots))
	}
	left := 0
	for _, p := range s.Plots {
		if p.Crop != "" {
			left++
		}
	}
	if crops == 0 || left >= crops {
		t.Fatalf("zombie phải có hoạt động dưới đất: %d → %d cây", crops, left)
	}
}

// Khách ghé thăm không nhận được dữ liệu mây, ô 49 không trộm được.
func TestVisitorSeesNoCloud(t *testing.T) {
	e, thief := thiefEnv(t, 100_000)
	e.set(100_000_000, 0)
	e.openFloors(1)
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.ok(e.f.PlantPot(ctx, e.uid, 1, 0))
	e.clock = e.clock.Add(43200 * time.Second)
	v, err := e.f.Visit(ctx, e.uid, thief)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(v)
	for _, leak := range []string{"cloud", "bean", `"pots"`} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("lộ %q khi ghé thăm: %s", leak, raw)
		}
	}
	if _, err := e.f.Steal(ctx, thief, e.uid, MaxPlots); err == nil {
		t.Fatal("ô 49 không trộm được")
	}
}

func TestWorthCountsCloud(t *testing.T) {
	e := newEnv(t, 100_000_000, 0)
	e.fullFarm(e.uid)
	base := e.worth()

	e.ok(e.f.PlantBean(ctx, e.uid)) // hạt đang lớn tính giá hạt tầng 1
	if got := e.worth() - base; got != 200_000 {
		t.Fatalf("hạt đang lớn: %d", got)
	}
	e.clock = e.clock.Add(24 * time.Hour)
	e.ok(e.f.State(ctx, e.uid))
	if got := e.worth() - base; got != 200_000 {
		t.Fatalf("tầng đã mở: %d", got)
	}
	e.ok(e.f.BuyPot(ctx, e.uid, 1))
	e.ok(e.f.PlantPot(ctx, e.uid, 1, 0))
	if got := e.worth() - base; got != 200_000+100_000+5_000 {
		t.Fatalf("tầng + chậu + cây: %d", got)
	}
	e.ok(e.f.PlantBean(ctx, e.uid)) // hạt tầng 2 đang lớn
	if got := e.worth() - base; got != 200_000+100_000+5_000+1_000_000 {
		t.Fatalf("thêm hạt tầng 2: %d", got)
	}
	// Thu hoạch: cây thành nông sản trong kho, tính theo giá bán thay vì giá hạt.
	e.clock = e.clock.Add(43200 * time.Second)
	if _, _, err := e.f.HarvestPot(ctx, e.uid, 1, 0); err != nil {
		t.Fatal(err)
	}
	if got := e.worth() - base; got != 200_000+100_000+20_000+1_000_000 {
		t.Fatalf("sau thu hoạch: %d", got)
	}
}

func TestCloudAPI(t *testing.T) {
	e := newEnv(t, 0, 0)
	u, token, err := e.w.Register(ctx, "maymay", "matkhau123", wallet.Profile{Name: "Mây"})
	if err != nil {
		t.Fatal(err)
	}
	owner := e.uid
	e.uid = u.ID
	defer func() { e.uid = owner }()
	e.setBalance(u.ID, 100_000_000)
	e.fullFarm(u.ID)
	srv := httptest.NewServer(e.f.Handler())
	defer srv.Close()
	post := func(path, body string) (int, map[string]json.RawMessage) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]json.RawMessage
		json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}

	code, out := post("/api/farm/bean/plant", ``)
	if code != http.StatusOK || out["farm"] == nil || out["user"] == nil {
		t.Fatalf("trồng đậu: %d %v", code, out)
	}
	var farm struct {
		Cloud struct {
			Available   bool  `json:"available"`
			Floors      int   `json:"floors"`
			BeanReadyAt int64 `json:"beanReadyAt"`
			Pots        []struct {
				Floor, Slot int
				ReadyAt     int64 `json:"readyAt"`
			} `json:"pots"`
		} `json:"cloud"`
	}
	json.Unmarshal(out["farm"], &farm)
	if !farm.Cloud.Available || farm.Cloud.BeanReadyAt == 0 {
		t.Fatalf("JSON cloud: %s", out["farm"])
	}
	e.clock = e.clock.Add(24 * time.Hour)

	if code, _ := post("/api/farm/pot/buy", `{}`); code != http.StatusBadRequest {
		t.Fatalf("thiếu floor: %d", code)
	}
	if code, _ := post("/api/farm/pot/buy", `{"floor":1}`); code != http.StatusOK {
		t.Fatalf("mua chậu: %d", code)
	}
	if code, _ := post("/api/farm/pot/plant", `{"floor":1}`); code != http.StatusBadRequest {
		t.Fatalf("thiếu slot: %d", code)
	}
	if code, _ := post("/api/farm/pot/plant", `{"floor":1,"slot":0}`); code != http.StatusOK {
		t.Fatalf("trồng chậu: %d", code)
	}
	if code, _ := post("/api/farm/pot/harvest", `{"floor":1,"slot":0}`); code != http.StatusConflict {
		t.Fatalf("chưa chín: %d", code)
	}
	e.clock = e.clock.Add(43200 * time.Second)
	code, out = post("/api/farm/pot/harvest", `{"floor":1,"slot":0}`)
	if code != http.StatusOK || out["gained"] == nil {
		t.Fatalf("thu hoạch: %d %v", code, out)
	}

	// Cùng điều kiện máu như thao tác nông trại khác.
	e.setHP(u.ID, 30)
	if code, _ := post("/api/farm/pot/plant", `{"floor":1,"slot":0}`); code == http.StatusOK {
		t.Fatal("dưới 40 máu không làm nông trên mây được")
	}
}
