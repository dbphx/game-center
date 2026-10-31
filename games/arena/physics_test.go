package arena

import (
	"math"
	mrand "math/rand/v2"
	"testing"
)

func flat(y int) Ground {
	g := make(Ground, Width)
	for i := range g {
		g[i] = y
	}
	return g
}

// landX là chỗ rơi theo công thức (không gió) xuống mặt phẳng y.
func landX(x, y, angle, power float64, ground float64) float64 {
	px, py, vx, vy := Launch(x, y, angle, power)
	// py + vy t + G t²/2 = ground
	a, b, c := Gravity/2, vy, py-ground
	t := (-b + math.Sqrt(b*b-4*a*c)) / (2 * a)
	return px + vx*t
}

func TestShotLandsWhereFormulaSays(t *testing.T) {
	g := flat(700)
	for _, tc := range [][2]float64{{45, 50}, {60, 70}, {30, 40}} {
		f := Simulate(g, 200, 700, tc[0], tc[1], 0, "a", nil)
		want := landX(200, 700, tc[0], tc[1], 700)
		if !f.Hit || math.Abs(f.X-want) > 8 || math.Abs(f.Y-700) > 1 {
			t.Fatalf("góc %v lực %v: rơi ở (%.1f, %.1f), công thức %.1f", tc[0], tc[1], f.X, f.Y, want)
		}
		last := f.Path[len(f.Path)-1]
		if last[0] != int(math.Round(f.X)) || last[1] != int(math.Round(f.Y)) {
			t.Fatalf("điểm cuối quỹ đạo phải là chỗ nổ: %v", last)
		}
	}
	// Bắn sang trái (góc 135) đối xứng với góc 45.
	r := Simulate(g, 1400, 700, 135, 50, 0, "a", nil)
	l := Simulate(g, 200, 700, 45, 50, 0, "a", nil)
	if math.Abs((1400-r.X)-(l.X-200)) > 1 {
		t.Fatalf("đối xứng: trái %.1f phải %.1f", 1400-r.X, l.X-200)
	}
	// Lực lớn bay ra khỏi sân: trượt.
	if f := Simulate(g, 1500, 700, 30, 100, 0, "a", nil); f.Hit {
		t.Fatalf("bay ra ngoài phải trượt: %+v", f.X)
	}
}

func TestWindPushesShot(t *testing.T) {
	g := flat(700)
	calm := Simulate(g, 200, 700, 60, 60, 0, "a", nil)
	right := Simulate(g, 200, 700, 60, 60, 10, "a", nil)
	left := Simulate(g, 200, 700, 60, 60, -10, "a", nil)
	if !(left.X < calm.X-50 && right.X > calm.X+50) {
		t.Fatalf("gió đẩy lệch: trái %.0f, lặng %.0f, phải %.0f", left.X, calm.X, right.X)
	}
}

func TestShotHitsBody(t *testing.T) {
	g := flat(700)
	l := Simulate(g, 200, 700, 45, 50, 0, "a", nil)
	target := Body{ID: "b", X: math.Round(l.X), Y: 700}
	f := Simulate(g, 200, 700, 45, 50, 0, "a", []Body{{ID: "a", X: 200, Y: 700}, target})
	if !f.Hit || f.Body != "b" {
		t.Fatalf("phải trúng thân b: %+v", f.Body)
	}
	// Không tự trúng mình ngay khi ra nòng.
	if f := Simulate(g, 200, 700, 80, 60, 0, "a", []Body{{ID: "a", X: 200, Y: 700}}); f.Body == "a" && f.Time < SelfSafe {
		t.Fatal("tự trúng mình khi vừa ra nòng")
	}
}

