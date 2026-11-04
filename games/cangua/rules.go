// Package cangua là Cờ Cá Ngựa online của DCity: 2–4 người (hoặc máy), mỗi người 4 ngựa,
// đổ 2 xúc xắc chạy hết một vòng bàn rồi lên chuồng đích. Cược và XP vào ví chung của hub.
package cangua

import (
	mrand "math/rand/v2"
	"slices"
)

// Bàn cờ: đường chung Track ô, mỗi màu một cạnh dài Side ô. Màu i xuất quân ở ô Side·i,
// cửa chuồng đích là ô ngay trước ô xuất phát (đi đủ một vòng). Chuồng đích có FinishSteps bậc.
const (
	Side        = 14
	Track       = 4 * Side // 56 ô
	FinishSteps = 6
	Horses      = 4

	Home       = -1  // ngựa còn trong chuồng
	FinishBase = 100 // ngựa ở bậc đích b có vị trí FinishBase + b (1..6)
)

// Colors là màu theo thứ tự ghế (chiều kim đồng hồ).
var Colors = []string{"red", "blue", "green", "yellow"}

// Board là vị trí 4 ngựa của từng ghế: pos[seat][horse].
type Board [][Horses]int

func NewBoard(seats int) Board {
	b := make(Board, seats)
	for i := range b {
		b[i] = [Horses]int{Home, Home, Home, Home}
	}
	return b
}

func startOf(seat int) int { return Side * seat }

// rel là số ô ngựa đã đi tính từ ô xuất phát của màu (0 = vừa ra, Track-1 = đứng ở cửa đích).
func rel(seat, pos int) int { return (pos - startOf(seat) + Track) % Track }

func abs(seat, r int) int { return (startOf(seat) + r) % Track }

// at trả ghế và ngựa đang đứng ở ô đường chung pos; seat = -1 nếu ô trống.
func (b Board) at(pos int) (seat, horse int) {
	for s := range b {
		for h, p := range b[s] {
			if p == pos {
				return s, h
			}
		}
	}
	return -1, -1
}

// Dice là số của 2 viên xúc xắc vừa đổ.
type Dice [2]int

func (d Dice) Sum() int     { return d[0] + d[1] }
func (d Dice) Double() bool { return d[0] == d[1] }

// has: có viên nào ra đúng số n.
func (d Dice) has(n int) bool { return d[0] == n || d[1] == n }

// canLeaveHome: có viên 1 hoặc 6, hoặc đổ đôi thì được xuất quân.
func (d Dice) canLeaveHome() bool { return d.has(1) || d.has(6) || d.Double() }

// Move là một nước đi hợp lệ: ngựa horse của ghế seat từ From tới To, có thể đá ngựa (KickSeat, KickHorse).
type Move struct {
	Seat, Horse int
	From, To    int
	KickSeat    int // -1 nếu không đá
	KickHorse   int
	Short       bool // đổ 1 + 1 và chọn đi 1 ô thay vì 2 ô
}

// land đặt ngựa xuống ô m.To của đường chung: không chồng lên ngựa mình, gặp đối thủ thì đá.
func (b Board) land(m Move) (Move, bool) {
	os, oh := b.at(m.To)
	switch {
	case os == m.Seat:
		return m, false
	case os >= 0:
		m.KickSeat, m.KickHorse = os, oh
	}
	return m, true
}

// walk đi n ô trên đường chung: không vượt ngựa nào, số dư thì dừng ngay ở cửa đích.
func (b Board) walk(m Move, n int) (Move, bool) {
	r := rel(m.Seat, m.From)
	n = min(n, Track-1-r)
	for s := 1; s < n; s++ {
		if os, _ := b.at(abs(m.Seat, r+s)); os >= 0 {
			return m, false
		}
	}
	m.To = abs(m.Seat, r+n)
	return b.land(m)
}

// enter lên chuồng đích từ cửa: tới bậc k, không vượt ngựa mình.
func (b Board) enter(m Move, k int) (Move, bool) {
	for s := 1; s <= k; s++ {
		if slices.Contains(b[m.Seat][:], FinishBase+s) {
			return m, false
		}
	}
	m.To = FinishBase + k
	return m, true
}

