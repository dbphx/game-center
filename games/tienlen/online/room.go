// Package online chạy Tiến Lên nhiều người qua WebSocket: phòng, người vào/ra, máy, lượt đánh,
// hẹn giờ và trả thưởng vào ví chung của hub.
//
// Mọi thao tác trên một Room đều chạy dưới Room.mu. Thứ tự khoá là Room.mu → Hub.mu,
// Hub không bao giờ giữ Hub.mu khi gọi vào Room.
package online

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dbphx/game-center/games/tienlen"
)

const (
	GameID     = "tienlen"
	MinPlayers = 2
	MaxPlayers = 4
	DefaultBet = 10
)

// BetOptions là các mức cược (xu mỗi mức) chủ phòng được chọn. Mức 0 là bàn chơi vui: không thắng thua xu, không cộng XP.
var BetOptions = []int{0, 10, 20, 50, 100, 200, 500}

const (
	PhaseLobby   = "lobby"
	PhasePlaying = "playing"
	PhaseOver    = "over"
)

// User là tài khoản hub đang chơi, kèm nhân vật để vẽ avatar.
type User struct {
	ID            int64
	Name          string
	Gender        string
	Outfit        string
	Hair          string // kiểu tóc, "default" là tóc mặc định
	Wings         string // cánh, rỗng là không cánh
	Face          string // gương mặt, "default" là mặt mặc định
	Level         int
	HP            int   // máu hiện tại
	HospitalUntil int64 // > 0 khi đang nằm viện
}

// Bank là ví xu và kinh nghiệm dùng chung của hub.
type Bank interface {
	Balance(ctx context.Context, userID int64) (int64, error)
	Settle(ctx context.Context, game, note string, deltas map[int64]int64) (map[int64]int64, error)
	AwardXP(ctx context.Context, xp map[int64]int64) (map[int64]int, error)
}

// XP sau mỗi ván: người về nhất được nhiều nhất, người bét vẫn có phần.
const (
	minXP   = 10
	bonusXP = 20
)

// Conn là kết nối tới một người chơi. Send không được chặn vì được gọi khi đang giữ khoá phòng.
type Conn interface {
	Send(msg any)
}

type Options struct {
	BotDelay    time.Duration // thời gian máy "suy nghĩ"
	TurnTimeout time.Duration // hết giờ thì máy đánh thay
	Grace       time.Duration // chờ người mất kết nối quay lại trước khi xoá khỏi phòng
	MinHP       int           // máu tối thiểu để tạo / vào bàn mới (0 = không kiểm tra)
}

var DefaultOptions = Options{BotDelay: 900 * time.Millisecond, TurnTimeout: 30 * time.Second, Grace: 30 * time.Second}

// gate kiểm tra người chơi đủ máu và không nằm viện để vào Casino.
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

type Player struct {
	ID     string // id công khai trong phòng
	UserID int64  // 0 với máy
	Name   string
	Gender string
	Outfit string
	Hair   string
	Wings  string
	Face   string
	Level  int
	Bot    bool
	Online bool
	Left   bool  // rời phòng giữa ván: máy đánh thay tới hết ván rồi xoá
	Coins  int64 // số dư ví (bản sao để hiển thị), máy không có ví

	conn      Conn
	graceTime *time.Timer
}

type Room struct {
	Code string

	mu       sync.Mutex
	hub      *Hub
	opts     Options
	hostID   string
	players  []*Player
	phase    string
	game     *tienlen.Game
	roster   string // id các ghế của game hiện tại; đổi người thì tạo game mới
	timer    *time.Timer
	timerGen int // tăng mỗi lần đặt hẹn giờ để bỏ qua hẹn giờ cũ
	deadline time.Time
	botSeq   int
	closed   bool
	bet      int             // xu mỗi mức cược, chủ phòng chỉnh giữa các ván
	gameBet  int             // mức cược của ván đang chơi / vừa xong
	gameXP   map[int64]int64 // XP từng người nhận được ở ván vừa xong
}

func randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Room) seatOf(p *Player) int { return slices.Index(r.players, p) }

func (r *Room) host() *Player { return r.findPlayer(r.hostID) }

func (r *Room) findPlayer(id string) *Player {
	for _, p := range r.players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (r *Room) humans() []*Player {
	var out []*Player
	for _, p := range r.players {
		if !p.Bot && !p.Left {
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

// join cho người dùng vào phòng, hoặc trả lại ghế cũ nếu họ đã ở trong phòng.
func (r *Room) join(u User, conn Conn) (*Player, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("phòng đã đóng")
	}
	for _, p := range r.players {
		if p.UserID == u.ID && !p.Left {
			p.Name, p.Gender, p.Outfit, p.Hair, p.Wings, p.Face, p.Level = u.Name, u.Gender, u.Outfit, u.Hair, u.Wings, u.Face, u.Level // có thể vừa đổi ở shop
			r.resume(p, conn)
			return p, nil
		}
	}
	if r.phase == PhasePlaying {
		return nil, errors.New("phòng đang chơi, đợi hết ván rồi vào lại")
	}
	if len(r.players) >= MaxPlayers {
		return nil, errors.New("phòng đã đủ 4 người")
	}
	// Chỉ kiểm tra máu khi ngồi ghế mới; người đang có ghế vào lại (mất mạng, tải lại trang) luôn được.
	if err := r.opts.gate(u); err != nil {
		return nil, err
	}
	p := &Player{ID: randomID(), UserID: u.ID, Name: u.Name, Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings, Face: u.Face, Level: u.Level, Online: true, conn: conn}
	if err := r.refreshCoins(p); err != nil {
		return nil, err
	}
	r.players = append(r.players, p)
	if r.host() == nil {
		r.hostID = p.ID
	}
	r.rosterChanged()
	return p, nil
}

func (r *Room) resume(p *Player, conn Conn) {
	if p.graceTime != nil {
		p.graceTime.Stop()
	}
	if p.conn != nil && p.conn != conn {
		p.conn.Send(map[string]string{"t": "replaced"})
	}
	p.conn, p.Online = conn, true
	if h := r.host(); h == nil || !h.Online {
		r.hostID = p.ID
	}
	r.scheduleIfTurn(p)
	r.broadcast()
}

// rosterChanged: sau khi đổi người, kết quả ván cũ không còn khớp ghế nên quay về phòng chờ.
func (r *Room) rosterChanged() {
	if r.phase == PhaseOver {
		r.phase = PhaseLobby
	}
	r.broadcast()
}

func (r *Room) remove(p *Player, notify bool) {
	i := r.seatOf(p)
	if i < 0 {
		return
	}
	r.players = slices.Delete(r.players, i, i+1)
	if p.graceTime != nil {
		p.graceTime.Stop()
	}
	if notify && p.conn != nil {
		p.conn.Send(map[string]string{"t": "left"})
	}
	if len(r.humans()) == 0 {
		r.close()
		return
	}
	if r.hostID == p.ID {
		r.pickHost()
	}
	r.rosterChanged()
}

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
	for _, p := range r.players {
		if p.graceTime != nil {
			p.graceTime.Stop()
		}
	}
	r.hub.delete(r.Code)
}

// Message là tin nhắn từ client.
type Message struct {
	T     string `json:"t"`
	Token string `json:"token,omitempty"` // token tài khoản hub, gửi kèm create / join
	Code  string `json:"code,omitempty"`
	Cards []int  `json:"cards,omitempty"`
	ID    string `json:"id,omitempty"`
	Bet   int    `json:"bet,omitempty"`
}

// Handle xử lý một hành động của người chơi đã ở trong phòng.
func (r *Room) Handle(p *Player, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seatOf(p) < 0 {
		return errors.New("bạn không còn ở trong phòng")
	}
	isHost := p.ID == r.hostID
	inLobby := r.phase != PhasePlaying

	switch m.T {
	case "addBot":
		switch {
		case !isHost:
			return errors.New("chỉ chủ phòng được thêm máy")
		case !inLobby:
			return errors.New("ván đang chơi")
		case len(r.players) >= MaxPlayers:
			return errors.New("phòng đã đủ 4 người")
		}
		r.botSeq++
		r.players = append(r.players, &Player{ID: randomID(), Name: fmt.Sprintf("Máy %d", r.botSeq), Bot: true, Online: true})
		r.rosterChanged()

	case "kick":
		target := r.findPlayer(m.ID)
		switch {
		case !isHost:
			return errors.New("chỉ chủ phòng được mời ra")
		case !inLobby:
			return errors.New("ván đang chơi")
		case target == nil || target == p:
			return errors.New("không mời ra được người này")
		}
		r.remove(target, true)

	case "setBet":
		switch {
		case !isHost:
			return errors.New("chỉ chủ phòng được đổi mức cược")
		case !inLobby:
			return errors.New("ván đang chơi")
		case !slices.Contains(BetOptions, m.Bet):
			return errors.New("mức cược không hợp lệ")
		}
		r.bet = m.Bet
		r.broadcast()

	case "start":
		if !isHost {
			return errors.New("chỉ chủ phòng được bắt đầu")
		}
		if !inLobby {
			return errors.New("ván đang chơi")
		}
		return r.startGame()

	case "play", "pass":
		if r.phase != PhasePlaying {
			return errors.New("chưa bắt đầu ván")
		}
		var err error
		if m.T == "play" {
			err = r.game.Play(r.seatOf(p), m.Cards)
		} else {
			err = r.game.Pass(r.seatOf(p))
		}
		if err != nil {
			return err
		}
		r.afterMove()

	case "hint":
		if r.phase != PhasePlaying || r.game.Turn != r.seatOf(p) {
			return errors.New("chưa tới lượt")
		}
		seat := r.seatOf(p)
		cards := tienlen.ChooseMove(r.game.Hands[seat], r.game.Context(seat))
		p.conn.Send(map[string]any{"t": "hint", "cards": cards})

	case "leave":
		r.leave(p)

	default:
		return fmt.Errorf("không hiểu lệnh %q", m.T)
	}
	return nil
}

func (r *Room) leave(p *Player) {
	if r.phase != PhasePlaying {
		r.remove(p, true)
		return
	}
	// Giữa ván: máy đánh thay tới hết ván, kết quả vẫn tính vào ví của người rời.
	p.Left, p.Online = true, false
	if p.conn != nil {
		p.conn.Send(map[string]string{"t": "left"})
		p.conn = nil
	}
	if len(r.humans()) == 0 {
		// Không còn ai xem: cho máy đánh nốt ngay để chốt xu rồi đóng phòng.
		for r.phase == PhasePlaying {
			r.autoMove()
		}
		r.close()
		return
	}
	if r.hostID == p.ID {
		r.pickHost()
	}
	r.scheduleIfTurn(p)
	r.broadcast()
}

// Disconnect được gọi khi mất kết nối websocket. Người chơi có Grace để vào lại.
func (r *Room) Disconnect(p *Player, conn Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seatOf(p) < 0 || p.conn != conn {
		return
	}
	p.conn, p.Online = nil, false
	p.graceTime = time.AfterFunc(r.opts.Grace, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if p.Online || r.seatOf(p) < 0 {
			return
		}
		if r.phase == PhasePlaying {
			// Đang chơi thì giữ ghế, máy tiếp tục đánh thay; chỉ đóng phòng nếu không còn ai.
			if !slices.ContainsFunc(r.humans(), func(h *Player) bool { return h.Online }) {
				for r.phase == PhasePlaying {
					r.autoMove()
				}
				r.close()
			}
			return
		}
		r.remove(p, false)
	})
	r.scheduleIfTurn(p)
	r.broadcast()
}

func (r *Room) startGame() error {
	r.players = slices.DeleteFunc(r.players, func(p *Player) bool { return p.Left })
	if len(r.players) < MinPlayers {
		r.broadcast()
		return errors.New("cần ít nhất 2 người, hãy thêm máy hoặc mời bạn bè")
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
	ids := make([]string, len(r.players))
	for i, p := range r.players {
		ids[i] = p.ID
	}
	roster := strings.Join(ids, ",")
	if r.game == nil || roster != r.roster {
		r.game, _ = tienlen.NewGame(len(r.players), mrand.New(mrand.NewPCG(mrand.Uint64(), mrand.Uint64())))
		r.roster = roster
	}
	r.game.Start()
	r.gameBet = r.bet
	r.gameXP = nil
	r.phase = PhasePlaying
	r.afterMove()
	return nil
}

func (r *Room) afterMove() {
	if r.game.Over {
		r.phase = PhaseOver
		r.stopTimer()
		r.settle()
	} else {
		r.schedule()
	}
	r.broadcast()
}

// settle ghi kết quả ván vào ví. Máy đóng vai nhà cái nên phần của máy không ghi vào đâu cả.
func (r *Room) settle() {
	if r.gameBet == 0 {
		r.awardXP() // bàn chơi vui: không thắng thua xu nhưng vẫn có kinh nghiệm
		return
	}
	deltas := map[int64]int64{}
	for i, units := range r.game.Money() {
		if p := r.players[i]; !p.Bot {
			deltas[p.UserID] += int64(units * r.gameBet)
		}
	}
	note := fmt.Sprintf("Tiến Lên phòng %s, ván %d", r.Code, r.game.GameNo)
	balances, err := r.hub.bank.Settle(context.Background(), GameID, note, deltas)
	if err != nil {
		log.Printf("tienlen: ghi kết quả %s: %v", note, err)
		return
	}
	for _, p := range r.players {
		if bal, ok := balances[p.UserID]; ok && !p.Bot {
			p.Coins = bal
		}
	}
	r.awardXP()
}

func (r *Room) awardXP() {
	n := len(r.players)
	xp := map[int64]int64{}
	for i, p := range r.players {
		if p.Bot {
			continue
		}
		place := slices.Index(r.game.Finished, i)
		if place < 0 { // tới trắng: người không thắng tính như về bét
			place = n - 1
		}
		xp[p.UserID] += int64(minXP + bonusXP*(n-1-place)/(n-1))
	}
	r.gameXP = xp
	levels, err := r.hub.bank.AwardXP(context.Background(), xp)
	if err != nil {
		log.Printf("tienlen: cộng XP phòng %s: %v", r.Code, err)
		return
	}
	for _, p := range r.players {
		if lv, ok := levels[p.UserID]; ok && !p.Bot {
			p.Level = lv
		}
	}
}

func (r *Room) stopTimer() {
	r.timerGen++
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.deadline = time.Time{}
}

// schedule đặt hẹn giờ cho lượt hiện tại: máy / người mất kết nối đánh sau BotDelay,
// người đang online có TurnTimeout trước khi máy đánh thay.
func (r *Room) schedule() {
	r.stopTimer()
	cur := r.players[r.game.Turn]
	delay := r.opts.BotDelay
	if !cur.Bot && cur.Online {
		delay = r.opts.TurnTimeout
		r.deadline = time.Now().Add(delay)
	}
	gen := r.timerGen
	r.timer = time.AfterFunc(delay, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if gen != r.timerGen || r.phase != PhasePlaying {
			return
		}
		r.autoMove()
	})
}

func (r *Room) scheduleIfTurn(p *Player) {
	if r.phase == PhasePlaying && r.seatOf(p) == r.game.Turn {
		r.schedule()
	}
}

func (r *Room) autoMove() {
	seat := r.game.Turn
	if move := tienlen.ChooseMove(r.game.Hands[seat], r.game.Context(seat)); move != nil {
		r.game.Play(seat, move)
	} else {
		r.game.Pass(seat)
	}
	r.afterMove()
}

func (r *Room) broadcast() {
	for _, p := range r.players {
		if p.conn != nil {
			p.conn.Send(r.stateFor(p))
		}
	}
}
