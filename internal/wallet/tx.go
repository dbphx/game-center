package wallet

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Tx là transaction dùng chung cho các game cần đổi xu / XP cùng lúc với dữ liệu riêng của game
// (ví dụ nông trại): mọi thay đổi trong một Tx cùng thành công hoặc cùng huỷ.
type Tx struct {
	*sql.Tx
	ctx context.Context
	now int64
}

// Begin mở transaction. Gọi Commit khi xong, Rollback (an toàn khi đã Commit) để huỷ.
func (w *Wallet) Begin(ctx context.Context) (*Tx, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, ctx: ctx, now: w.now().Unix()}, nil
}

// ExecSchema chạy câu lệnh tạo bảng của game (CREATE TABLE IF NOT EXISTS ...).
func (w *Wallet) ExecSchema(ctx context.Context, schema string) error {
	_, err := w.db.ExecContext(ctx, schema)
	return err
}

// Now là đồng hồ của ví (test có thể thay).
func (w *Wallet) Now() time.Time { return w.now() }

func (t *Tx) balance(userID int64) (int64, error) {
	var b int64
	err := t.QueryRowContext(t.ctx, `SELECT balance FROM users WHERE id = ?`, userID).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return b, err
}

// Charge trừ xu và ghi sổ cái; không đủ xu thì trả ErrNotEnoughCoins, không trừ gì.
func (t *Tx) Charge(userID, amount int64, game, note string) error {
	b, err := t.balance(userID)
	if err != nil {
		return err
	}
	if b < amount {
		return ErrNotEnoughCoins
	}
	return t.change(userID, b, -amount, game, note)
}

// Credit cộng xu và ghi sổ cái.
func (t *Tx) Credit(userID, amount int64, game, note string) error {
	b, err := t.balance(userID)
	if err != nil {
		return err
	}
	return t.change(userID, b, amount, game, note)
}

func (t *Tx) change(userID, balance, delta int64, game, note string) error {
	if delta == 0 {
		return nil
	}
	balance += delta
	if _, err := t.ExecContext(t.ctx, `UPDATE users SET balance = ? WHERE id = ?`, balance, userID); err != nil {
		return err
	}
	return addEntry(t.ctx, t.Tx, userID, delta, balance, game, note, t.now)
}

// AddXP cộng kinh nghiệm.
func (t *Tx) AddXP(userID, xp int64) error {
	_, err := t.ExecContext(t.ctx, `UPDATE users SET xp = xp + ? WHERE id = ?`, max(0, xp), userID)
	return err
}

// Level là level hiện tại của người chơi (tính theo XP trong transaction).
func (t *Tx) Level(userID int64) (int, error) {
	var xp int64
	err := t.QueryRowContext(t.ctx, `SELECT xp FROM users WHERE id = ?`, userID).Scan(&xp)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return LevelFor(xp), err
}
