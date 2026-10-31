// Package friends là danh sách bạn bè của DCity: gửi lời mời theo tên đăng nhập, chấp nhận / từ chối, huỷ kết bạn.
// Bạn bè được vào xem và trộm nông trại của nhau (luật trộm ở games/farm/steal.go).
package friends

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dbphx/game-center/games/farm"
	"github.com/dbphx/game-center/internal/wallet"
)

const GameID = "friends"

const MaxFriends = 100

// Mỗi lời mời là một dòng from → to 'pending'; kết bạn xong thì có hai dòng 'accepted' (mỗi chiều một dòng).
const schema = `
CREATE TABLE IF NOT EXISTS friends (
	user_id    INTEGER NOT NULL REFERENCES users(id),
	friend_id  INTEGER NOT NULL REFERENCES users(id),
	status     TEXT NOT NULL,           -- 'pending' (user_id mời friend_id) hoặc 'accepted'
	created_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, friend_id)
);
CREATE INDEX IF NOT EXISTS friends_to ON friends(friend_id, status);
`

// Error là lỗi hiện cho người chơi, kèm mã HTTP.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func bad(msg string) error      { return &Error{http.StatusBadRequest, msg} }
func conflict(msg string) error { return &Error{http.StatusConflict, msg} }
func notFound(msg string) error { return &Error{http.StatusNotFound, msg} }
func forbidden(msg string) error {
	return &Error{http.StatusForbidden, msg}
}

// Person là một người trong danh sách: tên hiển thị và ngoại hình (không có tên đăng nhập).
type Person struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
	Hair   string `json:"hair"`
	Wings  string `json:"wings"`
	Face   string `json:"face"`
}

// Friend là một người bạn, kèm tóm tắt nông trại để biết có gì mà trộm.
type Friend struct {
	Person
	Online      bool `json:"online"`
	Ripe        int  `json:"ripe"`
	DogGuarding bool `json:"dogGuarding"`
}

// List là danh sách gửi cho giao diện.
type List struct {
	Friends  []Friend `json:"friends"`
	Incoming []Person `json:"incoming"` // người khác mời mình
	Outgoing []Person `json:"outgoing"` // mình đã mời, chờ trả lời
}

// FarmView là nông trại của một người bạn (chỉ xem).
type FarmView struct {
	Owner  Person `json:"owner"`
	Friend bool   `json:"friend"` // người xem là bạn của chủ nhà: chỉ khi đó mới trộm được
	farm.Visit
}

// StealReply là kết quả một lần trộm kèm nông trại bạn sau khi trộm.
type StealReply struct {
	farm.StealResult
	Farm FarmView `json:"farm"`
}

type Friends struct {
	w    *wallet.Wallet
	farm *farm.Farm
	now  func() time.Time
}

func New(ctx context.Context, w *wallet.Wallet, f *farm.Farm) (*Friends, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng bạn bè: %w", err)
	}
	return &Friends{w: w, farm: f, now: time.Now}, nil
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const personColumns = `u.id, u.name, u.xp, u.gender, u.outfit, u.hair, u.wings, u.face`

func scanPeople(rows *sql.Rows) ([]Person, error) {
	defer rows.Close()
	list := []Person{}
	for rows.Next() {
		var p Person
		var xp int64
		if err := rows.Scan(&p.ID, &p.Name, &xp, &p.Gender, &p.Outfit, &p.Hair, &p.Wings, &p.Face); err != nil {
			return nil, err
		}
		p.Level = wallet.LevelFor(xp)
		list = append(list, p)
	}
	return list, rows.Err()
}

func person(ctx context.Context, q querier, id int64) (Person, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+personColumns+` FROM users u WHERE u.id = ?`, id)
	if err != nil {
		return Person{}, err
	}
	list, err := scanPeople(rows)
	if err != nil {
		return Person{}, err
	}
	if len(list) == 0 {
		return Person{}, notFound("không tìm thấy người chơi")
	}
	return list[0], nil
}

// status trả trạng thái dòng from → to ("" nếu không có).
func status(ctx context.Context, q querier, from, to int64) (string, error) {
	var s string
	err := q.QueryRowContext(ctx, `SELECT status FROM friends WHERE user_id = ? AND friend_id = ?`, from, to).Scan(&s)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return s, err
}

