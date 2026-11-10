package race

// Phòng đua online: 2–6 ghế (người hoặc máy), cược xu, đếm ngược rồi đua 3 vòng.
//
// Mỗi máy người chơi tự mô phỏng xe và gửi vị trí ≈15 lần/giây; server kiểm gói, tự tính vòng / về đích,
// chạy máy đua theo đường mẫu và phát vị trí mọi xe ≈15 lần/giây.
// Mọi thao tác trên một Room chạy dưới Room.mu; thứ tự khoá Room.mu → Hub.mu.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	mrand "math/rand/v2"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/dbphx/game-center/internal/chat"
	"github.com/dbphx/game-center/internal/wallet"
)

const (
	MinPlayers = 2
	MaxPlayers = 6
	DefaultBet = 100

	PhaseLobby     = "lobby"
	PhaseCountdown = "countdown"
	PhaseRacing    = "racing"
	PhaseOver      = "over"

	// XP mỗi cuộc đua cho người thật: nhất nhiều nhất, bét vẫn có phần (như Tiến Lên).
	minXP   = 10
	bonusXP = 20

	// Kiểm gói vị trí.
	speedTolerance = 1.1  // tốc độ báo lên được vượt trần (kể cả nitro) 10%
	moveTolerance  = 1.2  // quãng đường giữa hai gói được vượt trần 20%
	moveSlack      = 3.0  // cộng thêm vài mét cho sai số làm tròn / mạng
	minPacketGapMs = 150  // gói tới dồn: tính quãng đường cho phép như cách nhau ít nhất 150 ms
	sampleAhead    = 250  // mốc lấy mẫu (Ms) được sớm hơn lúc nhận tối đa ngần này (lệch đồng hồ) …
	sampleBehind   = 1500 // … và trễ hơn tối đa ngần này; ngoài khoảng thì dùng lúc server nhận
	sampleSlew     = 2    // ms: mỗi gói chỉ chỉnh độ lệch mốc chừng này (≈ 30 ms/giây)
	maxViolations  = 15   // vi phạm tới mức này thì bị loại (tính bét)
)

// BetOptions là các mức cược mỗi người chủ phòng được chọn.
var BetOptions = []int{100, 500, 1_000, 5_000, 10_000}

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

// Bank là ví xu và kinh nghiệm dùng chung.
type Bank interface {
	Balance(ctx context.Context, userID int64) (int64, error)
	Settle(ctx context.Context, game, note string, deltas map[int64]int64) (map[int64]int64, error)
	AwardXP(ctx context.Context, xp map[int64]int64) (map[int64]int, error)
}

// Loadouts trả xe đang dùng của người chơi (Garage).
type Loadouts interface {
	Loadout(ctx context.Context, uid int64) (CarView, error)
}

// Conn là kết nối tới một người. Send không được chặn vì được gọi khi đang giữ khoá phòng.
type Conn interface {
	Send(msg any)
}

type Options struct {
	Countdown    time.Duration // đếm ngược trước khi xuất phát
	FinishWindow time.Duration // sau khi người nhất về, chờ người khác tối đa chừng này
	MaxRace      time.Duration // cuộc đua không ai về đích quá lâu thì chốt theo tiến độ
	Grace        time.Duration // mất kết nối quá chừng này giữa cuộc đua thì tính bét
	Tick         time.Duration // nhịp phát vị trí và chạy máy đua; 0 = không tự chạy (test gọi step)
	MinHP        int           // máu tối thiểu để vào phòng (0 = không kiểm tra)
	Now          func() time.Time
	// OnEvent (tuỳ chọn) nhận sự kiện cho nhiệm vụ: "race_finish" (người thật đua xong, không bỏ cuộc), "race_win" (về nhất).
	// tx = nil: bên nhận tự mở giao dịch. Gọi trong goroutine riêng để không giữ khoá phòng.
	OnEvent func(ctx context.Context, tx *wallet.Tx, uid int64, kind string, n int)
}

var DefaultOptions = Options{Countdown: 3 * time.Second, FinishWindow: 90 * time.Second, MaxRace: 10 * time.Minute,
	Grace: 15 * time.Second, Tick: 66 * time.Millisecond}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o Options) gate(u User) error {
	if o.MinHP <= 0 {
		return nil
	}
	if u.HospitalUntil > 0 {
		return errors.New("bạn đang nằm viện")
	}
	if u.HP < o.MinHP {
		return fmt.Errorf("cần ít nhất %d HP để vào Trường đua", o.MinHP)
	}
	return nil
}

// flexBool nhận cả true/false lẫn số (khác 0 là true), để giao diện gửi kiểu nào cũng được.
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch x := v.(type) {
	case bool:
		*f = flexBool(x)
	case float64:
		*f = x != 0
	default:
		*f = false
	}
	return nil
}

