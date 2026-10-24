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

// migrate thêm cột mới vào bảng đã có từ bản cũ (CREATE TABLE IF NOT EXISTS không thêm cột).
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
	has := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		has = has || name == "fed_at"
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if !has {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE farm_stock ADD COLUMN fed_at INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FeedStock cho một con đang đói ăn: trừ tiền thức ăn, rút ngắn 25% thời gian còn lại tới lúc lớn, +1 XP.
func (f *Farm) FeedStock(ctx context.Context, uid int64, place string, id int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		pen := penOf(s, place)
		if pen == nil {
			return missing(place)
		}
		list := pen.Animals
		if place == placePond {
			list = pen.Fish
		}
		now := f.now().Unix()
		for _, a := range list {
			if a.ID != id {
				continue
			}
			st, _ := findStock(place, a.Kind)
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
			if err := tx.AddXP(uid, 1); err != nil {
				return err
			}
			g = Gained{XP: 1}
			ready := a.ReadyAt - (a.ReadyAt-now)/4
			_, err := tx.ExecContext(ctx, `UPDATE farm_stock SET ready_at = ?, fed_at = ? WHERE id = ? AND user_id = ?`, ready, now, id, uid)
			return err
		}
		if place == placeBarn {
			return notFound("không tìm thấy con vật này")
		}
		return notFound("không tìm thấy con cá này")
	})
	return s, g, err
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
