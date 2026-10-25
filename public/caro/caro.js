// Client Cờ Caro: luật, lượt, thắng thua do server Go quyết định; client chỉ hiển thị và gửi nước đi.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const xu = A.xu;
  const escapeHtml = A.escapeHtml;
  const signed = (n) => (n > 0 ? '+' : n < 0 ? '−' : '') + xu(Math.abs(n));

  const IMG = '/img/toon/caro/';
  const CASINO = '/img/toon/casino/';
  const N = 15; // cỡ bàn mặc định, server gửi kèm trong state
  const ROOM_KEY = 'caro.room'; // bàn đang ngồi, tải lại trang thì vào lại đúng ghế
  const DEFAULT_BETS = [0, 10, 20, 50, 100, 200, 500, 1000];
  const DEFAULT_BET = 10;
  const RULES = { free: 'Tự do', block2: 'Chặn 2 đầu' };
  const RULE_NOTE = { free: '5 quân liền là thắng', block2: '5 quân bị chặn cả 2 đầu không tính' };
  const BOT_NAMES = { easy: 'Dễ', normal: 'Vừa', hard: 'Khó' };
  const REASONS = { five: '5 quân liền hàng', timeout: 'hết giờ', resign: 'đầu hàng', left: 'rời bàn', draw: 'hết chỗ đánh' };
  const HALL_REFRESH = 3000;
  // Máy chưa có nhân vật riêng: dùng nhân vật vest.
  const BOT_LOOK = { gender: 'male', outfit: 'suit' };
  const betText = (b) => (b ? `${xu(b)} xu` : 'Chơi vui');

  let ws = null;
  let state = null;
  let pending = null; // tin nhắn chờ gửi khi kết nối xong
  let stopped = false; // bị tab khác thay thế thì không tự kết nối lại
  let deadline = 0; // thời điểm hết lượt (giờ máy mình)
  let cursor = { x: 7, y: 7 }; // ô đang chọn bằng bàn phím / chạm
  let armed = null; // điện thoại: ô đã chạm một lần, chạm lại để đánh
  let keyboardUsed = false;
  let hideResult = false; // người chơi bấm "Xem bàn cờ" sau khi hết ván
  let user = null;
  let dropped = -1; // số nước đã chạy hiệu ứng thả quân (không chạy lại khi chỉ vẽ lại bàn)

  // ---------- Lưu trữ (có thể bị chặn ở chế độ ẩn danh) ----------

  const store = (k, v) => { try { v == null ? sessionStorage.removeItem(k) : sessionStorage.setItem(k, v); } catch (e) { /* bỏ qua */ } };
  const load = (k) => { try { return sessionStorage.getItem(k); } catch (e) { return null; } };

  // ---------- Kết nối ----------

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/ws/caro`);
    ws.onopen = () => {
      const code = load(ROOM_KEY) || new URLSearchParams(location.search).get('room');
      if (code) send({ t: 'join', code: code.toUpperCase(), token: A.token() });
      else if (pending) send(pending);
      pending = null;
    };
    ws.onmessage = (e) => onMessage(JSON.parse(e.data));
    ws.onclose = () => {
      if (stopped) return;
      rejoining = !!state; // đang ở bàn mà rớt mạng: lần nối lại sẽ xin vào lại bàn cũ
      if (state) status('Mất kết nối, đang kết nối lại…');
      setTimeout(connect, 1000);
    };
  }

  // Nối lại sau khi máy chủ khởi động lại (cập nhật) mà bàn cũ không còn: đưa về sảnh và giải thích,
  // thay vì để bàn cũ đứng hình. Ván dở chưa chốt tiền nên không ai mất xu.
  let rejoining = false;
  function roomLost() {
    rejoining = false;
    store(ROOM_KEY, null);
    state = null;
    tchat.reset();
    history.replaceState(null, '', location.pathname);
    render();
    A.alert('Máy chủ vừa được cập nhật nên bàn cũ đã đóng. Ván đang dở được huỷ, không ai bị trừ xu. Mời bạn mở bàn mới.', { title: 'Bàn đã đóng' });
  }

  function send(msg) {
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg));
    else pending = msg;
  }

  function onMessage(m) {
    switch (m.t) {
      case 'session':
        store(ROOM_KEY, m.code);
        history.replaceState(null, '', '?room=' + m.code);
        break;
      case 'auth':
        stopped = true;
        A.requireLogin();
        break;
      case 'state':
        rejoining = false;
        onState(m);
        tchat.sync(m.chat);
        break;
      case 'chat':
        tchat.add(m);
        break;
      case 'error':
        if (rejoining) { roomLost(); break; }
        A.toast(m.msg);
        // Mã bàn sai / bàn đã đóng: quên mã để lần sau không vào lại.
        if (!state) { store(ROOM_KEY, null); history.replaceState(null, '', location.pathname); }
        break;
      case 'left':
        store(ROOM_KEY, null);
        state = null;
        tchat.reset();
        history.replaceState(null, '', location.pathname);
        render();
        break;
      case 'replaced':
        stopped = true;
        state = null;
        render();
        A.toast('Bạn đã mở bàn này ở tab khác');
        break;
    }
  }

  function onState(s) {
    const prev = state;
    state = s;
    if (s.code) store(ROOM_KEY, s.code);
    deadline = s.phase === 'playing' && s.turnMs > 0 ? Date.now() + s.turnMs : 0;
    if (s.phase !== 'over') hideResult = false;
    if (!prev || prev.phase !== 'playing' || s.phase !== 'playing') armed = null;
    if (armed && cellAt(s, armed.x, armed.y) !== '.') armed = null;
    const mine = me();
    if (mine) showBalance(mine.coins);
    // Hết ván: server vừa cộng xu / XP, tải lại level cho thanh trên cùng.
    if (s.phase === 'over' && prev && prev.phase === 'playing') {
      A.me().then((u) => { if (u) { user = u; showLevel(u); showBalance(u.balance); } }, () => {});
    }
    render();
  }

  // ---------- Tiện ích ----------

  const me = () => (state ? state.players.find((p) => p.id === state.you) : null);
  const isHost = () => !!(me() && me().host);
  const size = () => (state && state.size) || N;
  const cellAt = (s, x, y) => (s.board || '')[y * ((s && s.size) || N) + x] || '.';
  const myTurn = () => !!(state && state.phase === 'playing' && me() && state.turn === me().mark);
  const lookOf = (p) => (p.bot ? BOT_LOOK : p); // nguyên object người chơi: có tóc / cánh / mặt

  function showBalance(coins) {
    $('balance').textContent = xu(coins);
  }

  // Huy hiệu level + thanh XP, giống các trang khác.
  function showLevel(u) {
    const el = $('level-badge');
    const span = Math.max(1, u.nextXP - u.levelXP);
    const pct = Math.min(100, Math.round(((u.xp - u.levelXP) / span) * 100));
    el.innerHTML = `<span class="lv">Lv ${u.level}</span><span class="xp-bar"><span style="width:${pct}%"></span></span>`;
    el.title = `${u.xp - u.levelXP} / ${span} XP để lên level ${u.level + 1}`;
    el.classList.remove('hidden');
  }

  function status(msg) {
    $('status').textContent = msg || '';
  }

  const piece = (mark, cls = '') => `<img class="piece ${cls}" src="${IMG}piece_${mark}.png" alt="${mark.toUpperCase()}" draggable="false">`;

  // ---------- Hiển thị ----------

  // Chat trong bàn (người xem cũng chat được): bong bóng hiện trên nhân vật ở thẻ người chơi.
  const tchat = window.TableChat.mount({
    send: (text) => send({ t: 'chat', text }),
    isMe: (line) => !!state && !!state.you && line.id === state.you,
    anchor: (line) => document.querySelector(`.pcard[data-pid="${CSS.escape(line.id || '')}"] .pbody`),
  });

  function render() {
    const inRoom = !!state;
    tchat.el.classList.toggle('hidden', !inRoom);
    $('home').classList.toggle('hidden', inRoom);
    $('room').classList.toggle('hidden', !inRoom);
    setHallPolling(!inRoom);
    if (!inRoom) {
      $('result').classList.add('hidden');
      renderHall();
      return;
    }
    renderRoom();
  }

  // ---------- Sảnh ----------

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
      rooms = await A.api('/api/caro/rooms');
    } catch (e) {
      if (rooms === null) rooms = [];
    }
    if (!state) renderHall();
  }

  // Một bàn trong sảnh: bàn gỗ có bàn cờ nhỏ, 2 ghế hai bên, người ngồi quay vào bàn.
  function seatHtml(p, side, room, open) {
    const chair = `<img class="art art-chair" src="${CASINO}chair_${side}.png" alt="" draggable="false">`;
    if (p) {
      const body = C.sprite(lookOf(p), { dir: side === 'left' ? 'right' : 'left', size: 92, cls: 'seat-char' });
      const lvl = p.bot ? `<span class="lv-chip bot-chip">Máy</span>` : p.level ? `<span class="lv-chip">Lv ${p.level}</span>` : '';
      return `<div class="hseat ${side} filled" title="${escapeHtml(p.name)}">${chair}<span class="seat-body">${body}</span>
          <span class="seat-label"><span class="seat-name">${escapeHtml(p.name)}</span>${lvl}</span></div>`;
    }
    if (!open) return `<div class="hseat ${side} empty">${chair}</div>`;
    return `<button class="hseat ${side} empty" data-join="${escapeHtml(room.code)}" title="Ngồi vào bàn ${escapeHtml(room.code)}">
        ${chair}<span class="seat-join">${I.svg('plus')}Ngồi</span></button>`;
  }

  function tableHtml(room) {
    const players = room.players || [];
    const playing = room.phase === 'playing';
    const full = players.length >= (room.max || 2);
    const open = !playing && !full;
    const st = playing ? 'Đang đánh' : full ? 'Đủ người' : `Đang chờ · ${players.length}/2`;
    const watch = !open
      ? `<button class="watch" data-join="${escapeHtml(room.code)}">${I.svg('eye')}Xem${room.spectators ? ` · ${room.spectators}` : ''}</button>`
      : '';
    return `<div class="htable${playing ? ' playing' : ''}">
        <div class="hscene">
          ${seatHtml(players[0], 'left', room, open)}
          <img class="art art-table" src="${IMG}table_caro.png" alt="" draggable="false">
          ${seatHtml(players[1], 'right', room, open)}
        </div>
        <div class="hinfo">
          <span class="tcode">${escapeHtml(room.code)}</span>
          <span class="tbet">${I.svg('coins')}${betText(room.bet ?? DEFAULT_BET)}</span>
          <span class="trule">${RULES[room.rule] || RULES.free}</span>
          <span class="tstatus${playing ? ' live' : ''}">${st}</span>
          ${watch}
        </div>
      </div>`;
  }

  function renderHall() {
    const el = $('tables');
    if (rooms === null) {
      el.innerHTML = '<p class="hall-empty">Đang tải các bàn…</p>';
      return;
    }
    // Bàn còn ghế trống lên trước, rồi bàn đủ người, rồi bàn đang đánh.
    const order = (r) => (r.phase === 'playing' ? 2 : (r.players || []).length >= (r.max || 2) ? 1 : 0);
    const list = rooms.slice().sort((a, b) => order(a) - order(b));
    const invite = `<div class="htable invite">
        <div class="hscene">
          <div class="hseat left empty"><img class="art art-chair" src="${CASINO}chair_left.png" alt="" draggable="false"></div>
          <img class="art art-table" src="${IMG}table_caro.png" alt="" draggable="false">
          <div class="hseat right empty"><img class="art art-chair" src="${CASINO}chair_right.png" alt="" draggable="false"></div>
        </div>
        <div class="hinfo"><button class="primary" data-create>${I.svg('plus')}${list.length ? 'Mở thêm bàn' : 'Tạo bàn đầu tiên'}</button>
          <span class="tnote">${list.length ? 'Mời bạn hoặc chơi với máy' : 'Chưa có bàn nào, mở bàn và mời bạn bè'}</span></div>
      </div>`;
    el.innerHTML = list.map(tableHtml).join('') + invite;
  }

  function openCreate() {
    const bets = (state && state.betOptions) || DEFAULT_BETS;
    $('bet-grid').innerHTML = bets
      .map((b) => `<label class="bet-opt"><input type="radio" name="bet" value="${b}" ${b === DEFAULT_BET ? 'checked' : ''}>
          <span>${I.svg('coins')}${b ? xu(b) : 'Chơi vui'}<small>${b ? 'xu/ván' : '0 xu'}</small></span></label>`)
      .join('');
    $('rule-grid').innerHTML = Object.entries(RULES)
      .map(([k, name], i) => `<label class="bet-opt rule-opt"><input type="radio" name="rule" value="${k}" ${i === 0 ? 'checked' : ''}>
          <span>${name}<small>${RULE_NOTE[k]}</small></span></label>`)
      .join('');
    const dlg = $('dlg-create');
    const form = dlg.querySelector('form');
    form.onsubmit = (e) => {
      if (e.submitter && e.submitter.value === 'cancel') return;
      const fd = new FormData(form);
      const bet = fd.get('bet') === null ? DEFAULT_BET : Number(fd.get('bet'));
      send({ t: 'create', token: A.token(), bet, rule: fd.get('rule') || 'free' });
    };
    dlg.showModal();
  }

  function joinCode(code) {
    code = (code || '').trim().toUpperCase();
    if (!code) return A.toast('Nhập mã bàn');
    send({ t: 'join', code, token: A.token() });
  }

  // ---------- Trong bàn ----------

  function renderRoom() {
    const s = state;
    const host = isHost();
    const lobby = s.phase === 'lobby';
    $('room-code').textContent = s.code;
    $('room-bet').innerHTML = `${I.svg('coins')}${betText(s.phase === 'lobby' ? s.bet : s.gameBet ?? s.bet)}`;
    $('room-rule').textContent = RULES[s.rule] || RULES.free;
    $('room-rule').title = RULE_NOTE[s.rule] || '';
    $('room-watch').innerHTML = `${I.svg('eye')}${s.spectators || 0}`;
    $('room-watch').classList.toggle('hidden', !s.spectators);
    $('room-watch').title = 'Người đang xem';

    document.querySelectorAll('#room .host-only').forEach((el) => el.classList.toggle('hidden', !host));
    $('guest-wait').classList.toggle('hidden', host || !lobby);
    $('guest-wait').textContent = me() ? 'Chờ chủ bàn bắt đầu ván…' : 'Bạn đang xem bàn này';
    $('lobby-controls').classList.toggle('hidden', !lobby);
    $('play-controls').classList.toggle('hidden', s.phase !== 'playing' || !me());

    if (lobby) {
      const sel = $('sel-bet');
      const opts = s.betOptions || DEFAULT_BETS;
      if (sel.options.length !== opts.length) sel.innerHTML = opts.map((b) => `<option value="${b}">${b ? xu(b) + ' xu' : '0 (chơi vui)'}</option>`).join('');
      sel.value = s.bet;
      $('sel-rule').value = s.rule || 'free';
      $('btn-add-bot').disabled = s.players.length >= 2;
      $('sel-bot').disabled = s.players.length >= 2;
      $('btn-start').disabled = s.players.length < 2;
    }

    renderSeats();
    renderBoard();
    renderStatus();
    renderResult();
  }

  // Hai bên bàn cờ: X bên trái, O bên phải (trước khi chia quân thì theo thứ tự ngồi).
  function renderSeats() {
    const s = state;
    const byMark = (m) => s.players.find((p) => p.mark === m);
    const a = byMark('x') || s.players[0] || null;
    const b = byMark('o') || s.players.find((p) => p !== a) || null;
    $('seat-a').innerHTML = seatPanel(a, 'x');
    $('seat-b').innerHTML = seatPanel(b, 'o');
    tickTimers();
  }

  function seatPanel(p, fallbackMark) {
    const s = state;
    if (!p) {
      const host = isHost() && s.phase === 'lobby';
      return `<div class="pcard empty">
          <div class="pbody">${C.sprite({ gender: 'male', outfit: 'casual' }, { dir: 'down', size: 130, cls: 'ghost-char' })}</div>
          <div class="pname muted">Ghế trống</div>
          ${host ? `<button data-act="invite">${I.svg('link')}Mời bạn</button><button data-act="bot">${I.svg('bot')}Thêm máy</button>` : ''}
        </div>`;
    }
    const mark = p.mark || fallbackMark;
    const turn = s.phase === 'playing' && s.turn === mark;
    const mine = p.id === s.you;
    const lvl = p.bot ? `<span class="lv-chip bot-chip">Máy ${BOT_NAMES[p.botLevel] || ''}</span>` : p.level ? `<span class="lv-chip">Lv ${p.level}</span>` : '';
    const tags = [
      mine ? '<span class="tag me">Bạn</span>' : '',
      p.host ? `<span class="tag host">${I.svg('crown')}Chủ bàn</span>` : '',
      !p.bot && p.online === false ? `<span class="tag off">${I.svg('wifiOff')}Mất kết nối</span>` : '',
      s.phase === 'over' && p.ready ? `<span class="tag ready">${I.svg('check')}Sẵn sàng</span>` : '',
    ].join('');
    const kick = isHost() && !mine && s.phase === 'lobby'
      ? `<button class="link kick" data-kick="${escapeHtml(p.id)}" title="Mời ra">${I.svg('x')}Mời ra</button>` : '';
    const coins = p.bot ? '' : `<div class="pcoins">${I.svg('coins')}${xu(p.coins)}</div>`;
    return `<div class="pcard${turn ? ' turn' : ''}${mine ? ' mine' : ''}" data-mark="${mark}" data-pid="${escapeHtml(p.id)}">
        <div class="pmark">${piece(mark)}<span class="timer" data-timer="${mark}"><span class="timer-num"></span></span></div>
        <div class="pbody">${C.sprite(lookOf(p), { dir: 'down', size: 130, cls: 'p-char' })}</div>
        <div class="pname">${escapeHtml(p.name)}${lvl}</div>
        <div class="ptags">${tags}</div>
        ${coins}${kick}
      </div>`;
  }

  // Bàn cờ: lưới 15×15 nằm đúng trên ảnh board.png (lề 30px, ô 44px trên ảnh 720px).
  function renderBoard() {
    const s = state;
    const n = size();
    const grid = $('grid');
    const mark = me() && me().mark;
    const board = $('board');
    board.classList.toggle('can-move', myTurn());
    board.classList.toggle('mark-x', mark === 'x');
    board.classList.toggle('mark-o', mark === 'o');
    board.classList.toggle('kbd', keyboardUsed);
    const win = new Set((s.winLine || []).map(([x, y]) => `${x},${y}`));
    const animate = s.moves !== dropped;
    dropped = s.moves;
    let html = '';
    for (let y = 0; y < n; y++) {
      for (let x = 0; x < n; x++) {
        const v = cellAt(s, x, y);
        const cls = ['cell'];
        if (v === '.') cls.push('empty');
        if (s.last && s.last.x === x && s.last.y === y) cls.push('last');
        if (win.has(`${x},${y}`)) cls.push('win');
        if (cursor.x === x && cursor.y === y) cls.push('cursor');
        if (armed && armed.x === x && armed.y === y) cls.push('armed');
        html += `<div class="${cls.join(' ')}" data-x="${x}" data-y="${y}">${v === 'x' || v === 'o' ? piece(v, animate && s.last && s.last.x === x && s.last.y === y ? 'drop' : '') : ''}</div>`;
      }
    }
    grid.innerHTML = html;
    grid.style.setProperty('--n', n);
    renderStrike();
  }

  // Nét gạch qua 5 quân thắng: nối tâm ô đầu và ô cuối của đường thắng.
  function renderStrike() {
    const el = $('strike');
    const line = state.winLine;
    if (!line || line.length < 2) return el.classList.add('hidden');
    const n = size();
    const pts = line.slice().sort((a, b) => a[0] - b[0] || a[1] - b[1]);
    const [x1, y1] = pts[0];
    const [x2, y2] = pts[pts.length - 1];
    // Toạ độ theo % của khung lưới.
    const cx = (v) => ((v + 0.5) / n) * 100;
    const dx = cx(x2) - cx(x1);
    const dy = cx(y2) - cx(y1);
    const len = Math.hypot(dx, dy) + (100 / n) * 0.7;
    const ang = (Math.atan2(dy, dx) * 180) / Math.PI;
    el.style.left = `${(cx(x1) + cx(x2)) / 2}%`;
    el.style.top = `${(cx(y1) + cx(y2)) / 2}%`;
    el.style.width = `${len}%`;
    el.style.transform = `translate(-50%, -50%) rotate(${ang}deg)`;
    el.className = `strike mark-${(state.result && state.result.winner) || 'x'}`;
  }

  function renderStatus() {
    const s = state;
    if (s.phase === 'lobby') {
      return status(s.players.length < 2 ? 'Chờ thêm một người chơi — mời bạn hoặc thêm máy.' : isHost() ? 'Đủ 2 người, bấm Bắt đầu.' : 'Đủ 2 người, chờ chủ bàn bắt đầu.');
    }
    if (s.phase === 'over') return status(resultLine());
    const turnP = s.players.find((p) => p.mark === s.turn);
    if (myTurn()) status(`Tới lượt bạn (${s.turn.toUpperCase()}) — ${'ontouchstart' in window ? 'chạm 2 lần vào ô để đánh' : 'bấm vào ô để đánh'}`);
    else status(turnP ? `${turnP.name} (${s.turn.toUpperCase()}) đang nghĩ…` : '');
  }

  function resultLine() {
    const r = state.result;
    if (!r) return 'Hết ván';
    if (!r.winner) return 'Hoà — ' + (REASONS[r.reason] || '');
    const w = state.players.find((p) => p.mark === r.winner);
    const who = w ? (w.id === state.you ? 'Bạn' : w.name) : r.winner.toUpperCase();
    return `${who} thắng — ${REASONS[r.reason] || ''}`;
  }

  // Banner kết quả: thắng / thua / hoà, xu và XP nhận được.
  function renderResult() {
    const s = state;
    const el = $('result');
    const show = s.phase === 'over' && !hideResult;
    el.classList.toggle('hidden', !show);
    if (!show) return;
    const r = s.result || {};
    const mine = me();
    let title = 'Hết ván';
    let tone = 'draw';
    if (!r.winner) { title = 'Hoà!'; tone = 'draw'; }
    else if (mine && mine.mark === r.winner) { title = 'Bạn thắng!'; tone = 'win'; }
    else if (mine) { title = 'Bạn thua'; tone = 'lose'; }
    else { const w = s.players.find((p) => p.mark === r.winner); title = `${w ? w.name : r.winner.toUpperCase()} thắng`; tone = 'win'; }
    $('result').dataset.tone = tone;
    $('result-title').innerHTML = `${r.winner ? piece(r.winner, 'res-piece') : ''}${escapeHtml(title)}`;
    $('result-sub').textContent = resultLine();
    const money = mine && r.money ? r.money[mine.id] || 0 : 0;
    const xp = mine && r.xp ? r.xp[mine.id] || 0 : 0;
    // Bàn chơi vui (cược 0) không thắng thua xu: chỉ hiện XP.
    const coins = s.gameBet ? `<span class="gain ${money > 0 ? 'plus' : money < 0 ? 'minus' : ''}">${I.svg('coins')}${signed(money)} xu</span>` : '';
    $('result-gain').innerHTML = mine ? `${coins}${xp ? `<span class="gain xp">+${xp} XP</span>` : ''}` : '';
    const ready = mine && mine.ready;
    $('btn-rematch').disabled = !mine || !!ready;
    $('btn-rematch').innerHTML = ready ? `${I.svg('timer')}Chờ đối thủ…` : `${I.svg('play')}Ván mới <kbd>N</kbd>`;
  }

  // Đồng hồ 30 s dạng vòng tròn quanh quân của người tới lượt.
  function tickTimers() {
    if (!state) return;
    const left = deadline ? Math.max(0, deadline - Date.now()) : 0;
    const total = 30000;
    document.querySelectorAll('[data-timer]').forEach((el) => {
      const on = state.phase === 'playing' && state.turn === el.dataset.timer && deadline;
      el.classList.toggle('on', !!on);
      el.classList.toggle('low', !!on && left <= 10000);
      el.style.setProperty('--p', on ? Math.min(1, left / total) : 0);
      el.querySelector('.timer-num').textContent = on ? Math.ceil(left / 1000) : '';
    });
  }
  setInterval(tickTimers, 200);

  // ---------- Đánh cờ ----------

  function tryMove(x, y) {
    if (!myTurn()) return;
    if (cellAt(state, x, y) !== '.') return;
    armed = null;
    send({ t: 'move', x, y });
  }

  // Chuột: bấm là đánh. Cảm ứng: chạm lần 1 hiện quân mờ, chạm lại đúng ô đó mới đánh (tránh bấm nhầm).
  let lastPointer = 'mouse';
  $('grid').addEventListener('pointerdown', (e) => { lastPointer = e.pointerType; });
  $('grid').addEventListener('click', (e) => {
    const cell = e.target.closest('.cell');
    if (!cell || !state) return;
    const x = +cell.dataset.x;
    const y = +cell.dataset.y;
    keyboardUsed = false;
    cursor = { x, y };
    if (!myTurn() || cellAt(state, x, y) !== '.') return renderBoard();
    if (lastPointer === 'touch') {
      if (armed && armed.x === x && armed.y === y) return tryMove(x, y);
      armed = { x, y };
      return renderBoard();
    }
    tryMove(x, y);
  });

  // ---------- Nút ----------

  async function copyInvite() {
    const link = `${location.origin}/caro/?room=${state.code}`;
    try {
      await navigator.clipboard.writeText(link);
      A.toast('Đã sao chép link mời — gửi cho bạn bè để vào bàn');
    } catch (e) {
      A.ask({ title: 'Mời bạn', message: 'Sao chép link này gửi cho bạn bè:', input: link, ok: 'Xong', cancel: '' });
    }
  }

  async function resign() {
    if (!state || state.phase !== 'playing' || !me()) return;
    const ok = await A.confirm('Đầu hàng ván này? Bạn sẽ thua và mất tiền cược.', { title: 'Đầu hàng', ok: 'Đầu hàng', cancel: 'Đánh tiếp', danger: true });
    if (ok) send({ t: 'resign' });
  }

  async function leave() {
    if (!state) return;
    if (state.phase === 'playing' && me()) {
      const ok = await A.confirm('Rời bàn giữa ván sẽ bị xử thua. Vẫn rời?', { title: 'Rời bàn', ok: 'Rời bàn', cancel: 'Ở lại', danger: true });
      if (!ok) return;
    }
    send({ t: 'leave' });
  }

  function rematch() {
    if (state && state.phase === 'over' && me() && !me().ready) send({ t: 'rematch' });
  }

  $('btn-create').addEventListener('click', openCreate);
  $('btn-join').addEventListener('click', () => joinCode($('code').value));
  $('code').addEventListener('keydown', (e) => { if (e.key === 'Enter') joinCode($('code').value); });
  $('tables').addEventListener('click', (e) => {
    const join = e.target.closest('[data-join]');
    if (join) return joinCode(join.dataset.join);
    if (e.target.closest('[data-create]')) openCreate();
  });
  $('btn-invite').addEventListener('click', copyInvite);
  $('btn-leave').addEventListener('click', leave);
  $('btn-resign').addEventListener('click', resign);
  $('btn-rematch').addEventListener('click', rematch);
  $('btn-board').addEventListener('click', () => { hideResult = true; renderResult(); });
  $('btn-hall').addEventListener('click', () => send({ t: 'leave' }));
  $('btn-add-bot').addEventListener('click', () => send({ t: 'addBot', level: $('sel-bot').value }));
  $('btn-start').addEventListener('click', () => send({ t: 'start' }));
  $('sel-bet').addEventListener('change', (e) => send({ t: 'setBet', bet: Number(e.target.value) }));
  $('sel-rule').addEventListener('change', (e) => send({ t: 'setRule', rule: e.target.value }));
  $('room').addEventListener('click', (e) => {
    const kick = e.target.closest('[data-kick]');
    if (kick) return send({ t: 'kick', id: kick.dataset.kick });
    const act = e.target.closest('[data-act]');
    if (!act) return;
    if (act.dataset.act === 'invite') copyInvite();
    if (act.dataset.act === 'bot') send({ t: 'addBot', level: $('sel-bot').value });
  });

  // ---------- Bàn phím ----------

  const MOVES = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] };
  document.addEventListener('keydown', (e) => {
    if (!state || e.ctrlKey || e.metaKey || e.altKey) return;
    const t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    if (document.querySelector('dialog[open]')) return;
    const n = size();
    if (MOVES[e.key] && state.phase === 'playing') {
      e.preventDefault();
      const [dx, dy] = MOVES[e.key];
      cursor = { x: Math.max(0, Math.min(n - 1, cursor.x + dx)), y: Math.max(0, Math.min(n - 1, cursor.y + dy)) };
      keyboardUsed = true;
      armed = null;
      renderBoard();
    } else if ((e.key === 'Enter' || e.key === ' ') && state.phase === 'playing' && (!t || t.tagName !== 'BUTTON')) {
      e.preventDefault();
      keyboardUsed = true;
      tryMove(cursor.x, cursor.y);
    } else if (e.key === 'r' || e.key === 'R') {
      resign();
    } else if ((e.key === 'n' || e.key === 'N') && state.phase === 'over') {
      rematch();
    } else if (e.key === 'Escape' && state.phase === 'over' && !hideResult) {
      hideResult = true;
      renderResult();
    }
  });

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(28);
  $('home-art').innerHTML = `<span class="hall-pieces">${piece('x')}${piece('o')}</span>`;

  // Chưa đăng nhập thì về hub, xong quay lại đúng link bàn này.
  A.me().then(
    (u) => {
      if (!u) return A.requireLogin();
      user = u;
      $('user-name').textContent = u.name;
      $('user-avatar').innerHTML = C.head(u);
      showBalance(u.balance);
      showLevel(u);
      render();
      connect();
    },
    (e) => A.toast(e.message),
  );
})();
