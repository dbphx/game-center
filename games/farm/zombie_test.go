package farm

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Đất tốt ----------

func TestGrowSeconds(t *testing.T) {
	c, _ := findCrop("carrot") // 120 giây
	if got := growSeconds(c, false); got != 120 {
		t.Fatalf("đất thường: %d", got)
	}
	if got := growSeconds(c, true); got != 84 {
		t.Fatalf("đất tốt nhanh hơn 30%%: %d", got)
	}
	r := Crop{GrowSeconds: 61}
	if got := growSeconds(r, true); got != 42 { // floor(42,7)
		t.Fatalf("làm tròn xuống: %d", got)
	}
}

func TestUpgradePlot(t *testing.T) {
	e := newEnv(t, 50_000, 0)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	before := e.ok(e.f.State(ctx, e.uid)).Plots[0].ReadyAt

	s := e.ok(e.f.UpgradePlot(ctx, e.uid, 0))
	if !s.Plots[0].Good || s.Plots[1].Good {
		t.Fatalf("chỉ ô 0 lên đất tốt: %+v", s.Plots[:2])
	}
	if s.Plots[0].ReadyAt != before {
		t.Fatalf("cây đang trồng giữ nguyên giờ chín: %d, muốn %d", s.Plots[0].ReadyAt, before)
	}
	if b := e.user().Balance; b != 50_000-GoodSoilPrice-50 {
		t.Fatalf("xu sau nâng cấp: %d", b)
	}
	if _, err := e.f.UpgradePlot(ctx, e.uid, 0); err == nil {
		t.Fatal("không nâng cấp hai lần")
	} else {
		wantStatus(t, err, http.StatusConflict)
	}
	_, err := e.f.UpgradePlot(ctx, e.uid, 99)
	wantStatus(t, err, http.StatusBadRequest)

	poor := newEnv(t, GoodSoilPrice-1, 0)
	_, err = poor.f.UpgradePlot(ctx, poor.uid, 0)
	wantStatus(t, err, http.StatusPaymentRequired)
}

func TestGoodSoilAppliesToPlantAndPlantAll(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	e.ok(e.f.UpgradePlot(ctx, e.uid, 0))
	e.ok(e.f.UpgradePlot(ctx, e.uid, 1))
	now := e.clock.Unix()
	s := e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	if s.Plots[0].ReadyAt != now+84 {
		t.Fatalf("Plant trên đất tốt: %d", s.Plots[0].ReadyAt-now)
	}
	s, _, err := e.f.PlantAll(ctx, e.uid, "carrot")
	if err != nil {
		t.Fatal(err)
	}
	if s.Plots[1].ReadyAt != now+84 || s.Plots[2].ReadyAt != now+120 {
		t.Fatalf("PlantAll: đất tốt %d, đất thường %d", s.Plots[1].ReadyAt-now, s.Plots[2].ReadyAt-now)
	}
}

func TestWorthCountsGoodSoil(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	before, err := e.f.Worth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	e.ok(e.f.UpgradePlot(ctx, e.uid, 0))
	after, err := e.f.Worth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after[e.uid]-before[e.uid] != GoodSoilPrice {
		t.Fatalf("Worth tăng %d, muốn %d", after[e.uid]-before[e.uid], GoodSoilPrice)
	}
}

func TestCatalogHasGoodSoil(t *testing.T) {
	c := Catalog()
	if c["goodSoilPrice"] != int64(8000) || c["goodSoilSpeedup"] != 0.3 {
		t.Fatalf("catalog: %v %v", c["goodSoilPrice"], c["goodSoilSpeedup"])
	}
}

// ---------- Số zombie ----------

