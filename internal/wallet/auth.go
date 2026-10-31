package wallet

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	MinPasswordLen = 6
	MaxPasswordLen = 72 // giới hạn của bcrypt
)

var (
	ErrBadCredentials  = errors.New("sai tên đăng nhập hoặc mật khẩu")
	ErrUsernameTaken   = errors.New("tên đăng nhập đã có người dùng")
	ErrInvalidUsername = errors.New("tên đăng nhập gồm 3–20 ký tự: chữ thường không dấu, số, dấu _ hoặc .")
	ErrWeakPassword    = errors.New("mật khẩu cần ít nhất 6 ký tự")
	ErrHasCredentials  = errors.New("tài khoản đã có tên đăng nhập")
	ErrWrongPassword   = errors.New("mật khẩu hiện tại không đúng")
	ErrLongPassword    = errors.New("mật khẩu quá dài")
)

var usernameRe = regexp.MustCompile(`^[a-z0-9_.]{3,20}$`)

// NormalizeUsername chuẩn hoá tên đăng nhập (không phân biệt hoa thường).
func NormalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func checkCredentials(username, password string) error {
	if !usernameRe.MatchString(username) {
		return ErrInvalidUsername
	}
	if len(password) < MinPasswordLen {
		return ErrWeakPassword
	}
	if len(password) > MaxPasswordLen {
		return ErrLongPassword
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func isUniqueViolation(err error) bool {
	var se *sqlite.Error
	return errors.As(err, &se) && se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}

// newSession tạo phiên đăng nhập. Token chỉ trả về một lần; DB chỉ lưu hash.
func (w *Wallet) newSession(ctx context.Context, q interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, userID int64) (string, error) {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	_, err := q.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id, created_at) VALUES (?, ?, ?)`,
		hashToken(token), userID, w.now().Unix())
	return token, err
}

// Register tạo tài khoản và nhân vật mới (tặng StartBalance xu) rồi đăng nhập luôn.
func (w *Wallet) Register(ctx context.Context, username, password string, p Profile) (User, string, error) {
	username = NormalizeUsername(username)
	if err := checkCredentials(username, password); err != nil {
		return User{}, "", err
	}
	if err := p.normalize(); err != nil {
		return User{}, "", err
	}
	name := p.Name
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, "", err
	}
	if strings.TrimSpace(name) == "" {
		name = username
	}

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()
	now := w.now().Unix()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (username, password_hash, name, balance, gender, outfit, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		username, string(hash), CleanName(name), StartBalance, p.Gender, p.Outfit, now)
	if isUniqueViolation(err) {
		return User{}, "", ErrUsernameTaken
	}
	if err != nil {
		return User{}, "", err
	}
	id, _ := res.LastInsertId()
	if err := addEntry(ctx, tx, id, StartBalance, StartBalance, "hub", "Quà chào mừng", now); err != nil {
		return User{}, "", err
	}
	token, err := w.newSession(ctx, tx, id)
	if err != nil {
		return User{}, "", err
	}
	if _, err := w.revokeSessions(ctx, tx, id, hashToken(token)); err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", err
	}
	u, err := w.User(ctx, id)
	return u, token, err
}

// dummyHash dùng khi tên đăng nhập không tồn tại, để thời gian trả lời giống như sai mật khẩu.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("không-có-tài-khoản"), bcrypt.DefaultCost)

// Login kiểm tra mật khẩu và tạo phiên mới; mọi phiên cũ của tài khoản bị đá ra (ErrSessionElsewhere).
func (w *Wallet) Login(ctx context.Context, username, password string) (User, string, error) {
	var id int64
	var hash string
	err := w.db.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE username = ?`, NormalizeUsername(username)).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return User{}, "", ErrBadCredentials
	}
	if err != nil {
		return User{}, "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, "", ErrBadCredentials
	}
	// Mỗi tài khoản một phiên: phiên mới thay mọi phiên cũ, kết nối đang mở bằng token cũ bị ngắt.
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

// Logout xoá phiên đăng nhập của token này.
func (w *Wallet) Logout(ctx context.Context, token string) error {
	_, err := w.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// SetCredentials đặt tên đăng nhập + mật khẩu cho tài khoản khách cũ, giữ nguyên số xu.
func (w *Wallet) SetCredentials(ctx context.Context, id int64, username, password string) (User, error) {
	username = NormalizeUsername(username)
	if err := checkCredentials(username, password); err != nil {
		return User{}, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	// Tài khoản khách cũ (chưa có tên) hoặc tài khoản Google (tên tự đặt, chưa có mật khẩu — được chọn tên khác).
	res, err := w.db.ExecContext(ctx, `UPDATE users SET username = ?, password_hash = ? WHERE id = ? AND (username IS NULL OR password_hash = '')`,
		username, string(hash), id)
	if isUniqueViolation(err) {
		return User{}, ErrUsernameTaken
	}
	if err != nil {
		return User{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return User{}, ErrHasCredentials
	}
	return w.User(ctx, id)
}

// ChangePassword đổi mật khẩu sau khi kiểm tra mật khẩu cũ.
func (w *Wallet) ChangePassword(ctx context.Context, id int64, oldPassword, newPassword string) error {
	var hash string
	if err := w.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&hash); err != nil {
		return err
	}
	if hash == "" {
		return ErrNoPassword // tài khoản tạo bằng Google / khách cũ: đặt mật khẩu qua SetCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPassword)) != nil {
		return ErrWrongPassword
	}
	if len(newPassword) < MinPasswordLen {
		return ErrWeakPassword
	}
	if len(newPassword) > MaxPasswordLen {
		return ErrLongPassword
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = w.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, string(newHash), id)
	return err
}

// UserByToken trả người dùng của phiên. Token không có: ErrNotFound; token bị một lần đăng nhập khác
// đá ra: ErrSessionElsewhere (errors.Is(err, ErrNotFound) vẫn đúng).
func (w *Wallet) UserByToken(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNotFound
	}
	u, err := w.withOwned(ctx)(w.scanUser(w.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users
		WHERE id = (SELECT user_id FROM sessions WHERE token_hash = ?)`, hashToken(token))))
	if errors.Is(err, ErrNotFound) && w.SessionElsewhere(ctx, token) {
		return User{}, ErrSessionElsewhere
	}
	return u, err
}
