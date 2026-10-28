package wallet

import "context"

// Asset là phần tài sản ví nắm được của một người chơi: xu trong ví và đồ thời trang đã mua (tính theo giá shop),
// kèm tên hiển thị và ngoại hình để hiện ở bảng xếp hạng (không có tên đăng nhập).
type Asset struct {
	ID     int64
	Name   string
	Level  int
	Gender string
	Outfit string
	Hair   string
	Wings  string
	Face   string
	Coins  int64
	Items  int64 // tổng giá shop của quần áo, tóc, cánh, gương mặt đã có (món miễn phí / không còn bán tính 0)
}

// Assets đọc tài sản của mọi người chơi trong ba câu SQL (không truy vấn theo từng người).
func (w *Wallet) Assets(ctx context.Context) ([]Asset, error) {
	rows, err := w.db.QueryContext(ctx, `SELECT id, name, balance, xp, gender, outfit, hair, wings, face FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	list := []Asset{}
	at := map[int64]int{}
	for rows.Next() {
		var a Asset
		var xp int64
		if err := rows.Scan(&a.ID, &a.Name, &a.Coins, &xp, &a.Gender, &a.Outfit, &a.Hair, &a.Wings, &a.Face); err != nil {
			rows.Close()
			return nil, err
		}
		a.Level = LevelFor(xp)
		at[a.ID] = len(list)
		list = append(list, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	add := func(uid, price int64) {
		if i, ok := at[uid]; ok {
			list[i].Items += price
		}
	}

	// Quần áo: bảng user_outfits có cả món starter (giá 0).
	rows, err = w.db.QueryContext(ctx, `SELECT user_id, outfit FROM user_outfits`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uid int64
		var id string
		if err := rows.Scan(&uid, &id); err != nil {
			rows.Close()
			return nil, err
		}
		if o, ok := outfitInfo(id); ok {
			add(uid, o.Price)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Tóc / cánh / gương mặt đã mua.
	rows, err = w.db.QueryContext(ctx, `SELECT user_id, kind, item FROM user_items`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid int64
		var kind, id string
		if err := rows.Scan(&uid, &kind, &id); err != nil {
			return nil, err
		}
		if it, ok := itemInfo(kind, id); ok {
			add(uid, it.Price)
		}
	}
	return list, rows.Err()
}
