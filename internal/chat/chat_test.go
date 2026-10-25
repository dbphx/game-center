package chat

import (
	"strings"
	"testing"
	"time"
)

func TestPrepare(t *testing.T) {
	if got := Prepare("  chào   cả\tnhà  "); got != "chào cả nhà" {
		t.Fatalf("gộp khoảng trắng sai: %q", got)
	}
	if got := Prepare("đm lon bia dâm bụt"); got != "*** lon bia dâm bụt" {
		t.Fatalf("lọc từ tục sai: %q", got)
	}
	long := strings.Repeat("ă", 200)
	if n := len([]rune(Prepare(long))); n != MaxRunes {
		t.Fatalf("phải cắt còn %d ký tự, được %d", MaxRunes, n)
	}
	if Prepare("   ") != "" {
		t.Fatal("tin toàn khoảng trắng phải rỗng")
	}
}

func TestLimiter(t *testing.T) {
	var l Limiter
	now := time.Unix(1000, 0)
	if !l.Allow(now) {
		t.Fatal("tin đầu phải được nhận")
	}
	if l.Allow(now.Add(time.Second)) {
		t.Fatal("cách 1 giây phải bị chặn")
	}
	at := now
	for i := 1; i < MaxInWin; i++ {
		at = at.Add(1600 * time.Millisecond)
		if !l.Allow(at) {
			t.Fatalf("tin %d cách 1,6 giây phải được nhận", i+1)
		}
	}
	if l.Allow(at.Add(1600 * time.Millisecond)) {
		t.Fatal("tin thứ 6 trong 10 giây phải bị chặn")
	}
	if !l.Allow(now.Add(11 * time.Second)) {
		t.Fatal("qua cửa sổ 10 giây phải nhận lại")
	}
}

func TestLog(t *testing.T) {
	g := Log{Max: 3}
	for i := range 5 {
		g.Add(Line{Name: "A", Text: string(rune('a' + i))})
	}
	list := g.List()
	if len(list) != 3 || list[0].Text != "c" || list[2].Seq != 5 {
		t.Fatalf("log giữ sai: %+v", list)
	}
	if (&Log{}).List() == nil {
		t.Fatal("List phải khác nil")
	}
}
