package arena

// Bàn Đấu trường online: 2–4 ghế (người hoặc máy), người xem, bắn theo lượt (góc, lực, gió), cược và XP vào ví chung.
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
	"math"
	mrand "math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/dbphx/game-center/internal/chat"
)

const (
	GameID     = "arena"
	MinPlayers = 2
	MaxPlayers = 4
	DefaultBet = 10
	MaxRounds  = 40 // mỗi người tối đa 40 lượt

	PhaseLobby   = "lobby"
	PhasePlaying = "playing"
	PhaseOver    = "over"

	// XP mỗi trận cho người thật (kể cả bàn 0 xu).
	XPWin  = 30
	XPDraw = 15
	XPLose = 10

	aimEvery = 100 * time.Millisecond // tối đa ~10 tin ngắm mỗi giây
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

// GearSource đọc trang bị vĩnh viễn của người chơi (Armory); nil thì mọi người cấp 0.
type GearSource interface {
	Gear(ctx context.Context, userID int64) (Gear, error)
}

// Conn là kết nối tới một người. Send không được chặn vì được gọi khi đang giữ khoá bàn.
type Conn interface {
	Send(msg any)
}

type Options struct {
	BotDelay    time.Duration // thời gian máy "ngắm"
	TurnTimeout time.Duration // hết giờ thì mất lượt
	Grace       time.Duration // chờ người mất kết nối quay lại
	AfterShot   time.Duration // nghỉ sau khi đạn bay xong rồi mới sang lượt
	ReplayScale float64       // thời gian bay thật × hệ số này (giao diện diễn quỹ đạo); 0 trong test
	MaxRounds   int           // 0 = MaxRounds
	MinHP       int           // máu tối thiểu để ngồi ghế (0 = không kiểm tra)
}

var DefaultOptions = Options{BotDelay: 1200 * time.Millisecond, TurnTimeout: 25 * time.Second, Grace: 30 * time.Second,
	AfterShot: 800 * time.Millisecond, ReplayScale: 1}

func (o Options) gate(u User) error {
	if o.MinHP <= 0 {
		return nil
	}
	if u.HospitalUntil > 0 {
		return errors.New("bạn đang nằm viện")
	}
	if u.HP < o.MinHP {
		return fmt.Errorf("cần ít nhất %d HP để vào Đấu trường", o.MinHP)
	}
	return nil
}

func (o Options) maxRounds() int {
	if o.MaxRounds > 0 {
		return o.MaxRounds
	}
	return MaxRounds
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
	Gear     Gear // trang bị hiện tại (trong trận dùng bản chụp ở f.Gear)
	ready    bool // đã bấm chơi lại

	f         *Fighter // trên sân (nil khi chưa vào trận)
	facing    int
	items     map[string]int
	lastAngle float64
	lastPower float64
	aimAt     time.Time

	conn      Conn
	graceTime *time.Timer
	chatLim   chat.Limiter // chống spam chat
}

// Result là kết quả trận vừa xong.
type Result struct {
	Winner string           `json:"winner"` // id người thắng, "" khi hoà
	Reason string           `json:"reason"` // last | rounds | draw | left
	Money  map[string]int64 `json:"money"`
	XP     map[string]int64 `json:"xp"`
}

type Room struct {
	Code string

	mu         sync.Mutex
	hub        *Hub
	opts       Options
	hostID     string
	players    []*Player // tối đa 4 ghế
	spectators []*Player
	phase      string
	bet        int
	gameBet    int
	mapChoice  string // id map hoặc "random"
	field      Map    // map đang dùng (phòng chờ: map xem trước)
	ground     Ground
	wind       int
	turn       string // id người đang tới lượt, "" khi đạn đang bay / ngoài trận
	lastTurn   int    // chỉ số ghế vừa tới lượt (để chuyển lượt kể cả khi ghế đó vừa rời)
	round      int
	moveLeft   int
	leftPot    int64 // tiền cược của người bỏ trận giữa chừng, cộng cho người thắng
	result     *Result
	timer      *time.Timer
	timerGen   int
	deadline   time.Time
	botSeq     int
	closed     bool
	rng        *mrand.Rand
	chatLog    chat.Log // 20 tin chat gần nhất của bàn
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

func (r *Room) refreshGear(p *Player) error {
	if p.Bot || r.hub.gear == nil {
		return nil
	}
	g, err := r.hub.gear.Gear(context.Background(), p.UserID)
	if err == nil {
		p.Gear = g
	}
	return err
}

// pickField chọn map theo lựa chọn của chủ bàn ("random" thì bốc ngẫu nhiên) và sinh địa hình mới.
func (r *Room) pickField() {
	m, ok := findMap(r.mapChoice)
	if !ok {
		m = Maps[r.rng.IntN(len(Maps))]
	}
	r.field = m
	r.ground = m.Generate(r.rng)
}

func newPlayer(u User, conn Conn) *Player {
	return &Player{ID: randomID(), UserID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair,
		Wings: u.Wings, Face: u.Face, Level: u.Level, Online: true, conn: conn, facing: 1}
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
		if err := r.refreshGear(p); err != nil {
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

// rosterChanged: đổi người thì bỏ dấu "chơi lại". Kết quả trận vừa xong vẫn giữ để người ở lại còn xem.
func (r *Room) rosterChanged() {
	for _, p := range r.players {
		p.ready = false
	}
	r.broadcast()
}

// seatAdded: có người / máy mới ngồi vào ghế thì quay về phòng chờ.
func (r *Room) seatAdded() {
	if r.phase == PhaseOver {
		r.toLobby()
	}
	r.rosterChanged()
}

func (r *Room) toLobby() {
	r.phase, r.result, r.turn, r.round, r.wind = PhaseLobby, nil, "", 0, 0
	for _, p := range r.players {
		p.f = nil
	}
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
	if r.lastTurn >= i {
		r.lastTurn--
	}
	if r.empty() {
		r.close()
		return
	}
	if r.hostID == p.ID {
		r.pickHost()
	}
	if r.phase == PhasePlaying {
		r.afterLoss(r.turn == p.ID)
		return
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
	T      string  `json:"t"`
	Token  string  `json:"token,omitempty"`
	Code   string  `json:"code,omitempty"`
	Bet    *int    `json:"bet,omitempty"`
	Map    string  `json:"map,omitempty"`
	Level  string  `json:"level,omitempty"`
	ID     string  `json:"id,omitempty"`
	Text   string  `json:"text,omitempty"` // tin chat
	X      float64 `json:"x"`              // move: đích muốn đi tới
	Angle  float64 `json:"angle"`
	Power  float64 `json:"power"`
	Weapon string  `json:"weapon,omitempty"`
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
		r.sendAll(map[string]any{"t": "chat", "id": line.ID, "name": line.Name, "text": line.Text, "at": line.At, "seq": line.Seq})
		return nil
	}
	if !seated {
		return errors.New("bạn đang xem, chưa có ghế")
	}

	switch m.T {
	case "addBot", "add_bot":
		level := m.Level
		if !slices.Contains(BotLevels, level) {
			level = BotNormal
		}
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được thêm máy")
		case !inLobby:
			return errors.New("trận đang đánh")
		case len(r.players) >= MaxPlayers:
			return fmt.Errorf("bàn đã đủ %d người", MaxPlayers)
		}
		r.botSeq++
		names := map[string]string{BotEasy: "Máy dễ", BotNormal: "Máy", BotHard: "Máy khó"}
		name := names[level]
		if r.botSeq > 1 {
			name = fmt.Sprintf("%s %d", name, r.botSeq)
		}
		r.players = append(r.players, &Player{ID: randomID(), Name: name, Bot: true, BotLevel: level, Online: true,
			Gear: botGear[level], facing: 1})
		r.seatAdded()

	case "kick":
		var target *Player
		for _, q := range r.players {
			if q != p && q.ID == m.ID {
				target = q
			}
		}
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được mời ra")
		case !inLobby:
			return errors.New("trận đang đánh")
		case target == nil:
			return errors.New("không có ai để mời ra")
		}
		r.remove(target, true)

	case "setBet", "setMap", "settings":
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được đổi cài đặt bàn")
		case !inLobby:
			return errors.New("trận đang đánh")
		case m.Bet != nil && !slices.Contains(BetOptions, *m.Bet):
			return errors.New("mức cược không hợp lệ")
		case m.Map != "" && m.Map != MapRandom:
			if _, ok := findMap(m.Map); !ok {
				return errors.New("map không hợp lệ")
			}
		}
		if m.Bet != nil {
			r.bet = *m.Bet
		}
		if m.Map != "" && m.Map != r.mapChoice {
			r.mapChoice = m.Map
			if r.phase == PhaseOver {
				r.toLobby()
			}
			r.pickField()
		}
		r.broadcast()

	case "start":
		switch {
		case !isHost:
			return errors.New("chỉ chủ bàn được bắt đầu")
		case !inLobby:
			return errors.New("trận đang đánh")
		}
		return r.startGame()

	case "rematch", "ready":
		if r.phase != PhaseOver {
			if m.T == "ready" && r.phase == PhaseLobby {
				p.ready = !p.ready
				r.broadcast()
				return nil
			}
			return errors.New("chưa hết trận")
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
		if err := r.myTurn(p); err != nil {
			return err
		}
		nx, used := Walk(r.ground, p.f.X, m.X, r.moveLeft)
		if m.X < p.f.X {
			p.facing = -1
		} else if m.X > p.f.X {
			p.facing = 1
		}
		p.f.X, p.f.Y = nx, float64(r.ground.At(nx))
		r.moveLeft -= used
		r.broadcast()

	case "aim":
		if err := r.myTurn(p); err != nil {
			return err
		}
		p.lastAngle, p.lastPower = clampAim(m.Angle, m.Power)
		p.facing = facingOf(p.lastAngle, p.facing)
		now := time.Now()
		if now.Sub(p.aimAt) < aimEvery {
			return nil
		}
		p.aimAt = now
		msg := map[string]any{"t": "aim", "id": p.ID, "angle": p.lastAngle, "power": p.lastPower}
		for _, o := range append(slices.Clone(r.players), r.spectators...) {
			if o != p && o.conn != nil {
				o.conn.Send(msg)
			}
		}

	case "fire":
		if err := r.myTurn(p); err != nil {
			return err
		}
		w, ok := findWeapon(m.Weapon)
		if m.Weapon == "" {
			w, ok = findWeapon(WeaponShell)
		}
		if !ok {
			return errors.New("vũ khí không hợp lệ")
		}
		if w.Uses > 0 && p.items[w.ID] <= 0 {
			return fmt.Errorf("đã hết %s", w.Name)
		}
		a, pw := clampAim(m.Angle, m.Power)
		r.fire(p, w, a, pw)

	default:
		return fmt.Errorf("không hiểu lệnh %q", m.T)
	}
	return nil
}

func clampAim(angle, power float64) (float64, float64) {
	if math.IsNaN(angle) {
		angle = 45
	}
	if math.IsNaN(power) {
		power = 50
	}
	return math.Max(0, math.Min(180, angle)), math.Max(0, math.Min(100, power))
}

func facingOf(angle float64, cur int) int {
	switch {
	case angle < 90:
		return 1
	case angle > 90:
		return -1
	}
	return cur
}

func (r *Room) myTurn(p *Player) error {
	switch {
	case r.phase != PhasePlaying:
		return errors.New("chưa bắt đầu trận")
	case r.turn != p.ID || p.f == nil || !p.f.Alive:
		return errors.New("chưa tới lượt bạn")
	}
	return nil
}

// leave: người xem rời thì thôi; người chơi rời giữa trận thì thua (mất cược).
func (r *Room) leave(p *Player) {
	r.forfeit(p)
	r.remove(p, true)
}

// forfeit: người đang đánh bỏ trận thì mất tiền cược ngay; tiền đó cộng cho người thắng khi trận xong.
func (r *Room) forfeit(p *Player) {
	if r.phase != PhasePlaying || r.seatOf(p) < 0 || p.f == nil {
		return
	}
	p.f.Alive = false
	if p.Bot || r.gameBet == 0 {
		return
	}
	note := fmt.Sprintf("Đấu trường bàn %s (bỏ trận)", r.Code)
	if _, err := r.hub.bank.Settle(context.Background(), GameID, note, map[int64]int64{p.UserID: -int64(r.gameBet)}); err != nil {
		log.Printf("arena: ghi bỏ trận %s: %v", note, err)
		return
	}
	r.leftPot += int64(r.gameBet)
}

// Disconnect: mất kết nối. Có Grace để vào lại; quá hạn thì giữa trận xử thua, ngoài trận thì bỏ ghế.
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
		r.forfeit(p)
		r.remove(p, false)
	})
	r.broadcast()
}