// horseMoves liệt kê nước đi của một ngựa: thường một nước, đổ 1 + 1 trên đường chung thì có thể có hai.
func (b Board) horseMoves(seat, horse int, d Dice) []Move {
	p := b[seat][horse]
	m := Move{Seat: seat, Horse: horse, From: p, KickSeat: -1, KickHorse: -1}
	switch {
	case p == Home: // ra ô xuất phát rồi dừng, viên còn lại bỏ
		if !d.canLeaveHome() {
			return nil
		}
		m.To = startOf(seat)
		if mv, ok := b.land(m); ok {
			return []Move{mv}
		}
		return nil
	case p >= FinishBase: // trong chuồng đích: có viên nào bằng bậc kế tiếp thì lên
		next := p - FinishBase + 1
		if next > FinishSteps || !d.has(next) || slices.Contains(b[seat][:], FinishBase+next) {
			return nil
		}
		m.To = FinishBase + next
		return []Move{m}
	case rel(seat, p) == Track-1: // ở cửa đích: lên bậc bằng viên lớn hơn, bị chặn thì dùng viên nhỏ
		for _, k := range []int{max(d[0], d[1]), min(d[0], d[1])} {
			if mv, ok := b.enter(m, k); ok {
				return []Move{mv}
			}
		}
		return nil
	}
	var out []Move
	long, okLong := b.walk(m, d.Sum())
	if okLong {
		out = append(out, long)
	}
	if d == (Dice{1, 1}) { // gỡ kẹt: tổng nhỏ nhất là 2, riêng 1 + 1 được chọn đi 1 ô
		if short, ok := b.walk(m, 1); ok && (!okLong || short.To != long.To) {
			short.Short = true
			out = append(out, short)
		}
	}
	return out
}

// Moves liệt kê các nước đi hợp lệ của ghế seat với 2 viên d.
func (b Board) Moves(seat int, d Dice) []Move {
	var out []Move
	for h := range Horses {
		out = append(out, b.horseMoves(seat, h, d)...)
	}
	return out
}

// Apply thực hiện nước đi (ngựa bị đá về chuồng).
func (b Board) Apply(m Move) {
	if m.KickSeat >= 0 {
		b[m.KickSeat][m.KickHorse] = Home
	}
	b[m.Seat][m.Horse] = m.To
}

// Won: đủ 4 ngựa trong chuồng đích.
func (b Board) Won(seat int) bool {
	for _, p := range b[seat] {
		if p < FinishBase {
			return false
		}
	}
	return true
}

// ---------- Máy ----------

const (
	BotEasy   = "easy"
	BotNormal = "normal"
	BotHard   = "hard"
)

var BotLevels = []string{BotEasy, BotNormal, BotHard}

// threatened: ô pos (đường chung) nằm trong tầm 2–12 ô (tổng 2 viên) phía trước một ngựa đối thủ.
func (b Board) threatened(seat, pos int) bool {
	for s := range b {
		if s == seat {
			continue
		}
		for _, p := range b[s] {
			if p < 0 || p >= FinishBase {
				continue
			}
			if d := (pos - p + Track) % Track; d >= 2 && d <= 12 {
				return true
			}
		}
	}
	return false
}

// score chấm một nước: đá ngựa > lên đích > xuất quân > tiến ngựa dẫn đầu; máy khó tránh chỗ dễ bị đá.
func (b Board) score(m Move, level string) int {
	s := 0
	if m.KickSeat >= 0 {
		s += 1000
	}
	switch {
	case m.To >= FinishBase:
		s += 800 + (m.To - FinishBase)
	case m.From == Home:
		s += 600
	default:
		s += rel(m.Seat, m.To) * 4 // ưu tiên ngựa đi xa nhất
	}
	if level == BotHard && m.To < FinishBase {
		after := b.clone()
		after.Apply(m)
		if after.threatened(m.Seat, m.To) {
			s -= 400
		}
		if m.From >= 0 && m.From < FinishBase && b.threatened(m.Seat, m.From) {
			s += 300 // chạy thoát khỏi tầm đá
		}
	}
	return s
}

func (b Board) clone() Board { return slices.Clone(b) }

// BestMove chọn nước đi cho máy (hoặc đi thay người hết giờ); ok = false nếu không có nước nào.
func BestMove(b Board, seat int, d Dice, level string, rng *mrand.Rand) (Move, bool) {
	moves := b.Moves(seat, d)
	if len(moves) == 0 {
		return Move{}, false
	}
	if level == BotEasy && rng.IntN(3) == 0 { // máy dễ thỉnh thoảng đi bừa
		return moves[rng.IntN(len(moves))], true
	}
	best := moves[0]
	bestScore := b.score(best, level)
	for _, m := range moves[1:] {
		if s := b.score(m, level); s > bestScore {
			best, bestScore = m, s
		}
	}
	return best, true
}
