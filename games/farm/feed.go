package farm

import (
	"context"
	"fmt"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Cho vật nuôi / cá ăn ----------

// FeedCooldown là số giây sau lần ăn gần nhất (hoặc lúc mua) thì con vật đói lại.
const FeedCooldown = 600

// feedPrice là giá một lần cho ăn: 1/10 giá mua, ít nhất 5 xu.
func (s Stock) feedPrice() int64 { return max(5, s.Price/10) }

// hungry: con chưa lớn và đã qua FeedCooldown kể từ lần ăn gần nhất (chưa ăn lần nào thì tính từ lúc mua).
func (a Animal) hungry(now int64) bool {
	return a.ReadyAt > now && now-max(a.FedAt, a.BoughtAt) >= FeedCooldown
}

// stockJSON là một loại vật nuôi / cá trong catalog, kèm giá thức ăn.
type stockJSON struct {
	Stock
	FeedPrice int64 `json:"feedPrice"`
}

func withFeed(list []Stock) []stockJSON {
	out := make([]stockJSON, len(list))
	for i, s := range list {
		out[i] = stockJSON{s, s.feedPrice()}
	}
	return out
}

// columns là các cột thêm sau vào farm_stock; database từ bản cũ được ALTER TABLE thêm vào khi khởi động.
var columns = []struct{ name, def string }{
	{"fed_at", "INTEGER NOT NULL DEFAULT 0"},
	{"produced_at", "INTEGER NOT NULL DEFAULT 0"},
}

// migrate nâng cấp bảng từ bản cũ: thêm cột mới (CREATE TABLE IF NOT EXISTS không thêm cột), chuyển gà vịt ra sân.
func migrate(ctx context.Context, w *wallet.Wallet) error {
	tx, err := w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT name FROM pragma_table_info('farm_stock')`)
	if err != nil {
		return err
	}
	has := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		has[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range columns {
		if has[c.name] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `ALTER TABLE farm_stock ADD COLUMN `+c.name+` `+c.def); err != nil {
			return err
		}
	}
	// Gà vịt từ bản cũ nuôi trong chuồng → chuyển ra sân gia cầm; sân đủ chỗ cho cả đàn (tối đa MaxCapacity).
	yard := Buildings[placeYard]
	if _, err := tx.ExecContext(ctx, `UPDATE farm_stock SET place = ? WHERE place = ? AND kind IN ('chicken', 'duck')`,
		placeYard, placeBarn); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO farm_buildings (user_id, place, capacity)
		SELECT user_id, ?, MIN(?, MAX(?, COUNT(*))) FROM farm_stock WHERE place = ? GROUP BY user_id`,
		placeYard, yard.MaxCapacity, yard.Capacity, placeYard); err != nil {
		return err
	}
	return tx.Commit()
}

// FeedStock cho một con đang đói ăn: trừ tiền thức ăn, rút ngắn 25% thời gian còn lại tới lúc lớn, +1 XP.
func (f *Farm) FeedStock(ctx context.Context, uid int64, place string, id int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		_, a, st, err := findAnimal(s, place, id)
		if err != nil {
			return err
		}
		now := f.now().Unix()
		if a.ReadyAt <= now {
			return conflict(st.Name + " đã lớn, hãy thu hoạch")
		}
		if !a.hungry(now) {
			left := max(a.FedAt, a.BoughtAt) + FeedCooldown - now
			return conflict(fmt.Sprintf("%s chưa đói, cho ăn lại sau %s", st.Name, fmtWait(left)))
		}
		if err := tx.Charge(uid, st.feedPrice(), GameID, "Cho "+st.Name+" ăn"); err != nil {
			return err
		}
		g = Gained{XP: 1}
		return fed(ctx, tx, uid, a, now, 1)
	})
	return s, g, err
}

// fed ghi lần cho ăn của một con (đã trả tiền): rút ngắn 25% thời gian còn lại, cộng xp.
func fed(ctx context.Context, tx *wallet.Tx, uid int64, a Animal, now, xp int64) error {
	if err := tx.AddXP(uid, xp); err != nil {
		return err
	}
	ready := a.ReadyAt - (a.ReadyAt-now)/4
	_, err := tx.ExecContext(ctx, `UPDATE farm_stock SET ready_at = ?, fed_at = ? WHERE id = ? AND user_id = ?`, ready, now, a.ID, uid)
	return err
}

// fmtWait ghi số giây chờ kiểu "4 phút 10 giây".
func fmtWait(sec int64) string {
	if sec >= 60 {
		if sec%60 == 0 {
			return fmt.Sprintf("%d phút", sec/60)
		}
		return fmt.Sprintf("%d phút %d giây", sec/60, sec%60)
	}
	return fmt.Sprintf("%d giây", sec)
}
