package race

import (
	"context"
	"fmt"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Phụ kiện nâng cấp ----------
//
// Nâng một món (động cơ / turbo / lốp) lên cấp n cần n phụ kiện đúng loại + xu (upgradePrice). Phụ kiện không bán,
// chỉ nhận từ nhiệm vụ ở NPC nông trại (gói quests gọi GrantParts).

const partsSchema = `
CREATE TABLE IF NOT EXISTS race_parts (
	user_id INTEGER NOT NULL REFERENCES users(id),
	part    TEXT NOT NULL,
	qty     INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, part)
);
`

// Mã phụ kiện.
const (
	PartPiston = "piston"
	PartTurbo  = "turbo_kit"
	PartTire   = "tire_kit"
)

// PartItem là một loại phụ kiện; For là món nâng cấp dùng nó.
type PartItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	For  string `json:"for"` // engine / turbo / tires
}

// PartItems theo thứ tự món nâng cấp (Parts).
var PartItems = []PartItem{
	{PartPiston, "Piston", "engine"},
	{PartTurbo, "Bộ tăng áp", "turbo"},
	{PartTire, "Lốp đua", "tires"},
}

// PartFor trả phụ kiện của món nâng cấp up (engine / turbo / tires).
func PartFor(up string) PartItem {
	for _, p := range PartItems {
		if p.For == up {
			return p
		}
	}
	return PartItem{}
}

// FindPart tìm phụ kiện theo mã.
func FindPart(id string) (PartItem, bool) {
	for _, p := range PartItems {
		if p.ID == id {
			return p, true
		}
	}
	return PartItem{}, false
}

// partNeed là số phụ kiện để lên cấp toLevel.
func partNeed(toLevel int) int { return toLevel }

// PartQty là số phụ kiện người chơi đang có (đủ 3 loại, kể cả 0).
func PartQty(ctx context.Context, tx *wallet.Tx, uid int64) (map[string]int, error) {
	out := map[string]int{}
	for _, p := range PartItems {
		out[p.ID] = 0
	}
	rows, err := tx.QueryContext(ctx, `SELECT part, qty FROM race_parts WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		if _, ok := out[id]; ok {
			out[id] = n
		}
	}
	return out, rows.Err()
}

// GrantParts tặng n phụ kiện (n > 0) trong giao dịch tx — dùng cho phần thưởng nhiệm vụ.
func GrantParts(ctx context.Context, tx *wallet.Tx, uid int64, part string, n int) error {
	if _, ok := FindPart(part); !ok {
		return notFound("không có phụ kiện này")
	}
	if n <= 0 {
		return bad("số phụ kiện phải dương")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO race_parts (user_id, part, qty) VALUES (?, ?, ?)
		ON CONFLICT(user_id, part) DO UPDATE SET qty = qty + excluded.qty`, uid, part, n)
	return err
}

// useParts tiêu need phụ kiện; thiếu → 409 "Cần N <tên> (có M)". Câu UPDATE có điều kiện qty >= need nên hai yêu cầu
// cùng lúc không tiêu quá số đang có.
func useParts(ctx context.Context, tx *wallet.Tx, uid int64, p PartItem, need int) error {
	r, err := tx.ExecContext(ctx, `UPDATE race_parts SET qty = qty - ? WHERE user_id = ? AND part = ? AND qty >= ?`, need, uid, p.ID, need)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 1 {
		return nil
	}
	have := 0
	tx.QueryRowContext(ctx, `SELECT qty FROM race_parts WHERE user_id = ? AND part = ?`, uid, p.ID).Scan(&have)
	return conflict(fmt.Sprintf("Cần %d %s (có %d)", need, p.Name, have))
}
