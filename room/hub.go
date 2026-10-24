package room

import (
	"errors"
	"math/rand/v2"
	"strings"
	"sync"
)

const codeLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // bỏ I, O cho dễ đọc

// Hub giữ danh sách phòng và tra cứu phiên theo token để vào lại khi mất kết nối.
type Hub struct {
	opts    Options
	mu      sync.Mutex
	rooms   map[string]*Room
	byToken map[string]*Room
}

func NewHub(opts Options) *Hub {
	return &Hub{opts: opts, rooms: map[string]*Room{}, byToken: map[string]*Room{}}
}

// Session là kết quả vào phòng: token dùng để vào lại sau khi tải lại trang.
type Session struct {
	Room   *Room
	Player *Player
	Token  string
}

func (h *Hub) Create(name string, conn Conn) (*Session, error) {
	h.mu.Lock()
	r := &Room{hub: h, opts: h.opts, phase: PhaseLobby}
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
	return r.join(name, conn)
}

func (h *Hub) Join(code, name string, conn Conn) (*Session, error) {
	h.mu.Lock()
	r := h.rooms[strings.ToUpper(strings.TrimSpace(code))]
	h.mu.Unlock()
	if r == nil {
		return nil, errors.New("không tìm thấy phòng " + code)
	}
	return r.join(name, conn)
}

func (h *Hub) Resume(token string, conn Conn) (*Session, error) {
	h.mu.Lock()
	r := h.byToken[token]
	h.mu.Unlock()
	if r == nil {
		return nil, errors.New("phiên đã hết hạn")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.players {
		if p.token == token && !p.Left {
			r.resume(p, conn)
			return &Session{Room: r, Player: p, Token: token}, nil
		}
	}
	return nil, errors.New("phiên đã hết hạn")
}

func (r *Room) join(name string, conn Conn) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, err := r.addHuman(name, conn)
	if err != nil {
		return nil, err
	}
	return &Session{Room: r, Player: p, Token: p.token}, nil
}

func (h *Hub) register(token string, r *Room) {
	h.mu.Lock()
	h.byToken[token] = r
	h.mu.Unlock()
}

func (h *Hub) unregister(token string) {
	h.mu.Lock()
	delete(h.byToken, token)
	h.mu.Unlock()
}

func (h *Hub) delete(code string) {
	h.mu.Lock()
	delete(h.rooms, code)
	h.mu.Unlock()
}

func (h *Hub) RoomCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}