// Message là tin nhắn từ client.
type Message struct {
	T     string   `json:"t"`
	Token string   `json:"token,omitempty"`
	Code  string   `json:"code,omitempty"`
	Bet   *int     `json:"bet,omitempty"`
	Track string   `json:"track,omitempty"`
	Mode  string   `json:"mode,omitempty"`
	Box   int      `json:"box"`
	Seat  int      `json:"seat"`
	Text  string   `json:"text,omitempty"`
	X     float64  `json:"x"`
	Z     float64  `json:"z"`
	Rot   float64  `json:"rot"`
	Speed float64  `json:"speed"`
	Nitro flexBool `json:"nitro"`
	Drift flexBool `json:"drift"`
	// Ms: lúc máy người chơi lấy mẫu vị trí, tính từ lúc xuất phát theo đồng hồ của máy đó (ms). Dùng làm mốc thời gian
	// của vị trí cho các máy khác nội suy — không dính độ trễ chập chờn của mạng như lúc server nhận gói.
	Ms *int64 `json:"ms,omitempty"`
	// Steer / Throttle: điều khiển đang dùng (−1..1) để máy khác đoán tiếp bằng vật lý xe khi gói tới trễ.
	Steer    *float64 `json:"steer,omitempty"`
	Throttle *float64 `json:"throttle,omitempty"`
}

// Player là người ngồi ghế hoặc người xem.
type Player struct {
	ID     string
	UserID int64 // 0 với máy
	Name   string
	Gender string
	Outfit string
	Hair   string
	Wings  string
	Face   string
	Level  int
	Bot    bool
	Online bool
	Coins  int64
	Car    CarView

	sampleMs  int64 // mốc thời gian (đồng hồ server) của vị trí cuối, theo Ms máy người chơi gửi; 0 = chưa có
	sampleOff int64 // độ lệch nhỏ nhất lúc nhận − (xuất phát + Ms) trong cuộc đua này
	offOK     bool
	steer     float64 // điều khiển cuối (người: theo gói; máy: suy từ tốc độ quay)
	throttle  float64
	hasInput  bool
	conn      Conn
	left      bool // đã rời giữa cuộc đua: giữ ghế tới khi chốt kết quả
	graceTime *time.Timer
	chatLim   chat.Limiter

	// Trong cuộc đua.
	prog       progress
	rot, speed float64
	nitro      bool
	drift      bool
	dnf        bool // bỏ cuộc / bị loại: tính bét
	violations int
	place      int // 1.. khi đã về đích, 0 = chưa
	botSpeed   float64
	botS       float64 // máy: quãng đường đã đi trên đường mẫu
	botV       float64 // máy: vận tốc hiện tại

	// Đạo cụ (chế độ "items").
	items       []string
	shieldUntil int64 // ms
	boostUntil  int64 // ms
	spinUntil   int64 // ms: đang xoay vì bị trúng
	hitAtMs     int64 // lần bị trúng gần nhất
	botUseAt    int64 // máy: lúc dùng món kế tiếp
	used        int   // số món đã dùng trong cuộc đua (để thống kê / test)
}

// CarState là vị trí một xe trong tin "cars".
type CarState struct {
	Seat  int     `json:"seat"`
	At    int64   `json:"at"` // lúc server nhận / tính vị trí này (ms): giao diện nội suy theo mốc này, bỏ gói trùng
	X     float64 `json:"x"`
	Z     float64 `json:"z"`
	Rot   float64 `json:"rot"`
	Speed float64 `json:"speed"`
	Lap   int     `json:"lap"`
	CP    int     `json:"cp"`
	Nitro bool    `json:"nitro"`
	Drift bool    `json:"drift"`
	// Chế độ đạo cụ.
	Shield bool `json:"shield,omitempty"`
	Boost  bool `json:"boost,omitempty"`
	Spin   bool `json:"spin,omitempty"`
	// Điều khiển cuối (−1..1) để giao diện ngoại suy bằng vật lý xe; không có (máy khách cũ) thì giao diện đoán tuyến tính.
	Steer    *float64 `json:"steer,omitempty"`
	Throttle *float64 `json:"throttle,omitempty"`
}

// Place là một người trong bảng kết quả.
type Place struct {
	Seat   int   `json:"seat"`
	TimeMs int64 `json:"timeMs"` // 0 nếu không về đích
	DNF    bool  `json:"dnf,omitempty"`
}

// Result là kết quả cuộc đua; Money / Tax / XP theo ghế.
type Result struct {
	T      string  `json:"t"`
	Places []Place `json:"places"`
	Money  []int64 `json:"money"`
	Tax    []int64 `json:"tax"`
	XP     []int64 `json:"xp"`
}

