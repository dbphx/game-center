package farm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dbphx/game-center/internal/wallet"
)

// ---------- Chó giữ nhà ----------

// Mỗi nông trại có một con chó. Cho ăn một phần thức ăn cho chó thì no DogFullFor giây; trong lúc no chó canh nhà
// (bạn bè vào trộm có thể bị bắt), đói thì nằm ngủ, không canh.
const (
	DogFeedID    = "feed_dog"
	DogFullFor   = 4 * 3600 // 4 giờ no sau mỗi lần ăn
	dogRefeedGap = 3600     // còn no dưới 1 giờ thì được cho ăn tiếp
)

// Bảng chó, ô bị trộm và nhật ký trộm (tạo trong New cùng các bảng nông trại).
const stealSchema = `
CREATE TABLE IF NOT EXISTS farm_dog (
	user_id   INTEGER PRIMARY KEY REFERENCES users(id),
	fed_until INTEGER NOT NULL DEFAULT 0
);
-- Một ô chỉ bị trộm một lần mỗi vụ: khoá theo lúc gieo của vụ đó.
CREATE TABLE IF NOT EXISTS farm_stolen (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	idx        INTEGER NOT NULL,
	planted_at INTEGER NOT NULL,
	thief_id   INTEGER NOT NULL REFERENCES users(id),
	PRIMARY KEY (user_id, idx, planted_at)
);
CREATE TABLE IF NOT EXISTS farm_thefts (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	owner_id   INTEGER NOT NULL REFERENCES users(id),
	thief_id   INTEGER NOT NULL REFERENCES users(id),
	idx        INTEGER NOT NULL,
	planted_at INTEGER NOT NULL DEFAULT 0, -- vụ của ô lúc thử trộm: mỗi người chỉ thử một ô một lần mỗi vụ
	item       TEXT NOT NULL,
	qty        INTEGER NOT NULL,
	caught     INTEGER NOT NULL DEFAULT 0, -- 1 = bị chó đuổi kịp, không lấy được gì
	hp_lost    INTEGER NOT NULL DEFAULT 0, -- máu kẻ trộm bị trừ khi bị chó cắn
	at         INTEGER NOT NULL,
	-- Chó đang canh thì lần trộm là một cuộc đuổi: chờ kẻ trộm báo chạy thoát ra cổng (hoặc bị bắt) trước expires_ms.
	pending    INTEGER NOT NULL DEFAULT 0, -- 1 = đang bị đuổi, chưa có kết quả
	token      TEXT NOT NULL DEFAULT '',
	started_ms INTEGER NOT NULL DEFAULT 0,
	expires_ms INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS farm_thefts_pending ON farm_thefts(pending, expires_ms);
CREATE INDEX IF NOT EXISTS farm_thefts_owner ON farm_thefts(owner_id, id);
CREATE INDEX IF NOT EXISTS farm_thefts_thief ON farm_thefts(thief_id, owner_id, at);
CREATE INDEX IF NOT EXISTS farm_thefts_try ON farm_thefts(owner_id, idx, planted_at, thief_id);
`

// Dog là trạng thái chó của một nông trại.
type Dog struct {
	FedUntil int64 `json:"fedUntil"`
	Guarding bool  `json:"guarding"` // còn no (now < fedUntil) thì canh nhà
	FullFor  int64 `json:"fullFor"`
}

func loadDog(ctx context.Context, q querier, uid, now int64) (Dog, error) {
	d := Dog{FullFor: DogFullFor}
	err := q.QueryRowContext(ctx, `SELECT fed_until FROM farm_dog WHERE user_id = ?`, uid).Scan(&d.FedUntil)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Dog{}, err
	}
	d.Guarding = now < d.FedUntil
	return d, nil
}

// FeedDog cho chó ăn một phần thức ăn cho chó trong kho giống: no 4 giờ kể từ bây giờ.
// Chỉ cho ăn khi chó đói hoặc còn no dưới 1 giờ.
func (f *Farm) FeedDog(ctx context.Context, uid int64) (State, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, s State) error {
		now := f.now().Unix()
		if left := s.Dog.FedUntil - now; left >= dogRefeedGap {
			return conflict(fmt.Sprintf("Chó còn no, cho ăn lại sau %s", fmtWait(left-dogRefeedGap+1)))
		}
		if s.Storage.Seeds.Items[DogFeedID] < 1 {
			return conflict("Hết " + dogFeedName() + ", mua thêm ở cửa hàng")
		}
		if err := addItem(ctx, tx, uid, storeSeeds, DogFeedID, -1); err != nil {
			return err
		}
		if err := tx.AddXP(uid, 1); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO farm_dog (user_id, fed_until) VALUES (?, ?)
			ON CONFLICT (user_id) DO UPDATE SET fed_until = excluded.fed_until`, uid, now+DogFullFor)
		return err
	})
}

func dogFeedName() string {
	if fd, ok := findFeed(DogFeedID); ok {
		return fd.Name
	}
	return "Thức ăn cho chó"
}
