package arena

// Ngọc khảm: 4 loại (tấn công, phòng thủ, may mắn, thể lực) × 3 bậc, cộng thẳng vào chỉ số chiến đấu.
// Mỗi vũ khí, nón và áo giáp có 3 ô khảm, mở ở +3, +7, +10 theo cấp món đó; cấp giảm thì ngọc ở ô bị khoá tự về kho.
// Ô khảm lưu trong arena_sockets theo cột arm: id vũ khí, hoặc "hat" / "armor" cho nón / áo giáp.

import (
	"context"
	"errors"
	"fmt"

	"github.com/dbphx/game-center/internal/wallet"
)

const (
	GemAtk  = "atk"
	GemDef  = "def"
	GemLuck = "luck"
	GemSta  = "sta"

	SocketCount = 3
	GemTiers    = 3
)

var GemKinds = []string{GemAtk, GemDef, GemLuck, GemSta}

var gemNames = map[string]string{GemAtk: "Tấn công", GemDef: "Phòng thủ", GemLuck: "May mắn", GemSta: "Thể lực"}

// gemValues là giá trị cộng theo loại và bậc 1..3.
var gemValues = map[string][GemTiers]int{GemAtk: {30, 60, 100}, GemDef: {20, 40, 70}, GemLuck: {10, 20, 40}, GemSta: {50, 100, 180}}

// SocketUnlock là cấp trang bị mở từng ô khảm.
var SocketUnlock = [SocketCount]int{3, 7, 10}

// OpenSockets là số ô khảm đã mở ở cấp lv.
func OpenSockets(lv int) int {
	n := 0
	for _, u := range SocketUnlock {
		if lv >= u {
			n++
		}
	}
	return n
}

// GemKey là một loại ngọc (Kind rỗng = ô trống).
type GemKey struct {
	Kind string `json:"kind"`
	Tier int    `json:"tier"`
}

func (k GemKey) valid() bool {
	_, ok := gemValues[k.Kind]
	return ok && k.Tier >= 1 && k.Tier <= GemTiers
}

// Name là tên ngọc, vd "Ngọc Tấn công II".
func (k GemKey) Name() string {
	return fmt.Sprintf("Ngọc %s %s", gemNames[k.Kind], [...]string{"I", "II", "III"}[k.Tier-1])
}

// Value là giá trị cộng của ngọc.
func (k GemKey) Value() int { return gemValues[k.Kind][k.Tier-1] }

func (k GemKey) add(s *ArmStats) {
	switch k.Kind {
	case GemAtk:
		s.Atk += k.Value()
	case GemDef:
		s.Def += k.Value()
	case GemLuck:
		s.Luck += k.Value()
	case GemSta:
		s.Sta += k.Value()
	}
}

// GemInfo là ngọc gửi cho giao diện (trong kho có Qty, trong ô khảm thì Qty = 0).
type GemInfo struct {
	Kind  string `json:"kind"`
	Tier  int    `json:"tier"`
	Name  string `json:"name"`
	Value int    `json:"value"`
	Qty   int    `json:"qty,omitempty"`
}

func gemInfo(k GemKey, qty int) GemInfo {
	return GemInfo{Kind: k.Kind, Tier: k.Tier, Name: k.Name(), Value: k.Value(), Qty: qty}
}

// SocketInfo là một ô khảm của vũ khí / nón / áo giáp.
type SocketInfo struct {
	Idx      int      `json:"idx"`
	UnlockAt int      `json:"unlockAt"`
	Open     bool     `json:"open"`
	Gem      *GemInfo `json:"gem"`
}

// gemBonus là tổng chỉ số ngọc ở các ô đã mở của một món trang bị cấp lv.
func gemBonus(sockets [SocketCount]GemKey, lv int) ArmStats {
	var s ArmStats
	for i := 0; i < OpenSockets(lv); i++ {
		if sockets[i].valid() {
			sockets[i].add(&s)
		}
	}
	return s
}

func socketInfos(sockets [SocketCount]GemKey, lv int) []SocketInfo {
	out := make([]SocketInfo, SocketCount)
	for i := range out {
		out[i] = SocketInfo{Idx: i, UnlockAt: SocketUnlock[i], Open: i < OpenSockets(lv)}
		if sockets[i].valid() {
			g := gemInfo(sockets[i], 0)
			out[i].Gem = &g
		}
	}
	return out
}