type Room struct {
	Code string

	mu         sync.Mutex
	hub        *Hub
	opts       Options
	hostID     string
	players    []*Player
	spectators []*Player
	phase      string
	bet        int
	gameBet    int
	track      *Track
	mode       string // ModeSpeed hoặc ModeItems
	boxes      []*box
	fx         []*fx
	fxSeq      int
	fxAtMs     int64     // đạn / dầu đã mô phỏng tới lúc này
	startAt    time.Time // lúc xuất phát (hết đếm ngược)
	firstAt    time.Time // lúc người nhất về đích
	finishes   int
	result     *Result
	botSeq     int
	closed     bool
	rng        *mrand.Rand
	chatLog    chat.Log
	ticker     *time.Ticker
	stopTick   chan struct{}
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

func (r *Room) nowMs() int64 { return r.opts.now().UnixMilli() }

func (r *Room) racing() bool { return r.phase == PhaseCountdown || r.phase == PhaseRacing }

func newPlayer(u User, conn Conn) *Player {
	return &Player{ID: randomID(), UserID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair,
		Wings: u.Wings, Face: u.Face, Level: u.Level, Online: true, conn: conn}
}

func (p *Player) update(u User) {
	p.Name, p.Gender, p.Outfit, p.Hair, p.Wings, p.Face, p.Level = u.Name, u.Gender, u.Outfit, u.Hair, u.Wings, u.Face, u.Level
}

// refresh đọc lại số dư và xe đang dùng của người thật.
func (r *Room) refresh(p *Player) error {
	if p.Bot {
		return nil
	}
	ctx := context.Background()
	bal, err := r.hub.bank.Balance(ctx, p.UserID)
	if err != nil {
		return err
	}
	p.Coins = bal
	if r.hub.loadouts != nil {
		car, err := r.hub.loadouts.Loadout(ctx, p.UserID)
		if err != nil {
			return err
		}
		p.Car = car
	}
	return nil
}

// join: trả lại ghế / chỗ xem cũ; còn ghế trống (không đang đua) thì ngồi, không thì xem.
func (r *Room) join(u User, conn Conn) (*Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("phòng đã đóng")
	}
	for _, p := range append(slices.Clone(r.players), r.spectators...) {
		if p.UserID == u.ID && !p.Bot {
			p.update(u)
			r.resume(p, conn)
			return p, nil
		}
	}
	if !r.racing() && len(r.players) < MaxPlayers {
		if err := r.opts.gate(u); err != nil {
			return nil, err
		}
		p := newPlayer(u, conn)
		if err := r.refresh(p); err != nil {
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
	if !p.dnf {
		p.left = false
	}
	if h := r.host(); r.seatOf(p) >= 0 && (h == nil || !h.Online) {
		r.hostID = p.ID
	}
	r.broadcast()
}

// seatAdded: có người / máy mới ngồi sau khi đua xong thì quay về phòng chờ.
func (r *Room) seatAdded() {
	if r.phase == PhaseOver {
		r.phase = PhaseLobby
		r.result = nil
	}
	r.broadcast()
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
	} else if i := r.seatOf(p); i >= 0 {
		r.players = slices.Delete(slices.Clone(r.players), i, i+1)
		if r.phase == PhaseOver {
			r.phase = PhaseLobby
			r.result = nil
		}
		if r.hostID == p.ID {
			r.pickHost()
		}
	} else {
		return
	}
	if r.empty() {
		r.close()
		return
	}
	r.broadcast()
}

// empty: không còn người thật nào có mặt (ghế hay xem) thì đóng phòng.
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
		if !p.left && p.Online {
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
	r.stopTicker()
	for _, p := range append(slices.Clone(r.players), r.spectators...) {
		if p.graceTime != nil {
			p.graceTime.Stop()
		}
	}
	r.hub.delete(r.Code)
}

// Handle xử lý một hành động của người đã ở trong phòng.
func (r *Room) Handle(p *Player, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	seated := r.seatOf(p) >= 0
	if !seated && !slices.Contains(r.spectators, p) {
		return errors.New("bạn không còn ở trong phòng")
	}
	switch m.T {
	case "leave":
		r.leave(p)
		return nil
	case "chat":
		return r.chat(p, m.Text)
	}
	if !seated {
		return nil // người xem chỉ được chat / rời
	}
	switch m.T {
	case "state":
		r.carState(p, m)
		return nil
	case "pickup":
		return r.pickup(p, m.Box)
	case "use":
		return r.use(p)
	}
	if p.ID != r.hostID {
		return errors.New("chỉ chủ phòng được làm việc này")
	}
	if r.racing() {
		return errors.New("đang đua")
	}
	switch m.T {
	case "addBot":
		if len(r.players) >= MaxPlayers {
			return errors.New("phòng đã đủ 6 xe")
		}
		r.players = append(r.players, r.newBot())
		r.seatAdded()
	case "kick":
		if m.Seat < 0 || m.Seat >= len(r.players) || r.players[m.Seat] == p {
			return errors.New("ghế không hợp lệ")
		}
		r.remove(r.players[m.Seat], true)
	case "setBet":
		if m.Bet == nil || !slices.Contains(BetOptions, *m.Bet) {
			return errors.New("mức cược không hợp lệ")
		}
		r.bet = *m.Bet
		r.broadcast()
	case "setTrack":
		t := r.hub.tracks[m.Track]
		if t == nil {
			return errors.New("không có đường đua này")
		}
		r.track = t
		r.broadcast()
	case "setMode":
		if !validMode(m.Mode) {
			return errors.New("chế độ không hợp lệ")
		}
		r.mode = m.Mode
		r.broadcast()
	case "start":
		return r.start()
	}
	return nil
}

func (r *Room) chat(p *Player, raw string) error {
	text := chat.Prepare(raw)
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

// botNames là tên máy đua.
var botNames = []string{"Máy Tốc Độ", "Máy Sấm Sét", "Máy Gió Lốc", "Máy Bão Táp", "Máy Tia Lửa", "Máy Cuồng Phong"}

// newBot tạo máy với một xe ngẫu nhiên (cấp thấp, sơn ngẫu nhiên).
func (r *Room) newBot() *Player {
	r.botSeq++
	m := Cars[r.rng.IntN(3)]
	lv := Levels{r.rng.IntN(3), r.rng.IntN(3), r.rng.IntN(3)}
	paints := []string{}
	for _, it := range Items {
		if it.Kind == "paint" {
			paints = append(paints, it.ID)
		}
	}
	car := carView(m, ownedCar{levels: lv, cosmetics: Cosmetics{Paint: paints[r.rng.IntN(len(paints))]}}, true)
	return &Player{ID: randomID(), Name: botNames[(r.botSeq-1)%len(botNames)], Bot: true, Online: true, Car: car}
}

// start kiểm đủ người, đủ xu rồi bắt đầu đếm ngược.
func (r *Room) start() error {
	for _, p := range slices.Clone(r.players) {
		if p.left {
			r.players = slices.DeleteFunc(slices.Clone(r.players), func(q *Player) bool { return q == p }) // slice mới: không ghi nil vào mảng người gọi còn đang duyệt
		}
	}
	if r.track == nil {
		return errors.New("chưa có đường đua")
	}
	if len(r.players) < MinPlayers {
		r.broadcast()
		return errors.New("cần ít nhất 2 xe, hãy mời bạn hoặc thêm máy")
	}
	for _, p := range r.humans() {
		if err := r.refresh(p); err != nil {
			return err
		}
		if p.Coins < int64(r.bet) {
			r.broadcast()
			return fmt.Errorf("%s không đủ %d xu để vào cuộc đua", p.Name, r.bet)
		}
	}
	now := r.opts.now()
	r.startAt = now.Add(r.opts.Countdown)
	r.firstAt = time.Time{}
	r.finishes = 0
	r.result = nil
	r.gameBet = r.bet
	startMs := r.startAt.UnixMilli()
	for i, p := range r.players {
		sp := r.track.spawnOf(i)
		p.prog = progress{pos: Point{sp.X, sp.Z}, atMs: startMs, lapStartMs: startMs}
		p.rot, p.speed, p.nitro, p.drift = sp.Rot, 0, false, false
		p.dnf, p.violations, p.place, p.sampleMs, p.sampleOff, p.offOK = false, 0, 0, 0, 0, false
		p.steer, p.throttle, p.hasInput = 0, 0, false
		if p.Bot {
			// Máy chạy theo đường mẫu từ điểm gần vị trí xuất phát nhất, tốc độ 85–97% trần xe.
			p.botS = r.track.nearestS(p.prog.pos)
			p.botV = 0
			p.botSpeed = p.Car.Stats.MaxSpeed * (0.85 + 0.12*r.rng.Float64())
		}
	}
	r.resetItems()
	r.phase = PhaseCountdown
	r.startTicker()
	r.broadcast()
	return nil
}

// carState nhận vị trí người chơi gửi lên và kiểm gói. Gói sai bị bỏ; sai nhiều lần thì bị loại.
func (r *Room) carState(p *Player, m Message) {
	if p.Bot || p.dnf || p.prog.finished || !r.racing() {
		return
	}
	now := r.nowMs()
	if now < r.startAt.UnixMilli() {
		return // chưa xuất phát: bỏ qua, không tính vi phạm
	}
	r.phase = PhaseRacing
	maxV := p.Car.Stats.MaxSpeed * p.itemSpeedCap(now)
	gap := max(now-p.prog.atMs, minPacketGapMs)
	dist := math.Hypot(m.X-p.prog.pos.X, m.Z-p.prog.pos.Z)
	slack := moveSlack
	if p.hitAtMs > 0 && now-p.hitAtMs < hitGraceMs {
		slack += hitSlack // vừa bị trúng: xe xoay, vị trí client và server dễ lệch
	}
	bad := math.IsNaN(m.X) || math.IsNaN(m.Z) || math.IsInf(m.X, 0) || math.IsInf(m.Z, 0) ||
		m.Speed > maxV*speedTolerance || dist > maxV*float64(gap)/1000*moveTolerance+slack
	if bad {
		r.violation(p)
		return
	}
	p.rot, p.speed, p.nitro, p.drift = m.Rot, m.Speed, bool(m.Nitro), bool(m.Drift)
	p.sampleAt(r.startAt.UnixMilli(), m.Ms, now)
	if m.Steer != nil && m.Throttle != nil {
		p.steer, p.throttle, p.hasInput = unit(*m.Steer), unit(*m.Throttle), true
	}
	switch p.prog.move(r.track, Point{m.X, m.Z}, now, r.startAt.UnixMilli()) {
	case advFastLap:
		r.violation(p)
	case advFinished:
		r.finished(p)
	}
}

// sampleAt trả mốc thời gian (đồng hồ server) của vị trí vừa nhận, theo Ms (lúc máy người chơi lấy mẫu, tính từ xuất phát
// theo đồng hồ máy đó). Đồng hồ hai bên lệch một hằng số trong cả cuộc đua (máy đó nhận tin "room" muộn bao nhiêu), nên
// chuỗi Ms đều như nhịp gửi thật; cộng thêm độ lệch nhỏ nhất đã thấy (gói tới nhanh nhất) là ra lúc lấy mẫu theo đồng
// hồ server, không dính độ trễ chập chờn của từng gói. Không có / vô lý (lệch quá sampleAhead / sampleBehind) thì dùng
// lúc nhận; luôn tăng dần.
func (p *Player) sampleAt(startMs int64, ms *int64, recvMs int64) int64 {
	at := recvMs
	if ms != nil {
		est := startMs + *ms
		if d := recvMs - est; d >= -sampleAhead && d <= sampleBehind {
			switch {
			case !p.offOK:
				p.sampleOff, p.offOK = d, true
			case d < p.sampleOff:
				// Gói tới nhanh hơn: hạ độ lệch dần (≤ sampleSlew mỗi gói) để mốc không nhảy làm xe vụt lên ở máy khác.
				p.sampleOff -= min(p.sampleOff-d, sampleSlew)
			}
			// Không kẹp theo lúc nhận: gói tới nhanh hơn độ lệch đang dùng vẫn giữ đúng nhịp lấy mẫu (kẹp thì mốc của riêng
			// gói đó bị sớm hơn các gói khác → xe chậm nửa nhịp rồi vụt lên ở máy khác).
			at = est + p.sampleOff
		}
	}
	if at <= p.sampleMs {
		at = p.sampleMs + 1
	}
	p.sampleMs = at
	return at
}

// unit kẹp v vào −1..1 (NaN → 0).
func unit(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(-1, math.Min(1, v))
}

func (r *Room) violation(p *Player) {
	p.violations++
	if p.violations >= maxViolations {
		p.dnf = true
		if p.conn != nil {
			p.conn.Send(map[string]string{"t": "error", "msg": "bạn bị loại vì dữ liệu xe không hợp lệ"})
		}
		r.broadcast()
		r.checkEnd()
	}
}

// finished: một xe vừa về đích.
func (r *Room) finished(p *Player) {
	r.finishes++
	p.place = r.finishes
	if r.firstAt.IsZero() {
		r.firstAt = r.opts.now()
	}
	r.sendAll(map[string]any{"t": "finish", "seat": r.seatOf(p), "place": p.place, "timeMs": p.prog.finishMs})
	r.checkEnd()
}

// checkEnd chốt cuộc đua khi mọi xe đã về / bỏ cuộc, hoặc hết giờ chờ.
func (r *Room) checkEnd() {
	if !r.racing() {
		return
	}
	now := r.opts.now()
	running := 0
	for _, p := range r.players {
		if !p.prog.finished && !p.dnf {
			running++
		}
	}
	timeout := (!r.firstAt.IsZero() && now.Sub(r.firstAt) >= r.opts.FinishWindow) ||
		(r.opts.MaxRace > 0 && now.Sub(r.startAt) >= r.opts.MaxRace)
	humansIn := false
	for _, p := range r.humans() {
		if !p.prog.finished && !p.dnf {
			humansIn = true
		}
	}
	// Không còn người thật nào đang đua và đã có người về đích: máy không cần chạy nốt.
	if running == 0 || timeout || (!humansIn && r.finishes > 0) {
		r.end()
	}
}

// end xếp hạng, trả xu (nhất ăn cược của mọi người, thuế 5%), cộng XP.
func (r *Room) end() {
	r.stopTicker()
	r.phase = PhaseOver
	t := r.track
	order := slices.Clone(r.players)
	sort.SliceStable(order, func(i, j int) bool {
		a, b := order[i], order[j]
		switch {
		case a.dnf != b.dnf:
			return !a.dnf
		case a.prog.finished != b.prog.finished:
			return a.prog.finished
		case a.prog.finished:
			return a.place < b.place
		}
		return a.prog.score(t) > b.prog.score(t)
	})
	n := len(r.players)
	res := &Result{T: "result", Money: make([]int64, n), Tax: make([]int64, n), XP: make([]int64, n)}
	deltas, xp := map[int64]int64{}, map[int64]int64{}
	for place, p := range order {
		seat := r.seatOf(p)
		pl := Place{Seat: seat, DNF: p.dnf}
		if p.prog.finished && !p.dnf {
			pl.TimeMs = p.prog.finishMs
		}
		res.Places = append(res.Places, pl)
		d := -int64(r.gameBet)
		if place == 0 {
			d = int64(r.gameBet) * int64(n-1)
		}
		res.Money[seat] = d
		if p.Bot {
			continue
		}
		if ev := r.hub.opts.OnEvent; ev != nil && !p.dnf {
			uid, won := p.UserID, place == 0
			go func() {
				ev(context.Background(), nil, uid, "race_finish", 1)
				if won {
					ev(context.Background(), nil, uid, "race_win", 1)
				}
			}()
		}
		gain := int64(minXP + bonusXP*(n-1-place)/max(1, n-1))
		res.XP[seat] = gain
		xp[p.UserID] = gain
		if d != 0 {
			deltas[p.UserID] = d
		}
	}
	ctx := context.Background()
	if len(deltas) > 0 {
		note := fmt.Sprintf("Trường đua phòng %s", r.Code)
		balances, taxes, err := wallet.SettleTaxed(ctx, r.hub.bank, GameID, note, deltas)
		if err != nil {
			log.Printf("race: ghi kết quả %s: %v", note, err)
		}
		for i, p := range r.players {
			if p.Bot {
				continue
			}
			if bal, ok := balances[p.UserID]; ok {
				p.Coins = bal
			}
			if tx := taxes[p.UserID]; tx > 0 {
				res.Money[i] -= tx
				res.Tax[i] = tx
			}
		}
	}
	if len(xp) > 0 {
		levels, err := r.hub.bank.AwardXP(ctx, xp)
		if err != nil {
			log.Printf("race: cộng XP phòng %s: %v", r.Code, err)
		}
		for _, p := range r.players {
			if lv, ok := levels[p.UserID]; ok && !p.Bot {
				p.Level = lv
			}
		}
	}
	r.result = res
	r.sendAll(res)
	// Người đã rời giữa cuộc đua thì bỏ ghế sau khi chốt.
	for _, p := range slices.Clone(r.players) {
		if p.left {
			r.players = slices.DeleteFunc(slices.Clone(r.players), func(q *Player) bool { return q == p }) // slice mới: không ghi nil vào mảng người gọi còn đang duyệt
			if r.hostID == p.ID {
				r.pickHost()
			}
		}
	}
	if r.empty() {
		r.close()
		return
	}
	r.broadcast()
}

// leave: người xem rời thì thôi; người đua rời giữa cuộc đua thì tính bét (vẫn mất cược).
func (r *Room) leave(p *Player) {
	if r.racing() && r.seatOf(p) >= 0 {
		if p.conn != nil {
			p.conn.Send(map[string]string{"t": "left"})
		}
		if p.graceTime != nil {
			p.graceTime.Stop()
			p.graceTime = nil
		}
		p.conn, p.Online, p.left, p.dnf = nil, false, true, true
		if r.hostID == p.ID {
			r.pickHost()
		}
		r.broadcast()
		r.checkEnd()
		return
	}
	r.remove(p, true)
}

// Disconnect: mất kết nối. Quá Grace mà chưa vào lại: đang đua thì tính bét, ngoài cuộc đua thì bỏ ghế.
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
		r.graceOver(p)
	})
	r.broadcast()
}

