package baucua

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/dbphx/game-center/internal/chat"
)

// Lobby là mã sòng chung: luôn mở, máy luôn làm cái.
const Lobby = "chung"

const (
	MinBankroll  = 3000             // quỹ tối thiểu để làm cái
	QuitBankroll = 300              // quỹ dưới mức này thì tự thôi làm cái
	memberTTL    = 15 * time.Second // không poll quá lâu thì coi là đã rời bàn
	bankerAway   = 60 * time.Second // người làm cái vắng quá lâu thì tự thôi
	roomIdle     = 5 * time.Minute  // bàn riêng không ai có mặt quá lâu thì đóng
	sweepEvery   = 5 * time.Second
	maxRooms     = 200
	codeLetters  = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

func randomCode() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = codeLetters[rand.IntN(len(codeLetters))]
	}
	return string(b)
}

func normCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || strings.EqualFold(code, Lobby) {
		return Lobby
	}
	return strings.ToUpper(code)
}

// banker là người đang làm cái một bàn; bankroll đồng bộ với bảng baucua_banks.
type banker struct {
	uid      int64
	bankroll int64
	start    int64
	leaving  bool // đã xin thôi, thôi sau khi chốt vòng có cược
}

// room là một bàn bầu cua (sòng chung hoặc bàn riêng), sống trong bộ nhớ.
type room struct {
	code       string
	host       int64 // 0 với sòng chung
	seenAt     map[int64]time.Time
	lastActive time.Time
	banker     *banker
	dice       map[int64][3]string // kết quả các vòng đã quay
	history    []int64             // các vòng đã quay, mới nhất trước (giữ tối đa historyKeep)
	bankNet    map[int64]int64     // lãi / lỗ của quỹ cái theo vòng
	chat       chat.Log            // 20 tin chat gần nhất
	chatLim    map[int64]*chat.Limiter
}

func newRoom(code string, host int64, now time.Time) *room {
	return &room{code: code, host: host, seenAt: map[int64]time.Time{}, lastActive: now,
		dice: map[int64][3]string{}, bankNet: map[int64]int64{}, chatLim: map[int64]*chat.Limiter{}}
}

func (r *room) seen(uid int64, now time.Time) {
	r.seenAt[uid] = now
	r.lastActive = now
}

// absentFor là thời gian người chơi đã vắng mặt (0 nếu đang có mặt).
func (r *room) absentFor(uid int64, now time.Time) time.Duration {
	at, ok := r.seenAt[uid]
	if !ok {
		return time.Duration(1 << 62)
	}
	return max(0, now.Sub(at.Add(memberTTL)))
}

