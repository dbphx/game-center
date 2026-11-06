package tienlen

import (
	"math/rand/v2"
	"slices"
	"sort"
	"testing"
	"time"
)

// ---------- Máy mới: tình huống chọn sẵn ----------

func TestBotFinishesWithBossFirst(t *testing.T) {
	// Đối thủ còn 1 lá thì chỉ chặn được rác: 2♥ chắc thắng, đánh 2♥ rồi về bằng 3♠.
	move := ChooseMove(cs("3♠ 2♥"), Context{MustInclude: -1, Counts: []int{2, 1}, Seat: 0, LastPlayer: -1})
	if !slices.Equal(move, cs("2♥")) {
		t.Fatalf("muốn đánh 2♥ trước, được %v", move)
	}
}

func TestBotBlocksOpponentOnLastCard(t *testing.T) {
	move := ChooseMove(cs("6♠ 9♠ 9♣ K♣ 2♠"), Context{Prev: combo("5♦"), MustInclude: -1, Counts: []int{5, 1}, Seat: 0, LastPlayer: 1})
	if !slices.Equal(move, cs("2♠")) {
		t.Fatalf("đối thủ còn 1 lá phải chặn bằng lá cao nhất, được %v", move)
	}
}

func TestBotAvoidsSingleLeadWhenOpponentHasOne(t *testing.T) {
	move := ChooseMove(cs("3♠ 7♦ 7♥ J♣"), Context{MustInclude: -1, Counts: []int{4, 1}, Seat: 0, LastPlayer: -1})
	if got := GetCombo(move); got == nil || got.Type != Pair {
		t.Fatalf("đối thủ còn 1 lá không được mở rác, được %v", move)
	}
}

func TestBotKeepsPairsWhenFollowing(t *testing.T) {
	// Có 8♠ lẻ thì chặn bằng 8♠, không đập đôi 7.
	move := ChooseMove(cs("4♣ 7♦ 7♥ 8♠ J♣ J♦ K♥"), Context{Prev: combo("6♠"), MustInclude: -1, Counts: []int{7, 10}, Seat: 0, LastPlayer: 1})
	if !slices.Equal(move, cs("8♠")) {
		t.Fatalf("muốn chặn bằng 8♠, được %v", move)
	}
}

// Review Focus: lượt đầu bắt buộc có 3♠ mà 3♠ nằm trong sảnh thì vẫn đánh hợp lệ, có 3♠.
func TestBotMustIncludeLegal(t *testing.T) {
	hand := cs("3♠ 4♦ 5♥ 5♣ 6♠ 9♦ 9♥ 9♣ Q♠ K♦ A♣ 2♦ 2♣")
	move := ChooseMove(hand, Context{MustInclude: c("3♠"), MinOpponentCards: 13, Counts: []int{13, 13, 13, 13}, LastPlayer: -1})
	if GetCombo(move) == nil || !slices.Contains(move, c("3♠")) {
		t.Fatalf("lượt đầu phải đánh bộ hợp lệ có 3♠, được %v", move)
	}
}

// Review Focus: bàn ít người, lá chưa thấy có thể nằm ở phần không chia: không coi là chắc thắng.
func TestBotNoFalseBossWithUndealtCards(t *testing.T) {
	hand := cs("3♠ A♥")
	ctx := Context{MustInclude: -1, Counts: []int{2, 5}, Seat: 0, LastPlayer: -1}
	if newView(hand, ctx).isBoss(maskOf(cs("A♥")), combo("A♥")) {
		t.Fatal("2 chưa ra (có thể nằm ở phần không chia) thì A♥ không chắc thắng")
	}
	if move := ChooseMove(hand, ctx); !slices.Equal(move, cs("3♠")) {
		t.Fatalf("chưa chắc thắng thì mở bộ yếu trước, được %v", move)
	}
	ctx.Played = cs("2♠ 2♣ 2♦ 2♥")
	if !newView(hand, ctx).isBoss(maskOf(cs("A♥")), combo("A♥")) {
		t.Fatal("4 heo đã ra, đối thủ 5 lá không có tứ quý chặn rác → A♥ phải chắc thắng")
	}
}

