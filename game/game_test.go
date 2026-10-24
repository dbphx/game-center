package game

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// c tạo lá bài từ tên, ví dụ c("3♠"), c("10♥").
func c(name string) Card {
	r := []rune(name)
	rank, suit := string(r[:len(r)-1]), string(r[len(r)-1:])
	return slices.Index(RankNames, rank)*4 + slices.Index(SuitNames, suit)
}

func cs(s string) []Card {
	var out []Card
	for _, f := range strings.Fields(s) {
		out = append(out, c(f))
	}
	return out
}

func combo(s string) *Combo { return GetCombo(cs(s)) }

func TestGetCombo(t *testing.T) {
	cases := map[string]ComboType{
		"3♠":                   Single,
		"5♣ 5♥":                Pair,
		"7♠ 7♣ 7♦":             Triple,
		"9♠ 9♣ 9♦ 9♥":          Quad,
		"3♠ 4♦ 5♥":             Straight,
		"10♠ J♦ Q♥ K♣ A♠":      Straight,
		"3♠ 3♣ 4♦ 4♥ 5♠ 5♣":    PairSeq,
		"Q♠ Q♣ K♦ K♥ A♠ A♣":    PairSeq,
		"2♠ 2♥":                Pair,
		"K♠ A♦ 2♥":             "", // sảnh không được có heo
		"3♠ 5♦ 6♥":             "",
		"3♠ 3♣ 4♦ 4♥":          "", // 2 đôi thông không phải bộ
		"J♠ Q♠ K♠ A♠ 2♠":       "",
		"3♠ 3♣ 4♦ 4♥ 5♠ 5♣ 5♥": "",
	}
	for s, want := range cases {
		got := combo(s)
		if want == "" {
			if got != nil {
				t.Errorf("%s: muốn không hợp lệ, được %s", s, got.Type)
			}
		} else if got == nil || got.Type != want {
			t.Errorf("%s: muốn %s, được %v", s, want, got)
		}
	}
}

func TestCanBeat(t *testing.T) {
	seq3 := combo("3♠ 3♣ 4♦ 4♥ 5♠ 5♣")
	seq4 := combo("6♠ 6♣ 7♦ 7♥ 8♠ 8♣ 9♠ 9♣")
	quad := combo("J♠ J♣ J♦ J♥")
	cases := []struct {
		a, b *Combo
		want bool
		note string
	}{
		{combo("3♥"), combo("3♠"), true, "cùng rank so chất"},
		{combo("3♠"), combo("3♥"), false, ""},
		{combo("2♠"), combo("A♥"), true, ""},
		{combo("4♠ 5♠ 6♥"), combo("4♣ 5♦ 6♦"), true, ""},
		{combo("4♠ 5♠ 6♠ 7♠"), combo("3♣ 4♦ 5♦"), false, "sảnh khác độ dài"},
		{combo("8♠ 8♣"), combo("7♠"), false, "khác loại"},
		{seq3, combo("2♥"), true, "3 đôi thông chặt heo"},
		{seq3, combo("2♠ 2♥"), false, "3 đôi thông không chặt đôi heo"},
		{quad, combo("2♥"), true, ""},
		{quad, combo("2♠ 2♥"), true, ""},
		{quad, seq3, true, ""},
		{seq4, quad, true, ""},
		{seq4, combo("2♠ 2♥"), true, ""},
		{quad, seq4, false, ""},
		{seq3, combo("A♥"), false, "đôi thông không chặt rác thường"},
		{combo("Q♠ Q♣ Q♦ Q♥"), quad, true, "tứ quý lớn đè tứ quý nhỏ"},
	}
	for _, tc := range cases {
		if got := CanBeat(tc.a, tc.b); got != tc.want {
			t.Errorf("%v đè %v: muốn %v (%s)", tc.a.Cards, tc.b.Cards, tc.want, tc.note)
		}
	}
}

