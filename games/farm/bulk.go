package farm

import (
	"context"
	"fmt"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Làm nhanh hàng loạt: mỗi thao tác một transaction ----------

// balance đọc số dư trong transaction để biết mua được bao nhiêu trước khi trừ một lần.
func balance(ctx context.Context, tx *wallet.Tx, uid int64) (int64, error) {
	var b int64
	err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, uid).Scan(&b)
	return b, err
}

// PlantAll gieo một loại hạt vào mọi ô trống: dùng hạt trong kho trước, thiếu thì mua (tới khi hết xu).
func (f *Farm) PlantAll(ctx context.Context, uid int64, cropID string) (State, Gained, error) {
	c, ok := findCrop(cropID)
	if !ok {
		return State{}, Gained{}, bad("không có loại cây này")
	}
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		empty := []int{}
		for _, p := range s.Plots {
			if p.Crop == "" {
				empty = append(empty, p.Index)
			}
		}
		if len(empty) == 0 {
			return conflict("không còn ô đất trống")
		}
		if err := requireLevel(tx, uid, c.Level); err != nil {
			return err
		}
		fromStore := min(s.Storage.Seeds.Items[c.ID], len(empty))
		bal, err := balance(ctx, tx, uid)
		if err != nil {
			return err
		}
		buy := min(int64(len(empty)-fromStore), bal/c.SeedPrice)
		n := fromStore + int(buy)
		if n == 0 {
			return conflict(fmt.Sprintf("hết hạt %s và không đủ xu mua hạt (%d xu/hạt)", c.Name, c.SeedPrice))
		}
		if buy > 0 {
			g.Spent = buy * c.SeedPrice
			if err := tx.Charge(uid, g.Spent, GameID, fmt.Sprintf("Mua %d hạt %s", buy, c.Name)); err != nil {
				return err
			}
		}
		if err := addItem(ctx, tx, uid, storeSeeds, c.ID, -fromStore); err != nil {
			return err
		}
		now := f.now().Unix()
		for _, idx := range empty[:n] {
			if _, err := tx.ExecContext(ctx, `UPDATE farm_plots SET crop = ?, planted_at = ?, ready_at = ? WHERE user_id = ? AND idx = ?`,
				c.ID, now, now+c.GrowSeconds, uid, idx); err != nil {
				return err
			}
		}
		g.Count = n
		return nil
	})
	return s, g, err
}

// FeedAll cho mọi con đang đói / sắp đói mà còn cần ăn (chưa lớn, hoặc đang chờ ra sản phẩm) ở chuồng, sân, ao ăn, trừ thức ăn trong kho giống tới khi hết.
func (f *Farm) FeedAll(ctx context.Context, uid int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		now := f.now().Unix()
		g.Used = map[string]int{}
		hungry := 0
		for _, place := range []string{placeBarn, placeYard, placePond} {
			pen := penOf(s, place)
			if pen == nil {
				continue
			}
			fd := feedFor(place)
			for _, a := range append(pen.Animals, pen.Fish...) {
				if !a.canFeed(now) || !a.needsFood() {
					continue
				}
				hungry++
				n := portions(a.Kind)
				if s.Storage.Seeds.Items[fd.ID]-g.Used[fd.ID] < n {
					continue
				}
				g.Used[fd.ID] += n
				g.Count++
				g.XP++
				if err := fed(ctx, tx, uid, a, now, 1); err != nil {
					return err
				}
			}
		}
		if hungry == 0 {
			return conflict("không có con nào đang đói")
		}
		if g.Count == 0 {
			return conflict("Hết thức ăn, mua thêm ở cửa hàng")
		}
		for id, n := range g.Used {
			if err := addItem(ctx, tx, uid, storeSeeds, id, -n); err != nil {
				return err
			}
		}
		return nil
	})
	return s, g, err
}

// ProduceAll lấy mọi trứng / sữa / lông đã có (chuồng và sân) vào kho, tới khi kho đầy.
func (f *Farm) ProduceAll(ctx context.Context, uid int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		now := f.now().Unix()
		free := s.Storage.Products.free()
		g.Items = map[string]int{}
		ready := 0
		for _, pen := range []*Pen{s.Barn, s.Yard} {
			if pen == nil {
				continue
			}
			for _, a := range pen.Animals {
				if !a.ProductReady {
					continue
				}
				ready++
				if g.Count >= free {
					continue
				}
				p := yields[a.Kind].Product
				if err := produced(ctx, tx, uid, a.ID, p, now); err != nil {
					return err
				}
				g.Count++
				g.XP += p.XP
				g.Items[p.ID]++
			}
		}
		if ready == 0 {
			return conflict("chưa có trứng / sữa / lông nào để lấy")
		}
		if g.Count == 0 {
			return productsFull(s.Storage.Products)
		}
		return nil
	})
	return s, g, err
}

// CollectAllFish thu mọi con cá đã lớn vào kho sản phẩm, tới khi kho đầy.
func (f *Farm) CollectAllFish(ctx context.Context, uid int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		if s.Pond == nil {
			return missing(placePond)
		}
		free := s.Storage.Products.free()
		g.Items = map[string]int{}
		ready := 0
		for _, a := range s.Pond.Fish {
			if !a.Grown {
				continue
			}
			ready++
			if g.Count >= free {
				continue
			}
			st, _ := findStock(placePond, a.Kind)
			if _, err := tx.ExecContext(ctx, `DELETE FROM farm_stock WHERE id = ? AND user_id = ?`, a.ID, uid); err != nil {
				return err
			}
			g.Count++
			g.XP += st.XP
			g.Items[st.ID]++
		}
		if ready == 0 {
			return conflict("chưa có con cá nào lớn")
		}
		if g.Count == 0 {
			return productsFull(s.Storage.Products)
		}
		for item, n := range g.Items {
			if err := addItem(ctx, tx, uid, storeProducts, item, n); err != nil {
				return err
			}
		}
		return tx.AddXP(uid, g.XP)
	})
	return s, g, err
}
