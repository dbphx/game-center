// Package baucua là game Bầu Cua Tôm Cá của DCity. Có sòng chung "chung" (máy luôn làm nhà cái) và các bàn riêng
// người chơi tự mở để chơi với bạn; ở bàn riêng một người có thể nộp quỹ để làm cái thay máy.
// Vòng chạy liên tục theo đồng hồ: đặt cược 20 giây → lắc 4 giây → mở bát 6 giây. Mọi khoản xu đi qua ví chung
// (sổ cái game "baucua"); cược trừ ngay lúc đặt, thắng được trả lúc mở bát.
//
// Vòng tính "lười" theo thời gian: số vòng và pha suy ra từ đồng hồ (mọi bàn dùng chung mốc), kết quả của một vòng
// được quay khi hết giờ cược và trả thưởng ở request đầu tiên sau khi mở bát (dưới khoá, trong một transaction),
// nên mỗi bàn mỗi vòng chỉ quay và trả đúng một lần.
package baucua

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dbphx/game-center/internal/chat"
	"github.com/dbphx/game-center/internal/wallet"
)

const GameID = "baucua"

// Thời lượng các pha của một vòng.
const (
	BettingTime = 20 * time.Second
	RollingTime = 4 * time.Second
	ResultTime  = 6 * time.Second
	RoundTime   = BettingTime + RollingTime + ResultTime
)

const (
	PhaseBetting = "betting"
	PhaseRolling = "rolling"
	PhaseResult  = "result"
)

// MaxTotal là tổng cược tối đa của một người trong một vòng.
const MaxTotal = 20000

// Chips là các mức chip được đặt.
var Chips = []int64{10, 50, 100, 500, 1000}

// Face là một mặt xúc xắc.
type Face struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

var Faces = []Face{{"bau", "Bầu"}, {"cua", "Cua"}, {"tom", "Tôm"}, {"ca", "Cá"}, {"ga", "Gà"}, {"nai", "Nai"}}

func faceName(id string) (string, bool) {
	for _, f := range Faces {
		if f.ID == id {
			return f.Name, true
		}
	}
	return "", false
}

// XP mỗi vòng: có đặt cược (hoặc làm cái vòng có cược) được XPPlay, có lời thì thêm XPWin.
const (
	XPPlay = 2
	XPWin  = 3
)

// Bảng nằm chung database với ví để trừ / cộng xu cùng transaction với cược và quỹ cái.
// baucua_bets: settled = 1 khi vòng đã chốt; cược được hoàn (huỷ, khởi động lại) thì xoá.
// baucua_banks: quỹ của người đang làm cái từng bàn; khởi động lại thì hoàn về ví (bàn riêng chỉ sống trong bộ nhớ).
const schema = `
CREATE TABLE IF NOT EXISTS baucua_bets (
	id      INTEGER PRIMARY KEY AUTOINCREMENT,
	round   INTEGER NOT NULL,
	user_id INTEGER NOT NULL REFERENCES users(id),
	face    TEXT NOT NULL,
	amount  INTEGER NOT NULL,
	settled INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS baucua_bets_round ON baucua_bets(round, user_id);
CREATE INDEX IF NOT EXISTS baucua_bets_open ON baucua_bets(settled, round);
CREATE TABLE IF NOT EXISTS baucua_banks (
	room     TEXT PRIMARY KEY,
	user_id  INTEGER NOT NULL REFERENCES users(id),
	bankroll INTEGER NOT NULL,
	start    INTEGER NOT NULL
);
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

func errStatus(err error) int {
	var ge *Error
	switch {
	case errors.As(err, &ge):
		return ge.Status
	case errors.Is(err, wallet.ErrNotEnoughCoins):
		return http.StatusPaymentRequired
	case errors.Is(err, wallet.ErrInHospital):
		return http.StatusLocked
	case errors.Is(err, wallet.ErrLowHP):
		return http.StatusForbidden
	case errors.Is(err, wallet.ErrNotFound):
		return http.StatusUnauthorized
	}
	return http.StatusInternalServerError
}

// Game là các sòng bầu cua (sòng chung + bàn riêng).
type Game struct {
	w    *wallet.Wallet
	now  func() time.Time
	roll func() [3]string // quay 3 xúc xắc (test thay được)
	code func() string    // sinh mã bàn (test thay được)

	mu        sync.Mutex
	rooms     map[string]*room
	lastSweep time.Time
}

const historyKeep = 10

func randomRoll() [3]string {
	var d [3]string
	for i := range d {
		d[i] = Faces[rand.IntN(len(Faces))].ID
	}
	return d
}

// New tạo sòng chung, thêm cột bàn cho bảng cược cũ và hoàn các cược / quỹ cái còn treo (server tắt giữa vòng).
func New(ctx context.Context, w *wallet.Wallet) (*Game, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng bầu cua: %w", err)
	}
	if err := addRoomColumn(ctx, w); err != nil {
		return nil, fmt.Errorf("thêm cột bàn: %w", err)
	}
	g := &Game{w: w, now: time.Now, roll: randomRoll, code: randomCode, rooms: map[string]*room{}}
	g.rooms[Lobby] = newRoom(Lobby, 0, time.Time{})
	if err := g.refundOpen(ctx); err != nil {
		return nil, fmt.Errorf("hoàn cược treo: %w", err)
	}
	return g, nil
}

// addRoomColumn thêm cột room cho bảng cược tạo từ bản chỉ có sòng chung.
func addRoomColumn(ctx context.Context, w *wallet.Wallet) error {
	tx, err := w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('baucua_bets') WHERE name = 'room'`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE baucua_bets ADD COLUMN room TEXT NOT NULL DEFAULT 'chung'`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS baucua_bets_room ON baucua_bets(room, round, user_id)`); err != nil {
		return err
	}
	return tx.Commit()
}

