package farm

import (
	"context"

	"github.com/dbphx/game-center/internal/wallet"
)

// Cân bằng kinh tế 2026-11: vật nuôi và cá lớn lâu hơn, sản phẩm ra thưa hơn. Con đã lớn (hoặc sản phẩm đã chờ lấy)
// theo thời gian cũ phải vẫn lớn (vẫn chờ lấy) sau khi cập nhật; con đang lớn dở giữ số giây đã lớn.

// oldGrowSeconds là thời gian lớn trước đợt cân bằng.
var oldGrowSeconds = map[string]int64{
	"chicken": 900, "duck": 1200, "pig": 3600, "sheep": 7200, "cow": 10800,
	"tilapia": 600, "catfish": 1800, "carp": 3600, "koi": 14400,
}

// oldProductEvery là chu kỳ sản phẩm trước đợt cân bằng.
var oldProductEvery = map[string]int64{"chicken": 600, "duck": 900, "sheep": 1800, "cow": 1200}

const rebalanceMigration = "rebalance_2026_11_stock"

// migrateRebalance chạy đúng một lần (đánh dấu trong farm_migrations).
func migrateRebalance(ctx context.Context, w *wallet.Wallet) error {
	tx, err := w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS farm_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO farm_migrations (name) VALUES (?)`, rebalanceMigration)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return nil // đã chạy
	}
	for _, place := range []string{placeYard, placeBarn, placePond} {
		for _, st := range stockOf(place) {
			old, ok := oldGrowSeconds[st.ID]
			if !ok || st.GrowSeconds <= old {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE farm_stock SET grow_progress = ? WHERE kind = ? AND grow_progress >= ? AND grow_progress < ?`,
				st.GrowSeconds, st.ID, old, st.GrowSeconds); err != nil {
				return err
			}
			p := yields[st.ID].Product
			oldEvery, ok := oldProductEvery[st.ID]
			if p == nil || !ok || p.Every <= oldEvery {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE farm_stock SET product_progress = ? WHERE kind = ? AND product_progress >= ? AND product_progress < ?`,
				p.Every, st.ID, oldEvery, p.Every); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
