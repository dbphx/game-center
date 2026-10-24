// Bot đơn giản dựa trên heuristic: ưu tiên xả bài nhỏ, giữ heo và bom cho lúc cần.
(function (root) {
  'use strict';
  const R = typeof module !== 'undefined' && module.exports ? require('./rules.js') : root.Rules;

  // Phạt khi bộ bài làm vỡ đôi / sám / tứ quý đang có.
  function breakPenalty(combo, groups) {
    const used = new Map();
    for (const c of combo.cards) used.set(R.rankOf(c), (used.get(R.rankOf(c)) || 0) + 1);
    let p = 0;
    for (const [rank, n] of used) {
      const size = groups[rank].length;
      if (size > n) p += size >= 3 ? 12 : 6;
    }
    return p;
  }

  // Chi phí từ mức này trở lên nghĩa là bot chỉ đánh khi gấp, còn không thì bỏ lượt.
  const RELUCTANT = 1000;

  const hasTwo = (combo) => combo.cards.some((c) => R.rankOf(c) === R.RANK_2);

  // ctx: { prev: combo | null, mustInclude: card | null, minOpponentCards: number }
  // Trả về mảng lá bài để đánh, hoặc null để bỏ lượt.
  function chooseMove(hand, ctx) {
    const groups = R.groupByRank(hand);
    let combos = R.allCombos(hand);
    if (ctx.mustInclude != null) combos = combos.filter((c) => c.cards.includes(ctx.mustInclude));
    if (ctx.prev) combos = combos.filter((c) => R.canBeat(c, ctx.prev));
    if (combos.length === 0) return null;

    const finishing = combos.find((c) => c.cards.length === hand.length);
    if (finishing) return finishing.cards;

    const urgent = ctx.minOpponentCards <= 3 || hand.length <= 4;
    const scored = combos.map((c) => ({ c, cost: ctx.prev ? followCost(c, ctx, groups, urgent) : leadCost(c, ctx, groups) }));
    scored.sort((a, b) => a.cost - b.cost);
    const best = scored[0];

    if (ctx.prev && !urgent && best.cost >= RELUCTANT) return null;
    return best.c.cards;
  }

  function leadCost(c, ctx, groups) {
    const minRank = R.rankOf(c.cards[0]);
    let cost = minRank * 10 - (c.cards.length - 1) * 4 + breakPenalty(c, groups);
    if (hasTwo(c)) cost += 200;
    if (R.isBomb(c)) cost += 200;
    // Đối thủ còn 1 lá thì tránh đánh rác nhỏ.
    if (ctx.minOpponentCards === 1 && c.type === 'single') cost += 100 - R.rankOf(c.top) * 5;
    return cost;
  }

  function followCost(c, ctx, groups, urgent) {
    let cost = R.rankOf(c.top) * 10 + breakPenalty(c, groups);
    const prevIsTwos = R.isTwos(ctx.prev) || R.isBomb(ctx.prev);
    if (R.isBomb(c)) cost += prevIsTwos ? 20 : RELUCTANT;
    else if (hasTwo(c)) {
      const prevHigh = R.rankOf(ctx.prev.top) >= 9; // từ Q trở lên mới đáng ra heo
      cost += urgent || prevHigh ? 30 : RELUCTANT;
    }
    return cost;
  }

  const api = { chooseMove };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.AI = api;
})(this);
