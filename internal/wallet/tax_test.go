package wallet

import "testing"

func TestGameTax(t *testing.T) {
	for _, c := range []struct{ win, tax int64 }{{-500, 0}, {0, 0}, {19, 0}, {20, 1}, {150, 7}, {1000, 50}, {1_000_000, 50_000}} {
		if got := GameTax(c.win); got != c.tax {
			t.Fatalf("GameTax(%d) = %d, muốn %d", c.win, got, c.tax)
		}
	}
	got := Taxes(map[int64]int64{1: 200, 2: -200, 3: 10})
	if len(got) != 1 || got[1] != 10 {
		t.Fatalf("Taxes: %v", got)
	}
	if n := TaxNote("Cờ Caro bàn AB"); n != "Thuế 5% tiền thắng — Cờ Caro bàn AB" {
		t.Fatalf("TaxNote: %q", n)
	}
}

func TestSettleTaxed(t *testing.T) {
	w := open(t)
	a, _ := register(t, w, "thang")
	b, _ := register(t, w, "thua")
	bal, tax, err := SettleTaxed(ctx, w, "test", "Ván thử", map[int64]int64{a.ID: 200, b.ID: -200})
	if err != nil || tax[a.ID] != 10 || len(tax) != 1 || bal[a.ID] != StartBalance+190 || bal[b.ID] != StartBalance-200 {
		t.Fatalf("SettleTaxed: %v %v %v", bal, tax, err)
	}
	h, _ := w.History(ctx, a.ID, 2)
	if h[0].Delta != -10 || h[0].Note != "Thuế 5% tiền thắng — Ván thử" || h[1].Delta != 200 || h[1].Note != "Ván thử" {
		t.Fatalf("sổ cái: %+v", h)
	}
}
