// Trạng thái và luồng chơi của một bàn 4 người. Không phụ thuộc giao diện.
(function (root) {
  'use strict';
  const R = typeof module !== 'undefined' && module.exports ? require('./rules.js') : root.Rules;

  const PLAYERS = 4;
  const RANK_POINTS = [3, 1, -1, -3];

  class Game {
    constructor(rand = Math.random) {
      this.rand = rand;
      this.scores = new Array(PLAYERS).fill(0);
      this.lastWinner = null;
      this.gameNo = 0;
    }

    start() {
      this.gameNo++;
      this.hands = R.deal(this.rand);
      this.finished = [];
      this.passed = new Array(PLAYERS).fill(false);
      this.lastPlay = null;
      this.over = false;
      this.instant = null;
      this.history = [];

      for (let p = 0; p < PLAYERS; p++) {
        const reason = R.instantWin(this.hands[p]);
        if (reason) {
          this.instant = { player: p, reason };
          this.over = true;
          this.finished = [p];
          this.lastWinner = p;
          for (let q = 0; q < PLAYERS; q++) this.scores[q] += q === p ? 6 : -2;
          return;
        }
      }

      if (this.lastWinner === null) {
        this.turn = this.hands.findIndex((h) => h.includes(R.THREE_SPADES));
        this.mustInclude = R.THREE_SPADES;
      } else {
        this.turn = this.lastWinner;
        this.mustInclude = null;
      }
    }

    isActive(p) {
      return !this.finished.includes(p);
    }

    context(p) {
      let minOpponentCards = 13;
      for (let q = 0; q < PLAYERS; q++) {
        if (q !== p && this.isActive(q)) minOpponentCards = Math.min(minOpponentCards, this.hands[q].length);
      }
      return { prev: this.lastPlay && this.lastPlay.combo, mustInclude: this.mustInclude, minOpponentCards };
    }

    // Kiểm tra nước đi; trả về chuỗi lỗi hoặc null nếu hợp lệ.
    validate(p, cards) {
      if (this.over) return 'Ván đã kết thúc';
      if (p !== this.turn) return 'Chưa tới lượt';
      if (!cards.length) return 'Chưa chọn lá nào';
      if (!cards.every((c) => this.hands[p].includes(c))) return 'Lá bài không có trên tay';
      const combo = R.getCombo(cards);
      if (!combo) return 'Bộ bài không hợp lệ';
      if (this.mustInclude != null && !cards.includes(this.mustInclude)) {
        return 'Ván đầu phải đánh kèm ' + R.cardName(this.mustInclude);
      }
      if (this.lastPlay && !R.canBeat(combo, this.lastPlay.combo)) return 'Không chặn được bài trên bàn';
      return null;
    }

    play(p, cards) {
      const err = this.validate(p, cards);
      if (err) return err;
      const combo = R.getCombo(cards);
      const chop = this.lastPlay && combo.type !== this.lastPlay.combo.type;
      this.hands[p] = this.hands[p].filter((c) => !cards.includes(c));
      this.lastPlay = { player: p, combo };
      this.mustInclude = null;
      this.history.push({ player: p, combo, chop });

      if (this.hands[p].length === 0) {
        this.finished.push(p);
        if (this.finished.length === PLAYERS - 1) {
          for (let q = 0; q < PLAYERS; q++) if (this.isActive(q)) this.finished.push(q);
          this.finish();
          return null;
        }
      }
      this.advance();
      return null;
    }

    pass(p) {
      if (this.over) return 'Ván đã kết thúc';
      if (p !== this.turn) return 'Chưa tới lượt';
      if (!this.lastPlay) return 'Bạn đang được đánh trước, không thể bỏ lượt';
      this.passed[p] = true;
      this.history.push({ player: p, pass: true });
      this.advance();
      return null;
    }

    advance() {
      let i = this.turn;
      for (let step = 0; step < PLAYERS * 2; step++) {
        i = (i + 1) % PLAYERS;
        if (i === this.lastPlay.player) return this.newRound(i);
        if (this.isActive(i) && !this.passed[i]) {
          this.turn = i;
          return;
        }
      }
    }

    // Mọi người đã bỏ lượt: người đánh cuối được đánh tiếp.
    // Nếu người đó đã về nhất/nhì..., quyền đánh chuyển sang người kế tiếp.
    newRound(leader) {
      this.lastPlay = null;
      this.passed.fill(false);
      while (!this.isActive(leader)) leader = (leader + 1) % PLAYERS;
      this.turn = leader;
      this.history.push({ newRound: true, player: leader });
    }

    finish() {
      this.over = true;
      this.lastWinner = this.finished[0];
      this.finished.forEach((p, place) => (this.scores[p] += RANK_POINTS[place]));
    }
  }

  const api = { Game, PLAYERS };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.Engine = api;
})(this);