// present trả người đang có mặt, theo thứ tự id.
func (r *room) present(now time.Time) []int64 {
	var ids []int64
	for id, at := range r.seenAt {
		if now.Sub(at) < memberTTL {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// rollRound trả kết quả của vòng, quay nếu chưa quay.
func (r *room) rollRound(round int64, roll func() [3]string) [3]string {
	if d, ok := r.dice[round]; ok {
		return d
	}
	d := roll()
	r.dice[round] = d
	r.history = append(r.history, round)
	slices.SortFunc(r.history, func(a, b int64) int { return int(b - a) })
	for len(r.history) > historyKeep {
		old := r.history[len(r.history)-1]
		r.history = r.history[:len(r.history)-1]
		delete(r.dice, old)
		delete(r.bankNet, old)
	}
	return d
}

// openBets là số cược chưa chốt của bàn (mọi vòng).
func (g *Game) openBets(ctx context.Context, code string) (int, error) {
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var n int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM baucua_bets WHERE room = ? AND settled = 0`, code).Scan(&n)
	return n, err
}

// checkBanker cho người làm cái thôi khi đã xin thôi, quỹ dưới QuitBankroll hoặc vắng quá bankerAway — chỉ khi
// bàn không còn cược chưa chốt. Gọi khi đang giữ g.mu.
func (g *Game) checkBanker(ctx context.Context, r *room, c clock) error {
	b := r.banker
	if b == nil {
		return nil
	}
	if !b.leaving && b.bankroll >= QuitBankroll && r.absentFor(b.uid, g.now()) <= bankerAway {
		return nil
	}
	n, err := g.openBets(ctx, r.code)
	if err != nil || n > 0 {
		return err
	}
	return g.release(ctx, r)
}

// release hoàn quỹ còn lại về ví người làm cái và trả bàn cho máy làm cái.
func (g *Game) release(ctx context.Context, r *room) error {
	b := r.banker
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	note := "Bầu cua: rút quỹ làm cái"
	switch net := b.bankroll - b.start; {
	case net > 0:
		note += fmt.Sprintf(" (lãi %d)", net)
	case net < 0:
		note += fmt.Sprintf(" (lỗ %d)", -net)
	}
	if b.bankroll > 0 {
		if err := tx.Credit(b.uid, b.bankroll, GameID, note); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM baucua_banks WHERE room = ?`, r.code); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.banker = nil
	return nil
}

// sweep đồng bộ mọi bàn (để người làm cái vắng / hết quỹ được thôi) và đóng bàn riêng bỏ trống lâu.
// Chạy tối đa mỗi sweepEvery. Gọi khi đang giữ g.mu.
func (g *Game) sweep(ctx context.Context, now time.Time, c clock) error {
	if now.Sub(g.lastSweep) < sweepEvery {
		return nil
	}
	g.lastSweep = now
	for code, r := range g.rooms {
		if err := g.sync(ctx, r, c); err != nil {
			return err
		}
		if code == Lobby || len(r.present(now)) > 0 || now.Sub(r.lastActive) < roomIdle {
			continue
		}
		if n, err := g.openBets(ctx, code); err != nil || n > 0 {
			if err != nil {
				return err
			}
			continue
		}
		if r.banker != nil {
			if err := g.release(ctx, r); err != nil {
				return err
			}
		}
		delete(g.rooms, code)
	}
	return nil
}

// ---------- Bàn riêng ----------

// Create mở bàn riêng, người tạo là chủ bàn; trả mã bàn.
func (g *Game) Create(ctx context.Context, uid int64) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	if err := g.sweep(ctx, now, g.clockAt(now)); err != nil {
		return "", err
	}
	if len(g.rooms) > maxRooms {
		return "", conflict("đang có quá nhiều bàn, thử lại sau")
	}
	code := g.code()
	for i := 0; g.rooms[code] != nil || code == strings.ToUpper(Lobby); i++ {
		if i > 50 {
			return "", fmt.Errorf("không sinh được mã bàn")
		}
		code = g.code()
	}
	r := newRoom(code, uid, now)
	r.seen(uid, now)
	g.rooms[code] = r
	return code, nil
}

// Join ghi nhận người chơi có mặt ở bàn và trả trạng thái.
func (g *Game) Join(ctx context.Context, code string, uid int64) (State, error) {
	return g.State(ctx, code, uid)
}

// Chat gửi một tin vào bàn. Chỉ người đang có mặt mới chat được; chat dày bị chặn (409).
func (g *Game) Chat(ctx context.Context, code string, uid int64, text string) (State, error) {
	r, c, err := g.begin(ctx, code)
	defer g.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	now := g.now()
	if r.absentFor(uid, now) > 0 {
		return State{}, conflict("bạn chưa vào bàn này")
	}
	r.seen(uid, now)
	if text = chat.Prepare(text); text != "" {
		lim := r.chatLim[uid]
		if lim == nil {
			lim = &chat.Limiter{}
			r.chatLim[uid] = lim
		}
		if !lim.Allow(now) {
			return State{}, conflict(chat.ErrTooFast.Error())
		}
		u, err := g.w.User(ctx, uid)
		if err != nil {
			return State{}, err
		}
		r.chat.Add(chat.Line{Name: u.Name, Text: text, At: now.UnixMilli()})
	}
	return g.state(ctx, r, c, uid)
}

// Leave cho người chơi rời bàn ngay (không chờ hết hạn poll).
func (g *Game) Leave(ctx context.Context, code string, uid int64) error {
	r, _, err := g.begin(ctx, code)
	defer g.mu.Unlock()
	if err != nil {
		return err
	}
	if _, ok := r.seenAt[uid]; ok {
		r.seenAt[uid] = g.now().Add(-memberTTL)
	}
	return nil
}

// RoomSummary là một dòng trong danh sách bàn.
type RoomSummary struct {
	Code    string  `json:"code"`
	Host    *Who    `json:"host"`
	Banker  *Banker `json:"banker"`
	Members int     `json:"members"`
	Phase   string  `json:"phase"`
}

// Rooms liệt kê các bàn: sòng chung trước, rồi bàn đông người trước.
func (g *Game) Rooms(ctx context.Context) ([]RoomSummary, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	c := g.clockAt(now)
	if err := g.sweep(ctx, now, c); err != nil {
		return nil, err
	}
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	out := []RoomSummary{}
	for code, r := range g.rooms {
		s := RoomSummary{Code: code, Members: len(r.present(now)), Phase: c.phase}
		if r.host != 0 {
			h, err := g.who(ctx, tx, r.host)
			if err != nil {
				return nil, err
			}
			s.Host = &h
		}
		if b := r.banker; b != nil {
			w, err := g.who(ctx, tx, b.uid)
			if err != nil {
				return nil, err
			}
			s.Banker = &Banker{Who: w, Bankroll: b.bankroll, Start: b.start, Leaving: b.leaving}
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b RoomSummary) int {
		switch {
		case a.Code == Lobby:
			return -1
		case b.Code == Lobby:
			return 1
		case a.Members != b.Members:
			return b.Members - a.Members
		}
		return strings.Compare(a.Code, b.Code)
	})
	return out, nil
}

// ---------- Làm cái ----------

// Bank nộp quỹ bankroll để làm cái bàn riêng. Chỉ khi bàn chưa có ai làm cái và (đang cược mà chưa ai đặt, hoặc
// đang mở bát — áp dụng từ vòng sau).
func (g *Game) Bank(ctx context.Context, code string, uid, bankroll int64) (State, error) {
	r, c, err := g.begin(ctx, code)
	defer g.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	r.seen(uid, g.now())
	switch {
	case r.code == Lobby:
		return State{}, conflict("sòng chung luôn do máy làm cái, hãy mở bàn riêng")
	case r.banker != nil && r.banker.uid == uid:
		return State{}, conflict("bạn đang làm cái rồi")
	case r.banker != nil:
		return State{}, conflict("bàn đã có người làm cái")
	case bankroll < MinBankroll:
		return State{}, bad(fmt.Sprintf("quỹ làm cái tối thiểu %d xu", MinBankroll))
	}
	n, err := g.openBets(ctx, r.code)
	if err != nil {
		return State{}, err
	}
	if c.phase == PhaseRolling || n > 0 {
		return State{}, conflict("chỉ nhận làm cái khi bàn chưa có cược hoặc lúc mở bát")
	}
	tx, err := g.w.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback()
	if err := tx.Charge(uid, bankroll, GameID, "Bầu cua: đặt quỹ làm cái"); err != nil {
		return State{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO baucua_banks (room, user_id, bankroll, start) VALUES (?, ?, ?, ?)`,
		r.code, uid, bankroll, bankroll); err != nil {
		return State{}, err
	}
	if err := tx.Commit(); err != nil {
		return State{}, err
	}
	r.banker = &banker{uid: uid, bankroll: bankroll, start: bankroll}
	return g.state(ctx, r, c, uid)
}

// Unbank thôi làm cái: bàn còn cược chưa chốt thì thôi sau khi chốt, không thì hoàn quỹ ngay.
func (g *Game) Unbank(ctx context.Context, code string, uid int64) (State, error) {
	r, c, err := g.begin(ctx, code)
	defer g.mu.Unlock()
	if err != nil {
		return State{}, err
	}
	r.seen(uid, g.now())
	if r.banker == nil || r.banker.uid != uid {
		return State{}, conflict("bạn không làm cái bàn này")
	}
	r.banker.leaving = true
	if err := g.checkBanker(ctx, r, c); err != nil {
		return State{}, err
	}
	return g.state(ctx, r, c, uid)
}