var (
	ErrBadGem     = errors.New("ngọc không hợp lệ")
	ErrBadSocket  = errors.New("ô khảm không hợp lệ")
	ErrNoGem      = errors.New("bạn không có ngọc này")
	ErrLocked     = errors.New("ô khảm chưa mở — cường hoá trang bị lên cấp cao hơn")
	ErrEmptySlot  = errors.New("ô khảm đang trống")
	errBadGemRows = errors.New("dữ liệu ngọc hỏng")
)

const gemSchema = `
CREATE TABLE IF NOT EXISTS arena_gems (
	user_id INTEGER NOT NULL REFERENCES users(id),
	kind    TEXT NOT NULL,
	tier    INTEGER NOT NULL,
	qty     INTEGER NOT NULL,
	PRIMARY KEY (user_id, kind, tier)
);
CREATE TABLE IF NOT EXISTS arena_sockets (
	user_id INTEGER NOT NULL REFERENCES users(id),
	arm     TEXT NOT NULL,
	idx     INTEGER NOT NULL,
	kind    TEXT NOT NULL,
	tier    INTEGER NOT NULL,
	PRIMARY KEY (user_id, arm, idx)
);
`

// loadGems đọc kho ngọc và ô khảm vào inv, rồi tính chỉ số ngọc đang có hiệu lực:
// vũ khí đang trang bị + nón + áo giáp (inv.Gear đã có cấp các ô).
func loadGems(ctx context.Context, tx *wallet.Tx, uid int64, inv *Inventory) error {
	inv.Gems, inv.Sockets = map[GemKey]int{}, map[string][SocketCount]GemKey{}
	rows, err := tx.QueryContext(ctx, `SELECT kind, tier, qty FROM arena_gems WHERE user_id = ?`, uid)
	if err != nil {
		return err
	}
	for rows.Next() {
		var k GemKey
		var n int
		if err := rows.Scan(&k.Kind, &k.Tier, &n); err != nil {
			rows.Close()
			return err
		}
		if k.valid() && n > 0 {
			inv.Gems[k] = n
		}
	}
	rows.Close()
	rows, err = tx.QueryContext(ctx, `SELECT arm, idx, kind, tier FROM arena_sockets WHERE user_id = ?`, uid)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var arm string
		var idx int
		var k GemKey
		if err := rows.Scan(&arm, &idx, &k.Kind, &k.Tier); err != nil {
			return err
		}
		if idx >= 0 && idx < SocketCount && k.valid() {
			s := inv.Sockets[arm]
			s[idx] = k
			inv.Sockets[arm] = s
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	inv.Gear.Bonus = activeBonus(inv)
	return nil
}

// activeBonus là tổng ngọc của vũ khí, nón và áo đang dùng.
func activeBonus(inv *Inventory) ArmStats {
	var s ArmStats
	for _, b := range []ArmStats{
		gemBonus(inv.Sockets[inv.Gear.ArmID()], inv.Gear.Gun),
		gemBonus(inv.Sockets[inv.Gear.itemID(SlotHat)], inv.Gear.Hat),
		gemBonus(inv.Sockets[inv.Gear.itemID(SlotArmor)], inv.Gear.Armor),
	} {
		s.Atk, s.Def, s.Luck, s.Sta = s.Atk+b.Atk, s.Def+b.Def, s.Luck+b.Luck, s.Sta+b.Sta
	}
	return s
}

func addGems(ctx context.Context, tx *wallet.Tx, uid int64, k GemKey, n int) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO arena_gems (user_id, kind, tier, qty) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, kind, tier) DO UPDATE SET qty = qty + excluded.qty`, uid, k.Kind, k.Tier, n)
	return err
}

// unsocket trả ngọc ở ô idx của món arm (id vũ khí / "hat" / "armor") về kho (ô trống thì thôi).
func unsocket(ctx context.Context, tx *wallet.Tx, uid int64, arm string, idx int, k GemKey) error {
	if !k.valid() {
		return nil
	}
	if err := addGems(ctx, tx, uid, k, 1); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM arena_sockets WHERE user_id = ? AND arm = ? AND idx = ?`, uid, arm, idx)
	return err
}

// lockSockets trả về kho ngọc ở các ô của món arm (id vũ khí / "hat" / "armor") bị khoá khi món đó ở cấp lv.
func lockSockets(ctx context.Context, tx *wallet.Tx, uid int64, inv Inventory, arm string, lv int) error {
	s := inv.Sockets[arm]
	for i := OpenSockets(lv); i < SocketCount; i++ {
		if err := unsocket(ctx, tx, uid, arm, i, s[i]); err != nil {
			return err
		}
	}
	return nil
}