func TestZombieCount(t *testing.T) {
	cases := []struct {
		crops, count int
		chance       float64
	}{{0, 0, 0}, {1, 1, 0.0561}, {6, 1, 0.337}, {20, 2, 0.561}, {39, 3, 0.730}, {48, 3, 0.898}}
	for _, c := range cases {
		n, q := zombieCount(c.crops)
		if n != c.count || math.Abs(q-c.chance) > 0.001 {
			t.Fatalf("%d cây: %d con, q = %.4f; muốn %d, %.4f", c.crops, n, q, c.count, c.chance)
		}
	}
	for crops := 1; crops <= MaxPlots; crops++ {
		n, q := zombieCount(crops)
		if n < 1 || n > crops || q <= 0 || q > 1 {
			t.Fatalf("%d cây: %d con, q = %f", crops, n, q)
		}
	}
}

// Giả lập nhiều nhà không canh 12 đợt: còn lại khoảng 50% ở mọi cỡ.
func TestUnprotectedLosesHalfIn12Hours(t *testing.T) {
	for _, crops := range []int{6, 20, 39, 48} {
		total, alive := 0, 0
		for uid := int64(1); uid <= 3000; uid++ {
			left := make([]int, crops)
			for i := range left {
				left[i] = i
			}
			for w := int64(500_000); w < 500_012; w++ {
				for _, z := range pickZombies(uid, w, left) {
					if z.bites {
						for i, idx := range left {
							if idx == z.idx {
								left = append(left[:i], left[i+1:]...)
								break
							}
						}
					}
				}
			}
			total += crops
			alive += len(left)
		}
		ratio := float64(alive) / float64(total)
		if ratio < 0.47 || ratio > 0.53 {
			t.Fatalf("nhà %d cây còn %.3f sau 12 đợt", crops, ratio)
		}
	}
}

func TestPickZombiesDeterministic(t *testing.T) {
	plots := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	a, b := pickZombies(7, 500_001, plots), pickZombies(7, 500_001, plots)
	if len(a) != 2 || a[0] != b[0] || a[1] != b[1] {
		t.Fatalf("cùng (uid, đợt) phải ra cùng kết quả: %+v %+v", a, b)
	}
	if a[0].idx == a[1].idx {
		t.Fatalf("mỗi ô tối đa 1 zombie: %+v", a)
	}
	if got := pickZombies(7, 500_001, nil); len(got) != 0 {
		t.Fatalf("không có cây thì không có zombie: %+v", got)
	}
}

// ---------- Đợt tấn công ----------

const hour = 3600

// newZombieEnv: nông trại có bật zombie, đã được server thấy lần đầu ở đợt hiện tại.
func newZombieEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, 100_000_000, 0)
	e.f.noZombies = false
	e.ok(e.f.State(ctx, e.uid))
	return e
}

func (e *env) wave() int64 { return e.clock.Unix() / hour }

// spawn đặt sẵn một zombie thuộc đợt hiện tại lên ô idx và coi đợt hiện tại đã tính.
func (e *env) spawn(idx int, bites bool) {
	e.t.Helper()
	e.exec(`INSERT INTO farm_zombies (user_id, idx, wave, bites) VALUES (?, ?, ?, ?)`, e.uid, idx, e.wave(), bites)
	e.exec(`UPDATE farm_zombie_progress SET last_wave = ? WHERE user_id = ?`, e.wave(), e.uid)
}

func (e *env) setHP(uid int64, hp int) {
	e.t.Helper()
	e.exec(`UPDATE users SET hp = ?, hp_at = ?, hospital_until = 0 WHERE id = ?`, hp, time.Now().Unix(), uid)
}

func (e *env) buyPlots(n int) {
	e.t.Helper()
	for range n {
		e.ok(e.f.BuyPlot(ctx, e.uid))
	}
}

func zombieCells(s State) []int {
	out := []int{}
	for _, p := range s.Plots {
		if p.Zombie {
			out = append(out, p.Index)
		}
	}
	return out
}

func TestNoRetroactiveWaveOnFirstSight(t *testing.T) {
	e := newEnv(t, 100_000, 0)
	// Cây đã trồng từ trước, nhiều giờ trôi qua, rồi mới có tính năng zombie.
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.clock = e.clock.Add(48 * time.Hour)
	e.f.noZombies = false
	s := e.ok(e.f.State(ctx, e.uid))
	if len(zombieCells(s)) != 0 || s.Plots[0].Crop == "" || s.ZombieEaten != 0 {
		t.Fatalf("lần đầu thấy nông trại không tính ngược: %+v", s.Plots[0])
	}
}