func (r *Room) graceOver(p *Player) {
	if p.Online || r.closed {
		return
	}
	if r.racing() && r.seatOf(p) >= 0 {
		if !p.prog.finished {
			p.dnf = true
		}
		p.left = true
		if r.hostID == p.ID {
			r.pickHost()
		}
		r.broadcast()
		r.checkEnd()
		return
	}
	r.remove(p, false)
}

// ---------- Nhịp đua: máy chạy, phát vị trí, kiểm hết giờ ----------

func (r *Room) startTicker() {
	if r.opts.Tick <= 0 || r.ticker != nil {
		return
	}
	r.ticker = time.NewTicker(r.opts.Tick)
	r.stopTick = make(chan struct{})
	t, stop := r.ticker, r.stopTick
	go func() {
		for {
			select {
			case <-t.C:
				r.mu.Lock()
				if !r.closed && r.racing() {
					r.step()
				}
				r.mu.Unlock()
			case <-stop:
				return
			}
		}
	}()
}

func (r *Room) stopTicker() {
	if r.ticker != nil {
		r.ticker.Stop()
		close(r.stopTick)
		r.ticker, r.stopTick = nil, nil
	}
}

// step là một nhịp: chuyển sang "racing" khi hết đếm ngược, máy chạy, phát vị trí, kiểm hết giờ.
func (r *Room) step() {
	now := r.opts.now()
	nowMs := now.UnixMilli()
	if r.phase == PhaseCountdown && !now.Before(r.startAt) {
		r.phase = PhaseRacing
		r.broadcast()
	}
	if r.phase == PhaseRacing {
		// Duyệt bản sao: máy về đích có thể chốt cuộc đua ngay trong vòng này (end bỏ ghế người đã rời);
		// chốt rồi thì thôi chạy các máy còn lại.
		for _, p := range slices.Clone(r.players) {
			if !r.racing() {
				break
			}
			if p.Bot && !p.prog.finished {
				r.botStep(p, nowMs)
				if r.racing() {
					r.botItems(p, nowMs)
				}
			}
		}
		if r.racing() && r.mode == ModeItems {
			r.stepFx(nowMs)
		}
	}
	if !r.racing() {
		return // máy về đích có thể đã chốt cuộc đua
	}
	list := make([]CarState, 0, len(r.players))
	for i, p := range r.players {
		at := p.prog.atMs
		if !p.Bot && p.sampleMs > 0 {
			at = p.sampleMs
		}
		var steer, throttle *float64
		if p.hasInput {
			sv, tv := p.steer, p.throttle
			steer, throttle = &sv, &tv
		}
		list = append(list, CarState{Seat: i, At: at, Steer: steer, Throttle: throttle, X: p.prog.pos.X, Z: p.prog.pos.Z, Rot: p.rot, Speed: p.speed,
			Lap: p.prog.lap, CP: p.prog.cp, Nitro: p.nitro, Drift: p.drift,
			Shield: nowMs < p.shieldUntil, Boost: nowMs < p.boostUntil, Spin: nowMs < p.spinUntil})
	}
	r.sendAll(map[string]any{"t": "cars", "ms": nowMs, "list": list})
	r.checkEnd()
}

