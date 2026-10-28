package farm

import "context"

// Worth trả giá trị nông trại (xu) của mọi người chơi có nông trại, tính theo giá trong game:
//   - ô đất đã mua: tổng giá đã trả (6 ô miễn phí tính 0), cây đang trồng tính giá hạt;
//   - chuồng / ao: giá mua + các lần mở rộng đã trả (sân gia cầm có sẵn, chỉ tính mở rộng); mở rộng kho cũng vậy;
//   - vật nuôi / cá đang nuôi: giá mua (không tính lớn thêm để giá trị không nhảy theo đồng hồ);
//   - kho giống: hạt và thức ăn theo giá mua; kho sản phẩm: theo giá bán.
//
// Chỉ đọc (transaction bị huỷ cuối hàm), mỗi bảng một câu SQL cho mọi người chơi.
func (f *Farm) Worth(ctx context.Context) (map[int64]int64, error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	worth := map[int64]int64{}

	// Ô đất: đếm số ô của từng người, cộng hạt đang gieo.
	plots := map[int64]int{}
	rows, err := tx.QueryContext(ctx, `SELECT user_id, crop FROM farm_plots`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uid int64
		var crop string
		if err := rows.Scan(&uid, &crop); err != nil {
			rows.Close()
			return nil, err
		}
		plots[uid]++
		if c, ok := findCrop(crop); ok {
			worth[uid] += c.SeedPrice
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for uid, n := range plots {
		for have := FreePlots; have < n; have++ {
			worth[uid] += plotPrice(have)
		}
	}

	// Chuồng / sân / ao.
	rows, err = tx.QueryContext(ctx, `SELECT user_id, place, expansions FROM farm_buildings`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uid int64
		var place string
		var exp int
		if err := rows.Scan(&uid, &place, &exp); err != nil {
			rows.Close()
			return nil, err
		}
		b, ok := Buildings[place]
		if !ok {
			continue
		}
		worth[uid] += b.Price
		for i := range exp {
			worth[uid] += b.expandPrice(i)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Mở rộng kho giống / kho sản phẩm.
	rows, err = tx.QueryContext(ctx, `SELECT user_id, store, expansions FROM farm_storage`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uid int64
		var store string
		var exp int
		if err := rows.Scan(&uid, &store, &exp); err != nil {
			rows.Close()
			return nil, err
		}
		spec, ok := Stores[store]
		if !ok {
			continue
		}
		for i := range exp {
			worth[uid] += spec.expandPrice(i)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Vật nuôi / cá đang nuôi.
	rows, err = tx.QueryContext(ctx, `SELECT user_id, place, kind FROM farm_stock`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uid int64
		var place, kind string
		if err := rows.Scan(&uid, &place, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		if st, ok := findStock(place, kind); ok {
			worth[uid] += st.Price
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Đồ trong kho.
	rows, err = tx.QueryContext(ctx, `SELECT user_id, store, item, qty FROM farm_items WHERE qty > 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid, qty int64
		var store, item string
		if err := rows.Scan(&uid, &store, &item, &qty); err != nil {
			return nil, err
		}
		worth[uid] += qty * itemValue(store, item)
	}
	return worth, rows.Err()
}

// itemValue là giá một đơn vị trong kho: kho giống theo giá mua (hạt / thức ăn), kho sản phẩm theo giá bán.
func itemValue(store, item string) int64 {
	if store == storeSeeds {
		if c, ok := findCrop(item); ok {
			return c.SeedPrice
		}
		if fd, ok := findFeed(item); ok {
			return fd.Price
		}
		return 0
	}
	_, price, _ := product(item)
	return price
}