func TestCraterOnlyLowers(t *testing.T) {
	g := flat(700)
	g[790] = 600 // cột cao hơn tâm nổ
	Crater(g, 800, 700, 40)
	if g[800] != 740 || g[760] != 700 || g[700] != 700 || g[841] != 700 {
		t.Fatalf("hố: %d %d %d %d", g[800], g[760], g[700], g[841])
	}
	for x := 760; x <= 840; x++ {
		if g[x] < 700 && x != 790 {
			t.Fatalf("đất bị nâng ở %d: %d", x, g[x])
		}
	}
	if g[790] < 700 {
		t.Fatalf("cột cao phải bị khoét xuống: %d", g[790])
	}
	deep := flat(880)
	Crater(deep, 400, 880, 65)
	if deep[400] != Height {
		t.Fatalf("hố sâu kẹp ở đáy %d: %d", Height, deep[400])
	}
}

func TestSplashFalloffGearArmor(t *testing.T) {
	if Splash(30, 40, 0, 1, 0) != 30 || Splash(30, 40, 60, 1, 0) != 0 || Splash(30, 40, 100, 1, 0) != 0 {
		t.Fatal("tâm nổ đủ sát thương, ngoài r+20 không mất máu")
	}
	prev := 31
	for d := 0.0; d < 60; d += 5 {
		n := Splash(30, 40, d, 1, 0)
		if n > prev {
			t.Fatalf("sát thương phải giảm theo khoảng cách: d=%v %d > %d", d, n, prev)
		}
		prev = n
	}
	gun := Gear{Gun: 10} // đại bác +10: tấn công 20
	armor := Gear{Armor: 5}
	if n := Splash(30, 40, 0, gun.DmgMul(), 0); n != 36 {
		t.Fatalf("đại bác +10 = ×1.2: %d", n)
	}
	if n := Splash(30, 40, 0, 1, armor.ArmorPct()); n != 24 {
		t.Fatalf("giáp +5 (15%%) + phòng thủ 5: %d", n)
	}
	// Đại bác +1: 11/6/2/11 → 2·30.
	if (Gear{Gun: 1, Hat: 2, Armor: 3}).Power() != 1000+60+160+300 || (Gear{Hat: 4}).MaxHP() != 270 {
		t.Fatal("lực chiến / máu tối đa")
	}
	// Phòng thủ tối đa 60%.
	if p := (Gear{Arm: "tulanh", Gun: 10, Armor: 10}).ArmorPct(); p != MaxArmorPct {
		t.Fatalf("giảm sát thương tối đa: %v", p)
	}
}

func TestFireDamageDeathTele(t *testing.T) {
	g := flat(700)
	a := &Fighter{ID: "a", X: 200, Y: 700, HP: 200, MaxHP: 200, Alive: true}
	l := Simulate(g, 200, 700, 45, 50, 0, "a", nil)
	b := &Fighter{ID: "b", X: math.Round(l.X), Y: 700, HP: 200, MaxHP: 200, Alive: true}
	shell, _ := findWeapon(WeaponShell)
	res := Fire(g, []*Fighter{a, b}, a, shell, 45, 50, 0, nil)
	if len(res.Hits) != 1 || res.Hits[0].ID != "b" || res.Hits[0].Dmg != 31 || b.HP != 169 || len(res.Blasts) != 1 {
		t.Fatalf("trúng trực tiếp 30×1.1×0.95: %+v", res.Hits)
	}
	if b.Y <= 700 {
		t.Fatalf("hố dưới chân thì rơi xuống: %v", b.Y)
	}
	// Đạn chùm: 3 quỹ đạo.
	triple, _ := findWeapon(WeaponTriple)
	if res := Fire(g, []*Fighter{a, b}, a, triple, 45, 50, 0, nil); len(res.Paths) != 3 {
		t.Fatalf("đạn chùm 3 viên: %d", len(res.Paths))
	}
	// Đứng sát vực: bom tạ khoét xuống vực thì rơi chết.
	edge := flat(860)
	c := &Fighter{ID: "c", X: 200, Y: 860, HP: 200, MaxHP: 200, Alive: true}
	lc := Simulate(edge, 200, 860, 45, 50, 0, "c", nil)
	d := &Fighter{ID: "d", X: math.Round(lc.X), Y: 860, HP: 999, MaxHP: 999, Alive: true, Gear: Gear{Armor: 10}}
	heavy, _ := findWeapon(WeaponHeavy)
	res = Fire(edge, []*Fighter{c, d}, c, heavy, 45, 50, 0, nil)
	if d.Alive || len(res.Deaths) != 1 || res.Deaths[0] != "d" {
		t.Fatalf("rơi vực phải chết: %+v %+v", d, res.Deaths)
	}
	// Đạn dịch chuyển: người bắn tới chỗ đạn rơi, không khoét đất, không mất máu.
	g2 := flat(700)
	e := &Fighter{ID: "e", X: 200, Y: 700, HP: 200, MaxHP: 200, Alive: true}
	tele, _ := findWeapon(WeaponTele)
	res = Fire(g2, []*Fighter{e}, e, tele, 45, 50, 0, nil)
	if res.Tele == nil || math.Abs(e.X-l.X) > 1 || len(res.Blasts) != 0 || g2[int(e.X)] != 700 || e.HP != 200 {
		t.Fatalf("dịch chuyển: %+v %+v", res.Tele, e)
	}
}

