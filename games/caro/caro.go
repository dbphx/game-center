// Package caro là game Cờ Caro (gomoku) 15×15 của DCity: luật thắng, máy chơi và bàn online qua WebSocket.
package caro

import (
	"math/rand/v2"
	"slices"
	"strings"
)

const (
	Size  = 15
	cells = Size * Size

	Empty = '.'
	X     = 'x'
	O     = 'o'

	// Luật thắng của bàn.
	RuleFree   = "free"   // 5 quân liên tiếp trở lên là thắng
	RuleBlock2 = "block2" // luật Việt Nam: đường đúng 5 quân bị quân đối thủ chặn cả 2 đầu thì không tính; 6+ quân vẫn thắng.
	// Mép bàn KHÔNG tính là bị chặn, chỉ quân đối thủ mới chặn.
)

// Rules là các luật chủ bàn được chọn.
var Rules = []string{RuleBlock2, RuleFree}

// Board là bàn cờ, ô (x, y) ở vị trí y*Size+x.
type Board [cells]byte

func NewBoard() Board {
	var b Board
	for i := range b {
		b[i] = Empty
	}
	return b
}

func (b *Board) String() string { return string(b[:]) }

func inside(x, y int) bool { return x >= 0 && y >= 0 && x < Size && y < Size }

func (b *Board) at(x, y int) byte {
	if !inside(x, y) {
		return 0
	}
	return b[y*Size+x]
}

func (b *Board) Full() bool { return !strings.ContainsRune(b.String(), Empty) }

func other(m byte) byte {
	if m == X {
		return O
	}
	return X
}

// Bốn hướng: ngang, dọc, chéo xuống, chéo lên.
var dirs = [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}}

// WinLine trả các ô của đường thắng đi qua (x, y) theo luật rule, nil nếu nước đó chưa thắng.
// Quân ở (x, y) phải đã được đặt.
func (b *Board) WinLine(x, y int, rule string) [][2]int {
	m := b.at(x, y)
	if m != X && m != O {
		return nil
	}
	for _, d := range dirs {
		sx, sy := x, y
		for b.at(sx-d[0], sy-d[1]) == m {
			sx, sy = sx-d[0], sy-d[1]
		}
		var line [][2]int
		ex, ey := sx, sy
		for b.at(ex, ey) == m {
			line = append(line, [2]int{ex, ey})
			ex, ey = ex+d[0], ey+d[1]
		}
		if len(line) < 5 {
			continue
		}
		if rule == RuleBlock2 && len(line) == 5 {
			opp := other(m)
			if b.at(sx-d[0], sy-d[1]) == opp && b.at(ex, ey) == opp {
				continue // đúng 5 mà bị chặn hai đầu: không tính
			}
		}
		return line
	}
	return nil
}

// ---------- Máy chơi ----------

// Bậc khó của máy.
const (
	BotEasy   = "easy"
	BotNormal = "normal"
	BotHard   = "hard"
)

var BotLevels = []string{BotEasy, BotNormal, BotHard}

// Điểm của một thế cờ khi đặt quân: theo độ dài chuỗi và số đầu còn mở.
func shapeScore(n, open int) int {
	switch {
	case n >= 5:
		return 10_000_000
	case open == 0:
		return 0
	case n == 4 && open == 2:
		return 100_000
	case n == 4:
		return 10_000
	case n == 3 && open == 2:
		return 5_000
	case n == 3:
		return 500
	case n == 2 && open == 2:
		return 200
	case n == 2:
		return 50
	}
	return 10
}

// lineScore chấm điểm nếu m đặt ở (x, y): cộng điểm thế cờ của 4 hướng.
func (b *Board) lineScore(x, y int, m byte) int {
	total := 0
	for _, d := range dirs {
		n, open := 1, 0
		for _, s := range [2]int{1, -1} {
			cx, cy := x+s*d[0], y+s*d[1]
			for b.at(cx, cy) == m {
				n++
				cx, cy = cx+s*d[0], cy+s*d[1]
			}
			if b.at(cx, cy) == Empty {
				open++
			}
		}
		total += shapeScore(n, open)
	}
	return total
}

// candidates là các ô trống gần quân đã đặt (trong bán kính 2); bàn trống thì là ô giữa.
func (b *Board) candidates() [][2]int {
	var out [][2]int
	for y := range Size {
		for x := range Size {
			if b.at(x, y) != Empty {
				continue
			}
			near := false
			for dy := -2; dy <= 2 && !near; dy++ {
				for dx := -2; dx <= 2; dx++ {
					if c := b.at(x+dx, y+dy); c == X || c == O {
						near = true
						break
					}
				}
			}
			if near {
				out = append(out, [2]int{x, y})
			}
		}
	}
	if len(out) == 0 && b.at(Size/2, Size/2) == Empty {
		out = append(out, [2]int{Size / 2, Size / 2})
	}
	return out
}

// wins cho biết m đặt ở (x, y) có thắng ngay theo luật rule không.
func (b *Board) wins(x, y int, m byte, rule string) bool {
	b[y*Size+x] = m
	ok := b.WinLine(x, y, rule) != nil
	b[y*Size+x] = Empty
	return ok
}

type scored struct {
	pos   [2]int
	score int
}

// ranked chấm điểm các ô ứng viên cho m: tấn công cộng phòng thủ.
func (b *Board) ranked(m byte) []scored {
	opp := other(m)
	var out []scored
	for _, c := range b.candidates() {
		atk := b.lineScore(c[0], c[1], m)
		def := b.lineScore(c[0], c[1], opp)
		out = append(out, scored{c, atk*11/10 + def})
	}
	slices.SortFunc(out, func(a, b scored) int { return b.score - a.score })
	return out
}

// BestMove chọn nước cho m. Thứ tự ưu tiên: thắng ngay → chặn nước thắng của đối thủ → chấm điểm.
// easy chọn ngẫu nhiên trong vài nước tốt, hard nhìn thêm nước đáp trả của đối thủ.
func BestMove(b Board, m byte, rule, level string, rng *rand.Rand) (int, int, bool) {
	cand := b.candidates()
	if len(cand) == 0 {
		return 0, 0, false
	}
	opp := other(m)
	for _, c := range cand {
		if b.wins(c[0], c[1], m, rule) {
			return c[0], c[1], true
		}
	}
	if level != BotEasy || rng.IntN(4) > 0 { // máy dễ thỉnh thoảng quên chặn
		for _, c := range cand {
			if b.wins(c[0], c[1], opp, rule) {
				return c[0], c[1], true
			}
		}
	}
	list := b.ranked(m)
	switch level {
	case BotEasy:
		k := min(len(list), 5)
		p := list[rng.IntN(k)].pos
		return p[0], p[1], true
	case BotHard:
		best, bestVal := list[0].pos, -1<<62
		for _, s := range list[:min(len(list), 8)] {
			b[s.pos[1]*Size+s.pos[0]] = m
			reply := 0
			if r := b.ranked(opp); len(r) > 0 {
				reply = r[0].score
			}
			b[s.pos[1]*Size+s.pos[0]] = Empty
			if v := s.score - reply*9/10; v > bestVal {
				best, bestVal = s.pos, v
			}
		}
		return best[0], best[1], true
	}
	p := list[0].pos
	return p[0], p[1], true
}
