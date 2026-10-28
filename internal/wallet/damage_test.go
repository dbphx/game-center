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
