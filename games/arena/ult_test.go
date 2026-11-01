package arena

import (
	"errors"
	"math"
	"testing"
	"time"
)

// aimAt tìm góc / lực trúng thẳng thân target (sân phẳng, không gió).
func aimAt(t *testing.T, g Ground, fs []*Fighter, me, target *Fighter) (float64, float64) {
	t.Helper()
	lo, hi := 15.0, 85.0
	if target.X < me.X {
		lo, hi = 95, 165
	}
	for a := lo; a <= hi; a++ {
		for p := 10.0; p <= 100; p += 0.5 {
			if f := Simulate(g, me.X, me.Y, a, p, 0, me.ID, targetsOf(fs, me)); f.Body == target.ID {
				return a, p
			}
		}
	}
	t.Fatal("không tìm được phát trúng")
	return 0, 0
}

func fighter(id string, x float64) *Fighter {
	return &Fighter{ID: id, X: x, Y: 700, HP: 1000, MaxHP: 1000, Alive: true, Gear: Gear{Arm: ArmCannon}}
}

func TestUltTable(t *testing.T) {
	want := map[string][3]float64{ArmCannon: {45, 60, 3}, "dep": {22, 30, 5}, "choi": {60, 110, 1}, "launha": {50, 70, 1},
		"tv": {80, 60, 1}, "tulanh": {110, 90, 1}}
	for _, a := range Arms {
		u := UltOf(a.ID)
		w, ok := want[a.ID]
		if !ok || u.Arm != a.ID || u.ID != WeaponUlt || u.Dmg != w[0] || u.R != w[1] || float64(len(u.Spread)) != w[2] {
			t.Fatalf("chiêu cuối %s: %+v", a.ID, u)
		}
	}
	if UltOf("launha").Push != 80 || !UltOf("tv").Drain {
		t.Fatal("Lau sàn đẩy 80, Sóng nhiễu xoá nộ")
	}
}

func TestUltPushAndAbyss(t *testing.T) {
	g := flat(700)
	me, a, b, boss := fighter("me", 300), fighter("a", 700), fighter("b", 745), fighter("boss", 660)
	boss.Fixed, boss.Y = true, 700
	boss.Alive = false // trận boss kiểm riêng bên dưới
	fs := []*Fighter{me, a, b, boss}
	ang, pw := aimAt(t, g, []*Fighter{me, a}, me, a)
	res := Fire(g, fs, me, UltOf("launha").Weapon, ang, pw, 0, nil)
	if len(res.Blasts) != 1 {
		t.Fatalf("nổ: %+v", res.Blasts)
	}
	bx := float64(res.Blasts[0].X)
	if b.X != 745+80 || math.Abs(a.X-700) != 80 || (bx != 700 && (a.X-700)*(700-bx) < 0) {
		t.Fatalf("đẩy văng ra xa tâm nổ %v: a=%v b=%v", bx, a.X, b.X)
	}
	if len(res.Pushes) != 2 || me.X != 300 {
		t.Fatalf("pushes: %+v, người bắn không bị đẩy: %v", res.Pushes, me.X)
	}

	// Đứng sát mép phải: bị đẩy ra ngoài sân → rơi vực chết; boss (Fixed) đứng yên.
	g = flat(700)
	me, a, boss = fighter("me", 1250), fighter("a", 1555), fighter("boss", 1555)
	boss.Fixed = true
	ang, pw = aimAt(t, g, []*Fighter{me, a}, me, a)
	res = Fire(g, []*Fighter{me, a}, me, UltOf("launha").Weapon, ang, pw, 0, nil)
	if a.Alive || len(res.Deaths) != 1 || res.Deaths[0] != "a" {
		t.Fatalf("đẩy ra ngoài sân phải chết: %+v %+v", a, res.Deaths)
	}
	res = Fire(flat(700), []*Fighter{me, boss}, me, UltOf("launha").Weapon, ang, pw, 0, nil)
	if boss.X != 1555 {
		t.Fatalf("boss không bị đẩy: %v", boss.X)
	}
}

func TestUltDrainMarksHits(t *testing.T) {
	g := flat(700)
	me, a := fighter("me", 300), fighter("a", 700)
	fs := []*Fighter{me, a}
	ang, pw := aimAt(t, g, fs, me, a)
	res := Fire(g, fs, me, UltOf("tv").Weapon, ang, pw, 0, nil)
	if len(res.Drained) != 1 || res.Drained[0] != "a" || len(res.Hits) == 0 || res.Hits[0].Dmg < 70 {
		t.Fatalf("sóng nhiễu: %+v %+v", res.Drained, res.Hits)
	}
	res = Fire(flat(700), fs, me, UltOf(ArmCannon).Weapon, ang, pw, 0, nil)
	if len(res.Drained) != 0 || len(res.Paths) != 3 {
		t.Fatalf("pháo kích 3 viên, không xoá nộ: %+v", res)
	}
}