func (r *Room) startGame() error {
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
			return fmt.Errorf("%s không đủ %d xu để vào trận", p.Name, r.bet)
		}
		if err := r.refreshGear(p); err != nil {
			return err
		}
	}
	r.pickField()
	xs := spawnXs(r.ground, len(r.players), r.rng)
	for i, p := range r.players {
		x := xs[i]
		p.f = &Fighter{ID: p.ID, X: x, Y: float64(r.ground.At(x)), HP: p.Gear.MaxHP(), MaxHP: p.Gear.MaxHP(), Alive: true, Gear: p.Gear}
		p.items = startItems()
		p.ready = false
		p.facing = 1
		if x > Width/2 {
			p.facing = -1
		}
		p.lastAngle, p.lastPower = 90-float64(p.facing)*35, 60
	}
	r.result, r.leftPot = nil, 0
	r.gameBet = r.bet
	r.phase = PhasePlaying
	r.round = 1
	first := r.rng.IntN(len(r.players))
	r.beginTurn(first)
	return nil
}

func (r *Room) fighters() []*Fighter {
	var out []*Fighter
	for _, p := range r.players {
		if p.f != nil {
			out = append(out, p.f)
		}
	}
	return out
}

func (r *Room) alive() []*Player {
	var out []*Player
	for _, p := range r.players {
		if p.f != nil && p.f.Alive {
			out = append(out, p)
		}
	}
	return out
}

