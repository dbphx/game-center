package tienlen

import (
	"errors"
	"math/rand/v2"
	"slices"
)

// Mọi khoản tiền trong package này tính bằng "mức cược"; phòng chơi nhân với số xu của một mức.

// Luật "nhất ăn tất, đếm lá": chỉ người về nhất được tiền thứ hạng. Lúc người nhất hết bài, mỗi người
// còn lại trả số lá đang cầm × 1 mức; bị cóng (chưa đánh lá nào) trả congCards lá (gấp đôi 13).
// Tới bằng 3♠ (bộ bài cuối có 3 bích) thì phần đếm lá nhân threeSpadesMul.
// Tới trắng: mỗi người trả 13 lá × instantWinLoss (= 26 lá, như bị cóng).
const (
	congCards      = 26
	threeSpadesMul = 2
	instantWinLoss = 2
)

// WinBy mô tả cách người nhất về: Multiplier nhân vào tiền nhất ăn tất (không nhân tiền chặt / thối).
type WinBy struct {
	ThreeSpades bool `json:"threeSpades"`
	Multiplier  int  `json:"multiplier"`
}

type LastPlay struct {
	Player int    `json:"player"`
	Combo  *Combo `json:"combo"`
	Chain  int    `json:"-"` // tổng tiền chặt dồn tới bộ này, để tính chặt chồng
}

// Transfer là một khoản chuyển tiền giữa hai người: chặt heo, chặt chồng, thối.
type Transfer struct {
	From   int    `json:"from"`
	To     int    `json:"to"`
	Units  int    `json:"units"`
	Reason string `json:"reason"`
}

// Event là một dòng trong diễn biến ván: đánh, bỏ lượt, hoặc bắt đầu vòng mới.
type Event struct {
	Player   int       `json:"player"`
	Combo    *Combo    `json:"combo,omitempty"`
	Chop     bool      `json:"chop,omitempty"`
	Pay      *Transfer `json:"pay,omitempty"` // tiền chặt, nếu có
	Pass     bool      `json:"pass,omitempty"`
	NewRound bool      `json:"newRound,omitempty"`
}

type Instant struct {
	Player int    `json:"player"`
	Reason string `json:"reason"`
}

// Game là một bàn 2–4 người. Scores và LastWinner giữ qua nhiều ván.
type Game struct {
	Players    int
	Scores     []int // tổng tiền (mức cược) qua các ván
	LastWinner int   // -1 khi chưa có ván nào
	GameNo     int

	Hands       [][]Card
	Finished    []int // thứ tự về; hết ván thì đủ mọi người, người bị cóng ở cuối
	Cong        []int // người bị cóng: chưa đánh lá nào khi có người về nhất
	Passed      []bool
	LastPlay    *LastPlay
	Turn        int
	MustInclude Card // -1 khi không bắt buộc
	Over        bool
	Instant     *Instant
	History     []Event
	Delta       []int      // tiền "nhất ăn tất" của ván vừa xong
	WinBy       *WinBy     // cách người nhất về (tới bằng 3♠ thì ×2); nil khi chưa có người về nhất
	CardsLeft   []int      // số lá bị tính tiền của từng người lúc người nhất về (cóng: 26, người nhất: 0)
	Transfers   []Transfer // tiền chặt và thối của ván vừa xong

	rand *rand.Rand
}

func NewGame(players int, r *rand.Rand) (*Game, error) {
	if players < 2 || players > 4 {
		return nil, errors.New("cần 2–4 người chơi")
	}
	return &Game{Players: players, Scores: make([]int, players), LastWinner: -1, rand: r}, nil
}