func count(ctx context.Context, q querier, uid int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM friends WHERE user_id = ? AND status = 'accepted'`, uid).Scan(&n)
	return n, err
}

// AreFriends: a và b đã là bạn bè.
func (f *Friends) AreFriends(ctx context.Context, a, b int64) (bool, error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	s, err := status(ctx, tx, a, b)
	return s == "accepted", err
}

// List trả bạn bè, lời mời đến và lời mời đã gửi của uid.
func (f *Friends) List(ctx context.Context, uid int64) (List, error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return List{}, err
	}
	defer tx.Rollback()
	l, err := f.list(ctx, tx, uid)
	if err != nil {
		return List{}, err
	}
	if err := tx.Commit(); err != nil {
		return List{}, err
	}
	// Tóm tắt nông trại chạy transaction riêng (SQLite một kết nối: không lồng transaction).
	ids := make([]int64, len(l.Friends))
	for i, fr := range l.Friends {
		ids[i] = fr.ID
	}
	sum, err := f.farm.Summaries(ctx, uid, ids)
	if err != nil {
		return List{}, err
	}
	for i := range l.Friends {
		s := sum[l.Friends[i].ID]
		l.Friends[i].Ripe, l.Friends[i].DogGuarding = s.Ripe, s.DogGuarding
	}
	return l, nil
}

func (f *Friends) list(ctx context.Context, q querier, uid int64) (List, error) {
	var l List
	query := func(sql string) ([]Person, error) {
		rows, err := q.QueryContext(ctx, sql, uid)
		if err != nil {
			return nil, err
		}
		return scanPeople(rows)
	}
	people, err := query(`SELECT ` + personColumns + ` FROM friends f JOIN users u ON u.id = f.friend_id
		WHERE f.user_id = ? AND f.status = 'accepted' ORDER BY u.name COLLATE NOCASE, u.id`)
	if err != nil {
		return List{}, err
	}
	l.Friends = make([]Friend, len(people))
	for i, p := range people {
		l.Friends[i] = Friend{Person: p}
	}
	if l.Incoming, err = query(`SELECT ` + personColumns + ` FROM friends f JOIN users u ON u.id = f.user_id
		WHERE f.friend_id = ? AND f.status = 'pending' ORDER BY f.created_at DESC, u.id`); err != nil {
		return List{}, err
	}
	if l.Outgoing, err = query(`SELECT ` + personColumns + ` FROM friends f JOIN users u ON u.id = f.friend_id
		WHERE f.user_id = ? AND f.status = 'pending' ORDER BY f.created_at DESC, u.id`); err != nil {
		return List{}, err
	}
	return l, nil
}

// do chạy một thao tác trên bảng bạn bè trong transaction rồi trả danh sách mới.
func (f *Friends) do(ctx context.Context, uid int64, op func(tx *wallet.Tx, now int64) error) (List, error) {
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return List{}, err
	}
	defer tx.Rollback()
	if err := op(tx, f.now().Unix()); err != nil {
		return List{}, err
	}
	if err := tx.Commit(); err != nil {
		return List{}, err
	}
	return f.List(ctx, uid)
}

// befriend ghi hai dòng 'accepted' (thay lời mời nếu có), kiểm tra giới hạn số bạn của cả hai.
func befriend(ctx context.Context, tx *wallet.Tx, a, b, now int64) error {
	for _, id := range []int64{a, b} {
		n, err := count(ctx, tx, id)
		if err != nil {
			return err
		}
		if n >= MaxFriends {
			if id == a {
				return conflict(fmt.Sprintf("bạn đã có đủ %d bạn", MaxFriends))
			}
			return conflict(fmt.Sprintf("người này đã có đủ %d bạn", MaxFriends))
		}
	}
	for _, pair := range [][2]int64{{a, b}, {b, a}} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO friends (user_id, friend_id, status, created_at) VALUES (?, ?, 'accepted', ?)
			ON CONFLICT (user_id, friend_id) DO UPDATE SET status = 'accepted', created_at = excluded.created_at`, pair[0], pair[1], now); err != nil {
			return err
		}
	}
	return nil
}

