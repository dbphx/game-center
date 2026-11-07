package race

import (
	"encoding/json"
	"errors"
	mrand "math/rand/v2"
	"net/http"
	"slices"
	"sort"
	"strings"
	"sync"
)

const codeLetters = "ABCDEFGHJKLMNPQRSTUVWXYZ" // bỏ I, O cho dễ đọc

// Hub giữ các phòng đua đang mở.
type Hub struct {
	opts     Options
	bank     Bank
	loadouts Loadouts
	tracks   map[string]*Track
	mu       sync.Mutex
	rooms    map[string]*Room
}

func NewHub(bank Bank, loadouts Loadouts, tracks map[string]*Track, opts Options) *Hub {
	if tracks == nil {
		tracks = map[string]*Track{}
	}
	return &Hub{opts: opts, bank: bank, loadouts: loadouts, tracks: tracks, rooms: map[string]*Room{}}
}

// trackList là các đường đua để chọn, xếp theo id.
func (h *Hub) trackList() []TrackInfo {
	out := []TrackInfo{}
	for _, t := range h.tracks {
		out = append(out, TrackInfo{ID: t.ID, Name: t.Name, Laps: t.Laps})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// defaultTrack: đường được chọn sẵn khi tạo phòng ("neon" nếu có, không thì đường đầu tiên).
func (h *Hub) defaultTrack(want string) *Track {
	if t := h.tracks[want]; t != nil {
		return t
	}
	if t := h.tracks["neon"]; t != nil {
		return t
	}
	if l := h.trackList(); len(l) > 0 {
		return h.tracks[l[0].ID]
	}
	return nil
}

// Session là chỗ (ghế hoặc xem) của một kết nối trong một phòng.
type Session struct {
	Room   *Room
	Player *Player
}

// Create mở phòng mới. bet không hợp lệ (hoặc thiếu) thì dùng DefaultBet; track lạ thì dùng đường mặc định.
func (h *Hub) Create(u User, conn Conn, bet *int, track string) (*Session, error) {
	if len(h.tracks) == 0 {
		return nil, errors.New("Trường đua chưa có đường đua")
	}
	b := DefaultBet
	if bet != nil && slices.Contains(BetOptions, *bet) {
		b = *bet
	}
	if err := h.opts.gate(u); err != nil {
		return nil, err
	}
	r := &Room{hub: h, opts: h.opts, phase: PhaseLobby, bet: b, track: h.defaultTrack(track),
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

// Join vào phòng theo mã: còn ghế (chưa đua) thì ngồi, không thì xem.
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

func (h *Hub) snapshot() []*Room {
	h.mu.Lock()
	defer h.mu.Unlock()
	rooms := make([]*Room, 0, len(h.rooms))
	for _, r := range h.rooms {
		rooms = append(rooms, r)
	}
	return rooms
}

// RoomStats là số phòng và người đang online, hiện ở trang hub.
type RoomStats struct {
	Rooms   int `json:"rooms"`
	Players int `json:"players"`
}

func (h *Hub) Stats() RoomStats {
	rooms := h.snapshot()
	s := RoomStats{Rooms: len(rooms)}
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
	Level  int    `json:"level,omitempty"`
	Bot    bool   `json:"bot"`
	Car    string `json:"car"`
}

// RoomSummary là một phòng đang mở trong sảnh.
type RoomSummary struct {
	Code       string        `json:"code"`
	Phase      string        `json:"phase"`
	Bet        int           `json:"bet"`
	Track      string        `json:"track"`
	Players    []SeatSummary `json:"players"`
	Spectators int           `json:"spectators"`
	Max        int           `json:"max"`
}

// Rooms chụp lại các phòng: còn ghế trống trước, đang đua sau, cùng nhóm theo mã.
func (h *Hub) Rooms() []RoomSummary {
	out := []RoomSummary{}
	for _, r := range h.snapshot() {
		r.mu.Lock()
		s := RoomSummary{Code: r.Code, Phase: r.phase, Bet: r.bet, Players: []SeatSummary{},
			Spectators: len(r.spectators), Max: MaxPlayers}
		if r.track != nil {
			s.Track = r.track.ID
		}
		for _, p := range r.players {
			s.Players = append(s.Players, SeatSummary{Name: p.Name, Gender: p.Gender, Outfit: p.Outfit, Level: p.Level,
				Bot: p.Bot, Car: p.Car.ID})
		}
		closed, racing := r.closed, r.racing()
		r.mu.Unlock()
		if closed {
			continue
		}
		if racing {
			s.Phase = PhaseRacing
		}
		out = append(out, s)
	}
	rank := func(s RoomSummary) int {
		switch {
		case s.Phase == PhaseRacing:
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

// RoomsHandler trả danh sách phòng (GET /api/race/rooms), không cần đăng nhập.
func (h *Hub) RoomsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(h.Rooms())
	})
}

// RoomCount là số phòng đang mở.
func (h *Hub) RoomCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.rooms)
}
