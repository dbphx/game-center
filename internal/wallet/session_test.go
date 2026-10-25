package wallet

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// Mỗi tài khoản một phiên: đăng nhập lần hai đá phiên cũ ra, token cũ báo "đăng nhập ở nơi khác".
func TestOneSessionPerAccount(t *testing.T) {
	w := open(t)
	u, regToken := register(t, w, "mot")
	watch, stop := w.WatchRevoke(u.ID)
	defer stop()
	other, _ := w.WatchRevoke(u.ID + 1) // người khác không bị ảnh hưởng

	_, t1, err := w.Login(ctx, "mot", "matkhau123")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-watch:
	case <-time.After(time.Second):
		t.Fatal("kết nối đang mở phải được báo khi đăng nhập ở nơi khác")
	}
	select {
	case <-other:
		t.Fatal("tài khoản khác không được bị báo")
	default:
	}
	if _, err := w.UserByToken(ctx, regToken); !errors.Is(err, ErrSessionElsewhere) || !errors.Is(err, ErrNotFound) || !IsElsewhere(err) {
		t.Fatalf("token lúc đăng ký phải bị đá ra, được %v", err)
	}
	if got, err := w.UserByToken(ctx, t1); err != nil || got.ID != u.ID {
		t.Fatalf("token mới: %+v %v", got, err)
	}

	_, t2, err := w.Login(ctx, "mot", "matkhau123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.UserByToken(ctx, t1); !IsElsewhere(err) {
		t.Fatalf("token 1 phải bị đá ra, được %v", err)
	}
	if _, err := w.UserByToken(ctx, t2); err != nil {
		t.Fatalf("token 2 phải dùng được: %v", err)
	}
	if !w.SessionElsewhere(ctx, t1) || w.SessionElsewhere(ctx, t2) || w.SessionElsewhere(ctx, "khong-co") {
		t.Fatal("SessionElsewhere sai")
	}

	// Đăng xuất chỉ xoá phiên hiện tại; token đăng xuất là "không hợp lệ" chứ không phải "nơi khác".
	if err := w.Logout(ctx, t2); err != nil {
		t.Fatal(err)
	}
	if _, err := w.UserByToken(ctx, t2); !errors.Is(err, ErrNotFound) || IsElsewhere(err) {
		t.Fatalf("token đã đăng xuất: %v", err)
	}
	if _, err := w.UserByToken(ctx, "khong-co"); !errors.Is(err, ErrNotFound) || IsElsewhere(err) {
		t.Fatalf("token lạ: %v", err)
	}
	// Đăng nhập lại sau khi đăng xuất không đá ai (không có phiên cũ).
	watch2, stop2 := w.WatchRevoke(u.ID)
	defer stop2()
	if _, _, err := w.Login(ctx, "mot", "matkhau123"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watch2:
		t.Fatal("không có phiên cũ thì không báo")
	default:
	}
}

// Token bị đá ra chỉ được nhớ RevokedKeepSeconds, sau đó là "không hợp lệ".
func TestRevokedSessionsPruned(t *testing.T) {
	w, tick := clockWallet(t)
	register(t, w, "nguoicu")
	_, t1, _ := w.Login(ctx, "nguoicu", "matkhau123")
	w.Login(ctx, "nguoicu", "matkhau123")
	tick(RevokedKeepSeconds*time.Second + time.Hour)
	w.Login(ctx, "nguoicu", "matkhau123") // dọn bản ghi cũ
	if _, err := w.UserByToken(ctx, t1); !errors.Is(err, ErrNotFound) || IsElsewhere(err) {
		t.Fatalf("token quá cũ: %v", err)
	}
}

// Database v8 nâng lên v9: phiên đang có vẫn dùng được, đăng nhập lần sau mới đá ra.
func TestMigrateFromV8KeepsSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v8.db")
	w, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, token := register(t, w, "binh")
	w.Close()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(dropV9 + `PRAGMA user_version = 8;`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	w, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.UserByToken(ctx, token); err != nil {
		t.Fatalf("phiên cũ sau nâng cấp: %v", err)
	}
	w.Login(ctx, "binh", "matkhau123")
	if _, err := w.UserByToken(ctx, token); !IsElsewhere(err) {
		t.Fatalf("đăng nhập sau nâng cấp phải đá phiên cũ, được %v", err)
	}
}

// dropV9 bỏ bảng revoked_sessions để đưa database về dạng v8.
const dropV9 = `DROP TABLE revoked_sessions;`
