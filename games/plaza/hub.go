// Package plaza là phòng "Công viên" (Quảng trường) của DCity: ai mở trang /plaza/ là vào phòng, thấy nhau
// đi lại trong cảnh công viên, trò chuyện, gửi biểu cảm, tặng xu, mời chơi; đóng trang là rời phòng.
// Kết nối qua WebSocket /ws/plaza; vị trí không lưu vào database, chỉ tặng xu mới đi qua ví.
package plaza

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dbphx/game-center/internal/wallet"
)

const (
	GameID = "plaza"

	MaxChatRunes = 120
	ChatHistory  = 30
	FlushEvery   = 100 * time.Millisecond // gộp các gói vị trí rồi gửi một lần

	GiftMin      = 10
	GiftMax      = 1000
	GiftDailyCap = 5000

	// Kích thước cảnh công viên (toạ độ cảnh, khớp SCENE_W / SCENE_H trong public/plaza/plaza-scene.js).
	SceneW = 2400
	SceneH = 1000
)

// InScene cho biết (x, y) có nằm trong cảnh công viên không; vị trí ngoài cảnh bị bỏ qua.
func InScene(x, y float64) bool { return x >= 0 && x <= SceneW && y >= 0 && y <= SceneH }

var errNotInPlaza = errors.New("vào Quảng trường để gặp mọi người nhé")

var Emotes = []string{"wave", "heart", "laugh", "clap", "dance"}

var dirs = map[string]bool{"down": true, "up": true, "left": true, "right": true}

// Look là phần hình nhân vật người khác cần để vẽ.
type Look struct {
	Gender string `json:"gender"`
	Outfit string `json:"outfit"`
	Hair   string `json:"hair,omitempty"`
	Wings  string `json:"wings,omitempty"`
	Face   string `json:"face,omitempty"`
}

// Player là một phiên (một tab) đang ở thành phố. ID là mã phiên ngẫu nhiên, không lộ id tài khoản.
type Player struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Level    int     `json:"level"`
	Look     Look    `json:"look"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Dir      string  `json:"dir"`
	Walking  bool    `json:"walking"`
	Sitting  bool    `json:"sitting"`
	Hospital bool    `json:"hospital"` // đang nằm viện: giao diện vẽ tư thế nằm

	uid     int64
	conn    Conn
	visible bool // đang đứng trong Quảng trường: chỉ khi đó mới thấy / được thấy người khác
	dirty   bool // vị trí đổi, chờ gửi ở lần gộp tới

	chatTimes []time.Time
	lastEmote time.Time
	lastGift  time.Time
}

// Conn là kết nối của một phiên (WebSocket thật hoặc giả trong test).
type Conn interface{ Send(msg any) }

// Wallet là phần ví quảng trường cần.
type Wallet interface {
	UserByToken(ctx context.Context, token string) (wallet.User, error)
	User(ctx context.Context, id int64) (wallet.User, error)
	Gift(ctx context.Context, game string, from, to, amount, dailyCap int64, noteFrom, noteTo string) (int64, int64, error)
}

// ChatLine là một tin trong lịch sử trò chuyện.
type ChatLine struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Text string `json:"text"`
	At   int64  `json:"at"` // unix ms
}

// Message là tin client gửi lên.
type Message struct {
	T       string  `json:"t"`
	Token   string  `json:"token,omitempty"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Dir     string  `json:"dir,omitempty"`
	Walking bool    `json:"walking,omitempty"`
	Sitting bool    `json:"sitting,omitempty"`
	Text    string  `json:"text,omitempty"`
	Kind    string  `json:"kind,omitempty"`
	To      string  `json:"to,omitempty"`
	Amount  int64   `json:"amount,omitempty"`
	Game    string  `json:"game,omitempty"`
	Link    string  `json:"link,omitempty"`
}

// Hub giữ mọi phiên đang ở thành phố.
type Hub struct {
	w   Wallet
	now func() time.Time

	mu      sync.Mutex
	players map[string]*Player
	chat    []ChatLine
}

