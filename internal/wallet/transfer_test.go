package wallet

import (
	"errors"
	"testing"
	"time"
)

func TestGift(t *testing.T) {
	w := open(t)
	day := time.Date(2026, 10, 25, 9, 0, 0, 0, vietnam)
	w.now = func() time.Time { return day }
	a, _ := register(t, w, "anh")
	b, _ := register(t, w, "binh")

	if _, _, err := w.Gift(ctx, "plaza", a.ID, a.ID, 50, 0, "x", "y"); !errors.Is(err, ErrGiftSelf) {
		t.Fatalf("tự tặng mình phải lỗi, được %v", err)
	}
	fa, fb, err := w.Gift(ctx, "plaza", a.ID, b.ID, 50, 300, "Tặng 50 xu cho binh", "Nhận 50 xu từ an")
	if err != nil {
		t.Fatal(err)
	}
	if fa != StartBalance-50 || fb != StartBalance+50 {
		t.Fatalf("số dư sau tặng: %d %d", fa, fb)
	}
	h, _ := w.History(ctx, b.ID, 1)
	if len(h) != 1 || h[0].Delta != 50 || h[0].Game != "plaza" || h[0].Note != "Nhận 50 xu từ an" {
		t.Fatalf("sổ cái người nhận: %+v", h)
	}
	if sum, _ := w.GiftedToday(ctx, a.ID, "plaza"); sum != 50 {
		t.Fatalf("đã tặng hôm nay %d", sum)
	}

	// Hạn mức ngày: còn 250, tặng 300 bị từ chối và không đổi gì.
	var lim *GiftLimitError
	if _, _, err := w.Gift(ctx, "plaza", a.ID, b.ID, 300, 300, "x", "y"); !errors.As(err, &lim) || lim.Left != 250 {
		t.Fatalf("vượt hạn mức: %v", err)
	}
	if u, _ := w.User(ctx, a.ID); u.Balance != StartBalance-50 {
		t.Fatalf("bị từ chối mà vẫn trừ: %d", u.Balance)
	}
	// Không đủ xu: không trừ, không cộng.
	if _, _, err := w.Gift(ctx, "plaza", a.ID, b.ID, 5000, 0, "x", "y"); !errors.Is(err, ErrNotEnoughCoins) {
		t.Fatalf("không đủ xu: %v", err)
	}
	if u, _ := w.User(ctx, b.ID); u.Balance != StartBalance+50 {
		t.Fatalf("người nhận bị cộng sai: %d", u.Balance)
	}
	// Người nhận không tồn tại.
	if _, _, err := w.Gift(ctx, "plaza", a.ID, 9999, 10, 0, "x", "y"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("người nhận lạ: %v", err)
	}
	// Sang ngày mới (0h giờ VN) thì hạn mức tính lại.
	w.now = func() time.Time { return time.Date(2026, 10, 26, 0, 0, 1, 0, vietnam) }
	if _, _, err := w.Gift(ctx, "plaza", a.ID, b.ID, 300, 300, "x", "y"); err != nil {
		t.Fatalf("ngày mới vẫn bị chặn: %v", err)
	}
}
