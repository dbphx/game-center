// Client online: mọi luật do server Go quyết định, client chỉ hiển thị và gửi hành động.
(function () {
  'use strict';
  const R = window.Rules;
  const PLACES = ['Nhất', 'Nhì', 'Ba', 'Bét'];
  // Vị trí ghế theo số người, tính từ mình (luôn ở dưới) theo chiều đánh.
  const POSITIONS = { 2: ['bottom', 'top'], 3: ['bottom', 'right', 'left'], 4: ['bottom', 'right', 'top', 'left'] };
  const TOKEN_KEY = 'tlmn.token';
  const NAME_KEY = 'tlmn.name';

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
    ws = new WebSocket(`${proto}://${location.host}/ws`);
    ws.onopen = () => {
      const token = load(sessionStorage, TOKEN_KEY);
      if (token) send({ t: 'resume', token });
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
        store(sessionStorage, TOKEN_KEY, m.token);
        history.replaceState(null, '', '?room=' + m.code);
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
        toast(capitalize(m.msg));
        break;
      case 'left':
        store(sessionStorage, TOKEN_KEY, null);
        state = null;
        history.replaceState(null, '', location.pathname);
        render();
        break;
      case 'replaced':
        stopped = true;
        state = null;
        render();
        toast('Bạn đã mở phòng này ở tab khác');
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
    render();
  }

  // ---------- Tiện ích ----------

  const capitalize = (s) => (s ? s[0].toUpperCase() + s.slice(1) : s);
  const me = () => state.players.find((p) => p.id === state.you);
  const mySeat = () => state.players.findIndex((p) => p.id === state.you);
  const isHost = () => !!(state && me() && me().host);
  const myTurn = () => state.phase === 'playing' && state.turn === mySeat();
  const escapeHtml = (s) => s.replace(/[&<>"']/g, (ch) => `&#${ch.charCodeAt(0)};`);

  let toastTimer = null;
  function toast(msg) {
    const el = $('toast');
    el.textContent = msg;
    el.classList.remove('hidden');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.add('hidden'), 3000);
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
      return `<li><span class="pname">${escapeHtml(p.name)}</span>${tags}<span class="muted">${p.score} điểm</span>${kick}</li>`;
    });
    for (let i = state.players.length; i < 4; i++) rows.push('<li class="empty">Ghế trống</li>');
    $('lobby-players').innerHTML = rows.join('');
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
    el.classList.toggle('done', playing && p.place >= 0);
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
    if (p.place >= 0) badge('place', PLACES[p.place]);
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
    $('scores').innerHTML = state.players
      .map((p) => `<tr><td>${escapeHtml(p.id === state.you ? 'Bạn' : p.name)}</td><td>${p.score}</td></tr>`)
      .join('');

    const items = [`<li class="round"><b>Ván ${state.gameNo}</b></li>`];
    for (const h of state.history || []) {
      const who = escapeHtml(playerName(h.player));
      if (h.newRound) items.push(`<li class="round">— Vòng mới: ${who} đánh —</li>`);
      else if (h.pass) items.push(`<li>${who} bỏ lượt</li>`);
      else {
        const cards = h.combo.cards.map(R.cardName).join(' ');
        items.push(`<li class="${h.chop ? 'chop' : ''}">${who}: ${R.describe(h.combo)} <b>${cards}</b>${h.chop ? ' — CHẶT!' : ''}</li>`);
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
      .map((p, i) => ({ i, place: p.place < 0 ? 99 : p.place, delta: state.delta ? state.delta[i] : 0 }))
      .sort((a, b) => a.place - b.place || b.delta - a.delta);
    $('result-list').innerHTML = order
      .map(({ i, place, delta }) => {
        const label = place < 4 && !state.instant ? PLACES[place] + ': ' : '';
        const cls = delta > 0 ? 'delta-plus' : delta < 0 ? 'delta-minus' : '';
        return `<li>${label}${escapeHtml(playerName(i))} <span class="${cls}">${delta > 0 ? '+' : ''}${delta}</span></li>`;
      })
      .join('');
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

  function playerNameInput() {
    const name = $('name').value.trim();
    if (!name) {
      $('name').focus();
      toast('Nhập tên trước đã');
      return null;
    }
    store(localStorage, NAME_KEY, name);
    return name;
  }

  function leave() {
    if (state && state.phase === 'playing' && !confirm('Rời phòng giữa ván? Máy sẽ đánh thay bạn tới hết ván.')) return;
    send({ t: 'leave' });
  }

  $('btn-create').addEventListener('click', () => {
    const name = playerNameInput();
    if (name) send({ t: 'create', name });
  });
  $('btn-join').addEventListener('click', () => {
    const name = playerNameInput();
    const code = $('code').value.trim().toUpperCase();
    if (!name) return;
    if (code.length !== 4) return toast('Mã phòng gồm 4 chữ cái');
    send({ t: 'join', name, code });
  });
  $('code').addEventListener('keydown', (e) => e.key === 'Enter' && $('btn-join').click());
  $('name').addEventListener('keydown', (e) => e.key === 'Enter' && ($('code').value ? $('btn-join') : $('btn-create')).click());

  $('btn-copy').addEventListener('click', () => {
    $('share-link').select();
    navigator.clipboard?.writeText($('share-link').value).then(() => toast('Đã sao chép link'), () => {});
  });
  $('lobby-players').addEventListener('click', (e) => {
    const id = e.target.dataset && e.target.dataset.kick;
    if (id) send({ t: 'kick', id });
  });
  $('btn-add-bot').addEventListener('click', () => send({ t: 'addBot' }));
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

  $('name').value = load(localStorage, NAME_KEY) || '';
  const roomParam = new URLSearchParams(location.search).get('room');
  if (roomParam) $('code').value = roomParam.toUpperCase();
  render();
  connect();
})();
