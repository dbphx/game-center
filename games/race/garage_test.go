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
	if len(Cars) != 6 || Cars[0].Price != 0 {
		t.Fatalf("cần 1 xe miễn phí + 5 xe mua: %+v", Cars)
	}
	want := []int64{0, 20_000, 80_000, 250_000, 800_000, 2_000_000}
	for i, c := range Cars {
		if c.Price != want[i] {
			t.Fatalf("giá %s = %d, muốn %d", c.ID, c.Price, want[i])
		}
	}
	s0 := StatsOf(Cars[0], Levels{})
	if s0.MaxSpeed != 30 || s0.Accel != 12 || math.Abs(s0.Grip-0.70) > 1e-9 {
		t.Fatalf("xe khởi đầu cấp 0: %+v", s0)
	}
	top := StatsOf(Cars[len(Cars)-1], Levels{MaxLevel, MaxLevel, MaxLevel})
	for name, r := range map[string]float64{"tốc độ": top.MaxSpeed / s0.MaxSpeed, "tăng tốc": top.Accel / s0.Accel, "bám đường": top.Grip / s0.Grip} {
		if r < 1.19 || r > 1.21 {
			t.Fatalf("%s xe đắt nhất cấp 5 hơn xe khởi đầu %.3f lần, muốn ≈1,20", name, r)
		}
	}
	if top.Grip > 1 {
		t.Fatalf("bám đường vượt 1: %v", top.Grip)
	}
	// Giá nâng cấp lên cấp n = giá xe × 10% × n; xe khởi đầu tính như 10.000.
	if p := upgradePrice(Cars[0], 3); p != 3_000 {
		t.Fatalf("xe khởi đầu lên cấp 3: %d", p)
	}
	if p := upgradePrice(Cars[5], 5); p != 1_000_000 {
		t.Fatalf("xe đắt nhất lên cấp 5: %d", p)
	}
}

func TestGarageStarterAndBuyCar(t *testing.T) {
	e := newGarageEnv(t, 30_000)
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
	if v.UpgradePrice["starter"]["engine"] != 1_000 {
		t.Fatalf("giá nâng động cơ cấp 1: %v", v.UpgradePrice["starter"])
	}

	_, err = e.g.BuyCar(ctx, e.uid, "bolt") // 80.000, không đủ
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
		if paid := before - e.balance(); paid != int64(lv)*1_000 {
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
		if it.Kind != "paint" && (it.Price < 5_000 || it.Price > 50_000) {
			t.Fatalf("giá đồ trang trí %s = %d, ngoài 5.000–50.000", it.ID, it.Price)
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
