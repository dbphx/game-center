// Client Tiến Lên: mọi luật do server Go quyết định, client chỉ hiển thị và gửi hành động.
(function () {
  'use strict';
  const R = window.Rules;
  const A = window.Account;
  const PLACES = ['Nhất', 'Nhì', 'Ba', 'Bét'];
  // Vị trí ghế theo số người, tính từ mình (luôn ở dưới) theo chiều đánh.
  const POSITIONS = { 2: ['bottom', 'top'], 3: ['bottom', 'right', 'left'], 4: ['bottom', 'right', 'top', 'left'] };
  const ROOM_KEY = 'tlmn.room'; // phòng đang ngồi, để tải lại trang thì vào lại đúng ghế

  const $ = (id) => document.getElementById(id);

  let ws = null;
  let state = null;
  let selected = new Set();
  let pending = null; // tin nhắn chờ gửi khi kết nối xong
  let deadline = 0; // thời điểm hết lượt (giờ máy mình)
  let showLobby = false; // đã hết ván nhưng người chơi bấm sang phòng chờ
  let stopped = false; // bị tab khác thay thế thì không tự kết nối lại

  // ---------- Lưu trữ (có thể bị chặn ở chế độ ẩn danh) ----------

  const store = (s, k, v) => { try { v == null ? s.removeItem(k) : s.setItem(k, v); } catch (e) { /* bỏ qua */ } };
  const load = (s, k) => { try { return s.getItem(k); } catch (e) { return null; } };

  // ---------- Kết nối ----------

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/ws/tienlen`);
    ws.onopen = () => {
      const code = load(sessionStorage, ROOM_KEY) || new URLSearchParams(location.search).get('room');
      if (code) send({ t: 'join', code, token: A.token() });
      else if (pending) send(pending);
      pending = null;
    };
    ws.onmessage = (e) => onMessage(JSON.parse(e.data));
    ws.onclose = () => {
      if (stopped) return;
      if (state) status('Mất kết nối, đang kết nối lại...');
      setTimeout(connect, 1000);
    };
  }

  function send(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
    else pending = msg;
  }

  function onMessage(m) {
    switch (m.t) {
      case 'session':
        store(sessionStorage, ROOM_KEY, m.code);
        history.replaceState(null, '', '?room=' + m.code);
        break;
      case 'auth':
        stopped = true;
        A.requireLogin();
        break;
      case 'state':
        onState(m);
        break;
      case 'hint':
        if (!m.cards) return status('Gợi ý: nên bỏ lượt');
        selected = new Set(m.cards);
        status('Gợi ý: ' + R.describe(R.getCombo(m.cards)));
        render();
        break;
      case 'error':
        A.toast(m.msg);
        break;
      case 'left':
        store(sessionStorage, ROOM_KEY, null);
        state = null;
        history.replaceState(null, '', location.pathname);
        render();
        break;
      case 'replaced':
        stopped = true;
        state = null;
        render();
        A.toast('Bạn đã mở phòng này ở tab khác');
        break;
    }
  }

  function onState(s) {
    const prevPhase = state && state.phase;
    state = s;
    if (s.phase === 'playing') showLobby = false;
    if (s.phase === 'playing' && prevPhase !== 'playing') selected.clear();
    selected = new Set([...selected].filter((c) => s.hand.includes(c)));
    deadline = s.turnMs > 0 ? Date.now() + s.turnMs : 0;
    showBalance(me().coins);
    render();
  }

  // ---------- Tiện ích ----------

  const me = () => state.players.find((p) => p.id === state.you);
  const mySeat = () => state.players.findIndex((p) => p.id === state.you);
  const isHost = () => !!(state && me() && me().host);
  const myTurn = () => state.phase === 'playing' && state.turn === mySeat();
  const xu = A.xu;
  const signed = (n) => (n > 0 ? '+' : '') + xu(n);
  const escapeHtml = A.escapeHtml;
  const coinsText = (p) => (p.bot ? '—' : xu(p.coins) + ' xu');

  function showBalance(coins) {
    $('balance').textContent = '💰 ' + xu(coins) + ' xu';
  }

  function status(msg) {
    $('status').textContent = msg || '';
  }

  function cardEl(c, opts = {}) {
    const el = document.createElement('div');
    if (opts.back) {
      el.className = 'card back' + (opts.small ? ' small' : '');
      return el;
    }
    el.className = 'card' + (R.suitOf(c) >= 2 ? ' red' : '') + (opts.small ? ' small' : '');
    const rank = R.RANKS[R.rankOf(c)];
    const suit = R.SUITS[R.suitOf(c)];
    el.innerHTML = `<div class="corner">${rank}<br>${suit}</div><div class="pip">${suit}</div>`;
    return el;
  }

  // ---------- Hiển thị ----------

  function render() {
    let screen = 'home';
    if (state) screen = state.phase === 'lobby' || (state.phase === 'over' && showLobby) ? 'lobby' : 'game';
    for (const id of ['home', 'lobby', 'game']) $(id).classList.toggle('hidden', id !== screen);
    document.querySelectorAll('.host-only').forEach((el) => el.classList.toggle('hidden', !isHost()));
    document.querySelectorAll('.guest-only').forEach((el) => el.classList.toggle('hidden', !state || isHost()));

    if (screen === 'lobby') renderLobby();
    if (screen === 'game') renderGame();
  }

  function renderLobby() {
    $('lobby-code').textContent = state.code;
    $('share-link').value = `${location.origin}${location.pathname}?room=${state.code}`;
    const host = isHost();
    const rows = state.players.map((p) => {
      const tags = [
        p.id === state.you ? '<span class="tag me">Bạn</span>' : '',
        p.host ? '<span class="tag host">Chủ phòng</span>' : '',
        p.bot ? '<span class="tag bot">Máy</span>' : '',
        !p.online || p.left ? '<span class="tag off">Mất kết nối</span>' : '',
      ].join('');
      const kick = host && p.id !== state.you ? `<button class="kick" data-kick="${p.id}">Mời ra</button>` : '';
      return `<li><span class="pname">${escapeHtml(p.name)}</span>${tags}<span class="coins${!p.bot && p.coins < state.bet ? ' neg' : ''}">${coinsText(p)}</span>${kick}</li>`;
    });
    for (let i = state.players.length; i < 4; i++) rows.push('<li class="empty">Ghế trống</li>');
    $('lobby-players').innerHTML = rows.join('');
    const sel = $('bet');
    if (sel.options.length !== state.betOptions.length) {
      sel.innerHTML = state.betOptions.map((b) => `<option value="${b}">${xu(b)}</option>`).join('');
    }
    sel.value = state.bet;
    $('bet-text').textContent = xu(state.bet);
    $('low-coins').classList.toggle('hidden', me().coins >= state.bet);
    $('btn-add-bot').disabled = state.players.length >= 4;
    $('btn-start').disabled = state.players.filter((p) => !p.left).length < 2;
  }

  function renderGame() {
    const n = state.players.length;
    const mine = mySeat();
    document.querySelectorAll('.seat').forEach((el) => (el.innerHTML = ''));
    state.players.forEach((p, i) => renderSeat(p, i, POSITIONS[n][(i - mine + n) % n]));
    renderPile();
    renderControls();
    renderSide();
    renderResult();
  }

  function renderSeat(p, i, pos) {
    const el = document.querySelector(`.seat[data-pos="${pos}"]`);
    const isMe = p.id === state.you;
    const playing = state.phase === 'playing';
    el.className = 'seat';
    el.classList.toggle('turn', playing && state.turn === i);
    el.classList.toggle('done', playing && (p.place >= 0 || p.cong));
    el.classList.toggle('offline', !p.online);

    const name = document.createElement('div');
    name.className = 'name';
    name.textContent = isMe ? 'Bạn' : p.name;
    const badge = (cls, text, id) => {
      const b = document.createElement('span');
      b.className = 'badge ' + cls;
      b.textContent = text;
      if (id) b.id = id;
      name.appendChild(b);
    };
    if (p.bot) badge('', 'Máy');
    if (!p.online && !p.bot) badge('pass', p.left ? 'Đã rời' : 'Mất kết nối');
    if (p.cong) badge('cong', 'Cóng');
    else if (p.place >= 0) badge('place', PLACES[p.place]);
    else badge('', p.cards + ' lá');
    if (p.passed && playing) badge('pass', 'Bỏ lượt');
    if (playing && state.turn === i && deadline) badge('timer', '', 'turn-timer');
    el.appendChild(name);

    const hand = document.createElement('div');
    hand.className = 'hand';
    if (isMe) {
      hand.classList.add('mine');
      hand.classList.toggle('locked', !myTurn());
      for (const c of state.hand) {
        const ce = cardEl(c);
        if (selected.has(c)) ce.classList.add('selected');
        ce.addEventListener('click', () => toggle(c));
        hand.appendChild(ce);
      }
    } else if (p.reveal) {
      for (const c of p.reveal) hand.appendChild(cardEl(c, { small: true }));
    } else {
      for (let k = 0; k < p.cards; k++) hand.appendChild(cardEl(0, { back: true, small: true }));
    }
    el.appendChild(hand);
    updateTimer();
  }

  function playerName(i) {
    const p = state.players[i];
    return p.id === state.you ? 'Bạn' : p.name;
  }

  function renderPile() {
    const pile = $('pile');
    pile.innerHTML = '';
    const lp = state.lastPlay;
    if (lp) {
      for (const c of lp.combo.cards) pile.appendChild(cardEl(c));
      $('pile-label').textContent = `${playerName(lp.player)}: ${R.describe(lp.combo)}`;
    } else if (state.phase === 'playing') {
      $('pile-label').textContent = `${playerName(state.turn)} được đánh tự do`;
    } else {
      $('pile-label').textContent = '';
    }
  }

  function renderControls() {
    const turn = myTurn();
    $('btn-play').disabled = !turn || selected.size === 0;
    $('btn-pass').disabled = !turn || !state.lastPlay;
    $('btn-hint').disabled = !turn;
    $('btn-clear').disabled = selected.size === 0;
    if (state.phase !== 'playing') status('');
    else if (turn) {
      status(state.mustInclude >= 0 ? `Lượt của bạn — ván đầu phải đánh kèm ${R.cardName(state.mustInclude)}` : 'Lượt của bạn');
    } else status(`Đợi ${playerName(state.turn)}...`);
  }

  function renderSide() {
    $('game-code').textContent = state.code;
    $('game-bet').textContent = `Cược ${xu(state.gameBet)} xu / mức`;
    $('scores').innerHTML = state.players
      .map((p) => `<tr><td>${escapeHtml(p.id === state.you ? 'Bạn' : p.name)}</td><td class="coins">${coinsText(p)}</td></tr>`)
      .join('');

    const items = [`<li class="round"><b>Ván ${state.gameNo}</b></li>`];
    for (const h of state.history || []) {
      const who = escapeHtml(playerName(h.player));
      if (h.newRound) items.push(`<li class="round">— Vòng mới: ${who} đánh —</li>`);
      else if (h.pass) items.push(`<li>${who} bỏ lượt</li>`);
      else {
        const cards = h.combo.cards.map(R.cardName).join(' ');
        items.push(`<li class="${h.chop ? 'chop' : ''}">${who}: ${R.describe(h.combo)} <b>${cards}</b>${h.chop ? ' — CHẶT!' : ''}</li>`);
        if (h.pay) items.push(`<li class="pay">💰 ${transferText(h.pay)}</li>`);
      }
    }
    const log = $('log');
    const atBottom = log.scrollHeight - log.scrollTop - log.clientHeight < 40;
    log.innerHTML = items.join('');
    if (atBottom) log.scrollTop = log.scrollHeight;
  }

  function renderResult() {
    const over = state.phase === 'over';
    $('overlay').classList.toggle('hidden', !over);
    if (!over) return;
    if (state.instant) {
      $('result-title').textContent = `${playerName(state.instant.player)} tới trắng (${state.instant.reason})!`;
    } else {
      $('result-title').textContent = state.players[mySeat()].place === 0 ? 'Bạn về nhất!' : 'Kết thúc ván';
    }
    const order = state.players
      .map((p, i) => ({ i, p, place: p.place < 0 ? 99 : p.place, money: state.money ? state.money[i] : 0 }))
      .sort((a, b) => a.place - b.place || b.money - a.money);
    $('result-list').innerHTML = order
      .map(({ i, p, place, money }) => {
        const label = p.cong ? 'Cóng: ' : place < 4 && !state.instant ? PLACES[place] + ': ' : '';
        const cls = money > 0 ? 'delta-plus' : money < 0 ? 'delta-minus' : '';
        return `<li><span>${label}${escapeHtml(playerName(i))}</span><span class="${cls}">${signed(money)} xu</span></li>`;
      })
      .join('');

    // Chi tiết: tiền thứ hạng rồi tới từng khoản chặt / thối.
    const lines = [];
    if (state.delta) {
      const rank = state.players.map((_, i) => `${escapeHtml(playerName(i))} ${signed(state.delta[i] * state.gameBet)}`);
      lines.push(`<li>${state.instant ? 'Tới trắng' : 'Thứ hạng'}: ${rank.join(', ')}</li>`);
    }
    for (const t of state.transfers || []) lines.push(`<li>${transferText(t)}</li>`);
    $('result-transfers').innerHTML = lines.join('');
  }

  // "An chặt heo Bình: +20 xu" / "Bình thối 1 heo đỏ cho An: 20 xu"
  function transferText(t) {
    const from = escapeHtml(playerName(t.from));
    const to = escapeHtml(playerName(t.to));
    const amount = xu(t.units * state.gameBet) + ' xu';
    if (t.reason.startsWith('thối')) return `${from} ${t.reason} cho ${to}: ${amount}`;
    return `${to} ${t.reason} ${from}: ${amount}`;
  }

  function updateTimer() {
    const el = $('turn-timer');
    if (!el) return;
    const left = Math.max(0, Math.ceil((deadline - Date.now()) / 1000));
    el.textContent = `⏱ ${left}s`;
    el.classList.toggle('low', left <= 10);
  }
  setInterval(updateTimer, 250);

  // ---------- Hành động ----------

  function toggle(c) {
    if (!myTurn()) return;
    selected.has(c) ? selected.delete(c) : selected.add(c);
    render();
  }

  function leave() {
    if (state && state.phase === 'playing' && !confirm('Rời phòng giữa ván? Máy sẽ đánh thay bạn tới hết ván.')) return;
    send({ t: 'leave' });
  }

  $('btn-create').addEventListener('click', () => send({ t: 'create', token: A.token() }));
  $('btn-join').addEventListener('click', () => {
    const code = $('code').value.trim().toUpperCase();
    if (code.length !== 4) return A.toast('Mã phòng gồm 4 chữ cái');
    send({ t: 'join', code, token: A.token() });
  });
  $('code').addEventListener('keydown', (e) => e.key === 'Enter' && $('btn-join').click());

  $('btn-copy').addEventListener('click', () => {
    $('share-link').select();
    navigator.clipboard?.writeText($('share-link').value).then(() => A.toast('Đã sao chép link'), () => {});
  });
  $('lobby-players').addEventListener('click', (e) => {
    const id = e.target.dataset && e.target.dataset.kick;
    if (id) send({ t: 'kick', id });
  });
  $('btn-add-bot').addEventListener('click', () => send({ t: 'addBot' }));
  $('bet').addEventListener('change', (e) => send({ t: 'setBet', bet: Number(e.target.value) }));
  $('btn-start').addEventListener('click', () => send({ t: 'start' }));
  $('btn-again').addEventListener('click', () => send({ t: 'start' }));
  $('btn-to-lobby').addEventListener('click', () => { showLobby = true; render(); });
  $('btn-leave-lobby').addEventListener('click', leave);
  $('btn-leave-game').addEventListener('click', leave);

  $('btn-play').addEventListener('click', () => {
    if (!selected.size) return;
    const cards = [...selected];
    if (!R.getCombo(cards)) return status('Bộ bài không hợp lệ');
    send({ t: 'play', cards });
  });
  $('btn-pass').addEventListener('click', () => send({ t: 'pass' }));
  $('btn-hint').addEventListener('click', () => send({ t: 'hint' }));
  $('btn-clear').addEventListener('click', () => { selected.clear(); render(); });
  document.addEventListener('keydown', (e) => {
    if (!state || state.phase !== 'playing' || e.target.tagName === 'INPUT') return;
    if (e.key === 'Enter' && !$('btn-play').disabled) $('btn-play').click();
    else if (e.key === ' ' && !$('btn-pass').disabled) { e.preventDefault(); $('btn-pass').click(); }
  });

  // ---------- Khởi động ----------

  // Chưa có tài khoản thì về hub tạo, xong quay lại đúng link phòng này.
  A.me().then(
    (user) => {
      if (!user) return A.requireLogin();
      $('user-name').textContent = user.name;
      showBalance(user.balance);
      render();
      connect();
    },
    (e) => A.toast(e.message),
  );
})();
