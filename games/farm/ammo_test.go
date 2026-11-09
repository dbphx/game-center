package farm

import (
	"net/http"
	"slices"
	"testing"
	"time"
)

// Chọn đạn theo bậc giá trị mục tiêu: đúng loại trước, hết thì loại cao hơn nếu viên đó không đắt hơn món bị tấn công,
// không bao giờ dùng loại thấp hơn bậc; mục tiêu dưới 300 xu không bắn.
func TestPickAmmo(t *testing.T) {
	all := Ammo{Normal: 1, Medium: 1, High: 1}
	cases := []struct {
		value int64
		have  Ammo
		want  string
	}{
		{100, all, ""},
		{299, all, ""},
		{300, all, AmmoNormal},
		{1_999, all, AmmoNormal},
		{2_000, all, AmmoMedium},
		{7_999, all, AmmoMedium},
		{8_000, all, AmmoHigh},
		{600_000, all, AmmoHigh},
		// Hết đạn thường: mục tiêu 1.900 dùng đạn trung (500 ≤ 1.900), mục tiêu 380 không bắn (500 > 380).
		{1_900, Ammo{Medium: 1, High: 1}, AmmoMedium},
		{380, Ammo{Medium: 1, High: 1}, ""},
		// Chỉ còn đạn cao: bò 6.060 và heo 1.530 vẫn bắn (1.500 ≤ giá trị), mục tiêu 1.499 thì không.
		{6_060, Ammo{High: 1}, AmmoHigh},
		{1_530, Ammo{High: 1}, AmmoHigh},
		{1_499, Ammo{High: 1}, ""},
		// Không dùng loại thấp hơn bậc.
		{8_000, Ammo{Normal: 5, Medium: 5}, ""},
		{2_000, Ammo{Normal: 5}, ""},
	}
	for _, c := range cases {
		if got := pickAmmo(c.value, c.have); got != c.want {
			t.Errorf("giá trị %d, có %+v: muốn %q, được %q", c.value, c.have, c.want, got)
		}
	}
}

// buyAmmo mua n viên loại typ (n > 0).
func (e *env) buyAmmo(typ string, n int) State {
	e.t.Helper()
	return e.ok(e.f.BuyAmmo(ctx, e.uid, typ, n))
}

