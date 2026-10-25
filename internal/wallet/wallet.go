// Package wallet quản lý tài khoản người chơi và ví xu dùng chung cho mọi game trong hub.
//
// Mỗi thay đổi số dư đều ghi một dòng vào sổ cái (ledger) trong cùng transaction,
// nên luôn tra lại được xu đến từ đâu.
package wallet

import (
	"context"
	"database/sql"
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
	ID            int64    `json:"id"`
	Username      string   `json:"username"` // rỗng với tài khoản khách cũ chưa đặt tên đăng nhập
	Name          string   `json:"name"`
	Balance       int64    `json:"balance"`
	CanDaily      bool     `json:"canDaily"` // hôm nay còn nhận thưởng được không
	Gender        string   `json:"gender"`
	Outfit        string   `json:"outfit"`
	Owned         []string `json:"owned"`      // trang phục đã có (starter + đã mua)
	Hair          string   `json:"hair"`       // kiểu tóc đang để, "default" là tóc mặc định
	Wings         string   `json:"wings"`      // cánh đang đeo, rỗng là không cánh
	OwnedHair     []string `json:"ownedHair"`  // kiểu tóc đã có ("default" + đã mua)
	OwnedWings    []string `json:"ownedWings"` // cánh đã mua
	Face          string   `json:"face"`       // gương mặt, "default" là mặt mặc định
	OwnedFace     []string `json:"ownedFace"`  // gương mặt đã có ("default" + đã mua)
	XP            int64    `json:"xp"`
	Level         int      `json:"level"`
	LevelXP       int64    `json:"levelXP"` // XP lúc bắt đầu level hiện tại
	NextXP        int64    `json:"nextXP"`  // XP cần để lên level tiếp
	HP            int      `json:"hp"`      // máu hiện tại (đã tính tự hồi)
	MaxHP         int      `json:"maxHp"`
	HospitalUntil int64    `json:"hospitalUntil"` // unix lúc xuất viện, 0 nếu không nằm viện
	Sitting       bool     `json:"sitting"`       // đang ngồi ghế (hồi máu nhanh gấp đôi)
	RegenSeconds  int      `json:"regenSeconds"`  // số giây hồi 1 HP lúc này: 60 đứng, 30 ngồi
	Now           int64    `json:"now"`           // giờ server (unix), để giao diện đếm ngược
	lastDaily     string
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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("nâng cấp database: %w", err)
	}
	return &Wallet{db: db, now: time.Now}, nil
}

func (w *Wallet) Close() error { return w.db.Close() }

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

func addEntry(ctx context.Context, tx *sql.Tx, userID, delta, after int64, game, note string, at int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO ledger (user_id, delta, balance_after, game, note, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, delta, after, game, note, at)
	return err
}

const userColumns = `id, coalesce(username, ''), name, balance, last_daily, gender, outfit, xp, hp, coalesce(hp_at, 0), hospital_until, hair, wings, face, sit_until`

func (w *Wallet) scanUser(row *sql.Row) (User, error) {
	var u User
	var s hpState
	err := row.Scan(&u.ID, &u.Username, &u.Name, &u.Balance, &u.lastDaily, &u.Gender, &u.Outfit, &u.XP, &s.hp, &s.hpAt, &s.hospitalUntil, &u.Hair, &u.Wings, &u.Face, &s.sitUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.CanDaily = u.lastDaily != w.today()
	u.Level = LevelFor(u.XP)
	u.LevelXP, u.NextXP = LevelStart(u.Level), LevelStart(u.Level+1)
	u.Now = w.nowUnix()
	u.HP, u.HospitalUntil = s.current(u.Now)
	u.Sitting = u.HospitalUntil == 0 && s.sitUntil > u.Now
	u.RegenSeconds = RegenSeconds
	if u.Sitting {
		u.RegenSeconds = RegenSeconds / SitRegenMultiplier
	}
	u.MaxHP = MaxHP
	return u, err
}

func (w *Wallet) User(ctx context.Context, id int64) (User, error) {
	return w.withOwned(ctx)(w.scanUser(w.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)))
}

// withOwned nạp danh sách trang phục, tóc, cánh, gương mặt đã có cho người dùng vừa đọc.
func (w *Wallet) withOwned(ctx context.Context) func(User, error) (User, error) {
	return func(u User, err error) (User, error) {
		if err != nil {
			return u, err
		}
		if u.Owned, err = owned(ctx, w.db, u.ID); err != nil {
			return u, err
		}
		if u.OwnedHair, err = ownedItems(ctx, w.db, u.ID, KindHair); err != nil {
			return u, err
		}
		if u.OwnedWings, err = ownedItems(ctx, w.db, u.ID, KindWings); err != nil {
			return u, err
		}
		u.OwnedFace, err = ownedItems(ctx, w.db, u.ID, KindFace)
		return u, err
	}
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