// refundOpen hoàn mọi cược chưa chốt (mỗi người mỗi vòng một dòng sổ cái) và mọi quỹ cái còn giữ.
func (g *Game) refundOpen(ctx context.Context) error {
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type refund struct {
		round, uid, amount int64
		room               string
	}
	var bets, banks []refund
	err = queryRows(ctx, tx, `SELECT room, round, user_id, SUM(amount) FROM baucua_bets WHERE settled = 0 GROUP BY room, round, user_id`, nil,
		func(scan func(...any) error) error {
			var r refund
			if err := scan(&r.room, &r.round, &r.uid, &r.amount); err != nil {
				return err
			}
			bets = append(bets, r)
			return nil
		})
	if err != nil {
		return err
	}
	err = queryRows(ctx, tx, `SELECT room, user_id, bankroll FROM baucua_banks`, nil, func(scan func(...any) error) error {
		var r refund
		if err := scan(&r.room, &r.uid, &r.amount); err != nil {
			return err
		}
		banks = append(banks, r)
		return nil
	})
	if err != nil {
		return err
	}
	for _, r := range bets {
		if err := tx.Credit(r.uid, r.amount, GameID, fmt.Sprintf("Bầu cua vòng %d: hoàn cược (máy chủ khởi động lại)", r.round)); err != nil {
			return err
		}
	}
	for _, r := range banks {
		if err := tx.Credit(r.uid, r.amount, GameID, "Bầu cua: hoàn quỹ làm cái (máy chủ khởi động lại)"); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM baucua_bets WHERE settled = 0`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM baucua_banks`); err != nil {
		return err
	}
	return tx.Commit()
}

// queryRows chạy truy vấn và gọi each cho từng dòng.
func queryRows(ctx context.Context, tx *wallet.Tx, query string, args []any, each func(scan func(...any) error) error) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := each(rows.Scan); err != nil {
			return err
		}
	}
	return rows.Err()
}

// clock là vị trí hiện tại trong vòng.
type clock struct {
	round  int64
	phase  string
	endsAt time.Time // lúc hết pha hiện tại
	start  time.Time // lúc bắt đầu vòng
}

func (g *Game) clockAt(t time.Time) clock {
	ms := t.UnixMilli()
	cycle := RoundTime.Milliseconds()
	round := ms / cycle
	start := time.UnixMilli(round * cycle)
	c := clock{round: round, start: start}
	switch off := t.Sub(start); {
	case off < BettingTime:
		c.phase, c.endsAt = PhaseBetting, start.Add(BettingTime)
	case off < BettingTime+RollingTime:
		c.phase, c.endsAt = PhaseRolling, start.Add(BettingTime+RollingTime)
	default:
		c.phase, c.endsAt = PhaseResult, start.Add(RoundTime)
	}
	return c
}

