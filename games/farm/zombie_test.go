package farm

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
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
// pickZombies: mọi cây cùng tiến độ thì zombie chọn đều.
func pickZombies(uid, wave int64, eligible []int) []zombiePick {
	targets := make([]zombieTarget, len(eligible))
	for i, idx := range eligible {
		targets[i] = zombieTarget{idx: idx, progress: 1}
	}
	return pickTargets(uid, uint64(wave), targets)
}

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

// ---------- Súng đậu ----------

// armed: nông trại có zombie bật, đã mua súng và n viên đạn.
func armed(t *testing.T, n int) *env {
	t.Helper()
	e := newZombieEnv(t)
	e.ok(e.f.BuyGun(ctx, e.uid))
	if n > 0 {
		e.ok(e.f.BuyAmmo(ctx, e.uid, n))
	}
	return e
}

func TestBuyGunAndAmmoPrices(t *testing.T) {
	e := newZombieEnv(t)
	if s := e.ok(e.f.State(ctx, e.uid)); s.Gun.Owned || s.Gun.Ammo != 0 {
		t.Fatalf("chưa mua súng: %+v", s.Gun)
	}
	// Chưa có súng thì không mua được đạn.
	_, err := e.f.BuyAmmo(ctx, e.uid, 1)
	wantStatus(t, err, http.StatusConflict)

	before := e.user().Balance
	if s := e.ok(e.f.BuyGun(ctx, e.uid)); !s.Gun.Owned || s.Gun.Ammo != 0 {
		t.Fatalf("đã mua súng: %+v", s.Gun)
	}
	if got := before - e.user().Balance; got != 399_000 {
		t.Fatalf("giá súng: %d", got)
	}
	_, err = e.f.BuyGun(ctx, e.uid)
	wantStatus(t, err, http.StatusConflict)

	before = e.user().Balance
	if s := e.ok(e.f.BuyAmmo(ctx, e.uid, 3)); s.Gun.Ammo != 3 {
		t.Fatalf("đạn: %d", s.Gun.Ammo)
	}
	if got := before - e.user().Balance; got != 1_500 {
		t.Fatalf("giá 3 viên: %d", got)
	}
	for _, qty := range []int{0, -1, 1001} {
		_, err = e.f.BuyAmmo(ctx, e.uid, qty)
		wantStatus(t, err, http.StatusBadRequest)
	}
	if s := e.ok(e.f.BuyAmmo(ctx, e.uid, 1000)); s.Gun.Ammo != 1003 {
		t.Fatalf("mua tối đa 1000 viên một lần: %d", s.Gun.Ammo)
	}
}

func TestGunAndAmmoNotEnoughCoinsKeepsCoins(t *testing.T) {
	e := newEnv(t, GunPrice-1, 0)
	_, err := e.f.BuyGun(ctx, e.uid)
	wantStatus(t, err, http.StatusPaymentRequired)
	if b := e.user().Balance; b != GunPrice-1 {
		t.Fatalf("thiếu xu không mất xu: %d", b)
	}
	if s := e.ok(e.f.State(ctx, e.uid)); s.Gun.Owned {
		t.Fatal("thiếu xu không có súng")
	}
	e.set(GunPrice+AmmoPrice, 0)
	e.ok(e.f.BuyGun(ctx, e.uid))
	_, err = e.f.BuyAmmo(ctx, e.uid, 2)
	wantStatus(t, err, http.StatusPaymentRequired)
	if b := e.user().Balance; b != AmmoPrice {
		t.Fatalf("thiếu xu mua đạn không mất xu: %d", b)
	}
	if s := e.ok(e.f.State(ctx, e.uid)); s.Gun.Ammo != 0 {
		t.Fatalf("thiếu xu không có đạn: %d", s.Gun.Ammo)
	}
}

