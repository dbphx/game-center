package wallet

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestGoogleAccount(t *testing.T) {
	w := open(t)
	if _, _, err := w.LoginGoogle(ctx, "sub-1", "an@gmail.com"); !errors.Is(err, ErrGoogleNotLinked) {
		t.Fatalf("chưa liên kết: %v", err)
	}
	u, token, err := w.RegisterGoogle(ctx, "sub-1", "an@gmail.com", Profile{Name: "  An  ", Gender: "female"})
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "" || u.Name != "An" || u.Balance != StartBalance || u.Google == nil || u.Google.Email != "an@gmail.com" {
		t.Fatalf("tài khoản Google: %+v", u)
	}
	if _, _, err := w.RegisterGoogle(ctx, "sub-1", "an@gmail.com", Profile{}); !errors.Is(err, ErrGoogleTaken) {
		t.Fatalf("đăng ký lại cùng sub: %v", err)
	}
	// Không tên hiển thị → lấy phần trước @ của email.
	b, _, err := w.RegisterGoogle(ctx, "sub-2", "binh.tran@gmail.com", Profile{})
	if err != nil || b.Name != "binh.tran" {
		t.Fatalf("tên mặc định: %+v %v", b, err)
	}

	// Đăng nhập lại: phiên cũ bị đá ra, email cập nhật theo Google.
	u2, token2, err := w.LoginGoogle(ctx, "sub-1", "an.moi@gmail.com")
	if err != nil || u2.ID != u.ID || u2.Google.Email != "an.moi@gmail.com" {
		t.Fatalf("đăng nhập Google: %+v %v", u2, err)
	}
	if _, err := w.UserByToken(ctx, token); !IsElsewhere(err) {
		t.Fatalf("phiên cũ phải bị đá ra: %v", err)
	}
	if _, err := w.UserByToken(ctx, token2); err != nil {
		t.Fatal(err)
	}

	// Chưa có mật khẩu: đổi mật khẩu báo lỗi rõ, bỏ liên kết Google bị chặn; đặt tên đăng nhập + mật khẩu rồi mới bỏ được.
	if err := w.ChangePassword(ctx, u.ID, "", "matkhau456"); !errors.Is(err, ErrNoPassword) {
		t.Fatalf("đổi mật khẩu khi chưa có: %v", err)
	}
	if _, err := w.UnlinkGoogle(ctx, u.ID); !errors.Is(err, ErrNeedCredentials) {
		t.Fatalf("bỏ liên kết khi chưa có mật khẩu: %v", err)
	}
	if _, err := w.SetCredentials(ctx, u.ID, "an_google", "matkhau123"); err != nil {
		t.Fatal(err)
	}
	if u, err = w.UnlinkGoogle(ctx, u.ID); err != nil || u.Google != nil {
		t.Fatalf("bỏ liên kết: %+v %v", u, err)
	}
	if _, err := w.UnlinkGoogle(ctx, u.ID); !errors.Is(err, ErrNoGoogle) {
		t.Fatalf("bỏ liên kết lần hai: %v", err)
	}
	if _, _, err := w.Login(ctx, "an_google", "matkhau123"); err != nil {
		t.Fatalf("đăng nhập bằng mật khẩu sau khi bỏ Google: %v", err)
	}
}

func TestLinkGoogle(t *testing.T) {
	w := open(t)
	a, _ := register(t, w, "an_cu")
	b, _ := register(t, w, "binh")
	if _, err := w.ClaimDaily(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	u, err := w.LinkGoogle(ctx, a.ID, "sub-a", "an@gmail.com")
	if err != nil || u.Google == nil || u.Balance != StartBalance+DailyBonus {
		t.Fatalf("liên kết: %+v %v", u, err)
	}
	if _, err := w.LinkGoogle(ctx, a.ID, "sub-a", "an@gmail.com"); err != nil {
		t.Fatalf("liên kết lại đúng tài khoản phải được: %v", err)
	}
	if _, err := w.LinkGoogle(ctx, b.ID, "sub-a", "an@gmail.com"); !errors.Is(err, ErrGoogleTaken) {
		t.Fatalf("sub đã gắn tài khoản khác: %v", err)
	}
	if _, err := w.LinkGoogle(ctx, a.ID, "sub-khac", "an2@gmail.com"); !errors.Is(err, ErrHasGoogle) {
		t.Fatalf("tài khoản đã có Google khác: %v", err)
	}
	// Đăng nhập bằng Google vào đúng tài khoản cũ (giữ xu).
	u, _, err = w.LoginGoogle(ctx, "sub-a", "an@gmail.com")
	if err != nil || u.ID != a.ID || u.Balance != StartBalance+DailyBonus {
		t.Fatalf("đăng nhập Google tài khoản cũ: %+v %v", u, err)
	}
	if b, _ := w.User(ctx, b.ID); b.Google != nil {
		t.Fatalf("tài khoản khác không có Google: %+v", b.Google)
	}
}

// Database v9 nâng lên v10: tài khoản giữ nguyên, chưa liên kết Google, liên kết được ngay.
func TestMigrateFromV9AddsGoogle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v9.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, token := register(t, w, "an_cu")
	w.Close()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(dropV10 + `PRAGMA user_version = 9;`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	w, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	u, err := w.UserByToken(ctx, token)
	if err != nil || u.ID != a.ID || u.Google != nil {
		t.Fatalf("sau nâng cấp: %+v %v", u, err)
	}
	if _, err := w.LinkGoogle(ctx, a.ID, "sub-a", "an@gmail.com"); err != nil {
		t.Fatal(err)
	}
}

// dropV10 bỏ bảng user_google để đưa database về dạng v9.
const dropV10 = `DROP TABLE user_google;`
