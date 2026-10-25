package wallet

import (
	"context"
	"database/sql"
	"errors"
	"sync"
)

// Mỗi tài khoản chỉ có một phiên: đăng nhập (hoặc đăng ký) thành công thì mọi phiên cũ bị xoá và ghi vào
// revoked_sessions, để token cũ dùng lại được báo "đăng nhập ở nơi khác" thay vì "phiên không hợp lệ".
// Các kết nối WebSocket đang mở theo dõi WatchRevoke để ngắt ngay.

// RevokedKeepSeconds là thời gian nhớ token bị đá ra (sau đó token cũ chỉ còn là "không hợp lệ").
const RevokedKeepSeconds = 30 * 24 * 3600

// ErrSessionElsewhere: token đã bị thay bởi một lần đăng nhập khác. errors.Is(err, ErrNotFound) vẫn đúng
// nên các chỗ đang trả 401 cho ErrNotFound không cần sửa.
var ErrSessionElsewhere error = elsewhereError{}

type elsewhereError struct{}

func (elsewhereError) Error() string        { return "Tài khoản đã đăng nhập ở nơi khác" }
func (elsewhereError) Is(target error) bool { return target == ErrNotFound }

// Elsewhere để các hub WebSocket (không import wallet) nhận ra lỗi này qua interface{ Elsewhere() bool }.
func (elsewhereError) Elsewhere() bool { return true }

// IsElsewhere cho biết lỗi xác thực là do tài khoản vừa đăng nhập ở nơi khác.
func IsElsewhere(err error) bool { return errors.Is(err, ErrSessionElsewhere) }

// revokeSessions xoá mọi phiên của userID trừ keepHash (đang trong transaction đăng nhập),
// ghi lại token bị đá ra. Trả số phiên đã xoá.
func (w *Wallet) revokeSessions(ctx context.Context, tx *sql.Tx, userID int64, keepHash string) (int64, error) {
	now := w.now().Unix()
	if _, err := tx.ExecContext(ctx, `DELETE FROM revoked_sessions WHERE revoked_at < ?`, now-RevokedKeepSeconds); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO revoked_sessions (token_hash, user_id, revoked_at)
		SELECT token_hash, user_id, ? FROM sessions WHERE user_id = ? AND token_hash != ?`, now, userID, keepHash); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND token_hash != ?`, userID, keepHash)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// tokenRevoked cho biết token này đã bị một lần đăng nhập khác đá ra.
func (w *Wallet) tokenRevoked(ctx context.Context, token string) (bool, error) {
	var n int
	err := w.db.QueryRowContext(ctx, `SELECT count(*) FROM revoked_sessions WHERE token_hash = ?`, hashToken(token)).Scan(&n)
	return n > 0, err
}

// SessionElsewhere cho biết token không còn dùng được vì tài khoản đã đăng nhập ở nơi khác
// (dùng để thêm code "elsewhere" vào các trả lời 401).
func (w *Wallet) SessionElsewhere(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	ok, err := w.tokenRevoked(ctx, token)
	return err == nil && ok
}

// revokeWatch giữ các kênh theo dõi theo người dùng; kênh bị đóng khi phiên của người đó bị đá ra.
type revokeWatch struct {
	mu   sync.Mutex
	subs map[int64]map[chan struct{}]struct{}
}

// WatchRevoke trả kênh bị đóng khi tài khoản userID đăng nhập ở nơi khác (các phiên cũ bị đá ra),
// và hàm huỷ theo dõi (gọi khi kết nối đóng).
func (w *Wallet) WatchRevoke(userID int64) (<-chan struct{}, func()) {
	r := &w.revokes
	ch := make(chan struct{})
	r.mu.Lock()
	if r.subs == nil {
		r.subs = map[int64]map[chan struct{}]struct{}{}
	}
	if r.subs[userID] == nil {
		r.subs[userID] = map[chan struct{}]struct{}{}
	}
	r.subs[userID][ch] = struct{}{}
	r.mu.Unlock()
	return ch, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if set := r.subs[userID]; set != nil {
			delete(set, ch)
			if len(set) == 0 {
				delete(r.subs, userID)
			}
		}
	}
}

// fireRevoke đóng mọi kênh đang theo dõi userID (kết nối đó dùng token cũ).
func (w *Wallet) fireRevoke(userID int64) {
	r := &w.revokes
	r.mu.Lock()
	set := r.subs[userID]
	delete(r.subs, userID)
	r.mu.Unlock()
	for ch := range set {
		close(ch)
	}
}
