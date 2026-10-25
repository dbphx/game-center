package caro

// Bàn Cờ Caro online: 2 ghế (người hoặc máy), người xem, lượt 30 giây, cược và XP vào ví chung.
//
// Mọi thao tác trên một Room chạy dưới Room.mu. Thứ tự khoá là Room.mu → Hub.mu,
// Hub không bao giờ giữ Hub.mu khi gọi vào Room.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	mrand "math/rand/v2"
	"slices"
	"sync"
	"time"
)

const (
	GameID     = "caro"
	MaxPlayers = 2
	DefaultBet = 10

	PhaseLobby   = "lobby"
	PhasePlaying = "playing"
	PhaseOver    = "over"

	// XP mỗi ván cho người thật (kể cả bàn 0 xu).
	XPWin  = 20
	XPDraw = 12
	XPLose = 8
)

// BetOptions là các mức cược chủ bàn được chọn; 0 là chơi vui.
var BetOptions = []int{0, 10, 20, 50, 100, 200, 500, 1000}

// User là tài khoản hub, kèm nhân vật để vẽ avatar.
type User struct {
	ID            int64
	Name          string
	Gender        string
	Outfit        string
	Hair          string
	Wings         string
	Face          string
	Level         int
	HP            int
	HospitalUntil int64
}

// Bank là ví xu và kinh nghiệm dùng chung của hub.
type Bank interface {
	Balance(ctx context.Context, userID int64) (int64, error)
	Settle(ctx context.Context, game, note string, deltas map[int64]int64) (map[int64]int64, error)
	AwardXP(ctx context.Context, xp map[int64]int64) (map[int64]int, error)
}

// Conn là kết nối tới một người. Send không được chặn vì được gọi khi đang giữ khoá bàn.
type Conn interface {
	Send(msg any)
}

type Options struct {
	BotDelay    time.Duration // thời gian máy "suy nghĩ"
	TurnTimeout time.Duration // hết giờ thì người đang tới lượt thua
	Grace       time.Duration // chờ người mất kết nối quay lại
	MinHP       int           // máu tối thiểu để ngồi ghế (0 = không kiểm tra)
}

var DefaultOptions = Options{BotDelay: 700 * time.Millisecond, TurnTimeout: 30 * time.Second, Grace: 30 * time.Second}

func (o Options) gate(u User) error {
	if o.MinHP <= 0 {
		return nil
	}
	if u.HospitalUntil > 0 {
		return errors.New("bạn đang nằm viện")
	}
	if u.HP < o.MinHP {
		return fmt.Errorf("cần ít nhất %d HP để vào Casino", o.MinHP)
	}
	return nil
}

// Player là người ngồi ghế hoặc người xem.
type Player struct {
	ID       string
	UserID   int64 // 0 với máy
	Name     string
	Gender   string
	Outfit   string
	Hair     string
	Wings    string
	Face     string
	Level    int
	Bot      bool
	BotLevel string
	Online   bool
	Coins    int64
	Mark     byte // X / O trong ván, 0 khi chưa chia
	ready    bool // đã bấm chơi lại

	conn      Conn
	graceTime *time.Timer
}

// Result là kết quả ván vừa xong.
type Result struct {
	Winner string           `json:"winner"` // "x", "o" hoặc "" khi hoà
	Reason string           `json:"reason"` // five | timeout | resign | left | draw
	Money  map[string]int64 `json:"money"`
	XP     map[string]int64 `json:"xp"`
}

type Room struct {
	Code string

	mu         sync.Mutex
	hub        *Hub
	opts       Options
	hostID     string
	players    []*Player // tối đa 2 ghế
	spectators []*Player
	phase      string
	rule       string
	bet        int
	gameBet    int
	board      Board
	turn       byte
	moves      int
	last       *[2]int
	winLine    [][2]int
	result     *Result
	loserMark  byte // ai thua ván trước (để ván sau cầm X); 0 khi chưa có / hoà
	timer      *time.Timer
	timerGen   int
	deadline   time.Time
	botSeq     int
	closed     bool
	rng        *mrand.Rand
}

func randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Room) seatOf(p *Player) int { return slices.Index(r.players, p) }

