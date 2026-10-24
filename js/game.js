// Giao diện: người chơi ở ghế 0 (dưới), 3 bot ở ghế 1 (phải), 2 (trên), 3 (trái).
(function () {
  'use strict';
  const R = window.Rules;
  const NAMES = ['Bạn', 'Máy 1', 'Máy 2', 'Máy 3'];
  const PLACES = ['Nhất', 'Nhì', 'Ba', 'Bét'];
  const BOT_DELAY = 800;
  const HUMAN = 0;

  const game = new Engine.Game();
  let selected = new Set();
  let botTimer = null;

  const $ = (id) => document.getElementById(id);
  const seatEl = (p) => document.querySelector(`.seat[data-seat="${p}"]`);

  function cardEl(c, opts = {}) {
    const el = document.createElement('div');
    if (opts.back) {
      el.className = 'card back' + (opts.small ? ' small' : '');
      return el;
    }
    const red = R.suitOf(c) >= 2;
    el.className = 'card' + (red ? ' red' : '') + (opts.small ? ' small' : '');
    const rank = R.RANKS[R.rankOf(c)];
    const suit = R.SUITS[R.suitOf(c)];
    el.innerHTML = `<div class="corner">${rank}<br>${suit}</div><div class="pip">${suit}</div>`;
    el.dataset.card = c;
    return el;
  }

  function render() {
    for (let p = 0; p < 4; p++) renderSeat(p);
    renderPile();
    renderScores();
    renderControls();
  }

  function renderSeat(p) {
    const el = seatEl(p);
    el.innerHTML = '';
    el.classList.toggle('turn', !game.over && game.turn === p);
    el.classList.toggle('done', !game.isActive(p) && !game.over);
    el.classList.toggle('locked', p === HUMAN && (game.over || game.turn !== HUMAN));

    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = NAMES[p];
    const hand = game.hands[p];
    const badge = (cls, text) => {
      const b = document.createElement('span');
      b.className = 'badge ' + cls;
      b.textContent = text;
      name.appendChild(b);
    };
    const place = game.finished.indexOf(p);
    if (place >= 0) badge('place', PLACES[place]);
    else badge('', hand.length + ' lá');
    if (game.passed[p] && game.lastPlay && !game.over) badge('pass', 'Bỏ lượt');
    el.appendChild(name);

    const handEl = document.createElement('div');
    handEl.className = 'hand';
    const reveal = p === HUMAN || game.over;
    for (const c of hand) {
      const ce = cardEl(c, { back: !reveal, small: p !== HUMAN });
      if (p === HUMAN) {
        if (selected.has(c)) ce.classList.add('selected');
        ce.addEventListener('click', () => toggle(c));
      }
      handEl.appendChild(ce);
    }
    el.appendChild(handEl);
  }

  function renderPile() {
    const pile = $('pile');
    pile.innerHTML = '';
    const lp = game.lastPlay;
    if (lp) {
      for (const c of lp.combo.cards) pile.appendChild(cardEl(c));
      $('pile-label').textContent = `${NAMES[lp.player]}: ${R.describe(lp.combo)}`;
    } else if (!game.over) {
      $('pile-label').textContent = `${NAMES[game.turn]} đánh trước`;
    } else {
      $('pile-label').textContent = '';
    }
  }

  function renderScores() {
    $('scores').innerHTML = NAMES.map((n, p) => `<tr><td>${n}</td><td>${game.scores[p]}</td></tr>`).join('');
  }

  function renderControls() {
    const myTurn = !game.over && game.turn === HUMAN;
    $('btn-play').disabled = !myTurn || selected.size === 0;
    $('btn-pass').disabled = !myTurn || !game.lastPlay;
    $('btn-hint').disabled = !myTurn;
    $('btn-clear').disabled = selected.size === 0;
  }

  function status(msg) {
    $('status').textContent = msg || '';
  }

  function log(html, cls) {
    const li = document.createElement('li');
    li.innerHTML = html;
    if (cls) li.className = cls;
    $('log').appendChild(li);
    li.scrollIntoView({ block: 'nearest' });
  }

  // Ghi lại các sự kiện mới trong game.history kể từ lần ghi trước.
  let logged = 0;
  function flushLog() {
    for (; logged < game.history.length; logged++) {
      const h = game.history[logged];
      if (h.newRound) log(`— Vòng mới: ${NAMES[h.player]} đánh —`, 'round');
      else if (h.pass) log(`${NAMES[h.player]} bỏ lượt`);
      else {
        const cards = h.combo.cards.map(R.cardName).join(' ');
        log(`${NAMES[h.player]}: ${R.describe(h.combo)} <b>${cards}</b>${h.chop ? ' — CHẶT!' : ''}`, h.chop ? 'chop' : '');
      }
    }
  }

  function toggle(c) {
    if (game.over || game.turn !== HUMAN) return;
    selected.has(c) ? selected.delete(c) : selected.add(c);
    status('');
    render();
  }

  function afterMove() {
    flushLog();
    selected = new Set([...selected].filter((c) => game.hands[HUMAN].includes(c)));
    render();
    if (game.over) return showResult();
    if (game.turn === HUMAN) {
      status(game.mustInclude != null ? `Lượt của bạn — phải đánh kèm ${R.cardName(game.mustInclude)}` : 'Lượt của bạn');
    } else {
      status(`${NAMES[game.turn]} đang nghĩ...`);
      botTimer = setTimeout(botMove, BOT_DELAY);
    }
  }

  function botMove() {
    const p = game.turn;
    const move = AI.chooseMove(game.hands[p], game.context(p));
    if (move) game.play(p, move);
    else game.pass(p);
    afterMove();
  }

  function humanPlay() {
    const err = game.play(HUMAN, [...selected]);
    if (err) return status(err);
    selected.clear();
    afterMove();
  }

  function humanPass() {
    const err = game.pass(HUMAN);
    if (err) return status(err);
    selected.clear();
    afterMove();
  }

  function hint() {
    const move = AI.chooseMove(game.hands[HUMAN], game.context(HUMAN));
    if (!move) return status('Gợi ý: nên bỏ lượt');
    selected = new Set(move);
    status('Gợi ý: ' + R.describe(R.getCombo(move)));
    render();
  }

  function showResult() {
    status('');
    if (game.instant) {
      $('result-title').textContent = `${NAMES[game.instant.player]} tới trắng (${game.instant.reason})!`;
      $('result-list').innerHTML = '';
    } else {
      $('result-title').textContent = game.finished[0] === HUMAN ? 'Bạn về nhất!' : 'Kết thúc ván';
      $('result-list').innerHTML = game.finished.map((p, i) => `<li>${PLACES[i]}: ${NAMES[p]}</li>`).join('');
    }
    // Để người chơi xem bài còn lại của các máy trước khi mở hộp kết quả.
    setTimeout(() => $('overlay').classList.remove('hidden'), 1200);
  }

  function newGame() {
    clearTimeout(botTimer);
    $('overlay').classList.add('hidden');
    selected.clear();
    game.start();
    log(`<b>Ván ${game.gameNo}</b>`, 'round');
    logged = game.history.length;
    afterMove();
  }

  $('btn-play').addEventListener('click', humanPlay);
  $('btn-pass').addEventListener('click', humanPass);
  $('btn-hint').addEventListener('click', hint);
  $('btn-clear').addEventListener('click', () => { selected.clear(); render(); });
  $('btn-new').addEventListener('click', newGame);
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !$('btn-play').disabled) humanPlay();
    else if (e.key === ' ' && !$('btn-pass').disabled) { e.preventDefault(); humanPass(); }
  });

  newGame();
})();
