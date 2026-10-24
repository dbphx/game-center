package farm

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Kho giống và kho sản phẩm ----------

// Store là một kho của người chơi; Items chỉ gồm món còn hàng (id → số lượng).
type Store struct {
	Capacity        int            `json:"capacity"`
	Used            int            `json:"used"`
	MaxCapacity     int            `json:"maxCapacity"`
	NextExpandPrice int64          `json:"nextExpandPrice"`
	Items           map[string]int `json:"items"`
	expansions      int
}

type Storage struct {
	Seeds    *Store `json:"seeds"`
	Products *Store `json:"products"`
}

func (s *Store) free() int { return s.Capacity - s.Used }

func productsFull(s *Store) error {
	return conflict(fmt.Sprintf("kho sản phẩm đầy (%d/%d), hãy bán bớt hoặc mở rộng kho", s.Used, s.Capacity))
}

// loadStore đọc một kho, tạo kho mặc định nếu người chơi chưa có.
func loadStore(ctx context.Context, q querier, uid int64, store string) (*Store, error) {
	spec := Stores[store]
	if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO farm_storage (user_id, store, capacity) VALUES (?, ?, ?)`,
		uid, store, spec.Capacity); err != nil {
		return nil, err
	}
	st := &Store{MaxCapacity: spec.MaxCapacity, Items: map[string]int{}}
	if err := q.QueryRowContext(ctx, `SELECT capacity, expansions FROM farm_storage WHERE user_id = ? AND store = ?`, uid, store).
		Scan(&st.Capacity, &st.expansions); err != nil {
		return nil, err
	}
	if st.Capacity < spec.MaxCapacity {
		st.NextExpandPrice = spec.expandPrice(st.expansions)
	}
	rows, err := q.QueryContext(ctx, `SELECT item, qty FROM farm_items WHERE user_id = ? AND store = ? AND qty > 0`, uid, store)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item string
		var n int
		if err := rows.Scan(&item, &n); err != nil {
			return nil, err
		}
		st.Items[item] = n
		st.Used += n
	}
	return st, rows.Err()
}

// addItem cộng (delta > 0) hoặc trừ (delta < 0) số lượng một món trong kho.
func addItem(ctx context.Context, tx *wallet.Tx, uid int64, store, item string, delta int) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO farm_items (user_id, store, item, qty) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id, store, item) DO UPDATE SET qty = qty + excluded.qty`, uid, store, item, delta); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM farm_items WHERE user_id = ? AND store = ? AND item = ? AND qty <= 0`, uid, store, item)
	return err
}

func storeOf(s State, store string) *Store {
	if store == storeSeeds {
		return s.Storage.Seeds
	}
	return s.Storage.Products
}

// BuySeeds mua qty hạt giống (1..100) cất vào kho giống.
func (f *Farm) BuySeeds(ctx context.Context, uid int64, cropID string, qty int) (State, error) {
	c, ok := findCrop(cropID)
	if !ok {
		return State{}, bad("không có loại cây này")
	}
	if qty == 0 {
		qty = 1
	}
	if qty < 0 || qty > 100 {
		return State{}, bad("mỗi lần mua từ 1 đến 100 hạt")
	}
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		seeds := s.Storage.Seeds
		if seeds.free() < qty {
			return conflict(fmt.Sprintf("kho giống đầy (%d/%d), còn chỗ cho %d hạt", seeds.Used, seeds.Capacity, max(0, seeds.free())))
		}
		if err := requireLevel(tx, uid, c.Level); err != nil {
			return err
		}
		if err := tx.Charge(uid, c.SeedPrice*int64(qty), GameID, fmt.Sprintf("Mua %d hạt %s", qty, c.Name)); err != nil {
			return err
		}
		return addItem(ctx, tx, uid, storeSeeds, c.ID, qty)
	})
}

// ExpandStore mở rộng kho giống (store = "seeds") hoặc kho sản phẩm ("products").
func (f *Farm) ExpandStore(ctx context.Context, uid int64, store string) (State, error) {
	spec := Stores[store]
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		st := storeOf(s, store)
		if st.Capacity >= spec.MaxCapacity {
			return conflict(fmt.Sprintf("%s đã tối đa %d chỗ", spec.Name, spec.MaxCapacity))
		}
		if err := tx.Charge(uid, spec.expandPrice(st.expansions), GameID, "Mở rộng "+spec.Name); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE farm_storage SET capacity = ?, expansions = expansions + 1 WHERE user_id = ? AND store = ?`,
			min(spec.MaxCapacity, st.Capacity+spec.ExpandUnits), uid, store)
		return err
	})
}

