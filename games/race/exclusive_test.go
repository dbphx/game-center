package race

import (
	"net/http"
	"testing"
)

// Xe / đồ độc quyền: chỉ ghép từ mảnh (GrantCar / GrantItem), không mua được; chỉ số xe = hạng Siêu Tân Tinh.
func TestExclusiveCarsAndItems(t *testing.T) {
	e := newGarageEnv(t, 100_000_000)
	nova, _ := findCar("nova")
	for _, id := range []string{"x_aurora", "x_frost"} {
		m, ok := findCar(id)
		if !ok || !m.Exclusive || m.Rank != nova.Rank {
			t.Fatalf("%s: %+v", id, m)
		}
	}
	aurora, _ := findCar("x_aurora")
	if StatsOf(aurora, Levels{}) != StatsOf(nova, Levels{}) {
		t.Fatal("Cực Quang phải bằng chỉ số Siêu Tân Tinh")
	}
	frost, _ := findCar("x_frost")
	if frost.Kind != KindMoto || StatsOf(frost, Levels{}).Accel <= StatsOf(nova, Levels{}).Accel {
		t.Fatal("Băng Phong là moto: tăng tốc hơn ô tô cùng hạng")
	}
	v, _ := e.g.View(e.t.Context(), e.uid)
	if c := carOf(v, "x_aurora"); !c.Exclusive || c.Owned {
		t.Fatalf("garage: %+v", c)
	}
	if it := itemOf(v, "paint_x_chroma"); !it.Exclusive || it.Owned {
		t.Fatalf("sơn độc quyền chưa ghép thì chưa có: %+v", it)
	}
	_, err := e.g.BuyCar(e.t.Context(), e.uid, "x_aurora")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.g.BuyItem(e.t.Context(), e.uid, "neon_x_rainbow")
	wantStatus(t, err, http.StatusConflict)
	_, err = e.g.Equip(e.t.Context(), e.uid, "starter", "paint", "paint_x_chroma")
	wantStatus(t, err, http.StatusConflict)

	tx, _ := e.w.Begin(e.t.Context())
	if err := GrantCar(e.t.Context(), tx, e.uid, "x_aurora"); err != nil {
		t.Fatal(err)
	}
	if err := GrantItem(e.t.Context(), tx, e.uid, "paint_x_chroma"); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, GrantCar(e.t.Context(), tx, e.uid, "x_aurora"), http.StatusConflict)
	wantStatus(t, GrantItem(e.t.Context(), tx, e.uid, "paint_blue"), http.StatusConflict)
	if ok, _ := Owns(e.t.Context(), tx, e.uid, "x_aurora"); !ok {
		t.Fatal("Owns sau khi tặng")
	}
	tx.Commit()
	if _, err := e.g.Equip(e.t.Context(), e.uid, "x_aurora", "paint", "paint_x_chroma"); err != nil {
		t.Fatal(err)
	}
	if v, err = e.g.SelectCar(e.t.Context(), e.uid, "x_aurora"); err != nil || v.Current != "x_aurora" {
		t.Fatalf("chọn xe độc quyền: %v", err)
	}
}