func TestGunShootsFromSecond30AndKeepsCrop(t *testing.T) {
	e := armed(t, 5)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.ok(e.f.Plant(ctx, e.uid, 1, "carrot"))
	e.spawn(0, true)
	e.spawn(1, true)
	e.clock = e.clock.Add((GunDelay - 1) * time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if len(zombieCells(s)) != 2 || s.Gun.Ammo != 5 || s.GunKilled != 0 {
		t.Fatalf("trước giây 30 chưa bắn: %v %+v %d", zombieCells(s), s.Gun, s.GunKilled)
	}
	e.clock = e.clock.Add(time.Second)
	s = e.ok(e.f.State(ctx, e.uid))
	if len(zombieCells(s)) != 0 || s.Gun.Ammo != 3 || s.GunKilled != 2 {
		t.Fatalf("từ giây 30 bắn hết: %v %+v %d", zombieCells(s), s.Gun, s.GunKilled)
	}
	if s.Plots[0].Crop == "" || s.Plots[1].Crop == "" || s.ZombieEaten != 0 {
		t.Fatalf("súng bắn thì cây còn: %+v", s.Plots[:2])
	}
	// Báo một lần rồi về 0.
	if s = e.ok(e.f.State(ctx, e.uid)); s.GunKilled != 0 {
		t.Fatalf("gunKilled phải về 0: %d", s.GunKilled)
	}
}

func TestDoDoesNotClearGunKilled(t *testing.T) {
	e := armed(t, 1)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	e.clock = e.clock.Add(GunDelay * time.Second)
	if s := e.ok(e.f.Plant(ctx, e.uid, 1, "carrot")); s.GunKilled != 1 {
		t.Fatalf("thao tác khác vẫn trả số đã diệt: %d", s.GunKilled)
	}
	if s := e.ok(e.f.State(ctx, e.uid)); s.GunKilled != 1 {
		t.Fatalf("mở nông trại mới báo: %d", s.GunKilled)
	}
}

func TestOutOfAmmoLeavesZombieToHitOrBite(t *testing.T) {
	e := armed(t, 1)
	for i := range 3 {
		e.ok(e.f.Plant(ctx, e.uid, i, "carrot"))
		e.spawn(i, true)
	}
	e.clock = e.clock.Add(GunDelay * time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	// Đạn bắn theo thứ tự ô.
	if got := zombieCells(s); len(got) != 2 || got[0] != 1 || got[1] != 2 || s.Gun.Ammo != 0 {
		t.Fatalf("hết đạn còn 2 con ô 1, 2: %v %+v", got, s.Gun)
	}
	// Con còn lại vẫn đập được.
	hp := e.user().HP
	s = e.ok(e.f.HitZombie(ctx, e.uid, 1))
	if e.user().HP != hp-ZombieHitHP || s.Plots[1].Zombie {
		t.Fatalf("hết đạn vẫn đập tay: %+v", s.Plots[1])
	}
	// Con không ai đập thì ăn cây khi hết hạn.
	e.clock = e.clock.Add(ZombieWindow * time.Second)
	s = e.ok(e.f.State(ctx, e.uid))
	if s.Plots[2].Crop != "" || s.ZombieEaten != 1 {
		t.Fatalf("hết đạn, hết hạn thì mất cây: %+v eaten %d", s.Plots[2], s.ZombieEaten)
	}
	if s.Plots[0].Crop == "" || s.Plots[1].Crop == "" {
		t.Fatalf("hai ô được cứu còn cây: %+v", s.Plots[:2])
	}
}

func TestHitBeforeSecond30SavesAmmo(t *testing.T) {
	e := armed(t, 2)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	e.clock = e.clock.Add(10 * time.Second)
	e.ok(e.f.HitZombie(ctx, e.uid, 0))
	e.clock = e.clock.Add(time.Minute)
	if s := e.ok(e.f.State(ctx, e.uid)); s.Gun.Ammo != 2 || s.GunKilled != 0 {
		t.Fatalf("đập tay trước giây 30 không tốn đạn: %+v %d", s.Gun, s.GunKilled)
	}
}

func TestBuyAmmoMidWaveShootsOnNextSettle(t *testing.T) {
	e := armed(t, 0)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	e.clock = e.clock.Add(time.Minute)
	if s := e.ok(e.f.State(ctx, e.uid)); len(zombieCells(s)) != 1 {
		t.Fatalf("chưa có đạn thì zombie còn: %v", zombieCells(s))
	}
	e.ok(e.f.BuyAmmo(ctx, e.uid, 1))
	s := e.ok(e.f.State(ctx, e.uid))
	if len(zombieCells(s)) != 0 || s.Gun.Ammo != 0 || s.GunKilled != 1 || s.Plots[0].Crop == "" {
		t.Fatalf("mua đạn giữa đợt, lần tính sau bắn ngay: %v %+v %d", zombieCells(s), s.Gun, s.GunKilled)
	}
}

func TestNoGunDoesNotShoot(t *testing.T) {
	e := newZombieEnv(t)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, true)
	e.clock = e.clock.Add(GunDelay * time.Second)
	if s := e.ok(e.f.State(ctx, e.uid)); len(zombieCells(s)) != 1 || s.GunKilled != 0 {
		t.Fatalf("không có súng thì không bắn: %v %d", zombieCells(s), s.GunKilled)
	}
}

func TestOfflineAmmoSavesEveryCrop(t *testing.T) {
	e := armed(t, 500)
	e.buyPlots(42)
	if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil {
		t.Fatal(err)
	}
	e.clock = e.clock.Add(24*time.Hour + ZombieWindow*time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.ZombieEaten != 0 || len(zombieCells(s)) != 0 {
		t.Fatalf("đủ đạn thì không mất cây: eaten %d, zombie %v", s.ZombieEaten, zombieCells(s))
	}
	for _, p := range s.Plots {
		if p.Crop == "" {
			t.Fatalf("đủ đạn mà mất cây ô %d", p.Index)
		}
	}
	// 48 cây: mỗi đợt 3 con, 24 đợt.
	if s.GunKilled != 72 || s.Gun.Ammo != 500-72 {
		t.Fatalf("tốn đạn đúng số zombie: diệt %d, còn %d", s.GunKilled, s.Gun.Ammo)
	}
}

func TestOfflineRunsOutOfAmmoThenCropsGetEaten(t *testing.T) {
	e := armed(t, 4)
	e.buyPlots(42)
	if _, _, err := e.f.PlantAll(ctx, e.uid, "carrot"); err != nil {
		t.Fatal(err)
	}
	// Hai đợt đầu ăn hết 4 viên (3 con rồi 1 con); các đợt sau không còn đạn.
	e.clock = e.clock.Add(24*time.Hour + ZombieWindow*time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.GunKilled != 4 || s.Gun.Ammo != 0 {
		t.Fatalf("đạn hết giữa chừng: diệt %d, còn %d", s.GunKilled, s.Gun.Ammo)
	}
	if s.ZombieEaten == 0 {
		t.Fatal("hết đạn thì các đợt sau phải ăn cây")
	}
}

func TestWorthCountsGunAndAmmo(t *testing.T) {
	e := newZombieEnv(t)
	before, err := e.f.Worth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	e.ok(e.f.BuyGun(ctx, e.uid))
	e.ok(e.f.BuyAmmo(ctx, e.uid, 4))
	after, err := e.f.Worth(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := after[e.uid] - before[e.uid]; got != GunPrice+4*AmmoPrice {
		t.Fatalf("Worth tăng %d", got)
	}
}

func TestCatalogHasGun(t *testing.T) {
	c := Catalog()
	if c["gunPrice"] != int64(399_000) || c["ammoPrice"] != int64(500) {
		t.Fatalf("catalog: %v %v", c["gunPrice"], c["ammoPrice"])
	}
}

func TestGunAPI(t *testing.T) {
	e := newZombieEnv(t)
	u, token, err := e.w.Register(ctx, "bansung", "matkhau123", wallet.Profile{Name: "Bắn súng"})
	if err != nil {
		t.Fatal(err)
	}
	e.setBalance(u.ID, 1_000_000)
	srv := httptest.NewServer(e.f.Handler())
	defer srv.Close()
	post := func(path, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b := new(strings.Builder)
		if _, err := io.Copy(b, res.Body); err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, b.String()
	}
	if code, _ := post("/api/farm/gun/ammo", `{"qty":1}`); code != http.StatusConflict {
		t.Fatalf("chưa có súng: %d", code)
	}
	if code, body := post("/api/farm/gun/buy", `{}`); code != http.StatusOK || !strings.Contains(body, `"gun":{"owned":true,"ammo":0}`) || !strings.Contains(body, `"user"`) {
		t.Fatalf("mua súng: %d %s", code, body)
	}
	if code, _ := post("/api/farm/gun/buy", `{}`); code != http.StatusConflict {
		t.Fatalf("mua lần hai: %d", code)
	}
	if code, _ := post("/api/farm/gun/ammo", `{"qty":1001}`); code != http.StatusBadRequest {
		t.Fatalf("qty 1001: %d", code)
	}
	if code, _ := post("/api/farm/gun/ammo", `{}`); code != http.StatusBadRequest {
		t.Fatalf("thiếu qty: %d", code)
	}
	if code, body := post("/api/farm/gun/ammo", `{"qty":2}`); code != http.StatusOK || !strings.Contains(body, `"gun":{"owned":true,"ammo":2}`) || !strings.Contains(body, `"gunKilled":0`) {
		t.Fatalf("mua đạn: %d %s", code, body)
	}
}

// ---------- Zombie ăn vật nuôi và cá ----------

// count chạy một truy vấn đếm trên database thử.
func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// nextWave tua tới đúng đầu giờ kế tiếp.
func (e *env) nextWave() { e.clock = time.Unix((e.wave()+1)*hour, 0) }

// addStock thêm thẳng một con đang đói (nên không lớn thêm) với tiến độ cho trước và trả id của nó.
func (e *env) addStock(place, kind string, grow int64) int64 {
	e.t.Helper()
	e.exec(`INSERT OR IGNORE INTO farm_buildings (user_id, place, capacity) VALUES (?, ?, 100)`, e.uid, place)
	now := e.clock.Unix()
	e.exec(`INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at, fed_until, grow_progress, progress_at)
		VALUES (?, ?, ?, ?, 0, 0, ?, ?)`, e.uid, place, kind, now-1, grow, now)
	return int64(e.count(`SELECT MAX(id) FROM farm_stock WHERE user_id = ?`, e.uid))
}

// spawnStock đặt sẵn zombie đợt hiện tại lên con id và coi đợt hiện tại đã tính ở cả ruộng lẫn vật nuôi.
func (e *env) spawnStock(id int64, bites bool) {
	e.t.Helper()
	e.exec(`INSERT INTO farm_stock_zombies (stock_id, user_id, wave, bites) VALUES (?, ?, ?, ?)`, id, e.uid, e.wave(), bites)
	e.exec(`UPDATE farm_zombie_progress SET last_wave = ? WHERE user_id = ?`, e.wave(), e.uid)
	e.exec(`UPDATE farm_stock_zombie_progress SET last_wave = ? WHERE user_id = ?`, e.wave(), e.uid)
}

// stockZombies liệt kê id các con đang có zombie, theo State.
func stockZombies(s State) []int64 {
	out := []int64{}
	for _, pen := range []*Pen{s.Barn, s.Yard, s.Pond} {
		if pen == nil {
			continue
		}
		for _, a := range append(append([]Animal{}, pen.Animals...), pen.Fish...) {
			if a.Zombie {
				out = append(out, a.ID)
			}
		}
	}
	return out
}

func wantMsg(t *testing.T, err error, sub string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), sub) {
		t.Fatalf("lỗi %v, muốn chứa %q", err, sub)
	}
}

// Mỗi khu có con thì có ít nhất 1 zombie, tính riêng từng khu.
func TestStockWaveHitsEveryZone(t *testing.T) {
	e := newZombieEnv(t)
	barn := e.addStock(placeBarn, "pig", 0)
	yard := e.addStock(placeYard, "chicken", 0)
	pond := e.addStock(placePond, "tilapia", 0)
	e.nextWave()
	s := e.ok(e.f.State(ctx, e.uid))
	got := stockZombies(s)
	if len(got) != 3 || got[0] != barn || got[1] != yard || got[2] != pond {
		t.Fatalf("zombie: %v, muốn %d %d %d", got, barn, yard, pond)
	}
	if s.ZombieUntil != e.clock.Unix()+ZombieWindow {
		t.Fatalf("hạn đập tính cả vật nuôi: %d", s.ZombieUntil-e.clock.Unix())
	}
	if n, until, err := e.f.ZombieStatus(ctx, e.uid); err != nil || n != 3 || until != s.ZombieUntil {
		t.Fatalf("status: %d %d %v", n, until, err)
	}
}

// Con mua đúng lúc đợt bắt đầu không phải mục tiêu của đợt đó; chó không bao giờ có zombie.
func TestStockEligibilityAndDog(t *testing.T) {
	e := newZombieEnv(t)
	e.exec(`INSERT INTO farm_dog (user_id, fed_until) VALUES (?, ?)`, e.uid, int64(4_000_000_000))
	e.nextWave()
	if s := e.ok(e.f.State(ctx, e.uid)); len(stockZombies(s)) != 0 || s.ZombieUntil != 0 {
		t.Fatalf("nhà chỉ có chó mà có zombie: %v", stockZombies(s))
	}
	id := e.addStock(placeYard, "chicken", 0)
	e.exec(`UPDATE farm_stock SET bought_at = ? WHERE id = ?`, e.clock.Unix(), id)
	e.nextWave() // đợt sau mới đủ điều kiện
	if s := e.ok(e.f.State(ctx, e.uid)); len(stockZombies(s)) != 1 {
		t.Fatalf("con mua trước đợt phải bị nhắm: %v", stockZombies(s))
	}
	f := newZombieEnv(t)
	f.nextWave()
	f.ok(f.f.State(ctx, f.uid))
	id = f.addStock(placeYard, "chicken", 0)
	f.exec(`UPDATE farm_stock SET bought_at = ? WHERE id = ?`, f.clock.Unix(), id)
	if s := f.ok(f.f.State(ctx, f.uid)); len(stockZombies(s)) != 0 {
		t.Fatalf("con mua lúc đợt bắt đầu bị nhắm: %v", stockZombies(s))
	}
}

// survivors giả lập 12 đợt không canh; progress(i, w) là tiến độ của mục tiêu i ở đợt w.
func survivors(n int, progress func(i int, w int64) float64) float64 {
	total, alive := 0, 0
	for uid := int64(1); uid <= 3000; uid++ {
		left := make([]int, n)
		for i := range left {
			left[i] = i
		}
		for w := int64(500_000); w < 500_012; w++ {
			targets := make([]zombieTarget, len(left))
			for k, i := range left {
				targets[k] = zombieTarget{idx: i, progress: progress(i, w)}
			}
			for _, z := range pickTargets(uid, uint64(w), targets) {
				if z.bites {
					left = slices.DeleteFunc(left, func(i int) bool { return i == z.idx })
				}
			}
		}
		total += n
		alive += len(left)
	}
	return float64(alive) / float64(total)
}

// Lựa con gần lớn chỉ dồn rủi ro, không đổi tỉ lệ mất chung: còn khoảng 50% sau 12 đợt, kể cả khi trồng lệch giờ.
func TestUnprotectedStillLosesHalfWhenStaggered(t *testing.T) {
	for _, n := range []int{6, 20, 40} {
		same := survivors(n, func(int, int64) float64 { return 1 })
		// Lệch đều 12 giờ, tiến độ tăng theo đợt, đã chín thì chặn ở 1.
		staggered := survivors(n, func(i int, w int64) float64 {
			return min(1, (float64(w-500_000)+float64(i)*12/float64(n))/12)
		})
		t.Logf("%d con: cùng tiến độ còn %.3f, lệch giờ còn %.3f", n, same, staggered)
		for _, r := range []float64{same, staggered} {
			if r < 0.44 || r > 0.56 {
				t.Fatalf("%d con: cùng tiến độ %.3f, lệch giờ %.3f", n, same, staggered)
			}
		}
	}
}

// Cùng seed và cùng mục tiêu thì ra cùng kết quả; bằng tiến độ thì mọi mục tiêu đều có thể bị chọn.
func TestPickTargetsDeterministicAndFairOnTies(t *testing.T) {
	list := make([]zombieTarget, 20)
	for i := range list {
		list[i] = zombieTarget{idx: i, progress: 1}
	}
	a, b := pickTargets(7, 99, list), pickTargets(7, 99, list)
	if !slices.Equal(a, b) {
		t.Fatalf("cùng seed khác kết quả: %v / %v", a, b)
	}
	seen := map[int]bool{}
	for w := uint64(0); w < 400; w++ {
		for _, z := range pickTargets(7, w, list) {
			seen[z.idx] = true
		}
	}
	if len(seen) != len(list) {
		t.Fatalf("bằng tiến độ mà chỉ chọn %d/%d mục tiêu", len(seen), len(list))
	}
}

// setCrop đặt thẳng cây trên ô idx với tiến độ tính từ mốc start.
func (e *env) setCrop(idx int, planted, ready int64) {
	e.t.Helper()
	e.exec(`UPDATE farm_plots SET crop = 'carrot', planted_at = ?, ready_at = ? WHERE user_id = ? AND idx = ?`, planted, ready, e.uid, idx)
}

func TestZombiesPreferNearlyRipeCrops(t *testing.T) {
	e := newZombieEnv(t)
	start := (e.wave() + 1) * hour
	for i := 0; i < 6; i++ {
		e.setCrop(i, start-1000, start+9000) // tiến độ 0,1
	}
	e.setCrop(3, start-9000, start+1000) // tiến độ 0,9
	e.nextWave()
	if got := zombieCells(e.ok(e.f.State(ctx, e.uid))); len(got) != 1 || got[0] != 3 {
		t.Fatalf("zombie phải nhắm cây gần chín (ô 3): %v", got)
	}
}

// Cây đã chín để lâu không được miễn: tiến độ chặn ở 1,0 là mức cao nhất.
func TestRipeCropsAreNotExempt(t *testing.T) {
	e := newZombieEnv(t)
	start := (e.wave() + 1) * hour
	for i := 0; i < 6; i++ {
		e.setCrop(i, start-5000, start+5000) // tiến độ 0,5
	}
	e.setCrop(2, start-90000, start-80000) // đã chín từ lâu
	e.nextWave()
	if got := zombieCells(e.ok(e.f.State(ctx, e.uid))); len(got) != 1 || got[0] != 2 {
		t.Fatalf("zombie phải nhắm cây đã chín (ô 2): %v", got)
	}
}

func TestZombiesPreferNearlyGrownStock(t *testing.T) {
	e := newZombieEnv(t)
	for range 3 {
		e.addStock(placeBarn, "pig", 60) // heo lớn sau 3600 giây
	}
	near := e.addStock(placeBarn, "pig", 3500)
	for range 2 {
		e.addStock(placeBarn, "pig", 60)
	}
	e.nextWave()
	if got := stockZombies(e.ok(e.f.State(ctx, e.uid))); len(got) != 1 || got[0] != near {
		t.Fatalf("zombie phải nhắm con gần lớn %d: %v", near, got)
	}
	// Con đã lớn cũng không được miễn.
	f := newZombieEnv(t)
	for range 5 {
		f.addStock(placeBarn, "pig", 60)
	}
	grown := f.addStock(placeBarn, "pig", 3600)
	f.nextWave()
	if got := stockZombies(f.ok(f.f.State(ctx, f.uid))); len(got) != 1 || got[0] != grown {
		t.Fatalf("zombie phải nhắm con đã lớn %d: %v", grown, got)
	}
}

func TestHitStockZombie(t *testing.T) {
	e := newZombieEnv(t)
	id := e.addStock(placeYard, "chicken", 0)
	e.spawnStock(id, true)
	s := e.ok(e.f.HitStockZombie(ctx, e.uid, id))
	if len(stockZombies(s)) != 0 || len(s.Yard.Animals) != 1 {
		t.Fatalf("đập xong con còn nguyên, hết zombie: %+v", s.Yard.Animals)
	}
	if hp := e.user().HP; hp != wallet.MaxHP-ZombieHitHP {
		t.Fatalf("HP: %d", hp)
	}
	// Đập lần hai: không có zombie, không trừ thêm máu.
	_, err := e.f.HitStockZombie(ctx, e.uid, id)
	wantStatus(t, err, http.StatusConflict)
	if hp := e.user().HP; hp != wallet.MaxHP-ZombieHitHP {
		t.Fatalf("HP sau lần hai: %d", hp)
	}
	_, err = e.f.HitStockZombie(ctx, e.uid, 999_999)
	wantStatus(t, err, http.StatusNotFound)
	// Con của người khác cũng là 404.
	other, _, err := e.w.Register(ctx, "hangxom", "matkhau123", wallet.Profile{Name: "Hàng xóm"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.f.HitStockZombie(ctx, other.ID, id)
	wantStatus(t, err, http.StatusNotFound)
}

// Đúng giây hết hạn thì zombie đã hết hạn, không trừ máu; con bites = 1 bị ăn.
func TestHitStockAtExactExpiry(t *testing.T) {
	e := newZombieEnv(t)
	id := e.addStock(placeYard, "chicken", 0)
	e.spawnStock(id, true)
	e.clock = time.Unix(e.wave()*hour+ZombieWindow, 0)
	_, err := e.f.HitStockZombie(ctx, e.uid, id)
	wantStatus(t, err, http.StatusNotFound)
	if hp := e.user().HP; hp != wallet.MaxHP {
		t.Fatalf("HP: %d", hp)
	}
}

func TestHitStockAtLowHPGoesToHospitalThenRefused(t *testing.T) {
	e := newZombieEnv(t)
	a, b := e.addStock(placeYard, "chicken", 0), e.addStock(placeYard, "chicken", 0)
	e.spawnStock(a, true)
	e.spawnStock(b, true)
	e.setHP(e.uid, 2)
	e.ok(e.f.HitStockZombie(ctx, e.uid, a))
	if u := e.user(); u.HP != 0 || u.HospitalUntil == 0 {
		t.Fatalf("hết máu vào viện: %d %d", u.HP, u.HospitalUntil)
	}
	_, err := e.f.HitStockZombie(ctx, e.uid, b)
	wantStatus(t, err, http.StatusConflict)
	if got := stockZombies(e.ok(e.f.State(ctx, e.uid))); len(got) != 1 || got[0] != b {
		t.Fatalf("đang nằm viện thì zombie còn nguyên: %v", got)
	}
}

// Hai nhát đập cùng lúc một con chỉ trừ máu một lần.
func TestHitStockConcurrentCostsOnce(t *testing.T) {
	e := newZombieEnv(t)
	id := e.addStock(placeYard, "chicken", 0)
	e.spawnStock(id, true)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.f.HitStockZombie(ctx, e.uid, id); err != nil && errStatus(err) != http.StatusConflict {
				t.Errorf("lỗi lạ: %v", err)
			}
		}()
	}
	wg.Wait()
	if hp := e.user().HP; hp != wallet.MaxHP-ZombieHitHP {
		t.Fatalf("HP: %d", hp)
	}
}

func TestStockWithZombieIsBlocked(t *testing.T) {
	e := newZombieEnv(t)
	grown := func() int64 {
		id := e.addStock(placeYard, "chicken", 900)
		e.exec(`UPDATE farm_stock SET product_progress = 600 WHERE id = ?`, id)
		return id
	}
	a, b := grown(), grown()
	e.spawnStock(a, false)
	_, _, err := e.f.SellStock(ctx, e.uid, placeYard, a)
	wantMsg(t, err, "Phải đập zombie trước")
	_, _, err = e.f.CollectStock(ctx, e.uid, placeYard, a)
	wantMsg(t, err, "Phải đập zombie trước")
	_, _, err = e.f.ProduceStock(ctx, e.uid, placeYard, a)
	wantMsg(t, err, "Phải đập zombie trước")

	// Hàng loạt bỏ qua con có zombie, chỉ lấy con còn lại.
	if _, g, err := e.f.ProduceAll(ctx, e.uid); err != nil || g.Count != 1 {
		t.Fatalf("ProduceAll: %+v %v", g, err)
	}
	if _, g, err := e.f.CollectAllAnimals(ctx, e.uid); err != nil || g.Count != 1 {
		t.Fatalf("CollectAllAnimals: %+v %v", g, err)
	}
	s := e.ok(e.f.State(ctx, e.uid))
	if len(s.Yard.Animals) != 1 || s.Yard.Animals[0].ID != a {
		t.Fatalf("con có zombie phải còn lại: %+v, b = %d", s.Yard.Animals, b)
	}
	// Chỉ còn con có zombie: báo rõ phải đập, không báo chưa có con nào.
	_, _, err = e.f.CollectAllAnimals(ctx, e.uid)
	wantMsg(t, err, "đập zombie")
	_, _, err = e.f.ProduceAll(ctx, e.uid)
	wantMsg(t, err, "đập zombie")

	fish, other := e.addStock(placePond, "tilapia", 600), e.addStock(placePond, "tilapia", 600)
	e.spawnStock(fish, false)
	if _, g, err := e.f.CollectAllFish(ctx, e.uid); err != nil || g.Count != 1 {
		t.Fatalf("CollectAllFish: %+v %v (con kia %d)", g, err, other)
	}
	_, _, err = e.f.CollectAllFish(ctx, e.uid)
	wantMsg(t, err, "đập zombie")
}

func TestStockWithZombieCanStillBeFed(t *testing.T) {
	e := newZombieEnv(t)
	id := e.addStock(placeYard, "chicken", 60)
	e.giveItem(e.uid, storeSeeds, "feed_poultry", 3)
	e.spawnStock(id, false)
	s, _, err := e.f.FeedStock(ctx, e.uid, placeYard, id)
	if err != nil {
		t.Fatalf("vẫn cho ăn được: %v", err)
	}
	if got := stockZombies(s); len(got) != 1 {
		t.Fatalf("cho ăn không đuổi được zombie: %v", got)
	}
}

// Đợt cũ trước; trong một đợt, ruộng trước rồi vật nuôi; hết đạn thì dừng.
func TestGunShootsStockInOrder(t *testing.T) {
	e := armed(t, 2)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	a, b := e.addStock(placeYard, "chicken", 0), e.addStock(placeYard, "chicken", 0)
	e.spawn(0, false)
	e.spawnStock(a, false)
	e.spawnStock(b, false)
	e.clock = time.Unix(e.wave()*hour+GunDelay-1, 0)
	if s := e.ok(e.f.State(ctx, e.uid)); s.Gun.Ammo != 2 || len(stockZombies(s)) != 2 {
		t.Fatalf("trước giây 30 chưa bắn: %+v %v", s.Gun, stockZombies(s))
	}
	e.clock = time.Unix(e.wave()*hour+GunDelay, 0)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.Gun.Ammo != 0 || s.GunKilled != 2 {
		t.Fatalf("đạn %d, diệt %d", s.Gun.Ammo, s.GunKilled)
	}
	if len(zombieCells(s)) != 0 {
		t.Fatalf("ruộng phải được bắn trước: %v", zombieCells(s))
	}
	if got := stockZombies(s); len(got) != 1 || got[0] != b {
		t.Fatalf("con id nhỏ được bắn trước, còn %v", got)
	}
}

// Đợt cũ của vật nuôi được bắn trước đợt mới của ruộng.
func TestGunShootsOlderWaveFirstAcrossTables(t *testing.T) {
	e := armed(t, 1)
	id := e.addStock(placeYard, "chicken", 0)
	e.spawnStock(id, false)
	e.exec(`UPDATE farm_stock_zombies SET wave = wave - 1 WHERE stock_id = ?`, id)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))
	e.spawn(0, false)
	e.clock = time.Unix(e.wave()*hour+GunDelay, 0)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.GunKilled != 1 || len(stockZombies(s)) != 0 || len(zombieCells(s)) != 1 {
		t.Fatalf("đợt cũ phải bị bắn trước: diệt %d, vật nuôi %v, ruộng %v", s.GunKilled, stockZombies(s), zombieCells(s))
	}
}

