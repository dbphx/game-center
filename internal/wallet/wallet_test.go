package wallet

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func open(t *testing.T) *Wallet {
	t.Helper()
	w, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func TestGuestAccount(t *testing.T) {
	ctx := context.Background()
	w := open(t)
	u, token, err := w.CreateGuest(ctx, "  An   Nguyễn  ")
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "An Nguyễn" || u.Balance != StartBalance || !u.CanDaily {
		t.Fatalf("tài khoản mới sai: %+v", u)
	}
	got, err := w.UserByToken(ctx, token)
	if err != nil || got.ID != u.ID {
		t.Fatalf("tra token: %+v %v", got, err)
	}
	if _, err := w.UserByToken(ctx, "sai"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("token sai phải ErrNotFound, được %v", err)
	}
	if u, _ = w.Rename(ctx, u.ID, "Bình"); u.Name != "Bình" {
		t.Fatalf("đổi tên sai: %+v", u)
	}
}

func TestDailyOncePerVietnamDay(t *testing.T) {
	ctx := context.Background()
	w := open(t)
	clock := time.Date(2026, 10, 24, 16, 0, 0, 0, time.UTC) // 23h giờ VN
	w.now = func() time.Time { return clock }
	u, _, _ := w.CreateGuest(ctx, "An")

	if u, err := w.ClaimDaily(ctx, u.ID); err != nil || u.Balance != StartBalance+DailyBonus || u.CanDaily {
		t.Fatalf("nhận lần đầu: %+v %v", u, err)
	}
	if _, err := w.ClaimDaily(ctx, u.ID); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("nhận lần hai trong ngày phải lỗi, được %v", err)
	}
	clock = clock.Add(2 * time.Hour) // 1h sáng hôm sau giờ VN
	if u, err := w.ClaimDaily(ctx, u.ID); err != nil || u.Balance != StartBalance+2*DailyBonus {
		t.Fatalf("sang ngày mới phải nhận được: %+v %v", u, err)
	}
}

func TestSettleClampsAtZeroAndRecordsLedger(t *testing.T) {
	ctx := context.Background()
	w := open(t)
	a, _, _ := w.CreateGuest(ctx, "An")
	b, _, _ := w.CreateGuest(ctx, "Bình")

	bal, err := w.Settle(ctx, "tienlen", "ván 1", map[int64]int64{a.ID: 300, b.ID: -1500})
	if err != nil {
		t.Fatal(err)
	}
	if bal[a.ID] != 1300 || bal[b.ID] != 0 {
		t.Fatalf("số dư sai: %v", bal)
	}
	h, _ := w.History(ctx, b.ID, 10)
	if len(h) != 2 || h[0].Delta != -1000 || h[0].BalanceAfter != 0 || h[0].Game != "tienlen" || h[1].Note != "Quà chào mừng" {
		t.Fatalf("sổ cái sai: %+v", h)
	}
	if _, err := w.Settle(ctx, "tienlen", "x", map[int64]int64{999: 10}); err == nil {
		t.Fatal("người không tồn tại phải lỗi")
	}
	if u, _ := w.User(ctx, a.ID); u.Balance != 1300 {
		t.Fatalf("lỗi phải rollback cả transaction, số dư %d", u.Balance)
	}
}
