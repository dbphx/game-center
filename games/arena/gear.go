package arena

// Trang bị vĩnh viễn của Đấu trường: Vũ khí (nhiều loại mua bằng xu, mỗi loại cấp 0..10 riêng, trang bị một), Nón,
// Áo giáp (cấp 0..10), cường hoá bằng đá (5 cấp, mua bằng xu) có tỉ lệ thành công; chuyển cấp giữa hai vũ khí; lưu trong database.

import (
	"cmp"
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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Desc     string `json:"desc"`
	Atk      int    `json:"atk"`
	Def      int    `json:"def"`
	Luck     int    `json:"luck"`
	Sta      int    `json:"sta"`
	ReqLevel int    `json:"reqLevel"` // cấp nhân vật cần để mua (điền theo bậc giá, xem tierLevels)
}

// tierLevels là cấp nhân vật cần để mua món thứ i của mỗi ô (rẻ → đắt); món 0 là món mặc định miễn phí.
var tierLevels = []int{1, 1, 6, 12, 20, 30}

// withLevels điền cấp yêu cầu theo thứ tự giá trong danh mục.
func withLevels(list []Arm) []Arm {
	for i := range list {
		list[i].ReqLevel = tierLevels[min(i, len(tierLevels)-1)]
	}
	return list
}

const ArmCannon = "cannon"

var Arms = withLevels([]Arm{
	{ArmCannon, "Đại bác", 0, "Mặc định, ai cũng có", 100, 50, 20, 100, 0},
	{"dep", "Dép lào", 45000, "Nhẹ, ném đâu trúng đó", 150, 80, 40, 150, 0},
	{"choi", "Chổi chà", 90000, "Quét sạch đối thủ", 220, 120, 60, 220, 0},
	{"launha", "Cây lau nhà", 180000, "Lau sân, lau luôn đối thủ", 300, 160, 80, 300, 0},
	{"tv", "TV", 360000, "Nặng, nổ to như phim hành động", 400, 240, 100, 400, 0},
	{"tulanh", "Tủ lạnh", 750000, "Vũ khí tối thượng của mọi nhà", 550, 320, 120, 600, 0},
})

const (
	HatDefault   = "nonla"  // nón mặc định, ai cũng có
	ArmorDefault = "aothun" // áo mặc định, ai cũng có
)

// Hats là danh mục nón: thiên may mắn + thể lực. Cùng kiểu Arm (4 chỉ số gốc ở +0).
var Hats = withLevels([]Arm{
	{HatDefault, "Nón lá", 0, "Mặc định, che nắng che mưa", 0, 20, 20, 100, 0},
	{"mubh", "Mũ bảo hiểm", 60000, "An toàn là trên hết", 20, 60, 30, 150, 0},
	{"mulen", "Mũ len", 120000, "Ấm đầu, tỉnh táo", 40, 80, 50, 220, 0},
	{"caoboi", "Nón cao bồi", 240000, "Bắn nhanh như cao bồi miền Tây", 70, 100, 80, 320, 0},
	{"phuthuy", "Mũ phù thuỷ", 450000, "Phép màu gọi may mắn", 120, 120, 100, 500, 0},
	{"vuongmien", "Vương miện", 900000, "Đội lên là thành vua đấu trường", 160, 180, 130, 800, 0},
})

// Armors là danh mục áo giáp: thiên phòng thủ + thể lực.
var Armors = withLevels([]Arm{
	{ArmorDefault, "Áo thun", 0, "Mặc định, thoáng mát", 0, 40, 0, 150, 0},
	{"aomua", "Áo mưa", 60000, "Đạn trượt như nước", 0, 80, 10, 200, 0},
	{"bongda", "Áo bóng đá", 120000, "Chạy cả trận không mệt", 30, 110, 20, 260, 0},
	{"aophao", "Áo phao", 240000, "Rơi xuống nước vẫn nổi", 40, 160, 20, 380, 0},
	{"giapsat", "Giáp sắt", 450000, "Cứng như tường thành", 60, 240, 30, 550, 0},
	{"giaprong", "Giáp rồng", 900000, "Vảy rồng, đạn bật ra", 100, 320, 50, 800, 0},
})

