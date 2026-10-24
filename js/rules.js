// Luật Tiến Lên Miền Nam.
// Lá bài là số nguyên 0..51: rank = id >> 2 (0 = 3, ..., 11 = A, 12 = 2), suit = id & 3 (0 ♠, 1 ♣, 2 ♦, 3 ♥).
// So sánh 2 lá chỉ cần so id: rank trước, chất sau.
(function (root) {
  'use strict';

  const RANKS = ['3', '4', '5', '6', '7', '8', '9', '10', 'J', 'Q', 'K', 'A', '2'];
  const SUITS = ['♠', '♣', '♦', '♥'];
  const RANK_2 = 12;
  const THREE_SPADES = 0;

  const rankOf = (c) => c >> 2;
  const suitOf = (c) => c & 3;
  const cardName = (c) => RANKS[rankOf(c)] + SUITS[suitOf(c)];
  const sortCards = (cards) => cards.slice().sort((a, b) => a - b);

  const TYPE_NAMES = {
    single: 'Rác',
    pair: 'Đôi',
    triple: 'Sám cô',
    straight: 'Sảnh',
    pairseq: 'Đôi thông',
    quad: 'Tứ quý',
  };

  function newDeck() {
    return Array.from({ length: 52 }, (_, i) => i);
  }

  function shuffle(arr, rand = Math.random) {
    const a = arr.slice();
    for (let i = a.length - 1; i > 0; i--) {
      const j = Math.floor(rand() * (i + 1));
      [a[i], a[j]] = [a[j], a[i]];
    }
    return a;
  }

  function deal(rand) {
    const deck = shuffle(newDeck(), rand);
    return [0, 1, 2, 3].map((p) => sortCards(deck.slice(p * 13, p * 13 + 13)));
  }

  function isConsecutive(ranks) {
    for (let i = 1; i < ranks.length; i++) {
      if (ranks[i] !== ranks[i - 1] + 1) return false;
    }
    return true;
  }

  // Trả về { type, length, top, cards } hoặc null nếu không phải bộ hợp lệ.
  // length: số lá với sảnh, số đôi với đôi thông.
  function getCombo(cards) {
    if (!cards || cards.length === 0) return null;
    const cs = sortCards(cards);
    const n = cs.length;
    const top = cs[n - 1];
    const ranks = cs.map(rankOf);
    const allSame = ranks.every((r) => r === ranks[0]);
    const make = (type, length) => ({ type, length, top, cards: cs });

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

  const isTwos = (combo) => rankOf(combo.top) === RANK_2;

  // combo có đè được prev không (kể cả chặt).
  function canBeat(combo, prev) {
    if (!combo) return false;
    if (!prev) return true;

    if (combo.type === prev.type && combo.length === prev.length) {
      return combo.top > prev.top;
    }

    const quad = combo.type === 'quad';
    const seq3 = combo.type === 'pairseq' && combo.length === 3;
    const seq4 = combo.type === 'pairseq' && combo.length >= 4;

    // Chặt heo
    if (prev.type === 'single' && isTwos(prev)) return quad || seq3 || seq4;
    // Chặt đôi heo
    if (prev.type === 'pair' && isTwos(prev)) return quad || seq4;
    // Chặt chồng
    if (prev.type === 'pairseq' && prev.length === 3) return quad || seq4;
    if (prev.type === 'quad') return seq4;
    return false;
  }

  function isBomb(combo) {
    return combo.type === 'quad' || (combo.type === 'pairseq' && combo.length >= 3);
  }

  function describe(combo) {
    if (!combo) return '';
    let name = TYPE_NAMES[combo.type];
    if (combo.type === 'single' && isTwos(combo)) name = 'Heo';
    if (combo.type === 'pair' && isTwos(combo)) name = 'Đôi heo';
    if (combo.type === 'straight') name += ' ' + combo.length + ' lá';
    if (combo.type === 'pairseq') name = combo.length + ' đôi thông';
    return name;
  }

  function groupByRank(hand) {
    const groups = Array.from({ length: 13 }, () => []);
    for (const c of sortCards(hand)) groups[rankOf(c)].push(c);
    return groups;
  }

  // Tới trắng: thắng ngay khi vừa chia bài.
  function instantWin(hand) {
    const g = groupByRank(hand);
    if (g[RANK_2].length === 4) return 'Tứ quý heo';
    if (g.slice(0, 12).every((x) => x.length > 0)) return 'Sảnh rồng';
    const pairs = g.reduce((s, x) => s + Math.floor(x.length / 2), 0);
    if (pairs >= 6) return '6 đôi';
    let run = 0;
    for (let r = 0; r < 12; r++) {
      run = g[r].length >= 2 ? run + 1 : 0;
      if (run >= 5) return '5 đôi thông';
    }
    return null;
  }

  function choose2(arr) {
    const out = [];
    for (let i = 0; i < arr.length; i++)
      for (let j = i + 1; j < arr.length; j++) out.push([arr[i], arr[j]]);
    return out;
  }

  // Liệt kê các bộ có thể đánh từ một tay bài.
  // Với sảnh / đôi thông: các rank thấp dùng lá nhỏ nhất, chỉ rank cao nhất thử mọi lựa chọn.
  function allCombos(hand) {
    const g = groupByRank(hand);
    const out = [];
    const push = (cards) => out.push(getCombo(cards));

    for (let r = 0; r < 13; r++) {
      const cs = g[r];
      for (const c of cs) push([c]);
      for (const p of choose2(cs)) push(p);
      if (cs.length >= 3) {
        for (let skip = 0; skip < cs.length; skip++) {
          if (cs.length === 3 && skip > 0) break;
          push(cs.length === 3 ? cs : cs.filter((_, i) => i !== skip));
        }
      }
      if (cs.length === 4) push(cs);
    }

    for (let start = 0; start < 12; start++) {
      const body = [];
      for (let end = start; end < 12 && g[end].length > 0; end++) {
        if (end - start >= 2) {
          for (const topCard of g[end]) push(body.concat([topCard]));
        }
        body.push(g[end][0]);
      }
    }

    for (let start = 0; start < 12; start++) {
      const body = [];
      for (let end = start; end < 12 && g[end].length >= 2; end++) {
        if (end - start >= 2) {
          for (const p of choose2(g[end])) push(body.concat(p));
        }
        body.push(g[end][0], g[end][1]);
      }
    }
    return out;
  }

  const api = {
    RANKS, SUITS, RANK_2, THREE_SPADES,
    rankOf, suitOf, cardName, sortCards,
    newDeck, shuffle, deal,
    getCombo, canBeat, isBomb, isTwos, describe,
    groupByRank, instantWin, allCombos,
  };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.Rules = api;
})(this);