func TestWalkFundSlopeAbyss(t *testing.T) {
	g := flat(700)
	if x, used := Walk(g, 200, 1000, MoveFund); x != 320 || used != 120 {
		t.Fatalf("hết quỹ: %v %d", x, used)
	}
	if x, _ := Walk(g, 200, 150, MoveFund); x != 150 {
		t.Fatalf("đi trái tới đích: %v", x)
	}
	g[260] = 690 // vách cao 10 > MaxClimb
	if x, _ := Walk(g, 200, 300, MoveFund); x != 259 {
		t.Fatalf("dốc quá cao phải dừng: %v", x)
	}
	g[240] = 900
	if x, _ := Walk(g, 200, 300, MoveFund); x != 239 {
		t.Fatalf("không bước xuống vực: %v", x)
	}
}

func TestMapsGenerate(t *testing.T) {
	rng := mrand.New(mrand.NewPCG(1, 2))
	for _, m := range Maps {
		g := m.Generate(rng)
		if len(g) != Width {
			t.Fatal(m.ID)
		}
		abyss := 0
		for _, y := range g {
			if y < 240 || y > Height {
				t.Fatalf("%s: độ cao %d ngoài khung", m.ID, y)
			}
			if y >= AbyssY {
				abyss++
			}
		}
		if (m.ID == "beach") != (abyss > 0) {
			t.Fatalf("%s: số cột vực %d (chỉ bãi biển có vũng nước giữa)", m.ID, abyss)
		}
		for range 20 {
			if w := m.Wind(rng); w < -m.wind || w > m.wind {
				t.Fatalf("%s gió %d", m.ID, w)
			}
		}
		for _, x := range spawnXs(g, 4, rng) {
			if g.At(x) >= AbyssY {
				t.Fatalf("%s: chỗ đứng %v trên vực", m.ID, x)
			}
		}
	}
}

func TestBotAimsClose(t *testing.T) {
	g := flat(700)
	me := &Fighter{ID: "m", X: 200, Y: 700, HP: 200, Alive: true}
	foe := &Fighter{ID: "f", X: 1100, Y: 700, HP: 200, Alive: true}
	rng := mrand.New(mrand.NewPCG(3, 4))
	for _, wind := range []int{0, 7, -7} {
		aim := BotAim(g, []*Fighter{me, foe}, me, BotHard, wind, rng)
		f := Simulate(g, me.X, me.Y, aim.Angle, aim.Power, wind, me.ID, bodiesOf([]*Fighter{me, foe}))
		if !f.Hit || math.Abs(f.X-foe.X) > 60 {
			t.Fatalf("máy khó gió %d bắn lệch: rơi ở %.0f, mục tiêu %.0f", wind, f.X, foe.X)
		}
	}
}
