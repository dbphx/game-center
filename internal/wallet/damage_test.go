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
	if lost := damage(t, w, u.ID, 40); lost != 40 {
		t.Fatalf("lần hai trừ %d", lost)
	}
	if u, _ = w.User(ctx, u.ID); u.HP != 11 {
		t.Fatalf("còn %d, muốn 11", u.HP)
	}
	// Tụt dưới HospitalHP: nằm viện, máu về 0 (trừ hết phần còn lại); đang nằm viện thì không trừ thêm.
	if lost := damage(t, w, u.ID, 5); lost != 11 {
		t.Fatalf("vào viện trừ %d, muốn 11", lost)
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

// Máu tụt dưới HospitalHP (10) là phải nằm viện và trả viện phí, kể cả khi chưa về 0.
func TestHospitalBelowTenHP(t *testing.T) {
	w, tick := clockWallet(t)
	u, _ := register(t, w, "muoi")
	damage(t, w, u.ID, 90)
	if u, _ = w.User(ctx, u.ID); u.HP != 10 || u.HospitalUntil != 0 {
		t.Fatalf("còn 10 HP thì chưa vào viện: %+v", u)
	}
	tick(time.Second)
	damage(t, w, u.ID, 1)
	if u, _ = w.User(ctx, u.ID); u.HP != 0 || u.HospitalUntil == 0 || u.Balance != 0 {
		t.Fatalf("9 HP phải nằm viện và trả viện phí: %+v", u)
	}

	v, _ := register(t, w, "xedung")
	for range 4 { // 100 → 20 sau 4 lần xe tông
		tick(2 * time.Second)
		mustHit(t, w, v.ID)
	}
	if v, _ = w.User(ctx, v.ID); v.HP != 20 || v.HospitalUntil != 0 {
		t.Fatalf("20 HP chưa vào viện: %+v", v)
	}
	tick(2 * time.Second)
	if v = mustHit(t, w, v.ID); v.HospitalUntil == 0 {
		t.Fatalf("xe tông còn 0 HP phải nằm viện: %+v", v)
	}
}