// CollectStock thu một con đã lớn từ chuồng / ao vào kho sản phẩm, cộng XP.
func (f *Farm) CollectStock(ctx context.Context, uid int64, place string, id int64) (State, Gained, error) {
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
		for _, a := range list {
			if a.ID != id {
				continue
			}
			st, _ := findStock(place, a.Kind)
			if a.ReadyAt > f.now().Unix() {
				return conflict(st.Name + " chưa lớn")
			}
			if s.Storage.Products.free() < 1 {
				return productsFull(s.Storage.Products)
			}
			if err := addItem(ctx, tx, uid, storeProducts, st.ID, 1); err != nil {
				return err
			}
			if err := tx.AddXP(uid, st.XP); err != nil {
				return err
			}
			g = Gained{XP: st.XP, Count: 1, Items: map[string]int{st.ID: 1}}
			_, err := tx.ExecContext(ctx, `DELETE FROM farm_stock WHERE id = ? AND user_id = ?`, id, uid)
			return err
		}
		if place == placeBarn {
			return notFound("không tìm thấy con vật này")
		}
		return notFound("không tìm thấy con cá này")
	})
	return s, g, err
}

// SellProduct bán qty món item trong kho sản phẩm (qty = 0: bán hết món đó). Chỉ cộng xu, XP đã cộng lúc thu hoạch.
func (f *Farm) SellProduct(ctx context.Context, uid int64, item string, qty int) (State, Gained, error) {
	name, price, ok := product(item)
	if !ok {
		return State{}, Gained{}, bad("không có sản phẩm này")
	}
	if qty < 0 {
		return State{}, Gained{}, bad("số lượng không hợp lệ")
	}
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		have := s.Storage.Products.Items[item]
		if qty == 0 {
			qty = have
		}
		if have == 0 {
			return conflict("trong kho không có " + name)
		}
		if qty > have {
			return conflict(fmt.Sprintf("trong kho chỉ có %d %s", have, name))
		}
		g = Gained{Coins: price * int64(qty), Count: qty}
		if err := tx.Credit(uid, g.Coins, GameID, fmt.Sprintf("Bán %d %s", qty, name)); err != nil {
			return err
		}
		return addItem(ctx, tx, uid, storeProducts, item, -qty)
	})
	return s, g, err
}

// SellAllProducts bán toàn bộ kho sản phẩm, ghi một dòng sổ cái.
func (f *Farm) SellAllProducts(ctx context.Context, uid int64) (State, Gained, error) {
	var g Gained
	s, err := f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		items := s.Storage.Products.Items
		if len(items) == 0 {
			return conflict("kho sản phẩm đang trống")
		}
		names := make([]string, 0, len(items))
		for item, n := range items {
			name, price, ok := product(item)
			if !ok {
				continue // món không còn trong danh mục: để nguyên
			}
			g.Coins += price * int64(n)
			g.Count += n
			names = append(names, fmt.Sprintf("%d %s", n, name))
			if err := addItem(ctx, tx, uid, storeProducts, item, -n); err != nil {
				return err
			}
		}
		if g.Count == 0 {
			return conflict("kho sản phẩm không có món nào bán được")
		}
		sort.Strings(names)
		return tx.Credit(uid, g.Coins, GameID, "Bán "+strings.Join(names, ", "))
	})
	return s, g, err
}