func (r *Room) findPlayer(id string) *Player {
	for _, p := range r.players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (r *Room) host() *Player { return r.findPlayer(r.hostID) }

func (r *Room) humans() []*Player {
	var out []*Player
	for _, p := range r.players {
		if !p.Bot {
			out = append(out, p)
		}
	}
	return out
}

func (r *Room) byMark(m byte) *Player {
	for _, p := range r.players {
		if p.Mark == m {
			return p
		}
	}
	return nil
}

func (r *Room) refreshCoins(p *Player) error {
	bal, err := r.hub.bank.Balance(context.Background(), p.UserID)
	if err == nil {
		p.Coins = bal
	}
	return err
}

func newPlayer(u User, conn Conn) *Player {
	return &Player{ID: randomID(), UserID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair,
		Wings: u.Wings, Face: u.Face, Level: u.Level, Online: true, conn: conn}
}

func (p *Player) update(u User) {
	p.Name, p.Gender, p.Outfit, p.Hair, p.Wings, p.Face, p.Level = u.Name, u.Gender, u.Outfit, u.Hair, u.Wings, u.Face, u.Level
}

// join cho người vào bàn: trả lại ghế / chỗ xem cũ; còn ghế trống (không đang chơi) thì ngồi, không thì xem.
func (r *Room) join(u User, conn Conn) (*Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("bàn đã đóng")
	}
	for _, p := range append(slices.Clone(r.players), r.spectators...) {
		if p.UserID == u.ID && !p.Bot {
			p.update(u)
			r.resume(p, conn)
			return p, nil
		}
	}
	if r.phase != PhasePlaying && len(r.players) < MaxPlayers {
		if err := r.opts.gate(u); err != nil {
			return nil, err
		}
		p := newPlayer(u, conn)
		if err := r.refreshCoins(p); err != nil {
			return nil, err
		}
		r.players = append(r.players, p)
		if r.host() == nil {
			r.hostID = p.ID
		}
		r.seatAdded()
		return p, nil
	}
	p := newPlayer(u, conn)
	r.spectators = append(r.spectators, p)
	r.broadcast()
	return p, nil
}

func (r *Room) resume(p *Player, conn Conn) {
	if p.graceTime != nil {
		p.graceTime.Stop()
		p.graceTime = nil
	}
	if p.conn != nil && p.conn != conn {
		p.conn.Send(map[string]string{"t": "replaced"})
	}
	p.conn, p.Online = conn, true
	if h := r.host(); r.seatOf(p) >= 0 && (h == nil || !h.Online) {
		r.hostID = p.ID
	}
	r.broadcast()
}

// rosterChanged: đổi người thì bỏ dấu "chơi lại". Kết quả ván vừa xong vẫn giữ để người ở lại còn xem.
func (r *Room) rosterChanged() {
	for _, p := range r.players {
		p.ready = false
	}
	r.broadcast()
}

// seatAdded: có người / máy mới ngồi vào ghế thì quay về phòng chờ, ván mới chia lại X / O.
func (r *Room) seatAdded() {
	if r.phase == PhaseOver {
		r.phase = PhaseLobby
		r.board, r.result, r.winLine, r.last, r.loserMark, r.moves, r.turn = NewBoard(), nil, nil, nil, 0, 0, X
	}
	for _, p := range r.players {
		p.Mark = 0
	}
	r.rosterChanged()
}

func (r *Room) remove(p *Player, notify bool) {
	if p.graceTime != nil {
		p.graceTime.Stop()
	}
	if notify && p.conn != nil {
		p.conn.Send(map[string]string{"t": "left"})
	}
	if i := slices.Index(r.spectators, p); i >= 0 {
		r.spectators = slices.Delete(r.spectators, i, i+1)
		if r.empty() {
			r.close()
			return
		}
		r.broadcast()
		return
	}
	i := r.seatOf(p)
	if i < 0 {
		return
	}
	r.players = slices.Delete(r.players, i, i+1)
	if r.empty() {
		r.close()
		return
	}
	if r.hostID == p.ID {
		r.pickHost()
	}
	r.rosterChanged()
}

// empty: không còn người thật nào (ghế hay xem) thì đóng bàn.
func (r *Room) empty() bool { return len(r.humans()) == 0 && len(r.spectators) == 0 }

func (r *Room) pickHost() {
	r.hostID = ""
	for _, p := range r.humans() {
		if p.Online {
			r.hostID = p.ID
			return
		}
	}
	if h := r.humans(); len(h) > 0 {
		r.hostID = h[0].ID
	}
}