// Nộ trong trận: +10 đầu lượt, + sát thương gây ra / 2, + sát thương nhận; chưa đủ thì không dùng chiêu cuối; bắn xong về 0.
func TestRageFlow(t *testing.T) {
	_, ss, cs := table(t, fast, 0, nil, binh)
	do(t, ss[0], Message{T: "start"})
	r := ss[0].Room
	flatten(r, 300, 700)
	st := cs[0].last()
	if st.RageMax != RageMax || len(st.Ults) != len(Arms) || st.Ults[0].Name != "Pháo kích" {
		t.Fatalf("state rageMax/ults: %d %+v", st.RageMax, st.Ults)
	}
	cur := turnOf(ss, st.Turn)
	other := ss[0]
	if other == cur {
		other = ss[1]
	}
	if cur.Player.rage != RagePerTurn || other.Player.rage != 0 {
		t.Fatalf("đầu lượt +10: %d %d", cur.Player.rage, other.Player.rage)
	}
	if err := r.Handle(cur.Player, Message{T: "fire", Angle: 60, Power: 50, Weapon: WeaponUlt}); !errors.Is(err, ErrNoRage) {
		t.Fatalf("chưa đủ nộ: %v", err)
	}
	r.mu.Lock()
	ang, pw := aimAt(t, r.ground, r.fighters(), cur.Player.f, other.Player.f)
	r.mu.Unlock()
	do(t, cur, Message{T: "fire", Angle: ang, Power: pw, Weapon: WeaponShell})
	shot := cs[0].lastOf("shot")
	hits := shot["hits"].([]Hit)
	if len(hits) == 0 {
		t.Fatal("phải trúng")
	}
	dealt := 0
	for _, h := range hits {
		if h.ID == other.Player.ID {
			dealt = h.Dmg
		}
	}
	rage := shot["rage"].(map[string]int)
	if rage[cur.Player.ID] != RagePerTurn+dealt/2 || rage[other.Player.ID] != dealt {
		t.Fatalf("nộ sau phát bắn (gây %d): %+v", dealt, rage)
	}
	for _, p := range cs[0].last().Players {
		if p.Rage != rage[p.ID] {
			t.Fatalf("state rage: %+v", p)
		}
	}

	// Đủ nộ: chiêu cuối hợp lệ, tin shot có ult / ultName; bắn xong về 0.
	waitFor(t, "sang lượt", func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.turn == other.Player.ID })
	r.mu.Lock()
	other.Player.rage = RageMax
	ang, pw = aimAt(t, r.ground, r.fighters(), other.Player.f, cur.Player.f)
	r.mu.Unlock()
	do(t, other, Message{T: "fire", Angle: ang, Power: pw, Weapon: WeaponUlt})
	shot = cs[0].lastOf("shot")
	if shot["weapon"] != WeaponUlt || shot["ult"] != ArmCannon || shot["ultName"] != "Pháo kích" || len(shot["paths"].([][][2]int)) != 3 {
		t.Fatalf("tin chiêu cuối: %v %v %v", shot["weapon"], shot["ult"], shot["ultName"])
	}
	if rage := shot["rage"].(map[string]int); rage[other.Player.ID] != 0 {
		t.Fatalf("bắn chiêu cuối xong nộ về 0: %+v", rage)
	}
}

// Sóng nhiễu trong trận: người trúng mất hết nộ.
func TestRageDrainTV(t *testing.T) {
	_, ss, cs := table(t, fast, 0, fakeGear{1: {Arm: "tv"}, 2: {Arm: "tv"}}, binh)
	do(t, ss[0], Message{T: "start"})
	r := ss[0].Room
	flatten(r, 300, 700)
	cur := turnOf(ss, cs[0].last().Turn)
	other := ss[0]
	if other == cur {
		other = ss[1]
	}
	r.mu.Lock()
	cur.Player.rage, other.Player.rage = RageMax, 70
	ang, pw := aimAt(t, r.ground, r.fighters(), cur.Player.f, other.Player.f)
	r.mu.Unlock()
	do(t, cur, Message{T: "fire", Angle: ang, Power: pw, Weapon: WeaponUlt})
	shot := cs[0].lastOf("shot")
	if shot["ultName"] != "Sóng nhiễu" {
		t.Fatalf("ult tv: %v", shot["ultName"])
	}
	if rage := shot["rage"].(map[string]int); rage[other.Player.ID] != 0 || rage[cur.Player.ID] != 0 {
		t.Fatalf("sóng nhiễu xoá nộ: %+v", rage)
	}
}

// Máy đủ nộ thì tung chiêu cuối.
func TestBotUsesUlt(t *testing.T) {
	_, ss, cs := table(t, Options{BotDelay: 200 * time.Millisecond, TurnTimeout: time.Hour, Grace: time.Hour}, 0, nil)
	do(t, ss[0], Message{T: "addBot", Level: BotHard})
	r := ss[0].Room
	r.mu.Lock()
	bot := r.players[1]
	r.mu.Unlock()
	do(t, ss[0], Message{T: "start"})
	flatten(r, 300, 700)
	r.mu.Lock()
	bot.rage = RageMax
	if r.turn == ss[0].Player.ID {
		r.passTurn(r.lastTurn)
	}
	r.mu.Unlock()
	waitFor(t, "máy bắn", func() bool { s := cs[0].lastOf("shot"); return s != nil && s["by"] == bot.ID })
	if s := cs[0].lastOf("shot"); s["weapon"] != WeaponUlt {
		t.Fatalf("máy đủ nộ phải dùng chiêu cuối: %v", s["weapon"])
	}
}