// sync quay vòng đã hết giờ cược, chốt mọi vòng đã mở bát mà chưa chốt của một bàn, rồi xử lý người làm cái
// muốn thôi / hết quỹ / vắng mặt. Gọi khi đang giữ g.mu.
func (g *Game) sync(ctx context.Context, r *room, c clock) error {
	// Vòng hiện tại hết giờ cược thì quay luôn (kể cả không ai đặt) để có lịch sử.
	if c.phase != PhaseBetting {
		r.rollRound(c.round, g.roll)
	}
	// Chỉ trả thưởng vòng hiện tại khi đã mở bát: trả sớm lúc đang lắc thì số dư nhảy trước, lộ kết quả.
	last := c.round - 1
	if c.phase == PhaseResult {
		last = c.round
	}
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return err
	}
	var rounds []int64
	err = queryRows(ctx, tx, `SELECT DISTINCT round FROM baucua_bets WHERE room = ? AND settled = 0 AND round <= ? ORDER BY round`,
		[]any{r.code, last}, func(scan func(...any) error) error {
			var n int64
			if err := scan(&n); err != nil {
				return err
			}
			rounds = append(rounds, n)
			return nil
		})
	tx.Rollback()
	if err != nil {
		return err
	}
	for _, n := range rounds {
		if err := g.settle(ctx, r, n, r.rollRound(n, g.roll)); err != nil {
			return err
		}
	}
	return g.checkBanker(ctx, r, c)
}

// payout là số xu nhận về cho cược amount vào mặt face: mặt ra k lần thì được vốn + k lần cược.
func payout(face string, amount int64, dice [3]string) int64 {
	k := int64(0)
	for _, d := range dice {
		if d == face {
			k++
		}
	}
	if k == 0 {
		return 0
	}
	return amount * (1 + k)
}

func diceText(d [3]string) string {
	names := make([]string, len(d))
	for i, id := range d {
		names[i], _ = faceName(id)
	}
	return strings.Join(names, "-")
}

