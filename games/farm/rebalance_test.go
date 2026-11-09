package farm

import "testing"

// Con đã lớn (sản phẩm đã chờ lấy) theo thời gian cũ vẫn lớn sau đợt cân bằng; con đang lớn dở giữ số giây đã lớn.
// Chỉ chạy một lần: con lớn tới mốc cũ sau khi cập nhật không được đẩy lên.
func TestMigrateRebalanceKeepsGrownStock(t *testing.T) {
	e := newEnv(t, 0, 0)
	add := func(place, kind string, grow, product int64) int64 {
		e.exec(`INSERT INTO farm_stock (user_id, place, kind, bought_at, ready_at, grow_progress, product_progress) VALUES (?, ?, ?, 0, 0, ?, ?)`,
			e.uid, place, kind, grow, product)
		return int64(e.count(`SELECT MAX(id) FROM farm_stock`))
	}
	grownPig := add(placeBarn, "pig", 3600, 0)
	halfPig := add(placeBarn, "pig", 1800, 0)
	hen := add(placeYard, "chicken", 900, 600)
	koi := add(placePond, "koi", 14400, 0)
	e.exec(`DELETE FROM farm_migrations`)
	if _, err := New(ctx, e.w); err != nil {
		t.Fatal(err)
	}
	grow := func(id int64) (int, int) {
		g := e.count(`SELECT grow_progress FROM farm_stock WHERE id = ?`, id)
		p := e.count(`SELECT product_progress FROM farm_stock WHERE id = ?`, id)
		return g, p
	}
	if g, _ := grow(grownPig); g != 14400 {
		t.Fatalf("heo đã lớn phải vẫn lớn: %d", g)
	}
	if g, _ := grow(halfPig); g != 1800 {
		t.Fatalf("heo lớn dở giữ số giây: %d", g)
	}
	if g, p := grow(hen); g != 3600 || p != 1800 {
		t.Fatalf("gà đã lớn, trứng đã chờ: %d %d", g, p)
	}
	if g, _ := grow(koi); g != 43200 {
		t.Fatalf("koi đã lớn: %d", g)
	}
	e.exec(`UPDATE farm_stock SET grow_progress = 3600 WHERE id = ?`, halfPig)
	if _, err := New(ctx, e.w); err != nil {
		t.Fatal(err)
	}
	if g, _ := grow(halfPig); g != 3600 {
		t.Fatalf("chạy lại không được đẩy tiếp: %d", g)
	}
}