// botStep cho máy chạy theo đường mẫu: tăng tốc tới tốc độ riêng, đi quãng đường tương ứng thời gian trôi qua.
func (r *Room) botStep(p *Player, nowMs int64) {
	startMs := r.startAt.UnixMilli()
	from := max(p.prog.atMs, startMs)
	dt := float64(nowMs-from) / 1000
	if dt <= 0 {
		return
	}
	rot0, total := p.rot, dt
	// Điều khiển tương đương cho giao diện ngoại suy: ga hết cỡ, lái theo tốc độ quay vừa rồi (cùng công thức physics.js).
	defer func() {
		sp := math.Abs(p.speed)
		f := 2.3 * math.Min(1, sp/7) * (1 - 0.35*math.Min(1, sp/p.Car.Stats.MaxSpeed))
		dr := math.Remainder(p.rot-rot0, 2*math.Pi)
		p.throttle, p.steer, p.hasInput = 1, 0, true
		if f > 0 && total > 0 {
			p.steer = unit(dr / total / f)
		}
	}()
	// Chia nhỏ bước để máy không nhảy qua điểm kiểm tra khi nhịp bị trễ.
	for dt > 0 {
		h := math.Min(dt, 0.1)
		dt -= h
		top := p.botSpeed
		switch now := from + int64(h*1000); {
		case now < p.spinUntil:
			top = p.botSpeed * spinSpeed // bị trúng: xoay, chạy chậm
		case now < p.boostUntil:
			top = p.botSpeed * boostMul
		}
		if p.botV > top {
			p.botV = top
		} else {
			p.botV = math.Min(top, p.botV+p.Car.Stats.Accel*h)
		}
		p.botS += p.botV * h
		pt, rot := r.track.at(p.botS)
		from += int64(h * 1000)
		p.rot, p.speed = rot, p.botV
		if p.prog.move(r.track, pt, from, startMs) == advFinished {
			r.finished(p)
			return
		}
	}
	p.prog.atMs = nowMs
}