// settle trả thưởng một vòng của một bàn trong một transaction: mỗi người thắng một dòng sổ cái, cộng XP.
// Bàn có người làm cái thì tiền thua vào quỹ, tiền thắng lấy từ quỹ.
func (g *Game) settle(ctx context.Context, r *room, round int64, dice [3]string) error {
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	type sum struct{ bet, won int64 }
	users := map[int64]*sum{}
	var order []int64
	err = queryRows(ctx, tx, `SELECT user_id, face, amount FROM baucua_bets WHERE room = ? AND round = ? AND settled = 0`,
		[]any{r.code, round}, func(scan func(...any) error) error {
			var uid, amount int64
			var face string
			if err := scan(&uid, &face, &amount); err != nil {
				return err
			}
			s := users[uid]
			if s == nil {
				s = &sum{}
				users[uid] = s
				order = append(order, uid)
			}
			s.bet += amount
			s.won += payout(face, amount, dice)
			return nil
		})
	if err != nil {
		return err
	}
	var bankNet int64 // lãi / lỗ của quỹ cái trong vòng
	for _, uid := range order {
		s := users[uid]
		bankNet += s.bet - s.won
		if s.won > 0 {
			if err := tx.Credit(uid, s.won, GameID, fmt.Sprintf("Bầu cua vòng %d: ra %s, nhận %d", round, diceText(dice), s.won)); err != nil {
				return err
			}
		}
		xp := int64(XPPlay)
		if s.won > s.bet {
			xp += XPWin
		}
		if err := tx.AddXP(uid, xp); err != nil {
			return err
		}
	}
	b := r.banker
	banked := b != nil && len(order) > 0
	var bankroll int64
	if banked {
		bankroll = b.bankroll + bankNet
		if _, err := tx.ExecContext(ctx, `UPDATE baucua_banks SET bankroll = ? WHERE room = ?`, bankroll, r.code); err != nil {
			return err
		}
		xp := int64(XPPlay)
		if bankNet > 0 {
			xp += XPWin
		}
		if err := tx.AddXP(b.uid, xp); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE baucua_bets SET settled = 1 WHERE room = ? AND round = ? AND settled = 0`, r.code, round); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if banked {
		b.bankroll = bankroll
		r.bankNet[round] = bankNet
	}
	return nil
}

// ---------- Trạng thái ----------

// Who là hình nhân vật của một người chơi (đủ để vẽ chibi).
type Who struct {
	Name   string `json:"name"`
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
	Hair   string `json:"hair"`
	Wings  string `json:"wings"`
	Face   string `json:"face"`
	Level  int    `json:"level"`
}

type Mine struct {
	Bets   map[string]int64 `json:"bets"`
	Total  int64            `json:"total"`
	Won    *int64           `json:"won,omitempty"` // chỉ có khi mở bát: số xu nhận về
	Net    *int64           `json:"net,omitempty"`
	Banker bool             `json:"banker"` // mình đang làm cái bàn này
	Host   bool             `json:"host"`   // mình là chủ bàn
}

type Player struct {
	Name   string `json:"name"`
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
	Level  int    `json:"level"`
	Total  int64  `json:"total"`
}

// Banker là người đang làm cái.
type Banker struct {
	Who
	Bankroll int64  `json:"bankroll"`
	Start    int64  `json:"start"`         // quỹ lúc bắt đầu làm cái
	Net      *int64 `json:"net,omitempty"` // lãi / lỗ của quỹ trong vòng vừa mở bát
	Leaving  bool   `json:"leaving"`       // đã xin thôi, thôi sau khi chốt vòng này
}

// Member là người đang có mặt ở bàn, kèm tổng cược vòng này.
type Member struct {
	Who
	Total int64 `json:"total"`
}

type RoomInfo struct {
	Code          string   `json:"code"`
	Host          *Who     `json:"host"`   // nil với sòng chung
	Banker        *Banker  `json:"banker"` // nil = máy làm cái
	Members       []Member `json:"members"`
	MaxRoundTotal int64    `json:"maxRoundTotal"` // tổng cược tối đa cả bàn vòng này; 0 = không giới hạn (máy làm cái)
	RoundTotal    int64    `json:"roundTotal"`
}

type State struct {
	Round    int64            `json:"round"`
	Phase    string           `json:"phase"`
	EndsAt   int64            `json:"endsAt"`
	Now      int64            `json:"now"`
	Dice     []string         `json:"dice"`
	Totals   map[string]int64 `json:"totals"`
	Players  []Player         `json:"players"`
	Mine     Mine             `json:"mine"`
	History  [][]string       `json:"history"`
	Chips    []int64          `json:"chips"`
	MaxTotal int64            `json:"maxTotal"`
	Faces    []Face           `json:"faces"`
	Balance  *int64           `json:"balance,omitempty"`
	Room     RoomInfo         `json:"room"`
	Invite   string           `json:"invite"`
	Chat     []chat.Line      `json:"chat"` // 20 tin chat gần nhất của bàn; seq tăng dần để client chỉ hiện tin mới
}

const maxPlayers = 12

// begin khoá, tìm bàn, dọn bàn cũ và đồng bộ vòng; trả bàn và đồng hồ. Người gọi phải g.mu.Unlock().
func (g *Game) begin(ctx context.Context, code string) (*room, clock, error) {
	g.mu.Lock()
	now := g.now()
	c := g.clockAt(now)
	if err := g.sweep(ctx, now, c); err != nil {
		return nil, c, err
	}
	r := g.rooms[normCode(code)]
	if r == nil {
		return nil, c, notFound("không có bàn này")
	}
	if err := g.sync(ctx, r, c); err != nil {
		return nil, c, err
	}
	return r, c, nil
}

// State trả trạng thái một bàn; uid = 0 là người xem chưa đăng nhập. Có uid thì tính là đang có mặt.
func (g *Game) State(ctx context.Context, code string, uid int64) (State, error) {
	r, c, err := g.begin(ctx, code)
	defer g.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	if uid != 0 {
		r.seen(uid, g.now())
	}
	return g.state(ctx, r, c, uid)
}

func scanWho(scan func(...any) error, extra ...any) (Who, error) {
	var w Who
	var xp int64
	err := scan(append([]any{&w.Name, &w.Gender, &w.Outfit, &w.Hair, &w.Wings, &w.Face, &xp}, extra...)...)
	w.Level = wallet.LevelFor(xp)
	return w, err
}

const whoColumns = `u.name, u.gender, u.outfit, u.hair, u.wings, u.face, u.xp`

func (g *Game) who(ctx context.Context, tx *wallet.Tx, uid int64) (Who, error) {
	row := tx.QueryRowContext(ctx, `SELECT `+whoColumns+` FROM users u WHERE u.id = ?`, uid)
	return scanWho(row.Scan)
}

// roundTotal là tổng cược cả bàn vòng hiện tại.
func roundTotal(ctx context.Context, tx *wallet.Tx, code string, round int64) (int64, error) {
	var n int64
	err := tx.QueryRowContext(ctx, `SELECT coalesce(SUM(amount), 0) FROM baucua_bets WHERE room = ? AND round = ?`, code, round).Scan(&n)
	return n, err
}

// state dựng trạng thái. Gọi khi đang giữ g.mu và đã sync.
func (g *Game) state(ctx context.Context, r *room, c clock, uid int64) (State, error) {
	now := g.now()
	s := State{
		Round: c.round, Phase: c.phase, EndsAt: c.endsAt.UnixMilli(), Now: now.UnixMilli(),
		Totals: map[string]int64{}, Players: []Player{}, Mine: Mine{Bets: map[string]int64{}}, History: [][]string{},
		Chips: Chips, MaxTotal: MaxTotal, Faces: Faces,
		Room:   RoomInfo{Code: r.code, Members: []Member{}},
		Invite: "/baucua/?room=" + r.code,
		Chat:   r.chat.List(),
	}
	var dice [3]string
	if c.phase == PhaseResult {
		dice = r.dice[c.round]
		s.Dice = dice[:]
	}
	for _, n := range r.history {
		if n < c.round || (n == c.round && c.phase == PhaseResult) {
			d := r.dice[n]
			s.History = append(s.History, d[:])
		}
	}

	tx, err := g.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	err = queryRows(ctx, tx, `SELECT face, SUM(amount) FROM baucua_bets WHERE room = ? AND round = ? GROUP BY face`, []any{r.code, c.round},
		func(scan func(...any) error) error {
			var face string
			var n int64
			if err := scan(&face, &n); err != nil {
				return err
			}
			s.Totals[face] = n
			s.Room.RoundTotal += n
			return nil
		})
	if err != nil {
		return State{}, err
	}
	totals := map[int64]int64{} // tổng cược vòng này theo người
	err = queryRows(ctx, tx, `SELECT b.user_id, `+whoColumns+`, SUM(b.amount) AS total
		FROM baucua_bets b JOIN users u ON u.id = b.user_id WHERE b.room = ? AND b.round = ?
		GROUP BY b.user_id ORDER BY total DESC, b.user_id`, []any{r.code, c.round},
		func(scan func(...any) error) error {
			var id, total int64
			var w Who
			var xp int64
			if err := scan(&id, &w.Name, &w.Gender, &w.Outfit, &w.Hair, &w.Wings, &w.Face, &xp, &total); err != nil {
				return err
			}
			totals[id] = total
			if len(s.Players) < maxPlayers {
				s.Players = append(s.Players, Player{Name: w.Name, Gender: w.Gender, Outfit: w.Outfit, Level: wallet.LevelFor(xp), Total: total})
			}
			return nil
		})
	if err != nil {
		return State{}, err
	}

	// Thông tin bàn: chủ bàn, người làm cái, người có mặt.
	if r.host != 0 {
		h, err := g.who(ctx, tx, r.host)
		if err != nil {
			return State{}, err
		}
		s.Room.Host = &h
	}
	if b := r.banker; b != nil {
		w, err := g.who(ctx, tx, b.uid)
		if err != nil {
			return State{}, err
		}
		s.Room.Banker = &Banker{Who: w, Bankroll: b.bankroll, Start: b.start, Leaving: b.leaving}
		if net, ok := r.bankNet[c.round]; ok && c.phase == PhaseResult {
			s.Room.Banker.Net = &net
		}
		s.Room.MaxRoundTotal = b.bankroll / 3
	}
	for _, id := range r.present(now) {
		w, err := g.who(ctx, tx, id)
		if err != nil {
			return State{}, err
		}
		s.Room.Members = append(s.Room.Members, Member{Who: w, Total: totals[id]})
	}
	slices.SortStableFunc(s.Room.Members, func(a, b Member) int { return int(b.Total - a.Total) })

	if uid == 0 {
		return s, nil
	}
	s.Mine.Host = r.host == uid
	s.Mine.Banker = r.banker != nil && r.banker.uid == uid
	err = queryRows(ctx, tx, `SELECT face, SUM(amount) FROM baucua_bets WHERE room = ? AND round = ? AND user_id = ? GROUP BY face`,
		[]any{r.code, c.round, uid}, func(scan func(...any) error) error {
			var face string
			var n int64
			if err := scan(&face, &n); err != nil {
				return err
			}
			s.Mine.Bets[face] = n
			s.Mine.Total += n
			return nil
		})
	if err != nil {
		return State{}, err
	}
	if c.phase == PhaseResult && s.Mine.Total > 0 {
		var won int64
		for face, n := range s.Mine.Bets {
			won += payout(face, n, dice)
		}
		net := won - s.Mine.Total
		s.Mine.Won, s.Mine.Net = &won, &net
	}
	var bal int64
	if err := tx.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, uid).Scan(&bal); err != nil {
		return State{}, err
	}
	s.Balance = &bal
	return s, nil
}

// ---------- Đặt cược ----------

// Bet đặt amount xu vào mặt face cho vòng hiện tại của bàn, trừ xu ngay.
func (g *Game) Bet(ctx context.Context, code string, uid int64, face string, amount int64) (State, error) {
	name, ok := faceName(face)
	if !ok {
		return State{}, bad("không có mặt này")
	}
	if !slices.Contains(Chips, amount) {
		return State{}, bad("mức chip không hợp lệ")
	}
	r, c, err := g.begin(ctx, code)
	defer g.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	r.seen(uid, g.now())
	if c.phase != PhaseBetting {
		return State{}, conflict("đã hết giờ đặt cược, chờ vòng sau")
	}
	if r.banker != nil && r.banker.uid == uid {
		return State{}, conflict("bạn đang làm cái, không đặt cược được")
	}
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	var total int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(SUM(amount), 0) FROM baucua_bets WHERE room = ? AND round = ? AND user_id = ?`,
		r.code, c.round, uid).Scan(&total); err != nil {
		return State{}, err
	}
	if total+amount > MaxTotal {
		return State{}, conflict(fmt.Sprintf("mỗi vòng cược tối đa %d xu (đã đặt %d)", MaxTotal, total))
	}
	// Quỹ cái phải đủ trả trường hợp xấu nhất (mọi cược ra 3 lần): tổng cược cả bàn ≤ quỹ / 3.
	if b := r.banker; b != nil {
		all, err := roundTotal(ctx, tx, r.code, c.round)
		if err != nil {
			return State{}, err
		}
		if room := b.bankroll/3 - all; amount > room {
			return State{}, conflict(fmt.Sprintf("Quỹ cái chỉ nhận thêm %d xu", max(0, room)))
		}
	}
	if err := tx.Charge(uid, amount, GameID, fmt.Sprintf("Bầu cua vòng %d: đặt %d %s", c.round, amount, name)); err != nil {
		return State{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO baucua_bets (room, round, user_id, face, amount) VALUES (?, ?, ?, ?, ?)`,
		r.code, c.round, uid, face, amount); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	return g.state(ctx, r, c, uid)
}

// Clear huỷ mọi cược của người chơi trong vòng hiện tại của bàn (chỉ khi còn giờ cược), hoàn xu.
func (g *Game) Clear(ctx context.Context, code string, uid int64) (State, error) {
	r, c, err := g.begin(ctx, code)
	defer g.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	r.seen(uid, g.now())
	if c.phase != PhaseBetting {
		return State{}, conflict("đã hết giờ đặt cược, không huỷ được")
	}
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	var total int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(SUM(amount), 0) FROM baucua_bets WHERE room = ? AND round = ? AND user_id = ? AND settled = 0`,
		r.code, c.round, uid).Scan(&total); err != nil {
		return State{}, err
	}
	if total == 0 {
		return State{}, conflict("bạn chưa đặt cược vòng này")
	}
	if err := tx.Credit(uid, total, GameID, fmt.Sprintf("Bầu cua vòng %d: huỷ cược", c.round)); err != nil {
		return State{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM baucua_bets WHERE room = ? AND round = ? AND user_id = ? AND settled = 0`,
		r.code, c.round, uid); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	return g.state(ctx, r, c, uid)
}