func TestPlannerKeepsSets(t *testing.T) {
	pl := newPlanner()
	sets := pl.plan(maskOf(cs("3♠ 4♦ 5♥ 7♣ 7♦ 9♠ 9♣ 9♦ 9♥")))
	var types []ComboType
	for _, s := range sets {
		types = append(types, pl.comboOf(s).Type)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	if !slices.Equal(types, []ComboType{Pair, Quad, Straight}) {
		t.Fatalf("muốn sảnh + đôi + tứ quý, được %v", types)
	}
}

// ---------- Đấu máy mới với máy cũ ----------

type bot func([]Card, Context) []Card

type matchResult struct {
	games   int
	wins    [4]int
	money   [4]int
	slowest time.Duration
}

// playMatch chạy games ván 4 ghế liên tiếp trên một bàn, ghế p dùng bots[p].
func playMatch(t *testing.T, seed uint64, games int, bots [4]bot) matchResult {
	t.Helper()
	g, err := NewGame(4, rand.New(rand.NewPCG(seed, 7)))
	if err != nil {
		t.Fatal(err)
	}
	res := matchResult{games: games}
	for range games {
		g.Start()
		for steps := 0; !g.Over; steps++ {
			if steps > 2000 {
				t.Fatal("ván không kết thúc")
			}
			p := g.Turn
			start := time.Now()
			move := bots[p](g.Hands[p], g.Context(p))
			res.slowest = max(res.slowest, time.Since(start))
			if move != nil {
				if err := g.Play(p, move); err != nil {
					t.Fatalf("ghế %d đánh %v không hợp lệ: %v", p, move, err)
				}
			} else if err := g.Pass(p); err != nil {
				t.Fatalf("ghế %d bỏ lượt không hợp lệ: %v", p, err)
			}
		}
		res.wins[g.Finished[0]]++
		for p, m := range g.Money() {
			res.money[p] += m
		}
	}
	return res
}

// Máy mới ngồi ghế 0, 2 rồi ghế 1, 3 với cùng chuỗi chia bài; tỉ lệ thắng trung bình mỗi ghế phải từ 32%.
func TestNewBotBeatsLegacy(t *testing.T) {
	games := 2000
	if testing.Short() {
		games = 200
	}
	var newWins, oldWins, newMoney, oldMoney int
	var slowest time.Duration
	for _, seats := range [][2]int{{0, 2}, {1, 3}} {
		var bots [4]bot
		for p := range bots {
			bots[p] = legacyChooseMove
		}
		for _, p := range seats {
			bots[p] = ChooseMove
		}
		r := playMatch(t, 2026, games/2, bots)
		slowest = max(slowest, r.slowest)
		for p := range 4 {
			if slices.Contains(seats[:], p) {
				newWins, newMoney = newWins+r.wins[p], newMoney+r.money[p]
			} else {
				oldWins, oldMoney = oldWins+r.wins[p], oldMoney+r.money[p]
			}
		}
	}
	seatGames := float64(games * 2) // mỗi loại máy ngồi 2 ghế mỗi ván
	newRate, oldRate := float64(newWins)/seatGames, float64(oldWins)/seatGames
	t.Logf("%d ván: máy mới thắng %.1f%%/ghế, máy cũ %.1f%%/ghế; tiền TB mỗi ghế mỗi ván: mới %+.2f, cũ %+.2f mức; nước chậm nhất %v",
		games, newRate*100, oldRate*100, float64(newMoney)/seatGames, float64(oldMoney)/seatGames, slowest)
	if !testing.Short() && newRate < 0.32 {
		t.Errorf("máy mới thắng %.1f%%/ghế, muốn từ 32%%", newRate*100)
	}
	if newMoney <= oldMoney {
		t.Errorf("máy mới phải lời hơn máy cũ: %d ≤ %d", newMoney, oldMoney)
	}
}

// Nước chậm nhất của máy mới dưới 50 ms (bàn toàn máy mới, nhiều ván).
func TestNewBotSpeed(t *testing.T) {
	games := 300
	if testing.Short() {
		games = 50
	}
	r := playMatch(t, 99, games, [4]bot{ChooseMove, ChooseMove, ChooseMove, ChooseMove})
	t.Logf("%d ván, nước chậm nhất %v", games, r.slowest)
	if limit := 50 * time.Millisecond; r.slowest > limit && !raceOn {
		t.Errorf("nước chậm nhất %v, muốn dưới %v", r.slowest, limit)
	}
}

// Máy mới luôn đi hợp lệ với 2, 3, 4 người, kể cả ngồi cùng máy cũ.
func TestNewBotLegalAllTables(t *testing.T) {
	for players := 2; players <= 4; players++ {
		g, err := NewGame(players, rand.New(rand.NewPCG(5, uint64(players))))
		if err != nil {
			t.Fatal(err)
		}
		for i := range 300 {
			g.Start()
			for steps := 0; !g.Over; steps++ {
				if steps > 2000 {
					t.Fatal("ván không kết thúc")
				}
				p := g.Turn
				f := ChooseMove
				if (p+i)%2 == 1 {
					f = legacyChooseMove
				}
				if move := f(g.Hands[p], g.Context(p)); move != nil {
					if err := g.Play(p, move); err != nil {
						t.Fatalf("%d người, ghế %d đánh %v: %v", players, p, move, err)
					}
				} else if err := g.Pass(p); err != nil {
					t.Fatalf("%d người, ghế %d bỏ lượt: %v", players, p, err)
				}
			}
		}
	}
}

// ---------- Máy cũ (heuristic từng lượt), giữ nguyên làm đối chứng ----------

// Chi phí từ mức này trở lên nghĩa là bot chỉ đánh khi gấp, còn không thì bỏ lượt.
const legacyReluctant = 1000

// legacyBreakPenalty phạt khi bộ bài làm vỡ đôi / sám / tứ quý đang có.
func legacyBreakPenalty(c *Combo, groups [13][]Card) int {
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

func legacyHasTwo(c *Combo) bool {
	return slices.ContainsFunc(c.Cards, func(card Card) bool { return Rank(card) == Rank2 })
}

// legacyChooseMove chọn nước đi theo heuristic: ưu tiên xả bài nhỏ, giữ heo và bom cho lúc cần.
// Trả về nil để bỏ lượt.
func legacyChooseMove(hand []Card, ctx Context) []Card {
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
			return legacyLeadCost(c, ctx, groups)
		}
		return legacyFollowCost(c, ctx, groups, urgent)
	}
	sort.SliceStable(combos, func(i, j int) bool { return cost(combos[i]) < cost(combos[j]) })
	best := combos[0]

	if ctx.Prev != nil && !urgent && cost(best) >= legacyReluctant {
		return nil
	}
	return best.Cards
}

func legacyLeadCost(c *Combo, ctx Context, groups [13][]Card) int {
	cost := Rank(c.Cards[0])*10 - (len(c.Cards)-1)*4 + legacyBreakPenalty(c, groups)
	if legacyHasTwo(c) {
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

func legacyFollowCost(c *Combo, ctx Context, groups [13][]Card, urgent bool) int {
	cost := Rank(c.Top)*10 + legacyBreakPenalty(c, groups)
	prevIsTwos := ctx.Prev.IsTwos() || ctx.Prev.IsBomb()
	switch {
	case c.IsBomb():
		if prevIsTwos {
			cost += 20
		} else {
			cost += legacyReluctant
		}
	case legacyHasTwo(c):
		prevHigh := Rank(ctx.Prev.Top) >= 9 // từ Q trở lên mới đáng ra heo
		if urgent || prevHigh {
			cost += 30
		} else {
			cost += legacyReluctant
		}
	}
	return cost
}