// nearestS là quãng đường trên đường mẫu của điểm gần p nhất.
func (t *Track) nearestS(p Point) float64 {
	best, bestD := 0.0, math.Inf(1)
	for i := range t.Path {
		a, b := t.Path[i], t.Path[(i+1)%len(t.Path)]
		dx, dz := b.X-a.X, b.Z-a.Z
		l2 := dx*dx + dz*dz
		f := 0.0
		if l2 > 0 {
			f = math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Z-a.Z)*dz)/l2))
		}
		px, pz := a.X+dx*f, a.Z+dz*f
		if d := math.Hypot(p.X-px, p.Z-pz); d < bestD {
			best, bestD = t.cum[i]+math.Sqrt(l2)*f, d
		}
	}
	return best
}

// ---------- Gửi trạng thái ----------

func (r *Room) sendAll(msg any) {
	for _, p := range append(slices.Clone(r.players), r.spectators...) {
		if p.conn != nil {
			p.conn.Send(msg)
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

// SeatView là một ghế trong tin "room".
type SeatView struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Gender   string  `json:"gender,omitempty"`
	Outfit   string  `json:"outfit,omitempty"`
	Hair     string  `json:"hair,omitempty"`
	Wings    string  `json:"wings,omitempty"`
	Face     string  `json:"face,omitempty"`
	Level    int     `json:"level"`
	Bot      bool    `json:"bot"`
	Online   bool    `json:"online"`
	Host     bool    `json:"host"`
	Coins    int64   `json:"coins"`
	Car      CarView `json:"car"`
	Finished bool    `json:"finished"`
	Place    int     `json:"place"`
	TimeMs   int64   `json:"timeMs"`
	DNF      bool    `json:"dnf"`
}

// State là tin "room": phòng, ghế, xe của mỗi ghế, giai đoạn.
type State struct {
	T           string      `json:"t"`
	Code        string      `json:"code"`
	Phase       string      `json:"phase"`
	You         string      `json:"you,omitempty"`
	Seat        int         `json:"seat"` // ghế của người xem tin này, -1 nếu đang xem
	Seats       []SeatView  `json:"seats"`
	Bet         int         `json:"bet"`
	GameBet     int         `json:"gameBet"`
	BetOptions  []int       `json:"betOptions"`
	Track       string      `json:"track"`
	Tracks      []TrackInfo `json:"tracks"`
	Laps        int         `json:"laps"`
	Mode        string      `json:"mode"`        // "speed" hoặc "items"
	Boxes       []BoxView   `json:"boxes"`       // hộp đạo cụ (chế độ items, trong cuộc đua)
	NitroCharge float64     `json:"nitroCharge"` // hệ số tốc độ tích nitro
	StartAt     int64       `json:"startAt"`     // ms
	Now         int64       `json:"now"`         // ms, để giao diện bù lệch đồng hồ
	Spectators  int         `json:"spectators"`
	Result      *Result     `json:"result,omitempty"`
	Chat        []chat.Line `json:"chat"`
}

func (r *Room) stateFor(viewer *Player) State {
	s := State{T: "room", Code: r.Code, Phase: r.phase, Seat: r.seatOf(viewer), Seats: []SeatView{}, Bet: r.bet,
		GameBet: r.gameBet, BetOptions: BetOptions, Tracks: r.hub.trackList(), Now: r.nowMs(),
		Spectators: len(r.spectators), Result: r.result, Chat: r.chatLog.List(),
		Mode: r.mode, Boxes: r.boxViews(), NitroCharge: NitroCharge}
	if s.Seat >= 0 {
		s.You = viewer.ID
	}
	if r.track != nil {
		s.Track, s.Laps = r.track.ID, r.track.Laps
	}
	if !r.startAt.IsZero() {
		s.StartAt = r.startAt.UnixMilli()
	}
	for _, p := range r.players {
		s.Seats = append(s.Seats, SeatView{ID: p.ID, Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Hair: p.Hair,
			Wings: p.Wings, Face: p.Face, Level: p.Level, Bot: p.Bot, Online: p.Online, Host: p.ID == r.hostID,
			Coins: p.Coins, Car: p.Car, Finished: p.prog.finished, Place: p.place, TimeMs: p.prog.finishMs, DNF: p.dnf})
	}
	return s
}
