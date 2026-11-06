package tienlen

import (
	"math/bits"
	"slices"
)

// ---------- Máy đánh ----------
//
// Máy chỉ dùng bài trên tay mình và thông tin công khai (lá đã đánh ra, số lá từng người, ai đã bỏ lượt).
// Cách nghĩ:
//   - Chia tay bài thành các bộ sao cho ít lượt đánh nhất (planner), giữ đôi, sảnh, đôi thông, hàng.
//   - Bộ "chắc thắng" là bộ không tổ hợp nào từ lá chưa thấy chặn được (tính cả chặt). Lá chưa thấy gồm cả phần
//     bài không chia khi chơi dưới 4 người, nên máy không tự cho là chắc khi chưa chắc.
//   - Còn tối đa một bộ yếu thì đánh các bộ chắc thắng trước rồi về đích.
//   - Khi chặn: ưu tiên bộ có sẵn; không có thì tách tối đa một bộ để giữ nhịp; heo để dành tới khi đáng.
//   - Ván dừng khi có người hết bài và người thua trả theo số lá + thối heo/hàng, nên khi đối thủ sắp hết bài máy
//     chặn bằng mọi giá và xả heo.

// Context là thông tin công khai máy dùng để chọn nước đi.
type Context struct {
	Prev             *Combo // nil khi đang được đánh tự do
	MustInclude      Card   // -1 khi không bắt buộc
	MinOpponentCards int

	// Game.Context điền thêm các trường dưới; để trống (test, gọi tay) thì máy coi như không biết.
	Played     []Card // mọi lá đã đánh ra trong ván
	Counts     []int  // số lá đang cầm của từng ghế; 0 với ghế đã xong hoặc không còn đánh
	Seat       int
	Passed     []bool
	LastPlayer int // ghế đánh bộ đang nằm trên bàn; -1 khi đang được đánh tự do
}

type cardMask = uint64

const fullDeck cardMask = 1<<52 - 1

func maskOf(cards []Card) cardMask {
	var m cardMask
	for _, c := range cards {
		m |= 1 << c
	}
	return m
}

func cardsOf(m cardMask) []Card {
	out := make([]Card, 0, bits.OnesCount64(m))
	for m != 0 {
		c := bits.TrailingZeros64(m)
		out = append(out, c)
		m &= m - 1
	}
	return out
}

// ---------- Chia tay bài ----------

// Mỗi bộ tốn setCost; bộ thấp bị phạt thêm (12 − rank lá cao nhất) để cùng số bộ thì ưu tiên gom lá nhỏ vào bộ dài.
// Rác bị phạt thêm singlePenalty vì khó đi nhất.
const (
	setCost       = 100
	singlePenalty = 10
)

type planner struct {
	memo  map[cardMask]planStep
	combo map[cardMask]*Combo
}

type planStep struct {
	cost int
	set  cardMask // bộ đầu tiên của cách chia tốt nhất
}

func newPlanner() *planner {
	return &planner{memo: map[cardMask]planStep{}, combo: map[cardMask]*Combo{}}
}

func (pl *planner) comboOf(m cardMask) *Combo {
	if co, ok := pl.combo[m]; ok {
		return co
	}
	co := GetCombo(cardsOf(m))
	pl.combo[m] = co
	return co
}

func (pl *planner) setPenalty(m cardMask) int {
	co := pl.comboOf(m)
	switch {
	case co.IsBomb():
		return -30
	case co.IsTwos():
		return 0
	}
	if co.Type == Single {
		return Rank2 - Rank(co.Top) + singlePenalty
	}
	return Rank2 - Rank(co.Top)
}

// lowestOfRank trả lá nhỏ nhất của rank r còn trong m, -1 nếu không có.
func lowestOfRank(m cardMask, r int) int {
	x := m >> (r * 4) & 0xF
	if x == 0 {
		return -1
	}
	return r*4 + bits.TrailingZeros64(x)
}

// options liệt kê các bộ chứa lá nhỏ nhất của m. Sảnh / đôi thông lấy lá nhỏ nhất ở các rank sau.
func (pl *planner) options(m cardMask) []cardMask {
	c := bits.TrailingZeros64(m)
	r := Rank(c)
	var same []int
	for s := range 4 {
		if x := r*4 + s; x != c && m&(1<<x) != 0 {
			same = append(same, x)
		}
	}
	one := cardMask(1) << c
	opts := []cardMask{one}
	for i, a := range same {
		opts = append(opts, one|1<<a)
		for _, b := range same[i+1:] {
			opts = append(opts, one|1<<a|1<<b)
		}
	}
	if len(same) == 3 {
		opts = append(opts, one|1<<same[0]|1<<same[1]|1<<same[2])
	}
	if r >= Rank2 {
		return opts
	}
	cur := one
	for k := r + 1; k < Rank2; k++ {
		x := lowestOfRank(m, k)
		if x < 0 {
			break
		}
		cur |= 1 << x
		if k-r >= 2 {
			opts = append(opts, cur)
		}
	}
	for _, o := range same {
		cur := one | 1<<o
		for k := r + 1; k < Rank2; k++ {
			x := lowestOfRank(m, k)
			if x < 0 {
				break
			}
			y := lowestOfRank(m&^(1<<x), k)
			if y < 0 {
				break
			}
			cur |= 1<<x | 1<<y
			if k-r >= 2 {
				opts = append(opts, cur)
			}
		}
	}
	return opts
}

