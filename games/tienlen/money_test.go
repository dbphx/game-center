package tienlen

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

// Người nhất về là ván dừng ngay; người còn heo / hàng đều thối, xếp hạng theo số lá rồi ghế sau người nhất.
func TestGameStopsAtFirstFinish(t *testing.T) {
	g := fixedGame(t, "3♠", "4♠ 2♥", "5♠ 6♠")
	mustPlay(t, g, 0, "3♠") // người 0 về nhất bằng 3♠
	if !g.Over {
		t.Fatal("người nhất về thì ván phải dừng ngay")
	}
	if !slices.Equal(g.Finished, []int{0, 1, 2}) {
		t.Fatalf("bằng lá thì ghế gần sau người nhất xếp trước: %v", g.Finished)
	}
	if err := g.Play(1, cs("4♠")); err == nil {
		t.Fatal("hết ván không được đánh tiếp")
	}
	want := Transfer{From: 1, To: 0, Units: 2, Reason: "thối 1 heo đỏ"}
	if len(g.Transfers) != 1 || g.Transfers[0] != want {
		t.Fatalf("muốn %+v, được %+v", want, g.Transfers)
	}
	// Tới bằng 3♠: đếm lá ×2 — mỗi người còn 2 lá trả 4; thối heo đỏ không nhân.
	if g.WinBy == nil || !g.WinBy.ThreeSpades || g.WinBy.Multiplier != 2 {
		t.Fatalf("phải tới bằng 3♠ ×2: %+v", g.WinBy)
	}
	if !slices.Equal(g.CardsLeft, []int{0, 2, 2}) {
		t.Fatalf("số lá bị tính sai: %v", g.CardsLeft)
	}
	if m := g.Money(); !slices.Equal(m, []int{8 + 2, -4 - 2, -4}) {
		t.Fatalf("tiền sai: %v", m)
	}
}

// Mọi người thua còn heo / hàng đều thối, mỗi người một khoản; người ít lá xếp trước.
func TestEveryLoserPaysLeftover(t *testing.T) {
	g := fixedGame(t, "6♠", "7♠ 7♣ 7♦ 7♥ 2♠", "8♠ 2♦", "9♠ 10♠ J♦")
	mustPlay(t, g, 0, "6♠")
	if !g.Over {
		t.Fatal("phải hết ván")
	}
	if !slices.Equal(g.Finished, []int{0, 2, 3, 1}) {
		t.Fatalf("xếp hạng theo số lá: %v", g.Finished)
	}
	want := []Transfer{
		{From: 1, To: 0, Units: 5, Reason: "thối 1 heo đen, 1 tứ quý"},
		{From: 2, To: 0, Units: 2, Reason: "thối 1 heo đỏ"},
	}
	if !slices.Equal(g.Transfers, want) {
		t.Fatalf("thối: muốn %+v, được %+v", want, g.Transfers)
	}
	m := g.Money()
	if !slices.Equal(m, []int{5 + 2 + 3 + 7, -5 - 5, -2 - 2, -3}) || sum(m) != 0 {
		t.Fatalf("tiền sai: %v", m)
	}
}

