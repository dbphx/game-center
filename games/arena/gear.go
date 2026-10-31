package arena

// Trang bị vĩnh viễn của Đấu trường: Vũ khí (nhiều loại mua bằng xu, mỗi loại cấp 0..10 riêng, trang bị một), Nón,
// Áo giáp (cấp 0..10), cường hoá bằng đá (5 cấp, mua bằng xu) có tỉ lệ thành công; chuyển cấp giữa hai vũ khí; lưu trong database.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"strconv"
	"strings"

	"github.com/dbphx/game-center/internal/wallet"
)

const (
	SlotGun   = "gun"
	SlotHat   = "hat"
	SlotArmor = "armor"
	MaxLevel  = 10
)

var Slots = []string{SlotGun, SlotHat, SlotArmor}

var slotNames = map[string]string{SlotGun: "Súng", SlotHat: "Nón", SlotArmor: "Áo giáp"}

// Arm là một loại vũ khí; 4 chỉ số gốc (ở +0): tấn công, phòng thủ, may mắn, thể lực.
type Arm struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
	Desc  string `json:"desc"`
	Atk   int    `json:"atk"`
	Def   int    `json:"def"`
	Luck  int    `json:"luck"`
	Sta   int    `json:"sta"`
}

const ArmCannon = "cannon"

var Arms = []Arm{
	{ArmCannon, "Đại bác", 0, "Mặc định, ai cũng có", 10, 5, 2, 10},
	{"dep", "Dép lào", 1500, "Nhẹ, ném đâu trúng đó", 15, 8, 4, 15},
	{"choi", "Chổi chà", 3000, "Quét sạch đối thủ", 22, 12, 6, 22},
	{"launha", "Cây lau nhà", 6000, "Lau sân, lau luôn đối thủ", 30, 16, 8, 30},
	{"tv", "TV", 12000, "Nặng, nổ to như phim hành động", 40, 24, 10, 40},
	{"tulanh", "Tủ lạnh", 25000, "Vũ khí tối thượng của mọi nhà", 55, 32, 12, 60},
}

func findArm(id string) (Arm, bool) {
	for _, a := range Arms {
		if a.ID == id {
			return a, true
		}
	}
	return Arm{}, false
}

// ArmStats là 4 chỉ số của vũ khí (đã nhân cấp cường hoá).
type ArmStats struct {
	Atk  int `json:"atk"`
	Def  int `json:"def"`
	Luck int `json:"luck"`
	Sta  int `json:"sta"`
}

// Eff là chỉ số của vũ khí ở cấp lv: gốc × (1 + 0.1·lv), làm tròn.
func (a Arm) Eff(lv int) ArmStats {
	f := func(v int) int { return int(math.Round(float64(v) * (1 + 0.1*float64(lv)))) }
	return ArmStats{f(a.Atk), f(a.Def), f(a.Luck), f(a.Sta)}
}

// Gear là trang bị đang dùng: vũ khí Arm ở cấp Gun, nón, áo giáp.
type Gear struct {
	Arm   string   `json:"arm"` // id vũ khí đang trang bị ("" = đại bác)
	Gun   int      `json:"gun"` // cấp của vũ khí đang trang bị
	Hat   int      `json:"hat"`
	Armor int      `json:"armor"`
	Bonus ArmStats `json:"bonus"` // chỉ số cộng từ ngọc khảm ở các ô đã mở của vũ khí đang trang bị
}

// Weapon là vũ khí đang trang bị (mặc định đại bác).
func (g Gear) Weapon() Arm {
	if a, ok := findArm(g.Arm); ok {
		return a
	}
	return Arms[0]
}

// ArmID là id vũ khí đang trang bị.
func (g Gear) ArmID() string { return g.Weapon().ID }

// Stats là chỉ số vũ khí đang trang bị ở cấp hiện tại, cộng ngọc khảm.
func (g Gear) Stats() ArmStats {
	s := g.Weapon().Eff(g.Gun)
	s.Atk, s.Def, s.Luck, s.Sta = s.Atk+g.Bonus.Atk, s.Def+g.Bonus.Def, s.Luck+g.Bonus.Luck, s.Sta+g.Bonus.Sta
	return s
}

func (g Gear) level(slot string) int {
	switch slot {
	case SlotGun:
		return g.Gun
	case SlotHat:
		return g.Hat
	case SlotArmor:
		return g.Armor
	}
	return 0
}

