package arena

// Nộ và chiêu cuối: mỗi người trong trận có nộ 0..RageMax; đầy nộ thì bắn được chiêu cuối của vũ khí đang trang bị
// (weapon "ult"), bắn xong nộ về 0. Boss không có nộ.

import "errors"

const (
	RageMax     = 100
	RagePerTurn = 10 // đầu mỗi lượt của mình
	WeaponUlt   = "ult"
)

// Ult là chiêu cuối của một vũ khí; sát thương vẫn nhân Tấn công / chí mạng / Phòng thủ như đạn thường.
type Ult struct {
	Arm  string // id vũ khí
	Desc string
	Weapon
}

var Ults = []Ult{
	{ArmCannon, "3 quả pháo nổ rộng", Weapon{ID: WeaponUlt, Name: "Pháo kích", Dmg: 45, R: 60, Spread: []float64{-4, 0, 4}, Uses: -1}},
	{"dep", "5 chiếc dép rơi như mưa", Weapon{ID: WeaponUlt, Name: "Mưa dép", Dmg: 22, R: 30, Spread: []float64{-10, -5, 0, 5, 10}, Uses: -1}},
	{"choi", "Một nhát quét cực rộng", Weapon{ID: WeaponUlt, Name: "Quét sạch", Dmg: 60, R: 110, Spread: []float64{0}, Uses: -1}},
	{"launha", "Đẩy văng mọi đối thủ quanh chỗ nổ ra xa", Weapon{ID: WeaponUlt, Name: "Lau sàn", Dmg: 50, R: 70, Spread: []float64{0}, Uses: -1, Push: 80}},
	{"tv", "Đối thủ trúng đòn mất hết nộ", Weapon{ID: WeaponUlt, Name: "Sóng nhiễu", Dmg: 80, R: 60, Spread: []float64{0}, Uses: -1, Drain: true}},
	{"tulanh", "Cú rơi nặng nhất", Weapon{ID: WeaponUlt, Name: "Tủ lạnh rơi", Dmg: 110, R: 90, Spread: []float64{0}, Uses: -1}},
}

// UltOf là chiêu cuối của vũ khí arm (lạ thì của đại bác).
func UltOf(arm string) Ult {
	for _, u := range Ults {
		if u.Arm == arm {
			return u
		}
	}
	return Ults[0]
}

// UltInfo là chiêu cuối gửi cho giao diện.
type UltInfo struct {
	Arm  string `json:"arm"`
	ID   string `json:"id"`
	Name string `json:"name"`
	Desc string `json:"desc"`
	Dmg  int    `json:"dmg"`
	R    int    `json:"r"`
	N    int    `json:"n"` // số viên
}

func ultList() []UltInfo {
	out := make([]UltInfo, 0, len(Ults))
	for _, u := range Ults {
		out = append(out, UltInfo{Arm: u.Arm, ID: u.ID, Name: u.Name, Desc: u.Desc, Dmg: int(u.Dmg), R: int(u.R), N: len(u.Spread)})
	}
	return out
}

var ErrNoRage = errors.New("chưa đủ nộ để dùng chiêu cuối")

// weaponFor là đạn p dùng cho lệnh bắn id ("" = đạn thường, "ult" = chiêu cuối khi đủ nộ).
func weaponFor(p *Player, id string) (Weapon, error) {
	switch id {
	case "":
		id = WeaponShell
	case WeaponUlt:
		if p.rage < RageMax {
			return Weapon{}, ErrNoRage
		}
		return UltOf(p.f.Gear.ArmID()).Weapon, nil
	}
	w, ok := findWeapon(id)
	if !ok {
		return Weapon{}, errors.New("vũ khí không hợp lệ")
	}
	return w, nil
}

// applyRage cộng nộ sau một phát bắn của shooter (nil = boss): người bắn + sát thương gây cho người khác / 2,
// người trúng + sát thương nhận; chiêu cuối thì người bắn về 0; Sóng nhiễu xoá nộ người trúng. Trả nộ mọi người sau phát bắn.
func (r *Room) applyRage(shooter *Player, w Weapon, res ShotResult) map[string]int {
	dealt := 0
	for _, h := range res.Hits {
		if shooter == nil || h.ID != shooter.ID {
			dealt += h.Dmg
		}
		if p := r.findPlayer(h.ID); p != nil {
			p.rage += h.Dmg
		}
	}
	if shooter != nil {
		if w.ID == WeaponUlt {
			shooter.rage = 0
		} else {
			shooter.rage += dealt / 2
		}
	}
	for _, id := range res.Drained {
		if p := r.findPlayer(id); p != nil {
			p.rage = 0
		}
	}
	out := map[string]int{}
	for _, p := range r.players {
		p.rage = min(RageMax, max(0, p.rage))
		out[p.ID] = p.rage
	}
	return out
}
