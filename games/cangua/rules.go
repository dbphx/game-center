// Package cangua là Cờ Cá Ngựa online của DCity: 2–4 người (hoặc máy), mỗi người 4 ngựa,
// đổ xúc xắc chạy hết một vòng bàn rồi lên chuồng đích. Cược và XP vào ví chung của hub.
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

// Move là một nước đi hợp lệ: ngựa horse của ghế seat từ From tới To, có thể đá ngựa (KickSeat, KickHorse).
type Move struct {
	Seat, Horse int
	From, To    int
	KickSeat    int // -1 nếu không đá
	KickHorse   int
}

// target tính nước đi của một ngựa với số xúc xắc d; ok = false nếu không đi được.
func (b Board) target(seat, horse, d int) (Move, bool) {
	p := b[seat][horse]
	m := Move{Seat: seat, Horse: horse, From: p, KickSeat: -1, KickHorse: -1}
	switch {
	case p == Home: // chỉ ra 1 hoặc 6 mới được xuất quân
		if d != 1 && d != 6 {
			return m, false
		}
		m.To = startOf(seat)
	case p >= FinishBase: // trong chuồng đích: chỉ lên đúng bậc kế tiếp
		step := p - FinishBase
		if d != step+1 || step+1 > FinishSteps || slices.Contains(b[seat][:], FinishBase+step+1) {
			return m, false
		}
		m.To = FinishBase + step + 1
		return m, true
	default:
		r := rel(seat, p)
		if r == Track-1 { // đứng ở cửa đích: đổ k lên bậc k, không vượt ngựa mình
			for s := 1; s <= d; s++ {
				if slices.Contains(b[seat][:], FinishBase+s) {
					return m, false
				}
			}
			m.To = FinishBase + d
			return m, true
		}
		if r+d > Track-1 { // phải dừng đúng cửa, không đi quá
			return m, false
		}
		for s := 1; s < d; s++ { // không được vượt qua ngựa nào trên đường
			if os, _ := b.at(abs(seat, r+s)); os >= 0 {
				return m, false
			}
		}
		m.To = abs(seat, r+d)
	}
	os, oh := b.at(m.To)
	switch {
	case os == seat:
		return m, false // không đứng chồng lên ngựa mình
	case os >= 0:
		m.KickSeat, m.KickHorse = os, oh
	}
	return m, true
}

// Moves liệt kê các nước đi hợp lệ của ghế seat với số d.
func (b Board) Moves(seat, d int) []Move {
	var out []Move
	for h := range Horses {
		if m, ok := b.target(seat, h, d); ok {
			out = append(out, m)
		}
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

// threatened: ô pos (đường chung) nằm trong tầm 1–6 ô phía trước một ngựa đối thủ.
func (b Board) threatened(seat, pos int) bool {
	for s := range b {
		if s == seat {
			continue
		}
		for _, p := range b[s] {
			if p < 0 || p >= FinishBase {
				continue
			}
			if d := (pos - p + Track) % Track; d >= 1 && d <= 6 {
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
func BestMove(b Board, seat, d int, level string, rng *mrand.Rand) (Move, bool) {
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