func (g *Gear) set(slot string, lv int) {
	switch slot {
	case SlotGun:
		g.Gun = lv
	case SlotHat:
		g.Hat = lv
	case SlotArmor:
		g.Armor = lv
	}
}

// Power là lực chiến.
func (g Gear) Power() int {
	s := g.Stats()
	return 1000 + 2*(s.Atk+s.Def+s.Luck+s.Sta) + 80*g.Hat + 100*g.Armor
}

// MaxHP là máu tối đa trong trận: nón + thể lực vũ khí.
func (g Gear) MaxHP() int { return 200 + 15*g.Hat + g.Stats().Sta }

// DmgMul là hệ số sát thương: 1 + tấn công / 100.
func (g Gear) DmgMul() float64 { return 1 + float64(g.Stats().Atk)/100 }

// MaxArmorPct là tỉ lệ giảm sát thương tối đa.
const MaxArmorPct = 0.6

// ArmorPct là tỉ lệ giảm sát thương nhận: áo giáp 3%/cấp + phòng thủ vũ khí / 100, tối đa 0.6.
func (g Gear) ArmorPct() float64 {
	return math.Min(MaxArmorPct, 0.03*float64(g.Armor)+float64(g.Stats().Def)/100)
}

// CritPct là % chí mạng (×CritMul sát thương) = may mắn vũ khí.
func (g Gear) CritPct() int { return g.Stats().Luck }

const CritMul = 1.5

// TransferFeePerLevel là phí xu chuyển cấp, nhân số cấp chuyển.
const TransferFeePerLevel = 300

// TransferFee là phí chuyển cấp lv từ vũ khí này sang vũ khí khác.
func TransferFee(lv int) int64 { return TransferFeePerLevel * int64(lv) }

