package race

import (
	"net/http"
	"sync"
	"testing"
)

// grant tặng n phụ kiện part cho người chơi thử.
func (e *garageEnv) grant(part string, n int) {
	e.t.Helper()
	tx, err := e.w.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	defer tx.Rollback()
	if err := GrantParts(ctx, tx, e.uid, part, n); err != nil {
		e.t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

func TestPartCatalog(t *testing.T) {
	want := map[string]string{"engine": PartPiston, "turbo": PartTurbo, "tires": PartTire}
	for up, id := range want {
		if p := PartFor(up); p.ID != id {
			t.Fatalf("phụ kiện của %s: %+v", up, p)
		}
	}
	if len(PartItems) != 3 || PartItems[0].Name != "Piston" || PartItems[1].Name != "Bộ tăng áp" || PartItems[2].Name != "Lốp đua" {
		t.Fatalf("danh sách phụ kiện: %+v", PartItems)
	}
	if n := partNeed(4); n != 4 {
		t.Fatalf("lên cấp 4 cần %d phụ kiện", n)
	}
}

// Nâng cấp n cần n phụ kiện cùng loại + xu; thiếu phụ kiện → 409, không trừ gì.
func TestUpgradeNeedsParts(t *testing.T) {
	e := newGarageEnv(t, 100_000)
	v, err := e.g.View(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if v.Parts[PartPiston] != 0 || v.PartNeed["starter"]["engine"] != 1 || v.PartOf["engine"] != PartPiston || len(v.PartInfo) != 3 {
		t.Fatalf("garage mới: parts %v need %v of %v", v.Parts, v.PartNeed["starter"], v.PartOf)
	}
	if v.PartNeed["comet"]["engine"] != 0 {
		t.Fatal("xe chưa có thì không cần phụ kiện")
	}

	_, err = e.g.Upgrade(ctx, e.uid, "starter", "engine")
	wantStatus(t, err, http.StatusConflict)
	if err.Error() != "Cần 1 Piston (có 0)" || e.balance() != 100_000 {
		t.Fatalf("thiếu phụ kiện: %v, xu %d", err, e.balance())
	}

	e.grant(PartPiston, 2)
	if v, err = e.g.Upgrade(ctx, e.uid, "starter", "engine"); err != nil {
		t.Fatal(err)
	}
	if v.Parts[PartPiston] != 1 || e.balance() != 97_000 || carOf(v, "starter").Levels.Engine != 1 || v.PartNeed["starter"]["engine"] != 2 {
		t.Fatalf("sau nâng cấp 1: parts %v xu %d need %v", v.Parts, e.balance(), v.PartNeed["starter"])
	}
	_, err = e.g.Upgrade(ctx, e.uid, "starter", "engine")
	wantStatus(t, err, http.StatusConflict)
	if err.Error() != "Cần 2 Piston (có 1)" || e.balance() != 97_000 {
		t.Fatalf("cấp 2 thiếu phụ kiện: %v, xu %d", err, e.balance())
	}
	// Phụ kiện khác loại không dùng thay được.
	e.grant(PartTurbo, 5)
	_, err = e.g.Upgrade(ctx, e.uid, "starter", "engine")
	wantStatus(t, err, http.StatusConflict)

	// Thiếu xu thì không mất phụ kiện.
	e.grant(PartPiston, 1)
	e.setBalance(0)
	_, err = e.g.Upgrade(ctx, e.uid, "starter", "engine")
	wantStatus(t, err, http.StatusPaymentRequired)
	if v, _ = e.g.View(ctx, e.uid); v.Parts[PartPiston] != 2 {
		t.Fatalf("thiếu xu vẫn mất phụ kiện: %v", v.Parts)
	}
}

func TestGrantPartsBad(t *testing.T) {
	e := newGarageEnv(t, 0)
	tx, err := e.w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := GrantParts(ctx, tx, e.uid, "banh_lai", 1); err == nil {
		t.Fatal("phụ kiện không có phải lỗi")
	}
	if err := GrantParts(ctx, tx, e.uid, PartTire, 0); err == nil {
		t.Fatal("số lượng phải dương")
	}
}

// Nhiều lần nâng cùng lúc không tiêu quá số phụ kiện đang có.
func TestUpgradePartsConcurrent(t *testing.T) {
	e := newGarageEnv(t, 1_000_000)
	e.grant(PartTire, 3) // đủ cho cấp 1 (1) + cấp 2 (2)
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.g.Upgrade(ctx, e.uid, "starter", "tires")
		}()
	}
	wg.Wait()
	v, err := e.g.View(ctx, e.uid)
	if err != nil {
		t.Fatal(err)
	}
	if carOf(v, "starter").Levels.Tires != 2 || v.Parts[PartTire] != 0 || e.balance() != 1_000_000-3_000-6_000 {
		t.Fatalf("nâng đồng thời: lốp %d, phụ kiện %v, xu %d", carOf(v, "starter").Levels.Tires, v.Parts, e.balance())
	}
}
