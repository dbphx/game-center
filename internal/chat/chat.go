// Package chat là phần dùng chung cho chat trong DCity (quảng trường, các bàn casino):
// chuẩn hoá tin nhắn, lọc từ tục, chống spam và giữ lịch sử ngắn.
package chat

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxRunes = 120                     // độ dài tối đa một tin
	MinGap   = 1500 * time.Millisecond // tối thiểu giữa hai tin của một người
	Window   = 10 * time.Second        // cửa sổ đếm tin
	MaxInWin = 5                       // tối đa số tin trong cửa sổ
)

// ErrTooFast là lỗi khi một người chat dày quá.
var ErrTooFast = errors.New("Bạn chat nhanh quá")

// Lọc vài từ tục phổ biến: so theo từng từ (không theo chuỗi con) để không che nhầm
// từ thường như "lon bia", "dâm bụt"; từ bị lọc thay bằng ***.
var banned = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`địt đụ đjt djt lồn cặc buồi đéo đĩ
		đm dm đmm dmm đcm dcm cđm cdm đkm dkm vcl vkl vcc clm cmm loz
		fuck fucking shit bitch`) {
		banned[w] = true
	}
}

var wordRe = regexp.MustCompile(`[\p{L}\p{N}]+`)

// Clean thay các từ tục trong text bằng ***.
func Clean(text string) string {
	return wordRe.ReplaceAllStringFunc(text, func(w string) string {
		if banned[strings.ToLower(w)] {
			return "***"
		}
		return w
	})
}

// Normalize gộp khoảng trắng và cắt còn MaxRunes ký tự.
func Normalize(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(text) > MaxRunes {
		text = string([]rune(text)[:MaxRunes])
	}
	return text
}

// Prepare chuẩn hoá rồi lọc tin; trả "" nếu tin rỗng.
func Prepare(text string) string { return Clean(Normalize(text)) }

// Limiter chống spam cho một người. Không tự khoá: người gọi giữ mutex của mình.
type Limiter struct{ times []time.Time }

// Allow cho biết tin gửi lúc now có được nhận không (nếu được thì ghi lại).
func (l *Limiter) Allow(now time.Time) bool {
	recent := l.times[:0]
	for _, t := range l.times {
		if now.Sub(t) < Window {
			recent = append(recent, t)
		}
	}
	l.times = recent
	if n := len(recent); n > 0 && now.Sub(recent[n-1]) < MinGap || n >= MaxInWin {
		return false
	}
	l.times = append(l.times, now)
	return true
}

// Line là một tin trong lịch sử bàn. Seq tăng dần trong mỗi bàn (để client poll chỉ hiện tin mới).
type Line struct {
	Seq  int64  `json:"seq"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Text string `json:"text"`
	At   int64  `json:"at"` // unix ms
}

// Log giữ n tin gần nhất. Không tự khoá.
type Log struct {
	Max   int
	lines []Line
	seq   int64
}

// Add thêm tin (gán Seq) và trả tin đã thêm.
func (g *Log) Add(l Line) Line {
	g.seq++
	l.Seq = g.seq
	g.lines = append(g.lines, l)
	max := g.Max
	if max <= 0 {
		max = 20
	}
	if len(g.lines) > max {
		g.lines = append([]Line(nil), g.lines[len(g.lines)-max:]...)
	}
	return l
}

// List trả bản sao các tin, cũ trước; luôn khác nil.
func (g *Log) List() []Line {
	out := make([]Line, len(g.lines))
	copy(out, g.lines)
	return out
}