// itemSlot chuẩn hoá ô trang bị của API khảm: rỗng = "gun".
func itemSlot(slot string) (string, error) {
	switch slot {
	case "", SlotGun:
		return SlotGun, nil
	case SlotHat, SlotArmor:
		return slot, nil
	}
	return "", ErrBadSlot
}

// socketOwner trả id món được khảm (cột arm của arena_sockets) và cấp hiện tại của nó: món item của ô slot,
// item rỗng = món đang dùng của ô đó.
func socketOwner(inv Inventory, slot, item string) (string, int, error) {
	if item == "" {
		item = inv.Gear.itemID(slot)
	}
	lv, have := inv.Arms[item]
	if !have {
		return "", 0, ErrNotOwned
	}
	return item, lv, nil
}

func checkSocketArgs(slot, item string, idx int) error {
	s, err := itemSlot(slot)
	if err != nil {
		return err
	}
	if _, ok := findItem(s, item); item != "" && !ok {
		return ErrBadArm
	}
	if idx < 0 || idx >= SocketCount {
		return ErrBadSocket
	}
	return nil
}

// SocketGem khảm 1 viên ngọc k từ kho vào ô idx (đã mở) của vũ khí arm; ngọc cũ trong ô về kho.
func (a *Armory) SocketGem(ctx context.Context, uid int64, arm string, idx int, k GemKey) (Inventory, error) {
	return a.SocketSlotGem(ctx, uid, SlotGun, arm, idx, k)
}

// SocketSlotGem khảm ngọc vào ô idx của món item thuộc ô trang bị slot ("gun" / "hat" / "armor");
// item rỗng = món đang dùng của ô đó.
func (a *Armory) SocketSlotGem(ctx context.Context, uid int64, slot, item string, idx int, k GemKey) (Inventory, error) {
	if err := checkSocketArgs(slot, item, idx); err != nil {
		return Inventory{}, err
	}
	if !k.valid() {
		return Inventory{}, ErrBadGem
	}
	return a.withInv(ctx, uid, func(tx *wallet.Tx, inv Inventory) error {
		s, _ := itemSlot(slot)
		key, lv, err := socketOwner(inv, s, item)
		switch {
		case err != nil:
			return err
		case idx >= OpenSockets(lv):
			return ErrLocked
		case inv.Gems[k] <= 0:
			return ErrNoGem
		}
		if err := unsocket(ctx, tx, uid, key, idx, inv.Sockets[key][idx]); err != nil {
			return err
		}
		if err := addGems(ctx, tx, uid, k, -1); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO arena_sockets (user_id, arm, idx, kind, tier) VALUES (?, ?, ?, ?, ?)`,
			uid, key, idx, k.Kind, k.Tier)
		return err
	})
}

// UnsocketGem tháo ngọc ở ô idx của vũ khí arm về kho (miễn phí).
func (a *Armory) UnsocketGem(ctx context.Context, uid int64, arm string, idx int) (Inventory, error) {
	return a.UnsocketSlotGem(ctx, uid, SlotGun, arm, idx)
}

// UnsocketSlotGem tháo ngọc ở ô idx của món item (ô slot; item rỗng = nón / áo đang dùng) về kho.
func (a *Armory) UnsocketSlotGem(ctx context.Context, uid int64, slot, item string, idx int) (Inventory, error) {
	if err := checkSocketArgs(slot, item, idx); err != nil {
		return Inventory{}, err
	}
	return a.withInv(ctx, uid, func(tx *wallet.Tx, inv Inventory) error {
		s, _ := itemSlot(slot)
		key, _, err := socketOwner(inv, s, item)
		if err != nil {
			return err
		}
		k := inv.Sockets[key][idx]
		if !k.valid() {
			return ErrEmptySlot
		}
		return unsocket(ctx, tx, uid, key, idx, k)
	})
}

// GrantLoot cộng ngọc rơi và xu thưởng cho người chơi (thắng boss).
func (a *Armory) GrantLoot(ctx context.Context, uid int64, gems []GemKey, coins int64, note string) error {
	tx, err := a.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, k := range gems {
		if !k.valid() {
			return errBadGemRows
		}
		if err := addGems(ctx, tx, uid, k, 1); err != nil {
			return err
		}
	}
	if coins > 0 {
		if err := tx.Credit(uid, coins, GameID, note); err != nil {
			return err
		}
	}
	return tx.Commit()
}
