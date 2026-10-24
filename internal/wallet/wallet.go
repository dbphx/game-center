// Package wallet quản lý tài khoản người chơi và ví xu dùng chung cho mọi game trong hub.
//
// Mỗi thay đổi số dư đều ghi một dòng vào sổ cái (ledger) trong cùng transaction,
// nên luôn tra lại được xu đến từ đâu.
package wallet

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StartBalance = 1000 // xu tặng khi tạo tài khoản
	DailyBonus   = 500  // xu thưởng mỗi ngày
	MaxNameLen   = 16
)

var (
	ErrNotFound       = errors.New("không tìm thấy tài khoản")
	ErrAlreadyClaimed = errors.New("hôm nay bạn đã nhận thưởng rồi, mai quay lại nhé")
)

// Ngày tính theo giờ Việt Nam để thưởng hằng ngày reset lúc 0h.
var vietnam = time.FixedZone("ICT", 7*60*60)

type User struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Balance   int64  `json:"balance"`
	CanDaily  bool   `json:"canDaily"` // hôm nay còn nhận thưởng được không
	lastDaily string
}

type Entry struct {
	Delta        int64     `json:"delta"`
	BalanceAfter int64     `json:"balanceAfter"`
	Game         string    `json:"game"`
	Note         string    `json:"note"`
	At           time.Time `json:"at"`
}

type Wallet struct {
	db  *sql.DB
	now func() time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	token_hash TEXT NOT NULL UNIQUE,
	name       TEXT NOT NULL,
	balance    INTEGER NOT NULL CHECK (balance >= 0),
	last_daily TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS ledger (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id       INTEGER NOT NULL REFERENCES users(id),
	delta         INTEGER NOT NULL,
	balance_after INTEGER NOT NULL,
	game          TEXT NOT NULL,
	note          TEXT NOT NULL,
	created_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ledger_user ON ledger(user_id, id);
`

// Open mở (hoặc tạo) file SQLite ở path.
func Open(path string) (*Wallet, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// SQLite chỉ cho một writer; một kết nối là đủ và tránh lỗi "database is locked".
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("tạo bảng: %w", err)
	}
	return &Wallet{db: db, now: time.Now}, nil
}

func (w *Wallet) Close() error { return w.db.Close() }

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CleanName cắt khoảng trắng và giới hạn độ dài tên hiển thị.
func CleanName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	if r := []rune(name); len(r) > MaxNameLen {
		name = string(r[:MaxNameLen])
	}
	if name == "" {
		name = "Người chơi"
	}
	return name
}

func (w *Wallet) today() string { return w.now().In(vietnam).Format("2006-01-02") }

// CreateGuest tạo tài khoản khách. Token chỉ trả về một lần; DB chỉ lưu hash.
func (w *Wallet) CreateGuest(ctx context.Context, name string) (User, string, error) {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback()
	now := w.now().Unix()
	res, err := tx.ExecContext(ctx, `INSERT INTO users (token_hash, name, balance, created_at) VALUES (?, ?, ?, ?)`,
		hashToken(token), CleanName(name), StartBalance, now)
	if err != nil {
		return User{}, "", err
	}
	id, _ := res.LastInsertId()
	if err := addEntry(ctx, tx, id, StartBalance, StartBalance, "hub", "Quà chào mừng", now); err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return User{}, "", err
	}
	u, err := w.User(ctx, id)
	return u, token, err
}

func addEntry(ctx context.Context, tx *sql.Tx, userID, delta, after int64, game, note string, at int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO ledger (user_id, delta, balance_after, game, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, delta, after, game, note, at)
	return err
}

func (w *Wallet) scanUser(row *sql.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Balance, &u.lastDaily)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.CanDaily = u.lastDaily != w.today()
	return u, err
}

func (w *Wallet) User(ctx context.Context, id int64) (User, error) {
	return w.scanUser(w.db.QueryRowContext(ctx, `SELECT id, name, balance, last_daily FROM users WHERE id = ?`, id))
}

func (w *Wallet) UserByToken(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, ErrNotFound
	}
	return w.scanUser(w.db.QueryRowContext(ctx, `SELECT id, name, balance, last_daily FROM users WHERE token_hash = ?`, hashToken(token)))
}

func (w *Wallet) Rename(ctx context.Context, id int64, name string) (User, error) {
	if _, err := w.db.ExecContext(ctx, `UPDATE users SET name = ? WHERE id = ?`, CleanName(name), id); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// ClaimDaily cộng thưởng hằng ngày; mỗi ngày (giờ Việt Nam) chỉ nhận một lần.
func (w *Wallet) ClaimDaily(ctx context.Context, id int64) (User, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	today := w.today()
	res, err := tx.ExecContext(ctx, `UPDATE users SET balance = balance + ?, last_daily = ? WHERE id = ? AND last_daily != ?`,
		DailyBonus, today, id, today)
	if err != nil {
		return User{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return User{}, ErrAlreadyClaimed
	}
	var balance int64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, id).Scan(&balance); err != nil {
		return User{}, err
	}
	if err := addEntry(ctx, tx, id, DailyBonus, balance, "hub", "Thưởng hằng ngày", w.now().Unix()); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return w.User(ctx, id)
}

// Settle cộng / trừ xu cho nhiều người trong một transaction (ví dụ kết quả một ván).
// Số dư không xuống dưới 0: người thua quá số đang có thì chỉ mất hết số đang có.
// Trả về số dư mới của từng người.
func (w *Wallet) Settle(ctx context.Context, game, note string, deltas map[int64]int64) (map[int64]int64, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := w.now().Unix()
	balances := map[int64]int64{}
	for id, delta := range deltas {
		var balance int64
		if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, id).Scan(&balance); err != nil {
			return nil, fmt.Errorf("người chơi %d: %w", id, err)
		}
		delta = max(delta, -balance)
		balance += delta
		balances[id] = balance
		if delta == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET balance = ? WHERE id = ?`, balance, id); err != nil {
			return nil, err
		}
		if err := addEntry(ctx, tx, id, delta, balance, game, note, now); err != nil {
			return nil, err
		}
	}
	return balances, tx.Commit()
}

// History trả về các giao dịch gần nhất của một người, mới nhất trước.
func (w *Wallet) History(ctx context.Context, id int64, limit int) ([]Entry, error) {
	rows, err := w.db.QueryContext(ctx, `SELECT delta, balance_after, game, note, created_at FROM ledger
		WHERE user_id = ? ORDER BY id DESC LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var at int64
		if err := rows.Scan(&e.Delta, &e.BalanceAfter, &e.Game, &e.Note, &at); err != nil {
			return nil, err
		}
		e.At = time.Unix(at, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (w *Wallet) Balance(ctx context.Context, id int64) (int64, error) {
	u, err := w.User(ctx, id)
	return u.Balance, err
}
