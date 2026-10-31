package arena

import "math"

// Weapon là một loại đạn trong trận.
type Weapon struct {
	ID     string
	Name   string
	Dmg    float64   // sát thương mỗi viên ở tâm nổ
	R      float64   // bán kính nổ / hố
	Spread []float64 // độ lệch góc từng viên (một viên: {0})
	Uses   int       // số lần mỗi trận, −1 = vô hạn
	Tele   bool      // đạn dịch chuyển: người bắn tới chỗ đạn rơi, không nổ
}

const (
	WeaponShell  = "shell"
	WeaponTriple = "triple"
	WeaponHeavy  = "heavy"
	WeaponTele   = "tele"
)

var Weapons = []Weapon{
	{ID: WeaponShell, Name: "Đạn thường", Dmg: 30, R: 40, Spread: []float64{0}, Uses: -1},
	{ID: WeaponTriple, Name: "Đạn chùm", Dmg: 14, R: 28, Spread: []float64{-6, 0, 6}, Uses: 2},
	{ID: WeaponHeavy, Name: "Bom tạ", Dmg: 55, R: 65, Spread: []float64{0}, Uses: 1},
	{ID: WeaponTele, Name: "Đạn dịch chuyển", Spread: []float64{0}, Uses: 1, Tele: true},
}

func findWeapon(id string) (Weapon, bool) {
	for _, w := range Weapons {
		if w.ID == id {
			return w, true
		}
	}
	return Weapon{}, false
}

// startItems là số lần dùng vũ khí giới hạn đầu mỗi trận.
func startItems() map[string]int {
	out := map[string]int{}
	for _, w := range Weapons {
		if w.Uses > 0 {
			out[w.ID] = w.Uses
		}
	}
	return out
}

// Fighter là một người trên sân trong trận (chỉ số chụp lúc bắt đầu trận).
type Fighter struct {
	ID    string
	X, Y  float64
	HP    int
	MaxHP int
	Alive bool
	Gear  Gear
}

type Blast struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	R    int `json:"r"`
	At   int `json:"at"`   // chỉ số điểm trong quỹ đạo lúc nổ
	Path int `json:"path"` // quỹ đạo nào
}

type Hit struct {
	ID  string `json:"id"`
	Dmg int    `json:"dmg"`
	HP  int    `json:"hp"`
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// ShotResult là kết quả một phát bắn đã áp lên sân.
type ShotResult struct {
	Paths    [][][2]int
	Blasts   []Blast
	Hits     []Hit
	Deaths   []string
	Tele     *Point
	Duration float64 // giây bay của viên lâu nhất
}

func bodiesOf(fs []*Fighter) []Body {
	var out []Body
	for _, f := range fs {
		if f.Alive {
			out = append(out, Body{ID: f.ID, X: f.X, Y: f.Y})
		}
	}
	return out
}

// Fire bắn một phát của shooter bằng vũ khí w; sửa địa hình g và máu / vị trí các fighter ngay tại chỗ.
func Fire(g Ground, fs []*Fighter, shooter *Fighter, w Weapon, angle, power float64, wind int) ShotResult {
	res := ShotResult{}
	bodies := bodiesOf(fs)
	var flights []Flight
	for _, d := range w.Spread {
		a := math.Max(0, math.Min(180, angle+d))
		f := Simulate(g, shooter.X, shooter.Y, a, power, wind, shooter.ID, bodies)
		flights = append(flights, f)
		res.Paths = append(res.Paths, f.Path)
		res.Duration = math.Max(res.Duration, f.Time)
	}
	if w.Tele {
		f := flights[0]
		if f.Hit && f.X >= MinX && f.X <= MaxX && g.At(f.X) < AbyssY {
			shooter.X = math.Round(f.X)
			shooter.Y = float64(g.At(shooter.X))
			res.Tele = &Point{int(shooter.X), int(shooter.Y)}
		}
		return res
	}
	dmg := map[string]int{}
	for i, f := range flights {
		if !f.Hit {
			continue
		}
		res.Blasts = append(res.Blasts, Blast{X: int(math.Round(f.X)), Y: int(math.Round(f.Y)), R: int(w.R), At: len(f.Path) - 1, Path: i})
		Crater(g, f.X, f.Y, w.R)
		for _, t := range fs {
			if !t.Alive {
				continue
			}
			cx, cy := t.X, t.Y-BodyLift
			d := math.Hypot(f.X-cx, f.Y-cy)
			if t.ID == f.Body {
				d = 0 // trúng trực tiếp
			}
			if n := Splash(w.Dmg, w.R, d, shooter.Gear.DmgMul(), t.Gear.ArmorPct()); n > 0 {
				dmg[t.ID] += n
			}
		}
	}
	for _, t := range fs {
		if !t.Alive {
			continue
		}
		if n := dmg[t.ID]; n > 0 {
			t.HP = max(0, t.HP-n)
			res.Hits = append(res.Hits, Hit{ID: t.ID, Dmg: n, HP: t.HP})
		}
		t.Y = float64(g.At(t.X)) // rơi xuống mặt đất mới
		if t.HP <= 0 || t.Y >= AbyssY {
			t.Alive, t.HP = false, 0
			res.Deaths = append(res.Deaths, t.ID)
		}
	}
	return res
}