func TestInstantWin(t *testing.T) {
	cases := map[string]string{
		"2♠ 2♣ 2♦ 2♥ 3♠ 4♠ 5♠ 6♠ 7♠ 8♠ 9♠ J♠ K♠":  "Tứ quý heo",
		"3♠ 4♠ 5♠ 6♠ 7♠ 8♠ 9♠ 10♠ J♠ Q♠ K♠ A♠ 2♥": "Sảnh rồng",
		"3♠ 3♣ 5♠ 5♣ 7♠ 7♣ 9♠ 9♣ J♠ J♣ K♠ K♣ 2♥":  "6 đôi",
		"3♠ 3♣ 4♠ 4♣ 5♠ 5♣ 6♠ 6♣ 7♠ 7♣ 9♥ J♥ 2♥":  "5 đôi thông",
		"3♠ 4♣ 5♠ 6♣ 7♠ 8♣ 9♠ 10♣ J♠ Q♣ K♠ 2♣ 2♥": "",
	}
	for s, want := range cases {
		if got := InstantWin(cs(s)); got != want {
			t.Errorf("%s: muốn %q, được %q", s, want, got)
		}
	}
}

func TestAllCombosValid(t *testing.T) {
	all := AllCombos(cs("3♠ 3♣ 4♦ 4♥ 5♠ 5♣ 5♥ 6♦ 9♠ 9♣ 9♦ 9♥ 2♥"))
	has := func(t ComboType, length int) bool {
		return slices.ContainsFunc(all, func(c *Combo) bool { return c != nil && c.Type == t && c.Length == length })
	}
	if slices.Contains(all, nil) {
		t.Fatal("có bộ không hợp lệ")
	}
	if !has(PairSeq, 3) || !has(Straight, 4) || !has(Quad, 4) || !has(Triple, 3) {
		t.Error("thiếu bộ")
	}
}

func TestBotChopsTwoWithQuad(t *testing.T) {
	move := ChooseMove(cs("4♠ 9♠ 9♣ 9♦ 9♥ K♦"), Context{Prev: combo("2♥"), MustInclude: -1, MinOpponentCards: 10})
	if got := GetCombo(move); got == nil || got.Type != Quad {
		t.Fatalf("muốn chặt bằng tứ quý, được %v", move)
	}
}

func TestFirstGameLowestCardLeads(t *testing.T) {
	g, _ := NewGame(2, rand.New(rand.NewPCG(1, 2)))
	g.Start()
	if g.Instant != nil {
		t.Skip("tới trắng")
	}
	lowest := min(g.Hands[0][0], g.Hands[1][0])
	if g.MustInclude != lowest || g.Hands[g.Turn][0] != lowest {
		t.Fatalf("người giữ %s phải đánh trước", CardName(lowest))
	}
}

// Giả lập nhiều ván bot với nhau để chắc chắn luồng chơi không kẹt và luôn hợp lệ.
func TestSimulateBots(t *testing.T) {
	for players := 2; players <= 4; players++ {
		g, err := NewGame(players, rand.New(rand.NewPCG(42, uint64(players))))
		if err != nil {
			t.Fatal(err)
		}
		for range 500 {
			g.Start()
			for steps := 0; !g.Over; steps++ {
				if steps > 500 {
					t.Fatalf("%d người: ván bị kẹt", players)
				}
				p := g.Turn
				var err error
				if move := ChooseMove(g.Hands[p], g.Context(p)); move != nil {
					err = g.Play(p, move)
				} else {
					err = g.Pass(p)
				}
				if err != nil {
					t.Fatalf("%d người, người %d: %v", players, p, err)
				}
			}
			if g.Instant == nil && len(g.Finished) != players {
				t.Fatalf("%d người: xếp hạng thiếu %v", players, g.Finished)
			}
		}
		sum := 0
		for _, s := range g.Scores {
			sum += s
		}
		if sum != 0 {
			t.Errorf("%d người: tổng tiền %d khác 0", players, sum)
		}
	}
}
