package tienlen

import (
	"slices"
	"sort"
)

// Context là thông tin bot cần để chọn nước đi.
type Context struct {
	Prev             *Combo // nil khi đang được đánh tự do
	MustInclude      Card   // -1 khi không bắt buộc
	MinOpponentCards int
}

// Chi phí từ mức này trở lên nghĩa là bot chỉ đánh khi gấp, còn không thì bỏ lượt.
const reluctant = 1000

// breakPenalty phạt khi bộ bài làm vỡ đôi / sám / tứ quý đang có.
func breakPenalty(c *Combo, groups [13][]Card) int {
	used := map[int]int{}
	for _, card := range c.Cards {
		used[Rank(card)]++
	}
	p := 0
	for rank, n := range used {
		if size := len(groups[rank]); size > n {
			if size >= 3 {
				p += 12
			} else {
				p += 6
			}
		}
	}
	return p
}

func hasTwo(c *Combo) bool {
	return slices.ContainsFunc(c.Cards, func(card Card) bool { return Rank(card) == Rank2 })
}

// ChooseMove chọn nước đi theo heuristic: ưu tiên xả bài nhỏ, giữ heo và bom cho lúc cần.
// Trả về nil để bỏ lượt.
func ChooseMove(hand []Card, ctx Context) []Card {
	groups := GroupByRank(hand)
	var combos []*Combo
	for _, c := range AllCombos(hand) {
		if ctx.MustInclude >= 0 && !slices.Contains(c.Cards, ctx.MustInclude) {
			continue
		}
		if ctx.Prev != nil && !CanBeat(c, ctx.Prev) {
			continue
		}
		if len(c.Cards) == len(hand) {
			return c.Cards
		}
		combos = append(combos, c)
	}
	if len(combos) == 0 {
		return nil
	}

	urgent := ctx.MinOpponentCards <= 3 || len(hand) <= 4
	cost := func(c *Combo) int {
		if ctx.Prev == nil {
			return leadCost(c, ctx, groups)
		}
		return followCost(c, ctx, groups, urgent)
	}
	sort.SliceStable(combos, func(i, j int) bool { return cost(combos[i]) < cost(combos[j]) })
	best := combos[0]

	if ctx.Prev != nil && !urgent && cost(best) >= reluctant {
		return nil
	}
	return best.Cards
}

func leadCost(c *Combo, ctx Context, groups [13][]Card) int {
	cost := Rank(c.Cards[0])*10 - (len(c.Cards)-1)*4 + breakPenalty(c, groups)
	if hasTwo(c) {
		cost += 200
	}
	if c.IsBomb() {
		cost += 200
	}
	// Đối thủ còn 1 lá thì tránh đánh rác nhỏ.
	if ctx.MinOpponentCards == 1 && c.Type == Single {
		cost += 100 - Rank(c.Top)*5
	}
	return cost
}

func followCost(c *Combo, ctx Context, groups [13][]Card, urgent bool) int {
	cost := Rank(c.Top)*10 + breakPenalty(c, groups)
	prevIsTwos := ctx.Prev.IsTwos() || ctx.Prev.IsBomb()
	switch {
	case c.IsBomb():
		if prevIsTwos {
			cost += 20
		} else {
			cost += reluctant
		}
	case hasTwo(c):
		prevHigh := Rank(ctx.Prev.Top) >= 9 // từ Q trở lên mới đáng ra heo
		if urgent || prevHigh {
			cost += 30
		} else {
			cost += reluctant
		}
	}
	return cost
}
