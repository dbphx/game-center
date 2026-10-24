package online

import (
	"errors"
	"math/rand/v2"
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

func (h *Hub) Create(u User, conn Conn) (*Session, error) {
	h.mu.Lock()
	r := &Room{hub: h, opts: h.opts, phase: PhaseLobby, bet: DefaultBet}
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
