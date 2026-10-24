// Package game cài luật Tiến Lên Miền Nam, luồng ván bài và bot.
//
// Lá bài là số nguyên 0..51: rank = c / 4 (0 = 3, ..., 11 = A, 12 = 2), suit = c % 4 (0 ♠, 1 ♣, 2 ♦, 3 ♥).
// So sánh 2 lá chỉ cần so số: rank trước, chất sau. Client JS (public/js/rules.js) dùng cùng quy ước.
package tienlen

import (
	"math/rand/v2"
	"slices"
)

type Card = int

const (
	Rank2       = 12
	ThreeSpades = Card(0)
)

var (
	RankNames = []string{"3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K", "A", "2"}
	SuitNames = []string{"♠", "♣", "♦", "♥"}
)

func Rank(c Card) int        { return c / 4 }
func Suit(c Card) int        { return c % 4 }
func CardName(c Card) string { return RankNames[Rank(c)] + SuitNames[Suit(c)] }

type ComboType string

const (
	Single   ComboType = "single"
	Pair     ComboType = "pair"
	Triple   ComboType = "triple"
	Straight ComboType = "straight"
	PairSeq  ComboType = "pairseq" // đôi thông
	Quad     ComboType = "quad"
)

// Combo là một bộ bài hợp lệ. Length là số lá với sảnh, số đôi với đôi thông.
type Combo struct {
	Type   ComboType `json:"type"`
	Length int       `json:"length"`
	Top    Card      `json:"top"`
	Cards  []Card    `json:"cards"`
}

func (c *Combo) IsTwos() bool { return Rank(c.Top) == Rank2 }

func (c *Combo) IsBomb() bool {
	return c.Type == Quad || (c.Type == PairSeq && c.Length >= 3)
}

func Deal(r *rand.Rand, players int) [][]Card {
	deck := make([]Card, 52)
	for i := range deck {
		deck[i] = i
	}
	r.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	hands := make([][]Card, players)
	for p := range hands {
		hands[p] = slices.Sorted(slices.Values(deck[p*13 : p*13+13]))
	}
	return hands
}

func isConsecutive(ranks []int) bool {
	for i := 1; i < len(ranks); i++ {
		if ranks[i] != ranks[i-1]+1 {
			return false
		}
	}
	return true
}

// GetCombo trả về bộ bài tương ứng hoặc nil nếu không hợp lệ.
func GetCombo(cards []Card) *Combo {
	n := len(cards)
	if n == 0 {
		return nil
	}
	cs := slices.Sorted(slices.Values(cards))
	ranks := make([]int, n)
	allSame := true
	for i, c := range cs {
		ranks[i] = Rank(c)
		if ranks[i] != ranks[0] {
			allSame = false
		}
	}
	combo := func(t ComboType, length int) *Combo {
		return &Combo{Type: t, Length: length, Top: cs[n-1], Cards: cs}
	}

	if n == 1 {
		return combo(Single, 1)
	}
	if allSame {
		switch n {
		case 2:
			return combo(Pair, 2)
		case 3:
			return combo(Triple, 3)
		case 4:
			return combo(Quad, 4)
		}
	}
	if slices.Contains(ranks, Rank2) {
		return nil
	}
	if n >= 3 && isConsecutive(ranks) {
		return combo(Straight, n)
	}
	if n >= 6 && n%2 == 0 {
		pairRanks := make([]int, 0, n/2)
		for i := 0; i < n; i += 2 {
			if ranks[i] != ranks[i+1] {
				return nil
			}
			pairRanks = append(pairRanks, ranks[i])
		}
		if isConsecutive(pairRanks) {
			return combo(PairSeq, n/2)
		}
	}
	return nil
}

// CanBeat cho biết combo có đè được prev không (kể cả chặt). prev nil là đang được đánh tự do.
func CanBeat(combo, prev *Combo) bool {
	if combo == nil {
		return false
	}
	if prev == nil {
		return true
	}
	if combo.Type == prev.Type && combo.Length == prev.Length {
		return combo.Top > prev.Top
	}

	quad := combo.Type == Quad
	seq3 := combo.Type == PairSeq && combo.Length == 3
	seq4 := combo.Type == PairSeq && combo.Length >= 4

	switch {
	case prev.Type == Single && prev.IsTwos(): // chặt heo
		return quad || seq3 || seq4
	case prev.Type == Pair && prev.IsTwos(): // chặt đôi heo
		return quad || seq4
	case prev.Type == PairSeq && prev.Length == 3: // chặt chồng
		return quad || seq4
	case prev.Type == Quad:
		return seq4
	}
	return false
}

// GroupByRank chia tay bài theo rank, mỗi nhóm đã sắp xếp.
func GroupByRank(hand []Card) [13][]Card {
	var g [13][]Card
	for _, c := range slices.Sorted(slices.Values(hand)) {
		g[Rank(c)] = append(g[Rank(c)], c)
	}
	return g
}

// InstantWin trả về lý do tới trắng, hoặc "" nếu không có.
func InstantWin(hand []Card) string {
	g := GroupByRank(hand)
	if len(g[Rank2]) == 4 {
		return "Tứ quý heo"
	}
	dragon := true
	for r := 0; r < 12; r++ {
		if len(g[r]) == 0 {
			dragon = false
		}
	}
	if dragon {
		return "Sảnh rồng"
	}
	pairs := 0
	for _, x := range g {
		pairs += len(x) / 2
	}
	if pairs >= 6 {
		return "6 đôi"
	}
	run := 0
	for r := 0; r < 12; r++ {
		if len(g[r]) >= 2 {
			run++
		} else {
			run = 0
		}
		if run >= 5 {
			return "5 đôi thông"
		}
	}
	return ""
}

func choose2(cs []Card) [][]Card {
	var out [][]Card
	for i := range cs {
		for j := i + 1; j < len(cs); j++ {
			out = append(out, []Card{cs[i], cs[j]})
		}
	}
	return out
}

// AllCombos liệt kê các bộ có thể đánh từ một tay bài.
// Với sảnh / đôi thông: các rank thấp dùng lá nhỏ nhất, chỉ rank cao nhất thử mọi lựa chọn.
func AllCombos(hand []Card) []*Combo {
	g := GroupByRank(hand)
	var out []*Combo
	push := func(cards []Card) { out = append(out, GetCombo(slices.Clone(cards))) }

	for r := 0; r < 13; r++ {
		cs := g[r]
		for _, c := range cs {
			push([]Card{c})
		}
		for _, p := range choose2(cs) {
			push(p)
		}
		switch len(cs) {
		case 3:
			push(cs)
		case 4:
			for skip := range cs {
				push(slices.Delete(slices.Clone(cs), skip, skip+1))
			}
			push(cs)
		}
	}

	for start := 0; start < 12; start++ {
		var body []Card
		for end := start; end < 12 && len(g[end]) > 0; end++ {
			if end-start >= 2 {
				for _, top := range g[end] {
					push(append(slices.Clone(body), top))
				}
			}
			body = append(body, g[end][0])
		}
	}

	for start := 0; start < 12; start++ {
		var body []Card
		for end := start; end < 12 && len(g[end]) >= 2; end++ {
			if end-start >= 2 {
				for _, p := range choose2(g[end]) {
					push(append(slices.Clone(body), p...))
				}
			}
			body = append(body, g[end][0], g[end][1])
		}
	}
	return out
}