func (pl *planner) best(m cardMask) int {
	if m == 0 {
		return 0
	}
	if s, ok := pl.memo[m]; ok {
		return s.cost
	}
	step := planStep{cost: 1 << 30}
	for _, o := range pl.options(m) {
		if cost := setCost + pl.setPenalty(o) + pl.best(m&^o); cost < step.cost {
			step = planStep{cost, o}
		}
	}
	pl.memo[m] = step
	return step.cost
}

// plan trả cách chia tốt nhất của m thành các bộ.
func (pl *planner) plan(m cardMask) []cardMask {
	var out []cardMask
	for m != 0 {
		pl.best(m)
		s := pl.memo[m].set
		out = append(out, s)
		m &^= s
	}
	return out
}

// ---------- Lá chưa thấy và bộ chắc thắng ----------

type view struct {
	opp    []*Combo // bộ đối thủ có thể có từ lá chưa thấy, không dài hơn số lá nhiều nhất của một đối thủ
	minOpp int      // số lá ít nhất của một đối thủ còn đánh
	boss   map[cardMask]bool
}

func newView(hand []Card, ctx Context) *view {
	v := &view{minOpp: ctx.MinOpponentCards, boss: map[cardMask]bool{}}
	maxOpp := 13
	if len(ctx.Counts) > 0 {
		v.minOpp, maxOpp = 13, 0
		for q, n := range ctx.Counts {
			if q != ctx.Seat && n > 0 {
				v.minOpp, maxOpp = min(v.minOpp, n), max(maxOpp, n)
			}
		}
		if maxOpp == 0 {
			maxOpp = 13
		}
	}
	if v.minOpp <= 0 {
		v.minOpp = 13
	}
	unseen := cardsOf(fullDeck &^ maskOf(hand) &^ maskOf(ctx.Played))
	for _, co := range AllCombos(unseen) {
		if len(co.Cards) <= maxOpp {
			v.opp = append(v.opp, co)
		}
	}
	return v
}

// isBoss: không bộ nào từ lá chưa thấy chặn được co.
func (v *view) isBoss(m cardMask, co *Combo) bool {
	if b, ok := v.boss[m]; ok {
		return b
	}
	b := !slices.ContainsFunc(v.opp, func(o *Combo) bool { return CanBeat(o, co) })
	v.boss[m] = b
	return b
}

// reserved: heo và hàng, giữ để chặt / lấy lại quyền đánh.
func reserved(co *Combo) bool { return co.IsBomb() || co.IsTwos() }

// ---------- Chọn nước ----------

// ChooseMove chọn nước đi; trả nil để bỏ lượt (chỉ khi đang phải chặn).
func ChooseMove(hand []Card, ctx Context) []Card {
	if len(hand) == 0 {
		return nil
	}
	b := &brain{hand: hand, ctx: ctx, pl: newPlanner(), v: newView(hand, ctx), hm: maskOf(hand)}
	if ctx.Prev == nil {
		return b.lead()
	}
	return b.follow()
}

type brain struct {
	hand []Card
	ctx  Context
	pl   *planner
	v    *view
	hm   cardMask
}

func (b *brain) combo(m cardMask) *Combo { return b.pl.comboOf(m) }

func (b *brain) boss(m cardMask) bool { return b.v.isBoss(m, b.combo(m)) }

// weakCount đếm số bộ không chắc thắng trong cách chia.
func (b *brain) weakCount(sets []cardMask) int {
	n := 0
	for _, s := range sets {
		if !b.boss(s) {
			n++
		}
	}
	return n
}

// weaker: bộ nên đánh trước khi mở — lá cao nhất nhỏ hơn, bằng thì nhiều lá hơn.
func (b *brain) weaker(x, y cardMask) bool {
	cx, cy := b.combo(x), b.combo(y)
	if Rank(cx.Top) != Rank(cy.Top) {
		return Rank(cx.Top) < Rank(cy.Top)
	}
	return len(cx.Cards) > len(cy.Cards)
}

