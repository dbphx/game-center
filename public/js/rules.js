// Phần luật client cần để hiển thị: tên lá bài và nhận diện bộ bài.
// Server Go (game/rules.go) mới là nơi quyết định nước đi hợp lệ; quy ước lá bài giống hệt:
// id 0..51, rank = id >> 2 (0 = 3, ..., 11 = A, 12 = 2), suit = id & 3 (0 ♠, 1 ♣, 2 ♦, 3 ♥).
(function (root) {
  'use strict';

  const RANKS = ['3', '4', '5', '6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A', '2'];
  const SUITS = ['♠', '♣', '♦', '♥'];
  const RANK_2 = 12;

  const rankOf = (c) => c >> 2;
  const suitOf = (c) => c & 3;
  const cardName = (c) => RANKS[rankOf(c)] + SUITS[suitOf(c)];

  const TYPE_NAMES = {
    single: 'Rác',
    pair: 'Đôi',
    triple: 'Sám cô',
    straight: 'Sảnh',
    pairseq: 'Đôi thông',
    quad: 'Tứ quý',
  };

  function isConsecutive(ranks) {
    for (let i = 1; i < ranks.length; i++) {
      if (ranks[i] !== ranks[i - 1] + 1) return false;
    }
    return true;
  }

  // Trả về { type, length, top, cards } hoặc null nếu không phải bộ hợp lệ.
  function getCombo(cards) {
    if (!cards || cards.length === 0) return null;
    const cs = cards.slice().sort((a, b) => a - b);
    const n = cs.length;
    const ranks = cs.map(rankOf);
    const allSame = ranks.every((r) => r === ranks[0]);
    const make = (type, length) => ({ type, length, top: cs[n - 1], cards: cs });

    if (n === 1) return make('single', 1);
    if (allSame) {
      if (n === 2) return make('pair', 2);
      if (n === 3) return make('triple', 3);
      if (n === 4) return make('quad', 4);
    }
    if (ranks.includes(RANK_2)) return null;
    if (n >= 3 && isConsecutive(ranks)) return make('straight', n);
    if (n >= 6 && n % 2 === 0) {
      const pairRanks = [];
      for (let i = 0; i < n; i += 2) {
        if (ranks[i] !== ranks[i + 1]) return null;
        pairRanks.push(ranks[i]);
      }
      if (isConsecutive(pairRanks)) return make('pairseq', n / 2);
    }
    return null;
  }

  function describe(combo) {
    if (!combo) return '';
    const twos = rankOf(combo.top) === RANK_2;
    if (combo.type === 'single' && twos) return 'Heo';
    if (combo.type === 'pair' && twos) return 'Đôi heo';
    if (combo.type === 'straight') return 'Sảnh ' + combo.length + ' lá';
    if (combo.type === 'pairseq') return combo.length + ' đôi thông';
    return TYPE_NAMES[combo.type];
  }

  root.Rules = { RANKS, SUITS, rankOf, suitOf, cardName, getCombo, describe };
})(this);
