package race

import (
	"context"
	"errors"
	"math"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/dbphx/game-center/internal/wallet"
)

var ctx = context.Background()

type garageEnv struct {
	t   *testing.T
	w   *wallet.Wallet
	g   *Garage
	uid int64
}

func newGarageEnv(t *testing.T, balance int64) *garageEnv {
	t.Helper()
	w, err := wallet.Open(filepath.Join(t.TempDir(), "race.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	g, err := NewGarage(ctx, w)
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := w.Register(ctx, "taydua", "matkhau123", wallet.Profile{Name: "Tay đua"})
	if err != nil {
		t.Fatal(err)
	}
	e := &garageEnv{t: t, w: w, g: g, uid: u.ID}
	e.setBalance(balance)
	return e
}

func (e *garageEnv) setBalance(b int64) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = ? WHERE id = ?`, b, e.uid); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

// setLevel đặt XP đúng mức đầu của level.
func (e *garageEnv) setLevel(level int) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET xp = ? WHERE id = ?`, wallet.LevelStart(level), e.uid); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func (e *garageEnv) balance() int64 {
	e.t.Helper()
	b, err := e.w.Balance(ctx, e.uid)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func wantStatus(t *testing.T, err error, status int) {
	t.Helper()
	if got := errStatus(err); got != status {
		t.Fatalf("lỗi %v: mã %d, muốn %d", err, got, status)
	}
}

func carOf(v GarageView, id string) CarView {
	for _, c := range v.Cars {
		if c.ID == id {
			return c
		}
	}
	return CarView{}
}

func itemOf(v GarageView, id string) ItemView {
	for _, it := range v.Items {
		if it.ID == id {
			return it
		}
	}
	return ItemView{}
}

func TestCarCatalogAndStats(t *testing.T) {
	// 12 ô tô + 3 moto, thứ tự hạng theo giá.
	want := []struct {
		id    string
		price int64
		kind  string
		level int
	}{
		{"starter", 0, KindCar, 1}, {"comet", 60_000, KindCar, 3}, {"moto_dash", 90_000, KindMoto, 5}, {"sprint", 150_000, KindCar, 7},
		{"bolt", 240_000, KindCar, 9}, {"raptor", 450_000, KindCar, 12}, {"viper", 750_000, KindCar, 15},
		{"moto_storm", 900_000, KindMoto, 17}, {"zenith", 1_200_000, KindCar, 19}, {"phantom", 2_400_000, KindCar, 22},
		{"moto_neon", 3_000_000, KindMoto, 24}, {"vortex", 3_600_000, KindCar, 26}, {"apex", 4_800_000, KindCar, 28},
		{"blaze", 5_400_000, KindCar, 29}, {"nova", 6_000_000, KindCar, 30},
	}
	if len(Cars) != len(want) {
		t.Fatalf("có %d mẫu, muốn %d", len(Cars), len(want))
	}
	for i, c := range Cars {
		if c.ID != want[i].id || c.Price != want[i].price || c.Kind != want[i].kind || c.Level != want[i].level || c.Rank != i {
			t.Fatalf("mẫu %d: %+v, muốn %+v", i, c, want[i])
		}
	}
	s0 := StatsOf(Cars[0], Levels{})
	if s0.MaxSpeed != 30 || s0.Accel != 12 || math.Abs(s0.Grip-0.70) > 1e-9 {
		t.Fatalf("xe khởi đầu cấp 0: %+v", s0)
	}
	nova, _ := findCar("nova")
	top := StatsOf(nova, Levels{MaxLevel, MaxLevel, MaxLevel})
	for name, r := range map[string]float64{"tốc độ": top.MaxSpeed / s0.MaxSpeed, "tăng tốc": top.Accel / s0.Accel, "bám đường": top.Grip / s0.Grip} {
		if r < 1.19 || r > 1.21 {
			t.Fatalf("%s xe đắt nhất cấp 5 hơn xe khởi đầu %.3f lần, muốn ≈1,20", name, r)
		}
	}
	// Không mẫu nào (kể cả moto) vượt trần +20% ở bất kỳ chỉ số nào; hạng cao hơn thì tốc độ không thấp hơn.
	prev := 0.0
	for _, c := range Cars {
		st := StatsOf(c, Levels{MaxLevel, MaxLevel, MaxLevel})
		if st.MaxSpeed > s0.MaxSpeed*1.2+0.01 || st.Accel > s0.Accel*1.2+0.01 || st.Grip > math.Min(1, s0.Grip*1.2)+0.01 {
			t.Fatalf("%s vượt trần: %+v", c.ID, st)
		}
		if st.MaxSpeed < prev {
			t.Fatalf("%s chậm hơn xe hạng dưới", c.ID)
		}
		prev = st.MaxSpeed
	}
	// Giá nâng cấp lên cấp n = giá xe × 10% × n; xe khởi đầu tính như 30.000.
	if p := upgradePrice(Cars[0], 3); p != 9_000 {
		t.Fatalf("xe khởi đầu lên cấp 3: %d", p)
	}
	if p := upgradePrice(nova, 5); p != 3_000_000 {
		t.Fatalf("xe đắt nhất lên cấp 5: %d", p)
	}
}

// Moto: tăng tốc +5%, bám đường −5% so với ô tô cùng hạng (khi chưa chạm trần); không lắp được cánh gió.
func TestMotoStatsAndSpoiler(t *testing.T) {
	m, _ := findCar("moto_storm")
	asCar := m
	asCar.Kind = KindCar
	ms, cs := StatsOf(m, Levels{}), StatsOf(asCar, Levels{}) // chỉ số làm tròn 2 chữ số nên so sai số 1,5%
	if ms.MaxSpeed != cs.MaxSpeed || math.Abs(ms.Accel/cs.Accel-1.05) > 0.015 || math.Abs(ms.Grip/cs.Grip-0.95) > 0.015 {
		t.Fatalf("moto %+v so với ô tô cùng hạng %+v", ms, cs)
	}
	e := newGarageEnv(t, 1_000_000)
	e.setLevel(5)
	if _, err := e.g.BuyCar(ctx, e.uid, "moto_dash"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.g.BuyItem(ctx, e.uid, "spoiler_low"); err != nil {
		t.Fatal(err)
	}
	_, err := e.g.Equip(ctx, e.uid, "moto_dash", "spoiler", "spoiler_low")
	wantStatus(t, err, http.StatusConflict)
	v, err := e.g.Equip(ctx, e.uid, "moto_dash", "neon", "")
	if err != nil {
		t.Fatalf("moto vẫn lắp / tháo đồ khác được: %v", err)
	}
	if c := carOf(v, "moto_dash"); c.Kind != KindMoto || c.Cosmetics.Spoiler != "" {
		t.Fatalf("moto trong garage: %+v", c)
	}
	if v.NitroCharge != NitroCharge || NitroCharge != 2 {
		t.Fatalf("hệ số tích nitro: %v", v.NitroCharge)
	}
	if carOf(v, "starter").Kind != KindCar {
		t.Fatal("ô tô phải có kind car")
	}
}

func TestGarageStarterAndBuyCar(t *testing.T) {
	e := newGarageEnv(t, 70_000)
	v, err := e.g.View(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if v.Current != "starter" || !carOf(v, "starter").Owned || carOf(v, "comet").Owned || v.MaxLevel != MaxLevel {
		t.Fatalf("garage mới: %+v", v)
	}
	if carOf(v, "starter").Cosmetics.Paint == "" {
		t.Fatal("xe khởi đầu phải có màu sơn")
	}
	if v.UpgradePrice["starter"]["engine"] != 3_000 {
		t.Fatalf("giá nâng động cơ cấp 1: %v", v.UpgradePrice["starter"])
	}

	// Chưa đủ cấp: báo "Cần cấp N", không trừ xu.
	_, err = e.g.BuyCar(ctx, e.uid, "comet")
	wantStatus(t, err, http.StatusForbidden)
	if err.Error() != "Cần cấp 3 để mua Sao Chổi" || e.balance() != 70_000 || carOf(v, "comet").Level != 3 {
		t.Fatalf("chưa đủ cấp: %v, xu %d", err, e.balance())
	}
	e.setLevel(9)
	_, err = e.g.BuyCar(ctx, e.uid, "bolt") // 240.000, không đủ
	wantStatus(t, err, http.StatusPaymentRequired)
	_, err = e.g.BuyCar(ctx, e.uid, "khong-co")
	wantStatus(t, err, http.StatusNotFound)
	if v, err = e.g.BuyCar(ctx, e.uid, "comet"); err != nil {
		t.Fatal(err)
	}
	if !carOf(v, "comet").Owned || v.Current != "comet" || e.balance() != 10_000 {
		t.Fatalf("mua Sao Chổi: %+v, xu %d", v, e.balance())
	}
	_, err = e.g.BuyCar(ctx, e.uid, "comet")
	wantStatus(t, err, http.StatusConflict)

	if v, err = e.g.SelectCar(ctx, e.uid, "starter"); err != nil || v.Current != "starter" {
		t.Fatalf("chọn xe: %v %+v", err, v)
	}
	_, err = e.g.SelectCar(ctx, e.uid, "nova")
	wantStatus(t, err, http.StatusConflict)
	lo, err := e.g.Loadout(ctx, e.uid)
	if err != nil || lo.ID != "starter" {
		t.Fatalf("xe đang dùng: %+v %v", lo, err)
	}
}

func TestUpgradeToLevelFive(t *testing.T) {
	e := newGarageEnv(t, 100_000)
	var v GarageView
	var err error
	for lv := 1; lv <= MaxLevel; lv++ {
		before := e.balance()
		if v, err = e.g.Upgrade(ctx, e.uid, "starter", "engine"); err != nil {
			t.Fatal(err)
		}
		if paid := before - e.balance(); paid != int64(lv)*3_000 {
			t.Fatalf("lên cấp %d trả %d", lv, paid)
		}
	}
	c := carOf(v, "starter")
	if c.Levels.Engine != MaxLevel || v.UpgradePrice["starter"]["engine"] != 0 {
		t.Fatalf("cấp tối đa: %+v %v", c.Levels, v.UpgradePrice["starter"])
	}
	if c.Stats.MaxSpeed <= 30 || c.Stats.Accel != 12 {
		t.Fatalf("chỉ số sau nâng động cơ: %+v", c.Stats)
	}
	before := e.balance()
	_, err = e.g.Upgrade(ctx, e.uid, "starter", "engine")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.g.Upgrade(ctx, e.uid, "starter", "canh")
	wantStatus(t, err, http.StatusBadRequest)
	_, err = e.g.Upgrade(ctx, e.uid, "comet", "turbo")
	wantStatus(t, err, http.StatusConflict) // chưa có xe
	if e.balance() != before {
		t.Fatal("thao tác lỗi không được trừ xu")
	}
	e.setBalance(0)
	_, err = e.g.Upgrade(ctx, e.uid, "starter", "tires")
	wantStatus(t, err, http.StatusPaymentRequired)
}

func TestBuyAndEquipItems(t *testing.T) {
	e := newGarageEnv(t, 100_000)
	v, _ := e.g.View(ctx, e.uid)
	if !itemOf(v, "paint_blue").Owned || itemOf(v, "paint_blue").Price != 0 {
		t.Fatal("sơn miễn phí, ai cũng có")
	}
	if itemOf(v, "neon_pink").Owned {
		t.Fatal("chưa mua đèn neon")
	}
	for _, it := range Items {
		if it.Kind != "paint" && (it.Price < 10_000 || it.Price > 100_000) {
			t.Fatalf("giá đồ trang trí %s = %d, ngoài 10.000–100.000", it.ID, it.Price)
		}
	}
	_, err := e.g.Equip(ctx, e.uid, "starter", "neon", "neon_pink")
	wantStatus(t, err, http.StatusConflict) // chưa mua
	before := e.balance()
	if v, err = e.g.BuyItem(ctx, e.uid, "neon_pink"); err != nil {
		t.Fatal(err)
	}
	if !itemOf(v, "neon_pink").Owned || before-e.balance() != itemOf(v, "neon_pink").Price {
		t.Fatal("mua đèn neon")
	}
	_, err = e.g.BuyItem(ctx, e.uid, "neon_pink")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.g.BuyItem(ctx, e.uid, "paint_blue")
	wantStatus(t, err, http.StatusConflict) // sơn có sẵn
	if v, err = e.g.Equip(ctx, e.uid, "starter", "neon", "neon_pink"); err != nil {
		t.Fatal(err)
	}
	if carOf(v, "starter").Cosmetics.Neon != "neon_pink" {
		t.Fatal("lắp đèn neon")
	}
	if v, err = e.g.Equip(ctx, e.uid, "starter", "paint", "paint_blue"); err != nil || carOf(v, "starter").Cosmetics.Paint != "paint_blue" {
		t.Fatalf("đổi sơn: %v", err)
	}
	_, err = e.g.Equip(ctx, e.uid, "starter", "spoiler", "neon_pink")
	wantStatus(t, err, http.StatusBadRequest) // sai loại
	_, err = e.g.Equip(ctx, e.uid, "starter", "paint", "")
	wantStatus(t, err, http.StatusBadRequest) // xe phải có sơn
	if v, err = e.g.Equip(ctx, e.uid, "starter", "neon", ""); err != nil || carOf(v, "starter").Cosmetics.Neon != "" {
		t.Fatalf("tháo đèn neon: %v", err)
	}
	lo, _ := e.g.Loadout(ctx, e.uid)
	if lo.Cosmetics.Paint != "paint_blue" {
		t.Fatalf("xe vào phòng mang đồ đang lắp: %+v", lo)
	}
}

func TestGarageErrorsAreTyped(t *testing.T) {
	if errStatus(wallet.ErrNotEnoughCoins) != http.StatusPaymentRequired {
		t.Fatal("thiếu xu phải 402")
	}
	if errStatus(errors.New("lạ")) != http.StatusInternalServerError {
		t.Fatal("lỗi lạ phải 500")
	}
}
