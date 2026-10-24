// Client Tiến Lên: mọi luật do server Go quyết định, client chỉ hiển thị và gửi hành động.
(function () {
  'use strict';
  const R = window.Rules;
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  // Avatar là mặt nhân vật của người chơi; máy dùng icon robot.
  const face = (p) => (p.bot ? I.avatar('', { bot: true }) : C.head({ gender: p.gender, outfit: p.outfit }));
  const lv = (p) => (p.bot || !p.level ? '' : `<span class="lv-chip">Lv ${p.level}</span>`);
  const PLACES = ['Nhất', 'Nhì', 'Ba', 'Bét'];
  // Vị trí ghế theo số người, tính từ mình (luôn ở dưới) theo chiều đánh.
  const POSITIONS = { 2: ['bottom', 'top'], 3: ['bottom', 'right', 'left'], 4: ['bottom', 'right', 'top', 'left'] };
  const ROOM_KEY = 'tlmn.room'; // phòng đang ngồi, để tải lại trang thì vào lại đúng ghế
  const DEFAULT_BETS = [0, 10, 20, 50, 100, 200, 500];
  const DEFAULT_BET = 10;
  // Mức 0 là bàn chơi vui, không thắng thua xu.
  const betText = (b) => (b ? `${xu(b)} xu/mức` : 'Chơi vui · 0 xu');
  const HALL_REFRESH = 3000;
  const SEATS = ['bottom', 'left', 'top', 'right']; // vị trí 4 ghế quanh bàn trong sảnh

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

  // Bàn vừa tạo từ sảnh: nếu server chưa áp mức cược đã chọn thì chủ bàn tự đặt lại.
  let pendingBet = null;

  function onState(s) {
    const prevPhase = state && state.phase;
    state = s;
    if (s.phase === 'playing') showLobby = false;
    if (s.phase === 'playing' && prevPhase !== 'playing') selected.clear();
    selected = new Set([...selected].filter((c) => s.hand.includes(c)));
    deadline = s.turnMs > 0 ? Date.now() + s.turnMs : 0;
    if (pendingBet !== null && s.phase === 'lobby') {
      const mine = s.players.find((p) => p.id === s.you);
      if (mine && mine.host && s.bet !== pendingBet) send({ t: 'setBet', bet: pendingBet });
      pendingBet = null;
    }
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
    $('balance').textContent = xu(coins);
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
    const corner = `${rank}<br>${suit}`;
    el.innerHTML = `<div class="corner">${corner}</div><div class="pip">${suit}</div><div class="corner br">${corner}</div>`;
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
    setHallPolling(screen === 'home');
  }

  // ---------- Sảnh casino ----------

  // Hình sảnh là ảnh hoạt hình trong /img/toon/casino/ (xem manifest.json ở đó): sàn thảm, bàn nỉ, ghế 4 hướng,
  // chậu cây. Mỗi bàn là một cảnh 360x300 (CSS nhân --s); người ngồi là nhân vật chibi (Character.sprite).
  const IMG = '/img/toon/casino/';
  // Ghế quay lưng ra ngoài bàn; nhân vật ngồi nhìn vào bàn.
  const CHAIR = { top: 'chair_down', bottom: 'chair_up', left: 'chair_left', right: 'chair_right' };
  const FACE = { top: 'down', bottom: 'up', left: 'right', right: 'left' };
  const img = (name, cls) => `<img class="art ${cls}" src="${IMG}${name}.png" alt="" draggable="false">`;
  const scene = (inner, cls = '') => `<div class="tscene${cls}">
      ${img('plant', 'art-plant')}${img('table_felt', 'art-table')}
      ${inner}
    </div>`;

  let rooms = null; // null: chưa tải lần nào
  let hallTimer = null;

  function setHallPolling(on) {
    if (on && !hallTimer) {
      loadRooms();
      hallTimer = setInterval(loadRooms, HALL_REFRESH);
    } else if (!on && hallTimer) {
      clearInterval(hallTimer);
      hallTimer = null;
    }
  }

  async function loadRooms() {
    try {
      rooms = await A.api('/api/tienlen/rooms');
    } catch (e) {
      if (rooms === null) rooms = []; // lỗi mạng: giữ dữ liệu cũ, lần đầu thì coi như chưa có bàn
    }
    if (!state) renderHall();
  }

  // Máy chưa có nhân vật riêng: dùng nhân vật vest để ngồi bàn, kèm chip "Máy".
  const BOT_LOOK = { gender: 'male', outfit: 'suit' };

  function seatHtml(p, pos, room, open) {
    const chair = img(CHAIR[pos], 'art-chair');
    if (p) {
      const look = p.bot ? BOT_LOOK : { gender: p.gender, outfit: p.outfit };
      const body = C.sprite(look, { dir: FACE[pos], size: 88, cls: 'seat-char' });
      const crown = p.host ? `<span class="seat-crown" title="Chủ bàn">${I.svg('crown')}</span>` : '';
      const lvl = p.bot || !p.level ? '<span class="lv-chip bot-chip">Máy</span>' : `<span class="lv-chip">Lv ${p.level}</span>`;
      return `<div class="tseat ${pos} filled${p.online === false && !p.bot ? ' away' : ''}" title="${escapeHtml(p.name)}">
          ${chair}<span class="seat-body">${body}</span>
          <span class="seat-label">${crown}<span class="seat-name">${escapeHtml(p.name)}</span>${lvl}</span>
        </div>`;
    }
    if (!open) return `<div class="tseat ${pos} empty closed">${chair}</div>`;
    return `<button class="tseat ${pos} empty" data-join="${room.code}" title="Ngồi vào bàn ${room.code}">
        ${chair}<span class="seat-join">${I.svg('plus')}Ngồi</span>
      </button>`;
  }

  function tableHtml(room) {
    const players = room.players || [];
    const max = room.max || 4;
    const playing = room.phase === 'playing';
    const full = players.length >= max;
    const open = !playing && !full;
    const status = playing ? `Đang chơi · ván ${room.gameNo || 1}` : full ? 'Đủ người' : `Đang chờ · ${players.length}/${max}`;
    const note = playing ? 'Đợi hết ván để vào' : full ? 'Bàn đã đủ 4 người' : '';
    const seats = SEATS.map((pos, i) => (i < max ? seatHtml(players[i], pos, room, open) : '')).join('');
    return `<div class="ttable${open ? '' : ' busy'}${playing ? ' playing' : ''}"${note ? ` title="${note}"` : ''}>
        ${scene(`${seats}
          <div class="tfelt">
            <span class="tcode">${escapeHtml(room.code)}</span>
            <span class="tbet">${I.svg('coins')}${betText(room.bet ?? DEFAULT_BET)}</span>
            <span class="tstatus${playing ? ' live' : ''}">${status}</span>
            ${note ? `<span class="tnote">${note}</span>` : ''}
          </div>`)}
      </div>`;
  }

  function renderHall() {
    const el = $('tables');
    if (rooms === null) {
      el.innerHTML = '<p class="hall-empty muted">Đang tải các bàn...</p>';
      return;
    }
    // Bàn còn ghế trống lên trước, rồi tới bàn đang chơi.
    const order = (r) => (r.phase === 'playing' ? 2 : (r.players || []).length >= (r.max || 4) ? 1 : 0);
    const list = rooms.slice().sort((a, b) => order(a) - order(b));
    const invite = `<div class="ttable invite">
        ${scene(`${SEATS.map((pos) => `<div class="tseat ${pos} empty closed">${img(CHAIR[pos], 'art-chair')}</div>`).join('')}
          <div class="tfelt"><button class="primary" data-create>${I.svg('plus')}${list.length ? 'Mở thêm bàn' : 'Tạo bàn đầu tiên'}</button>
            <span class="tnote">${list.length ? 'Chọn mức cược tuỳ ý' : 'Chưa có bàn nào, mở bàn và mời bạn bè'}</span></div>`)}
      </div>`;
    el.innerHTML = list.map(tableHtml).join('') + invite;
  }

  function openCreate() {
    const bets = (state && state.betOptions) || DEFAULT_BETS;
    const grid = $('bet-grid');
    grid.innerHTML = bets
      .map((b) => `<label class="bet-opt"><input type="radio" name="bet" value="${b}" ${b === DEFAULT_BET ? 'checked' : ''}>
          <span>${I.svg('coins')}${b ? xu(b) : 'Chơi vui'}<small>${b ? 'xu/mức' : '0 xu'}</small></span></label>`)
      .join('');
    const dlg = $('dlg-create');
    const form = dlg.querySelector('form');
    form.onsubmit = (e) => {
      if (e.submitter && e.submitter.value === 'cancel') return;
      const v = new FormData(form).get('bet');
      const bet = v === null ? DEFAULT_BET : Number(v);
      pendingBet = bet;
      send({ t: 'create', token: A.token(), bet });
    };
    dlg.showModal();
  }

  function renderLobby() {
    $('lobby-code').textContent = state.code;
    $('share-link').value = `${location.origin}${location.pathname}?room=${state.code}`;
    const host = isHost();
    const rows = state.players.map((p) => {
      const tags = [
        p.id === state.you ? '<span class="tag me">Bạn</span>' : '',
        p.host ? `<span class="tag host">${I.svg('crown')}Chủ phòng</span>` : '',
        p.bot ? `<span class="tag bot">${I.svg('bot')}Máy</span>` : '',
        !p.online || p.left ? `<span class="tag off">${I.svg('wifiOff')}Mất kết nối</span>` : '',
      ].join('');
      const kick =
        host && p.id !== state.you
          ? `<button class="kick link" data-kick="${p.id}" title="Mời ra">${I.svg('x')}</button>`
          : '';
      const coins = p.bot ? '' : `<span class="coins${p.coins < state.bet ? ' neg' : ''}">${I.svg('coins')}${xu(p.coins)}</span>`;
      return `<li>${face(p)}<span class="pname">${escapeHtml(p.name)}${lv(p)}${tags}</span>${coins}${kick}</li>`;
    });
    for (let i = state.players.length; i < 4; i++) rows.push(`<li class="empty">${I.svg('userPlus')}Ghế trống</li>`);
    $('lobby-players').innerHTML = rows.join('');
    const sel = $('bet');
    if (sel.options.length !== state.betOptions.length) {
      sel.innerHTML = state.betOptions.map((b) => `<option value="${b}">${b ? xu(b) : '0 (chơi vui)'}</option>`).join('');
    }
    sel.value = state.bet;
    $('bet-text').textContent = state.bet ? xu(state.bet) : '0 (chơi vui)';
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
    name.innerHTML = `${face(p)}<span>${escapeHtml(isMe ? 'Bạn' : p.name)}</span>${lv(p)}`;
    const badge = (cls, text, icon, id) => {
      const b = document.createElement('span');
      b.className = 'badge ' + cls;
      b.innerHTML = (icon ? I.svg(icon) : '') + `<span>${escapeHtml(text)}</span>`;
      if (id) b.id = id;
      name.appendChild(b);
    };
    if (!p.online && !p.bot) badge('pass', p.left ? 'Đã rời' : 'Mất kết nối', 'wifiOff');
    if (p.cong) badge('cong', 'Cóng');
    else if (p.place >= 0) badge('place', PLACES[p.place], p.place === 0 ? 'trophy' : '');
    else badge('', p.cards + ' lá', 'layers');
    if (p.passed && playing) badge('pass', 'Bỏ lượt', 'skip');
    if (playing && state.turn === i && deadline) badge('timer', '', 'timer', 'turn-timer');
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
    $('game-bet').innerHTML = `${I.svg('coins')}${state.gameBet ? `${xu(state.gameBet)} / mức` : 'Chơi vui'}`;
    $('scores').innerHTML = state.players
      .map(
        (p) =>
          `<tr><td>${face(p)}${escapeHtml(p.id === state.you ? 'Bạn' : p.name)}</td>` +
          `<td><span class="coins">${coinsText(p)}</span></td></tr>`,
      )
      .join('');

    const items = [`<li class="round"><b>Ván ${state.gameNo}</b></li>`];
    for (const h of state.history || []) {
      const who = escapeHtml(playerName(h.player));
      if (h.newRound) items.push(`<li class="round">— Vòng mới: ${who} đánh —</li>`);
      else if (h.pass) items.push(`<li>${who} bỏ lượt</li>`);
      else {
        const cards = h.combo.cards.map(R.cardName).join(' ');
        const chop = h.chop ? ` ${I.svg('flame')} CHẶT!` : '';
        items.push(`<li class="${h.chop ? 'chop' : ''}">${who}: ${R.describe(h.combo)} <b>${cards}</b>${chop}</li>`);
        if (h.pay) items.push(`<li class="pay">${I.svg('coins')}${transferText(h.pay)}</li>`);
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
    const myMoney = state.money ? state.money[mySeat()] : 0;
    if (state.instant) {
      $('result-title').textContent = `${playerName(state.instant.player)} tới trắng (${state.instant.reason})!`;
    } else {
      $('result-title').textContent = state.players[mySeat()].place === 0 ? 'Bạn về nhất!' : 'Kết thúc ván';
    }
    const won = state.players[mySeat()].place === 0 || myMoney > 0;
    $('result-icon').className = 'result-icon' + (won ? '' : ' lose');
    $('result-icon').innerHTML = I.svg(won ? 'trophy' : 'layers');
    const order = state.players
      .map((p, i) => ({ i, p, place: p.place < 0 ? 99 : p.place, money: state.money ? state.money[i] : 0 }))
      .sort((a, b) => a.place - b.place || b.money - a.money);
    $('result-list').innerHTML = order
      .map(({ i, p, place, money }) => {
        const label = p.cong ? 'Cóng' : place < 4 && !state.instant ? PLACES[place] : '';
        const cls = money > 0 ? 'delta-plus' : money < 0 ? 'delta-minus' : '';
        return `<li><span class="place">${label}</span><span class="who">${face(p)}${escapeHtml(playerName(i))}</span><span class="${cls}">${signed(money)} xu</span></li>`;
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
    el.lastElementChild.textContent = `${left}s`;
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

  $('btn-create').addEventListener('click', openCreate);
  $('tables').addEventListener('click', (e) => {
    const join = e.target.closest('[data-join]');
    if (join) return send({ t: 'join', code: join.dataset.join, token: A.token() });
    if (e.target.closest('[data-create]')) openCreate();
  });
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
    const btn = e.target.closest('[data-kick]');
    if (btn) send({ t: 'kick', id: btn.dataset.kick });
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

  $('logo').innerHTML = I.logo(28);
  // Biển hiệu CASINO hoạt hình (bộ hình bản đồ) làm biểu tượng sảnh.
  $('home-art').innerHTML = `<img class="art hall-art" src="/img/toon/city/sign_casino.png" alt="" draggable="false">`;

  // Chưa đăng nhập thì về hub, xong quay lại đúng link phòng này.
  A.me().then(
    (user) => {
      if (!user) return A.requireLogin();
      $('user-name').textContent = user.name;
      $('user-avatar').innerHTML = C.head({ gender: user.gender, outfit: user.outfit });
      showBalance(user.balance);
      render();
      connect();
    },
    (e) => A.toast(e.message),
  );
})();