func (g *Game) Start() {
	n := g.Players
	g.GameNo++
	g.Hands = Deal(g.rand, n)
	g.Finished = nil
	g.Cong = nil
	g.Passed = make([]bool, n)
	g.LastPlay = nil
	g.Over = false
	g.Instant = nil
	g.History = nil
	g.Delta = make([]int, n)
	g.WinBy = nil
	g.CardsLeft = nil
	g.Transfers = nil
	g.MustInclude = -1

	for p := range n {
		if reason := InstantWin(g.Hands[p]); reason != "" {
			g.Instant = &Instant{Player: p, Reason: reason}
			g.Over = true
			g.Finished = []int{p}
			g.LastWinner = p
			g.CardsLeft = make([]int, n)
			for q := range n {
				if q != p {
					g.CardsLeft[q] = 13 * instantWinLoss
					g.Delta[q] = -g.CardsLeft[q]
					g.Delta[p] += g.CardsLeft[q]
				}
			}
			g.settle()
			return
		}
	}

	if g.LastWinner < 0 {
		// Ván đầu: ai giữ lá nhỏ nhất (đủ 4 người thì là 3♠) đánh trước và phải đánh kèm lá đó.
		lowest := 52
		for p, h := range g.Hands {
			if h[0] < lowest {
				lowest, g.Turn = h[0], p
			}
		}
		g.MustInclude = lowest
	} else {
		g.Turn = g.LastWinner
	}
}

// IsActive cho biết người chơi còn đang đánh (chưa về, không bị cóng).
func (g *Game) IsActive(p int) bool {
	return !slices.Contains(g.Finished, p) && !slices.Contains(g.Cong, p)
}

func (g *Game) activeCount() int {
	n := 0
	for p := range g.Players {
		if g.IsActive(p) {
			n++
		}
	}
	return n
}

func (g *Game) Context(p int) Context {
	ctx := Context{MustInclude: g.MustInclude, MinOpponentCards: 13}
	if g.LastPlay != nil {
		ctx.Prev = g.LastPlay.Combo
	}
	for q := range g.Players {
		if q != p && g.IsActive(q) {
			ctx.MinOpponentCards = min(ctx.MinOpponentCards, len(g.Hands[q]))
		}
	}
	return ctx
}

// Validate kiểm tra nước đi; trả về lỗi để hiện cho người chơi.
func (g *Game) Validate(p int, cards []Card) error {
	switch {
	case g.Over:
		return errors.New("ván đã kết thúc")
	case p != g.Turn:
		return errors.New("chưa tới lượt")
	case len(cards) == 0:
		return errors.New("chưa chọn lá nào")
	}
	seen := map[Card]bool{}
	for _, c := range cards {
		if seen[c] {
			return errors.New("lá bài bị trùng")
		}
		seen[c] = true
		if !slices.Contains(g.Hands[p], c) {
			return errors.New("lá bài không có trên tay")
		}
	}
	combo := GetCombo(cards)
	if combo == nil {
		return errors.New("bộ bài không hợp lệ")
	}
	if g.MustInclude >= 0 && !seen[g.MustInclude] {
		return errors.New("ván đầu phải đánh kèm " + CardName(g.MustInclude))
	}
	if g.LastPlay != nil && !CanBeat(combo, g.LastPlay.Combo) {
		return errors.New("không chặn được bài trên bàn")
	}
	return nil
}

func (g *Game) Play(p int, cards []Card) error {
	if err := g.Validate(p, cards); err != nil {
		return err
	}
	combo := GetCombo(cards)
	prev := g.LastPlay
	g.Hands[p] = slices.DeleteFunc(g.Hands[p], func(c Card) bool { return slices.Contains(cards, c) })
	g.LastPlay = &LastPlay{Player: p, Combo: combo}
	g.MustInclude = -1
	ev := Event{Player: p, Combo: combo}

	// Chặt heo / chặt hàng: người bị chặt trả cho người chặt. Chặt chồng thì người bị chặt
	// cuối cùng trả luôn phần mình đã ăn trước đó, nên người chặt sau cùng ăn trọn.
	if prev != nil && combo.IsBomb() && (prev.Combo.IsTwos() || prev.Combo.IsBomb()) {
		units := prev.Chain + ChopValue(prev.Combo)
		t := Transfer{From: prev.Player, To: p, Units: units, Reason: chopReason(prev)}
		g.Transfers = append(g.Transfers, t)
		g.LastPlay.Chain = units
		ev.Chop, ev.Pay = true, &t
	}
	g.History = append(g.History, ev)

	if len(g.Hands[p]) == 0 {
		g.Finished = append(g.Finished, p)
		if len(g.Finished) == 1 {
			// Bộ bài cuối cùng có 3 bích: tiền nhất ăn tất nhân đôi.
			g.WinBy = &WinBy{Multiplier: 1}
			if slices.Contains(cards, ThreeSpades) {
				g.WinBy = &WinBy{ThreeSpades: true, Multiplier: threeSpadesMul}
			}
			for q := range g.Players {
				if g.IsActive(q) && len(g.Hands[q]) == 13 {
					g.Cong = append(g.Cong, q)
				}
			}
			// Đếm lá ngay lúc người nhất về: những lá còn trên tay mỗi người là số tiền phải trả.
			g.CardsLeft = make([]int, g.Players)
			for q := range g.Players {
				if q == p {
					continue
				}
				g.CardsLeft[q] = len(g.Hands[q])
				if slices.Contains(g.Cong, q) {
					g.CardsLeft[q] = congCards
				}
			}
		}
		if g.activeCount() <= 1 {
			g.finish()
			return nil
		}
	}
	g.advance()
	return nil
}

