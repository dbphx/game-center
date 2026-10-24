package game

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// fixedGame tạo ván với tay bài cho sẵn, người 0 đánh trước, không ràng buộc ván đầu.
func fixedGame(t *testing.T, hands ...string) *Game {
	t.Helper()
	g, err := NewGame(len(hands), rand.New(rand.NewPCG(1, 1)))
	if err != nil {
		t.Fatal(err)
	}
	g.Start()
	g.Over, g.Instant, g.Finished, g.Transfers = false, nil, nil, nil
	g.Delta = make([]int, len(hands))
	for p, h := range hands {
		g.Hands[p] = slices.Sorted(slices.Values(cs(h)))
	}
	g.Turn, g.MustInclude, g.LastPlay = 0, -1, nil
	return g
}

func mustPlay(t *testing.T, g *Game, p int, cards string) {
	t.Helper()
	if err := g.Play(p, cs(cards)); err != nil {
		t.Fatalf("người %d đánh %s: %v", p, cards, err)
	}
}

func mustPass(t *testing.T, g *Game, p int) {
	t.Helper()
	if err := g.Pass(p); err != nil {
		t.Fatalf("người %d bỏ lượt: %v", p, err)
	}
}

func TestLeftover(t *testing.T) {
	cases := []struct {
		hand  string
		units int
		desc  string
	}{
		{"3♠ 5♦ 9♥", 0, ""},
		{"2♠ 2♥", 3, "1 heo đen, 1 heo đỏ"},
		{"7♠ 7♣ 7♦ 7♥ 2♣", 5, "1 heo đen, 1 tứ quý"},
		{"3♠ 3♣ 4♦ 4♥ 5♠ 5♣ 9♥", 3, "1 3 đôi thông"},
		{"3♠ 3♣ 4♦ 4♥ 5♠ 5♣ 6♠ 6♣", 6, "1 4 đôi thông"},
		{"Q♠ Q♣ K♦ K♥ A♠ A♣ 2♠ 2♣", 5, "2 heo đen, 1 3 đôi thông"},
	}
	for _, tc := range cases {
		units, desc := Leftover(cs(tc.hand))
		if units != tc.units || desc != tc.desc {
			t.Errorf("%s: muốn %d %q, được %d %q", tc.hand, tc.units, tc.desc, units, desc)
		}
	}
}

func TestChopTwoPays(t *testing.T) {
	g := fixedGame(t, "2♥ 4♠", "3♠ 3♣ 4♦ 4♥ 5♠ 5♣ 9♦", "8♠ 9♠")
	mustPlay(t, g, 0, "2♥")
	mustPlay(t, g, 1, "3♠ 3♣ 4♦ 4♥ 5♠ 5♣")
	want := Transfer{From: 0, To: 1, Units: 2, Reason: "chặt heo"}
	if len(g.Transfers) != 1 || g.Transfers[0] != want {
		t.Fatalf("muốn %+v, được %+v", want, g.Transfers)
	}
	if ev := g.History[len(g.History)-1]; !ev.Chop || ev.Pay == nil {
		t.Fatal("sự kiện chặt phải có tiền")
	}
}

// Heo bị 3 đôi thông chặt, rồi tứ quý chặt chồng: người có 3 đôi thông trả cả tiền heo đã ăn.
func TestChopChain(t *testing.T) {
	g := fixedGame(t,
		"2♦ 3♥",
		"3♠ 3♣ 4♦ 4♥ 5♠ 5♣ 6♠",
		"9♠ 9♣ 9♦ 9♥ 7♠",
	)
	mustPlay(t, g, 0, "2♦")
	mustPlay(t, g, 1, "3♠ 3♣ 4♦ 4♥ 5♠ 5♣")
	mustPlay(t, g, 2, "9♠ 9♣ 9♦ 9♥")
	if len(g.Transfers) != 2 {
		t.Fatalf("muốn 2 khoản, được %+v", g.Transfers)
	}
	if got := g.Transfers[1]; got.From != 1 || got.To != 2 || got.Units != 2+3 || got.Reason != "chặt chồng" {
		t.Fatalf("chặt chồng sai: %+v", got)
	}
	m := g.Money()
	if m[0] != -2 || m[1] != -3 || m[2] != 5 {
		t.Fatalf("tiền chặt sai: %v", m)
	}
}

func TestLastPlayerPaysLeftover(t *testing.T) {
	g := fixedGame(t, "3♠", "4♠ 2♥", "5♠ 6♠")
	mustPlay(t, g, 0, "3♠") // người 0 về nhất
	mustPlay(t, g, 1, "4♠")
	mustPlay(t, g, 2, "5♠")
	mustPass(t, g, 1)
	mustPlay(t, g, 2, "6♠") // người 2 về nhì, người 1 bét còn heo đỏ
	if !g.Over || !slices.Equal(g.Finished, []int{0, 2, 1}) {
		t.Fatalf("xếp hạng sai: %v", g.Finished)
	}
	want := Transfer{From: 1, To: 0, Units: 2, Reason: "thối 1 heo đỏ"}
	if len(g.Transfers) != 1 || g.Transfers[0] != want {
		t.Fatalf("muốn %+v, được %+v", want, g.Transfers)
	}
	if m := g.Money(); !slices.Equal(m, []int{2 + 2, -2 - 2, 0}) {
		t.Fatalf("tiền sai: %v", m)
	}
}

func TestCong(t *testing.T) {
	thirteen := "3♣ 4♣ 5♣ 6♣ 7♣ 8♣ 9♣ 10♣ J♣ Q♣ K♣ A♣ 2♠" // chưa đánh lá nào
	g := fixedGame(t, "3♠ 4♠", "5♠ 6♠", thirteen, "9♥ 10♥")
	mustPlay(t, g, 0, "3♠")
	mustPlay(t, g, 1, "5♠")
	mustPass(t, g, 2)
	mustPlay(t, g, 3, "9♥")
	mustPass(t, g, 0)
	mustPass(t, g, 1)
	mustPlay(t, g, 3, "10♥") // người 3 về nhất, người 2 vẫn còn 13 lá → cóng
	if !slices.Equal(g.Cong, []int{2}) {
		t.Fatalf("người 2 phải bị cóng, được %v", g.Cong)
	}
	if g.Over || g.Turn == 2 {
		t.Fatal("ván phải tiếp tục và bỏ qua người bị cóng")
	}
	mustPass(t, g, 0)
	mustPass(t, g, 1) // hết vòng, người 3 đã về nên người 0 được đánh
	if g.Turn != 0 || g.LastPlay != nil {
		t.Fatalf("người 0 phải được đánh tự do, lượt đang là %d", g.Turn)
	}
	mustPlay(t, g, 0, "4♠")
	if !g.Over {
		t.Fatal("chỉ còn 1 người thì hết ván")
	}
	if g.Finished[3] != 2 {
		t.Fatalf("người bị cóng phải xếp bét: %v", g.Finished)
	}
	if t2 := g.Transfers[len(g.Transfers)-1]; t2.From != 2 || t2.To != 3 || t2.Units != 1 {
		t.Fatalf("người bị cóng phải thối heo đen: %+v", t2)
	}
}