// Người offline lâu có tiến độ ruộng cũ: lần đầu mở sau deploy không mất con nào, đợt đầu của vật nuôi là giờ kế tiếp.
func TestNoRetroactiveStockWaves(t *testing.T) {
	e := newZombieEnv(t)
	id := e.addStock(placeBarn, "pig", 0)
	e.exec(`DELETE FROM farm_stock_zombie_progress WHERE user_id = ?`, e.uid)
	e.exec(`UPDATE farm_zombie_progress SET last_wave = ? WHERE user_id = ?`, e.wave()-300, e.uid)
	s := e.ok(e.f.State(ctx, e.uid))
	if len(s.Barn.Animals) != 1 || len(stockZombies(s)) != 0 || s.ZombieEatenStock != 0 {
		t.Fatalf("không tính bù vật nuôi: %+v, %v, %d", s.Barn.Animals, stockZombies(s), s.ZombieEatenStock)
	}
	e.nextWave()
	if got := stockZombies(e.ok(e.f.State(ctx, e.uid))); len(got) != 1 || got[0] != id {
		t.Fatalf("đợt kế tiếp mới có zombie: %v", got)
	}
	// Nông trại mới toanh: dòng tiến độ vật nuôi được tạo cả ở nhánh lần đầu thấy.
	f := newEnv(t, 1000, 0)
	f.f.noZombies = false
	f.ok(f.f.State(ctx, f.uid))
	if n := f.count(`SELECT count(*) FROM farm_stock_zombie_progress WHERE user_id = ?`, f.uid); n != 1 {
		t.Fatalf("dòng tiến độ vật nuôi: %d", n)
	}
}