func chopReason(prev *LastPlay) string {
	switch {
	case prev.Chain > 0:
		return "chặt chồng"
	case prev.Combo.Type == Single:
		return "chặt heo"
	case prev.Combo.Type == Pair:
		return "chặt đôi heo"
	default:
		return "chặt hàng"
	}
}

func (g *Game) Pass(p int) error {
	switch {
	case g.Over:
		return errors.New("ván đã kết thúc")
	case p != g.Turn:
		return errors.New("chưa tới lượt")
	case g.LastPlay == nil:
		return errors.New("bạn đang được đánh trước, không thể bỏ lượt")
	}
	g.Passed[p] = true
	g.History = append(g.History, Event{Player: p, Pass: true})
	g.advance()
	return nil
}

func (g *Game) advance() {
	i := g.Turn
	for range g.Players * 2 {
		i = (i + 1) % g.Players
		if i == g.LastPlay.Player {
			g.newRound(i)
			return
		}
		if g.IsActive(i) && !g.Passed[i] {
			g.Turn = i
			return
		}
	}
}

// Mọi người đã bỏ lượt: người đánh cuối được đánh tiếp.
// Nếu người đó đã về nhất/nhì..., quyền đánh chuyển sang người kế tiếp.
func (g *Game) newRound(leader int) {
	g.LastPlay = nil
	clear(g.Passed)
	for !g.IsActive(leader) {
		leader = (leader + 1) % g.Players
	}
	g.Turn = leader
	g.History = append(g.History, Event{Player: leader, NewRound: true})
}

func (g *Game) finish() {
	g.Over = true
	for q := range g.Players {
		if g.IsActive(q) {
			g.Finished = append(g.Finished, q)
		}
	}
	g.Finished = append(g.Finished, g.Cong...)
	winner := g.Finished[0]
	g.LastWinner = winner
	// Nhất ăn tất, đếm lá: mỗi người còn lại trả người nhất số lá đã ghi lúc người nhất về.
	mul := 1
	if g.WinBy != nil {
		mul = g.WinBy.Multiplier
	}
	for _, p := range g.Finished[1:] {
		pay := g.CardsLeft[p] * mul
		g.Delta[p] = -pay
		g.Delta[winner] += pay
	}

	// Thối: người về bét và người bị cóng trả heo / hàng còn trên tay cho người về nhất.
	payers := append([]int{g.Finished[g.Players-1]}, g.Cong...)
	for _, p := range slices.Compact(slices.Sorted(slices.Values(payers))) {
		if units, desc := Leftover(g.Hands[p]); units > 0 {
			g.Transfers = append(g.Transfers, Transfer{From: p, To: winner, Units: units, Reason: "thối " + desc})
		}
	}
	g.settle()
}

// Money trả về tổng tiền (mức cược) từng người được/mất trong ván vừa xong.
func (g *Game) Money() []int {
	m := slices.Clone(g.Delta)
	for _, t := range g.Transfers {
		m[t.From] -= t.Units
		m[t.To] += t.Units
	}
	return m
}

func (g *Game) settle() {
	for p, d := range g.Money() {
		g.Scores[p] += d
	}
}
