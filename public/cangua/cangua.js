// Client Cờ Cá Ngựa: xúc xắc, nước đi, đá ngựa, thắng thua do server Go quyết định (games/cangua);
// client chỉ hiển thị, chạy hiệu ứng ngựa nhảy từng ô và gửi lệnh đổ / đi.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const xu = A.xu;
  const escapeHtml = A.escapeHtml;
  const signed = (n) => (n > 0 ? '+' : n < 0 ? '−' : '') + xu(Math.abs(n));

  const IMG = '/img/toon/cangua/';
  const ROOM_KEY = 'cangua.room'; // bàn đang ngồi, tải lại trang thì vào lại đúng ghế
  const DEFAULT_BETS = [0, 10, 20, 50, 100, 200, 500, 1000];
  const DEFAULT_BET = 10;
  const BOT_NAMES = { easy: 'Dễ', normal: 'Vừa', hard: 'Khó' };
  const HALL_REFRESH = 3000;
  const BOT_LOOK = { gender: 'male', outfit: 'suit' }; // máy chưa có nhân vật riêng
  const betText = (b) => (b ? `${xu(b)} xu` : 'Chơi vui');

  // ---------- Hình học bàn cờ (khớp ảnh board.png 720×720, cùng công thức với script vẽ) ----------
  // Lưới 15×15, lề 30px, ô 44px. Đường chung 56 ô đi chiều kim đồng hồ; ô xuất phát của màu i là 14·i,
  // cửa vào đích là ô 14·i − 1. Mỗi màu có chuồng 6×6 ở một góc (4 ổ ngựa) và cột đích 6 bậc dẫn vào giữa.
  const COLORS = ['red', 'blue', 'green', 'yellow'];
  const COLOR_NAMES = { red: 'Đỏ', blue: 'Xanh dương', green: 'Xanh lá', yellow: 'Vàng' };
  const SIDE = 14;
  const TRACK_N = 56;
  const PX = 720;
  const MARGIN = 30;
  const CELL = 44;
  const rot = ([x, y]) => [14 - y, x]; // quay 90° theo chiều kim đồng hồ quanh tâm (7,7)
  const rotn = (c, n) => { for (let i = 0; i < n; i++) c = rot(c); return c; };
  const RED_Q = [[0, 6], [1, 6], [2, 6], [3, 6], [4, 6], [5, 6], [6, 6], [6, 5], [6, 4], [6, 3], [6, 2], [6, 1], [6, 0], [7, 0]];
  const TRACK = [0, 1, 2, 3].flatMap((q) => RED_Q.map((c) => rotn(c, q)));
  const HOME = Object.fromEntries(COLORS.map((col, i) => [col, [1, 2, 3, 4, 5, 6].map((b) => rotn([b, 7], i))]));
  const CORNER = [[0, 0], [9, 0], [9, 9], [0, 9]];
  const SLOTS = Object.fromEntries(COLORS.map((col, i) => {
    const [ox, oy] = CORNER[i];
    return [col, [[ox + 1.5, oy + 1.5], [ox + 3.5, oy + 1.5], [ox + 1.5, oy + 3.5], [ox + 3.5, oy + 3.5]]];
  }));
  const ci = (color) => Math.max(0, COLORS.indexOf(color));
  const relOf = (color, pos) => (pos - SIDE * ci(color) + TRACK_N) % TRACK_N;
  const absOf = (color, r) => (SIDE * ci(color) + r) % TRACK_N;

  // Toạ độ (% của bàn) của tâm một ô / ổ ngựa.
  const pct = ([cx, cy]) => [((MARGIN + CELL * cx + CELL / 2) / PX) * 100, ((MARGIN + CELL * cy + CELL / 2) / PX) * 100];
  function cellOf(color, horse, pos) {
    if (pos < 0) return SLOTS[color][horse];
    if (pos >= 100) return HOME[color][Math.min(6, Math.max(1, pos - 100)) - 1];
    return TRACK[pos % TRACK_N];
  }

  // Các ô ngựa đi qua (không gồm ô đứng) để chạy hiệu ứng nhảy từng ô.
  function pathOf(color, from, to) {
    if (from < 0) return [to];
    if (from >= 100) {
      const out = [];
      for (let b = from - 99; b <= to - 100; b++) out.push(100 + b);
      return out.length ? out : [to];
    }
    const r = relOf(color, from);
    const out = [];
    if (to >= 100) {
      for (let k = r + 1; k <= TRACK_N - 1; k++) out.push(absOf(color, k));
      for (let b = 1; b <= to - 100; b++) out.push(100 + b);
    } else {
      const steps = (relOf(color, to) - r + TRACK_N) % TRACK_N;
      for (let k = 1; k <= steps; k++) out.push(absOf(color, r + k));
    }
    return out.length ? out : [to];
  }

  // Ô đích dự kiến của một ngựa với số xúc xắc d (chỉ để xem trước; nước hợp lệ do server gửi trong movable).
  function previewTarget(color, pos, d) {
    if (pos < 0) return SIDE * ci(color);
    if (pos >= 100) return pos + 1;
    const r = relOf(color, pos);
    if (r === TRACK_N - 1) return 100 + d;
    return absOf(color, r + d);
  }

  let ws = null;
  let state = null;
  let pending = null;
  let stopped = false;
  let deadline = 0;
  let hideResult = false;
  let user = null;
  let pick = 0; // ngựa đang chọn bằng bàn phím
  let shown = null; // vị trí ngựa đang vẽ trên bàn (trễ hơn state khi đang chạy hiệu ứng)
  let animating = false; // đang chạy hiệu ứng trong hàng đợi: chưa cho đổ / đi

  const store = (k, v) => { try { v == null ? sessionStorage.removeItem(k) : sessionStorage.setItem(k, v); } catch (e) { /* bỏ qua */ } };
  const load = (k) => { try { return sessionStorage.getItem(k); } catch (e) { return null; } };

  // ---------- Kết nối ----------

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/ws/cangua`);
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

  // Nối lại sau khi máy chủ khởi động lại (cập nhật) mà bàn cũ không còn: đưa về sảnh và giải thích.
  let rejoining = false;
  function roomLost() {
    rejoining = false;
    store(ROOM_KEY, null);
    state = null;
    shown = null;
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
        m._at = Date.now();
        inbox.push(m);
        pump();
        tchat.sync(m.chat);
        break;
      case 'chat':
        tchat.add(m);
        break;
      case 'error':
        if (rejoining) { roomLost(); break; }
        A.toast(m.msg);
        if (!state) { store(ROOM_KEY, null); history.replaceState(null, '', location.pathname); }
        break;
      case 'left':
        inbox.length = 0;
        store(ROOM_KEY, null);
        state = null;
        shown = null;
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

  // Hàng đợi hoạt ảnh: mỗi state từ server được áp dụng lần lượt, sau khi chạy xong hiệu ứng của nó
  // (xúc xắc lăn → giữ số → ngựa nhảy từng ô → báo lượt). State tới trong lúc đang chạy thì chờ.
  const inbox = [];
  let pumping = false;
  async function pump() {
    if (pumping) return;
    pumping = true;
    try {
      while (inbox.length) {
        const s = inbox.shift();
        const prev = state;
        // Chỉ chạy hiệu ứng khi đang ở cùng một ván (vào bàn / tải lại trang thì vẽ thẳng).
        if (prev && prev.code === s.code && prev.phase === 'playing' && (s.phase === 'playing' || s.phase === 'over')) {
          animating = true;
          try { await playEvents(prev, s); } catch (e) { /* hiệu ứng lỗi thì bỏ qua, vẫn áp dụng state */ }
          animating = false;
        }
        onState(s);
        if (prev && s.phase === 'playing' && (prev.turn !== s.turn || prev.phase !== 'playing')) announceTurn(prev, s);
      }
    } finally {
      pumping = false;
      animating = false;
    }
  }

  // Các sự kiện giữa hai state: một lần đổ xúc xắc và / hoặc một nước đi.
  async function playEvents(prev, s) {
    const moved = !!s.last && key(s.last) !== key(prev.last);
    const sameTurn = prev.turn === s.turn;
    // Một lần đổ: có số xúc xắc, chưa đi (nước đi luôn là state riêng) và có gì đó về xúc xắc đổi:
    // vừa đổ (rolled false → true), hoặc đổ không đi được (sang lượt / ra 6 đổ tiếp — rolled vẫn false).
    const rolled = !moved && s.dice != null && (
      (s.rolled && !prev.rolled) ||
      (!s.rolled && !prev.rolled && (!sameTurn || s.sixes !== prev.sixes || s.dice !== prev.dice)));
    if (rolled) {
      const roller = s.rolled ? s.turn : prev.turn;
      const p = s.players.find((x) => x.id === roller);
      await rollDice(s.dice, p && p.color);
      if (!s.rolled) {
        // Đổ xong mà không có nước đi.
        if (sameTurn && s.dice === 6) toast('Ra 6 nhưng không có nước đi — được đổ tiếp!', p && p.color);
        else if (s.dice === 6 && (prev.sixes || 0) >= 2) toast('Ra 6 ba lần liền — mất lượt!', p && p.color);
        else toast('Không có nước đi — mất lượt', p && p.color);
        await sleep(reduced() ? 200 : 900);
      } else if (!(s.movable || []).length) {
        // Server giữ lượt ~1.8 s với rolled=true, movable rỗng rồi mới chuyển lượt / cho đổ tiếp.
        if (s.dice === 6 && (s.sixes || 0) < 3) toast('Ra 6 nhưng không có nước đi — được đổ tiếp!', p && p.color);
        else if (s.dice === 6) toast('Ra 6 ba lần liền — mất lượt!', p && p.color);
        else toast('Không có nước đi — mất lượt', p && p.color);
      }
    }
    if (moved) {
      // Đổ và đi gộp trong một state (chưa thấy lần đổ): lăn xúc xắc trước rồi mới cho ngựa chạy.
      if (!prev.rolled && s.dice != null) {
        const p = s.players.find((x) => x.color === s.last.color);
        await rollDice(s.dice, p && p.color);
      }
      shown = JSON.parse(JSON.stringify(prev.horses || {}));
      await animateMove(s.last, s.horses);
      // Ra 6 thì được đổ thêm (vẫn lượt mình, chưa đổ).
      const mover = s.players.find((x) => x.color === s.last.color);
      if (prev.dice === 6 && mover && s.turn === mover.id && !s.rolled && s.phase === 'playing') {
        toast(mover.id === s.you ? 'Ra 6 — bạn được đổ tiếp!' : `Ra 6 — ${mover.name} được đổ tiếp!`, mover.color);
        await sleep(reduced() ? 150 : 500);
      }
    }
  }

  function onState(s) {
    const prev = state;
    state = s;
    if (s.code) store(ROOM_KEY, s.code);
    deadline = s.phase === 'playing' && s.turnMs > 0 ? (s._at || Date.now()) + s.turnMs : 0;
    if (s.phase !== 'over') hideResult = false;
    const mine = me();
    if (mine) showBalance(mine.coins);
    if (s.phase === 'over' && prev && prev.phase === 'playing') {
      A.me().then((u) => { if (u) { user = u; showLevel(u); showBalance(u.balance); } }, () => {});
    }
    // Chọn sẵn ngựa đi được đầu tiên cho bàn phím.
    const mv = s.movable || [];
    if (mv.length && !mv.includes(pick)) pick = mv[0];
    render();
  }

  // ---------- Tiện ích ----------

  const me = () => (state ? state.players.find((p) => p.id === state.you) : null);
  const isHost = () => !!(me() && me().host);
  const lookOf = (p) => (p.bot ? BOT_LOOK : p);
  const myTurn = () => !!(state && state.phase === 'playing' && state.you && state.turn === state.you);
  const canRoll = () => myTurn() && !state.rolled && !animating;
  const canMove = () => myTurn() && state.rolled && (state.movable || []).length > 0 && !animating;
  const playerOf = (color) => (state ? state.players.find((p) => p.color === color) : null);

  function showBalance(coins) { $('balance').textContent = xu(coins); }

  function showLevel(u) {
    const el = $('level-badge');
    const span = Math.max(1, u.nextXP - u.levelXP);
    const p = Math.min(100, Math.round(((u.xp - u.levelXP) / span) * 100));
    el.innerHTML = `<span class="lv">Lv ${u.level}</span><span class="xp-bar"><span style="width:${p}%"></span></span>`;
    el.title = `${u.xp - u.levelXP} / ${span} XP để lên level ${u.level + 1}`;
    el.classList.remove('hidden');
  }

  function status(msg) { $('status').textContent = msg || ''; }

  const horseImg = (color, cls = '') => `<img class="horse-img ${cls}" src="${IMG}horse_${color}.png" alt="Ngựa ${COLOR_NAMES[color] || ''}" draggable="false">`;

  // Chat trong bàn: bong bóng hiện trên nhân vật ở thẻ người chơi.
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

  let rooms = null;
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
      rooms = await A.api('/api/cangua/rooms');
    } catch (e) {
      if (rooms === null) rooms = [];
    }
    if (!state) renderHall();
  }

  // Một bàn trong sảnh: bàn gỗ có bàn cờ nhỏ, 4 ghế màu quanh bàn, người ngồi là chibi.
  function hallSeat(p, color, room, open) {
    if (p) {
      const lvl = p.bot ? '<span class="lv-chip bot-chip">Máy</span>' : p.level ? `<span class="lv-chip">Lv ${p.level}</span>` : '';
      return `<div class="hseat c-${color} filled" title="${escapeHtml(p.name)}">
          <span class="seat-body">${C.sprite(lookOf(p), { dir: 'down', size: 74, cls: 'seat-char' })}</span>
          <span class="seat-label"><span class="seat-name">${escapeHtml(p.name)}</span>${lvl}</span></div>`;
    }
    const ring = `<span class="seat-ring">${horseImg(color, 'mini')}</span>`;
    if (!open) return `<div class="hseat c-${color} empty">${ring}</div>`;
    return `<button class="hseat c-${color} empty" data-join="${escapeHtml(room.code)}" title="Ngồi vào bàn ${escapeHtml(room.code)}">
        ${ring}<span class="seat-join">${I.svg('plus')}Ngồi</span></button>`;
  }

  function tableHtml(room) {
    const players = room.players || [];
    const playing = room.phase === 'playing';
    const max = room.max || 4;
    const full = players.length >= max;
    const open = !playing && !full;
    const st = playing ? 'Đang chơi' : full ? 'Đủ người' : `Đang chờ · ${players.length}/${max}`;
    const byColor = (c) => players.find((p) => p.color === c) || null;
    // Chưa có màu (phòng chờ cũ): xếp theo thứ tự ngồi.
    const seats = COLORS.map((c, i) => hallSeat(byColor(c) || (players.every((p) => !p.color) ? players[i] : null), c, room, open)).join('');
    const watch = !open
      ? `<button class="watch" data-join="${escapeHtml(room.code)}">${I.svg('eye')}Xem${room.spectators ? ` · ${room.spectators}` : ''}</button>`
      : '';
    return `<div class="htable${playing ? ' playing' : ''}">
        <div class="hscene">
          <img class="art art-table" src="${IMG}table_cangua.png" alt="" draggable="false">
          ${seats}
        </div>
        <div class="hinfo">
          <span class="tcode">${escapeHtml(room.code)}</span>
          <span class="tbet">${I.svg('coins')}${betText(room.bet ?? DEFAULT_BET)}</span>
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
    const order = (r) => (r.phase === 'playing' ? 2 : (r.players || []).length >= (r.max || 4) ? 1 : 0);
    const list = rooms.slice().sort((a, b) => order(a) - order(b));
    const invite = `<div class="htable invite">
        <div class="hscene"><img class="art art-table" src="${IMG}table_cangua.png" alt="" draggable="false"></div>
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
    const dlg = $('dlg-create');
    const form = dlg.querySelector('form');
    form.onsubmit = (e) => {
      if (e.submitter && e.submitter.value === 'cancel') return;
      const v = new FormData(form).get('bet');
      send({ t: 'create', token: A.token(), bet: v === null ? DEFAULT_BET : Number(v) });
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
    $('room-bet').innerHTML = `${I.svg('coins')}${betText(lobby ? s.bet : s.gameBet ?? s.bet)}`;
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
      $('btn-add-bot').disabled = s.players.length >= 4;
      $('sel-bot').disabled = s.players.length >= 4;
      $('btn-start').disabled = s.players.length < 2;
    }

    renderSeats();
    renderHorses();
    renderDice();
    renderStatus();
    renderResult();
  }

  // Bốn thẻ người chơi ở bốn phía bàn, đúng góc chuồng của màu.
  function renderSeats() {
    for (const color of COLORS) $('seat-' + color).innerHTML = seatPanel(playerOf(color), color);
    tickTimers();
  }

  function seatPanel(p, color) {
    const s = state;
    if (!p) {
      const host = isHost() && s.phase === 'lobby';
      return `<div class="pcard c-${color} empty">
          <div class="pmark">${horseImg(color)}</div>
          <div class="pbody">${C.sprite({ gender: 'male', outfit: 'casual' }, { dir: 'down', size: 96, cls: 'ghost-char' })}</div>
          <div class="pinfo"><div class="pname muted">Ghế ${COLOR_NAMES[color]}</div>
          ${host ? `<div class="pbtns"><button data-act="invite">${I.svg('link')}Mời</button><button data-act="bot">${I.svg('bot')}Máy</button></div>` : ''}</div>
        </div>`;
    }
    const turn = s.phase === 'playing' && s.turn === p.id;
    const mine = p.id === s.you;
    const lvl = p.bot ? `<span class="lv-chip bot-chip">Máy ${BOT_NAMES[p.botLevel] || ''}</span>` : p.level ? `<span class="lv-chip">Lv ${p.level}</span>` : '';
    const home = ((s.horses && s.horses[color]) || []).filter((v) => v >= 100).length;
    const tags = [
      mine ? '<span class="tag me">Bạn</span>' : '',
      p.host ? `<span class="tag host">${I.svg('crown')}Chủ bàn</span>` : '',
      !p.bot && p.online === false ? `<span class="tag off">${I.svg('wifiOff')}Mất kết nối</span>` : '',
      p.auto && !p.bot ? `<span class="tag off">${I.svg('bot')}Máy đi thay</span>` : '',
      s.phase === 'over' && p.ready ? `<span class="tag ready">${I.svg('check')}Sẵn sàng</span>` : '',
    ].join('');
    const kick = isHost() && !mine && s.phase === 'lobby'
      ? `<button class="link kick" data-kick="${escapeHtml(p.id)}" title="Mời ra">${I.svg('x')}Mời ra</button>` : '';
    const coins = p.bot ? '' : `<div class="pcoins">${I.svg('coins')}${xu(p.coins)}</div>`;
    return `<div class="pcard c-${color}${turn ? ' turn' : ''}${mine ? ' mine' : ''}" data-pid="${escapeHtml(p.id)}">
        <div class="pmark">${horseImg(color)}<span class="timer" data-timer="${escapeHtml(p.id)}"><span class="timer-num"></span></span></div>
        <div class="pbody">${C.sprite(lookOf(p), { dir: 'down', size: 96, cls: 'p-char' })}</div>
        <div class="pinfo">
          <div class="pname">${escapeHtml(p.name)}${lvl}</div>
          <div class="pgoal" title="Ngựa đã về đích">${[0, 1, 2, 3].map((i) => `<span class="${i < home ? 'on' : ''}"></span>`).join('')}</div>
          ${coins}
          <div class="ptags">${tags}</div>${kick}
        </div>
      </div>`;
  }

  // ---------- Ngựa trên bàn ----------

  // Mỗi ngựa là một phần tử cố định (theo màu + số), chỉ đổi toạ độ để chạy chuyển động.
  function horseEl(color, i) {
    const id = `h-${color}-${i}`;
    let el = document.getElementById(id);
    if (!el) {
      el = document.createElement('button');
      el.type = 'button';
      el.id = id;
      el.className = `horse c-${color}`;
      el.dataset.color = color;
      el.dataset.horse = i;
      el.innerHTML = `${horseImg(color)}<span class="hnum">${i + 1}</span>`;
      el.tabIndex = -1;
      $('horses').appendChild(el);
    }
    return el;
  }

  function place(el, color, i, pos) {
    const [x, y] = pct(cellOf(color, i, pos));
    el.style.left = x + '%';
    el.style.top = y + '%';
    el.style.zIndex = String(10 + Math.round(y * 10));
  }

  const key = (o) => (o ? JSON.stringify(o) : '');

  // Vẽ ngựa theo state đã áp dụng (hiệu ứng nước đi chạy trong hàng đợi trước khi áp dụng state).
  function renderHorses() {
    const s = state;
    const colors = s.players.map((p) => p.color).filter(Boolean);
    // Bỏ ngựa của màu không còn ai ngồi.
    document.querySelectorAll('#horses .horse').forEach((el) => {
      if (!colors.includes(el.dataset.color) || !s.horses || !s.horses[el.dataset.color]) el.remove();
    });
    if (!s.horses) return;
    shown = JSON.parse(JSON.stringify(s.horses));
    drawShown();
  }

  function drawShown() {
    const s = state;
    const mv = canMove() ? s.movable || [] : [];
    const myColor = me() && me().color;
    for (const [color, list] of Object.entries(shown || {})) {
      list.forEach((pos, i) => {
        const el = horseEl(color, i);
        el.classList.remove('hopping');
        place(el, color, i, pos);
        const can = color === myColor && mv.includes(i);
        el.classList.toggle('movable', can);
        el.classList.toggle('picked', can && i === pick);
        el.classList.toggle('home', pos < 0);
        el.classList.toggle('done', pos >= 100);
      });
    }
    drawGhost();
  }

  // Ngựa mờ ở ô sẽ tới của ngựa đang chọn (rê chuột / bàn phím).
  let hoverHorse = -1;
  function drawGhost() {
    let g = $('ghost');
    const s = state;
    const i = hoverHorse >= 0 ? hoverHorse : pick;
    const myColor = me() && me().color;
    if (!canMove() || !myColor || !(s.movable || []).includes(i) || !s.dice) {
      if (g) g.classList.add('hidden');
      return;
    }
    if (!g) {
      g = document.createElement('div');
      g.id = 'ghost';
      g.className = 'ghost';
      $('horses').appendChild(g);
    }
    const to = previewTarget(myColor, s.horses[myColor][i], s.dice);
    g.innerHTML = horseImg(myColor);
    g.classList.remove('hidden');
    place(g, myColor, i, to);
  }

  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const reduced = () => window.matchMedia && matchMedia('(prefers-reduced-motion: reduce)').matches;

  // Hiệu ứng một nước đi: ngựa nhảy lần lượt từng ô; nếu đá ngựa khác thì ngựa đó bay về chuồng.
  async function animateMove(last, final) {
    const { color, horse, from, to } = last;
    if (!shown[color]) shown[color] = [-1, -1, -1, -1];
    drawShown();
    const el = horseEl(color, horse);
    el.classList.remove('movable', 'picked');
    $('ghost') && $('ghost').classList.add('hidden');
    const hop = reduced() ? 40 : 230;
    for (const pos of pathOf(color, from, to)) {
      el.classList.remove('hopping');
      void el.offsetWidth; // chạy lại hiệu ứng nhảy
      el.classList.add('hopping');
      place(el, color, horse, pos);
      await sleep(hop);
    }
    el.classList.remove('hopping');
    shown[color][horse] = to;
    if (last.kicked && shown[last.kicked.color]) {
      const k = last.kicked;
      const kel = horseEl(k.color, k.horse);
      toast('Đá ngựa đối thủ về chuồng!', color);
      kel.classList.add('kicked');
      shown[k.color][k.horse] = -1;
      place(kel, k.color, k.horse, -1);
      await sleep(reduced() ? 60 : 700);
      kel.classList.remove('kicked');
    }
    if (final) shown = JSON.parse(JSON.stringify(final));
  }

  // ---------- Xúc xắc giữa bàn ----------

  // Lăn xúc xắc ~1.1 s (nảy, xoay, đổi mặt chậm dần), dừng ở mặt thật rồi giữ số ~0.6 s.
  async function rollDice(n, color) {
    const img = $('dice');
    const box = $('dice-box');
    box.className = `center-dice${color ? ' c-' + color : ''}`;
    img.classList.remove('idle');
    $('btn-roll').disabled = true;
    $('btn-roll').classList.remove('pulse');
    if (reduced()) {
      img.src = `${IMG}dice_${n}.png`;
      await sleep(400);
      return;
    }
    box.classList.add('rolling');
    const steps = [70, 75, 85, 95, 110, 130, 155, 185];
    let last = 0;
    for (const ms of steps) {
      let f = 1 + Math.floor(Math.random() * 6);
      if (f === last) f = (f % 6) + 1;
      last = f;
      img.src = `${IMG}dice_${f}.png`;
      await sleep(ms);
    }
    img.src = `${IMG}dice_${n}.png`;
    box.classList.remove('rolling');
    box.classList.add('landed');
    await sleep(600);
    box.classList.remove('landed');
  }

  // Trạng thái xúc xắc khi không lăn: mặt vừa đổ (hoặc 6 mờ), khung màu người tới lượt, nhấp nháy khi tới lượt mình.
  function renderDice() {
    const s = state;
    const img = $('dice');
    const box = $('dice-box');
    $('btn-roll').disabled = !canRoll();
    $('btn-roll').classList.toggle('pulse', canRoll());
    const six = s.sixes || 0;
    $('sixes').classList.toggle('hidden', !(s.phase === 'playing' && six > 0));
    $('sixes').textContent = six ? `Đã ra 6 × ${six}` : '';
    const tp = s.players.find((p) => p.id === s.turn);
    box.className = `center-dice${tp && tp.color ? ' c-' + tp.color : ''}${canRoll() ? ' ready' : ''}`;
    box.classList.toggle('hidden', s.phase === 'lobby');
    box.disabled = !canRoll();
    box.title = canRoll() ? 'Bấm để đổ (Space)' : '';
    if (s.dice) img.src = `${IMG}dice_${s.dice}.png`;
    // Chưa đổ trong lượt này thì xúc xắc mờ (số cũ của lượt trước).
    img.classList.toggle('idle', s.phase === 'playing' && !s.rolled);
  }

  // Dòng thông báo ngắn trên bàn (ra 6, mất lượt, đá ngựa…).
  let toastTimer = null;
  function toast(text, color) {
    const el = $('dice-toast');
    el.className = `dice-toast${color ? ' c-' + color : ''}`;
    el.textContent = text;
    void el.offsetWidth;
    el.classList.add('show');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.add('hidden'), 2200);
  }

  // Banner báo lượt: trượt vào mỗi khi đổi lượt; tới lượt mình thì nổi bật, rung thẻ, kêu "ting".
  function announceTurn(prev, s) {
    const p = s.players.find((x) => x.id === s.turn);
    const el = $('turn-banner');
    if (!p) return el.classList.add('hidden');
    const mine = p.id === s.you;
    el.className = `turn-banner c-${p.color}${mine ? ' mine' : ''}`;
    el.innerHTML = `<span class="tb-head">${C.head(lookOf(p))}</span>
      <span class="tb-text">${mine ? '<b>Tới lượt bạn!</b> Bấm xúc xắc / Space' : `Lượt của <b>${escapeHtml(p.name)}</b> (${COLOR_NAMES[p.color] || ''})`}</span>`;
    void el.offsetWidth;
    el.classList.add('in');
    if (mine) {
      const card = document.querySelector(`.pcard[data-pid="${CSS.escape(p.id)}"]`);
      if (card) { card.classList.remove('shake'); void card.offsetWidth; card.classList.add('shake'); }
      ding();
    }
  }

  // Âm báo ngắn tới lượt (WebAudio, không cần file âm thanh; trình duyệt chặn thì thôi).
  let audio = null;
  function ding() {
    try {
      audio = audio || new (window.AudioContext || window.webkitAudioContext)();
      if (audio.state === 'suspended') audio.resume();
      const t = audio.currentTime;
      [880, 1320].forEach((f, i) => {
        const o = audio.createOscillator();
        const g = audio.createGain();
        o.type = 'sine';
        o.frequency.value = f;
        g.gain.setValueAtTime(0.0001, t + i * 0.12);
        g.gain.exponentialRampToValueAtTime(0.18, t + i * 0.12 + 0.02);
        g.gain.exponentialRampToValueAtTime(0.0001, t + i * 0.12 + 0.25);
        o.connect(g).connect(audio.destination);
        o.start(t + i * 0.12);
        o.stop(t + i * 0.12 + 0.3);
      });
    } catch (e) { /* không có âm thanh */ }
  }

  function renderStatus() {
    const s = state;
    if (s.phase === 'lobby') {
      const n = s.players.length;
      return status(n < 2 ? 'Chờ thêm người chơi — mời bạn hoặc thêm máy.' : isHost() ? `Đủ ${n} người, bấm Bắt đầu.` : `Đủ ${n} người, chờ chủ bàn bắt đầu.`);
    }
    if (s.phase === 'over') return status(resultLine());
    const tp = s.players.find((p) => p.id === s.turn);
    if (myTurn()) {
      if (!s.rolled) return status('Tới lượt bạn — bấm Đổ (Space)');
      if ((s.movable || []).length) return status(`Ra ${s.dice} — chọn ngựa để đi (1–4, Enter)`);
      return status(`Ra ${s.dice} — không có ngựa nào đi được`);
    }
    if (!tp) return status('');
    status(s.rolled && s.dice ? `${tp.name} ra ${s.dice}…` : `${tp.name} đang đổ xúc xắc…`);
  }

  function resultLine() {
    const s = state;
    const w = s.players.find((p) => p.id === s.winner);
    if (!w) return 'Hết ván';
    return `${w.id === s.you ? 'Bạn' : w.name} (${COLOR_NAMES[w.color] || ''}) đưa đủ 4 ngựa về đích trước`;
  }

  function renderResult() {
    const s = state;
    const el = $('result');
    const show = s.phase === 'over' && !hideResult;
    const wasHidden = el.classList.contains('hidden');
    el.classList.toggle('hidden', !show);
    if (!show) return;
    const r = s.result || {};
    const mine = me();
    const w = s.players.find((p) => p.id === s.winner);
    let title = 'Hết ván';
    let tone = 'draw';
    if (w && mine && w.id === mine.id) { title = 'Bạn thắng!'; tone = 'win'; }
    else if (w && mine) { title = 'Bạn thua'; tone = 'lose'; }
    else if (w) { title = `${w.name} thắng`; tone = 'win'; }
    el.dataset.tone = tone;
    $('result-title').innerHTML = `${w && w.color ? horseImg(w.color, 'res-horse') : ''}${escapeHtml(title)}`;
    $('result-sub').textContent = resultLine();
    const money = mine && r.money ? r.money[mine.id] || 0 : 0;
    const xp = mine && r.xp ? r.xp[mine.id] || 0 : 0;
    const coins = s.gameBet ? `<span class="gain ${money > 0 ? 'plus' : money < 0 ? 'minus' : ''}">${I.svg('coins')}${signed(money)} xu</span>` : '';
    $('result-gain').innerHTML = mine ? `${coins}${xp ? `<span class="gain xp">+${xp} XP</span>` : ''}` : '';
    const ready = mine && mine.ready;
    $('btn-rematch').disabled = !mine || !!ready;
    $('btn-rematch').innerHTML = ready ? `${I.svg('timer')}Chờ người khác…` : `${I.svg('play')}Ván mới <kbd>N</kbd>`;
    if (wasHidden && tone === 'win') confetti();
  }

  // Pháo giấy: các mảnh cắt từ dải confetti.png rơi xuống, xoay.
  function confetti() {
    const box = $('confetti');
    if (reduced()) return;
    box.innerHTML = Array.from({ length: 46 }, () => {
      const left = Math.random() * 100;
      const delay = Math.random() * 0.8;
      const dur = 2.2 + Math.random() * 1.6;
      const piece = Math.floor(Math.random() * 6);
      const spin = Math.random() < 0.5 ? -1 : 1;
      return `<i style="left:${left}%;animation-delay:${delay}s;animation-duration:${dur}s;background-position:${-piece * 24}px 0;--spin:${spin}"></i>`;
    }).join('');
    setTimeout(() => { box.innerHTML = ''; }, 4500);
  }

  // Đồng hồ 20 s dạng vòng quanh ngựa của người tới lượt.
  function tickTimers() {
    if (!state) return;
    const left = deadline ? Math.max(0, deadline - Date.now()) : 0;
    const total = Math.max(1000, state.turnMs > 0 ? 20000 : 20000);
    document.querySelectorAll('[data-timer]').forEach((el) => {
      const on = state.phase === 'playing' && state.turn === el.dataset.timer && deadline;
      el.classList.toggle('on', !!on);
      el.classList.toggle('low', !!on && left <= 6000);
      el.style.setProperty('--p', on ? Math.min(1, left / total) : 0);
      el.querySelector('.timer-num').textContent = on ? Math.ceil(left / 1000) : '';
    });
  }
  setInterval(tickTimers, 200);

  // ---------- Hành động ----------

  function roll() {
    if (!canRoll()) return;
    $('btn-roll').disabled = true;
    send({ t: 'roll' });
  }

  function moveHorse(i) {
    if (!canMove() || !(state.movable || []).includes(i)) return;
    hoverHorse = -1;
    send({ t: 'move', horse: i });
  }

  $('horses').addEventListener('click', (e) => {
    const h = e.target.closest('.horse');
    if (!h || !state) return;
    const color = h.dataset.color;
    const i = +h.dataset.horse;
    if (me() && color === me().color) { pick = i; moveHorse(i); }
  });
  $('horses').addEventListener('pointerover', (e) => {
    const h = e.target.closest('.horse.movable');
    hoverHorse = h ? +h.dataset.horse : -1;
    if (state) drawGhost();
  });
  $('horses').addEventListener('pointerleave', () => { hoverHorse = -1; if (state) drawGhost(); });

  async function copyInvite() {
    const link = `${location.origin}/cangua/?room=${state.code}`;
    try {
      await navigator.clipboard.writeText(link);
      A.toast('Đã sao chép link mời — gửi cho bạn bè để vào bàn');
    } catch (e) {
      A.ask({ title: 'Mời bạn', message: 'Sao chép link này gửi cho bạn bè:', input: link, ok: 'Xong', cancel: '' });
    }
  }

  async function leave() {
    if (!state) return;
    if (state.phase === 'playing' && me()) {
      const ok = await A.confirm('Rời bàn giữa ván sẽ để máy đi thay và bạn vẫn có thể bị thua tiền cược. Vẫn rời?', { title: 'Rời bàn', ok: 'Rời bàn', cancel: 'Ở lại', danger: true });
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
  $('btn-roll').addEventListener('click', roll);
  $('dice-box').addEventListener('click', roll);
  $('btn-rematch').addEventListener('click', rematch);
  $('btn-board').addEventListener('click', () => { hideResult = true; renderResult(); });
  $('btn-hall').addEventListener('click', () => send({ t: 'leave' }));
  $('btn-add-bot').addEventListener('click', () => send({ t: 'addBot', level: $('sel-bot').value }));
  $('btn-start').addEventListener('click', () => send({ t: 'start' }));
  $('sel-bet').addEventListener('change', (e) => send({ t: 'setBet', bet: Number(e.target.value) }));
  $('room').addEventListener('click', (e) => {
    const kick = e.target.closest('[data-kick]');
    if (kick) return send({ t: 'kick', id: kick.dataset.kick });
    const act = e.target.closest('[data-act]');
    if (!act) return;
    if (act.dataset.act === 'invite') copyInvite();
    if (act.dataset.act === 'bot') send({ t: 'addBot', level: $('sel-bot').value });
  });

  // ---------- Bàn phím ----------

  document.addEventListener('keydown', (e) => {
    if (!state || e.ctrlKey || e.metaKey || e.altKey) return;
    const t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    if (document.querySelector('dialog[open]')) return;
    const mv = state.movable || [];
    // Space / Enter: chưa đổ thì đổ, đã đổ thì đi ngựa đang chọn (nút đang focus thì để nút tự xử lý).
    if ((e.key === ' ' || e.key === 'Enter') && (canRoll() || canMove())) {
      if (t && t.tagName === 'BUTTON' && !t.classList.contains('horse') && t.id !== 'btn-roll') return;
      e.preventDefault();
      if (canRoll()) roll();
      else moveHorse(pick);
    } else if (/^[1-4]$/.test(e.key) && canMove()) {
      const i = +e.key - 1;
      if (mv.includes(i)) { e.preventDefault(); pick = i; drawShown(); moveHorse(i); }
    } else if (e.key === 'Tab' && canMove() && mv.length) {
      e.preventDefault();
      const k = mv.indexOf(pick);
      pick = mv[(k + (e.shiftKey ? mv.length - 1 : 1)) % mv.length];
      drawShown();
    } else if ((e.key === 'n' || e.key === 'N') && state.phase === 'over') {
      rematch();
    } else if (e.key === 'Escape' && state.phase === 'over' && !hideResult) {
      hideResult = true;
      renderResult();
    }
  });

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(28);
  $('home-art').innerHTML = `<span class="hall-horses">${horseImg('red')}${horseImg('blue')}<img class="hall-dice" src="${IMG}dice_6.png" alt="" draggable="false"></span>`;

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