// Con bị ăn: chuồng trống lại, giá trị giảm, báo đúng một lần; con không ăn thì bỏ đi.
func TestEatenStockFreesCapacityAndReportsOnce(t *testing.T) {
	e := newZombieEnv(t)
	eat := e.addStock(placeBarn, "cow", 0)
	keep := e.addStock(placeBarn, "cow", 0)
	before := e.ok(e.f.State(ctx, e.uid)).Barn.Used
	worth := e.worth()
	e.spawnStock(eat, true)
	e.spawnStock(keep, false)
	e.clock = time.Unix(e.wave()*hour+ZombieWindow, 0)
	s, err := e.f.do(ctx, e.uid, func(*wallet.Tx, State) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if s.ZombieEatenStock != 1 || len(s.Barn.Animals) != 1 || s.Barn.Animals[0].ID != keep || s.Barn.Animals[0].Zombie {
		t.Fatalf("sau hạn: ăn %d, %+v", s.ZombieEatenStock, s.Barn.Animals)
	}
	if s.Barn.Used >= before {
		t.Fatalf("chuồng phải trống lại: %d, trước %d", s.Barn.Used, before)
	}
	if w := e.worth(); w >= worth {
		t.Fatalf("giá trị phải giảm: %d, trước %d", w, worth)
	}
	if s = e.ok(e.f.State(ctx, e.uid)); s.ZombieEatenStock != 1 || s.ZombieEaten != 0 {
		t.Fatalf("GET báo một lần: %d, cây %d", s.ZombieEatenStock, s.ZombieEaten)
	}
	if s = e.ok(e.f.State(ctx, e.uid)); s.ZombieEatenStock != 0 {
		t.Fatalf("đã báo rồi: %d", s.ZombieEatenStock)
	}
	if s.ZombieUntil != 0 {
		t.Fatalf("hết zombie thì đồng hồ tắt: %d", s.ZombieUntil)
	}
}

// Không đường xoá nào để sót dòng zombie: mọi lệnh xoá con vật đều đi qua deleteStock.
func TestDeleteStockRemovesItsZombieRow(t *testing.T) {
	e := newZombieEnv(t)
	id := e.addStock(placeYard, "chicken", 0)
	e.spawnStock(id, false)
	tx, err := e.w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteStock(ctx, tx, e.uid, id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM farm_stock_zombies WHERE stock_id = ?`, id); n != 0 {
		t.Fatalf("sót %d dòng zombie", n)
	}
	if n := e.count(`SELECT count(*) FROM farm_stock WHERE id = ?`, id); n != 0 {
		t.Fatalf("con vật còn: %d", n)
	}
	// Mọi lệnh xoá farm_stock trong code đều phải đi qua deleteStock (trừ chính nó).
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		found += strings.Count(string(src), "DELETE FROM farm_stock WHERE")
	}
	if found != 1 {
		t.Fatalf("có %d lệnh xoá farm_stock, chỉ được 1 trong deleteStock", found)
	}
}

// Tính lại cùng (uid, đợt) trên dữ liệu giống nhau ra cùng kết quả.
func TestStockWaveIsDeterministic(t *testing.T) {
	picks := func() []int64 {
		e := newZombieEnv(t)
		for range 40 {
			e.addStock(placePond, "tilapia", 600)
		}
		e.nextWave()
		s, err := e.f.do(ctx, e.uid, func(*wallet.Tx, State) error { return errors.New("huỷ") })
		if err == nil {
			t.Fatal("phải huỷ transaction", s)
		}
		return stockZombies(e.ok(e.f.State(ctx, e.uid)))
	}
	a, b := picks(), picks()
	if len(a) != 3 || !slices.Equal(a, b) {
		t.Fatalf("hai lần khác nhau: %v / %v", a, b)
	}
}

// Đập được cả khi máu dưới mức làm nông, qua API thật; khách ghé thăm không có dữ liệu zombie vật nuôi.
func TestHitStockAPIAndVisit(t *testing.T) {
	e := newZombieEnv(t)
	u, token, err := e.w.Register(ctx, "keu", "matkhau123", wallet.Profile{Name: "Kêu"})
	if err != nil {
		t.Fatal(err)
	}
	owner := e.uid
	e.uid = u.ID
	defer func() { e.uid = owner }()
	e.ok(e.f.State(ctx, u.ID))
	id := e.addStock(placeYard, "chicken", 0)
	e.spawnStock(id, true)
	e.setHP(u.ID, 30)
	srv := httptest.NewServer(e.f.Handler())
	defer srv.Close()
	post := func(body string) (int, map[string]json.RawMessage) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/farm/zombie/hit-stock", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		out := map[string]json.RawMessage{}
		raw, _ := io.ReadAll(res.Body)
		json.Unmarshal(raw, &out)
		return res.StatusCode, out
	}
	code, out := post(`{"id":` + strconv.FormatInt(id, 10) + `}`)
	if code != http.StatusOK || out["farm"] == nil || out["user"] == nil {
		t.Fatalf("đập dưới 40 máu: %d %v", code, out)
	}
	if code, _ = post(`{"id":999999}`); code != http.StatusNotFound {
		t.Fatalf("con lạ: %d", code)
	}
	raw, err := json.Marshal(Visit{})
	if err != nil {
		t.Fatal(err)
	}
	// Khách thấy danh sách con rút gọn (Visit.Stock, xem TestVisitStockFields), không thấy chuồng và thông tin nuôi của chủ.
	if strings.Contains(string(raw), "animals") || strings.Contains(string(raw), "barn") {
		t.Fatalf("Visit lộ vật nuôi: %s", raw)
	}
}