func NewHub(w Wallet) *Hub {
	return &Hub{w: w, now: time.Now, players: map[string]*Player{}}
}

// Run gộp và gửi vị trí định kỳ tới khi ctx kết thúc.
func (h *Hub) Run(ctx context.Context) {
	t := time.NewTicker(FlushEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			h.Flush()
		}
	}
}

// Online là số phiên đang ở Quảng trường.
func (h *Hub) Online() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, p := range h.players {
		if p.visible {
			n++
		}
	}
	return n
}

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func lookOf(u wallet.User) Look {
	return Look{Gender: u.Gender, Outfit: u.Outfit, Hair: u.Hair, Wings: u.Wings, Face: u.Face}
}

func (p *Player) setUser(u wallet.User) {
	p.uid, p.Name, p.Level, p.Look, p.Hospital = u.ID, u.Name, u.Level, lookOf(u), u.HospitalUntil > 0
}

// Hello xác thực token rồi vào phòng công viên: "welcome" gồm người đang ở công viên và lịch sử chat.
// Người khác chỉ thấy phiên mới khi nó gửi vị trí đầu tiên (đã đặt chân vào cảnh).
func (h *Hub) Hello(ctx context.Context, token string, c Conn) (*Player, error) {
	u, err := h.w.UserByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	p := &Player{ID: newID(), Dir: "down", conn: c}
	p.setUser(u)
	h.mu.Lock()
	defer h.mu.Unlock()
	// Mỗi tài khoản một phiên ở công viên: phiên cũ (tab khác, hoặc kết nối chết chưa hết readTimeout khi vừa
	// thoát ra vào lại) bị bỏ ngay — không thì người khác thấy hai nhân vật giống nhau.
	for _, o := range h.players {
		if o.uid == p.uid {
			h.drop(o)
		}
	}
	others := []Player{}
	for _, o := range h.players {
		if o.visible {
			others = append(others, *o)
		}
	}
	h.players[p.ID] = p
	c.Send(map[string]any{"t": "welcome", "you": p.ID, "players": others, "chat": append([]ChatLine{}, h.chat...), "emotes": Emotes,
		"gift":  map[string]int{"min": GiftMin, "max": GiftMax, "daily": GiftDailyCap},
		"scene": map[string]int{"w": SceneW, "h": SceneH}})
	return p, nil
}

// Leave xoá phiên (đóng tab / mất kết nối).
func (h *Hub) Leave(p *Player) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.players[p.ID] != p {
		return
	}
	h.remove(p)
}

// remove bỏ p khỏi công viên và báo người khác. Gọi khi đang giữ h.mu.
func (h *Hub) remove(p *Player) {
	delete(h.players, p.ID)
	if p.visible {
		h.broadcast(p, map[string]any{"t": "leave", "id": p.ID})
	}
}

// drop bỏ phiên cũ khi cùng tài khoản vừa vào lại: báo "replaced" (tab cũ thôi tự nối lại) rồi đóng kết nối.
// Gọi khi đang giữ h.mu.
func (h *Hub) drop(p *Player) {
	h.remove(p)
	p.conn.Send(map[string]any{"t": "replaced"})
	if c, ok := p.conn.(interface{ Close() }); ok {
		c.Close()
	}
}

// broadcast gửi msg tới mọi phiên đang ở Quảng trường, trừ skip (skip nil = gửi cả). Gọi khi đang giữ h.mu.
func (h *Hub) broadcast(skip *Player, msg any) {
	for _, o := range h.players {
		if o != skip && o.visible {
			o.conn.Send(msg)
		}
	}
}

func validNum(v, limit float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && math.Abs(v) <= limit
}

// Handle xử lý một tin của phiên p. Lỗi trả về được gửi lại riêng cho p.
func (h *Hub) Handle(ctx context.Context, p *Player, m Message) error {
	switch m.T {
	case "move":
		return h.move(p, m)
	case "chat":
		return h.chatMsg(p, m.Text)
	case "emote":
		return h.emote(p, m.Kind)
	case "gift":
		return h.gift(ctx, p, m.To, m.Amount)
	case "look":
		return h.refresh(ctx, p)
	case "invite":
		return h.invite(p, m)
	case "ping":
		return nil
	}
	return nil
}