// Request gửi lời mời kết bạn tới người có tên đăng nhập username (không phân biệt hoa thường) hoặc id.
// Người kia đã mời mình trước thì thành bạn luôn.
func (f *Friends) Request(ctx context.Context, uid int64, username string, id int64) (List, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, now int64) error {
		to := id
		if name := wallet.NormalizeUsername(username); name != "" {
			err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, name).Scan(&to)
			if errors.Is(err, sql.ErrNoRows) {
				return notFound("không có người chơi tên " + strings.TrimSpace(username))
			}
			if err != nil {
				return err
			}
		} else if to == 0 {
			return bad("nhập tên đăng nhập của người muốn kết bạn")
		} else if _, err := person(ctx, tx, to); err != nil {
			return err
		}
		if to == uid {
			return bad("không thể tự kết bạn với mình")
		}
		mine, err := status(ctx, tx, uid, to)
		if err != nil {
			return err
		}
		switch mine {
		case "accepted":
			return conflict("hai bạn đã là bạn bè")
		case "pending":
			return conflict("bạn đã gửi lời mời rồi, chờ người ấy trả lời")
		}
		theirs, err := status(ctx, tx, to, uid)
		if err != nil {
			return err
		}
		if theirs == "pending" {
			return befriend(ctx, tx, uid, to, now)
		}
		if n, err := count(ctx, tx, uid); err != nil {
			return err
		} else if n >= MaxFriends {
			return conflict(fmt.Sprintf("bạn đã có đủ %d bạn", MaxFriends))
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO friends (user_id, friend_id, status, created_at) VALUES (?, ?, 'pending', ?)`, uid, to, now)
		return err
	})
}

// Accept chấp nhận lời mời của from.
func (f *Friends) Accept(ctx context.Context, uid, from int64) (List, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, now int64) error {
		s, err := status(ctx, tx, from, uid)
		if err != nil {
			return err
		}
		if s != "pending" {
			return notFound("không có lời mời này")
		}
		return befriend(ctx, tx, uid, from, now)
	})
}

// Decline từ chối lời mời của from (hoặc rút lại lời mời mình đã gửi cho from).
func (f *Friends) Decline(ctx context.Context, uid, from int64) (List, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, _ int64) error {
		r, err := tx.ExecContext(ctx, `DELETE FROM friends WHERE status = 'pending'
			AND ((user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?))`, from, uid, uid, from)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return notFound("không có lời mời này")
		}
		return nil
	})
}

// Remove huỷ kết bạn với other (xoá cả hai chiều).
func (f *Friends) Remove(ctx context.Context, uid, other int64) (List, error) {
	return f.do(ctx, uid, func(tx *wallet.Tx, _ int64) error {
		r, err := tx.ExecContext(ctx, `DELETE FROM friends WHERE status = 'accepted'
			AND ((user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?))`, uid, other, other, uid)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return notFound("hai bạn chưa là bạn bè")
		}
		return nil
	})
}

// FinishSteal / Steal vẫn chỉ cho bạn bè.
func (f *Friends) requireFriend(ctx context.Context, uid, other int64) (Person, error) {
	ok, err := f.AreFriends(ctx, uid, other)
	if err != nil {
		return Person{}, err
	}
	if !ok {
		return Person{}, forbidden("chỉ vào được nông trại của bạn bè")
	}
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return Person{}, err
	}
	defer tx.Rollback()
	return person(ctx, tx, other)
}

// Farm trả nông trại của other cho uid xem. Ai cũng xem được (vd từ Khu danh vọng); chỉ bạn bè mới có lượt trộm.
func (f *Friends) Farm(ctx context.Context, uid, other int64) (FarmView, error) {
	if uid == other {
		return FarmView{}, bad("đây là nông trại của bạn")
	}
	friend, err := f.AreFriends(ctx, uid, other)
	if err != nil {
		return FarmView{}, err
	}
	tx, err := f.w.Begin(ctx)
	if err != nil {
		return FarmView{}, err
	}
	owner, err := person(ctx, tx, other)
	tx.Rollback()
	if err != nil {
		return FarmView{}, err
	}
	v, err := f.farm.Visit(ctx, other, uid)
	if err != nil {
		return FarmView{}, err
	}
	if !friend {
		v.StealsLeft = 0
	}
	return FarmView{Owner: owner, Friend: friend, Visit: v}, nil
}

// FinishSteal chốt cuộc chó đuổi chase khi uid trộm ở nhà bạn other: escaped = đã chạy ra tới cổng.
func (f *Friends) FinishSteal(ctx context.Context, uid, other int64, chase string, escaped bool) (StealReply, error) {
	owner, err := f.requireFriend(ctx, uid, other)
	if err != nil {
		return StealReply{}, err
	}
	res, err := f.farm.FinishSteal(ctx, uid, other, chase, escaped)
	if err != nil {
		return StealReply{}, err
	}
	return StealReply{StealResult: res, Farm: FarmView{Owner: owner, Visit: res.Visit}}, nil
}

// Steal: uid trộm ô plot trong nông trại của bạn other.
func (f *Friends) Steal(ctx context.Context, uid, other int64, plot int) (StealReply, error) {
	owner, err := f.requireFriend(ctx, uid, other)
	if err != nil {
		return StealReply{}, err
	}
	res, err := f.farm.Steal(ctx, uid, other, plot)
	if err != nil {
		return StealReply{}, err
	}
	return StealReply{StealResult: res, Farm: FarmView{Owner: owner, Visit: res.Visit}}, nil
}
