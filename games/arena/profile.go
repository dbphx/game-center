package arena

// Hồ sơ nhân vật trong Đấu trường: xem trang bị đang dùng, chỉ số, lực chiến của một người chơi khác
// (GET /api/arena/players/{username}). Không lộ số dư, đá, bùa hay kho ngọc.

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strings"

	"github.com/dbphx/game-center/internal/wallet"
)

// ErrNoPlayer: không có người chơi với tên đăng nhập này.
var ErrNoPlayer = errors.New("không tìm thấy người chơi")

// ProfileItem là món đang dùng ở một ô (vũ khí / nón / áo).
type ProfileItem struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Level   int          `json:"level"`
	Form    int          `json:"form"` // dạng hình: 1, 2 (+7), 3 (+10)
	Atk     int          `json:"atk"`  // chỉ số gốc
	Def     int          `json:"def"`
	Luck    int          `json:"luck"`
	Sta     int          `json:"sta"`
	Eff     ArmStats     `json:"eff"` // đã nhân cấp và cộng ngọc khảm
	Sockets []SocketInfo `json:"sockets"`
}

// ProfileStats là chỉ số chiến đấu tổng (vũ khí + nón + áo + ngọc).
type ProfileStats struct {
	Atk      int `json:"atk"`
	Def      int `json:"def"`
	Luck     int `json:"luck"`
	Sta      int `json:"sta"`
	MaxHP    int `json:"maxHp"`
	DmgPct   int `json:"dmgPct"`
	ArmorPct int `json:"armorPct"`
	CritPct  int `json:"critPct"`
}

// Profile là hồ sơ Đấu trường của một người chơi.
type Profile struct {
	Username string       `json:"username"`
	Name     string       `json:"name"`
	Gender   string       `json:"gender"`
	Outfit   string       `json:"outfit"`
	Hair     string       `json:"hair"`
	Wings    string       `json:"wings"`
	Face     string       `json:"face"`
	Level    int          `json:"level"`
	Gear     Gear         `json:"gear"`
	Power    int          `json:"power"`
	Stats    ProfileStats `json:"stats"`
	Weapon   ProfileItem  `json:"weapon"`
	Hat      ProfileItem  `json:"hat"`
	Armor    ProfileItem  `json:"armor"`
}

// Form là dạng hình của món cấp lv: +10 → 3, +7 → 2, còn lại 1.
func Form(lv int) int {
	switch {
	case lv >= 10:
		return 3
	case lv >= 7:
		return 2
	}
	return 1
}

func profileItem(inv Inventory, slot string) ProfileItem {
	a := inv.Gear.item(slot)
	lv := inv.Gear.level(slot)
	return ProfileItem{ID: a.ID, Name: a.Name, Level: lv, Form: Form(lv), Atk: a.Atk, Def: a.Def, Luck: a.Luck, Sta: a.Sta,
		Eff: a.Eff(lv).plus(gemBonus(inv.Sockets[a.ID], lv)), Sockets: socketInfos(inv.Sockets[a.ID], lv)}
}

// Profile đọc hồ sơ Đấu trường của người có tên đăng nhập username.
func (a *Armory) Profile(ctx context.Context, username string) (Profile, error) {
	name := wallet.NormalizeUsername(username)
	if name == "" {
		return Profile{}, ErrNoPlayer
	}
	tx, err := a.w.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer tx.Rollback()
	var uid int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, name).Scan(&uid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Profile{}, ErrNoPlayer
		}
		return Profile{}, err
	}
	inv, err := loadInventory(ctx, tx, uid)
	if err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(); err != nil {
		return Profile{}, err
	}
	u, err := a.w.User(ctx, uid)
	if err != nil {
		return Profile{}, err
	}
	g := inv.Gear
	st := g.Stats()
	return Profile{Username: u.Username, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings,
		Face: u.Face, Level: u.Level, Gear: g.Norm(), Power: g.Power(),
		Stats: ProfileStats{Atk: st.Atk, Def: st.Def, Luck: st.Luck, Sta: st.Sta, MaxHP: g.MaxHP(),
			DmgPct: int(math.Round(float64(st.Atk) / 10)), ArmorPct: int(math.Round(100 * g.ArmorPct())), CritPct: g.CritPct()},
		Weapon: profileItem(inv, SlotGun), Hat: profileItem(inv, SlotHat), Armor: profileItem(inv, SlotArmor)}, nil
}

// profileHandler: GET /api/arena/players/{username} (cần đăng nhập).
func (a *Armory) profileHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := a.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")); err != nil {
		writeErr(w, err)
		return
	}
	p, err := a.Profile(r.Context(), r.PathValue("username"))
	if errors.Is(err, ErrNoPlayer) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
