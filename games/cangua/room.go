package cangua

// Bàn Cờ Cá Ngựa online: 2–4 ghế (người hoặc máy), người xem, lượt 20 giây, cược và XP vào ví chung.
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

	"github.com/dbphx/game-center/internal/chat"
)

const (
	GameID     = "cangua"
	MinPlayers = 2
	MaxPlayers = 4
	DefaultBet = 10

	PhaseLobby   = "lobby"
	PhasePlaying = "playing"
	PhaseOver    = "over"

	// XP mỗi ván cho người thật (kể cả bàn 0 xu).
	XPWin   = 25
	XPOther = 10

	maxSixes = 3 // ra 6 lần thứ 3 liên tiếp thì mất lượt
)

// BetOptions là các mức cược chủ bàn được chọn; 0 là chơi vui.
var BetOptions = []int{0, 10, 20, 50, 100, 200, 500, 1000}

func colorOf(seat int) string { return Colors[seat%len(Colors)] }

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
	BotDelay    time.Duration // máy "suy nghĩ" trước khi đổ (sau khi hoạt ảnh lượt trước xong)
	TurnTimeout time.Duration // hết giờ thì máy đổ / đi thay người đang tới lượt

	// Nhịp hoạt ảnh của giao diện (0 trong test cho nhanh): xúc xắc lăn + giữ số, ngựa nhảy từng ô.
	// Máy và đồng hồ của người tiếp theo chỉ bắt đầu khi hoạt ảnh đã xong.
	RollShow     time.Duration // từ lúc đổ tới khi số hiện xong (lăn ~1.2 s + giữ ~0.6 s)
	BotMoveDelay time.Duration // máy chờ thêm sau khi số hiện rồi mới đi ngựa
	StepAnim     time.Duration // mỗi ô ngựa nhảy
	MaxMoveAnim  time.Duration // trần thời gian ngựa chạy một nước
	Grace        time.Duration // chờ người mất kết nối quay lại; quá hạn giữa ván thì máy đánh thay
	MinHP        int           // máu tối thiểu để ngồi ghế (0 = không kiểm tra)
}

var DefaultOptions = Options{BotDelay: 1400 * time.Millisecond, TurnTimeout: 20 * time.Second, Grace: 30 * time.Second,
	RollShow: 1800 * time.Millisecond, BotMoveDelay: 300 * time.Millisecond, StepAnim: 250 * time.Millisecond,
	MaxMoveAnim: 3500 * time.Millisecond}

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
	auto     bool // máy đang đánh thay (rời bàn / mất kết nối giữa ván)
	left     bool // đã rời bàn giữa ván, bỏ ghế khi hết ván
	ready    bool // đã bấm chơi lại

	conn      Conn
	graceTime *time.Timer
	chatLim   chat.Limiter
}

// Kick là ngựa bị đá trong nước vừa đi.
type Kick struct {
	Color string `json:"color"`
	Horse int    `json:"horse"`
}

