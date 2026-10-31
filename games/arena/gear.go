package arena

// Trang bị vĩnh viễn của Đấu trường: Súng, Nón, Áo giáp (cấp 0..10), cường hoá bằng đá (5 cấp, mua bằng xu)
// có tỉ lệ thành công; lưu trong database.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
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

// Gear là cấp từng ô trang bị.
type Gear struct {
	Gun   int `json:"gun"`
	Hat   int `json:"hat"`
	Armor int `json:"armor"`
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
func (g Gear) Power() int { return 1000 + 120*g.Gun + 80*g.Hat + 100*g.Armor }

// MaxHP là máu tối đa trong trận.
func (g Gear) MaxHP() int { return 200 + 15*g.Hat }

// DmgMul là hệ số sát thương của súng.
func (g Gear) DmgMul() float64 { return 1 + 0.08*float64(g.Gun) }

// ArmorPct là tỉ lệ giảm sát thương nhận (0..0.3).
func (g Gear) ArmorPct() float64 { return 0.03 * float64(g.Armor) }

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
}

// Outcome là kết quả một lần cường hoá.
type Outcome struct {
	OK        bool   `json:"ok"`
	Slot      string `json:"slot"`
	Level     int    `json:"level"` // cấp sau lần cường hoá
	From      int    `json:"from"`  // cấp trước lần cường hoá
	Rate      int    `json:"rate"`  // tỉ lệ đã cộng bùa may mắn
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
	Result    *Outcome         `json:"result,omitempty"`
}

// Inventory là trang bị + đá của một người.
type Inventory struct {
	Gear   Gear
	Stones [6]int // chỉ số 1..5
	Charms map[string]int
}

func reply(inv Inventory, balance int64) GearReply {
	g := inv.Gear
	r := GearReply{Gear: g, Power: g.Power(), MaxLevel: MaxLevel, Balance: balance, Stones: map[string]int{},
		StoneShop: Stones, Fee: map[string]int64{}, Rates: map[string][]int{}, Charms: map[string]int{},
		CharmShop: Charms, LuckBonus: LuckBonus,
		Stats: GearStats{DmgPct: 8 * g.Gun, MaxHP: g.MaxHP(), ArmorPct: 3 * g.Armor}}
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
)

// Armory là kho trang bị và đá cường hoá của mọi người chơi.
type Armory struct {
	w *wallet.Wallet
	// roll trả số ngẫu nhiên 0..99 cho mỗi lần cường hoá (test thay để có kết quả cố định).
	roll func() int
}

// New tạo bảng trang bị và trả kho.
func New(ctx context.Context, w *wallet.Wallet) (*Armory, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng đấu trường: %w", err)
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
	inv := Inventory{Charms: map[string]int{}}
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
		inv.Gear.set(s, min(max(lv, 0), MaxLevel))
	}
	rows.Close()
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
	out := Outcome{OK: a.roll() < rate, Slot: slot, Level: lv, From: lv, Rate: rate}
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
	note := fmt.Sprintf("Đấu trường: cường hoá %s +%d %s", slotNames[slot], lv+1, verdict)
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
	case errors.Is(err, ErrBadSlot), errors.Is(err, ErrBadStone), errors.Is(err, ErrBadQty), errors.Is(err, ErrBadCharm):
		status = http.StatusBadRequest
	case errors.Is(err, ErrMaxed), errors.Is(err, ErrNoStone), errors.Is(err, ErrNoCharm):
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
	type body struct {
		Slot    string `json:"slot"`
		Stone   int    `json:"stone"`
		Tier    int    `json:"tier"`
		Qty     int    `json:"qty"`
		ID      string `json:"id"`
		Luck    bool   `json:"luck"`
		Protect bool   `json:"protect"`
	}
	authed := func(op func(r *http.Request, uid int64, b body) (Inventory, *Outcome, error)) http.HandlerFunc {
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
			inv, out, err := op(r, u.ID, b)
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
			rep.Result = out
			writeJSON(w, http.StatusOK, rep)
		}
	}
	mux.HandleFunc("GET /api/arena/gear", authed(func(r *http.Request, uid int64, _ body) (Inventory, *Outcome, error) {
		inv, err := a.Inventory(r.Context(), uid)
		return inv, nil, err
	}))
	mux.HandleFunc("POST /api/arena/stones/buy", authed(func(r *http.Request, uid int64, b body) (Inventory, *Outcome, error) {
		inv, err := a.BuyStones(r.Context(), uid, b.Tier, b.Qty)
		return inv, nil, err
	}))
	mux.HandleFunc("POST /api/arena/charms/buy", authed(func(r *http.Request, uid int64, b body) (Inventory, *Outcome, error) {
		inv, err := a.BuyCharms(r.Context(), uid, b.ID, b.Qty)
		return inv, nil, err
	}))
	mux.HandleFunc("POST /api/arena/gear/upgrade", authed(func(r *http.Request, uid int64, b body) (Inventory, *Outcome, error) {
		inv, out, err := a.Upgrade(r.Context(), uid, b.Slot, b.Stone, b.Luck, b.Protect)
		return inv, &out, err
	}))
	return mux
}
