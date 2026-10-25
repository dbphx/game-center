package wallet

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Tặng xu giữa hai người chơi (ví dụ ở quảng trường): trừ người tặng, cộng người nhận và ghi sổ cái
// hai bên trong cùng một transaction. Tổng xu tặng mỗi ngày (giờ Việt Nam) của một người có hạn mức.

var (
	ErrGiftSelf  = errors.New("không tự tặng xu cho chính mình được")
	ErrGiftLimit = errors.New("hôm nay bạn đã tặng đủ hạn mức")
)

// GiftLimitError cho biết còn tặng được bao nhiêu hôm nay; errors.Is(err, ErrGiftLimit) vẫn đúng.
type GiftLimitError struct{ Left int64 }

func (e *GiftLimitError) Error() string {
	if e.Left <= 0 {
		return ErrGiftLimit.Error()
	}
	return fmt.Sprintf("hôm nay bạn chỉ còn tặng được %d xu", e.Left)
}
func (e *GiftLimitError) Is(target error) bool { return target == ErrGiftLimit }

// dayStart là unix lúc 0h (giờ Việt Nam) của ngày chứa t.
func dayStart(t time.Time) int64 {
	y, m, d := t.In(vietnam).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, vietnam).Unix()
}

// GiftedToday là tổng xu userID đã tặng hôm nay trong game (đọc từ sổ cái).
func (w *Wallet) GiftedToday(ctx context.Context, userID int64, game string) (int64, error) {
	var sum int64
	err := w.db.QueryRowContext(ctx, `SELECT coalesce(-SUM(delta), 0) FROM ledger
		WHERE user_id = ? AND game = ? AND delta < 0 AND created_at >= ?`, userID, game, dayStart(w.now())).Scan(&sum)
	return sum, err
}

// Gift chuyển amount xu từ from sang to. dailyCap > 0 giới hạn tổng xu from tặng trong ngày (theo sổ cái của game).
// Trả số dư mới của hai người.
func (w *Wallet) Gift(ctx context.Context, game string, from, to, amount, dailyCap int64, noteFrom, noteTo string) (fromBal, toBal int64, err error) {
	if from == to {
		return 0, 0, ErrGiftSelf
	}
	if amount <= 0 {
		return 0, 0, errors.New("số xu không hợp lệ")
	}
	tx, err := w.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	if dailyCap > 0 {
		var sum int64
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(-SUM(delta), 0) FROM ledger
			WHERE user_id = ? AND game = ? AND delta < 0 AND created_at >= ?`, from, game, dayStart(w.now())).Scan(&sum); err != nil {
			return 0, 0, err
		}
		if sum+amount > dailyCap {
			return 0, 0, &GiftLimitError{Left: dailyCap - sum}
		}
	}
	if _, err := tx.balance(to); err != nil {
		return 0, 0, err
	}
	if err := tx.Charge(from, amount, game, noteFrom); err != nil {
		return 0, 0, err
	}
	if err := tx.Credit(to, amount, game, noteTo); err != nil {
		return 0, 0, err
	}
	if fromBal, err = tx.balance(from); err != nil {
		return 0, 0, err
	}
	if toBal, err = tx.balance(to); err != nil {
		return 0, 0, err
	}
	return fromBal, toBal, tx.Commit()
}