func TestWaveArrivesOnTheHourForEligibleCropsOnly(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot")) // gieo đúng đầu giờ hiện tại: đủ điều kiện cho đợt sau
	e.clock = e.clock.Add(time.Hour - time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if len(zombieCells(s)) != 0 || s.ZombieUntil != 0 {
		t.Fatalf("chưa tới đầu giờ: %v", zombieCells(s))
	}
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.clock = e.clock.Add(time.Second) // đúng đầu giờ
	s = e.ok(e.f.State(ctx, e.uid))
	got := zombieCells(s)
	// 2 cây đủ điều kiện → 1 con.
	if len(got) != 1 || got[0] > 1 {
		t.Fatalf("đợt đầu giờ: %v", got)
	}
	if s.ZombieUntil != e.clock.Unix()+ZombieWindow {
		t.Fatalf("hạn đập: %d", s.ZombieUntil-e.clock.Unix())
	}
	// Cây gieo đúng lúc đợt bắt đầu không phải mục tiêu của đợt đó.
	f := newZombieEnv(t)
	f.clock = f.clock.Add(time.Hour)
	f.ok(f.f.Plant(ctx, f.uid, 0, "carrot"))
	if s := f.ok(f.f.State(ctx, f.uid)); len(zombieCells(s)) != 0 {
		t.Fatalf("cây gieo lúc đợt bắt đầu bị nhắm: %v", zombieCells(s))
	}
}

func TestEmptyFarmHasNoZombies(t *testing.T) {
	e := newZombieEnv(t)
	e.clock = e.clock.Add(3 * time.Hour)
	if s := e.ok(e.f.State(ctx, e.uid)); len(zombieCells(s)) != 0 || s.ZombieEaten != 0 {
		t.Fatalf("nhà không có cây: %v", zombieCells(s))
	}
}

func TestExpiryBitesExactlyAtFiveMinutes(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.spawn(0, true)
	e.spawn(1, false)
	e.clock = e.clock.Add(ZombieWindow*time.Second - time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.Plots[0].Crop == "" || len(zombieCells(s)) != 2 {
		t.Fatalf("trước hạn chưa mất cây: %+v", s.Plots[:2])
	}
	e.clock = e.clock.Add(time.Second)
	s = e.ok(e.f.State(ctx, e.uid))
	if s.Plots[0].Crop != "" {
		t.Fatalf("bites = 1 phải mất cây: %+v", s.Plots[0])
	}
	if s.Plots[1].Crop == "" || s.Plots[1].Zombie {
		t.Fatalf("bites = 0 zombie bỏ đi, cây còn: %+v", s.Plots[1])
	}
	if s.ZombieEaten != 1 || s.ZombieUntil != 0 {
		t.Fatalf("báo mất cây: eaten %d, until %d", s.ZombieEaten, s.ZombieUntil)
	}
	// Báo một lần rồi về 0.
	if s = e.ok(e.f.State(ctx, e.uid)); s.ZombieEaten != 0 {
		t.Fatalf("eaten phải về 0: %d", s.ZombieEaten)
	}
}

func TestDoDoesNotClearEaten(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	e.clock = e.clock.Add(ZombieWindow * time.Second)
	s := e.ok(e.f.Plant(ctx, e.uid, 1, "carrot")) // đi qua do
	if s.ZombieEaten != 1 {
		t.Fatalf("do vẫn trả số đã ăn: %d", s.ZombieEaten)
	}
	if s = e.ok(e.f.State(ctx, e.uid)); s.ZombieEaten != 1 {
		t.Fatalf("do không được xoá: %d", s.ZombieEaten)
	}
}

func TestOfflineSettlesEveryWave(t *testing.T) {
	e := newZombieEnv(t)
	e.buyPlots(42)
	if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(12*time.Hour + ZombieWindow*time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	alive := 0
	for _, p := range s.Plots {
		if p.Crop != "" {
			alive++
		}
	}
	if alive == MaxPlots || alive == 0 || s.ZombieEaten != MaxPlots-alive {
		t.Fatalf("vắng 12 giờ: còn %d cây, báo ăn %d", alive, s.ZombieEaten)
	}
	if len(zombieCells(s)) != 0 {
		t.Fatalf("mọi đợt đã quá hạn: %v", zombieCells(s))
	}
}

func TestSettleIsDeterministicAfterRollback(t *testing.T) {
	// Thao tác lỗi làm rollback phần tính bù; lần sau tính lại phải ra đúng như vậy.
	mk := func() *env {
		e := newZombieEnv(t)
		if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil {
			t.Fatal(err)
		}
		e.clock = e.clock.Add(5 * time.Hour)
		return e
	}
	a, b := mk(), mk()
	if _, err := a.f.Plant(ctx, a.uid, 99, "carrot"); err == nil {
		t.Fatal("ô sai phải lỗi")
	}
	sa, sb := a.ok(a.f.State(ctx, a.uid)), b.ok(b.f.State(ctx, b.uid))
	for i := range sa.Plots {
		if (sa.Plots[i].Crop != "") != (sb.Plots[i].Crop != "") || sa.Plots[i].Zombie != sb.Plots[i].Zombie {
			t.Fatalf("ô %d khác nhau sau rollback: %+v / %+v", i, sa.Plots[i], sb.Plots[i])
		}
	}
}

func TestVeryLongAbsenceIsCappedAndFast(t *testing.T) {
	e := newZombieEnv(t)
	if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(5 * 365 * 24 * time.Hour)
	start := time.Now()
	s := e.ok(e.f.State(ctx, e.uid))
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("vắng lâu chạy quá chậm: %v", d)
	}
	if s.Now != e.clock.Unix() {
		t.Fatalf("now: %d", s.Now)
	}
	var last int64
	tx, _ := e.w.Begin(ctx)
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `SELECT last_wave FROM farm_zombie_progress WHERE user_id = ?`, e.uid).Scan(&last); err != nil {
		t.Fatal(err)
	}
	if last != e.wave() {
		t.Fatalf("last_wave %d, muốn %d", last, e.wave())
	}
}

// ---------- Đập ----------

func TestHitZombieCosts2HPAndSavesCrop(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	bal, xp := e.user().Balance, e.user().XP
	s := e.ok(e.f.HitZombie(ctx, e.uid, 0))
	if s.Plots[0].Zombie || s.Plots[0].Crop == "" {
		t.Fatalf("đập xong còn cây, hết zombie: %+v", s.Plots[0])
	}
	u := e.user()
	if u.HP != wallet.MaxHP-ZombieHitHP || u.Balance != bal || u.XP != xp {
		t.Fatalf("HP %d, xu %d, XP %d", u.HP, u.Balance, u.XP)
	}
	// Hết hạn đợt cũ: cây không bị ăn.
	e.clock = e.clock.Add(ZombieWindow * time.Second)
	if s = e.ok(e.f.State(ctx, e.uid)); s.Plots[0].Crop == "" || s.ZombieEaten != 0 {
		t.Fatalf("cây đã được cứu: %+v", s.Plots[0])
	}
}

// Bấm đập hai lần cùng một ô: lần hai báo không có zombie và không trừ thêm máu.
func TestHitTwiceDoesNotCostMore(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	e.ok(e.f.HitZombie(ctx, e.uid, 0))
	_, err := e.f.HitZombie(ctx, e.uid, 0)
	wantStatus(t, err, http.StatusConflict)
	if hp := e.user().HP; hp != wallet.MaxHP-ZombieHitHP {
		t.Fatalf("HP sau lần đập thứ hai: %d", hp)
	}
	_, err = e.f.HitZombie(ctx, e.uid, 99)
	wantStatus(t, err, http.StatusBadRequest)
}

// Đập đúng giây hết hạn: zombie đã hết hạn, không trừ máu.
func TestHitAtExactExpiryCostsNothing(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	e.clock = e.clock.Add(ZombieWindow * time.Second)
	_, err := e.f.HitZombie(ctx, e.uid, 0)
	wantStatus(t, err, http.StatusConflict)
	if hp := e.user().HP; hp != wallet.MaxHP {
		t.Fatalf("HP: %d", hp)
	}
	if s := e.ok(e.f.State(ctx, e.uid)); s.Plots[0].Crop != "" || s.ZombieEaten != 1 {
		t.Fatalf("hết hạn thì cây bị ăn: %+v, eaten %d", s.Plots[0], s.ZombieEaten)
	}
}

// Máu ≤ 2 mà đập: nhát đó vẫn tính, zombie chết, vào viện; nhát sau bị từ chối.
func TestHitAtLowHPGoesToHospitalThenRefused(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.spawn(0, true)
	e.spawn(1, true)
	e.setHP(e.uid, 2)
	s := e.ok(e.f.HitZombie(ctx, e.uid, 0))
	if s.Plots[0].Zombie {
		t.Fatal("nhát cuối vẫn giết zombie")
	}
	u := e.user()
	if u.HP != 0 || u.HospitalUntil == 0 {
		t.Fatalf("hết máu vào viện: HP %d, viện %d", u.HP, u.HospitalUntil)
	}
	_, err := e.f.HitZombie(ctx, e.uid, 1)
	wantStatus(t, err, http.StatusConflict)
	if s = e.ok(e.f.State(ctx, e.uid)); !s.Plots[1].Zombie {
		t.Fatal("đang nằm viện thì zombie còn nguyên")
	}
}

func TestHitWorksBelowFarmHPThroughAPI(t *testing.T) {
	e := newZombieEnv(t)
	u, token, err := e.w.Register(ctx, "keu", "matkhau123", wallet.Profile{Name: "Kêu"})
	if err != nil {
		t.Fatal(err)
	}
	owner := e.uid
	e.uid = u.ID
	defer func() { e.uid = owner }()
	e.setBalance(u.ID, 100_000)
	e.ok(e.f.State(ctx, u.ID))
	e.ok(e.f.Plant(ctx, u.ID, 0, "carrot"))
	e.ok(e.f.Plant(ctx, u.ID, 1, "carrot"))
	e.spawn(0, true)
	e.setHP(u.ID, 30)
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
	if code, _ := post("/api/farm/plot/upgrade", `{"plot":0}`); code == http.StatusOK {
		t.Fatal("thao tác nông trại khác vẫn cần từ 40 máu")
	}
	code, out := post("/api/farm/zombie/hit", `{"plot":0}`)
	if code != http.StatusOK || out["farm"] == nil || out["user"] == nil {
		t.Fatalf("đập dưới 40 máu: %d %v", code, out)
	}
	if hp := e.user().HP; hp != 28 {
		t.Fatalf("HP: %d", hp)
	}
	e.setHP(u.ID, wallet.MaxHP)
	if code, out = post("/api/farm/plot/upgrade", `{"plot":1}`); code != http.StatusOK || out["farm"] == nil {
		t.Fatalf("nâng cấp: %d", code)
	}

	// Trạng thái cho bản đồ thành phố.
	e.spawn(1, true)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/farm/zombie/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var st struct {
		Count int   `json:"count"`
		Until int64 `json:"until"`
	}
	json.NewDecoder(res.Body).Decode(&st)
	if st.Count != 1 || st.Until != e.wave()*hour+ZombieWindow {
		t.Fatalf("status: %+v", st)
	}
	res2, err := http.Get(srv.URL + "/api/farm/zombie/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("chưa đăng nhập: %d", res2.StatusCode)
	}
}

func TestZombieStatus(t *testing.T) {
	e := newZombieEnv(t)
	if n, until, err := e.f.ZombieStatus(ctx, e.uid); err != nil || n != 0 || until != 0 {
		t.Fatalf("không có zombie: %d %d %v", n, until, err)
	}
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.clock = e.clock.Add(time.Hour) // tới đợt mới mà chỉ gọi status (đang ở bản đồ)
	n, until, err := e.f.ZombieStatus(ctx, e.uid)
	if err != nil || n != 1 || until != e.clock.Unix()+ZombieWindow {
		t.Fatalf("status: %d %d %v", n, until, err)
	}
}

// ---------- Chặn né ----------

func TestCannotHarvestPlotWithZombie(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.clock = e.clock.Add(time.Minute * 3)
	e.spawn(0, true)
	_, _, err := e.f.Harvest(ctx, e.uid, 0)
	wantStatus(t, err, http.StatusConflict)
	if !strings.Contains(err.Error(), "zombie") {
		t.Fatalf("thông báo: %v", err)
	}
	e.ok(e.f.HitZombie(ctx, e.uid, 0))
	if _, _, err := e.f.Harvest(ctx, e.uid, 0); err != nil {
		t.Fatal(err)
	}
}

func TestHarvestAllSkipsZombiePlots(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.clock = e.clock.Add(time.Minute * 3)
	e.spawn(0, true)
	s, g, err := e.f.HarvestAll(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if g.Count != 1 || s.Plots[0].Crop == "" || s.Plots[1].Crop != "" {
		t.Fatalf("chỉ thu ô không có zombie: %+v %+v", g, s.Plots[:2])
	}
	// Chỉ còn cây chín có zombie: báo rõ phải đập, không báo "chưa có ô nào chín".
	_, _, err = e.f.HarvestAll(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)
	if !strings.Contains(err.Error(), "zombie") {
		t.Fatalf("thông báo: %v", err)
	}
}

// Bạn bè xem khi chủ offline qua đầu giờ: thấy đúng cây còn lại, không trộm được ô có zombie.
func TestVisitSettlesOwnerAndStealBlocked(t *testing.T) {
	e, thief := thiefEnv(t, 100_000)
	e.f.noZombies = false
	e.ok(e.f.State(ctx, e.uid))
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.clock = e.clock.Add(time.Hour) // đợt mới, chủ không online; 1 cây → 1 zombie
	v, err := e.f.Visit(ctx, e.uid, thief)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Plots[0].Zombie || v.Plots[0].Crop == "" {
		t.Fatalf("khách phải thấy zombie: %+v", v.Plots[0])
	}
	_, err = e.f.Steal(ctx, thief, e.uid, 0)
	wantStatus(t, err, http.StatusConflict)
	if !strings.Contains(err.Error(), "zombie") {
		t.Fatalf("thông báo: %v", err)
	}
	sum, err := e.f.Summaries(ctx, thief, []int64{e.uid})
	if err != nil {
		t.Fatal(err)
	}
	if sum[e.uid].Ripe != 0 {
		t.Fatalf("ô có zombie không tính là trộm được: %d", sum[e.uid].Ripe)
	}
	// Quá hạn mà bị ăn: khách thấy ô trống ngay cả khi chủ vẫn offline.
	e.exec(`UPDATE farm_zombies SET bites = 1 WHERE user_id = ?`, e.uid)
	e.clock = e.clock.Add(ZombieWindow * time.Second)
	if v, err = e.f.Visit(ctx, e.uid, thief); err != nil || v.Plots[0].Crop != "" || v.Plots[0].Zombie {
		t.Fatalf("khách thấy đúng cây còn lại: %+v %v", v.Plots[0], err)
	}
}

func TestMigrateAddsGoodColumnOnce(t *testing.T) {
	e := newEnv(t, 0, 0)
	for range 2 { // New chạy migrate; chạy thêm lần nữa không lỗi
		if err := migrate(ctx, e.w, e.clock.Unix()); err != nil {
			t.Fatal(err)
		}
	}
}
