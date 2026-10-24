// Package baucua là game Bầu Cua Tôm Cá của DCity: một sòng chung cho mọi người, máy làm nhà cái.
// Vòng chạy liên tục theo đồng hồ: đặt cược 20 giây → lắc 4 giây → mở bát 6 giây. Mọi khoản xu đi qua ví chung
// (sổ cái game "baucua"); cược trừ ngay lúc đặt, thắng được trả lúc lắc xong.
//
// Vòng tính "lười" theo thời gian: số vòng và pha suy ra từ đồng hồ, kết quả của một vòng được quay và trả thưởng
// ở request đầu tiên sau khi hết giờ cược (dưới khoá, trong một transaction), nên chỉ quay và trả đúng một lần.
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

// XP mỗi vòng: có đặt cược được XPPlay, có lời thì thêm XPWin.
const (
	XPPlay = 2
	XPWin  = 3
)

// Bảng cược nằm chung database với ví để trừ / cộng xu và ghi cược trong cùng transaction.
// settled = 1 khi vòng đã chốt; cược được hoàn (huỷ, khởi động lại) thì xoá.
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
`

// Error là lỗi hiện cho người chơi, kèm mã HTTP.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func bad(msg string) error      { return &Error{http.StatusBadRequest, msg} }
func conflict(msg string) error { return &Error{http.StatusConflict, msg} }

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

// Game là sòng bầu cua chung.
type Game struct {
	w    *wallet.Wallet
	now  func() time.Time
	roll func() [3]string // quay 3 xúc xắc (test thay được)

	mu      sync.Mutex
	dice    map[int64][3]string // kết quả các vòng đã quay
	history []int64             // các vòng đã quay, mới nhất trước (giữ tối đa historyKeep)
}

const historyKeep = 10

func randomRoll() [3]string {
	var d [3]string
	for i := range d {
		d[i] = Faces[rand.IntN(len(Faces))].ID
	}
	return d
}

// New tạo sòng và hoàn các cược còn treo (server tắt giữa vòng).
func New(ctx context.Context, w *wallet.Wallet) (*Game, error) {
	if err := w.ExecSchema(ctx, schema); err != nil {
		return nil, fmt.Errorf("tạo bảng bầu cua: %w", err)
	}
	g := &Game{w: w, now: time.Now, roll: randomRoll, dice: map[int64][3]string{}}
	if err := g.refundOpen(ctx); err != nil {
		return nil, fmt.Errorf("hoàn cược treo: %w", err)
	}
	return g, nil
}

// refundOpen hoàn mọi cược chưa chốt, mỗi người mỗi vòng một dòng sổ cái.
func (g *Game) refundOpen(ctx context.Context) error {
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT round, user_id, SUM(amount) FROM baucua_bets WHERE settled = 0 GROUP BY round, user_id`)
	if err != nil {
		return err
	}
	type refund struct{ round, uid, amount int64 }
	var list []refund
	for rows.Next() {
		var r refund
		if err := rows.Scan(&r.round, &r.uid, &r.amount); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range list {
		if err := tx.Credit(r.uid, r.amount, GameID, fmt.Sprintf("Bầu cua vòng %d: hoàn cược (máy chủ khởi động lại)", r.round)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM baucua_bets WHERE settled = 0`); err != nil {
		return err
	}
	return tx.Commit()
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

// sync quay và chốt mọi vòng đã hết giờ cược mà chưa chốt. Gọi khi đang giữ g.mu.
func (g *Game) sync(ctx context.Context, c clock) error {
	// Vòng hiện tại hết giờ cược thì quay luôn (kể cả không ai đặt) để có lịch sử.
	if c.phase != PhaseBetting {
		g.rollRound(c.round)
	}
	last := c.round - 1
	if c.phase != PhaseBetting {
		last = c.round
	}
	rows, err := g.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer rows.Rollback()
	q, err := rows.QueryContext(ctx, `SELECT DISTINCT round FROM baucua_bets WHERE settled = 0 AND round <= ? ORDER BY round`, last)
	if err != nil {
		return err
	}
	var rounds []int64
	for q.Next() {
		var r int64
		if err := q.Scan(&r); err != nil {
			q.Close()
			return err
		}
		rounds = append(rounds, r)
	}
	q.Close()
	if err := q.Err(); err != nil {
		return err
	}
	rows.Rollback()
	for _, r := range rounds {
		if err := g.settle(ctx, r, g.rollRound(r)); err != nil {
			return err
		}
	}
	return nil
}

// rollRound trả kết quả của vòng, quay nếu chưa quay. Gọi khi đang giữ g.mu.
func (g *Game) rollRound(round int64) [3]string {
	if d, ok := g.dice[round]; ok {
		return d
	}
	d := g.roll()
	g.dice[round] = d
	g.history = append([]int64{round}, g.history...)
	slices.SortFunc(g.history, func(a, b int64) int { return int(b - a) })
	for len(g.history) > historyKeep {
		old := g.history[len(g.history)-1]
		g.history = g.history[:len(g.history)-1]
		delete(g.dice, old)
	}
	return d
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

// settle trả thưởng một vòng trong một transaction: mỗi người thắng một dòng sổ cái, cộng XP.
func (g *Game) settle(ctx context.Context, round int64, dice [3]string) error {
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT user_id, face, amount FROM baucua_bets WHERE round = ? AND settled = 0`, round)
	if err != nil {
		return err
	}
	type sum struct{ bet, won int64 }
	users := map[int64]*sum{}
	var order []int64
	for rows.Next() {
		var uid, amount int64
		var face string
		if err := rows.Scan(&uid, &face, &amount); err != nil {
			rows.Close()
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
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, uid := range order {
		s := users[uid]
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
	if _, err := tx.ExecContext(ctx, `UPDATE baucua_bets SET settled = 1 WHERE round = ? AND settled = 0`, round); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------- Trạng thái ----------

type Mine struct {
	Bets  map[string]int64 `json:"bets"`
	Total int64            `json:"total"`
	Won   *int64           `json:"won,omitempty"` // chỉ có khi mở bát: số xu nhận về
	Net   *int64           `json:"net,omitempty"`
}

type Player struct {
	Name   string `json:"name"`
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
	Level  int    `json:"level"`
	Total  int64  `json:"total"`
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
}

const maxPlayers = 12

// State trả trạng thái sòng; uid = 0 là người xem chưa đăng nhập.
func (g *Game) State(ctx context.Context, uid int64) (State, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.clockAt(g.now())
	if err := g.sync(ctx, c); err != nil {
		return State{}, err
	}
	return g.state(ctx, c, uid)
}

// state dựng trạng thái. Gọi khi đang giữ g.mu và đã sync.
func (g *Game) state(ctx context.Context, c clock, uid int64) (State, error) {
	s := State{
		Round: c.round, Phase: c.phase, EndsAt: c.endsAt.UnixMilli(), Now: g.now().UnixMilli(),
		Totals: map[string]int64{}, Players: []Player{}, Mine: Mine{Bets: map[string]int64{}}, History: [][]string{},
		Chips: Chips, MaxTotal: MaxTotal, Faces: Faces,
	}
	var dice [3]string
	if c.phase == PhaseResult {
		dice = g.dice[c.round]
		s.Dice = dice[:]
	}
	for _, r := range g.history {
		if r < c.round || (r == c.round && c.phase == PhaseResult) {
			d := g.dice[r]
			s.History = append(s.History, d[:])
		}
	}

	rows, err := g.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer rows.Rollback()
	q, err := rows.QueryContext(ctx, `SELECT face, SUM(amount) FROM baucua_bets WHERE round = ? GROUP BY face`, c.round)
	if err != nil {
		return State{}, err
	}
	for q.Next() {
		var face string
		var n int64
		if err := q.Scan(&face, &n); err != nil {
			q.Close()
			return State{}, err
		}
		s.Totals[face] = n
	}
	q.Close()
	q, err = rows.QueryContext(ctx, `SELECT u.name, u.gender, u.outfit, u.xp, SUM(b.amount) AS total
		FROM baucua_bets b JOIN users u ON u.id = b.user_id WHERE b.round = ?
		GROUP BY b.user_id ORDER BY total DESC, b.user_id LIMIT ?`, c.round, maxPlayers)
	if err != nil {
		return State{}, err
	}
	for q.Next() {
		var p Player
		var xp int64
		if err := q.Scan(&p.Name, &p.Gender, &p.Outfit, &xp, &p.Total); err != nil {
			q.Close()
			return State{}, err
		}
		p.Level = wallet.LevelFor(xp)
		s.Players = append(s.Players, p)
	}
	q.Close()
	if uid == 0 {
		return s, nil
	}
	q, err = rows.QueryContext(ctx, `SELECT face, SUM(amount) FROM baucua_bets WHERE round = ? AND user_id = ? GROUP BY face`, c.round, uid)
	if err != nil {
		return State{}, err
	}
	for q.Next() {
		var face string
		var n int64
		if err := q.Scan(&face, &n); err != nil {
			q.Close()
			return State{}, err
		}
		s.Mine.Bets[face] = n
		s.Mine.Total += n
	}
	q.Close()
	if c.phase == PhaseResult && s.Mine.Total > 0 {
		var won int64
		for face, n := range s.Mine.Bets {
			won += payout(face, n, dice)
		}
		net := won - s.Mine.Total
		s.Mine.Won, s.Mine.Net = &won, &net
	}
	var bal int64
	if err := rows.QueryRowContext(ctx, `SELECT balance FROM users WHERE id = ?`, uid).Scan(&bal); err != nil {
		return State{}, err
	}
	s.Balance = &bal
	return s, nil
}

// ---------- Đặt cược ----------

// Bet đặt amount xu vào mặt face cho vòng hiện tại, trừ xu ngay.
func (g *Game) Bet(ctx context.Context, uid int64, face string, amount int64) (State, error) {
	name, ok := faceName(face)
	if !ok {
		return State{}, bad("không có mặt này")
	}
	if !slices.Contains(Chips, amount) {
		return State{}, bad("mức chip không hợp lệ")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.clockAt(g.now())
	if err := g.sync(ctx, c); err != nil {
		return State{}, err
	}
	if c.phase != PhaseBetting {
		return State{}, conflict("đã hết giờ đặt cược, chờ vòng sau")
	}
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	var total int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(SUM(amount), 0) FROM baucua_bets WHERE round = ? AND user_id = ?`, c.round, uid).Scan(&total); err != nil {
		return State{}, err
	}
	if total+amount > MaxTotal {
		return State{}, conflict(fmt.Sprintf("mỗi vòng cược tối đa %d xu (đã đặt %d)", MaxTotal, total))
	}
	if err := tx.Charge(uid, amount, GameID, fmt.Sprintf("Bầu cua vòng %d: đặt %d %s", c.round, amount, name)); err != nil {
		return State{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO baucua_bets (round, user_id, face, amount) VALUES (?, ?, ?, ?)`, c.round, uid, face, amount); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	return g.state(ctx, c, uid)
}

// Clear huỷ mọi cược của người chơi trong vòng hiện tại (chỉ khi còn giờ cược), hoàn xu.
func (g *Game) Clear(ctx context.Context, uid int64) (State, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.clockAt(g.now())
	if err := g.sync(ctx, c); err != nil {
		return State{}, err
	}
	if c.phase != PhaseBetting {
		return State{}, conflict("đã hết giờ đặt cược, không huỷ được")
	}
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	var total int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(SUM(amount), 0) FROM baucua_bets WHERE round = ? AND user_id = ? AND settled = 0`, c.round, uid).Scan(&total); err != nil {
		return State{}, err
	}
	if total == 0 {
		return State{}, conflict("bạn chưa đặt cược vòng này")
	}
	if err := tx.Credit(uid, total, GameID, fmt.Sprintf("Bầu cua vòng %d: huỷ cược", c.round)); err != nil {
		return State{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM baucua_bets WHERE round = ? AND user_id = ? AND settled = 0`, c.round, uid); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	return g.state(ctx, c, uid)
}