// beginTurn trao lượt cho ghế i: gió mới, quỹ di chuyển mới.
func (r *Room) beginTurn(i int) {
	p := r.players[i]
	r.lastTurn = i
	r.turn = p.ID
	r.moveLeft = MoveFund
	r.wind = r.field.Wind(r.rng)
	r.schedule()
	r.broadcast()
}

// passTurn chuyển lượt cho người còn sống kế tiếp sau ghế from (from = −1: bắt đầu từ ghế đầu).
// Đi hết một vòng ghế thì sang vòng mới; quá số vòng tối đa thì chốt trận theo máu.
func (r *Room) passTurn(from int) {
	n := len(r.players)
	for k := 1; k <= n; k++ {
		j := from + k
		wrapped := j >= n
		j %= n
		p := r.players[j]
		if p.f == nil || !p.f.Alive {
			continue
		}
		if wrapped {
			r.round++
			if r.round > r.opts.maxRounds() {
				r.round = r.opts.maxRounds()
				r.finishByHP()
				return
			}
		}
		r.beginTurn(j)
		return
	}
	r.finish(nil, "draw")
}

// afterLoss: có người chết / bỏ trận. Còn ≤ 1 người thì chốt trận; người đang tới lượt vừa mất thì sang lượt.
func (r *Room) afterLoss(wasTurn bool) {
	if r.checkEnd("left") {
		return
	}
	if wasTurn {
		r.stopTimer()
		r.passTurn(r.lastTurn)
		return
	}
	r.broadcast()
}

