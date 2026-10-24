const test = require('node:test');
const assert = require('node:assert');
const R = require('../js/rules.js');
const AI = require('../js/ai.js');
const { Game } = require('../js/engine.js');

// Tạo lá bài từ tên, ví dụ c('3♠'), c('10♥'), c('2♦').
const c = (name) => R.RANKS.indexOf(name.slice(0, -1)) * 4 + R.SUITS.indexOf(name.slice(-1));
const cs = (s) => s.split(' ').map(c);
const combo = (s) => R.getCombo(cs(s));

test('nhận diện bộ bài', () => {
  assert.equal(combo('3♠').type, 'single');
  assert.equal(combo('5♣ 5♥').type, 'pair');
  assert.equal(combo('7♠ 7♣ 7♦').type, 'triple');
  assert.equal(combo('9♠ 9♣ 9♦ 9♥').type, 'quad');
  assert.equal(combo('3♠ 4♦ 5♥').type, 'straight');
  assert.equal(combo('10♠ J♦ Q♥ K♣ A♠').length, 5);
  assert.equal(combo('3♠ 3♣ 4♦ 4♥ 5♠ 5♣').type, 'pairseq');
  assert.equal(combo('2♠ 2♥').type, 'pair');
  assert.equal(combo('K♠ A♦ 2♥'), null, 'sảnh không được có heo');
  assert.equal(combo('3♠ 5♦ 6♥'), null);
  assert.equal(combo('3♠ 3♣ 4♦ 4♥'), null, '2 đôi thông không phải bộ');
  assert.equal(combo('Q♠ Q♣ K♦ K♥ A♠ A♣').type, 'pairseq');
});

test('so bài cùng loại', () => {
  assert.ok(R.canBeat(combo('3♥'), combo('3♠')));
  assert.ok(!R.canBeat(combo('3♠'), combo('3♥')));
  assert.ok(R.canBeat(combo('2♠'), combo('A♥')));
  assert.ok(R.canBeat(combo('4♠ 5♠ 6♥'), combo('4♣ 5♦ 6♦')));
  assert.ok(!R.canBeat(combo('4♠ 5♠ 6♠ 7♠'), combo('3♣ 4♦ 5♦')), 'sảnh khác độ dài');
  assert.ok(!R.canBeat(combo('8♠ 8♣'), combo('7♠')));
});

test('chặt heo và chặt chồng', () => {
  const seq3 = combo('3♠ 3♣ 4♦ 4♥ 5♠ 5♣');
  const seq4 = combo('6♠ 6♣ 7♦ 7♥ 8♠ 8♣ 9♠ 9♣');
  const quad = combo('J♠ J♣ J♦ J♥');
  assert.ok(R.canBeat(seq3, combo('2♥')));
  assert.ok(!R.canBeat(seq3, combo('2♠ 2♥')));
  assert.ok(R.canBeat(quad, combo('2♥')));
  assert.ok(R.canBeat(quad, combo('2♠ 2♥')));
  assert.ok(R.canBeat(quad, seq3));
  assert.ok(R.canBeat(seq4, quad));
  assert.ok(R.canBeat(seq4, combo('2♠ 2♥')));
  assert.ok(!R.canBeat(quad, seq4));
  assert.ok(!R.canBeat(seq3, combo('A♥')), 'đôi thông không chặt rác thường');
  assert.ok(R.canBeat(combo('Q♠ Q♣ Q♦ Q♥'), quad));
});

test('tới trắng', () => {
  assert.equal(R.instantWin(cs('2♠ 2♣ 2♦ 2♥ 3♠ 4♠ 5♠ 6♠ 7♠ 8♠ 9♠ J♠ K♠')), 'Tứ quý heo');
  assert.equal(R.instantWin(cs('3♠ 4♠ 5♠ 6♠ 7♠ 8♠ 9♠ 10♠ J♠ Q♠ K♠ A♠ 2♥')), 'Sảnh rồng');
  assert.equal(R.instantWin(cs('3♠ 3♣ 5♠ 5♣ 7♠ 7♣ 9♠ 9♣ J♠ J♣ K♠ K♣ 2♥')), '6 đôi');
  assert.equal(R.instantWin(cs('3♠ 3♣ 4♠ 4♣ 5♠ 5♣ 6♠ 6♣ 7♠ 7♣ 9♥ J♥ 2♥')), '5 đôi thông');
  assert.equal(R.instantWin(cs('3♠ 4♣ 5♠ 6♣ 7♠ 8♣ 9♠ 10♣ J♠ Q♣ K♠ 2♣ 2♥')), null);
});

test('allCombos chỉ sinh bộ hợp lệ', () => {
  const hand = cs('3♠ 3♣ 4♦ 4♥ 5♠ 5♣ 5♥ 6♦ 9♠ 9♣ 9♦ 9♥ 2♥');
  const all = R.allCombos(hand);
  assert.ok(all.every(Boolean));
  assert.ok(all.some((x) => x.type === 'pairseq' && x.length === 3));
  assert.ok(all.some((x) => x.type === 'straight' && x.length === 4));
  assert.ok(all.some((x) => x.type === 'quad'));
});

test('bot chặt heo bằng tứ quý', () => {
  const hand = cs('4♠ 9♠ 9♣ 9♦ 9♥ K♦');
  const move = AI.chooseMove(hand, { prev: combo('2♥'), mustInclude: null, minOpponentCards: 10 });
  assert.equal(R.getCombo(move).type, 'quad');
});

// Giả lập nhiều ván bot với nhau để chắc chắn luồng chơi không kẹt và luôn hợp lệ.
test('giả lập 500 ván bot', () => {
  let seed = 42;
  const rand = () => ((seed = (seed * 1103515245 + 12345) % 2147483648) / 2147483648);
  const g = new Game(rand);
  for (let n = 0; n < 500; n++) {
    g.start();
    let steps = 0;
    while (!g.over) {
      assert.ok(steps++ < 500, 'ván bị kẹt');
      const p = g.turn;
      const move = AI.chooseMove(g.hands[p], g.context(p));
      const err = move ? g.play(p, move) : g.pass(p);
      assert.equal(err, null, `người ${p}: ${err}`);
    }
    if (!g.instant) assert.deepEqual(g.finished.slice().sort(), [0, 1, 2, 3]);
  }
  assert.equal(g.scores.reduce((a, b) => a + b, 0), 0);
});
