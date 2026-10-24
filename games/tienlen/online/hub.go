package online

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
)

const codeLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // bỏ I, O cho dễ đọc

// Hub giữ danh sách phòng Tiến Lên đang mở.
type Hub struct {
	opts  Options
	bank  Bank
	mu    sync.Mutex
	rooms map[string]*Room
}

func NewHub(bank Bank, opts Options) *Hub {
	return &Hub{opts: opts, bank: bank, rooms: map[string]*Room{}}
}

// Session là chỗ ngồi của một kết nối trong một phòng.
type Session struct {
	Room   *Room
	Player *Player
}

// Create mở bàn mới. bet không nằm trong BetOptions thì dùng DefaultBet.
func (h *Hub) Create(u User, conn Conn, bet int) (*Session, error) {
	if !slices.Contains(BetOptions, bet) {
		bet = DefaultBet
	}
	if err := h.opts.gate(u); err != nil {
		return nil, err
	}
	h.mu.Lock()
	r := &Room{hub: h, opts: h.opts, phase: PhaseLobby, bet: bet}
	for {
		var b strings.Builder
		for range 4 {
			b.WriteByte(codeLetters[rand.IntN(len(codeLetters))])
		}
		if h.rooms[b.String()] == nil {
			r.Code = b.String()
			break
		}
	}
	h.rooms[r.Code] = r
	h.mu.Unlock()

	p, err := r.join(u, conn)
	if err != nil {
		h.delete(r.Code)
		return nil, err
	}
	return &Session{Room: r, Player: p}, nil
}

// Join vào phòng theo mã; người đã có ghế trong phòng thì được trả lại ghế cũ.
func (h *Hub) Join(code string, u User, conn Conn) (*Session, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	h.mu.Lock()
	r := h.rooms[code]
	h.mu.Unlock()
	if r == nil {
		return nil, errors.New("không tìm thấy phòng " + code)
	}
	p, err := r.join(u, conn)
	if err != nil {
		return nil, err
	}
	return &Session{Room: r, Player: p}, nil
}

func (h *Hub) delete(code string) {
	h.mu.Lock()
	delete(h.rooms, code)
	h.mu.Unlock()
}

// Stats là số phòng và số người đang online, để hiện ở trang hub.
type Stats struct {
	Rooms   int `json:"rooms"`
	Players int `json:"players"`
}

func (h *Hub) Stats() Stats {
	h.mu.Lock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	h.mu.Unlock()

	s := Stats{Rooms: len(rooms)}
	for _, r := range rooms {
		r.mu.Lock()
		for _, p := range r.players {
			if !p.Bot && p.Online {
				s.Players++
			}
		}
		r.mu.Unlock()
	}
	return s
}

func (h *Hub) RoomCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}

// SeatSummary là người ngồi ở một ghế, chỉ gồm thông tin công khai để vẽ bàn ghế ở sảnh.
type SeatSummary struct {
	Name   string `json:"name"`
	Gender string `json:"gender,omitempty"`
	Outfit string `json:"outfit,omitempty"`
	Level  int    `json:"level,omitempty"`
	Bot    bool   `json:"bot"`
	Host   bool   `json:"host"`
	Online bool   `json:"online"`
}

// RoomSummary là một bàn đang mở trong sảnh Casino. Không lộ bài hay id nội bộ.
type RoomSummary struct {
	Code    string        `json:"code"`
	Phase   string        `json:"phase"`
	Bet     int           `json:"bet"`
	GameNo  int           `json:"gameNo"`
	Max     int           `json:"max"`
	Players []SeatSummary `json:"players"`
}

// Rooms chụp lại các bàn đang mở: bàn còn vào được (lobby / over) trước, đang chơi sau, cùng nhóm theo mã.
func (h *Hub) Rooms() []RoomSummary {
	h.mu.Lock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	h.mu.Unlock()

	out := []RoomSummary{}
	for _, r := range rooms {
		r.mu.Lock()
		s := RoomSummary{Code: r.Code, Phase: r.phase, Bet: r.bet, Max: MaxPlayers, Players: []SeatSummary{}}
		if r.game != nil {
			s.GameNo = r.game.GameNo
		}
		for _, p := range r.players {
			if p.Left {
				continue
			}
			s.Players = append(s.Players, SeatSummary{
				Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Level: p.Level,
				Bot: p.Bot, Host: p.ID == r.hostID, Online: p.Online,
			})
		}
		closed := r.closed
		r.mu.Unlock()
		if !closed && len(s.Players) > 0 {
			out = append(out, s)
		}
	}
	playing := func(s RoomSummary) int {
		if s.Phase == PhasePlaying {
			return 1
		}
		return 0
	}
	slices.SortFunc(out, func(a, b RoomSummary) int {
		if d := playing(a) - playing(b); d != 0 {
			return d
		}
		return strings.Compare(a.Code, b.Code)
	})
	return out
}

// RoomsHandler trả danh sách bàn (GET /api/tienlen/rooms), không cần đăng nhập.
func (h *Hub) RoomsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(h.Rooms())
	})
}
