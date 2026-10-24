package game

import (
	"errors"
	"math/rand/v2"
	"slices"
)

// Điểm theo thứ hạng, tổng luôn bằng 0.
var rankPoints = map[int][]int{2: {1, -1}, 3: {2, 0, -2}, 4: {3, 1, -1, -3}}

const instantWinLoss = 2

type LastPlay struct {
	Player int    `json:"player"`
	Combo  *Combo `json:"combo"`
}

// Event là một dòng trong diễn biến ván: đánh, bỏ lượt, hoặc bắt đầu vòng mới.
type Event struct {
	Player   int    `json:"player"`
	Combo    *Combo `json:"combo,omitempty"`
	Chop     bool   `json:"chop,omitempty"`
	Pass     bool   `json:"pass,omitempty"`
	NewRound bool   `json:"newRound,omitempty"`
}

type Instant struct {
	Player int    `json:"player"`
	Reason string `json:"reason"`
}

// Game là một bàn 2–4 người. Scores và LastWinner giữ qua nhiều ván.
type Game struct {
	Players    int
	Scores     []int
	LastWinner int // -1 khi chưa có ván nào
	GameNo     int

	Hands       [][]Card
	Finished    []int
	Passed      []bool
	LastPlay    *LastPlay
	Turn        int
	MustInclude Card // -1 khi không bắt buộc
	Over        bool
	Instant     *Instant
	History     []Event
	Delta       []int // điểm cộng/trừ của ván vừa xong

	rand *rand.Rand
}

func NewGame(players int, r *rand.Rand) (*Game, error) {
	if rankPoints[players] == nil {
		return nil, errors.New("cần 2–4 người chơi")
	}
	return &Game{Players: players, Scores: make([]int, players), LastWinner: -1, rand: r}, nil
}

func (g *Game) Start() {
	n := g.Players
	g.GameNo++
	g.Hands = Deal(g.rand, n)
	g.Finished = nil
	g.Passed = make([]bool, n)
	g.LastPlay = nil
	g.Over = false
	g.Instant = nil
	g.History = nil
	g.Delta = make([]int, n)
	g.MustInclude = -1

	for p := range n {
		if reason := InstantWin(g.Hands[p]); reason != "" {
			g.Instant = &Instant{Player: p, Reason: reason}
			g.Over = true
			g.Finished = []int{p}
			g.LastWinner = p
			for q := range n {
				if q == p {
					g.Delta[q] = instantWinLoss * (n - 1)
				} else {
					g.Delta[q] = -instantWinLoss
				}
			}
			g.applyDelta()
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

func (g *Game) IsActive(p int) bool { return !slices.Contains(g.Finished, p) }

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
	chop := g.LastPlay != nil && combo.Type != g.LastPlay.Combo.Type
	g.Hands[p] = slices.DeleteFunc(g.Hands[p], func(c Card) bool { return slices.Contains(cards, c) })
	g.LastPlay = &LastPlay{Player: p, Combo: combo}
	g.MustInclude = -1
	g.History = append(g.History, Event{Player: p, Combo: combo, Chop: chop})

	if len(g.Hands[p]) == 0 {
		g.Finished = append(g.Finished, p)
		if len(g.Finished) == g.Players-1 {
			for q := range g.Players {
				if g.IsActive(q) {
					g.Finished = append(g.Finished, q)
				}
			}
			g.finish()
			return nil
		}
	}
	g.advance()
	return nil
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
	g.LastWinner = g.Finished[0]
	for place, p := range g.Finished {
		g.Delta[p] = rankPoints[g.Players][place]
	}
	g.applyDelta()
}

func (g *Game) applyDelta() {
	for p, d := range g.Delta {
		g.Scores[p] += d
	}
}
