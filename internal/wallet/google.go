package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Đăng nhập bằng Google: tài khoản Google (sub — mã không đổi của Google) liên kết với đúng một tài khoản DCity.
// Tài khoản tạo bằng Google là tài khoản thường: có tên đăng nhập tự đặt theo email (để kết bạn, tìm người chơi),
// chưa có mật khẩu (password_hash rỗng) — đặt sau qua SetCredentials (được đổi luôn tên đăng nhập).
// Việc xác minh ID token của Google làm ở internal/googleauth.

// Google là tài khoản Google đã liên kết (chỉ trả email, không lộ sub).
type Google struct {
	Email string `json:"email"`
}

var (
	ErrGoogleNotLinked = errors.New("tài khoản Google này chưa liên kết với tài khoản DCity nào")
	ErrGoogleTaken     = errors.New("tài khoản Google này đã liên kết với một tài khoản DCity khác")
	ErrHasGoogle       = errors.New("tài khoản đã liên kết một tài khoản Google khác, bỏ liên kết trước")
	ErrNoGoogle        = errors.New("tài khoản chưa liên kết Google")
	ErrNeedCredentials = errors.New("đặt tên đăng nhập và mật khẩu trước khi bỏ liên kết Google")
	ErrNoPassword      = errors.New("tài khoản chưa có mật khẩu — đặt tên đăng nhập và mật khẩu trước")
)

// startSession tạo phiên mới cho id và đá mọi phiên cũ (mỗi tài khoản một phiên), như Login.
func (w *Wallet) startSession(ctx context.Context, id int64) (User, string, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()
	token, err := w.newSession(ctx, tx, id)
	if err != nil {
		return User{}, "", err
	}
	revoked, err := w.revokeSessions(ctx, tx, id, hashToken(token))
	if err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", err
	}
	if revoked > 0 {
		w.fireRevoke(id)
	}
	u, err := w.User(ctx, id)
	return u, token, err
}

// LoginGoogle đăng nhập tài khoản đã liên kết với sub (email cập nhật theo Google). Chưa liên kết: ErrGoogleNotLinked.
func (w *Wallet) LoginGoogle(ctx context.Context, sub, email string) (User, string, error) {
	var id int64
	err := w.db.QueryRowContext(ctx, `SELECT user_id FROM user_google WHERE sub = ?`, sub).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrGoogleNotLinked
	}
	if err != nil {
		return User{}, "", err
	}
	if _, err := w.db.ExecContext(ctx, `UPDATE user_google SET email = ? WHERE sub = ? AND email != ?`, email, sub, email); err != nil {
		return User{}, "", err
	}
	return w.startSession(ctx, id)
}

// RegisterGoogle tạo tài khoản mới (không tên đăng nhập / mật khẩu) gắn với sub, tặng StartBalance xu, đăng nhập luôn.
func (w *Wallet) RegisterGoogle(ctx context.Context, sub, email string, p Profile) (User, string, error) {
	if err := p.normalize(); err != nil {
		return User{}, "", err
	}
	name := p.Name
	if strings.TrimSpace(name) == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()
	var taken int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_google WHERE sub = ?`, sub).Scan(&taken); err != nil {
		return User{}, "", err
	}
	if taken > 0 {
		return User{}, "", ErrGoogleTaken
	}
	now := w.now().Unix()
	username, err := freeUsername(ctx, tx, email)
	if err != nil {
		return User{}, "", err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO users (username, password_hash, name, balance, gender, outfit, created_at)
		VALUES (?, '', ?, ?, ?, ?, ?)`, username, CleanName(name), StartBalance, p.Gender, p.Outfit, now)
	if err != nil {
		return User{}, "", err
	}
	id, _ := res.LastInsertId()
	if err := addEntry(ctx, tx, id, StartBalance, StartBalance, "hub", "Quà chào mừng", now); err != nil {
		return User{}, "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_google (sub, user_id, email, created_at) VALUES (?, ?, ?, ?)`,
		sub, id, email, now); err != nil {
		if isUniqueViolation(err) {
			return User{}, "", ErrGoogleTaken
		}
		return User{}, "", err
	}
	token, err := w.newSession(ctx, tx, id)
	if err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", err
	}
	u, err := w.User(ctx, id)
	return u, token, err
}

// LinkGoogle gắn tài khoản Google vào tài khoản đang đăng nhập (giữ nguyên xu, nông trại, đồ).
func (w *Wallet) LinkGoogle(ctx context.Context, id int64, sub, email string) (User, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var owner int64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM user_google WHERE sub = ?`, sub).Scan(&owner)
	switch {
	case err == nil && owner == id:
		tx.Rollback()          // đóng transaction trước khi đọc lại (database chỉ một kết nối)
		return w.User(ctx, id) // đã liên kết đúng tài khoản này rồi
	case err == nil:
		return User{}, ErrGoogleTaken
	case !errors.Is(err, sql.ErrNoRows):
		return User{}, err
	}
	var has int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_google WHERE user_id = ?`, id).Scan(&has); err != nil {
		return User{}, err
	}
	if has > 0 {
		return User{}, ErrHasGoogle
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_google (sub, user_id, email, created_at) VALUES (?, ?, ?, ?)`,
		sub, id, email, w.now().Unix()); err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrGoogleTaken
		}
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// UnlinkGoogle bỏ liên kết Google; chỉ được khi tài khoản còn cách đăng nhập khác (tên đăng nhập + mật khẩu).
func (w *Wallet) UnlinkGoogle(ctx context.Context, id int64) (User, error) {
	var username sql.NullString
	var hash string
	if err := w.db.QueryRowContext(ctx, `SELECT username, password_hash FROM users WHERE id = ?`, id).Scan(&username, &hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	if !username.Valid || hash == "" {
		return User{}, ErrNeedCredentials
	}
	res, err := w.db.ExecContext(ctx, `DELETE FROM user_google WHERE user_id = ?`, id)
	if err != nil {
		return User{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return User{}, ErrNoGoogle
	}
	return w.User(ctx, id)
}

type queryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// freeUsername chọn tên đăng nhập chưa ai dùng từ phần trước @ của email (chỉ giữ a-z 0-9 _ .),
// trùng thì thêm số: an.nguyen, an.nguyen2, an.nguyen3...
func freeUsername(ctx context.Context, q queryRower, email string) (string, error) {
	local, _, _ := strings.Cut(NormalizeUsername(email), "@")
	base := strings.Trim(strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			return r
		}
		return -1
	}, local), ".")
	if len(base) > 16 {
		base = base[:16]
	}
	if len(base) < 3 {
		base = "dcity" + base
	}
	for n := 1; n < 10000; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s%d", base, n)
		}
		var one int
		err := q.QueryRowContext(ctx, `SELECT 1 FROM users WHERE username = ?`, name).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return name, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", errors.New("không tìm được tên đăng nhập trống")
}

// backfillGoogleUsernames đặt tên đăng nhập cho tài khoản Google tạo trước khi có tên tự đặt (username NULL).
func backfillGoogleUsernames(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT u.id, g.email FROM users u JOIN user_google g ON g.user_id = u.id WHERE u.username IS NULL`)
	if err != nil {
		return err
	}
	type acc struct {
		id    int64
		email string
	}
	var list []acc
	for rows.Next() {
		var a acc
		if err := rows.Scan(&a.id, &a.email); err != nil {
			rows.Close()
			return err
		}
		list = append(list, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, a := range list {
		name, err := freeUsername(ctx, db, a.email)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `UPDATE users SET username = ? WHERE id = ? AND username IS NULL`, name, a.id); err != nil {
			return err
		}
	}
	return nil
}
