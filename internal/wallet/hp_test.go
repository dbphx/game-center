package wallet

import (
	"errors"
	"testing"
	"time"
)

// clockWallet trả ví có đồng hồ điều khiển được và hàm tua giờ.
func clockWallet(t *testing.T) (*Wallet, func(time.Duration)) {
	t.Helper()
	w := open(t)
	clock := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return clock }
	return w, func(d time.Duration) { clock = clock.Add(d) }
}

func mustHit(t *testing.T, w *Wallet, id int64) User {
	t.Helper()
	u, err := w.Hit(ctx, id, "car")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestHitRegenAndHospital(t *testing.T) {
	w, tick := clockWallet(t)
	u, _ := register(t, w, "binh")
	if u.HP != MaxHP || u.MaxHP != MaxHP || u.HospitalUntil != 0 || u.Now == 0 {
		t.Fatalf("tài khoản mới phải đầy máu: %+v", u)
	}
	if _, err := w.Hit(ctx, u.ID, "meteor"); !errors.Is(err, ErrBadSource) {
		t.Fatalf("nguồn lạ phải lỗi, được %v", err)
	}

	if u = mustHit(t, w, u.ID); u.HP != 80 {
		t.Fatalf("bị xe đụng phải còn 80, được %d", u.HP)
	}
	tick(time.Second) // va chạm dồn dập trong 1.5 giây bị bỏ qua
	if u = mustHit(t, w, u.ID); u.HP != 80 {
		t.Fatalf("chống spam: phải vẫn 80, được %d", u.HP)
	}
	tick(time.Second)
	if u = mustHit(t, w, u.ID); u.HP != 60 {
		t.Fatalf("lần đụng sau phải còn 60, được %d", u.HP)
	}

	tick(150 * time.Second) // hồi 1 HP mỗi phút: 2 phút rưỡi → +2
	if u, _ = w.User(ctx, u.ID); u.HP != 62 {
		t.Fatalf("tự hồi phải lên 62, được %d", u.HP)
	}
	tick(time.Hour)
	if u, _ = w.User(ctx, u.ID); u.HP != MaxHP {
		t.Fatalf("hồi không vượt %d, được %d", MaxHP, u.HP)
	}

	for range 5 {
		tick(2 * time.Second)
		u = mustHit(t, w, u.ID)
	}
	if u.HP != 0 || u.HospitalUntil != u.Now+HospitalStay {
		t.Fatalf("hết máu phải nằm viện 5 phút: %+v", u)
	}
	tick(2 * time.Second)
	if _, err := w.Hit(ctx, u.ID, "car"); !errors.Is(err, ErrInHospital) {
		t.Fatalf("đang nằm viện không bị đụng nữa, được %v", err)
	}
	tick(148 * time.Second) // nửa thời gian nằm viện → nửa máu
	if u, _ = w.User(ctx, u.ID); u.HP != 50 || u.HospitalUntil == 0 {
		t.Fatalf("giữa kỳ nằm viện phải 50 HP và vẫn nằm viện: %+v", u)
	}
	tick(150 * time.Second)
	if u, _ = w.User(ctx, u.ID); u.HP != MaxHP || u.HospitalUntil != 0 {
		t.Fatalf("hết hạn phải tự xuất viện đầy máu: %+v", u)
	}
}

func TestHeal(t *testing.T) {
	w, tick := clockWallet(t)
	u, _ := register(t, w, "binh")
	if _, err := w.Heal(ctx, u.ID); !errors.Is(err, ErrFullHP) {
		t.Fatalf("đầy máu phải lỗi, được %v", err)
	}

	for range 2 {
		tick(2 * time.Second)
		mustHit(t, w, u.ID)
	}
	u, err := w.Heal(ctx, u.ID) // thiếu 40 HP → 80 xu
	if err != nil || u.HP != MaxHP || u.Balance != StartBalance-80 {
		t.Fatalf("chữa 40 HP: %+v %v", u, err)
	}
	if h, _ := w.History(ctx, u.ID, 1); h[0].Game != "hospital" || h[0].Note != "Chữa trị 40 HP" || h[0].Delta != -80 {
		t.Fatalf("sổ cái chữa trị sai: %+v", h)
	}

	for range 5 {
		tick(2 * time.Second)
		mustHit(t, w, u.ID)
	}
	if u, err = w.Heal(ctx, u.ID); err != nil || u.HP != MaxHP || u.HospitalUntil != 0 || u.Balance != StartBalance-80-DischargePrice {
		t.Fatalf("xuất viện ngay: %+v %v", u, err)
	}
	if h, _ := w.History(ctx, u.ID, 1); h[0].Note != "Viện phí xuất viện" {
		t.Fatalf("sổ cái xuất viện sai: %+v", h)
	}

	// Thiếu xu thì không chữa, không đổi gì.
	w.db.Exec(`UPDATE users SET balance = 10 WHERE id = ?`, u.ID)
	tick(2 * time.Second)
	mustHit(t, w, u.ID)
	if _, err := w.Heal(ctx, u.ID); !errors.Is(err, ErrNotEnoughCoins) {
		t.Fatalf("thiếu xu phải lỗi, được %v", err)
	}
	if u, _ = w.User(ctx, u.ID); u.HP != 80 || u.Balance != 10 {
		t.Fatalf("thiếu xu không được đổi gì: %+v", u)
	}
}

func TestCheckZone(t *testing.T) {
	cases := []struct {
		u    User
		zone string
		want error
	}{
		{User{HP: 100}, "casino", nil},
		{User{HP: 29}, "casino", ErrLowHP},
		{User{HP: 30}, "casino", nil},
		{User{HP: 39}, "farm", ErrLowHP},
		{User{HP: 9}, "shop", ErrLowHP},
		{User{HP: 0}, "bank", nil},
		{User{HP: 90, HospitalUntil: 123}, "shop", ErrInHospital},
	}
	for _, tc := range cases {
		if err := CheckZone(tc.u, tc.zone); !errors.Is(err, tc.want) {
			t.Errorf("%+v %s: muốn %v, được %v", tc.u, tc.zone, tc.want, err)
		}
	}
	if err := CheckZone(User{HP: 5}, "farm"); err.Error() != "cần ít nhất 40 HP để làm nông" {
		t.Errorf("thông báo nông trại sai: %v", err)
	}
	if err := CheckZone(User{HP: 5}, "casino"); err.Error() != "cần ít nhất 30 HP để vào Casino" {
		t.Errorf("thông báo casino sai: %v", err)
	}
}