// Stone là một cấp đá cường hoá.
type Stone struct {
	Tier  int    `json:"tier"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

var Stones = []Stone{
	{1, "Đá Lục", 100}, {2, "Đá Lam", 300}, {3, "Đá Tím", 800}, {4, "Đá Vàng", 2000}, {5, "Đá Đỏ", 5000},
}

const MaxBuy = 99

// Charm là một loại bùa dùng kèm khi cường hoá.
type Charm struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
	Desc  string `json:"desc"`
}

const (
	CharmLuck    = "luck"
	CharmProtect = "protect"
	LuckBonus    = 15 // bùa may mắn cộng 15% tỉ lệ
)

var Charms = []Charm{
	{CharmLuck, "Bùa may mắn", 1500, "+15% tỉ lệ"},
	{CharmProtect, "Bùa giữ cấp", 2000, "Thất bại không tụt cấp"},
}

func findCharm(id string) (Charm, bool) {
	for _, c := range Charms {
		if c.ID == id {
			return c, true
		}
	}
	return Charm{}, false
}

func findStone(tier int) (Stone, bool) {
	if tier < 1 || tier > len(Stones) {
		return Stone{}, false
	}
	return Stones[tier-1], true
}

// Rate là tỉ lệ (%) cường hoá từ cấp lv lên lv+1 bằng đá tier.
func Rate(lv, tier int) int { return min(100, max(5, 95-10*lv+12*(tier-1))) }

// Fee là phí xu mỗi lần cường hoá từ cấp lv (trừ cả khi thất bại).
func Fee(lv int) int64 { return 50 * int64(lv+1) }

// GearStats là chỉ số hiện cho người chơi.
type GearStats struct {
	DmgPct   int `json:"dmgPct"`
	MaxHP    int `json:"maxHp"`
	ArmorPct int `json:"armorPct"`
	Atk      int `json:"atk"`
	Def      int `json:"def"`
	Luck     int `json:"luck"`
	Sta      int `json:"sta"`
}

// ArmInfo là một vũ khí trong GearReply.
type ArmInfo struct {
	Arm
	Eff      ArmStats     `json:"eff"` // đã nhân cấp và cộng ngọc khảm
	Owned    bool         `json:"owned"`
	Level    int          `json:"level"`
	Equipped bool         `json:"equipped"`
	Sockets  []SocketInfo `json:"sockets"`
}

// Transfer là kết quả một lần chuyển cấp.
type Transfer struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Level int    `json:"level"`
}

// Outcome là kết quả một lần cường hoá.
type Outcome struct {
	OK        bool   `json:"ok"`
	Slot      string `json:"slot"`
	Arm       string `json:"arm,omitempty"` // vũ khí được cường hoá (ô gun)
	Level     int    `json:"level"`         // cấp sau lần cường hoá
	From      int    `json:"from"`          // cấp trước lần cường hoá
	Rate      int    `json:"rate"`          // tỉ lệ đã cộng bùa may mắn
	Dropped   bool   `json:"dropped"`
	Protected bool   `json:"protected"` // thất bại nhưng giữ cấp nhờ bùa
	Evolved   bool   `json:"evolved"`   // vừa lên +7 hoặc +10: trang bị đổi hình (bậc mới)
}

// GearReply là trả lời các API trang bị: GET /api/arena/gear, mua đá, cường hoá.
type GearReply struct {
	Gear      Gear             `json:"gear"`
	Power     int              `json:"power"`
	Stats     GearStats        `json:"stats"`
	MaxLevel  int              `json:"maxLevel"`
	Balance   int64            `json:"balance"`
	Stones    map[string]int   `json:"stones"` // số viên theo tier "1".."5"
	StoneShop []Stone          `json:"stoneShop"`
	Fee       map[string]int64 `json:"fee"`   // phí lần cường hoá kế tiếp; 0 khi đã max
	Rates     map[string][]int `json:"rates"` // tỉ lệ % gốc theo tier 1..5 cho lần kế tiếp; [] khi đã max
	Charms    map[string]int   `json:"charms"`
	CharmShop []Charm          `json:"charmShop"`
	LuckBonus int              `json:"luckBonus"`
	Arms      []ArmInfo        `json:"arms"`
	Gems      []GemInfo        `json:"gems"` // ngọc trong kho (chỉ loại còn ≥ 1 viên)
	// TransferFeePerLevel: phí chuyển cấp = giá trị này × cấp của vũ khí nguồn.
	TransferFeePerLevel int64     `json:"transferFeePerLevel"`
	Result              *Outcome  `json:"result,omitempty"`
	Transfer            *Transfer `json:"transfer,omitempty"`
}

// Inventory là trang bị + đá của một người.
type Inventory struct {
	Gear    Gear
	Arms    map[string]int // vũ khí đã sở hữu → cấp (luôn có đại bác)
	Gems    map[GemKey]int
	Sockets map[string][SocketCount]GemKey
	Stones  [6]int // chỉ số 1..5
	Charms  map[string]int
}

func reply(inv Inventory, balance int64) GearReply {
	g := inv.Gear
	r := GearReply{Gear: g, Power: g.Power(), MaxLevel: MaxLevel, Balance: balance, Stones: map[string]int{},
		StoneShop: Stones, Fee: map[string]int64{}, Rates: map[string][]int{}, Charms: map[string]int{},
		CharmShop: Charms, LuckBonus: LuckBonus, TransferFeePerLevel: TransferFeePerLevel}
	st := g.Stats()
	r.Stats = GearStats{DmgPct: st.Atk, MaxHP: g.MaxHP(), ArmorPct: int(math.Round(100 * g.ArmorPct())),
		Atk: st.Atk, Def: st.Def, Luck: st.Luck, Sta: st.Sta}
	for _, a := range Arms {
		lv, owned := inv.Arms[a.ID]
		if a.ID == ArmCannon {
			owned = true
		}
		eff := a.Eff(lv)
		b := gemBonus(inv.Sockets[a.ID], lv)
		eff.Atk, eff.Def, eff.Luck, eff.Sta = eff.Atk+b.Atk, eff.Def+b.Def, eff.Luck+b.Luck, eff.Sta+b.Sta
		r.Arms = append(r.Arms, ArmInfo{Arm: a, Eff: eff, Owned: owned, Level: lv, Equipped: a.ID == g.ArmID(),
			Sockets: socketInfos(inv.Sockets[a.ID], lv)})
	}
	r.Gems = []GemInfo{}
	for _, kind := range GemKinds {
		for t := 1; t <= GemTiers; t++ {
			k := GemKey{kind, t}
			if n := inv.Gems[k]; n > 0 {
				r.Gems = append(r.Gems, gemInfo(k, n))
			}
		}
	}
	for _, st := range Stones {
		r.Stones[strconv.Itoa(st.Tier)] = inv.Stones[st.Tier]
	}
	for _, c := range Charms {
		r.Charms[c.ID] = inv.Charms[c.ID]
	}
	for _, s := range Slots {
		lv := g.level(s)
		rates := []int{}
		if lv < MaxLevel {
			r.Fee[s] = Fee(lv)
			for _, st := range Stones {
				rates = append(rates, Rate(lv, st.Tier))
			}
		} else {
			r.Fee[s] = 0
		}
		r.Rates[s] = rates
	}
	return r
}

const schema = `
CREATE TABLE IF NOT EXISTS arena_gear (
	user_id INTEGER NOT NULL REFERENCES users(id),
	slot    TEXT NOT NULL,
	level   INTEGER NOT NULL,
	PRIMARY KEY (user_id, slot)
);
CREATE TABLE IF NOT EXISTS arena_charms (
	user_id INTEGER NOT NULL REFERENCES users(id),
	kind    TEXT NOT NULL,
	qty     INTEGER NOT NULL,
	PRIMARY KEY (user_id, kind)
);
CREATE TABLE IF NOT EXISTS arena_arms (
	user_id  INTEGER NOT NULL REFERENCES users(id),
	arm      TEXT NOT NULL,
	level    INTEGER NOT NULL,
	equipped INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, arm)
);
CREATE TABLE IF NOT EXISTS arena_stones (
	user_id INTEGER NOT NULL REFERENCES users(id),
	tier    INTEGER NOT NULL,
	qty     INTEGER NOT NULL,
	PRIMARY KEY (user_id, tier)
);
`

var (
	ErrBadSlot  = errors.New("ô trang bị không hợp lệ")
	ErrBadStone = errors.New("loại đá không hợp lệ")
	ErrBadQty   = fmt.Errorf("số lượng phải từ 1 đến %d", MaxBuy)
	ErrMaxed    = errors.New("trang bị đã đạt cấp tối đa")
	ErrNoStone  = errors.New("bạn hết loại đá này, hãy mua thêm")
	ErrBadCharm = errors.New("loại bùa không hợp lệ")
	ErrNoCharm  = errors.New("bạn hết bùa đã chọn, hãy mua thêm")
	ErrBadArm   = errors.New("vũ khí không hợp lệ")
	ErrOwned    = errors.New("bạn đã có vũ khí này")
	ErrNotOwned = errors.New("bạn chưa có vũ khí này, hãy mua trước")
	ErrSameArm  = errors.New("hãy chọn hai vũ khí khác nhau")
	ErrNoLevel  = errors.New("vũ khí nguồn chưa cường hoá, không có cấp để chuyển")
	ErrLowLevel = errors.New("vũ khí nguồn phải có cấp cao hơn vũ khí nhận")
)

// Armory là kho trang bị và đá cường hoá của mọi người chơi.
type Armory struct {
	w *wallet.Wallet
	// roll trả số ngẫu nhiên 0..99 cho mỗi lần cường hoá (test thay để có kết quả cố định).
	roll func() int
}

// New tạo bảng trang bị và trả kho.
func New(ctx context.Context, w *wallet.Wallet) (*Armory, error) {
	if err := w.ExecSchema(ctx, schema+gemSchema); err != nil {
		return nil, fmt.Errorf("tạo bảng đấu trường: %w", err)
	}
	// Chuyển cấp ô "Súng" cũ (arena_gear slot gun) thành cấp đại bác đang trang bị — chỉ lần đầu (đã có hàng thì bỏ qua).
	tx, err := w.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO arena_arms (user_id, arm, level, equipped)
		SELECT user_id, ?, level, 1 FROM arena_gear WHERE slot = ? ON CONFLICT (user_id, arm) DO NOTHING`, ArmCannon, SlotGun); err != nil {
		return nil, fmt.Errorf("chuyển cấp súng cũ: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Armory{w: w, roll: func() int { return mrand.IntN(100) }}, nil
}

// Gear đọc trang bị của người chơi (chưa cường hoá ô nào thì cấp 0).
func (a *Armory) Gear(ctx context.Context, uid int64) (Gear, error) {
	inv, err := a.Inventory(ctx, uid)
	return inv.Gear, err
}

// Inventory đọc trang bị và đá của người chơi.
func (a *Armory) Inventory(ctx context.Context, uid int64) (Inventory, error) {
	tx, err := a.w.Begin(ctx)
	if err != nil {
		return Inventory{}, err
	}
	defer tx.Rollback()
	inv, err := loadInventory(ctx, tx, uid)
	if err != nil {
		return Inventory{}, err
	}
	return inv, tx.Commit()
}

func loadInventory(ctx context.Context, tx *wallet.Tx, uid int64) (Inventory, error) {
	inv := Inventory{Charms: map[string]int{}, Arms: map[string]int{ArmCannon: 0}}
	rows, err := tx.QueryContext(ctx, `SELECT slot, level FROM arena_gear WHERE user_id = ?`, uid)
	if err != nil {
		return inv, err
	}
	for rows.Next() {
		var s string
		var lv int
		if err := rows.Scan(&s, &lv); err != nil {
			rows.Close()
			return inv, err
		}
		if s != SlotGun { // cấp vũ khí nằm ở arena_arms
			inv.Gear.set(s, min(max(lv, 0), MaxLevel))
		}
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT arm, level, equipped FROM arena_arms WHERE user_id = ?`, uid)
	if err != nil {
		return inv, err
	}
	equipped := ArmCannon
	for rows.Next() {
		var id string
		var lv, eq int
		if err := rows.Scan(&id, &lv, &eq); err != nil {
			rows.Close()
			return inv, err
		}
		if _, ok := findArm(id); ok {
			inv.Arms[id] = min(max(lv, 0), MaxLevel)
			if eq != 0 {
				equipped = id
			}
		}
	}
	rows.Close()
	inv.Gear.Arm, inv.Gear.Gun = equipped, inv.Arms[equipped]
	if err := loadGems(ctx, tx, uid, &inv); err != nil {
		return inv, err
	}
	rows, err = tx.QueryContext(ctx, `SELECT kind, qty FROM arena_charms WHERE user_id = ?`, uid)
	if err != nil {
		return inv, err
	}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			rows.Close()
			return inv, err
		}
		if _, ok := findCharm(k); ok {
			inv.Charms[k] = max(0, n)
		}
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT tier, qty FROM arena_stones WHERE user_id = ?`, uid)
	if err != nil {
		return inv, err
	}
	defer rows.Close()
	for rows.Next() {
		var t, n int
		if err := rows.Scan(&t, &n); err != nil {
			return inv, err
		}
		if t >= 1 && t <= len(Stones) {
			inv.Stones[t] = max(0, n)
		}
	}
	return inv, rows.Err()
}

func addStones(ctx context.Context, tx *wallet.Tx, uid int64, tier, n int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO arena_stones (user_id, tier, qty) VALUES (?, ?, ?)
		ON CONFLICT (user_id, tier) DO UPDATE SET qty = qty + excluded.qty`, uid, tier, n)
	return err
}

// setArm ghi cấp vũ khí arm (tạo hàng nếu chưa có; equipped chỉ dùng khi tạo mới).
func setArm(ctx context.Context, tx *wallet.Tx, uid int64, arm string, lv int, equipped bool) error {
	eq := 0
	if equipped {
		eq = 1
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO arena_arms (user_id, arm, level, equipped) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, arm) DO UPDATE SET level = excluded.level`, uid, arm, lv, eq)
	return err
}