func (r *Room) close() {
	r.closed = true
	r.stopTimer()
	for _, p := range append(slices.Clone(r.players), r.spectators...) {
		if p.graceTime != nil {
			p.graceTime.Stop()
		}
	}
	r.hub.delete(r.Code)
}

// Message là tin nhắn từ client.
type Message struct {
	T     string `json:"t"`
	Token string `json:"token,omitempty"`
	Code  string `json:"code,omitempty"`
	Bet   *int   `json:"bet,omitempty"`
	Rule  string `json:"rule,omitempty"`
	Level string `json:"level,omitempty"`
	ID    string `json:"id,omitempty"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

// Handle xử lý một hành động của người đã ở trong bàn.
func (r *Room) Handle(p *Player, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	seated := r.seatOf(p) >= 0
	if !seated && !slices.Contains(r.spectators, p) {
		return errors.New("bạn không còn ở trong bàn")
	}
	isHost := p.ID == r.hostID
	inLobby := r.phase != PhasePlaying

	switch m.T {
	case "leave":
		r.leave(p)
		return nil
	}
	if !seated {
		return errors.New("bạn đang xem, chưa có ghế")
	}

	switch m.T {
	case "addBot":
		level := m.Level
		if !slices.Contains(BotLevels, level) {
			level = BotNormal
		}
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được thêm máy")
		case !inLobby:
			return errors.New("ván đang chơi")
		case len(r.players) >= MaxPlayers:
			return errors.New("bàn đã đủ 2 người")
		}
		r.botSeq++
		names := map[string]string{BotEasy: "Máy dễ", BotNormal: "Máy", BotHard: "Máy khó"}
		r.players = append(r.players, &Player{ID: randomID(), Name: names[level], Bot: true, BotLevel: level, Online: true})
		r.seatAdded()

	case "kick":
		var target *Player
		for _, q := range r.players {
			if q != p && (m.ID == "" || q.ID == m.ID) {
				target = q
			}
		}
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được mời ra")
		case !inLobby:
			return errors.New("ván đang chơi")
		case target == nil:
			return errors.New("không có ai để mời ra")
		}
		r.remove(target, true)

	case "setBet":
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được đổi mức cược")
		case !inLobby:
			return errors.New("ván đang chơi")
		case m.Bet == nil || !slices.Contains(BetOptions, *m.Bet):
			return errors.New("mức cược không hợp lệ")
		}
		r.bet = *m.Bet
		r.broadcast()

	case "setRule":
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được đổi luật")
		case !inLobby:
			return errors.New("ván đang chơi")
		case !slices.Contains(Rules, m.Rule):
			return errors.New("luật không hợp lệ")
		}
		r.rule = m.Rule
		r.broadcast()

	case "start":
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được bắt đầu")
		case !inLobby:
			return errors.New("ván đang chơi")
		}
		return r.startGame()

	case "rematch":
		if r.phase != PhaseOver {
			return errors.New("chưa hết ván")
		}
		p.ready = true
		for _, q := range r.players {
			if !q.Bot && !q.ready {
				r.broadcast()
				return nil
			}
		}
		return r.startGame()

	case "move":
		if r.phase != PhasePlaying {
			return errors.New("chưa bắt đầu ván")
		}
		if p.Mark != r.turn {
			return errors.New("chưa tới lượt bạn")
		}
		return r.place(m.X, m.Y)

	case "resign":
		if r.phase != PhasePlaying {
			return errors.New("chưa bắt đầu ván")
		}
		r.finish(other(p.Mark), "resign")

	default:
		return fmt.Errorf("không hiểu lệnh %q", m.T)
	}
	return nil
}

// leave: người xem rời thì thôi; người chơi rời giữa ván thì thua ván ("left").
func (r *Room) leave(p *Player) {
	if r.phase == PhasePlaying && r.seatOf(p) >= 0 {
		r.finish(other(p.Mark), "left")
	}
	r.remove(p, true)
}

// Disconnect: mất kết nối. Có Grace để vào lại; quá hạn thì giữa ván xử thua, ngoài ván thì bỏ ghế.
func (r *Room) Disconnect(p *Player, conn Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.conn != conn || (r.seatOf(p) < 0 && !slices.Contains(r.spectators, p)) {
		return
	}
	p.conn, p.Online = nil, false
	p.graceTime = time.AfterFunc(r.opts.Grace, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if p.Online || r.closed {
			return
		}
		if r.phase == PhasePlaying && r.seatOf(p) >= 0 {
			r.finish(other(p.Mark), "left")
		}
		r.remove(p, false)
	})
	r.broadcast()
}

func (r *Room) startGame() error {
	if len(r.players) < MaxPlayers {
		r.broadcast()
		return errors.New("cần 2 người, hãy mời bạn hoặc thêm máy")
	}
	for _, p := range r.players {
		if p.Bot {
			continue
		}
		if err := r.refreshCoins(p); err != nil {
			return err
		}
		if p.Coins < int64(r.bet) {
			r.broadcast()
			return fmt.Errorf("%s không đủ %d xu để vào ván", p.Name, r.bet)
		}
	}
	// Ai cầm X (đi trước): ván đầu là chủ bàn; sau đó người thua ván trước; hoà thì đổi lượt.
	a, b := r.players[0], r.players[1]
	switch {
	case r.loserMark != 0 && r.byMark(r.loserMark) != nil:
		loser := r.byMark(r.loserMark)
		winner := a
		if loser == a {
			winner = b
		}
		loser.Mark, winner.Mark = X, O
	case a.Mark != 0 && b.Mark != 0 && r.result != nil: // hoà: đổi bên
		a.Mark, b.Mark = b.Mark, a.Mark
	default:
		first := r.host()
		if first == nil {
			first = a
		}
		for _, p := range r.players {
			p.Mark = O
		}
		first.Mark = X
	}
	for _, p := range r.players {
		p.ready = false
	}
	r.board = NewBoard()
	r.turn, r.moves, r.last, r.winLine, r.result, r.loserMark = X, 0, nil, nil, nil, 0
	r.gameBet = r.bet
	r.phase = PhasePlaying
	r.schedule()
	r.broadcast()
	return nil
}

// place đặt quân của bên đang tới lượt, rồi kiểm tra thắng / hoà.
func (r *Room) place(x, y int) error {
	if !inside(x, y) || r.board.at(x, y) != Empty {
		return errors.New("ô này không đánh được")
	}
	r.board[y*Size+x] = r.turn
	r.moves++
	r.last = &[2]int{x, y}
	if line := r.board.WinLine(x, y, r.rule); line != nil {
		r.winLine = line
		r.finish(r.turn, "five")
		return nil
	}
	if r.board.Full() {
		r.finish(0, "draw")
		return nil
	}
	r.turn = other(r.turn)
	r.schedule()
	r.broadcast()
	return nil
}

// finish chốt ván: winner là X / O, 0 khi hoà. Trả cược và XP vào ví.
func (r *Room) finish(winner byte, reason string) {
	r.stopTimer()
	r.phase = PhaseOver
	res := &Result{Reason: reason, Money: map[string]int64{}, XP: map[string]int64{}}
	if winner != 0 {
		res.Winner = string(winner)
		r.loserMark = other(winner)
	} else {
		r.loserMark = 0
	}
	deltas, xp := map[int64]int64{}, map[int64]int64{}
	for _, p := range r.players {
		var d, gain int64
		switch {
		case winner == 0:
			gain = XPDraw
		case p.Mark == winner:
			d, gain = int64(r.gameBet), XPWin
		default:
			d, gain = -int64(r.gameBet), XPLose
		}
		res.Money[p.ID] = d
		if p.Bot {
			continue // máy là nhà cái, không có ví
		}
		res.XP[p.ID] = gain
		xp[p.UserID] = gain
		if d != 0 {
			deltas[p.UserID] = d
		}
	}
	r.result = res
	ctx := context.Background()
	if len(deltas) > 0 {
		note := fmt.Sprintf("Cờ Caro bàn %s", r.Code)
		balances, err := r.hub.bank.Settle(ctx, GameID, note, deltas)
		if err != nil {
			log.Printf("caro: ghi kết quả %s: %v", note, err)
		}
		for _, p := range r.players {
			if bal, ok := balances[p.UserID]; ok && !p.Bot {
				p.Coins = bal
			}
		}
	}
	if len(xp) > 0 {
		levels, err := r.hub.bank.AwardXP(ctx, xp)
		if err != nil {
			log.Printf("caro: cộng XP bàn %s: %v", r.Code, err)
		}
		for _, p := range r.players {
			if lv, ok := levels[p.UserID]; ok && !p.Bot {
				p.Level = lv
			}
		}
	}
	r.broadcast()
}

func (r *Room) stopTimer() {
	r.timerGen++
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.deadline = time.Time{}
}

// schedule: tới lượt máy thì máy đánh sau BotDelay; tới lượt người thì có TurnTimeout, hết giờ là thua.
func (r *Room) schedule() {
	r.stopTimer()
	cur := r.byMark(r.turn)
	if cur == nil {
		return
	}
	gen := r.timerGen
	if cur.Bot {
		r.timer = time.AfterFunc(r.opts.BotDelay, func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if gen != r.timerGen || r.phase != PhasePlaying {
				return
			}
			x, y, ok := BestMove(r.board, r.turn, r.rule, cur.BotLevel, r.rng)
			if !ok {
				r.finish(0, "draw")
				return
			}
			r.place(x, y)
		})
		return
	}
	r.deadline = time.Now().Add(r.opts.TurnTimeout)
	r.timer = time.AfterFunc(r.opts.TurnTimeout, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.timerGen || r.phase != PhasePlaying {
			return
		}
		r.finish(other(r.turn), "timeout")
	})
}

func (r *Room) broadcast() {
	for _, p := range append(slices.Clone(r.players), r.spectators...) {
		if p.conn != nil {
			p.conn.Send(r.stateFor(p))
		}
	}
}

// ---------- Trạng thái gửi client ----------

type PlayerView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Gender   string `json:"gender,omitempty"`
	Outfit   string `json:"outfit,omitempty"`
	Hair     string `json:"hair,omitempty"`
	Wings    string `json:"wings,omitempty"`
	Face     string `json:"face,omitempty"`
	Level    int    `json:"level,omitempty"`
	Bot      bool   `json:"bot"`
	BotLevel string `json:"botLevel,omitempty"`
	Host     bool   `json:"host"`
	Online   bool   `json:"online"`
	Mark     string `json:"mark"`
	Coins    int64  `json:"coins"`
	Ready    bool   `json:"ready"`
}

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type State struct {
	T          string       `json:"t"`
	Code       string       `json:"code"`
	You        string       `json:"you"` // id ghế của người xem trạng thái, "" nếu đang xem
	Phase      string       `json:"phase"`
	Size       int          `json:"size"`
	Board      string       `json:"board"`
	Turn       string       `json:"turn"`
	Players    []PlayerView `json:"players"`
	Last       *Point       `json:"last"`
	WinLine    [][2]int     `json:"winLine"`
	Result     *Result      `json:"result"`
	Bet        int          `json:"bet"`
	GameBet    int          `json:"gameBet"`
	BetOptions []int        `json:"betOptions"`
	Rule       string       `json:"rule"`
	TurnMs     int          `json:"turnMs"`
	Spectators int          `json:"spectators"`
	Moves      int          `json:"moves"`
}

func (r *Room) stateFor(viewer *Player) State {
	s := State{T: "state", Code: r.Code, Phase: r.phase, Size: Size, Board: r.board.String(), Turn: string(r.turn),
		Players: []PlayerView{}, WinLine: r.winLine, Result: r.result, Bet: r.bet, GameBet: r.gameBet,
		BetOptions: BetOptions, Rule: r.rule, Spectators: len(r.spectators), Moves: r.moves}
	if r.seatOf(viewer) >= 0 {
		s.You = viewer.ID
	}
	if r.last != nil {
		s.Last = &Point{r.last[0], r.last[1]}
	}
	if !r.deadline.IsZero() {
		s.TurnMs = max(0, int(time.Until(r.deadline).Milliseconds()))
	}
	for _, p := range r.players {
		v := PlayerView{ID: p.ID, Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Hair: p.Hair, Wings: p.Wings, Face: p.Face,
			Level: p.Level, Bot: p.Bot, BotLevel: p.BotLevel, Host: p.ID == r.hostID, Online: p.Online, Coins: p.Coins, Ready: p.ready}
		if p.Mark != 0 {
			v.Mark = string(p.Mark)
		}
		s.Players = append(s.Players, v)
	}
	return s
}
