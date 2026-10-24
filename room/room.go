// Package room quản lý các phòng chơi online: người vào/ra, máy, lượt đánh và hẹn giờ.
//
// Mọi thao tác trên một Room đều chạy dưới Room.mu. Thứ tự khoá là Room.mu → Hub.mu,
// Hub không bao giờ giữ Hub.mu khi gọi vào Room.
package room

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dbphx/game-center/game"
)

const (
	MinPlayers = 2
	MaxPlayers = 4
	maxNameLen = 16
)

const (
	PhaseLobby   = "lobby"
	PhasePlaying = "playing"
	PhaseOver    = "over"
)

// Conn là kết nối tới một người chơi. Send không được chặn vì được gọi khi đang giữ khoá phòng.
type Conn interface {
	Send(msg any)
}

type Options struct {
	BotDelay    time.Duration // thời gian máy "suy nghĩ"
	TurnTimeout time.Duration // hết giờ thì máy đánh thay
	Grace       time.Duration // chờ người mất kết nối quay lại trước khi xoá khỏi phòng
}

var DefaultOptions = Options{BotDelay: 900 * time.Millisecond, TurnTimeout: 30 * time.Second, Grace: 30 * time.Second}

type Player struct {
	ID     string
	Name   string
	Bot    bool
	Online bool
	Left   bool // rời phòng giữa ván: máy đánh thay tới hết ván rồi xoá
	Score  int

	token     string
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
	game     *game.Game
	roster   string // id các ghế của game hiện tại; đổi người thì tạo game mới
	timer    *time.Timer
	timerGen int // tăng mỗi lần đặt hẹn giờ để bỏ qua hẹn giờ cũ
	deadline time.Time
	botSeq   int
	closed   bool
}

func randomID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func cleanName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Người chơi"
	}
	if r := []rune(name); len(r) > maxNameLen {
		name = string(r[:maxNameLen])
	}
	return name
}

func (r *Room) seatOf(p *Player) int { return slices.Index(r.players, p) }

func (r *Room) host() *Player {
	for _, p := range r.players {
		if p.ID == r.hostID {
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

func (r *Room) addHuman(name string, conn Conn) (*Player, error) {
	if r.closed {
		return nil, errors.New("phòng đã đóng")
	}
	if r.phase == PhasePlaying {
		return nil, errors.New("phòng đang chơi, đợi hết ván rồi vào lại")
	}
	if len(r.players) >= MaxPlayers {
		return nil, errors.New("phòng đã đủ 4 người")
	}
	p := &Player{ID: randomID(), Name: cleanName(name), Online: true, token: randomID(), conn: conn}
	r.players = append(r.players, p)
	if r.host() == nil {
		r.hostID = p.ID
	}
	r.hub.register(p.token, r)
	r.rosterChanged()
	return p, nil
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
	if !p.Bot {
		r.hub.unregister(p.token)
		if notify && p.conn != nil {
			p.conn.Send(map[string]string{"t": "left"})
		}
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
		if !p.Bot {
			r.hub.unregister(p.token)
		}
		if p.graceTime != nil {
			p.graceTime.Stop()
		}
	}
	r.hub.delete(r.Code)
}

// Message là tin nhắn từ client.
type Message struct {
	T     string `json:"t"`
	Name  string `json:"name,omitempty"`
	Code  string `json:"code,omitempty"`
	Token string `json:"token,omitempty"`
	Cards []int  `json:"cards,omitempty"`
	ID    string `json:"id,omitempty"`
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
		cards := game.ChooseMove(r.game.Hands[seat], r.game.Context(seat))
		p.conn.Send(map[string]any{"t": "hint", "cards": cards})

	case "leave":
		r.leave(p)

	default:
		return fmt.Errorf("không hiểu lệnh %q", m.T)
	}
	return nil
}

func (r *Room) findPlayer(id string) *Player {
	for _, p := range r.players {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (r *Room) leave(p *Player) {
	if r.phase != PhasePlaying {
		r.remove(p, true)
		return
	}
	// Giữa ván: máy đánh thay tới hết ván.
	p.Left, p.Online = true, false
	r.hub.unregister(p.token)
	if p.conn != nil {
		p.conn.Send(map[string]string{"t": "left"})
		p.conn = nil
	}
	if len(r.humans()) == 0 {
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
				r.close()
			}
			return
		}
		r.remove(p, false)
	})
	r.scheduleIfTurn(p)
	r.broadcast()
}

func (r *Room) resume(p *Player, conn Conn) {
	if p.graceTime != nil {
		p.graceTime.Stop()
	}
	if p.conn != nil && p.conn != conn {
		p.conn.Send(map[string]string{"t": "replaced"})
	}
	p.conn, p.Online = conn, true
	if r.host() == nil || !r.host().Online {
		r.hostID = p.ID
	}
	r.scheduleIfTurn(p)
	r.broadcast()
}

func (r *Room) startGame() error {
	for _, p := range slices.Clone(r.players) {
		if p.Left {
			r.players = slices.DeleteFunc(r.players, func(q *Player) bool { return q == p })
		}
	}
	if len(r.players) < MinPlayers {
		r.broadcast()
		return errors.New("cần ít nhất 2 người, hãy thêm máy hoặc mời bạn bè")
	}
	ids := make([]string, len(r.players))
	for i, p := range r.players {
		ids[i] = p.ID
	}
	roster := strings.Join(ids, ",")
	if r.game == nil || roster != r.roster {
		r.game, _ = game.NewGame(len(r.players), mrand.New(mrand.NewPCG(mrand.Uint64(), mrand.Uint64())))
		r.roster = roster
	}
	r.game.Start()
	r.phase = PhasePlaying
	r.afterMove()
	return nil
}

func (r *Room) afterMove() {
	if r.game.Over {
		r.phase = PhaseOver
		r.stopTimer()
		for i, p := range r.players {
			p.Score += r.game.Delta[i]
		}
	} else {
		r.schedule()
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
	if move := game.ChooseMove(r.game.Hands[seat], r.game.Context(seat)); move != nil {
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