// checkEnd chốt trận khi còn ≤ 1 người sống.
func (r *Room) checkEnd(reason string) bool {
	alive := r.alive()
	switch len(alive) {
	case 0:
		r.finish(nil, "draw")
	case 1:
		r.finish(alive[0], reason)
	default:
		return false
	}
	return true
}

// fire bắn một phát cho p: tính quỹ đạo, nổ, máu; gửi "shot" rồi state; nghỉ cho đạn bay xong rồi sang lượt.
func (r *Room) fire(p *Player, w Weapon, angle, power float64) {
	r.stopTimer()
	if w.Uses > 0 {
		p.items[w.ID]--
	}
	p.lastAngle, p.lastPower = angle, power
	p.facing = facingOf(angle, p.facing)
	res := Fire(r.ground, r.fighters(), p.f, w, angle, power, r.wind)
	shot := map[string]any{"t": "shot", "by": p.ID, "weapon": w.ID, "angle": angle, "power": power, "wind": r.wind,
		"paths": res.Paths, "blasts": orEmpty(res.Blasts), "hits": orEmpty(res.Hits), "deaths": orEmpty(res.Deaths), "tele": res.Tele}
	r.sendAll(shot)
	r.turn = ""
	if r.checkEnd("last") {
		return
	}
	r.broadcast()
	pause := time.Duration(res.Duration*r.opts.ReplayScale*float64(time.Second)) + r.opts.AfterShot
	gen := r.timerGen
	r.timer = time.AfterFunc(pause, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.timerGen || r.phase != PhasePlaying {
			return
		}
		r.passTurn(r.lastTurn) // lastTurn đã được chỉnh nếu có người rời bàn trong lúc đạn bay
	})
}