func TestGunUsesMatchingAmmoTier(t *testing.T) {
	e := armed(t, 0)
	e.buyAmmo(AmmoNormal, 5)
	e.buyAmmo(AmmoMedium, 5)
	e.buyAmmo(AmmoHigh, 5)
	e.ok(e.f.Plant(ctx, e.uid, 0, "carrot"))       // 100: không bắn
	e.ok(e.f.Plant(ctx, e.uid, 1, "pumpkin"))      // 1.900: đạn thường
	e.ok(e.f.Plant(ctx, e.uid, 2, "durian"))       // 8.000: đạn cao
	chicken := e.addStock(placeYard, "chicken", 0) // 380: đạn thường
	fish := e.addStock(placePond, "tilapia", 0)    // 230: không bắn
	cow := e.addStock(placeBarn, "cow", 0)         // 6.060: đạn trung
	koi := e.addStock(placePond, "koi", 0)         // 11.600: đạn cao
	for i := range 3 {
		e.spawn(i, true)
	}
	for _, id := range []int64{chicken, fish, cow, koi} {
		e.spawnStock(id, true)
	}
	e.clock = e.clock.Add(GunDelay * time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.GunKilled != 5 || s.Gun.Ammo != (Ammo{Normal: 3, Medium: 4, High: 3}) {
		t.Fatalf("bắn đúng loại: diệt %d, còn %+v", s.GunKilled, s.Gun.Ammo)
	}
	if got := zombieCells(s); len(got) != 1 || got[0] != 0 {
		t.Fatalf("chỉ còn zombie ô cà rốt: %v", got)
	}
	if got := stockZombies(s); len(got) != 1 || !slices.Contains(got, fish) {
		t.Fatalf("chỉ còn zombie cá rô phi: %v", got)
	}
	// Hết hạn: zombie bị bỏ qua ăn như khi không có súng; đạn không bị trừ thêm.
	e.clock = e.clock.Add(ZombieWindow * time.Second)
	s = e.ok(e.f.State(ctx, e.uid))
	if s.Plots[0].Crop != "" || s.Plots[2].Crop != "durian" || s.Gun.Ammo != (Ammo{Normal: 3, Medium: 4, High: 3}) {
		t.Fatalf("hết hạn: ô %+v, đạn %+v", s.Plots[:3], s.Gun.Ammo)
	}
}

func TestGunFallsBackOnlyToHigherTier(t *testing.T) {
	e := armed(t, 0)
	e.buyAmmo(AmmoMedium, 1)
	e.ok(e.f.Plant(ctx, e.uid, 0, "durian"))       // cần đạn cao, chỉ có trung → không bắn
	e.ok(e.f.Plant(ctx, e.uid, 1, "pumpkin"))      // 1.900, hết đạn thường → dùng trung
	chicken := e.addStock(placeYard, "chicken", 0) // 380 < 500 → không bắn bằng đạn trung
	e.spawn(0, true)
	e.spawn(1, true)
	e.spawnStock(chicken, true)
	e.clock = e.clock.Add(GunDelay * time.Second)
	s := e.ok(e.f.State(ctx, e.uid))
	if s.GunKilled != 1 || s.Gun.Ammo != (Ammo{}) {
		t.Fatalf("bí ngô được bắn bằng đạn trung: diệt %d, còn %+v", s.GunKilled, s.Gun.Ammo)
	}
	if got := zombieCells(s); len(got) != 1 || got[0] != 0 {
		t.Fatalf("sầu riêng không được bắn bằng đạn thấp hơn: %v", got)
	}
	if got := stockZombies(s); len(got) != 1 || got[0] != chicken {
		t.Fatalf("gà không bị bắn bằng đạn đắt hơn nó: %v", got)
	}
}

func TestBuyAmmoTypesAndPrices(t *testing.T) {
	e := armed(t, 0)
	for _, c := range []struct {
		typ   string
		price int64
	}{{AmmoNormal, 100}, {AmmoMedium, 500}, {AmmoHigh, 1_500}} {
		before := e.user().Balance
		e.buyAmmo(c.typ, 2)
		if got := before - e.user().Balance; got != 2*c.price {
			t.Fatalf("2 viên %s: %d", c.typ, got)
		}
	}
	if s := e.ok(e.f.State(ctx, e.uid)); s.Gun.Ammo != (Ammo{Normal: 2, Medium: 2, High: 2}) {
		t.Fatalf("đạn: %+v", s.Gun.Ammo)
	}
	for _, typ := range []string{"", "super"} {
		_, err := e.f.BuyAmmo(ctx, e.uid, typ, 1)
		wantStatus(t, err, http.StatusBadRequest)
	}
	types, ok := Catalog()["ammoTypes"].([]AmmoType)
	if !ok || len(types) != 3 || types[0].ID != AmmoNormal || types[0].Min != 300 || types[1].Min != 2_000 || types[2].Min != 8_000 ||
		types[2].Price != 1_500 {
		t.Fatalf("catalog ammoTypes: %+v", Catalog()["ammoTypes"])
	}
}

// Đạn cũ (một loại, người chơi đã trả 2.500 xu/viên) giữ 1:1 thành Đạn cao.
func TestMigrateOldAmmoBecomesHigh(t *testing.T) {
	e := newEnv(t, 0, 0)
	e.exec(`ALTER TABLE farm_guns DROP COLUMN ammo_normal`)
	e.exec(`ALTER TABLE farm_guns DROP COLUMN ammo_medium`)
	e.exec(`INSERT INTO farm_guns (user_id, ammo) VALUES (?, 7)`, e.uid)
	if _, err := New(ctx, e.w); err != nil {
		t.Fatal(err)
	}
	if s := e.ok(e.f.State(ctx, e.uid)); !s.Gun.Owned || s.Gun.Ammo != (Ammo{High: 7}) {
		t.Fatalf("đạn cũ thành đạn cao: %+v", s.Gun)
	}
}