func (h *Hub) move(p *Player, m Message) error {
	if !validNum(m.X, SceneW) || !validNum(m.Y, SceneH) || !InScene(m.X, m.Y) {
		return nil // bỏ qua gói hỏng / ngoài cảnh công viên
	}
	if !dirs[m.Dir] {
		m.Dir = "down"
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	p.X, p.Y, p.Dir, p.Walking, p.Sitting = math.Round(m.X*10)/10, math.Round(m.Y*10)/10, m.Dir, m.Walking, m.Sitting
	if !p.visible {
		// Vị trí đầu tiên: hiện với mọi người trong công viên.
		h.broadcast(p, map[string]any{"t": "join", "player": *p})
		p.visible = true
		return nil
	}
	p.dirty = true
	return nil
}

// moveView là vị trí gọn gửi trong gói gộp.
type moveView struct {
	ID      string  `json:"id"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Dir     string  `json:"dir"`
	Walking bool    `json:"walking"`
	Sitting bool    `json:"sitting"`
}

// Flush gửi một gói "moves" gồm mọi vị trí đổi kể từ lần trước (mỗi người chỉ nhận vị trí người khác).
func (h *Hub) Flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	var moved []*Player
	for _, p := range h.players {
		if p.dirty {
			p.dirty = false
			moved = append(moved, p)
		}
	}
	if len(moved) == 0 {
		return
	}
	for _, o := range h.players {
		if !o.visible {
			continue
		}
		list := make([]moveView, 0, len(moved))
		for _, p := range moved {
			if p != o {
				list = append(list, moveView{p.ID, p.X, p.Y, p.Dir, p.Walking, p.Sitting})
			}
		}
		if len(list) > 0 {
			o.conn.Send(map[string]any{"t": "moves", "list": list})
		}
	}
}

func (h *Hub) chatMsg(p *Player, text string) error {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) > MaxChatRunes {
		text = string([]rune(text)[:MaxChatRunes])
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if !p.visible {
		return errNotInPlaza
	}
	// Chống spam: tối thiểu 1,5 giây giữa hai tin và tối đa 5 tin trong 10 giây.
	recent := p.chatTimes[:0]
	for _, t := range p.chatTimes {
		if now.Sub(t) < 10*time.Second {
			recent = append(recent, t)
		}
	}
	p.chatTimes = recent
	if n := len(recent); n > 0 && now.Sub(recent[n-1]) < 1500*time.Millisecond || n >= 5 {
		return errors.New("bạn nói nhanh quá, chờ chút nhé")
	}
	p.chatTimes = append(p.chatTimes, now)
	line := ChatLine{ID: p.ID, Name: p.Name, Text: Clean(text), At: now.UnixMilli()}
	h.chat = append(h.chat, line)
	if len(h.chat) > ChatHistory {
		h.chat = h.chat[len(h.chat)-ChatHistory:]
	}
	h.broadcast(nil, map[string]any{"t": "chat", "id": p.ID, "name": p.Name, "text": line.Text, "at": line.At})
	return nil
}

func (h *Hub) emote(p *Player, kind string) error {
	ok := false
	for _, e := range Emotes {
		ok = ok || e == kind
	}
	if !ok {
		return nil
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if !p.visible || now.Sub(p.lastEmote) < time.Second {
		return nil // ngoài Quảng trường hoặc bấm dồn: bỏ qua lặng lẽ
	}
	p.lastEmote = now
	h.broadcast(nil, map[string]any{"t": "emote", "id": p.ID, "kind": kind})
	return nil
}

// gift tặng xu cho người chơi có phiên to. Chuyển xu trong một transaction của ví; báo số dư mới cho
// mọi phiên của hai người và phát hiệu ứng cho mọi người ở Quảng trường.
func (h *Hub) gift(ctx context.Context, p *Player, to string, amount int64) error {
	if amount < GiftMin || amount > GiftMax {
		return fmt.Errorf("mỗi lần tặng từ %d đến %d xu", GiftMin, GiftMax)
	}
	now := h.now()
	h.mu.Lock()
	target := h.players[to]
	if target == nil {
		h.mu.Unlock()
		return errors.New("người này đã rời thành phố")
	}
	if !p.visible || !target.visible {
		h.mu.Unlock()
		return errors.New("chỉ tặng xu được khi cả hai đang ở Quảng trường")
	}
	if target.uid == p.uid {
		h.mu.Unlock()
		return wallet.ErrGiftSelf
	}
	if now.Sub(p.lastGift) < 2*time.Second {
		h.mu.Unlock()
		return errors.New("chờ chút rồi tặng tiếp nhé")
	}
	p.lastGift = now
	from, toUID, fromName, toName := p.uid, target.uid, p.Name, target.Name
	h.mu.Unlock()

	fb, tb, err := h.w.Gift(ctx, GameID, from, toUID, amount, GiftDailyCap,
		fmt.Sprintf("Tặng %d xu cho %s", amount, toName), fmt.Sprintf("Nhận %d xu từ %s", amount, fromName))
	if err != nil {
		if errors.Is(err, wallet.ErrNotEnoughCoins) {
			return errors.New("không đủ xu để tặng")
		}
		var lim *wallet.GiftLimitError
		if errors.As(err, &lim) {
			return fmt.Errorf("hôm nay bạn chỉ còn tặng được %d / %d xu", max(0, lim.Left), GiftDailyCap)
		}
		if errors.Is(err, wallet.ErrGiftSelf) || errors.Is(err, wallet.ErrNotFound) {
			return err
		}
		return errors.New("lỗi máy chủ, thử lại sau")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, o := range h.players {
		switch o.uid {
		case from:
			o.conn.Send(map[string]any{"t": "balance", "balance": fb})
		case toUID:
			o.conn.Send(map[string]any{"t": "balance", "balance": tb})
		}
	}
	h.broadcast(nil, map[string]any{"t": "gift", "from": p.ID, "to": to, "fromName": fromName, "toName": toName, "amount": amount})
	return nil
}

// refresh đọc lại tài khoản (đổi đồ, lên level, nằm viện...) rồi báo cho mọi người.
func (h *Hub) refresh(ctx context.Context, p *Player) error {
	h.mu.Lock()
	uid := p.uid
	h.mu.Unlock()
	u, err := h.w.User(ctx, uid)
	if err != nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	p.setUser(u)
	if p.visible {
		h.broadcast(p, map[string]any{"t": "look", "id": p.ID, "name": p.Name, "level": p.Level, "look": p.Look, "hospital": p.Hospital})
	}
	return nil
}

var inviteGames = map[string]string{"caro": "/caro/", "baucua": "/baucua/", "tienlen": "/tienlen/"}

// invite chuyển lời mời chơi game (kèm link bàn) tới riêng một người.
func (h *Hub) invite(p *Player, m Message) error {
	base, ok := inviteGames[m.Game]
	if !ok || !strings.HasPrefix(m.Link, base) || len(m.Link) > 80 || strings.ContainsAny(m.Link, "<>\"' ") {
		return errors.New("lời mời không hợp lệ")
	}
	now := h.now()
	h.mu.Lock()
	defer h.mu.Unlock()
	target := h.players[m.To]
	if target == nil {
		return errors.New("người này đã rời thành phố")
	}
	if !p.visible || !target.visible {
		return errors.New("chỉ mời chơi được khi cả hai đang ở Quảng trường")
	}
	if now.Sub(p.lastEmote) < time.Second {
		return nil
	}
	p.lastEmote = now
	target.conn.Send(map[string]any{"t": "invite", "from": p.ID, "name": p.Name, "game": m.Game, "link": m.Link})
	return nil
}