func (b *brain) lead() []Card {
	sets := b.pl.plan(b.hm)
	if len(sets) == 1 {
		return b.hand
	}
	if m := b.ctx.MustInclude; m >= 0 {
		for _, s := range sets {
			if s&(1<<m) != 0 {
				return cardsOf(s)
			}
		}
	}
	var bosses, weak []cardMask
	for _, s := range sets {
		if b.boss(s) {
			bosses = append(bosses, s)
		} else {
			weak = append(weak, s)
		}
	}
	// Về đích: đánh bộ chắc thắng để giữ quyền, chừa bộ yếu cuối cùng.
	if len(weak) <= 1 {
		if len(bosses) > 0 {
			return cardsOf(bosses[0])
		}
		return cardsOf(weak[0])
	}

	// Đối thủ còn 1 lá: không mở rác nhỏ. Có bộ nhiều lá thì mở, không thì đánh lá cao nhất.
	if b.v.minOpp == 1 {
		var multi []cardMask
		for _, s := range sets {
			if len(b.combo(s).Cards) > 1 && !reserved(b.combo(s)) {
				multi = append(multi, s)
			}
		}
		if len(multi) > 0 {
			return cardsOf(slices.MinFunc(multi, b.order))
		}
		return []Card{slices.Max(b.hand)}
	}

	cand := slices.DeleteFunc(slices.Clone(weak), func(s cardMask) bool { return reserved(b.combo(s)) })
	if len(cand) == 0 {
		cand = weak
	}
	// Đối thủ còn 2 lá: tránh mở đôi nếu có lựa chọn khác.
	if b.v.minOpp == 2 {
		if rest := slices.DeleteFunc(slices.Clone(cand), func(s cardMask) bool { return b.combo(s).Type == Pair }); len(rest) > 0 {
			cand = rest
		}
	}
	return cardsOf(slices.MinFunc(cand, b.order))
}

func (b *brain) order(x, y cardMask) int {
	switch {
	case x == y:
		return 0
	case b.weaker(x, y):
		return -1
	}
	return 1
}

type reply struct {
	m        cardMask
	co       *Combo
	natural  bool // nằm sẵn trong một cách chia không tốn thêm bộ
	extra    int  // số bộ phải tách thêm so với khi đánh một bộ có sẵn
	boss     bool
	restWeak int
}

func (b *brain) follow() []Card {
	prev := b.ctx.Prev
	baseSets := len(b.pl.plan(b.hm))
	var replies []reply
	for _, co := range AllCombos(b.hand) {
		if !CanBeat(co, prev) || (b.ctx.MustInclude >= 0 && !slices.Contains(co.Cards, b.ctx.MustInclude)) {
			continue
		}
		if len(co.Cards) == len(b.hand) {
			return co.Cards
		}
		m := maskOf(co.Cards)
		rest := b.pl.plan(b.hm &^ m)
		replies = append(replies, reply{m: m, co: co, natural: len(rest) <= baseSets-1, extra: len(rest) - (baseSets - 1), boss: b.boss(m), restWeak: b.weakCount(rest)})
	}
	if len(replies) == 0 {
		return nil
	}
	low := func(list []reply) []Card {
		return slices.MinFunc(list, func(x, y reply) int { return x.co.Top - y.co.Top }).co.Cards
	}
	pick := func(keep func(r reply) bool) []reply {
		var out []reply
		for _, r := range replies {
			if keep(r) {
				out = append(out, r)
			}
		}
		return out
	}

	// Chặn xong không ai chặn lại được và chỉ còn tối đa một bộ yếu: chắc về đích.
	if win := pick(func(r reply) bool { return r.boss && r.restWeak <= 1 }); len(win) > 0 {
		return low(win)
	}
	// Chặt heo / chặt hàng: ăn tiền và lấy quyền đánh.
	if prev.IsTwos() || prev.IsBomb() {
		if chop := pick(func(r reply) bool { return r.co.IsBomb() }); len(chop) > 0 {
			return low(chop)
		}
	}
	// Đối thủ sắp hết bài: chặn bằng mọi giá, ưu tiên bộ chắc thắng, rồi bộ cao nhất (xả luôn heo).
	if b.v.minOpp <= 2 {
		if boss := pick(func(r reply) bool { return r.boss && !r.co.IsBomb() }); len(boss) > 0 {
			return low(boss)
		}
		return slices.MaxFunc(replies, func(x, y reply) int { return x.co.Top - y.co.Top }).co.Cards
	}
	// Bình thường: chặn bằng bộ có sẵn trong cách chia. Heo chỉ ra khi đối thủ còn ít bài hoặc mình sắp hết.
	// (Ngưỡng chọn bằng đo đấu với máy cũ trong ai_test.go.)
	twoOK := b.v.minOpp <= 4 || baseSets <= 3
	if natural := pick(func(r reply) bool {
		return r.natural && !r.co.IsBomb() && (!r.co.IsTwos() || twoOK)
	}); len(natural) > 0 {
		return low(natural)
	}
	// Không có bộ sẵn: chấp nhận tách tối đa một bộ (không dùng heo, hàng) để giữ nhịp, thay vì bỏ lượt.
	if cheap := pick(func(r reply) bool { return r.extra <= 1 && !reserved(r.co) }); len(cheap) > 0 {
		return low(cheap)
	}
	return nil
}
