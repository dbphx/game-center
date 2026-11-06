package farm

import (
	"strings"
	"testing"
	"time"
)

// Thuế bán: nông sản (cây trồng, cây mây) 10%, đồ từ vật nuôi / cá và con vật 5%; làm tròn xuống trên tổng từng món.
func TestSaleTax(t *testing.T) {
	cases := []struct {
		item  string
		gross int64
		tax   int64
	}{
		{"carrot", 90, 9},
		{"carrot", 99, 9}, // làm tròn xuống
		{"fairy_peach", 600_000, 60_000},
		{"egg", 60, 3},
		{"meat_cow", 3400, 170},
		{"tilapia", 219, 10},
		{"wool", 19, 0},
	}
	for _, c := range cases {
		if got := saleTax(c.item, c.gross); got != c.tax {
			t.Errorf("thuế %s trên %d: %d, muốn %d", c.item, c.gross, got, c.tax)
		}
	}
	cat := Catalog()
	if cat["cropTax"] != CropTaxPercent || cat["stockTax"] != StockTaxPercent || CropTaxPercent != 10 || StockTaxPercent != 5 {
		t.Fatalf("catalog thuế: %v %v", cat["cropTax"], cat["stockTax"])
	}
}

func TestSellProductTaxed(t *testing.T) {
	e := newEnv(t, 0, 0)
	e.ok(e.f.State(ctx, e.uid))
	e.giveItem(e.uid, storeProducts, "carrot", 3) // 3 × 90 = 270, thuế 27
	e.giveItem(e.uid, storeProducts, "egg", 7)    // 7 × 60 = 420, thuế 21
	e.giveItem(e.uid, storeProducts, "fairy_peach", 1)

	bal := e.user().Balance
	if _, g, err := e.f.SellProduct(ctx, e.uid, "carrot", 3); err != nil || g.Coins != 243 || g.Tax != 27 || e.user().Balance != bal+243 {
		t.Fatalf("bán cà rốt: %+v %v", g, err)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Bán 3 Cà rốt, thuế 27 xu" || h[0].Delta != 243 {
		t.Fatalf("sổ cái: %+v", h[0])
	}
	bal = e.user().Balance
	if _, g, err := e.f.SellProduct(ctx, e.uid, "egg", 1); err != nil || g.Coins != 57 || g.Tax != 3 || e.user().Balance != bal+57 {
		t.Fatalf("bán 1 trứng: %+v %v", g, err)
	}
	// Bán tất cả: 6 trứng (360, thuế 18) + 1 đào tiên (600.000, thuế 60.000), thuế tính trên tổng từng món.
	bal = e.user().Balance
	_, g, err := e.f.SellAllProducts(ctx, e.uid)
	if err != nil || g.Tax != 60_018 || g.Coins != 360+600_000-60_018 || e.user().Balance != bal+g.Coins {
		t.Fatalf("bán tất cả: %+v %v", g, err)
	}
	h, _ = e.w.History(ctx, e.uid, 1)
	if !strings.HasSuffix(h[0].Note, ", thuế 60.018 xu") {
		t.Fatalf("sổ cái bán tất cả: %+v", h[0])
	}
}

func TestSellStockTaxed(t *testing.T) {
	e := newEnv(t, 100_000, levelXP(2))
	e.ok(e.f.BuyBuilding(ctx, e.uid, placeBarn))
	s := e.ok(e.f.BuyStock(ctx, e.uid, placeBarn, "pig"))
	id := s.Barn.Animals[0].ID
	e.wellFed()
	e.clock = e.clock.Add(time.Hour)
	before := e.user()
	_, g, err := e.f.SellStock(ctx, e.uid, placeBarn, id) // heo 1.300, thuế 65
	if err != nil || g.Coins != 1235 || g.Tax != 65 || g.XP != 12 || e.user().Balance != before.Balance+1235 {
		t.Fatalf("bán heo: %+v %v", g, err)
	}
	h, _ := e.w.History(ctx, e.uid, 1)
	if h[0].Note != "Bán Heo, thuế 65 xu" {
		t.Fatalf("sổ cái: %+v", h[0])
	}
}