// LastMove là nước vừa đi, để giao diện vẽ ngựa chạy.
type LastMove struct {
	Color  string `json:"color"`
	Horse  int    `json:"horse"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	Kicked *Kick  `json:"kicked,omitempty"`
}

// Result là kết quả ván vừa xong.
type Result struct {
	Money map[string]int64 `json:"money"`
	XP    map[string]int64 `json:"xp"`
}

type Room struct {
	Code string

	mu         sync.Mutex
	hub        *Hub
	opts       Options
	hostID     string
	players    []*Player // tối đa 4 ghế, màu theo thứ tự ghế
	spectators []*Player
	phase      string
	bet        int
	gameBet    int
	board      Board
	turn       int  // ghế đang tới lượt
	dice       int  // số vừa đổ, 0 khi chưa đổ
	rolled     bool // lượt này đã đổ, đang chờ đi ngựa
	sixes      int  // số lần ra 6 liên tiếp trong lượt
	last       *LastMove
	winner     string // id người thắng
	lastWinner string // để ván sau người thắng đi trước
	result     *Result
	timer      *time.Timer
	timerGen   int
	deadline   time.Time
	botSeq     int
	closed     bool
	rng        *mrand.Rand
	dicer      func() int // thay xúc xắc trong test; nil = quay ngẫu nhiên
	busyUntil  time.Time  // giao diện đang chạy hoạt ảnh (xúc xắc / ngựa) tới lúc này
	chatLog    chat.Log
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

// join: trả lại ghế / chỗ xem cũ; còn ghế trống (không đang chơi) thì ngồi, không thì xem.
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

// resume: người cũ vào lại (kể cả sau khi máy đã đánh thay) thì lấy lại quyền điều khiển.
func (r *Room) resume(p *Player, conn Conn) {
	if p.graceTime != nil {
		p.graceTime.Stop()
		p.graceTime = nil
	}
	if p.conn != nil && p.conn != conn {
		p.conn.Send(map[string]string{"t": "replaced"})
	}
	p.conn, p.Online, p.auto, p.left = conn, true, false, false
	if h := r.host(); r.seatOf(p) >= 0 && (h == nil || !h.Online) {
		r.hostID = p.ID
	}
	if r.phase == PhasePlaying && r.seatOf(p) == r.turn {
		r.schedule() // đang tới lượt: chuyển hẹn giờ máy sang hẹn giờ người
	}
	r.broadcast()
}

func (r *Room) rosterChanged() {
	for _, p := range r.players {
		p.ready = false
	}
	r.broadcast()
}

// seatAdded: có người / máy mới ngồi thì quay về phòng chờ (giữ kết quả ván trước tới lúc đó).
func (r *Room) seatAdded() {
	if r.phase == PhaseOver {
		r.resetGame()
		r.phase = PhaseLobby
	}
	r.rosterChanged()
}

func (r *Room) resetGame() {
	r.board, r.result, r.last, r.winner = nil, nil, nil, ""
	r.dice, r.rolled, r.sixes = 0, false, 0
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
	if r.phase == PhaseOver {
		r.resetGame()
		r.phase = PhaseLobby
	}
	if r.empty() {
		r.close()
		return
	}
	if r.hostID == p.ID {
		r.pickHost()
	}
	r.rosterChanged()
}

// empty: không còn người thật nào có mặt (ghế hay xem) thì đóng bàn.
func (r *Room) empty() bool {
	for _, p := range r.humans() {
		if !p.left {
			return false
		}
	}
	return len(r.spectators) == 0
}

func (r *Room) pickHost() {
	r.hostID = ""
	for _, p := range r.humans() {
		if p.Online && !p.left {
			r.hostID = p.ID
			return
		}
	}
	for _, p := range r.humans() {
		if !p.left {
			r.hostID = p.ID
			return
		}
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
	Level string `json:"level,omitempty"`
	ID    string `json:"id,omitempty"`
	Horse int    `json:"horse"`
	Text  string `json:"text,omitempty"`
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
	case "chat": // người xem cũng được chat
		text := chat.Prepare(m.Text)
		if text == "" {
			return nil
		}
		now := time.Now()
		if !p.chatLim.Allow(now) {
			return chat.ErrTooFast
		}
		line := r.chatLog.Add(chat.Line{ID: p.ID, Name: p.Name, Text: text, At: now.UnixMilli()})
		msg := map[string]any{"t": "chat", "id": line.ID, "name": line.Name, "text": line.Text, "at": line.At, "seq": line.Seq}
		for _, o := range append(slices.Clone(r.players), r.spectators...) {
			if o.conn != nil {
				o.conn.Send(msg)
			}
		}
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
			return errors.New("bàn đã đủ 4 người")
		}
		r.botSeq++
		names := map[string]string{BotEasy: "Máy dễ", BotNormal: "Máy", BotHard: "Máy khó"}
		r.players = append(r.players, &Player{ID: randomID(), Name: fmt.Sprintf("%s %d", names[level], r.botSeq),
			Bot: true, BotLevel: level, Online: true})
		r.seatAdded()

	case "kick":
		target := r.findPlayer(m.ID)
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được mời ra")
		case !inLobby:
			return errors.New("ván đang chơi")
		case target == nil || target == p:
			return errors.New("không mời ra được người này")
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

	case "roll":
		if err := r.myTurn(p); err != nil {
			return err
		}
		if r.rolled {
			return errors.New("bạn đã đổ rồi, hãy chọn ngựa để đi")
		}
		r.roll()

	case "move":
		if err := r.myTurn(p); err != nil {
			return err
		}
		if !r.rolled {
			return errors.New("hãy đổ xúc xắc trước")
		}
		if r.stuck() {
			return errors.New("lượt này không đi được ngựa nào")
		}
		mv, ok := r.board.target(r.turn, m.Horse, r.dice)
		if m.Horse < 0 || m.Horse >= Horses || !ok {
			return errors.New("ngựa này không đi được")
		}
		r.apply(mv)

	default:
		return fmt.Errorf("không hiểu lệnh %q", m.T)
	}
	return nil
}

func (r *Room) myTurn(p *Player) error {
	switch {
	case r.phase != PhasePlaying:
		return errors.New("chưa bắt đầu ván")
	case r.seatOf(p) != r.turn:
		return errors.New("chưa tới lượt bạn")
	}
	return nil
}

// leave: người xem rời thì thôi; người chơi rời giữa ván thì máy đánh thay tới hết ván (vẫn ăn / thua tiền).
func (r *Room) leave(p *Player) {
	if r.phase == PhasePlaying && r.seatOf(p) >= 0 {
		if p.conn != nil {
			p.conn.Send(map[string]string{"t": "left"})
		}
		if p.graceTime != nil {
			p.graceTime.Stop()
			p.graceTime = nil
		}
		p.conn, p.Online, p.auto, p.left = nil, false, true, true
		if r.hostID == p.ID {
			r.pickHost()
		}
		if r.empty() {
			r.fastForward() // không còn ai xem: máy chơi nốt ván ngay để chốt tiền
			r.close()
			return
		}
		if r.seatOf(p) == r.turn {
			r.schedule()
		}
		r.broadcast()
		return
	}
	r.remove(p, true)
}

// Disconnect: mất kết nối. Có Grace để vào lại; quá hạn thì giữa ván máy đánh thay, ngoài ván thì bỏ ghế.
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
			p.auto = true
			if r.seatOf(p) == r.turn {
				r.schedule()
			}
			r.broadcast()
			return
		}
		r.remove(p, false)
	})
	r.broadcast()
}

func (r *Room) startGame() error {
	// Ai đã rời từ ván trước thì bỏ ghế.
	for _, p := range slices.Clone(r.players) {
		if p.left {
			r.players = slices.DeleteFunc(r.players, func(q *Player) bool { return q == p })
		}
	}
	if len(r.players) < MinPlayers {
		r.broadcast()
		return errors.New("cần ít nhất 2 người, hãy mời bạn hoặc thêm máy")
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
	for _, p := range r.players {
		p.ready, p.auto = false, false
	}
	r.resetGame()
	r.board = NewBoard(len(r.players))
	r.turn = 0
	if w := r.findPlayer(r.lastWinner); w != nil { // người thắng ván trước đi trước
		r.turn = r.seatOf(w)
	} else if h := r.host(); h != nil {
		r.turn = r.seatOf(h)
	}
	r.gameBet = r.bet
	r.phase = PhasePlaying
	r.schedule()
	r.broadcast()
	return nil
}

// roll đổ xúc xắc cho ghế đang tới lượt.
func (r *Room) roll() {
	d := r.rng.IntN(6) + 1
	if r.dicer != nil {
		d = r.dicer()
	}
	r.dice, r.rolled = d, true
	r.busy(r.opts.RollShow)
	if d == 6 {
		r.sixes++
		if r.sixes >= maxSixes { // 3 lần 6 liên tiếp: mất lượt (giữ số trên màn hình rồi mới chuyển)
			r.schedule()
			r.broadcast()
			return
		}
	}
	if len(r.board.Moves(r.turn, d)) == 0 {
		if d == 6 { // không đi được nhưng ra 6 thì vẫn được đổ tiếp
			r.rolled = false
		}
		// Không đi được: giữ số hiện trên màn hình hết RollShow rồi mới chuyển lượt (schedule → step).
		r.schedule()
		r.broadcast()
		return
	}
	r.schedule()
	r.broadcast()
}

// apply đi ngựa, kiểm tra thắng, rồi ra 6 thì đổ tiếp, không thì sang lượt người sau.
func (r *Room) apply(mv Move) {
	r.board.Apply(mv)
	r.busy(min(time.Duration(cells(mv))*r.opts.StepAnim, r.opts.MaxMoveAnim))
	r.last = &LastMove{Color: colorOf(mv.Seat), Horse: mv.Horse, From: mv.From, To: mv.To}
	if mv.KickSeat >= 0 {
		r.last.Kicked = &Kick{Color: colorOf(mv.KickSeat), Horse: mv.KickHorse}
	}
	if r.board.Won(mv.Seat) {
		r.finish(r.players[mv.Seat])
		return
	}
	if r.dice == 6 {
		r.rolled = false
		r.schedule()
		r.broadcast()
		return
	}
	r.nextTurn()
}

// stuck: đã đổ nhưng không được đi (3 lần 6 hoặc không có nước) — chỉ chờ hoạt ảnh rồi sang lượt.
func (r *Room) stuck() bool {
	return r.rolled && (r.sixes >= maxSixes || len(r.board.Moves(r.turn, r.dice)) == 0)
}

// busy: giao diện cần thêm d để chạy hoạt ảnh, tính từ bây giờ.
func (r *Room) busy(d time.Duration) {
	if d > 0 {
		r.busyUntil = time.Now().Add(d)
	}
}

// cells là số ô ngựa nhảy trong nước đi (để tính thời gian hoạt ảnh).
func cells(mv Move) int {
	switch {
	case mv.From == Home:
		return 1
	case mv.From >= FinishBase:
		return mv.To - mv.From
	case mv.To >= FinishBase:
		return mv.To - FinishBase
	}
	return (mv.To - mv.From + Track) % Track
}

func (r *Room) nextTurn() {
	r.turn = (r.turn + 1) % len(r.players)
	r.rolled, r.sixes = false, 0
	r.schedule()
	r.broadcast()
}

// step: máy (hoặc người hết giờ / vắng mặt) làm một bước: chưa đổ thì đổ, đã đổ thì đi nước tốt nhất.
func (r *Room) step() {
	cur := r.players[r.turn]
	if !r.rolled {
		r.roll()
		return
	}
	if r.stuck() {
		r.nextTurn()
		return
	}
	level := cur.BotLevel
	if level == "" {
		level = BotNormal
	}
	if mv, ok := BestMove(r.board, r.turn, r.dice, level, r.rng); ok {
		r.apply(mv)
		return
	}
	r.nextTurn()
}

// fastForward chơi nốt ván ngay (mọi ghế do máy đi) — dùng khi không còn người nào có mặt.
func (r *Room) fastForward() {
	r.stopTimer()
	for i := 0; i < 20000 && r.phase == PhasePlaying; i++ {
		r.step()
		r.stopTimer()
	}
}

// finish chốt ván: người thắng ăn mỗi người thua một mức cược (máy là nhà cái, không có ví).
func (r *Room) finish(w *Player) {
	r.stopTimer()
	r.phase = PhaseOver
	r.winner, r.lastWinner = w.ID, w.ID
	r.rolled = false
	res := &Result{Money: map[string]int64{}, XP: map[string]int64{}}
	deltas, xp := map[int64]int64{}, map[int64]int64{}
	losers := int64(len(r.players) - 1)
	for _, p := range r.players {
		d, gain := -int64(r.gameBet), int64(XPOther)
		if p == w {
			d, gain = int64(r.gameBet)*losers, XPWin
		}
		res.Money[p.ID] = d
		if p.Bot {
			continue
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
		note := fmt.Sprintf("Cờ Cá Ngựa bàn %s", r.Code)
		balances, err := r.hub.bank.Settle(ctx, GameID, note, deltas)
		if err != nil {
			log.Printf("cangua: ghi kết quả %s: %v", note, err)
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
			log.Printf("cangua: cộng XP bàn %s: %v", r.Code, err)
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

// schedule: lượt máy (hoặc người đang được máy đánh thay) thì máy đổ sau BotDelay / đi sau BotMoveDelay;
// lượt người thì có TurnTimeout, hết giờ máy đổ / đi thay một bước rồi hẹn giờ lại.
// Mọi khoảng chờ đều cộng thêm phần hoạt ảnh còn lại (busyUntil).
func (r *Room) schedule() {
	r.stopTimer()
	if r.phase != PhasePlaying {
		return
	}
	cur := r.players[r.turn]
	gen := r.timerGen
	anim := max(0, time.Until(r.busyUntil)) // chờ hoạt ảnh lượt trước xong
	wait := anim + r.opts.TurnTimeout
	switch {
	case r.stuck(): // ai cũng vậy: hiện số xong là sang lượt
		wait = anim
	case (cur.Bot || cur.auto) && r.rolled:
		wait = anim + r.opts.BotMoveDelay
	case cur.Bot || cur.auto:
		wait = anim + r.opts.BotDelay
	default:
		r.deadline = time.Now().Add(wait) // đồng hồ 20 s của người chỉ chạy sau hoạt ảnh
	}
	r.timer = time.AfterFunc(wait, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.timerGen || r.phase != PhasePlaying || r.closed {
			return
		}
		r.step()
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
	Auto     bool   `json:"auto"` // máy đang đánh thay
	Color    string `json:"color"`
	Coins    int64  `json:"coins"`
	Ready    bool   `json:"ready"`
}

type State struct {
	T          string           `json:"t"`
	Code       string           `json:"code"`
	You        string           `json:"you"` // id ghế của người xem trạng thái, "" nếu đang xem
	Phase      string           `json:"phase"`
	Players    []PlayerView     `json:"players"`
	Turn       string           `json:"turn"` // id người tới lượt
	Dice       *int             `json:"dice"`
	Rolled     bool             `json:"rolled"`
	Sixes      int              `json:"sixes"`
	Movable    []int            `json:"movable"`
	Horses     map[string][]int `json:"horses"`
	Last       *LastMove        `json:"last"`
	Winner     *string          `json:"winner"`
	Result     *Result          `json:"result"`
	Bet        int              `json:"bet"`
	GameBet    int              `json:"gameBet"`
	BetOptions []int            `json:"betOptions"`
	TurnMs     int              `json:"turnMs"`
	Spectators int              `json:"spectators"`
	Chat       []chat.Line      `json:"chat"`
}

func (r *Room) stateFor(viewer *Player) State {
	s := State{T: "state", Code: r.Code, Phase: r.phase, Players: []PlayerView{}, Rolled: r.rolled, Sixes: r.sixes,
		Movable: []int{}, Horses: map[string][]int{}, Last: r.last, Result: r.result, Bet: r.bet, GameBet: r.gameBet,
		BetOptions: BetOptions, Spectators: len(r.spectators), Chat: r.chatLog.List()}
	seat := r.seatOf(viewer)
	if seat >= 0 {
		s.You = viewer.ID
	}
	if r.dice > 0 {
		d := r.dice
		s.Dice = &d
	}
	if r.winner != "" {
		w := r.winner
		s.Winner = &w
	}
	if !r.deadline.IsZero() {
		s.TurnMs = max(0, int(time.Until(r.deadline).Milliseconds()))
	}
	if r.phase == PhasePlaying && r.turn < len(r.players) {
		s.Turn = r.players[r.turn].ID
		if seat == r.turn && r.rolled && !r.stuck() {
			for _, mv := range r.board.Moves(r.turn, r.dice) {
				s.Movable = append(s.Movable, mv.Horse)
			}
		}
	}
	for i := range r.board {
		s.Horses[colorOf(i)] = slices.Clone(r.board[i][:])
	}
	for i, p := range r.players {
		s.Players = append(s.Players, PlayerView{ID: p.ID, Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Hair: p.Hair,
			Wings: p.Wings, Face: p.Face, Level: p.Level, Bot: p.Bot, BotLevel: p.BotLevel, Host: p.ID == r.hostID,
			Online: p.Online, Auto: p.auto, Color: colorOf(i), Coins: p.Coins, Ready: p.ready})
	}
	return s
}
