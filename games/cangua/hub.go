package cangua

import (
	"encoding/json"
	"errors"
	mrand "math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
)

const codeLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // bỏ I, O cho dễ đọc

// Hub giữ các bàn Cờ Cá Ngựa đang mở.
type Hub struct {
	opts  Options
	bank  Bank
	mu    sync.Mutex
	rooms map[string]*Room
}

func NewHub(bank Bank, opts Options) *Hub {
	return &Hub{opts: opts, bank: bank, rooms: map[string]*Room{}}
}

// Session là chỗ (ghế hoặc xem) của một kết nối trong một bàn.
type Session struct {
	Room   *Room
	Player *Player
}

// Create mở bàn mới. bet không hợp lệ (hoặc thiếu) thì dùng DefaultBet.
func (h *Hub) Create(u User, conn Conn, bet *int) (*Session, error) {
	b := DefaultBet
	if bet != nil && slices.Contains(BetOptions, *bet) {
		b = *bet
	}
	if err := h.opts.gate(u); err != nil {
		return nil, err
	}
	r := &Room{hub: h, opts: h.opts, phase: PhaseLobby, bet: b,
		rng: mrand.New(mrand.NewPCG(mrand.Uint64(), mrand.Uint64()))}
	h.mu.Lock()
	for {
		var sb strings.Builder
		for range 4 {
			sb.WriteByte(codeLetters[mrand.IntN(len(codeLetters))])
		}
		if h.rooms[sb.String()] == nil {
			r.Code = sb.String()
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

// Join vào bàn theo mã: còn ghế thì ngồi, đủ người thì xem.
func (h *Hub) Join(code string, u User, conn Conn) (*Session, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	h.mu.Lock()
	r := h.rooms[code]
	h.mu.Unlock()
	if r == nil {
		return nil, errors.New("không tìm thấy bàn " + code)
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

func (h *Hub) snapshot() []*Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	return rooms
}

func (h *Hub) RoomCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}

// Stats là số bàn và người đang online, hiện ở trang hub.
type Stats struct {
	Rooms   int `json:"rooms"`
	Players int `json:"players"`
}

func (h *Hub) Stats() Stats {
	rooms := h.snapshot()
	s := Stats{Rooms: len(rooms)}
	for _, r := range rooms {
		r.mu.Lock()
		for _, p := range append(slices.Clone(r.players), r.spectators...) {
			if !p.Bot && p.Online {
				s.Players++
			}
		}
		r.mu.Unlock()
	}
	return s
}

// SeatSummary là người ngồi một ghế, chỉ thông tin công khai để vẽ ở sảnh.
type SeatSummary struct {
	Name   string `json:"name"`
	Gender string `json:"gender,omitempty"`
	Outfit string `json:"outfit,omitempty"`
	Hair   string `json:"hair,omitempty"`
	Wings  string `json:"wings,omitempty"`
	Face   string `json:"face,omitempty"`
	Level  int    `json:"level,omitempty"`
	Bot    bool   `json:"bot"`
	Color  string `json:"color"`
}

// RoomSummary là một bàn đang mở trong sảnh.
type RoomSummary struct {
	Code       string        `json:"code"`
	Phase      string        `json:"phase"`
	Bet        int           `json:"bet"`
	Players    []SeatSummary `json:"players"`
	Spectators int           `json:"spectators"`
	Max        int           `json:"max"`
}

// Rooms chụp lại các bàn: bàn còn ghế trống trước, đang chơi sau, cùng nhóm theo mã.
func (h *Hub) Rooms() []RoomSummary {
	out := []RoomSummary{}
	for _, r := range h.snapshot() {
		r.mu.Lock()
		s := RoomSummary{Code: r.Code, Phase: r.phase, Bet: r.bet, Players: []SeatSummary{},
			Spectators: len(r.spectators), Max: MaxPlayers}
		for i, p := range r.players {
			s.Players = append(s.Players, SeatSummary{Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Hair: p.Hair,
				Wings: p.Wings, Face: p.Face, Level: p.Level, Bot: p.Bot, Color: colorOf(i)})
		}
		closed := r.closed
		r.mu.Unlock()
		if !closed {
			out = append(out, s)
		}
	}
	rank := func(s RoomSummary) int {
		switch {
		case s.Phase == PhasePlaying:
			return 2
		case len(s.Players) >= s.Max:
			return 1
		}
		return 0
	}
	slices.SortFunc(out, func(a, b RoomSummary) int {
		if d := rank(a) - rank(b); d != 0 {
			return d
		}
		return strings.Compare(a.Code, b.Code)
	})
	return out
}

// RoomsHandler trả danh sách bàn (GET /api/cangua/rooms), không cần đăng nhập.
func (h *Hub) RoomsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(h.Rooms())
	})
}
