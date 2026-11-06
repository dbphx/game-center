package wallet

import (
	"testing"
	"time"
)

func damage(t *testing.T, w *Wallet, id int64, amount int) int {
	t.Helper()
	tx, err := w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	lost, err := tx.Damage(id, amount)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return lost
}

func TestDamageHospitalizes(t *testing.T) {
	w, tick := clockWallet(t)
	u, _ := register(t, w, "binh")
	if lost := damage(t, w, u.ID, 50); lost != 50 {
		t.Fatalf("trừ %d, muốn 50", lost)
	}
	if u, _ = w.User(ctx, u.ID); u.HP != 50 || u.HospitalUntil != 0 {
		t.Fatalf("sau 50: %+v", u)
	}
	tick(time.Minute) // hồi 1 HP
	if lost := damage(t, w, u.ID, 50); lost != 50 {
		t.Fatalf("lần hai trừ %d", lost)
	}
	if u, _ = w.User(ctx, u.ID); u.HP != 1 {
		t.Fatalf("còn %d, muốn 1", u.HP)
	}
	// Hết máu: nằm viện, chỉ trừ phần máu còn lại; đang nằm viện thì không trừ thêm.
	if lost := damage(t, w, u.ID, 50); lost != 1 {
		t.Fatalf("hết máu trừ %d, muốn 1", lost)
	}
	if u, _ = w.User(ctx, u.ID); u.HP != 0 || u.HospitalUntil == 0 {
		t.Fatalf("phải nằm viện: %+v", u)
	}
	if lost := damage(t, w, u.ID, 50); lost != 0 {
		t.Fatalf("nằm viện mà vẫn trừ %d", lost)
	}
}

// Mỗi lần vào viện bị trừ viện phí HospitalFee; không đủ thì trừ hết; đang nằm viện thì không trừ lần hai.
func TestHospitalFee(t *testing.T) {
	w, tick := clockWallet(t)
	rich, _ := register(t, w, "giau")
	tx, err := w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Credit(rich.ID, 600_000, "test", "nạp"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	damage(t, w, rich.ID, 50)
	if u, _ := w.User(ctx, rich.ID); u.Balance != StartBalance+600_000 {
		t.Fatalf("chưa vào viện mà đã trừ: %d", u.Balance)
	}
	damage(t, w, rich.ID, 100)
	u, _ := w.User(ctx, rich.ID)
	if u.HospitalUntil == 0 || u.Balance != StartBalance+600_000-HospitalFee {
		t.Fatalf("vào viện phải trừ %d: %+v", HospitalFee, u)
	}
	damage(t, w, rich.ID, 100)
	if u2, _ := w.User(ctx, rich.ID); u2.Balance != u.Balance {
		t.Fatalf("đang nằm viện bị trừ thêm: %d → %d", u.Balance, u2.Balance)
	}

	poor, _ := register(t, w, "ngheo")
	for range 5 { // xe tông 5 lần: hết máu, vào viện
		tick(2 * time.Second)
		mustHit(t, w, poor.ID)
	}
	if p, _ := w.User(ctx, poor.ID); p.HospitalUntil == 0 || p.Balance != 0 {
		t.Fatalf("không đủ xu thì trừ hết: %+v", p)
	}
}
