package tienlen

import (
	"fmt"
	"strings"
)

// Giá các loại heo / hàng, tính bằng mức cược. Dùng cho cả tiền chặt lẫn tiền thối.
const (
	blackTwoUnits = 1 // heo đen ♠ ♣
	redTwoUnits   = 2 // heo đỏ ♦ ♥
	seq3Units     = 3 // 3 đôi thông
	quadUnits     = 4 // tứ quý
	seq4Units     = 6 // 4 đôi thông trở lên
)

func twoUnits(c Card) int {
	if Suit(c) >= 2 {
		return redTwoUnits
	}
	return blackTwoUnits
}

// ChopValue là số tiền người có bộ bài bị chặt phải trả.
func ChopValue(c *Combo) int {
	switch {
	case c.IsTwos() && (c.Type == Single || c.Type == Pair):
		units := 0
		for _, card := range c.Cards {
			units += twoUnits(card)
		}
		return units
	case c.Type == Quad:
		return quadUnits
	case c.Type == PairSeq && c.Length >= 4:
		return seq4Units
	case c.Type == PairSeq && c.Length == 3:
		return seq3Units
	}
	return 0
}

// Leftover tính tiền thối cho heo và hàng còn trên tay, kèm mô tả ("1 heo đỏ, 1 tứ quý").
func Leftover(hand []Card) (int, string) {
	g := GroupByRank(hand)
	units := 0
	var parts []string
	add := func(n, each int, name string) {
		if n > 0 {
			units += n * each
			parts = append(parts, fmt.Sprintf("%d %s", n, name))
		}
	}

	black, red := 0, 0
	for _, c := range g[Rank2] {
		if twoUnits(c) == redTwoUnits {
			red++
		} else {
			black++
		}
	}
	add(black, blackTwoUnits, "heo đen")
	add(red, redTwoUnits, "heo đỏ")

	quads := 0
	for r := range Rank2 {
		if len(g[r]) == 4 {
			quads++
		}
	}
	add(quads, quadUnits, "tứ quý")

	// Mỗi dãy đôi liên tiếp dài nhất tính là một bộ đôi thông.
	seq3, seq4, run := 0, 0, 0
	for r := 0; r <= Rank2; r++ {
		if r < Rank2 && len(g[r]) >= 2 {
			run++
			continue
		}
		switch {
		case run >= 4:
			seq4++
		case run == 3:
			seq3++
		}
		run = 0
	}
	add(seq3, seq3Units, "3 đôi thông")
	add(seq4, seq4Units, "4 đôi thông")

	return units, strings.Join(parts, ", ")
}