// Bằng lá thì xếp theo ghế đánh sau người nhất, vòng quanh bàn.
func TestRankTieBySeatAfterWinner(t *testing.T) {
	g := fixedGame(t, "9♠ 10♠", "5♠ 6♠", "4♥", "3♦ 3♣")
	g.Turn = 2
	mustPlay(t, g, 2, "4♥") // người 2 về nhất, ai cũng còn 2 lá
	if !g.Over || !slices.Equal(g.Finished, []int{2, 3, 0, 1}) {
		t.Fatalf("xếp hạng sai: %v", g.Finished)
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
	if !g.Over {
		t.Fatal("người nhất về thì ván phải dừng ngay")
	}
	if !slices.Equal(g.Finished, []int{3, 0, 1, 2}) {
		t.Fatalf("người bị cóng phải xếp bét: %v", g.Finished)
	}
	if len(g.Transfers) != 1 {
		t.Fatalf("chỉ người cóng có heo nên chỉ một khoản thối: %+v", g.Transfers)
	}
	if t2 := g.Transfers[0]; t2.From != 2 || t2.To != 3 || t2.Units != 1 {
		t.Fatalf("người bị cóng phải thối heo đen: %+v", t2)
	}
}

// Bàn 2 người: người thua vừa bị cóng vừa còn heo chỉ trả thối một lần.
func TestTwoPlayersCongPaysLeftoverOnce(t *testing.T) {
	thirteen := "3♣ 4♣ 5♣ 6♣ 7♣ 8♣ 9♣ 10♣ J♣ Q♣ K♣ A♣ 2♥"
	g := fixedGame(t, "3♦ 4♦", thirteen)
	mustPlay(t, g, 0, "3♦")
	mustPass(t, g, 1)
	mustPlay(t, g, 0, "4♦")
	if !g.Over || !slices.Equal(g.Cong, []int{1}) || !slices.Equal(g.Finished, []int{0, 1}) {
		t.Fatalf("hết ván, người 1 cóng: over %v cóng %v xếp %v", g.Over, g.Cong, g.Finished)
	}
	if len(g.Transfers) != 1 || g.Transfers[0] != (Transfer{From: 1, To: 0, Units: 2, Reason: "thối 1 heo đỏ"}) {
		t.Fatalf("thối đúng một lần: %+v", g.Transfers)
	}
	if m := g.Money(); !slices.Equal(m, []int{28, -28}) {
		t.Fatalf("cóng 26 + thối 2: %v", m)
	}
}

// Người nhất về bằng nước chặt heo: tính cả tiền chặt lẫn đếm lá, ván dừng ngay.
func TestWinningChopCountsBoth(t *testing.T) {
	g := fixedGame(t, "2♥ 4♠", "3♠ 3♣ 4♦ 4♥ 5♠ 5♣", "8♠ 9♠")
	mustPlay(t, g, 0, "2♥")
	mustPlay(t, g, 1, "3♠ 3♣ 4♦ 4♥ 5♠ 5♣") // chặt heo và hết bài
	if !g.Over || g.Finished[0] != 1 {
		t.Fatalf("người 1 về nhất, ván dừng: %v %v", g.Over, g.Finished)
	}
	if !slices.Contains(g.Transfers, Transfer{From: 0, To: 1, Units: 2, Reason: "chặt heo"}) {
		t.Fatalf("thiếu tiền chặt: %+v", g.Transfers)
	}
	// Có 3♠ trong bộ cuối nên đếm lá ×2: người 0 còn 1 lá, người 2 còn 2 lá.
	if m := g.Money(); !slices.Equal(m, []int{-2 - 2, 2 + 2 + 4, -4}) || sum(m) != 0 {
		t.Fatalf("tiền sai: %v", m)
	}
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// Nhất ăn tất, đếm lá: lúc người nhất về mỗi người còn 1 lá nên trả 1 mức; không có tiền nhì/ba/bét.
func TestWinnerTakesAll(t *testing.T) {
	cases := []struct {
		hands []string
		want  []int
	}{
		{[]string{"5♠", "6♠"}, []int{1, -1}},
		{[]string{"5♠", "6♠", "7♠"}, []int{2, -1, -1}},
		{[]string{"5♠", "6♠", "7♠", "8♠"}, []int{3, -1, -1, -1}},
	}
	for _, tc := range cases {
		g := fixedGame(t, tc.hands...)
		mustPlay(t, g, 0, "5♠") // người 0 về nhất (người khác có 1 lá nên không cóng), ván dừng ngay
		if !g.Over {
			t.Fatalf("%d người: phải hết ván", len(tc.hands))
		}
		if !slices.Equal(g.Delta, tc.want) || sum(g.Money()) != 0 {
			t.Fatalf("%d người: muốn %v, được %v", len(tc.hands), tc.want, g.Delta)
		}
	}
}

// Đếm lá lúc người nhất về: người 0, 1 còn 1 lá trả 1, người bị cóng trả 26; tổng luôn bằng 0.
func TestWinnerTakesAllWithCong(t *testing.T) {
	thirteen := "3♣ 4♣ 5♣ 6♣ 7♣ 8♣ 9♣ 10♣ J♣ Q♣ K♣ A♣ 2♠"
	g := fixedGame(t, "4♠ 6♥", "5♠ 7♠", thirteen, "9♥ 10♥")
	mustPlay(t, g, 0, "4♠")
	mustPlay(t, g, 1, "5♠")
	mustPass(t, g, 2)
	mustPlay(t, g, 3, "9♥")
	mustPass(t, g, 0)
	mustPass(t, g, 1)
	mustPlay(t, g, 3, "10♥") // người 3 về nhất (không có 3♠), người 2 cóng
	if g.WinBy == nil || g.WinBy.ThreeSpades || g.WinBy.Multiplier != 1 {
		t.Fatalf("không có 3♠ thì ×1: %+v", g.WinBy)
	}
	if !g.Over {
		t.Fatal("phải hết ván")
	}
	// Người 0, 1 còn 1 lá trả 1; người 2 cóng trả 26 → người 3 được 28. Thối: cóng thối heo đen 2♠ (1).
	if !slices.Equal(g.CardsLeft, []int{1, 1, 26, 0}) || !slices.Equal(g.Delta, []int{-1, -1, -26, 28}) {
		t.Fatalf("đếm lá sai: lá %v, tiền %v", g.CardsLeft, g.Delta)
	}
	m := g.Money()
	if !slices.Equal(m, []int{-1, -1, -27, 29}) || sum(m) != 0 {
		t.Fatalf("tổng tiền sai: %v", m)
	}
}

// Tới bằng bộ có 3♠ (đôi 3) thì ×2; tiền chặt không nhân.
func TestThreeSpadesPairDoubles(t *testing.T) {
	g := fixedGame(t, "3♠ 3♥ 2♦", "4♠ 4♣ 9♦", "5♦ 6♦")
	mustPlay(t, g, 0, "2♦")
	mustPass(t, g, 1)
	mustPass(t, g, 2) // người 0 được đánh tiếp
	mustPlay(t, g, 0, "3♠ 3♥")
	if g.WinBy == nil || !g.WinBy.ThreeSpades || g.WinBy.Multiplier != 2 {
		t.Fatalf("đôi có 3♠ phải ×2: %+v", g.WinBy)
	}
	if !g.Over || !slices.Equal(g.Finished, []int{0, 2, 1}) {
		t.Fatalf("phải hết ván, người ít lá xếp trước: %v", g.Finished)
	}
	// Lúc người 0 về: người 1 còn 3 lá, người 2 còn 2 lá → ×2 thành 6 và 4.
	if !slices.Equal(g.Delta, []int{10, -6, -4}) || sum(g.Money()) != 0 {
		t.Fatalf("×2 sai: delta %v, money %v", g.Delta, g.Money())
	}
}

// Tới trắng: mỗi người trả 13 lá × 2 = 26 mức.
func TestInstantWinTakesAll(t *testing.T) {
	g, _ := NewGame(4, rand.New(rand.NewPCG(1, 1)))
	for seed := uint64(0); seed < 200000; seed++ {
		g.rand = rand.New(rand.NewPCG(seed, 7))
		g.Start()
		if g.Instant == nil {
			continue
		}
		w := g.Instant.Player
		if len(g.Finished) != 4 || g.Finished[0] != w {
			t.Fatalf("tới trắng phải xếp đủ hạng: %v", g.Finished)
		}
		for q, d := range g.Delta {
			if (q == w && d != 78) || (q != w && d != -26) {
				t.Fatalf("tới trắng sai: %v", g.Delta)
			}
		}
		return
	}
	t.Skip("không gặp ván tới trắng nào")
}

// Đếm lá theo số lá thật còn trên tay lúc người nhất về, người về sau vẫn trả theo số lá lúc đó.
func TestCountCardsAtFirstFinish(t *testing.T) {
	g := fixedGame(t, "5♠", "6♠ 7♠ 8♠ 9♦", "J♠ K♥")
	mustPlay(t, g, 0, "5♠") // người 0 về nhất: người 1 còn 4 lá, người 2 còn 2 lá
	if !g.Over || !slices.Equal(g.Finished, []int{0, 2, 1}) {
		t.Fatalf("phải hết ván ngay: %v", g.Finished)
	}
	if !slices.Equal(g.CardsLeft, []int{0, 4, 2}) || !slices.Equal(g.Delta, []int{6, -4, -2}) || sum(g.Money()) != 0 {
		t.Fatalf("đếm lá sai: lá %v, tiền %v", g.CardsLeft, g.Delta)
	}
}
