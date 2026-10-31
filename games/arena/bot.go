package arena

import (
	"math"
	mrand "math/rand/v2"
)

const (
	BotEasy   = "easy"
	BotNormal = "normal"
	BotHard   = "hard"
)

var BotLevels = []string{BotEasy, BotNormal, BotHard}

// botGear là trang bị của máy theo mức.
var botGear = map[string]Gear{BotEasy: {}, BotNormal: {Gun: 3, Hat: 3, Armor: 3}, BotHard: {Gun: 6, Hat: 6, Armor: 6}}

// botNoise là sai số (lực, góc) của máy theo mức.
var botNoise = map[string][2]float64{BotEasy: {12, 8}, BotNormal: {6, 4}, BotHard: {2, 1}}

// Aim là phát bắn máy chọn.
type Aim struct {
	Angle  float64
	Power  float64
	Weapon string
}

// BotAim chọn mục tiêu (đối thủ gần nhất; máy khó chọn người ít máu nhất), thử mọi góc / lực bằng chính hàm mô phỏng
// của server để tìm phát rơi gần mục tiêu nhất, rồi cộng sai số theo mức.
func BotAim(g Ground, fs []*Fighter, me *Fighter, items map[string]int, level string, wind int, rng *mrand.Rand) Aim {
	var target *Fighter
	for _, f := range fs {
		if f == me || !f.Alive {
			continue
		}
		switch {
		case target == nil:
			target = f
		case level == BotHard && f.HP < target.HP:
			target = f
		case level != BotHard && math.Abs(f.X-me.X) < math.Abs(target.X-me.X):
			target = f
		}
	}
	if target == nil {
		return Aim{Angle: 60, Power: 50, Weapon: WeaponShell}
	}
	bodies := bodiesOf(fs)
	lo, hi := 15.0, 85.0
	if target.X < me.X {
		lo, hi = 95, 165
	}
	best, bestD := Aim{Angle: (lo + hi) / 2, Power: 60}, math.Inf(1)
	tx, ty := target.X, target.Y-BodyLift
	for a := lo; a <= hi; a += 2 {
		for p := 20.0; p <= 100; p += 2 {
			f := Simulate(g, me.X, me.Y, a, p, wind, me.ID, bodies)
			if !f.Hit {
				continue
			}
			d := math.Hypot(f.X-tx, f.Y-ty)
			if f.Body == target.ID {
				d = 0
			}
			if f.Body == me.ID || math.Hypot(f.X-me.X, f.Y-me.Y) < 70 {
				d += 400 // tránh tự nổ vào mình
			}
			if d < bestD {
				best, bestD = Aim{Angle: a, Power: p}, d
			}
		}
	}
	n := botNoise[level]
	best.Power = math.Max(5, math.Min(100, best.Power+(rng.Float64()*2-1)*n[0]))
	best.Angle = math.Max(0, math.Min(180, best.Angle+(rng.Float64()*2-1)*n[1]))
	best.Weapon = WeaponShell
	switch {
	case items[WeaponHeavy] > 0 && target.HP > 40 && level != BotEasy && rng.IntN(3) == 0:
		best.Weapon = WeaponHeavy
	case items[WeaponTriple] > 0 && rng.IntN(4) == 0:
		best.Weapon = WeaponTriple
	}
	return best
}
