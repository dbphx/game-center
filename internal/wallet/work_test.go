package wallet

import (
	"errors"
	"testing"
	"time"
)

func spend(t *testing.T, w *Wallet, id int64, amount int) (int, error) {
	t.Helper()
	tx, err := w.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	hp, err := tx.SpendHP(id, amount)
	if err == nil {
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return hp, err
}

func TestSpendHP(t *testing.T) {
	w, tick := clockWallet(t)
	u, _ := register(t, w, "tho")
	if hp, err := spend(t, w, u.ID, 8); err != nil || hp != 92 {
		t.Fatalf("trừ 8 HP: còn %d, lỗi %v", hp, err)
	}
	if u, _ = w.User(ctx, u.ID); u.HP != 92 {
		t.Fatalf("ví phải còn 92 HP, được %d", u.HP)
	}
	tick(90 * time.Second) // hồi 1 HP, còn 30 giây lẻ
	if hp, _ := spend(t, w, u.ID, 10); hp != 83 {
		t.Fatalf("sau hồi 1 HP rồi trừ 10 phải còn 83, được %d", hp)
	}
	tick(30 * time.Second) // 30 giây lẻ được giữ lại: thêm 30 giây là đủ 1 HP
	if u, _ = w.User(ctx, u.ID); u.HP != 84 {
		t.Fatalf("phần hồi dở phải được giữ: muốn 84, được %d", u.HP)
	}
	if _, err := spend(t, w, u.ID, 84); !errors.Is(err, ErrLowHP) {
		t.Fatalf("không được trừ tới 0 HP, lỗi %v", err)
	}
	for range 5 {
		tick(2 * time.Second)
		mustHit(t, w, u.ID)
	}
	if _, err := spend(t, w, u.ID, 1); !errors.Is(err, ErrInHospital) {
		t.Fatalf("đang nằm viện phải lỗi, được %v", err)
	}
}