func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// finishByHP: hết số vòng — ai còn nhiều máu nhất (theo %) thắng; bằng nhau thì hoà.
func (r *Room) finishByHP() {
	var best *Player
	bestPct, tie := -1.0, false
	for _, p := range r.alive() {
		pct := float64(p.f.HP) / float64(max(1, p.f.MaxHP))
		switch {
		case pct > bestPct+1e-9:
			best, bestPct, tie = p, pct, false
		case math.Abs(pct-bestPct) <= 1e-9:
			tie = true
		}
	}
	if tie || best == nil {
		r.finish(nil, "draw")
		return
	}
	r.finish(best, "rounds")
}

// finish chốt trận: winner nil khi hoà. Người thắng ăn cược của mọi người thua (máy là nhà cái) cộng tiền
// người bỏ trận; hoà thì không ai mất xu, tiền người bỏ trận chia đều cho người thật còn lại.
func (r *Room) finish(winner *Player, reason string) {
	r.stopTimer()
	r.phase = PhaseOver
	r.turn = ""
	res := &Result{Reason: reason, Money: map[string]int64{}, XP: map[string]int64{}}
	if winner != nil {
		res.Winner = winner.ID
	} else {
		res.Reason = "draw"
	}
	deltas, xp := map[int64]int64{}, map[int64]int64{}
	var share int64
	if winner == nil && r.leftPot > 0 && len(r.humans()) > 0 {
		share = r.leftPot / int64(len(r.humans()))
	}
	losers := int64(len(r.players) - 1)
	for _, p := range r.players {
		var d, gain int64
		switch {
		case winner == nil:
			d, gain = share, XPDraw
		case p == winner:
			d, gain = int64(r.gameBet)*losers+r.leftPot, XPWin
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
	r.leftPot = 0
	r.result = res
	ctx := context.Background()
	if len(deltas) > 0 {
		note := fmt.Sprintf("Đấu trường bàn %s", r.Code)
		balances, err := r.hub.bank.Settle(ctx, GameID, note, deltas)
		if err != nil {
			log.Printf("arena: ghi kết quả %s: %v", note, err)
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
			log.Printf("arena: cộng XP bàn %s: %v", r.Code, err)
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

// schedule: tới lượt máy thì máy bắn sau BotDelay; tới lượt người thì có TurnTimeout, hết giờ là mất lượt.
func (r *Room) schedule() {
	r.stopTimer()
	cur := r.findPlayer(r.turn)
	if cur == nil {
		return
	}
	gen := r.timerGen
	if cur.Bot {
		r.timer = time.AfterFunc(r.opts.BotDelay, func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if gen != r.timerGen || r.phase != PhasePlaying || r.turn != cur.ID {
				return
			}
			aim := BotAim(r.ground, r.fighters(), cur.f, cur.items, cur.BotLevel, r.wind, r.rng)
			w, _ := findWeapon(aim.Weapon)
			r.fire(cur, w, aim.Angle, aim.Power)
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
		r.passTurn(r.lastTurn)
	})
}

func (r *Room) sendAll(msg any) {
	for _, o := range append(slices.Clone(r.players), r.spectators...) {
		if o.conn != nil {
			o.conn.Send(msg)
		}
	}
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
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Gender    string         `json:"gender,omitempty"`
	Outfit    string         `json:"outfit,omitempty"`
	Hair      string         `json:"hair,omitempty"`
	Wings     string         `json:"wings,omitempty"`
	Face      string         `json:"face,omitempty"`
	Level     int            `json:"level,omitempty"`
	Bot       bool           `json:"bot"`
	BotLevel  string         `json:"botLevel,omitempty"`
	Host      bool           `json:"host"`
	Online    bool           `json:"online"`
	Ready     bool           `json:"ready"`
	Coins     int64          `json:"coins"`
	X         float64        `json:"x"`
	Y         float64        `json:"y"`
	HP        int            `json:"hp"`
	MaxHP     int            `json:"maxHp"`
	Alive     bool           `json:"alive"`
	Facing    int            `json:"facing"`
	Gear      Gear           `json:"gear"`
	Power     int            `json:"power"`
	Items     map[string]int `json:"items"`
	LastAngle float64        `json:"lastAngle"`
	LastPower float64        `json:"lastPower"`
}

// WeaponInfo là vũ khí gửi cho giao diện.
type WeaponInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Dmg  int    `json:"dmg"`
	R    int    `json:"r"`
	Uses int    `json:"uses"` // −1 = vô hạn
	N    int    `json:"n"`    // số viên mỗi phát
}

func weaponList() []WeaponInfo {
	out := make([]WeaponInfo, 0, len(Weapons))
	for _, w := range Weapons {
		out = append(out, WeaponInfo{ID: w.ID, Name: w.Name, Dmg: int(w.Dmg), R: int(w.R), Uses: w.Uses, N: len(w.Spread)})
	}
	return out
}

type State struct {
	T          string       `json:"t"`
	Code       string       `json:"code"`
	You        string       `json:"you"` // id ghế của người xem trạng thái, "" nếu đang xem
	Phase      string       `json:"phase"`
	Map        MapInfo      `json:"map"`
	MapChoice  string       `json:"mapChoice"` // lựa chọn của chủ bàn: id map hoặc "random"
	Maps       []MapInfo    `json:"maps"`
	Weapons    []WeaponInfo `json:"weapons"`
	Ground     Ground       `json:"ground"`
	Wind       int          `json:"wind"`
	Turn       string       `json:"turn"`
	TurnMs     int          `json:"turnMs"`
	Round      int          `json:"round"`
	MaxRounds  int          `json:"maxRounds"`
	MoveLeft   int          `json:"moveLeft"`
	Players    []PlayerView `json:"players"`
	Result     *Result      `json:"result"`
	Bet        int          `json:"bet"`
	GameBet    int          `json:"gameBet"`
	BetOptions []int        `json:"betOptions"`
	Spectators int          `json:"spectators"`
	Chat       []chat.Line  `json:"chat"` // 20 tin chat gần nhất của bàn
}

func (r *Room) stateFor(viewer *Player) State {
	s := State{T: "state", Code: r.Code, Phase: r.phase, Map: MapInfo{r.field.ID, r.field.Name}, MapChoice: r.mapChoice,
		Maps: mapList(), Weapons: weaponList(), Ground: r.ground, Wind: r.wind, Turn: r.turn, Round: r.round,
		MaxRounds: r.opts.maxRounds(), Players: []PlayerView{}, Result: r.result, Bet: r.bet, GameBet: r.gameBet,
		BetOptions: BetOptions, Spectators: len(r.spectators), Chat: r.chatLog.List()}
	if r.seatOf(viewer) >= 0 {
		s.You = viewer.ID
	}
	if r.turn != "" {
		s.MoveLeft = r.moveLeft
	}
	if !r.deadline.IsZero() {
		s.TurnMs = max(0, int(time.Until(r.deadline).Milliseconds()))
	}
	for _, p := range r.players {
		v := PlayerView{ID: p.ID, Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Hair: p.Hair, Wings: p.Wings, Face: p.Face,
			Level: p.Level, Bot: p.Bot, BotLevel: p.BotLevel, Host: p.ID == r.hostID, Online: p.Online, Ready: p.ready,
			Coins: p.Coins, Facing: p.facing, Gear: p.Gear, Power: p.Gear.Power(), Items: map[string]int{},
			LastAngle: p.lastAngle, LastPower: p.lastPower, HP: p.Gear.MaxHP(), MaxHP: p.Gear.MaxHP(), Alive: true}
		if p.f != nil {
			v.X, v.Y, v.HP, v.MaxHP, v.Alive = p.f.X, p.f.Y, p.f.HP, p.f.MaxHP, p.f.Alive
			v.Gear, v.Power = p.f.Gear, p.f.Gear.Power()
		}
		for k, n := range p.items {
			v.Items[k] = n
		}
		s.Players = append(s.Players, v)
	}
	return s
}