// withInv chạy op trong một giao dịch trên kho đã đọc, rồi đọc lại kho.
func (a *Armory) withInv(ctx context.Context, uid int64, op func(tx *wallet.Tx, inv Inventory) error) (Inventory, error) {
	tx, err := a.w.Begin(ctx)
	if err != nil {
		return Inventory{}, err
	}
	defer tx.Rollback()
	inv, err := loadInventory(ctx, tx, uid)
	if err != nil {
		return Inventory{}, err
	}
	if err := op(tx, inv); err != nil {
		return Inventory{}, err
	}
	if inv, err = loadInventory(ctx, tx, uid); err != nil {
		return Inventory{}, err
	}
	return inv, tx.Commit()
}

// BuyArm mua vũ khí id bằng xu (cấp 0, chưa trang bị).
func (a *Armory) BuyArm(ctx context.Context, uid int64, id string) (Inventory, error) {
	arm, ok := findArm(id)
	if !ok {
		return Inventory{}, ErrBadArm
	}
	return a.withInv(ctx, uid, func(tx *wallet.Tx, inv Inventory) error {
		if _, have := inv.Arms[id]; have {
			return ErrOwned
		}
		if err := tx.Charge(uid, arm.Price, GameID, "Đấu trường: mua "+arm.Name); err != nil {
			return err
		}
		return setArm(ctx, tx, uid, id, 0, false)
	})
}