// catalog là danh mục món của một ô trang bị.
func catalog(slot string) []Arm {
	switch slot {
	case SlotGun:
		return Arms
	case SlotHat:
		return Hats
	case SlotArmor:
		return Armors
	}
	return nil
}

// defaultItem là món mặc định (miễn phí) của một ô.
func defaultItem(slot string) string {
	switch slot {
	case SlotHat:
		return HatDefault
	case SlotArmor:
		return ArmorDefault
	}
	return ArmCannon
}

// findItem tìm món id trong danh mục của ô slot.
func findItem(slot, id string) (Arm, bool) {
	for _, a := range catalog(slot) {
		if a.ID == id {
			return a, true
		}
	}
	return Arm{}, false
}

// slotOfItem trả ô trang bị của món id (id các món không trùng nhau giữa các ô).
func slotOfItem(id string) (string, bool) {
	for _, s := range Slots {
		if _, ok := findItem(s, id); ok {
			return s, true
		}
	}
	return "", false
}

func findArm(id string) (Arm, bool) { return findItem(SlotGun, id) }

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

// Gear là trang bị đang dùng: vũ khí Arm ở cấp Gun, nón HatID ở cấp Hat, áo ArmorID ở cấp Armor.
type Gear struct {
	Arm     string   `json:"arm"`     // id vũ khí đang trang bị ("" = đại bác)
	Gun     int      `json:"gun"`     // cấp của vũ khí đang trang bị
	HatID   string   `json:"hatId"`   // id nón đang đội ("" = nón lá)
	Hat     int      `json:"hat"`     // cấp của nón đang đội
	ArmorID string   `json:"armorId"` // id áo đang mặc ("" = áo thun)
	Armor   int      `json:"armor"`   // cấp của áo đang mặc
	Bonus   ArmStats `json:"bonus"`   // chỉ số cộng từ ngọc khảm ở các ô đã mở của vũ khí, nón và áo đang dùng
}

// item là món đang dùng ở ô slot (rỗng / lạ thì món mặc định).
func (g Gear) item(slot string) Arm {
	id := map[string]string{SlotGun: g.Arm, SlotHat: g.HatID, SlotArmor: g.ArmorID}[slot]
	if a, ok := findItem(slot, id); ok {
		return a
	}
	a, _ := findItem(slot, defaultItem(slot))
	return a
}

// itemID là id món đang dùng ở ô slot.
func (g Gear) itemID(slot string) string { return g.item(slot).ID }

// Weapon là vũ khí đang trang bị (mặc định đại bác).
func (g Gear) Weapon() Arm { return g.item(SlotGun) }

// ArmID là id vũ khí đang trang bị.
func (g Gear) ArmID() string { return g.Weapon().ID }

// Norm điền id mặc định cho các ô còn trống (gửi cho giao diện).
func (g Gear) Norm() Gear {
	g.Arm, g.HatID, g.ArmorID = g.itemID(SlotGun), g.itemID(SlotHat), g.itemID(SlotArmor)
	return g
}

func (s ArmStats) plus(o ArmStats) ArmStats {
	return ArmStats{s.Atk + o.Atk, s.Def + o.Def, s.Luck + o.Luck, s.Sta + o.Sta}
}

// Stats là tổng chỉ số vũ khí + nón + áo đang dùng ở cấp hiện tại, cộng ngọc khảm.
func (g Gear) Stats() ArmStats {
	return g.Weapon().Eff(g.Gun).plus(g.item(SlotHat).Eff(g.Hat)).plus(g.item(SlotArmor).Eff(g.Armor)).plus(g.Bonus)
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

// Power là lực chiến: 10000 + 2 × tổng 4 chỉ số.
func (g Gear) Power() int {
	s := g.Stats()
	return 10000 + 2*(s.Atk+s.Def+s.Luck+s.Sta)
}

// MaxHP là máu tối đa trong trận: 2000 + thể lực tổng.
func (g Gear) MaxHP() int { return 2000 + g.Stats().Sta }

// DmgMul là hệ số sát thương: 1 + tấn công / 1000.
func (g Gear) DmgMul() float64 { return 1 + float64(g.Stats().Atk)/1000 }

// MaxArmorPct là tỉ lệ giảm sát thương tối đa.
const MaxArmorPct = 0.6

// ArmorPct là tỉ lệ giảm sát thương nhận: phòng thủ tổng / 1000, tối đa 0.6.
func (g Gear) ArmorPct() float64 {
	return math.Min(MaxArmorPct, float64(g.Stats().Def)/1000)
}

// CritPermille là tỉ lệ chí mạng tính theo phần nghìn (×CritMul sát thương) = may mắn tổng (may mắn 40 = 4%).
func (g Gear) CritPermille() int { return g.Stats().Luck }

// CritPct là % chí mạng làm tròn (hiển thị) = may mắn / 10.
func (g Gear) CritPct() int { return int(math.Round(float64(g.Stats().Luck) / 10)) }

const CritMul = 1.5

// TransferFeePerLevel là phí xu chuyển cấp, nhân số cấp chuyển.
const TransferFeePerLevel = 3000

// TransferFee là phí chuyển cấp lv từ vũ khí này sang vũ khí khác.
func TransferFee(lv int) int64 { return TransferFeePerLevel * int64(lv) }

// Stone là một cấp đá cường hoá.
type Stone struct {
	Tier  int    `json:"tier"`
	Name  string `json:"name"`
	Price int64  `json:"price"`
}

var Stones = []Stone{
	{1, "Đá Lục", 500}, {2, "Đá Lam", 1500}, {3, "Đá Tím", 4000}, {4, "Đá Vàng", 10000}, {5, "Đá Đỏ", 25000},
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
	{CharmLuck, "Bùa may mắn", 3000, "+15% tỉ lệ"},
	{CharmProtect, "Bùa giữ cấp", 4000, "Thất bại không tụt cấp"},
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

// Rate là tỉ lệ (%) cường hoá từ cấp lv lên lv+1 bằng đá tier: càng lên cao càng khó, tối thiểu 1%, không quá RateCap.
func Rate(lv, tier int) int { return min(RateCap(lv), max(1, 80-10*lv+6*(tier-1))) }

// RateCap là tỉ lệ tối đa từ cấp lv, kể cả khi cộng bùa may mắn: lên +10 không quá 5%.
func RateCap(lv int) int {
	if lv >= MaxLevel-1 {
		return 5
	}
	return 95
}

// Fee là phí xu mỗi lần cường hoá từ cấp lv (trừ cả khi thất bại).
func Fee(lv int) int64 { return 100 * int64(lv+1) }

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
	Slot  string `json:"slot"`
	From  string `json:"from"`
	To    string `json:"to"`
	Level int    `json:"level"`
}

// Outcome là kết quả một lần cường hoá.
type Outcome struct {
	OK        bool   `json:"ok"`
	Slot      string `json:"slot"`
	Arm       string `json:"arm,omitempty"` // vũ khí được cường hoá (ô gun)
	Item      string `json:"item"`          // id món được cường hoá (vũ khí / nón / áo đang dùng)
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
	Hats      []ArmInfo        `json:"hats"`
	Armors    []ArmInfo        `json:"armors"`
	Gems      []GemInfo        `json:"gems"` // ngọc trong kho (chỉ loại còn ≥ 1 viên)
	// SlotSockets là 3 ô khảm của nón ("hat") và áo ("armor") đang dùng, cùng định dạng arms[].sockets (giữ để tương thích).
	SlotSockets map[string][]SocketInfo `json:"slotSockets"`
	// TransferFeePerLevel: phí chuyển cấp = giá trị này × cấp của vũ khí nguồn.
	TransferFeePerLevel int64     `json:"transferFeePerLevel"`
	Result              *Outcome  `json:"result,omitempty"`
	Transfer            *Transfer `json:"transfer,omitempty"`
}

// Inventory là trang bị + đá của một người.
type Inventory struct {
	Gear    Gear
	Arms    map[string]int // mọi món đã sở hữu (vũ khí, nón, áo) → cấp; luôn có đại bác, nón lá, áo thun
	Gems    map[GemKey]int
	Sockets map[string][SocketCount]GemKey
	Stones  [6]int // chỉ số 1..5
	Charms  map[string]int
}

func reply(inv Inventory, balance int64) GearReply {
	g := inv.Gear
	r := GearReply{Gear: g.Norm(), Power: g.Power(), MaxLevel: MaxLevel, Balance: balance, Stones: map[string]int{},
		StoneShop: Stones, Fee: map[string]int64{}, Rates: map[string][]int{}, Charms: map[string]int{},
		CharmShop: Charms, LuckBonus: LuckBonus, TransferFeePerLevel: TransferFeePerLevel}
	st := g.Stats()
	r.Stats = GearStats{DmgPct: int(math.Round(float64(st.Atk) / 10)), MaxHP: g.MaxHP(), ArmorPct: int(math.Round(100 * g.ArmorPct())),
		Atk: st.Atk, Def: st.Def, Luck: st.Luck, Sta: st.Sta}
	r.Arms, r.Hats, r.Armors = itemInfos(inv, SlotGun), itemInfos(inv, SlotHat), itemInfos(inv, SlotArmor)
	r.SlotSockets = map[string][]SocketInfo{
		SlotHat:   socketInfos(inv.Sockets[g.itemID(SlotHat)], g.Hat),
		SlotArmor: socketInfos(inv.Sockets[g.itemID(SlotArmor)], g.Armor),
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

// itemInfos là danh mục món của ô slot kèm cấp, chỉ số đã nhân cấp + ngọc, sở hữu, đang dùng, ô khảm.
func itemInfos(inv Inventory, slot string) []ArmInfo {
	out := []ArmInfo{}
	using := inv.Gear.itemID(slot)
	for _, a := range catalog(slot) {
		lv, owned := inv.Arms[a.ID]
		owned = owned || a.ID == defaultItem(slot)
		eff := a.Eff(lv).plus(gemBonus(inv.Sockets[a.ID], lv))
		out = append(out, ArmInfo{Arm: a, Eff: eff, Owned: owned, Level: lv, Equipped: a.ID == using,
			Sockets: socketInfos(inv.Sockets[a.ID], lv)})
	}
	return out
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
	ErrBadArm   = errors.New("trang bị không hợp lệ")
	ErrOwned    = errors.New("bạn đã có món này")
	ErrNotOwned = errors.New("bạn chưa có món này, hãy mua trước")
	ErrSameArm  = errors.New("hãy chọn hai món khác nhau")
	ErrNoLevel  = errors.New("món nguồn chưa cường hoá, không có cấp để chuyển")
	ErrLowLevel = errors.New("món nguồn phải có cấp cao hơn món nhận")
)

// NeedLevelError: nhân vật chưa đủ cấp để mua món.
type NeedLevelError struct {
	Level int
	Name  string
}

func (e *NeedLevelError) Error() string {
	return fmt.Sprintf("Cần cấp %d để mua %s", e.Level, e.Name)
}

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
	// Tương tự cấp nón / áo giáp cũ → nón lá / áo thun đang dùng; ngọc ô khảm "hat" / "armor" cũ → ô của món đó.
	for _, s := range Slots {
		if _, err := tx.ExecContext(ctx, `INSERT INTO arena_arms (user_id, arm, level, equipped)
			SELECT user_id, ?, level, 1 FROM arena_gear WHERE slot = ? ON CONFLICT (user_id, arm) DO NOTHING`, defaultItem(s), s); err != nil {
			return nil, fmt.Errorf("chuyển cấp %s cũ: %w", slotNames[s], err)
		}
		if s == SlotGun {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE arena_sockets SET arm = ? WHERE arm = ?`, defaultItem(s), s); err != nil {
			return nil, fmt.Errorf("chuyển ngọc %s cũ: %w", slotNames[s], err)
		}
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
	inv := Inventory{Charms: map[string]int{}, Arms: map[string]int{}}
	equipped := map[string]string{}
	for _, s := range Slots {
		inv.Arms[defaultItem(s)] = 0
		equipped[s] = defaultItem(s)
	}
	// Cấp mọi món (vũ khí, nón, áo) nằm ở arena_arms; arena_gear chỉ còn dữ liệu cũ đã chuyển lúc khởi động.
	rows, err := tx.QueryContext(ctx, `SELECT arm, level, equipped FROM arena_arms WHERE user_id = ?`, uid)
	if err != nil {
		return inv, err
	}
	for rows.Next() {
		var id string
		var lv, eq int
		if err := rows.Scan(&id, &lv, &eq); err != nil {
			rows.Close()
			return inv, err
		}
		if s, ok := slotOfItem(id); ok {
			inv.Arms[id] = min(max(lv, 0), MaxLevel)
			if eq != 0 {
				equipped[s] = id
			}
		}
	}
	rows.Close()
	inv.Gear.Arm, inv.Gear.Gun = equipped[SlotGun], inv.Arms[equipped[SlotGun]]
	inv.Gear.HatID, inv.Gear.Hat = equipped[SlotHat], inv.Arms[equipped[SlotHat]]
	inv.Gear.ArmorID, inv.Gear.Armor = equipped[SlotArmor], inv.Arms[equipped[SlotArmor]]
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
	return a.BuyItem(ctx, uid, SlotGun, id)
}

// EquipArm trang bị vũ khí id đã sở hữu.
func (a *Armory) EquipArm(ctx context.Context, uid int64, id string) (Inventory, error) {
	return a.EquipItem(ctx, uid, SlotGun, id)
}

// TransferArm chuyển cấp cường hoá từ vũ khí from sang vũ khí to.
func (a *Armory) TransferArm(ctx context.Context, uid int64, from, to string) (Inventory, Transfer, error) {
	return a.TransferItem(ctx, uid, SlotGun, from, to)
}

// BuyItem mua món id của ô slot (gun / hat / armor) bằng xu (cấp 0, chưa dùng).
func (a *Armory) BuyItem(ctx context.Context, uid int64, slot, id string) (Inventory, error) {
	it, ok := findItem(slot, id)
	if !ok {
		return Inventory{}, ErrBadArm
	}
	return a.withInv(ctx, uid, func(tx *wallet.Tx, inv Inventory) error {
		if _, have := inv.Arms[id]; have {
			return ErrOwned
		}
		lv, err := tx.Level(uid)
		if err != nil {
			return err
		}
		if lv < it.ReqLevel {
			return &NeedLevelError{it.ReqLevel, it.Name}
		}
		if err := tx.Charge(uid, it.Price, GameID, "Đấu trường: mua "+it.Name); err != nil {
			return err
		}
		return setArm(ctx, tx, uid, id, 0, false)
	})
}

// EquipItem dùng món id đã sở hữu ở ô slot (bỏ món đang dùng của ô đó).
func (a *Armory) EquipItem(ctx context.Context, uid int64, slot, id string) (Inventory, error) {
	if _, ok := findItem(slot, id); !ok {
		return Inventory{}, ErrBadArm
	}
	return a.withInv(ctx, uid, func(tx *wallet.Tx, inv Inventory) error {
		lv, have := inv.Arms[id]
		if !have {
			return ErrNotOwned
		}
		ids := []any{uid}
		marks := make([]string, 0, len(catalog(slot)))
		for _, it := range catalog(slot) {
			ids, marks = append(ids, it.ID), append(marks, "?")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE arena_arms SET equipped = 0 WHERE user_id = ? AND arm IN (`+
			strings.Join(marks, ",")+`)`, ids...); err != nil {
			return err
		}
		if err := setArm(ctx, tx, uid, id, lv, true); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE arena_arms SET equipped = 1 WHERE user_id = ? AND arm = ?`, uid, id)
		return err
	})
}

// TransferItem chuyển cấp cường hoá trong cùng ô slot: to nhận cấp của from, from về +0 (ngọc ở ô bị khoá về kho);
// phí TransferFee.
func (a *Armory) TransferItem(ctx context.Context, uid int64, slot, from, to string) (Inventory, Transfer, error) {
	fa, ok1 := findItem(slot, from)
	ta, ok2 := findItem(slot, to)
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
		using := inv.Gear.itemID(slot)
		note := fmt.Sprintf("Đấu trường: chuyển cấp +%d từ %s sang %s", lf, fa.Name, ta.Name)
		if err := tx.Charge(uid, TransferFee(lf), GameID, note); err != nil {
			return err
		}
		if err := setArm(ctx, tx, uid, to, lf, to == using); err != nil {
			return err
		}
		if err := setArm(ctx, tx, uid, from, 0, from == using); err != nil {
			return err
		}
		if err := lockSockets(ctx, tx, uid, inv, from, 0); err != nil {
			return err
		}
		t = Transfer{Slot: slot, From: from, To: to, Level: lf}
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
	it := inv.Gear.item(slot)
	out := Outcome{Slot: slot, Item: it.ID, Level: lv, From: lv}
	name := it.Name
	if slot == SlotGun {
		out.Arm = it.ID
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
		rate = min(RateCap(lv), rate+LuckBonus)
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
	if out.Level != lv {
		if err := setArm(ctx, tx, uid, it.ID, out.Level, true); err != nil {
			return Inventory{}, Outcome{}, err
		}
		// Tụt cấp: ngọc ở ô bị khoá về kho.
		if err := lockSockets(ctx, tx, uid, inv, it.ID, out.Level); err != nil {
			return Inventory{}, Outcome{}, err
		}
		if inv, err = loadInventory(ctx, tx, uid); err != nil {
			return Inventory{}, Outcome{}, err
		}
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
	var need *NeedLevelError
	switch {
	case errors.As(err, &need):
		status = http.StatusForbidden
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
	mux.HandleFunc("GET /api/arena/players/{username}", a.profileHandler)
	type body struct {
		Slot    string `json:"slot"`
		Stone   int    `json:"stone"`
		Tier    int    `json:"tier"`
		Qty     int    `json:"qty"`
		ID      string `json:"id"`
		From    string `json:"from"`
		To      string `json:"to"`
		Arm     string `json:"arm"`
		Item    string `json:"item"`
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
		inv, err := a.SocketSlotGem(r.Context(), uid, b.Slot, cmp.Or(b.Item, b.Arm), b.Idx, GemKey{b.Kind, b.Tier})
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/gems/unsocket", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.UnsocketSlotGem(r.Context(), uid, b.Slot, cmp.Or(b.Item, b.Arm), b.Idx)
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
	mux.HandleFunc("POST /api/arena/items/buy", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.BuyItem(r.Context(), uid, cmp.Or(b.Slot, SlotGun), b.ID)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/items/equip", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, err := a.EquipItem(r.Context(), uid, cmp.Or(b.Slot, SlotGun), b.ID)
		return inv, extra{}, err
	}))
	mux.HandleFunc("POST /api/arena/items/transfer", authed(func(r *http.Request, uid int64, b body) (Inventory, extra, error) {
		inv, t, err := a.TransferItem(r.Context(), uid, cmp.Or(b.Slot, SlotGun), b.From, b.To)
		return inv, extra{tr: &t}, err
	}))
	return mux
}