// EquipArm trang bị vũ khí id đã sở hữu.
func (a *Armory) EquipArm(ctx context.Context, uid int64, id string) (Inventory, error) {
	if _, ok := findArm(id); !ok {
		return Inventory{}, ErrBadArm
	}
	return a.withInv(ctx, uid, func(tx *wallet.Tx, inv Inventory) error {
		lv, have := inv.Arms[id]
		if !have {
			return ErrNotOwned
		}
		if _, err := tx.ExecContext(ctx, `UPDATE arena_arms SET equipped = 0 WHERE user_id = ?`, uid); err != nil {
			return err
		}
		if err := setArm(ctx, tx, uid, id, lv, true); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE arena_arms SET equipped = 1 WHERE user_id = ? AND arm = ?`, uid, id)
		return err
	})
}

// TransferArm chuyển cấp cường hoá từ vũ khí from sang vũ khí to: to nhận cấp của from, from về +0; phí TransferFee.
func (a *Armory) TransferArm(ctx context.Context, uid int64, from, to string) (Inventory, Transfer, error) {
	fa, ok1 := findArm(from)
	ta, ok2 := findArm(to)
	if !ok1 || !ok2 {
		return Inventory{}, Transfer{}, ErrBadArm
	}
	var t Transfer
	inv, err := a.withInv(ctx, uid, func(tx *wallet.Tx, inv Inventory) error {
		lf, haveF := inv.Arms[from]
		lt, haveT := inv.Arms[to]
		switch {
		case from == to:
			return ErrSameArm
		case !haveF || !haveT:
			return ErrNotOwned
		case lf <= 0:
			return ErrNoLevel
		case lf <= lt:
			return ErrLowLevel
		}
		note := fmt.Sprintf("Đấu trường: chuyển cấp +%d từ %s sang %s", lf, fa.Name, ta.Name)
		if err := tx.Charge(uid, TransferFee(lf), GameID, note); err != nil {
			return err
		}
		if err := setArm(ctx, tx, uid, to, lf, to == inv.Gear.Arm); err != nil {
			return err
		}
		if err := setArm(ctx, tx, uid, from, 0, from == inv.Gear.Arm); err != nil {
			return err
		}
		if err := lockSockets(ctx, tx, uid, inv, from, 0); err != nil {
			return err
		}
		t = Transfer{From: from, To: to, Level: lf}
		return nil
	})
	return inv, t, err
}

func addCharms(ctx context.Context, tx *wallet.Tx, uid int64, kind string, n int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO arena_charms (user_id, kind, qty) VALUES (?, ?, ?)
		ON CONFLICT (user_id, kind) DO UPDATE SET qty = qty + excluded.qty`, uid, kind, n)
	return err
}

// BuyCharms mua qty lá bùa loại id bằng xu.
func (a *Armory) BuyCharms(ctx context.Context, uid int64, id string, qty int) (Inventory, error) {
	c, ok := findCharm(id)
	if !ok {
		return Inventory{}, ErrBadCharm
	}
	if qty < 1 || qty > MaxBuy {
		return Inventory{}, ErrBadQty
	}
	tx, err := a.w.Begin(ctx)
	if err != nil {
		return Inventory{}, err
	}
	defer tx.Rollback()
	if err := tx.Charge(uid, c.Price*int64(qty), GameID, fmt.Sprintf("Đấu trường: mua %d %s", qty, c.Name)); err != nil {
		return Inventory{}, err
	}
	if err := addCharms(ctx, tx, uid, id, qty); err != nil {
		return Inventory{}, err
	}
	inv, err := loadInventory(ctx, tx, uid)
	if err != nil {
		return Inventory{}, err
	}
	return inv, tx.Commit()
}

// BuyStones mua qty viên đá cấp tier bằng xu.
func (a *Armory) BuyStones(ctx context.Context, uid int64, tier, qty int) (Inventory, error) {
	st, ok := findStone(tier)
	if !ok {
		return Inventory{}, ErrBadStone
	}
	if qty < 1 || qty > MaxBuy {
		return Inventory{}, ErrBadQty
	}
	tx, err := a.w.Begin(ctx)
	if err != nil {
		return Inventory{}, err
	}
	defer tx.Rollback()
	if err := tx.Charge(uid, st.Price*int64(qty), GameID, fmt.Sprintf("Đấu trường: mua %d %s", qty, st.Name)); err != nil {
		return Inventory{}, err
	}
	if err := addStones(ctx, tx, uid, tier, qty); err != nil {
		return Inventory{}, err
	}
	inv, err := loadInventory(ctx, tx, uid)
	if err != nil {
		return Inventory{}, err
	}
	return inv, tx.Commit()
}

// Upgrade cường hoá một ô bằng 1 viên đá cấp tier + phí xu, thành công theo tỉ lệ Rate (+LuckBonus nếu dùng bùa may mắn).
// Thất bại mất đá và phí, tụt 1 cấp (cấp 0 thì giữ) — trừ khi dùng bùa giữ cấp. Bùa đã chọn bị tiêu dù thành hay bại.
func (a *Armory) Upgrade(ctx context.Context, uid int64, slot string, tier int, luck, protect bool) (Inventory, Outcome, error) {
	if _, ok := slotNames[slot]; !ok {
		return Inventory{}, Outcome{}, ErrBadSlot
	}
	if _, ok := findStone(tier); !ok {
		return Inventory{}, Outcome{}, ErrBadStone
	}
	tx, err := a.w.Begin(ctx)
	if err != nil {
		return Inventory{}, Outcome{}, err
	}
	defer tx.Rollback()
	inv, err := loadInventory(ctx, tx, uid)
	if err != nil {
		return Inventory{}, Outcome{}, err
	}
	lv := inv.Gear.level(slot)
	out := Outcome{Slot: slot, Level: lv, From: lv}
	name := slotNames[slot]
	if slot == SlotGun {
		out.Arm = inv.Gear.ArmID()
		name = inv.Gear.Weapon().Name
	}
	switch {
	case lv >= MaxLevel:
		return Inventory{}, Outcome{}, ErrMaxed
	case inv.Stones[tier] <= 0:
		return Inventory{}, Outcome{}, ErrNoStone
	case luck && inv.Charms[CharmLuck] <= 0, protect && inv.Charms[CharmProtect] <= 0:
		return Inventory{}, Outcome{}, ErrNoCharm
	}
	rate := Rate(lv, tier)
	if luck {
		rate = min(100, rate+LuckBonus)
	}
	out.OK, out.Rate = a.roll() < rate, rate
	var verdict string
	switch {
	case out.OK:
		out.Level = lv + 1
		out.Evolved = out.Level == 7 || out.Level == 10
		verdict = "thành công"
	case protect:
		out.Protected = true
		verdict = "thất bại (giữ cấp nhờ bùa)"
	case lv > 0:
		out.Level, out.Dropped = lv-1, true
		verdict = fmt.Sprintf("thất bại, tụt về +%d", lv-1)
	default:
		verdict = "thất bại"
	}
	note := fmt.Sprintf("Đấu trường: cường hoá %s +%d %s", name, lv+1, verdict)
	if err := tx.Charge(uid, Fee(lv), GameID, note); err != nil {
		return Inventory{}, Outcome{}, err
	}
	if err := addStones(ctx, tx, uid, tier, -1); err != nil {
		return Inventory{}, Outcome{}, err
	}
	inv.Stones[tier]--
	for kind, used := range map[string]bool{CharmLuck: luck, CharmProtect: protect} {
		if !used {
			continue
		}
		if err := addCharms(ctx, tx, uid, kind, -1); err != nil {
			return Inventory{}, Outcome{}, err
		}
		inv.Charms[kind]--
	}
	if out.Level != lv && slot == SlotGun {
		if err := setArm(ctx, tx, uid, out.Arm, out.Level, true); err != nil {
			return Inventory{}, Outcome{}, err
		}
		if err := lockSockets(ctx, tx, uid, inv, out.Arm, out.Level); err != nil {
			return Inventory{}, Outcome{}, err
		}
		inv.Gear.set(slot, out.Level)
		inv.Arms[out.Arm] = out.Level
		if inv, err = loadInventory(ctx, tx, uid); err != nil {
			return Inventory{}, Outcome{}, err
		}
	} else if out.Level != lv {
		if _, err := tx.ExecContext(ctx, `INSERT INTO arena_gear (user_id, slot, level) VALUES (?, ?, ?)
			ON CONFLICT (user_id, slot) DO UPDATE SET level = excluded.level`, uid, slot, out.Level); err != nil {
			return Inventory{}, Outcome{}, err
		}
		inv.Gear.set(slot, out.Level)
	}
	return inv, out, tx.Commit()
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status, msg := http.StatusInternalServerError, err.Error()
	switch {
	case errors.Is(err, ErrBadSlot), errors.Is(err, ErrBadStone), errors.Is(err, ErrBadQty), errors.Is(err, ErrBadCharm),
		errors.Is(err, ErrBadArm), errors.Is(err, ErrBadGem), errors.Is(err, ErrBadSocket):
		status = http.StatusBadRequest
	case errors.Is(err, ErrMaxed), errors.Is(err, ErrNoStone), errors.Is(err, ErrNoCharm), errors.Is(err, ErrOwned),
		errors.Is(err, ErrNotOwned), errors.Is(err, ErrSameArm), errors.Is(err, ErrNoLevel), errors.Is(err, ErrLowLevel),
		errors.Is(err, ErrNoGem), errors.Is(err, ErrLocked), errors.Is(err, ErrEmptySlot):
		status = http.StatusConflict
	case errors.Is(err, wallet.ErrNotEnoughCoins):
		status, msg = http.StatusPaymentRequired, "không đủ xu"
	case errors.Is(err, wallet.ErrNotFound):
		status, msg = http.StatusUnauthorized, "phiên đăng nhập không hợp lệ"
	default:
		log.Printf("arena: %v", err)
		msg = "lỗi máy chủ, thử lại sau"
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// Handler trả API Đấu trường, gắn ở /api/arena/...: trang bị, đá cường hoá, bùa (cần đăng nhập) và danh sách map.
func (a *Armory) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/arena/maps", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, mapList())
	})
	mux.Handle("GET /api/arena/bosses", BossesHandler())
	type body struct {
		Slot    string `json:"slot"`
		Stone   int    `json:"stone"`
		Tier    int    `json:"tier"`
		Qty     int    `json:"qty"`
		ID      string `json:"id"`
		From    string `json:"from"`
		To      string `json:"to"`
		Arm     string `json:"arm"`
		Idx     int    `json:"idx"`
		Kind    string `json:"kind"`
		Luck    bool   `json:"luck"`
		Protect bool   `json:"protect"`
	}
	type extra struct {
		out *Outcome
		tr  *Transfer
	}
	authed := func(op func(r *http.Request, uid int64, b body) (Inventory, extra, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			u, err := a.w.UserByToken(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			if err != nil {
				writeErr(w, err)
				return
			}
			var b body
			if r.Method == http.MethodPost {
				json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&b)
			}
			inv, ex, err := op(r, u.ID, b)
			if err != nil {
				writeErr(w, err)
				return
			}
			bal, err := a.w.Balance(r.Context(), u.ID)
			if err != nil {
				writeErr(w, err)
				return
			}
			rep := reply(inv, bal)
			rep.Result, rep.Transfer = ex.out, ex.tr
			writeJSON(w, http.StatusOK, rep)
		}
	}
	mux.HandleFunc("GET /api/arena/gear", authed(func(r *http.Request, uid int64, _ body) (Inventory, extra, error) {
		inv, err := a.Inventory(r.Context(), uid)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/stones/buy", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.BuyStones(r.Context(), uid, b.Tier, b.Qty)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/charms/buy", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.BuyCharms(r.Context(), uid, b.ID, b.Qty)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/gear/upgrade", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, out, err := a.Upgrade(r.Context(), uid, b.Slot, b.Stone, b.Luck, b.Protect)
		return inv, extra{out: &out}, err
	}))
	mux.HandleFunc("POST /api/arena/gems/socket", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.SocketGem(r.Context(), uid, b.Arm, b.Idx, GemKey{b.Kind, b.Tier})
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/gems/unsocket", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.UnsocketGem(r.Context(), uid, b.Arm, b.Idx)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/arms/buy", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.BuyArm(r.Context(), uid, b.ID)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/arms/equip", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.EquipArm(r.Context(), uid, b.ID)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/arms/transfer", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, t, err := a.TransferArm(r.Context(), uid, b.From, b.To)
		return inv, extra{tr: &t}, err
	}))
	return mux
}
