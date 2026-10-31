// Client Đấu trường: server Go mô phỏng đường đạn, địa hình, máu, lượt; client chỉ hiển thị, diễn lại phát bắn
// và gửi lệnh (đi, ngắm, bắn). Thế giới 1600 × 900 đơn vị, y hướng xuống, góc tuyệt đối 0 = phải, 90 = lên, 180 = trái.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const xu = A.xu;
  const escapeHtml = A.escapeHtml;
  const signed = (n) => (n > 0 ? '+' : n < 0 ? '−' : '') + xu(Math.abs(n));

  const IMG = '/img/toon/arena/';
  const W = 1600;
  const H = 900;
  const RESULT_DELAY = 2500; // ms giữ chiến trường trước khi hiện bảng kết quả
  const ROOM_KEY = 'arena.room';
  const DEFAULT_BETS = [0, 10, 20, 50, 100, 200, 500, 1000];
  const DEFAULT_BET = 10;
  const TURN_MS = 25000;
  const MOVE_BUDGET = 120;
  const MAX_SEATS = 4;
  const STEP_MS = 1000 / 30; // các điểm trong paths cách nhau 1/30 s
  const CHARGE_MS = 1800; // giữ nút bắn: lực 0 → 100
  const DEFAULT_MAPS = [
    { id: 'meadow', name: 'Đồng cỏ' },
    { id: 'desert', name: 'Sa mạc' },
    { id: 'snow', name: 'Núi tuyết' },
    { id: 'volcano', name: 'Núi lửa' },
    { id: 'beach', name: 'Bãi biển' },
  ];
  const MAP_NOTE = { meadow: 'Đồi thoải, gió nhẹ', desert: 'Gió mạnh ±10', snow: 'Núi cao dốc', volcano: 'Địa hình gồ ghề', beach: 'Giữa có vũng nước sâu' };
  const WEAPONS = [
    { id: 'shell', name: 'Đạn thường', note: '30 sát thương' },
    { id: 'triple', name: 'Đạn chùm', note: '3 viên × 14' },
    { id: 'heavy', name: 'Bom tạ', note: '55 sát thương, nổ rộng' },
    { id: 'tele', name: 'Dịch chuyển', note: 'Bay tới đâu, bạn tới đó' },
  ];
  const GEAR = {
    gun: { name: 'Vũ khí', stat: (l) => `+${l * 8}% sát thương` },
    hat: { name: 'Nón', stat: (l) => `+${l * 15} máu tối đa` },
    armor: { name: 'Áo giáp', stat: (l) => `−${l * 3}% sát thương nhận` },
  };
  const BOT_NAMES = { easy: 'Dễ', normal: 'Vừa', hard: 'Khó' };
  const REASONS = { last: 'trụ lại cuối cùng', rounds: 'hết lượt, còn nhiều máu nhất', draw: 'ngang máu', left: 'đối thủ rời bàn' };
  const BOSS_REASONS = { boss: 'đã hạ boss', wiped: 'cả đội bị hạ', rounds: 'hết lượt mà boss vẫn còn' };
  const BOT_LOOK = { gender: 'male', outfit: 'suit' };
  const SEAT_TINT = ['#e5484d', '#2f86d6', '#3aa94a', '#d97a06'];
  const HALL_REFRESH = 3000;
  // Dạng trang bị theo cấp cường hoá: +0..+6 dạng 1, +7..+9 biến đổi lần 1, +10 huyền thoại.
  const form = (lv) => (lv >= 10 ? 3 : lv >= 7 ? 2 : 1);
  // Vũ khí (đồ ném): mỗi món có cấp cường hoá riêng; món đang dùng là ô "Vũ khí". Server gửi danh sách đầy đủ
  // trong /api/arena/gear (arms) — bảng này chỉ để có tên khi chưa tải.
  const ARM_NAMES = { cannon: 'Đại bác', dep: 'Dép lào', choi: 'Chổi chà', launha: 'Cây lau nhà', tv: 'Ti vi', tulanh: 'Tủ lạnh' };
  const armOf = (g) => (g && g.arm) || 'cannon';
  const armName = (id) => {
    const a = gear && (gear.arms || []).find((x) => x.id === id);
    return (a && a.name) || ARM_NAMES[id] || id;
  };
  // Bốn thuộc tính của vũ khí; chỉ số sau cường hoá = gốc × (1 + 0,1 × cấp).
  const ARM_STATS = [
    { k: 'atk', name: 'Tấn công', short: 'TC' },
    { k: 'def', name: 'Phòng thủ', short: 'PT' },
    { k: 'luck', name: 'May mắn', short: 'MM', unit: '%' },
    { k: 'sta', name: 'Thể lực', short: 'TL' },
  ];
  const armData = (id) => (gear && (gear.arms || []).find((x) => x.id === id)) || null;
  const statAt = (a, k, lv) => Math.round((a[k] || 0) * (1 + 0.1 * lv));
  const statVal = (st, v) => `${v}${st.unit || ''}`;
  // Dòng chỉ số ngắn của vũ khí ở cấp lv (mặc định cấp hiện có).
  const armStatLine = (id, lv) => {
    const a = armData(id);
    if (!a) return '';
    return ARM_STATS.map((st) => `${st.short} ${statVal(st, statAt(a, st.k, lv))}`).join(' · ');
  };
  // ---------- Chế độ Đấu boss: cả bàn hợp sức đánh một boss do máy điều khiển ----------
  // state.mode === 'boss', state.boss = { id, kind, name, x, y, hp, maxHp, alive }.
  const isBoss = (s) => !!s && (s.mode === 'boss' || !!s.boss);
  const bossOf = (s) => (s && s.boss) || null;
  const bossKind = (b) => (b && (b.kind || b.bossId || b.id)) || '';
  // Id của boss trong tin bắn / trúng đòn (server có thể dùng id riêng hoặc 'boss').
  const isBossId = (id) => {
    const b = bossOf(state);
    return !!b && (id === b.id || id === 'boss' || id === `boss:${bossKind(b)}`);
  };
  let bosses = [];
  async function loadBosses() {
    if (bosses.length) return bosses;
    try {
      const r = await A.api('/api/arena/bosses');
      bosses = Array.isArray(r) ? r : (r && r.bosses) || [];
    } catch (e) { bosses = []; }
    return bosses;
  }
  // Icon vũ khí; chưa có ảnh riêng thì dùng hình súng theo dạng.
  const armIcon = (id, lv, cls = '') => `<img class="${cls}" src="${IMG}icon_arm_${id}.png" onerror="this.onerror=null;this.src='${IMG}gun_${form(lv || 0)}.png'" alt="" draggable="false">`;
  const betText = (b) => (b ? `${xu(b)} xu` : 'Chơi vui');
  const clamp = (v, a, b) => Math.max(a, Math.min(b, v));

  let ws = null;
  let state = null; // state đang hiển thị
  let queued = []; // state / tin đến trong lúc đang diễn phát bắn
  let pending = null;
  let stopped = false;
  let rejoining = false;
  let deadline = 0;
  let hideResult = false;
  let resultTimer = 0;
  let user = null;
  let maps = DEFAULT_MAPS;

  // Ngắm của mình: độ nâng 0..90 + hướng mặt → góc tuyệt đối.
  const aim = { elev: 45, facing: 1, power: 60, weapon: 'shell', turnKey: '' };
  const aimOf = () => (aim.facing > 0 ? aim.elev : 180 - aim.elev);
  const peers = {}; // ngắm của người khác: id → { angle, power }

  // Hiển thị chiến trường
  let ground = null; // Int16Array đang vẽ (khoét dần khi nổ trong lúc diễn)
  let terrainDirty = true;
  let shot = null; // { msg, t0, done:Set }
  const booms = []; // { x, y, r, t0 }
  const shakes = { until: 0 };
  let cam = { x: 0, follow: true }; // điện thoại: camera chạy ngang theo đạn / người tới lượt

  const store = (k, v) => { try { v == null ? sessionStorage.removeItem(k) : sessionStorage.setItem(k, v); } catch (e) { /* bỏ qua */ } };
  const load = (k) => { try { return sessionStorage.getItem(k); } catch (e) { return null; } };

  // ---------- Ảnh ----------

  const imgs = {};
  function img(name) {
    if (imgs[name]) return imgs[name];
    const im = new Image();
    im.decoding = 'async';
    im.onload = () => { if (/^(ground|grass)_/.test(name)) terrainDirty = true; };
    im.src = IMG + name + '.png';
    imgs[name] = im;
    return im;
  }
  const ready = (im) => im && im.complete && im.naturalWidth > 0;
  ['shot_shell', 'shot_triple', 'shot_heavy', 'shot_tele', 'smoke', 'boom_1', 'boom_2', 'boom_3', 'boom_4', 'boom_5', 'boom_6'].forEach(img);

  // ---------- Kết nối ----------

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/ws/arena`);
    ws.onopen = () => {
      const code = load(ROOM_KEY) || new URLSearchParams(location.search).get('room');
      if (code) send({ t: 'join', code: code.toUpperCase(), token: A.token() });
      else if (pending) send(pending);
      pending = null;
    };
    ws.onmessage = (e) => onMessage(JSON.parse(e.data));
    ws.onclose = () => {
      if (stopped) return;
      rejoining = !!state;
      if (state) note('Mất kết nối, đang kết nối lại…');
      setTimeout(connect, 1000);
    };
  }

  function roomLost() {
    rejoining = false;
    leaveRoomView();
    A.alert('Máy chủ vừa được cập nhật nên bàn cũ đã đóng. Trận đang dở được huỷ, không ai bị trừ xu. Mời bạn mở bàn mới.', { title: 'Bàn đã đóng' });
  }

  function leaveRoomView() {
    store(ROOM_KEY, null);
    state = null;
    queued = [];
    shot = null;
    tchat.reset();
    history.replaceState(null, '', location.pathname);
    render();
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
        // Đang diễn phát bắn: giữ state lại, diễn xong mới áp (đất bị khoét, máu trừ đúng lúc nổ).
        m.at = Date.now(); // đồng hồ lượt tính từ lúc nhận, không phải lúc áp
        if (shot) queued.push(m);
        else onState(m);
        tchat.sync(m.chat);
        break;
      case 'shot':
        startShot(m);
        break;
      case 'aim':
        if (m.id && (!state || m.id !== state.you)) peers[m.id] = { angle: m.angle, power: m.power, at: Date.now() };
        break;
      case 'chat':
        tchat.add(m);
        break;
      case 'loot':
        showLoot(m);
        break;
      case 'error':
        if (rejoining) { roomLost(); break; }
        A.toast(m.msg || m.error || 'Lỗi');
        if (!state) { store(ROOM_KEY, null); history.replaceState(null, '', location.pathname); }
        break;
      case 'left':
        leaveRoomView();
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
    if (s.maps && s.maps.length) maps = s.maps;
    deadline = s.phase === 'playing' && s.turnMs > 0 ? (s.at || Date.now()) + s.turnMs : 0;
    if (s.ground && s.ground.length) {
      ground = Int16Array.from(s.ground);
      terrainDirty = true;
    }
    if (s.phase !== 'over') { hideResult = false; clearTimeout(resultTimer); resultTimer = 0; }
    if (s.phase === 'over' && prev && prev.phase === 'playing' && prev.code === s.code) {
      hideResult = true;
      clearTimeout(resultTimer);
      resultTimer = setTimeout(() => { resultTimer = 0; hideResult = false; if (state && state.phase === 'over') renderResult(); }, RESULT_DELAY);
    }
    // Lượt mới của mình: lấy lại ngắm của phát trước để canh tiếp.
    const mine = me();
    const key = s.phase === 'playing' ? `${s.round}:${s.turn}` : '';
    if (mine && key !== aim.turnKey) {
      aim.turnKey = key;
      if (s.turn === mine.id) {
        const a = mine.lastAngle || (mine.facing < 0 ? 135 : 45);
        aim.facing = a > 90 ? -1 : 1;
        aim.elev = aim.facing > 0 ? a : 180 - a;
        if (mine.lastPower) aim.power = mine.lastPower;
        cam.follow = true;
        pingTurn();
      }
    }
    if (mine) showBalance(mine.coins);
    if (s.phase === 'over' && prev && prev.phase === 'playing') {
      A.me().then((u) => { if (u) { user = u; showLevel(u); showBalance(u.balance); } }, () => {});
    }
    if (prev && prev.code !== s.code) { for (const k in peers) delete peers[k]; }
    render();
  }

  // ---------- Tiện ích ----------

  const me = () => (state ? state.players.find((p) => p.id === state.you) : null);
  const isHost = () => !!(me() && me().host);
  const myTurn = () => !!(state && state.phase === 'playing' && me() && me().alive !== false && state.turn === me().id && !shot);
  const lookOf = (p) => (p.bot ? BOT_LOOK : p);
  const mapName = (id) => (id === 'random' ? 'Ngẫu nhiên' : (maps.find((m) => m.id === id) || {}).name || id);
  const seatIndex = (p) => Math.max(0, state.players.indexOf(p));

  function showBalance(coins) { $('balance').textContent = xu(coins); }
  function showLevel(u) {
    const el = $('level-badge');
    const span = Math.max(1, u.nextXP - u.levelXP);
    const pct = Math.min(100, Math.round(((u.xp - u.levelXP) / span) * 100));
    el.innerHTML = `<span class="lv">Lv ${u.level}</span><span class="xp-bar"><span style="width:${pct}%"></span></span>`;
    el.title = `${u.xp - u.levelXP} / ${span} XP để lên level ${u.level + 1}`;
    el.classList.remove('hidden');
  }
  function note(msg) { $('hud-note').textContent = msg || ''; $('lobby-note').textContent = msg || ''; }

  // Chat trong bàn: bong bóng hiện trên đầu nhân vật trong chiến trường (hoặc thẻ phòng chờ).
  const tchat = window.TableChat.mount({
    send: (text) => send({ t: 'chat', text }),
    isMe: (line) => !!state && !!state.you && line.id === state.you,
    anchor: (line) => document.querySelector(`#units .unit[data-pid="${CSS.escape(line.id || '')}"] .u-char`) ||
      document.querySelector(`.lcard[data-pid="${CSS.escape(line.id || '')}"] .lbody`),
  });

  // ---------- Hiển thị chung ----------

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
    try { rooms = await A.api('/api/arena/rooms'); } catch (e) { if (rooms === null) rooms = []; }
    if (!Array.isArray(rooms)) rooms = (rooms && rooms.rooms) || [];
    if (!state) renderHall();
  }

  function tableHtml(room) {
    const players = room.players || [];
    const max = room.max || MAX_SEATS;
    const playing = room.phase === 'playing';
    const full = players.length >= max;
    const open = !playing && !full;
    const mapId = room.map && typeof room.map === 'object' ? room.map.id : room.map;
    const bg = mapId && mapId !== 'random' ? `style="background-image:url('${IMG}bg_${escapeHtml(mapId)}.png')"` : '';
    const seats = [];
    for (let i = 0; i < max; i++) {
      const p = players[i];
      seats.push(p
        ? `<span class="tseat" title="${escapeHtml(p.name)}">${C.sprite(lookOf(p), { dir: i % 2 ? 'left' : 'right', size: 66 })}<span class="tname">${escapeHtml(p.name)}</span></span>`
        : `<span class="tseat empty">${open ? `<button class="tjoin" data-join="${escapeHtml(room.code)}">${I.svg('plus')}Vào</button>` : ''}</span>`);
    }
    const st = playing ? 'Đang đấu' : full ? 'Đủ người' : `Đang chờ · ${players.length}/${max}`;
    const watch = !open ? `<button class="watch" data-join="${escapeHtml(room.code)}">${I.svg('eye')}Xem${room.spectators ? ` · ${room.spectators}` : ''}</button>` : '';
    return `<div class="atable${playing ? ' playing' : ''}">
        <div class="ascene" ${bg}><div class="aseats">${seats.join('')}</div></div>
        <div class="hinfo">
          <span class="tcode">${escapeHtml(room.code)}</span>
          ${room.mode === 'boss' || room.boss ? `<span class="tboss">${I.svg('flame')}Boss: ${escapeHtml(room.bossName || (room.boss && room.boss.name) || (bosses.find((b) => b.id === room.boss) || {}).name || room.boss || '')}</span>`
            : `<span>${I.svg('coins')}${betText(room.bet ?? DEFAULT_BET)}</span>`}
          <span>${escapeHtml(room.mapName || mapName(mapId || 'random'))}</span>
          <span class="tstatus${playing ? ' live' : ''}">${st}</span>
          ${watch}
        </div>
      </div>`;
  }

  function renderHall() {
    const el = $('tables');
    if (rooms === null) { el.innerHTML = '<p class="hall-empty">Đang tải các bàn…</p>'; return; }
    const order = (r) => (r.phase === 'playing' ? 2 : (r.players || []).length >= (r.max || MAX_SEATS) ? 1 : 0);
    const list = rooms.slice().sort((a, b) => order(a) - order(b));
    const invite = `<div class="atable invite">
        <div class="ascene" style="background-image:url('${IMG}bg_meadow.png')"><img class="invite-cannon" src="${IMG}cannon_base.png" alt="" draggable="false"></div>
        <div class="hinfo"><button class="primary" data-create>${I.svg('plus')}${list.length ? 'Mở thêm bàn' : 'Tạo bàn đầu tiên'}</button>
          <span class="tnote">${list.length ? 'Mời bạn hoặc đấu với máy' : 'Chưa có bàn nào, mở bàn và mời bạn bè'}</span></div>
      </div>`;
    el.innerHTML = list.map(tableHtml).join('') + invite;
  }

  function openCreate() {
    loadBosses().then(renderBossGrid);
    renderBossGrid();
    setCreateMode('pvp');
    const bets = (state && state.betOptions) || DEFAULT_BETS;
    $('bet-grid').innerHTML = bets
      .map((b) => `<label class="bet-opt"><input type="radio" name="bet" value="${b}" ${b === DEFAULT_BET ? 'checked' : ''}>
          <span>${I.svg('coins')}${b ? xu(b) : 'Chơi vui'}<small>${b ? 'xu/trận' : '0 xu'}</small></span></label>`)
      .join('');
    $('map-grid').innerHTML = [{ id: 'random', name: 'Ngẫu nhiên' }, ...maps]
      .map((m, i) => `<label class="map-opt"><input type="radio" name="map" value="${escapeHtml(m.id)}" ${i === 0 ? 'checked' : ''}>
          <span class="mo-card"${m.id === 'random' ? '' : ` style="background-image:url('${IMG}bg_${escapeHtml(m.id)}.png')"`}>
            <b>${escapeHtml(m.name)}</b><small>${escapeHtml(MAP_NOTE[m.id] || (m.id === 'random' ? 'Máy chọn mỗi trận' : ''))}</small></span></label>`)
      .join('');
    const dlg = $('dlg-create');
    const form = dlg.querySelector('form');
    form.onsubmit = (e) => {
      if (e.submitter && e.submitter.value === 'cancel') return;
      const fd = new FormData(form);
      const bet = fd.get('bet') === null ? DEFAULT_BET : Number(fd.get('bet'));
      if (fd.get('mode') === 'boss') {
        const boss = fd.get('boss') || (bosses[0] && bosses[0].id);
        if (!boss) { e.preventDefault(); A.toast('Chưa tải được danh sách boss'); return; }
        send({ t: 'create', token: A.token(), mode: 'boss', boss, bet: 0, map: fd.get('map') || 'random' });
        return;
      }
      send({ t: 'create', token: A.token(), bet, map: fd.get('map') || 'random' });
    };
    dlg.showModal();
  }

  function setCreateMode(mode) {
    document.querySelectorAll('#dlg-create input[name="mode"]').forEach((r) => { r.checked = r.value === mode; });
    $('dlg-create').dataset.mode = mode;
  }

  function renderBossGrid() {
    const el = $('boss-grid');
    if (!bosses.length) { el.innerHTML = '<p class="muted">Đang tải danh sách boss…</p>'; return; }
    el.innerHTML = bosses.map((b, i) => `<label class="boss-opt"><input type="radio" name="boss" value="${escapeHtml(b.id)}" ${i === 0 ? 'checked' : ''}>
        <span class="bo-card"><img src="${IMG}boss_${escapeHtml(b.id)}.png" alt="" draggable="false">
          <b>${escapeHtml(b.name)}</b><small>${escapeHtml([BOT_NAMES[b.level] ? `Độ khó ${BOT_NAMES[b.level]}` : '', b.coins ? `thưởng ${xu(b.coins)} xu + ngọc` : ''].filter(Boolean).join(' · '))}</small></span></label>`).join('');
  }
  $('dlg-create').addEventListener('change', (e) => {
    if (e.target.name === 'mode') setCreateMode(e.target.value);
  });

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
    $('room-bet').innerHTML = isBoss(s) ? `${I.svg('flame')}Boss: ${escapeHtml((bossOf(s) || {}).name || '')}` : `${I.svg('coins')}${betText(lobby ? s.bet : s.gameBet ?? s.bet)}`;
    const mid = s.map && s.map.id;
    $('room-map').textContent = s.map ? s.map.name || mapName(mid) : mapName(s.mapChoice || 'random');
    $('room-round').textContent = s.phase === 'playing' && s.round ? `Lượt ${s.round}/${s.maxRounds || 40}` : '';
    $('room-round').classList.toggle('hidden', !(s.phase === 'playing' && s.round));
    $('room-watch').innerHTML = `${I.svg('eye')}${s.spectators || 0}`;
    $('room-watch').classList.toggle('hidden', !s.spectators);
    $('room-watch').title = 'Người đang xem';

    $('lobby').classList.toggle('hidden', !lobby);
    $('battle').classList.toggle('hidden', lobby);
    if (lobby) renderLobby(host);
    else renderBattle();
    renderResult();
  }

  function renderLobby(host) {
    const s = state;
    document.querySelectorAll('#lobby .host-only').forEach((el) => el.classList.toggle('hidden', !host));
    const mine = me();
    $('btn-ready').classList.toggle('hidden', host || !mine);
    $('btn-ready').innerHTML = mine && mine.ready ? `${I.svg('x')}Huỷ sẵn sàng` : `${I.svg('check')}Sẵn sàng`;
    const sel = $('sel-bet');
    const opts = s.betOptions || DEFAULT_BETS;
    if (sel.options.length !== opts.length) sel.innerHTML = opts.map((b) => `<option value="${b}">${b ? xu(b) + ' xu' : '0 (chơi vui)'}</option>`).join('');
    sel.value = s.bet;
    const ms = $('sel-map');
    const all = [{ id: 'random', name: 'Ngẫu nhiên' }, ...maps];
    if (ms.options.length !== all.length) ms.innerHTML = all.map((m) => `<option value="${escapeHtml(m.id)}">${escapeHtml(m.name)}</option>`).join('');
    const choice = s.mapChoice || (s.map && s.map.id) || 'random';
    ms.value = choice;
    const full = s.players.length >= MAX_SEATS;
    $('btn-add-bot').disabled = full;
    $('sel-bot').disabled = full;
    $('btn-start').disabled = s.players.length < (isBoss(s) ? 1 : 2);
    $('sel-bet').closest('label').classList.toggle('hidden', !host || isBoss(s));
    // Phòng boss: chủ bàn đổi boss ngay ở phòng chờ.
    const bl = (s.bosses && s.bosses.length ? s.bosses : bosses);
    $('sel-boss').closest('label').classList.toggle('hidden', !host || !isBoss(s));
    if (isBoss(s)) {
      const bs = $('sel-boss');
      const sig = bl.map((b) => b.id).join(',');
      if (bs.dataset.sig !== sig) { bs.innerHTML = bl.map((b) => `<option value="${escapeHtml(b.id)}">${escapeHtml(b.name)}</option>`).join(''); bs.dataset.sig = sig; }
      bs.value = (bossOf(s) || {}).id || '';
    }
    $('lobby-map').style.backgroundImage = choice !== 'random' ? `url('${IMG}bg_${choice}.png')` : '';
    $('lobby-map').innerHTML = `<span class="lm-name">${escapeHtml(mapName(choice))}</span><span class="lm-note">${escapeHtml(MAP_NOTE[choice] || 'Mỗi trận một bản đồ')}</span>`
      + (isBoss(s) && bossOf(s) ? `<img class="lm-boss" src="${IMG}boss_${escapeHtml(bossOf(s).id)}.png" alt="" draggable="false"><span class="lm-bossname">${I.svg('flame')}${escapeHtml(bossOf(s).name)} · ${xu(bossOf(s).maxHp || 0)} máu</span>` : '');
    const cards = [];
    for (let i = 0; i < MAX_SEATS; i++) cards.push(lobbyCard(s.players[i], i));
    $('lobby-seats').innerHTML = cards.join('');
    let msg;
    if (!mine) msg = 'Bạn đang xem bàn này';
    else if (isBoss(s)) msg = host ? `Đấu boss ${(bossOf(s) || {}).name || ''} — mời thêm bạn hoặc bắt đầu luôn.` : 'Chờ chủ bàn bắt đầu đánh boss…';
    else if (s.players.length < 2) msg = 'Chờ thêm người — mời bạn hoặc thêm máy.';
    else if (host) msg = 'Đủ người, bấm Bắt đầu.';
    else msg = 'Chờ chủ bàn bắt đầu…';
    $('lobby-note').textContent = msg;
  }

  function gearBadges(p) {
    const g = p.gear || {};
    return ['gun', 'hat', 'armor'].map((k) => {
      const arm = k === 'gun' && armOf(g) !== 'cannon' ? armOf(g) : '';
      const pic = arm ? armIcon(arm, g[k]) : g[k] ? `<img src="${IMG}${k}_${form(g[k])}.png" alt="" draggable="false">` : '';
      const title = arm ? `${ARM_NAMES[arm] || arm} cấp ${g[k] || 0}` : `${GEAR[k].name} cấp ${g[k] || 0}`;
      return `<span class="gb" title="${escapeHtml(title)}">${pic}<b>${g[k] || 0}</b></span>`;
    }).join('');
  }

  function lobbyCard(p, i) {
    const s = state;
    if (!p) {
      const host = isHost();
      return `<div class="lcard empty">
          <div class="lbody">${C.sprite({ gender: 'male', outfit: 'casual' }, { dir: 'down', size: 110, cls: 'ghost-char' })}</div>
          <div class="lname muted">Ghế trống</div>
          ${host ? `<button data-act="invite">${I.svg('link')}Mời</button><button data-act="bot">${I.svg('bot')}Thêm máy</button>` : ''}
        </div>`;
    }
    const mine = p.id === s.you;
    const lvl = p.bot ? `<span class="lv-chip bot-chip">Máy ${BOT_NAMES[p.botLevel] || ''}</span>` : p.level ? `<span class="lv-chip">Lv ${p.level}</span>` : '';
    const tags = [
      mine ? '<span class="tag me">Bạn</span>' : '',
      p.host ? `<span class="tag host">${I.svg('crown')}Chủ bàn</span>` : '',
      !p.bot && p.online === false ? `<span class="tag off">${I.svg('wifiOff')}Mất kết nối</span>` : '',
      !p.host && p.ready ? `<span class="tag ready">${I.svg('check')}Sẵn sàng</span>` : '',
    ].join('');
    const kick = isHost() && !mine ? `<button class="link kick" data-kick="${escapeHtml(p.id)}">${I.svg('x')}Mời ra</button>` : '';
    return `<div class="lcard${mine ? ' mine' : ''}" data-pid="${escapeHtml(p.id)}" style="--seat:${SEAT_TINT[i]}">
        <div class="lbody">${C.sprite(lookOf(p), { dir: 'down', size: 110 })}</div>
        <div class="lname">${escapeHtml(p.name)}${lvl}</div>
        <div class="lpower" title="Lực chiến">${I.svg('flame')}${xu(p.power || 1000)}</div>
        <div class="lgear">${gearBadges(p)}</div>
        <div class="ptags">${tags}</div>
        ${kick}
      </div>`;
  }

  // ---------- Chiến trường ----------

  function renderBattle() {
    const s = state;
    const mine = me();
    const playing = s.phase === 'playing';
    $('hud').classList.toggle('hidden', !mine || !playing || mine.alive === false);
    renderUnits();
    renderHud();
    renderWind();
    renderTurnPill();
    const mid = (s.map && s.map.id) || 'meadow';
    $('viewport').style.setProperty('--bg', `url('${IMG}bg_${mid}.png')`);
    fit();
    startLoop();
  }

  // Một đơn vị: xe pháo + nòng xoay (đổi dáng theo dạng súng), nhân vật đứng sau mặc áo giáp, nón trên đầu,
  // hào quang khi trang bị đã biến đổi (+7 / +10), tên + thanh máu.
  function unitHtml(p) {
    const i = seatIndex(p);
    const gr = p.gear || {};
    const hat = gr.hat ? `<img class="u-hat f${form(gr.hat)}" src="${IMG}hat_${form(gr.hat)}.png" alt="" draggable="false">` : '';
    const armor = gr.armor ? `<img class="u-armor f${form(gr.armor)}" src="${IMG}armor_${form(gr.armor)}.png" alt="" draggable="false">` : '';
    const top = Math.max(form(gr.gun || 0), form(gr.hat || 0), form(gr.armor || 0));
    const aura = top >= 2 ? `<img class="u-aura f${top}" src="${IMG}aura_${top === 3 ? 10 : 7}.png" alt="" draggable="false">` : '';
    const gf = form(gr.gun || 0);
    const arm = armOf(gr);
    // Vũ khí khác Đại bác: nòng là chính món đồ (arm_<id>.png). Đại bác dạng 2 / 3 dùng cannon_barrel_<dạng>.png
    // nếu có, không thì nòng thường (ánh sáng theo class).
    const barrel = arm !== 'cannon'
      ? `<img class="u-barrel arm arm-${arm}${gf > 1 ? ` f${gf}` : ''}" src="${IMG}arm_${arm}.png" onerror="this.onerror=null;this.src='${IMG}cannon_barrel.png'" alt="" draggable="false">`
      : gf > 1
        ? `<img class="u-barrel f${gf}" src="${IMG}cannon_barrel_${gf}.png" onerror="this.onerror=null;this.src='${IMG}cannon_barrel.png'" alt="" draggable="false">`
        : `<img class="u-barrel" src="${IMG}cannon_barrel.png" alt="" draggable="false">`;
    const gunIcon = !gr.gun ? '' : arm !== 'cannon'
      ? armIcon(arm, gr.gun, `u-gun f${gf}`)
      : `<img class="u-gun f${gf}" src="${IMG}gun_${gf}.png" alt="" draggable="false">`;
    return `<div class="unit" data-pid="${escapeHtml(p.id)}" style="--seat:${SEAT_TINT[i]}">
        ${aura}
        <div class="u-flip">
          <span class="u-char">${C.sprite(lookOf(p), { dir: 'right', size: 74 })}${armor}${hat}</span>
          ${barrel}
          <img class="u-base" src="${IMG}cannon_base.png" alt="" draggable="false">
          ${gunIcon}
        </div>
        <div class="u-tag"><span class="u-name">${escapeHtml(p.name)}</span><span class="u-hp"><i></i></span></div>
        <span class="u-arrow"></span>
      </div>`;
  }

  function bossHtml(b) {
    const k = escapeHtml(bossKind(b));
    return `<div class="unit boss-unit" data-boss="1" style="--bs:${Number(b.size) || 2}">
        <img class="boss-img" src="${IMG}boss_${k}.png" alt="" draggable="false">
        <img class="boss-img hurt" src="${IMG}boss_${k}_hurt.png" onerror="this.remove()" alt="" draggable="false">
        <div class="u-tag"><span class="u-name">${escapeHtml(b.name || 'Boss')}</span></div>
      </div>`;
  }

  function placeBoss(b) {
    const el = document.querySelector('#units .boss-unit');
    if (!el) return;
    const x = b.x || 0;
    const y = b.y != null ? b.y : ground ? ground[clamp(Math.round(x), 0, W - 1)] : 700;
    el.style.transform = `translate(${x}px, ${y}px)`;
    el.classList.toggle('dead', b.alive === false || b.hp <= 0);
    el.classList.toggle('turn', state.phase === 'playing' && isBossId(state.turn));
    // Ảnh boss quay mặt sang trái; người chơi gần nhất ở bên phải thì lật lại.
    const near = state.players.filter((p) => p.alive !== false).sort((p, q) => Math.abs(p.x - x) - Math.abs(q.x - x))[0];
    el.classList.toggle('face-right', !!near && near.x > x);
  }

  // Thanh máu boss to trên đầu màn hình.
  function renderBossBar() {
    const el = $('boss-bar');
    const b = bossOf(state);
    el.classList.toggle('hidden', !b || state.phase === 'lobby');
    if (!b) return;
    const pct = b.maxHp ? clamp(b.hp / b.maxHp, 0, 1) : 0;
    el.innerHTML = `<img class="bb-face" src="${IMG}boss_${escapeHtml(bossKind(b))}.png" alt="" draggable="false">
      <div class="bb-main"><div class="bb-name">${escapeHtml(b.name || 'Boss')}<span>${xu(Math.max(0, Math.round(b.hp)))}/${xu(b.maxHp || 0)}</span></div>
      <div class="bb-track"><i style="width:${pct * 100}%"></i></div></div>`;
  }

  function bossHurt() {
    const el = document.querySelector('#units .boss-unit');
    if (!el) return;
    el.classList.remove('hit'); void el.offsetWidth; el.classList.add('hit');
  }

  function renderUnits() {
    const box = $('units');
    const s = state;
    const b = bossOf(s);
    const want = s.players.map((p) => [p.id, JSON.stringify(p.gear || {}), p.outfit, p.hair, p.gender, p.bot].join(',')).join('|') + (b ? `|boss:${bossKind(b)}` : '');
    if (box.dataset.sig !== want) {
      box.innerHTML = s.players.map(unitHtml).join('') + (b ? bossHtml(b) : '');
      box.dataset.sig = want;
    }
    for (const p of s.players) placeUnit(p);
    if (b) placeBoss(b);
    renderBossBar();
  }

  function angleOf(p) {
    if (p.id === state.you && myTurn()) return aimOf();
    const peer = peers[p.id];
    if (peer && state.turn === p.id) return peer.angle;
    return p.lastAngle || (p.facing < 0 ? 135 : 45);
  }

  function placeUnit(p) {
    const el = document.querySelector(`#units .unit[data-pid="${CSS.escape(p.id)}"]`);
    if (!el) return;
    const x = p.x;
    const y = p.y != null ? p.y : ground ? ground[clamp(Math.round(x), 0, W - 1)] : 700;
    el.style.transform = `translate(${x}px, ${y}px)`;
    const a = angleOf(p);
    const face = a > 90 ? -1 : 1;
    el.classList.toggle('left', face < 0);
    el.classList.toggle('dead', p.alive === false);
    el.classList.toggle('turn', state.phase === 'playing' && state.turn === p.id);
    el.classList.toggle('mine', p.id === state.you);
    // Nòng: xoay theo độ nâng (trong khung đã lật theo hướng mặt).
    const elev = face > 0 ? a : 180 - a;
    el.querySelector('.u-barrel').style.transform = `rotate(${-elev}deg)`;
    const pct = p.maxHp ? clamp(p.hp / p.maxHp, 0, 1) : 0;
    const bar = el.querySelector('.u-hp i');
    bar.style.width = `${pct * 100}%`;
    bar.classList.toggle('low', pct < 0.3);
    el.querySelector('.u-hp').title = `${Math.max(0, Math.round(p.hp))}/${p.maxHp}`;
  }

  function renderWind() {
    const w = state.wind || 0;
    $('wind-num').textContent = Math.abs(w);
    const arrow = $('wind-arrow');
    arrow.style.transform = `scaleX(${w < 0 ? -1 : 1})`;
    arrow.style.opacity = w ? 1 : 0.25;
    arrow.style.width = `${28 + Math.abs(w) * 4}px`;
    $('wind').classList.toggle('strong', Math.abs(w) >= 7);
  }

  function renderTurnPill() {
    const s = state;
    const el = $('turn-pill');
    if (shot) {
      const by = s.players.find((q) => q.id === shot.msg.by);
      const w = WEAPONS.find((x) => x.id === shot.msg.weapon);
      el.className = 'turn-pill firing';
      if (isBossId(shot.msg.by)) { el.innerHTML = `<span>${escapeHtml((bossOf(s) || {}).name || 'Boss')} ra đòn!</span>`; return; }
      // Ném bằng đồ (dép, ti vi…) với đạn thường thì gọi tên món đồ.
      const arm = shot.msg.arm && shot.msg.arm !== 'cannon' && shot.msg.weapon !== 'tele' ? shot.msg.arm : '';
      const what = arm && (!w || w.id === 'shell') ? `ném ${(ARM_NAMES[arm] || arm).toLowerCase()}` : `bắn ${w ? w.name.toLowerCase() : ''}`;
      el.innerHTML = by ? `${C.head(lookOf(by), { size: 26 })}<span>${by.id === s.you ? 'Bạn' : escapeHtml(by.name)} ${escapeHtml(what)}!</span>` : '';
      return;
    }
    if (s.phase !== 'playing') { el.innerHTML = s.phase === 'over' ? escapeHtml(resultLine()) : ''; el.className = 'turn-pill'; return; }
    const p = s.players.find((q) => q.id === s.turn);
    const mine = p && p.id === s.you;
    el.className = 'turn-pill' + (mine ? ' mine' : '');
    if (!p && isBossId(s.turn)) { el.innerHTML = `<span>Lượt của ${escapeHtml((bossOf(s) || {}).name || 'Boss')}</span><b class="clock" id="clock"></b>`; tickClock(); return; }
    el.innerHTML = p ? `${C.head(lookOf(p), { size: 26 })}<span>${mine ? 'Lượt của bạn' : escapeHtml(p.name)}</span><b class="clock" id="clock"></b>` : '';
    tickClock();
  }

  function tickClock() {
    const el = $('clock');
    if (!el || !deadline) { if (el) el.textContent = ''; return; }
    const left = Math.max(0, deadline - Date.now());
    el.textContent = Math.ceil(left / 1000);
    el.classList.toggle('low', left <= 8000);
  }
  setInterval(tickClock, 250);

  function renderHud() {
    const s = state;
    const mine = me();
    if (!mine) return;
    const can = myTurn();
    $('hud').classList.toggle('waiting', !can);
    // Đạn đặc biệt không giới hạn số lần mỗi trận.
    $('weapons').innerHTML = WEAPONS.map((w) => `<button class="weapon${aim.weapon === w.id ? ' on' : ''}" data-weapon="${w.id}" ${!can ? 'disabled' : ''} title="${w.name} — ${w.note}">
          <img src="${IMG}icon_${w.id}.png" alt="" draggable="false"><span class="wn">${w.name}</span></button>`).join('');
    ['btn-left', 'btn-right', 'btn-up', 'btn-down', 'btn-fire'].forEach((id) => { $(id).disabled = !can; });
    const left = s.moveLeft != null ? s.moveLeft : MOVE_BUDGET;
    $('move-fill').style.width = `${clamp(left / MOVE_BUDGET, 0, 1) * 100}%`;
    $('angle-last').textContent = mine.lastAngle ? `trước ${elevText(mine.lastAngle)}°` : '';
    const lp = mine.lastPower || 0;
    $('power-last').style.left = `${lp}%`;
    $('power-last').classList.toggle('hidden', !lp);
    updateAimHud();
    if (shot) note('');
    else if (s.phase === 'playing') {
      const p = s.players.find((q) => q.id === s.turn);
      note(can ? 'Giữ Bắn (hoặc Space) để tích lực, thả để bắn · ←→ đi · ↑↓ chỉnh góc' : p ? `${p.name} đang ngắm…` : '');
    }
  }
  const elevText = (a) => (a > 90 ? 180 - a : a);

  function updateAimHud() {
    $('angle-num').textContent = Math.round(aim.elev);
    $('power-num').textContent = Math.round(aim.power);
    $('power-fill').style.width = `${aim.power}%`;
  }

  // ---------- Điều khiển ----------

  let aimSent = 0;
  let aimTimer = 0;
  function aimChanged() {
    updateAimHud();
    const mine = me();
    if (mine) placeUnit(mine);
    // Phát lại cho người khác thấy, tối đa 10 lần / giây.
    const now = Date.now();
    clearTimeout(aimTimer);
    const go = () => { aimSent = Date.now(); send({ t: 'aim', angle: Math.round(aimOf()), power: Math.round(aim.power) }); };
    if (now - aimSent > 100) go();
    else aimTimer = setTimeout(go, 100);
  }

  function nudgeAngle(d) {
    if (!myTurn()) return;
    aim.elev = clamp(aim.elev + d, 0, 90);
    aimChanged();
  }

  // Đi: mỗi bước 16 đơn vị; ấn ngược hướng đang nhìn thì quay mặt (giữ độ nâng).
  function step(dir) {
    if (!myTurn()) return;
    if (aim.facing !== dir) { aim.facing = dir; aimChanged(); }
    const mine = me();
    const left = state.moveLeft != null ? state.moveLeft : MOVE_BUDGET;
    if (left <= 0) return;
    send({ t: 'move', x: Math.round(clamp(mine.x + dir * 16, 20, W - 20) * 10) / 10 });
  }

  function fire() {
    if (!myTurn()) return;
    clearTimeout(aimTimer); // tin ngắm chờ gửi mà tới sau phát bắn thì server báo "chưa tới lượt"
    aimSent = Date.now();
    send({ t: 'fire', angle: Math.round(aimOf()), power: Math.round(aim.power), weapon: aim.weapon });
    charge = null;
    $('btn-fire').classList.remove('charging');
  }

  // Giữ nút bắn / Space: lực chạy 0 → 100 (chạm 100 thì lùi lại), thả là bắn. Nhấn nhanh = bắn theo lực đang đặt.
  let charge = null;
  function chargeStart() {
    if (!myTurn() || charge) return;
    charge = { t0: performance.now(), from: aim.power, moved: false };
    $('btn-fire').classList.add('charging');
  }
  function chargeTick(now) {
    if (!charge) return;
    const dt = now - charge.t0;
    if (dt < 180) return;
    charge.moved = true;
    const ph = ((dt - 180) / CHARGE_MS) % 2;
    aim.power = Math.round((ph <= 1 ? ph : 2 - ph) * 100);
    updateAimHud();
  }
  function chargeEnd() {
    if (!charge) return;
    if (charge.moved) updateAimHud();
    fire();
  }

  // Kéo thanh lực để đặt lực.
  function powerFromPointer(e) {
    const r = $('power-bar').getBoundingClientRect();
    aim.power = Math.round(clamp((e.clientX - r.left) / r.width, 0, 1) * 100);
    aimChanged();
  }
  $('power-bar').addEventListener('pointerdown', (e) => {
    if (!myTurn()) return;
    $('power-bar').setPointerCapture(e.pointerId);
    powerFromPointer(e);
    const move = (ev) => powerFromPointer(ev);
    const up = () => { $('power-bar').removeEventListener('pointermove', move); $('power-bar').removeEventListener('pointerup', up); };
    $('power-bar').addEventListener('pointermove', move);
    $('power-bar').addEventListener('pointerup', up);
  });

  // Nút giữ được: lặp lại khi giữ.
  function holdable(id, fn, every = 90) {
    const el = $(id);
    let t = 0;
    const stop = () => { clearInterval(t); t = 0; };
    el.addEventListener('pointerdown', (e) => { e.preventDefault(); fn(); stop(); t = setInterval(fn, every); });
    ['pointerup', 'pointerleave', 'pointercancel'].forEach((ev) => el.addEventListener(ev, stop));
    el.addEventListener('contextmenu', (e) => e.preventDefault());
  }
  holdable('btn-left', () => step(-1), 140);
  holdable('btn-right', () => step(1), 140);
  holdable('btn-up', () => nudgeAngle(1), 70);
  holdable('btn-down', () => nudgeAngle(-1), 70);
  const fb = $('btn-fire');
  fb.addEventListener('pointerdown', (e) => { e.preventDefault(); chargeStart(); });
  fb.addEventListener('pointerup', chargeEnd);
  fb.addEventListener('pointercancel', () => { charge = null; fb.classList.remove('charging'); });
  fb.addEventListener('contextmenu', (e) => e.preventDefault());
  $('weapons').addEventListener('click', (e) => {
    const b = e.target.closest('[data-weapon]');
    if (!b || b.disabled) return;
    aim.weapon = b.dataset.weapon;
    renderHud();
  });

  // ---------- Diễn phát bắn ----------

  function startShot(m) {
    // Đang diễn phát trước (hiếm): áp nốt rồi diễn phát mới.
    if (shot) finishShot();
    const paths = (m.paths || []).filter((p) => p && p.length);
    const len = Math.max(1, ...paths.map((p) => p.length));
    const blasts = (m.blasts || []).map((b, i) => ({ ...b, at: b.at != null ? b.at : len - 1, i }));
    const end = Math.max(len, ...blasts.map((b) => b.at + 1));
    shot = { msg: m, paths, blasts, t0: performance.now(), dur: end * STEP_MS + 900, fired: new Set(), hitsShown: false };
    // Người bắn: xoay nòng theo phát bắn.
    if (m.by) peers[m.by] = { angle: m.angle, power: m.power, at: Date.now() };
    cam.follow = true;
    panned = false;
    charge = null;
    $('btn-fire').classList.remove('charging');
    // Khoá điều khiển ngay khi đạn bay (state mới còn đang chờ diễn xong).
    if (state && state.phase !== 'lobby') { renderHud(); renderTurnPill(); }
    startLoop();
  }

  function finishShot() {
    const s = shot;
    shot = null;
    if (s && !s.hitsShown) showHits(s.msg);
    const list = queued;
    queued = [];
    for (const m of list) onState(m);
    if (!list.length && state) render();
  }

  function showHits(m) {
    for (const h of m.hits || []) {
      if (state && isBossId(h.id)) {
        const b = bossOf(state);
        floater(b.x, (b.y || 700) - 220, `−${Math.round(h.dmg)}`, h.crit ? 'dmg crit' : 'dmg');
        if (h.crit) floater(b.x, (b.y || 700) - 270, 'Chí mạng!', 'crit-label');
        if (h.hp != null) b.hp = h.hp;
        bossHurt();
        placeBoss(b);
        renderBossBar();
        continue;
      }
      const p = state && state.players.find((q) => q.id === h.id);
      if (!p) continue;
      floater(p.x, (p.y || 700) - 120, `−${Math.round(h.dmg)}`, h.crit ? 'dmg crit' : 'dmg');
      if (h.crit) floater(p.x, (p.y || 700) - 170, 'Chí mạng!', 'crit-label');
      // Cập nhật thanh máu ngay khi nổ, không chờ state.
      p.hp = h.hp;
      placeUnit(p);
      const el = document.querySelector(`#units .unit[data-pid="${CSS.escape(p.id)}"]`);
      if (el) { el.classList.remove('hit'); void el.offsetWidth; el.classList.add('hit'); }
    }
    for (const id of m.deaths || []) {
      if (state && isBossId(id)) { const b = bossOf(state); b.alive = false; floater(b.x, (b.y || 700) - 260, 'Hạ boss!', 'ko'); placeBoss(b); continue; }
      const p = state && state.players.find((q) => q.id === id);
      if (p) floater(p.x, (p.y || 700) - 150, 'Hạ gục!', 'ko');
    }
    if (m.tele && state) {
      const p = state.players.find((q) => q.id === m.by);
      if (p) { p.x = m.tele.x; p.y = m.tele.y; placeUnit(p); floater(p.x, p.y - 120, 'Dịch chuyển!', 'tele'); }
    }
  }

  function floater(x, y, text, cls) {
    const el = document.createElement('span');
    el.className = 'floater ' + cls;
    el.textContent = text;
    el.style.left = `${x}px`;
    el.style.top = `${y}px`;
    $('floaters').appendChild(el);
    setTimeout(() => el.remove(), 1600);
  }

  // Khoét hố tròn trên ground đang vẽ (server sẽ gửi ground chuẩn trong state sau).
  function carve(cx, cy, r) {
    if (!ground || !r) return;
    for (let x = Math.max(0, Math.floor(cx - r)); x <= Math.min(W - 1, Math.ceil(cx + r)); x++) {
      const dy = Math.sqrt(Math.max(0, r * r - (x - cx) * (x - cx)));
      const bottom = Math.min(H, Math.round(cy + dy));
      if (ground[x] < bottom) ground[x] = bottom;
    }
    terrainDirty = true;
  }

  // ---------- Vẽ canvas ----------

  const cv = $('cv');
  const g = cv.getContext('2d');
  const terrain = document.createElement('canvas');
  terrain.width = W;
  terrain.height = H;
  const tg = terrain.getContext('2d');
  let loopOn = false;

  function startLoop() {
    if (loopOn) return;
    loopOn = true;
    requestAnimationFrame(frame);
  }

  function buildTerrain() {
    terrainDirty = false;
    tg.clearRect(0, 0, W, H);
    if (!ground) return;
    const id = (state && state.map && state.map.id) || 'meadow';
    const tex = img('ground_' + id);
    const grass = img('grass_' + id);
    tg.save();
    tg.beginPath();
    tg.moveTo(0, H);
    for (let x = 0; x < W; x++) tg.lineTo(x, ground[x]);
    tg.lineTo(W, ground[W - 1]);
    tg.lineTo(W, H);
    tg.closePath();
    tg.fillStyle = ready(tex) ? tg.createPattern(tex, 'repeat') : '#8a5a2b';
    tg.fill();
    tg.restore();
    // Dải viền mặt đất: cắt ảnh grass thành cột 4px, đặt theo độ cao từng cột.
    if (ready(grass)) {
      const gw = grass.naturalWidth;
      const gh = grass.naturalHeight;
      for (let x = 0; x < W; x += 4) {
        const y = ground[x];
        if (y >= H - 2) continue;
        tg.drawImage(grass, x % gw, 0, Math.min(4, gw - (x % gw)), gh, x, y - Math.round(gh * 0.35), 4, gh);
      }
    }
  }

  function frame(now) {
    if (!state || state.phase === 'lobby' || $('battle').classList.contains('hidden')) { loopOn = false; return; }
    requestAnimationFrame(frame);
    chargeTick(now);
    if (terrainDirty) buildTerrain();
    const id = (state.map && state.map.id) || 'meadow';
    const bg = img('bg_' + id);
    g.save();
    if (now < shakes.until) g.translate((Math.random() - 0.5) * 10, (Math.random() - 0.5) * 8);
    if (ready(bg)) g.drawImage(bg, 0, 0, W, H);
    else { g.fillStyle = '#9fd6f5'; g.fillRect(0, 0, W, H); }
    g.drawImage(terrain, 0, 0);
    drawAimGuide();
    let focus = null;
    if (shot) focus = drawShot(now);
    drawBooms(now);
    g.restore();
    // Camera điện thoại: theo đạn, không thì theo người tới lượt.
    if (!focus && state.phase === 'playing') {
      const p = state.players.find((q) => q.id === state.turn);
      if (p) focus = { x: p.x };
    }
    updateCamera(focus);
    // Nòng của người khác chạy theo tin ngắm.
    if (state.phase === 'playing' && state.turn !== state.you) {
      const p = state.players.find((q) => q.id === state.turn);
      if (p && peers[p.id]) placeUnit(p);
    }
  }

  // Lượt mình: vạch ngắm ngắn theo góc (dài theo lực) + vạch mờ của phát trước.
  function drawAimGuide() {
    if (!myTurn()) return;
    const p = me();
    const y = p.y != null ? p.y : ground[clamp(Math.round(p.x), 0, W - 1)];
    const ox = p.x;
    const oy = y - 34;
    const line = (deg, len, style, width, dash) => {
      const r = (deg * Math.PI) / 180;
      g.save();
      g.strokeStyle = style;
      g.lineWidth = width;
      g.setLineDash(dash);
      g.lineCap = 'round';
      g.beginPath();
      g.moveTo(ox + Math.cos(r) * 46, oy - Math.sin(r) * 46);
      g.lineTo(ox + Math.cos(r) * (46 + len), oy - Math.sin(r) * (46 + len));
      g.stroke();
      g.restore();
    };
    if (p.lastAngle) line(p.lastAngle, 60, 'rgba(255,255,255,.45)', 3, [4, 6]);
    line(aimOf(), 40 + aim.power * 0.9, 'rgba(255,214,80,.95)', 4, [10, 7]);
  }

  function drawShot(now) {
    const s = shot;
    const t = Math.max(0, (now - s.t0) / STEP_MS); // mốc rAF có thể sớm hơn lúc nhận tin bắn
    let focus = null;
    for (const b of s.blasts) {
      if (!s.fired.has(b.i) && t >= b.at) {
        s.fired.add(b.i);
        carve(b.x, b.y, b.r);
        booms.push({ x: b.x, y: b.y, r: Math.max(26, b.r || 0), t0: now });
        shakes.until = now + (b.r >= 60 ? 380 : 220);
        if (!s.hitsShown) { s.hitsShown = true; showHits(s.msg); }
      }
    }
    const kind = s.msg.weapon || 'shell';
    // Bắn bằng dép / chổi / ti vi… thì vật bay là món đồ đó, xoay tròn; ảnh chưa tải thì dùng đạn thường.
    const arm = s.msg.arm && s.msg.arm !== 'cannon' && kind !== 'tele' ? s.msg.arm : '';
    const byBoss = state && isBossId(s.msg.by);
    const thrown = byBoss ? img('throw_boss_' + bossKind(bossOf(state))) : arm ? img('throw_' + arm) : null;
    // Đạn boss (weapon "boss_*") chưa tải ảnh thì tạm dùng đạn thường, không gọi ảnh shot_ không tồn tại.
    const sprite = thrown && ready(thrown) ? thrown : img('shot_' + (['shell', 'triple', 'heavy', 'tele'].includes(kind) ? kind : 'shell'));
    const spin = thrown && ready(thrown) ? ((now - s.t0) / 1000) * 11 * (s.msg.angle > 90 ? -1 : 1) : null;
    const smoke = img('smoke');
    s.paths.forEach((path, pi) => {
      const i = Math.floor(t);
      const blast = s.blasts.find((b) => (b.path != null ? b.path : b.i) === pi) || null;
      const stop = blast ? Math.min(path.length - 1, blast.at) : path.length - 1;
      if (i > stop) return;
      const a = path[Math.min(i, path.length - 1)];
      const b = path[Math.min(i + 1, path.length - 1)];
      const f = t - i;
      const x = a[0] + (b[0] - a[0]) * f;
      const y = a[1] + (b[1] - a[1]) * f;
      // Vệt khói phía sau.
      if (ready(smoke)) {
        for (let k = 1; k <= 6; k++) {
          const q = path[clamp(i - k * 2, 0, path.length - 1)];
          g.globalAlpha = 0.32 - k * 0.045;
          const sz = 14 + k * 4;
          g.drawImage(smoke, q[0] - sz / 2, q[1] - sz / 2, sz, sz);
        }
        g.globalAlpha = 1;
      }
      const ang = Math.atan2(b[1] - a[1], b[0] - a[0]);
      const size = spin != null ? (kind === 'heavy' ? 48 : kind === 'triple' ? 28 : 36) : kind === 'heavy' ? 40 : kind === 'triple' ? 24 : 30;
      g.save();
      g.translate(x, y);
      g.rotate(spin != null ? spin + pi : ang);
      if (ready(sprite)) g.drawImage(sprite, -size / 2, -size / 2, size, size);
      else { g.fillStyle = '#333'; g.fillRect(-5, -5, 10, 10); }
      g.restore();
      if (pi === 0 || !focus) focus = { x, y };
    });
    if (now - s.t0 > s.dur) finishShot();
    return focus;
  }

  function drawBooms(now) {
    for (let i = booms.length - 1; i >= 0; i--) {
      const b = booms[i];
      const k = Math.max(0, Math.floor((now - b.t0) / 80));
      if (k >= 6) { booms.splice(i, 1); continue; }
      const im = img('boom_' + (k + 1));
      const sz = b.r * 3.2;
      if (ready(im)) g.drawImage(im, b.x - sz / 2, b.y - sz * 0.6, sz, sz);
    }
  }

  // ---------- Co giãn / camera ----------

  let scale = 1;
  let panned = false; // điện thoại: người dùng kéo xem chỗ khác
  function fit() {
    const vp = $('viewport');
    const w = vp.clientWidth;
    const h = vp.clientHeight;
    if (!w || !h) return;
    const fitScale = Math.min(w / W, h / H);
    // Màn hẹp (điện thoại dọc): phóng to cho thấy rõ, cho cuộn ngang theo camera.
    scale = w >= 700 ? fitScale : Math.max(fitScale, Math.min(h / H, 0.6));
    vp.classList.toggle('zoomed', scale * W > w + 2);
    updateCamera(null, true);
  }
  function updateCamera(focus, force) {
    const vp = $('viewport');
    const w = vp.clientWidth;
    const h = vp.clientHeight;
    const sw = W * scale;
    const sh = H * scale;
    let ox = (w - sw) / 2;
    if (sw > w) {
      if (focus && !panned) {
        const target = clamp(w / 2 - focus.x * scale, w - sw, 0);
        cam.x += (target - cam.x) * (force ? 1 : 0.12);
      }
      cam.x = clamp(cam.x, w - sw, 0);
      ox = cam.x;
    }
    const oy = sw > w ? h - sh : Math.max(0, (h - sh) / 2); // phóng to: cắt bớt trời phía trên, giữ mặt đất
    $('stage').style.transform = `translate(${ox}px, ${oy}px) scale(${scale})`;
  }
  window.addEventListener('resize', () => { if (state && state.phase !== 'lobby') fit(); });

  // Kéo ngang chiến trường để xem (điện thoại). Lượt mới / phát bắn mới thì camera lại tự theo.
  (function () {
    const vp = $('viewport');
    let drag = null;
    vp.addEventListener('pointerdown', (e) => {
      if (!vp.classList.contains('zoomed')) return;
      drag = { x: e.clientX, cx: cam.x };
    });
    vp.addEventListener('pointermove', (e) => {
      if (!drag) return;
      const dx = e.clientX - drag.x;
      if (Math.abs(dx) > 6) panned = true;
      if (panned) { cam.x = drag.cx + dx; updateCamera(null); }
    });
    const end = () => { drag = null; };
    vp.addEventListener('pointerup', end);
    vp.addEventListener('pointercancel', end);
  })();
  function pingTurn() { panned = false; }

  // ---------- Kết quả ----------

  // Trận boss: result.victory = phe người chơi thắng (winner luôn là id boss, không dùng để xét).
  function bossWon() {
    const r = (state && state.result) || {};
    return !r.victory;
  }

  // Rương rơi ngọc sau trận boss.
  function showLoot(m) {
    const mine = state && state.you;
    const drops = (m.drops && mine && m.drops[mine]) || [];
    const dlg = $('dlg-loot');
    const box = $('loot-box');
    box.classList.remove('open');
    $('loot-items').innerHTML = drops.length ? drops.map((g, i) => `<span class="loot-gem" style="--i:${i}">${gemImg(g)}<b>${escapeHtml(g.name || 'Ngọc')}</b>${g.value ? `<small>${escapeHtml(gemText(g))}</small>` : ''}</span>`).join('')
      : '<p class="muted">Lần này rương trống — thử lại nhé!</p>';
    $('loot-title').textContent = drops.length ? `Nhận ${drops.length} viên ngọc!` : 'Rương boss';
    if (!dlg.open) dlg.showModal();
    setTimeout(() => box.classList.add('open'), 450);
    gear = null; // kho ngọc đã đổi, mở Trang bị sẽ tải lại
  }

  function resultLine() {
    const r = state.result;
    if (!r) return 'Hết trận';
    if (isBoss(state)) {
      const name = (bossOf(state) || {}).name || 'Boss';
      return bossWon() ? `${name} thắng — ${BOSS_REASONS[r.reason] || 'cả đội thua'}` : `Đã hạ ${name}!`;
    }
    if (!r.winner) return 'Hoà — ' + (REASONS[r.reason] || REASONS.draw);
    const w = state.players.find((p) => p.id === r.winner);
    const who = w ? (w.id === state.you ? 'Bạn' : w.name) : '?';
    return `${who} thắng — ${REASONS[r.reason] || ''}`;
  }

  function renderResult() {
    const s = state;
    const el = $('result');
    const show = s.phase === 'over' && !hideResult && !shot;
    $('over-controls').classList.toggle('hidden', s.phase !== 'over' || show || !me() || !!resultTimer);
    $('btn-rematch-bar').disabled = !me() || !!me().ready;
    el.classList.toggle('hidden', !show);
    if (!show) return;
    const r = s.result || {};
    const mine = me();
    let title;
    let tone;
    const w = s.players.find((p) => p.id === r.winner);
    if (isBoss(s)) { title = bossWon() ? 'Boss thắng' : 'Hạ boss!'; tone = bossWon() ? 'lose' : 'win'; }
    else if (!r.winner) { title = 'Hoà!'; tone = 'draw'; }
    else if (mine && mine.id === r.winner) { title = 'Bạn thắng!'; tone = 'win'; }
    else if (mine) { title = 'Bạn thua'; tone = 'lose'; }
    else { title = `${w ? w.name : '?'} thắng`; tone = 'win'; }
    el.dataset.tone = tone;
    $('result-title').innerHTML = `${w ? C.head(lookOf(w), { size: 52 }) : ''}${escapeHtml(title)}`;
    $('result-sub').textContent = resultLine();
    const money = mine && r.money ? r.money[mine.id] || 0 : 0;
    const xp = mine && r.xp ? r.xp[mine.id] || 0 : 0;
    const coins = s.gameBet || (isBoss(s) && money) ? `<span class="gain ${money > 0 ? 'plus' : money < 0 ? 'minus' : ''}">${I.svg('coins')}${signed(money)} xu</span>` : '';
    $('result-gain').innerHTML = mine ? `${coins}${xp ? `<span class="gain xp">+${xp} XP</span>` : ''}` : '';
    const rd = mine && mine.ready;
    $('btn-rematch').disabled = !mine || !!rd;
    $('btn-rematch').innerHTML = rd ? `${I.svg('timer')}Chờ người khác…` : `${I.svg('play')}Trận mới <kbd>N</kbd>`;
  }

  // ---------- Trang bị: mua đá cường hoá, cường hoá theo tỉ lệ ----------

  const STONE_NAMES = { 1: 'Đá Lục', 2: 'Đá Lam', 3: 'Đá Tím', 4: 'Đá Vàng', 5: 'Đá Đỏ' };
  let gear = null;
  const forge = { slot: 'gun', stone: 1, busy: false, luck: false, protect: false };
  const CHARMS = [
    { id: 'luck', name: 'Bùa may mắn', price: 1500, desc: '+15% tỉ lệ' },
    { id: 'protect', name: 'Bùa giữ cấp', price: 2000, desc: 'Thất bại không tụt cấp' },
  ];
  const charmsOf = (id) => ((gear && gear.charms) || {})[id] || 0;
  const charmImg = (id, cls = '') => `<img class="charm ${cls}" src="${IMG}charm_${id}.png" alt="" draggable="false">`;
  const buyQty = {}; // tier → số lượng đang chọn
  const stoneImg = (t, cls = '') => `<img class="stone ${cls}" src="${IMG}stone_${t}.png" alt="" draggable="false">`;
  const stoneName = (t) => {
    const s = gear && (gear.stoneShop || []).find((x) => x.tier === t);
    return (s && s.name) || STONE_NAMES[t] || `Đá ${t}`;
  };
  const stonesOf = (t) => ((gear && gear.stones) || {})[t] || 0;

  async function openGear() {
    $('dlg-gear').showModal();
    try { gear = await A.api('/api/arena/gear'); } catch (e) { $('gear-list').innerHTML = `<p class="muted">${escapeHtml(e.message)}</p>`; return; }
    // Mặc định chọn loại đá đang có, rẻ nhất.
    if (!stonesOf(forge.stone)) forge.stone = [1, 2, 3, 4, 5].find((t) => stonesOf(t)) || forge.stone;
    renderGear();
  }

  function renderGear() {
    if (!gear) return;
    $('gear-power').textContent = xu(gear.power);
    $('gear-balance').textContent = xu(gear.balance);
    $('hall-power').textContent = xu(gear.power);
    const max = gear.maxLevel || 10;
    $('gear-list').innerHTML = ['gun', 'hat', 'armor'].map((k) => {
      const lv = (gear.gear || {})[k] || 0;
      const pips = Array.from({ length: max }, (_, i) => `<i class="${i < lv ? 'on' : ''}"></i>`).join('');
      const pic = k === 'gun' ? armIcon(armOf(gear.gear), lv, 'gc-img') : `<img class="gc-img" src="${IMG}${k}_${form(lv)}.png" alt="" draggable="false">`;
      const name = k === 'gun' ? armName(armOf(gear.gear)) : GEAR[k].name;
      return `<button type="button" class="gear-card${forge.slot === k ? ' on' : ''}" data-slot="${k}">
          ${pic}
          <span class="gc-info">
            <span class="gc-name">${escapeHtml(name)} <span class="lv-chip">+${lv}</span></span>
            <span class="gc-pips">${pips}</span>
            <span class="gc-stat">${k === 'gun' ? armStatLine(armOf(gear.gear), lv) || GEAR[k].stat(lv) : GEAR[k].stat(lv)}</span>
            <span class="gc-form">${lv >= 10 ? 'Dạng huyền thoại' : lv >= 7 ? 'Đã biến đổi · lần nữa ở +10' : 'Biến đổi ở +7 / +10'}</span>
          </span>
        </button>`;
    }).join('');
    renderForge();
    renderStoneShop();
    renderArms();
  }

  function renderForge() {
    const k = forge.slot;
    const lv = (gear.gear || {})[k] || 0;
    const max = gear.maxLevel || 10;
    const slotName = k === 'gun' ? armName(armOf(gear.gear)) : GEAR[k].name;
    const rates = ((gear.rates || {})[k]) || [];
    const fee = ((gear.fee || {})[k]) || 0;
    if (lv >= max || !rates.length) {
      $('forge').innerHTML = `<div class="forge-max">${I.svg('trophy')}${escapeHtml(slotName)} đã đạt cấp tối đa +${max}</div>`;
      return;
    }
    // Bùa đã tick mà hết lá (vừa dùng xong) thì bỏ tick.
    if (forge.luck && !charmsOf('luck')) forge.luck = false;
    if (forge.protect && !charmsOf('protect')) forge.protect = false;
    const bonus = forge.luck ? gear.luckBonus || 15 : 0;
    const rateOf = (t) => Math.min(100, (rates[t - 1] || 0) + bonus);
    const rate = rateOf(forge.stone);
    const have = stonesOf(forge.stone);
    const stones = [1, 2, 3, 4, 5].map((t) => {
      const n = stonesOf(t);
      return `<button type="button" class="stone-pick${forge.stone === t ? ' on' : ''}${n ? '' : ' none'}" data-stone="${t}" title="${escapeHtml(stoneName(t))}">
          ${stoneImg(t)}<b class="sp-qty">${n}</b><span class="sp-rate">${rateOf(t)}%</span></button>`;
    }).join('');
    const charm = (c) => {
      const n = charmsOf(c.id);
      return `<label class="charm-tick${n ? '' : ' none'}${forge[c.id] ? ' on' : ''}" title="${escapeHtml(c.desc)}">
          <input type="checkbox" data-charm="${c.id}" ${forge[c.id] ? 'checked' : ''} ${n ? '' : 'disabled'}>
          ${charmImg(c.id)}<span><b>${escapeHtml(c.name)}</b><small>${escapeHtml(c.desc)} · còn ${n} lá</small></span></label>`;
    };
    const short = !have ? 'Hết đá loại này — sang tab Cửa hàng' : gear.balance < fee ? 'Không đủ xu trả phí' : '';
    const risk = lv > 0 && !forge.protect
      ? `<div class="forge-risk">${I.svg('flame')}Thất bại sẽ tụt về +${lv - 1}${lv === 7 || lv === 10 ? ' (mất dạng biến đổi)' : ''}</div>`
      : lv > 0 ? `<div class="forge-safe">${I.svg('shield')}Có bùa giữ cấp: thất bại vẫn giữ +${lv}</div>` : '';
    const evolve = lv + 1 === 7 || lv + 1 === 10 ? `<span class="evolve-tag">${I.svg('sparkles')}Biến đổi!</span>` : '';
    $('forge').innerHTML = `
      <div class="forge-head">${escapeHtml(slotName)} <b>+${lv}</b> → <b class="up">+${lv + 1}</b> ${evolve}
        <span class="muted">${k === 'gun' ? armStatLine(armOf(gear.gear), lv + 1) || GEAR[k].stat(lv + 1) : GEAR[k].stat(lv + 1)}</span></div>
      <div class="stone-row">${stones}</div>
      <div class="charm-row">${CHARMS.map(charm).join('')}</div>
      <div class="forge-go">
        <div class="forge-anvil" id="forge-anvil">
          ${k === 'gun' ? armIcon(armOf(gear.gear), lv, 'fa-item').replace('<img ', '<img id="forge-item" ') : `<img class="fa-item" id="forge-item" src="${IMG}${k}_${form(lv)}.png" alt="" draggable="false">`}
          ${stoneImg(forge.stone, 'fa-stone')}
          <img class="fa-fx hidden" id="forge-fx" alt="" draggable="false">
          <span class="fa-label hidden" id="forge-label"></span>
        </div>
        <div class="forge-info">
          <div class="rate-line">Tỉ lệ <b class="rate ${rate >= 80 ? 'hi' : rate >= 40 ? 'mid' : 'lo'}">${rate}%</b>${bonus ? ` <small class="bonus">(+${bonus}% bùa)</small>` : ''}</div>
          <div class="rate-bar"><i style="width:${rate}%"></i></div>
          <div class="fee">${escapeHtml(stoneName(forge.stone))} ×1 · Phí ${I.svg('coins')}${xu(fee)}</div>
          ${risk}
          <button type="button" class="primary forge-btn" id="btn-forge" ${short || forge.busy ? 'disabled' : ''}>${I.svg('sparkles')}Cường hoá</button>
          ${short ? `<div class="forge-short">${short}</div>` : ''}
        </div>
      </div>`;
  }

  function renderStoneShop() {
    const shop = gear.stoneShop && gear.stoneShop.length ? gear.stoneShop
      : [1, 2, 3, 4, 5].map((t) => ({ tier: t, name: STONE_NAMES[t], price: [100, 300, 800, 2000, 5000][t - 1] }));
    $('stone-shop').innerHTML = shop.map((s) => {
      const q = buyQty[s.tier] || 1;
      const cost = s.price * q;
      return `<div class="stone-card" data-tier="${s.tier}">
          ${stoneImg(s.tier, 'sc-img')}
          <div class="sc-info"><b>${escapeHtml(s.name)}</b><span class="muted">Đang có ${stonesOf(s.tier)} · ${I.svg('coins')}${xu(s.price)}/viên</span></div>
          <div class="qty">
            <button type="button" data-q="-1">−</button><input type="number" min="1" max="99" value="${q}" data-qty="${s.tier}" aria-label="Số lượng"><button type="button" data-q="1">+</button>
          </div>
          <button type="button" class="primary" data-buy="${s.tier}" ${gear.balance < cost ? 'disabled title="Không đủ xu"' : ''}>Mua · ${I.svg('coins')}${xu(cost)}</button>
        </div>`;
    }).join('') + `<div class="shop-sep">Bùa cường hoá</div>` + (gear.charmShop && gear.charmShop.length ? gear.charmShop : CHARMS).map((c) => {
      const q = buyQty[c.id] || 1;
      const cost = c.price * q;
      return `<div class="stone-card" data-tier="${c.id}">
          ${charmImg(c.id, 'sc-img')}
          <div class="sc-info"><b>${escapeHtml(c.name)}</b><span class="muted">${escapeHtml(c.desc || '')} · có ${charmsOf(c.id)} · ${I.svg('coins')}${xu(c.price)}/lá</span></div>
          <div class="qty">
            <button type="button" data-q="-1">−</button><input type="number" min="1" max="99" value="${q}" data-qty="${c.id}" aria-label="Số lượng"><button type="button" data-q="1">+</button>
          </div>
          <button type="button" class="primary" data-buy-charm="${c.id}" ${gear.balance < cost ? 'disabled title="Không đủ xu"' : ''}>Mua · ${I.svg('coins')}${xu(cost)}</button>
        </div>`;
    }).join('');
  }

  // ---------- Vũ khí: mua, trang bị, chuyển cấp cường hoá ----------

  const xfer = { from: '', to: '', busy: false };

  function renderArms() {
    const box = $('arms-grid');
    if (!box || !gear) return;
    const arms = gear.arms || [];
    if (!arms.length) { box.innerHTML = '<p class="muted">Chưa có danh sách vũ khí.</p>'; $('arm-xfer').innerHTML = ''; return; }
    // Thanh chỉ số so với món mạnh nhất cùng thuộc tính (tính cả cấp đã cường hoá).
    const top = {};
    for (const st of ARM_STATS) top[st.k] = Math.max(1, ...arms.map((a) => (a.eff || a)[st.k] || 0));
    box.innerHTML = arms.map((a) => {
      const vals = a.owned && a.eff ? a.eff : a;
      const bars = ARM_STATS.map((st) => {
        const v = vals[st.k] || 0;
        return `<span class="as-row as-${st.k}" title="${st.name}${st.k === 'luck' ? ' (tỉ lệ chí mạng)' : ''}">
            <span class="as-name">${st.name}</span><span class="as-bar"><i style="width:${Math.round((v / top[st.k]) * 100)}%"></i></span><b>${statVal(st, v)}</b></span>`;
      }).join('');
      let act;
      if (a.equipped) act = `<button type="button" class="arm-btn" disabled>${I.svg('check')}Đang dùng</button>`;
      else if (a.owned) act = `<button type="button" class="arm-btn primary" data-equip="${escapeHtml(a.id)}">Trang bị</button>`;
      else act = `<button type="button" class="arm-btn primary" data-buy-arm="${escapeHtml(a.id)}" ${gear.balance < a.price ? 'disabled title="Không đủ xu"' : ''}>Mua · ${I.svg('coins')}${xu(a.price)}</button>`;
      return `<div class="arm-card${a.equipped ? ' on' : ''}${a.owned ? '' : ' locked'}">
          <div class="ac-top">${armIcon(a.id, a.level, 'ac-img')}
            <div class="ac-title"><b>${escapeHtml(a.name)}</b>${a.owned ? `<span class="lv-chip">+${a.level || 0}</span>` : `<span class="ac-price">${a.price ? `${I.svg('coins')}${xu(a.price)}` : 'Miễn phí'}</span>`}</div></div>
          ${a.desc ? `<div class="ac-desc">${escapeHtml(a.desc)}</div>` : ''}
          <div class="ac-stats">${bars}</div>
          ${socketsHtml(a)}
          ${act}
        </div>`;
    }).join('');
    renderXfer();
    renderGemPick();
    renderGemBag();
  }

  // ---------- Ngọc khảm: 3 ô trên mỗi vũ khí, mở ở +3 / +7 / +10 ----------

  const gemSel = { arm: '', idx: -1 }; // ô đang chọn để khảm / gỡ
  const statOf = (kind) => ARM_STATS.find((st) => st.k === kind) || { k: kind, name: kind, short: kind };
  const gemImg = (g, cls = '') => `<img class="gem ${cls}" src="${IMG}gem_${g.kind}_${g.tier}.png" alt="" draggable="false">`;
  const gemText = (g) => `+${statVal(statOf(g.kind), g.value)} ${statOf(g.kind).name}`;
  // Lực chiến một viên ngọc góp vào (lực chiến = 1000 + 2 × tổng 4 chỉ số + …); server có thể gửi sẵn `power`.
  const gemPower = (g) => (g ? (g.power != null ? g.power : 2 * (g.value || 0)) : 0);

  function socketsHtml(a) {
    if (!a.owned || !Array.isArray(a.sockets)) return '';
    return `<div class="sockets">${a.sockets.map((sk) => {
      const on = gemSel.arm === a.id && gemSel.idx === sk.idx ? ' on' : '';
      if (!sk.open) {
        return `<span class="socket locked" title="Mở khi cường hoá lên +${sk.unlockAt}"><img src="${IMG}socket_locked.png" onerror="this.style.visibility='hidden'" alt="" draggable="false"><b>+${sk.unlockAt}</b></span>`;
      }
      const tip = sk.gem ? `${sk.gem.name || 'Ngọc'} ${gemText(sk.gem)} — bấm để gỡ` : 'Ô trống — bấm để khảm ngọc';
      return `<button type="button" class="socket${sk.gem ? ' full' : ''}${on}" data-socket="${escapeHtml(a.id)}:${sk.idx}" title="${escapeHtml(tip)}">
          <img src="${IMG}socket_empty.png" onerror="this.style.visibility='hidden'" alt="" draggable="false">${sk.gem ? gemImg(sk.gem, 'sk-gem') : ''}</button>`;
    }).join('')}</div>`;
  }

  // Khung chọn ngọc cho ô đang chọn: khảm (xem trước lực chiến tăng) hoặc gỡ.
  function renderGemPick() {
    const el = $('gem-pick');
    if (!el) return;
    const a = armData(gemSel.arm);
    const sk = a && (a.sockets || []).find((x) => x.idx === gemSel.idx && x.open);
    if (!sk) { gemSel.arm = ''; gemSel.idx = -1; el.innerHTML = ''; el.classList.add('hidden'); return; }
    el.classList.remove('hidden');
    const live = !!a.equipped; // ngọc chỉ tính vào lực chiến khi vũ khí đang dùng
    const old = sk.gem;
    const delta = (g) => gemPower(g) - gemPower(old);
    const deltaTag = (d) => !live ? '<span class="gp-delta muted">khi trang bị</span>'
      : `<span class="gp-delta ${d > 0 ? 'plus' : d < 0 ? 'minus' : ''}">${I.svg('flame')}${d > 0 ? '+' : ''}${xu(d)}</span>`;
    const gems = (gear.gems || []).filter((g) => g.qty > 0);
    const head = `<div class="gp-head">${armIcon(a.id, a.level, 'gp-arm')}<span><b>${escapeHtml(a.name)}</b> · ô ${sk.idx + 1}${old ? ` · đang khảm ${escapeHtml(old.name || 'ngọc')} (${escapeHtml(gemText(old))})` : ' · trống'}</span>
        <button type="button" class="link gp-close" data-gem-close>${I.svg('x')}</button></div>`;
    const unsock = old ? `<button type="button" class="gp-unsock" data-unsocket>${gemImg(old)}<span>Gỡ ngọc · về kho</span>${deltaTag(-gemPower(old))}</button>` : '';
    const list = gems.length ? gems.map((g) => `<button type="button" class="gp-gem" data-gem="${escapeHtml(g.kind)}:${g.tier}">
          ${gemImg(g)}<span class="gp-name"><b>${escapeHtml(g.name || 'Ngọc')}</b><small>${escapeHtml(gemText(g))} · còn ${g.qty}</small></span>${deltaTag(delta(g))}</button>`).join('')
      : '<p class="muted gp-none">Kho ngọc trống — đánh boss để nhận ngọc.</p>';
    el.innerHTML = `${head}${unsock}<div class="gp-list">${list}</div>`;
  }

  function renderGemBag() {
    const el = $('gem-bag');
    if (!el || !gear) return;
    const gems = (gear.gems || []).filter((g) => g.qty > 0);
    el.innerHTML = gems.length ? gems.map((g) => `<div class="stone-card gem-card">
        ${gemImg(g, 'sc-img')}
        <div class="sc-info"><b>${escapeHtml(g.name || 'Ngọc')}</b><span class="muted">${escapeHtml(gemText(g))} · cấp ${g.tier}</span></div>
        <b class="gem-qty">×${g.qty}</b>
      </div>`).join('') + '<p class="hint">Khảm ngọc vào ô trên vũ khí ở tab Vũ khí (ô mở ở +3, +7, +10).</p>'
      : '<p class="muted">Chưa có ngọc nào. Thắng chế độ Đấu boss để nhận ngọc từ rương.</p>';
  }

  // Lực chiến tăng / giảm: số bay lên cạnh ô Lực chiến.
  function powerFx(before) {
    const d = (gear.power || 0) - (before || 0);
    if (!d) return;
    const box = document.querySelector('#dlg-gear .gear-power');
    if (!box) return;
    const el = document.createElement('span');
    el.className = `power-fx ${d > 0 ? 'plus' : 'minus'}`;
    el.textContent = `${d > 0 ? '+' : ''}${xu(d)}`;
    box.appendChild(el);
    box.classList.remove('pulse'); void box.offsetWidth; box.classList.add('pulse');
    setTimeout(() => el.remove(), 1600);
  }

  async function gemAction(path, body, msg) {
    const before = gear.power;
    try {
      gear = await A.api(path, { method: 'POST', body });
      showBalance(gear.balance);
      A.toast(msg);
    } catch (err) {
      A.toast(err.message);
    }
    renderGear();
    powerFx(before);
  }

  function renderXfer() {
    const el = $('arm-xfer');
    const owned = (gear.arms || []).filter((a) => a.owned);
    const froms = owned.filter((a) => (a.level || 0) > 0);
    if (!froms.length || owned.length < 2) {
      el.innerHTML = `<div class="xf-head">${I.svg('sparkles')}Chuyển cấp cường hoá</div>
        <p class="muted xf-empty">${owned.length < 2 ? 'Cần sở hữu ít nhất 2 vũ khí để chuyển cấp.' : 'Chưa có vũ khí nào đã cường hoá để chuyển.'}</p>`;
      return;
    }
    if (!froms.some((a) => a.id === xfer.from)) xfer.from = froms[0].id;
    const src = owned.find((a) => a.id === xfer.from);
    const tos = owned.filter((a) => a.id !== src.id && (a.level || 0) < src.level);
    if (!tos.some((a) => a.id === xfer.to)) xfer.to = tos.length ? tos[0].id : '';
    const dst = tos.find((a) => a.id === xfer.to);
    const opt = (a, sel) => `<option value="${escapeHtml(a.id)}"${a.id === sel ? ' selected' : ''}>${escapeHtml(a.name)} +${a.level || 0}</option>`;
    const n = src.level;
    const fee = (gear.transferFeePerLevel || 300) * n;
    let preview = '<p class="muted xf-empty">Không có vũ khí nào cấp thấp hơn để nhận.</p>';
    if (dst) {
      const rows = ARM_STATS.map((st) => {
        const before = statAt(dst, st.k, dst.level || 0);
        const after = statAt(dst, st.k, n);
        return `<span class="xf-stat"><span>${st.name}</span><b>${statVal(st, before)}</b>→<b class="up">${statVal(st, after)}</b></span>`;
      }).join('');
      preview = `<div class="xf-preview">
          <div class="xf-move">${armIcon(src.id, n, 'xf-img')}<span><b>+${n}</b> →</span>${armIcon(dst.id, n, 'xf-img')}</div>
          <div class="xf-note">${escapeHtml(dst.name)} <b>+${dst.level || 0} → +${n}</b>; ${escapeHtml(src.name)} về <b>+0</b></div>
          <div class="xf-stats">${rows}</div>
        </div>
        <button type="button" class="primary xf-btn" id="btn-xfer" ${gear.balance < fee || xfer.busy ? 'disabled' : ''}>${I.svg('sparkles')}Chuyển cấp · ${I.svg('coins')}${xu(fee)}</button>
        ${gear.balance < fee ? '<div class="forge-short">Không đủ xu trả phí</div>' : ''}`;
    }
    el.innerHTML = `<div class="xf-head">${I.svg('sparkles')}Chuyển cấp cường hoá <small class="muted">phí ${xu(gear.transferFeePerLevel || 300)} xu mỗi cấp</small></div>
      <div class="xf-pick">
        <label>Từ<select data-xfer="from">${froms.map((a) => opt(a, xfer.from)).join('')}</select></label>
        <label>Sang<select data-xfer="to" ${tos.length ? '' : 'disabled'}>${tos.map((a) => opt(a, xfer.to)).join('')}</select></label>
      </div>
      ${preview}
      <img class="xf-fx hidden" id="xfer-fx" src="${IMG}fx_upgrade_ok.png" alt="" draggable="false">`;
  }

  async function armAction(path, body, okMsg) {
    try {
      gear = await A.api(path, { method: 'POST', body });
      showBalance(gear.balance);
      A.toast(okMsg());
    } catch (err) {
      A.toast(err.message);
    }
    renderGear();
  }

  async function doTransfer() {
    if (xfer.busy) return;
    const src = armData(xfer.from);
    const dst = armData(xfer.to);
    if (!src || !dst) return;
    const n = src.level || 0;
    const fee = (gear.transferFeePerLevel || 300) * n;
    const ok = await A.confirm(`Chuyển +${n} từ ${src.name} sang ${dst.name} với phí ${xu(fee)} xu? ${src.name} sẽ về +0.`,
      { title: 'Chuyển cấp cường hoá', ok: 'Chuyển' });
    if (!ok) return;
    xfer.busy = true;
    try {
      gear = await A.api('/api/arena/arms/transfer', { method: 'POST', body: { from: src.id, to: dst.id } });
      showBalance(gear.balance);
      const t = gear.transfer || { level: n };
      xfer.busy = false;
      renderGear();
      const fx = $('xfer-fx');
      if (fx) { fx.classList.remove('hidden'); void fx.offsetWidth; fx.classList.add('play'); }
      A.toast(`Đã chuyển +${t.level} sang ${dst.name}`);
    } catch (err) {
      xfer.busy = false;
      A.toast(err.message);
      renderGear();
    }
  }

  // Hiệu ứng trên đe: thành công loé sáng, thất bại vỡ khói; lên +7 / +10 thì "Biến đổi!" (hình cũ → hình mới).
  function forgeFx(kind, label, fromSrc) {
    const fx = $('forge-fx');
    const anvil = $('forge-anvil');
    if (!fx || !anvil) return;
    fx.src = `${IMG}${kind === 'evolve' ? 'fx_evolve' : kind === 'ok' ? 'fx_upgrade_ok' : 'fx_upgrade_fail'}.png`;
    fx.classList.remove('hidden');
    const lb = $('forge-label');
    lb.textContent = label;
    lb.className = `fa-label ${kind}`;
    if (kind === 'evolve' && fromSrc) {
      const item = $('forge-item');
      const to = item.src;
      item.src = fromSrc;
      setTimeout(() => { item.src = to; }, 700);
    }
    anvil.classList.remove('ok', 'fail', 'evolve');
    void anvil.offsetWidth;
    anvil.classList.add(kind);
  }

  async function doForge() {
    if (forge.busy) return;
    forge.busy = true;
    const slot = forge.slot;
    const fromLv = (gear.gear || {})[slot] || 0;
    const fromSrc = slot === 'gun' && armOf(gear.gear) !== 'cannon' ? '' : `${IMG}${slot}_${form(fromLv)}.png`;
    $('btn-forge').disabled = true;
    $('forge-anvil').classList.add('working');
    try {
      const [res] = await Promise.all([
        A.api('/api/arena/gear/upgrade', { method: 'POST', body: { slot, stone: forge.stone, luck: forge.luck, protect: forge.protect } }),
        new Promise((r) => setTimeout(r, 650)), // cho búa gõ một lúc rồi mới hiện kết quả
      ]);
      gear = res;
      forge.busy = false;
      showBalance(gear.balance);
      renderGear();
      const r = res.result || {};
      const name = slot === 'gun' ? armName(r.arm || armOf(gear.gear)) : GEAR[slot].name;
      if (r.ok && r.evolved) {
        forgeFx('evolve', `Biến đổi! +${r.level}`, fromSrc);
        A.toast(`${name} biến đổi! Lên +${r.level}`);
      } else if (r.ok) {
        forgeFx('ok', `+${r.level}`);
        A.toast(`Cường hoá thành công! ${name} lên +${r.level}`);
      } else if (r.dropped) {
        forgeFx('fail', `Tụt về +${r.level}`);
        A.toast(`Thất bại (tỉ lệ ${r.rate}%) — ${name} tụt từ +${r.from ?? r.level + 1} về +${r.level}`);
      } else {
        forgeFx('fail', r.protected ? 'Giữ cấp nhờ bùa' : 'Thất bại');
        A.toast(`Thất bại (tỉ lệ ${r.rate}%)${r.protected ? ` — bùa giữ cấp đã giữ ${name} ở +${r.level}` : ''}`);
      }
    } catch (err) {
      forge.busy = false;
      A.toast(err.message);
      renderGear();
    }
  }

  async function buyCharm(id) {
    const q = clamp(Math.round(buyQty[id] || 1), 1, 99);
    try {
      gear = await A.api('/api/arena/charms/buy', { method: 'POST', body: { id, qty: q } });
      showBalance(gear.balance);
      A.toast(`Đã mua ${q} ${(CHARMS.find((c) => c.id === id) || {}).name || 'bùa'}`);
    } catch (err) {
      A.toast(err.message);
    }
    renderGear();
  }

  async function buyStones(t) {
    const q = clamp(Math.round(buyQty[t] || 1), 1, 99);
    try {
      gear = await A.api('/api/arena/stones/buy', { method: 'POST', body: { tier: t, qty: q } });
      showBalance(gear.balance);
      A.toast(`Đã mua ${q} ${stoneName(t)}`);
      forge.stone = t;
    } catch (err) {
      A.toast(err.message);
    }
    renderGear();
  }

  $('dlg-gear').addEventListener('click', (e) => {
    const tab = e.target.closest('[data-tab]');
    if (tab) {
      document.querySelectorAll('#dlg-gear [data-tab]').forEach((b) => b.classList.toggle('on', b === tab));
      $('pane-forge').classList.toggle('hidden', tab.dataset.tab !== 'forge');
      $('pane-stones').classList.toggle('hidden', tab.dataset.tab !== 'stones');
      $('pane-arms').classList.toggle('hidden', tab.dataset.tab !== 'arms');
      $('pane-gems').classList.toggle('hidden', tab.dataset.tab !== 'gems');
      return;
    }
    const ba = e.target.closest('[data-buy-arm]');
    if (ba) {
      const id = ba.dataset.buyArm;
      armAction('/api/arena/arms/buy', { id }, () => `Đã mua ${armName(id)}`);
      return;
    }
    const eq = e.target.closest('[data-equip]');
    if (eq) {
      const id = eq.dataset.equip;
      armAction('/api/arena/arms/equip', { id }, () => `Đã trang bị ${armName(id)}`);
      return;
    }
    if (e.target.closest('#btn-xfer')) { doTransfer(); return; }
    const sk = e.target.closest('[data-socket]');
    if (sk) {
      const [arm, idx] = sk.dataset.socket.split(':');
      const same = gemSel.arm === arm && gemSel.idx === Number(idx);
      gemSel.arm = same ? '' : arm;
      gemSel.idx = same ? -1 : Number(idx);
      renderArms();
      if (!same) $('gem-pick').scrollIntoView({ block: 'nearest', behavior: 'smooth' });
      return;
    }
    if (e.target.closest('[data-gem-close]')) { gemSel.arm = ''; gemSel.idx = -1; renderArms(); return; }
    const gm = e.target.closest('[data-gem]');
    if (gm) {
      const [kind, tier] = gm.dataset.gem.split(':');
      gemAction('/api/arena/gems/socket', { arm: gemSel.arm, idx: gemSel.idx, kind, tier: Number(tier) }, 'Đã khảm ngọc');
      return;
    }
    if (e.target.closest('[data-unsocket]')) {
      gemAction('/api/arena/gems/unsocket', { arm: gemSel.arm, idx: gemSel.idx }, 'Đã gỡ ngọc về kho');
      return;
    }
    const slot = e.target.closest('[data-slot]');
    if (slot) { forge.slot = slot.dataset.slot; renderGear(); return; }
    const st = e.target.closest('[data-stone]');
    if (st) { forge.stone = Number(st.dataset.stone); renderForge(); return; }
    if (e.target.closest('#btn-forge')) { doForge(); return; }
    const q = e.target.closest('[data-q]');
    if (q) {
      const raw = q.closest('[data-tier]').dataset.tier;
      const t = /^\d+$/.test(raw) ? Number(raw) : raw;
      buyQty[t] = clamp((buyQty[t] || 1) + Number(q.dataset.q), 1, 99);
      renderStoneShop();
      return;
    }
    const buy = e.target.closest('[data-buy]');
    if (buy) { buyStones(Number(buy.dataset.buy)); return; }
    const bc = e.target.closest('[data-buy-charm]');
    if (bc) buyCharm(bc.dataset.buyCharm);
  });
  $('forge').addEventListener('change', (e) => {
    const c = e.target.closest('[data-charm]');
    if (!c) return;
    forge[c.dataset.charm] = c.checked;
    renderForge();
  });
  $('arm-xfer').addEventListener('change', (e) => {
    const sel = e.target.closest('[data-xfer]');
    if (!sel) return;
    xfer[sel.dataset.xfer] = sel.value;
    renderXfer();
  });
  $('stone-shop').addEventListener('change', (e) => {
    const inp = e.target.closest('[data-qty]');
    if (!inp) return;
    buyQty[inp.dataset.qty] = clamp(Math.round(Number(inp.value) || 1), 1, 99);
    renderStoneShop();
  });
  async function loadPower() {
    try { gear = await A.api('/api/arena/gear'); $('hall-power').textContent = xu(gear.power); } catch (e) { /* bỏ qua */ }
  }

  // ---------- Nút ----------

  async function copyInvite() {
    const link = `${location.origin}/arena/?room=${state.code}`;
    try {
      await navigator.clipboard.writeText(link);
      A.toast('Đã sao chép link mời — gửi cho bạn bè để vào bàn');
    } catch (e) {
      A.ask({ title: 'Mời bạn', message: 'Sao chép link này gửi cho bạn bè:', input: link, ok: 'Xong', cancel: '' });
    }
  }
  async function leave() {
    if (!state) return;
    if (state.phase === 'playing' && me() && me().alive !== false) {
      const ok = await A.confirm('Rời bàn giữa trận sẽ bị xử thua. Vẫn rời?', { title: 'Rời bàn', ok: 'Rời bàn', cancel: 'Ở lại', danger: true });
      if (!ok) return;
    }
    send({ t: 'leave' });
  }
  function rematch() {
    if (state && state.phase === 'over' && me() && !me().ready) send({ t: 'rematch' });
  }
  const addBot = () => send({ t: 'add_bot', level: $('sel-bot').value });

  $('btn-create').addEventListener('click', openCreate);
  $('btn-gear').addEventListener('click', openGear);
  $('btn-gear-room').addEventListener('click', openGear);
  $('btn-join').addEventListener('click', () => joinCode($('code').value));
  $('code').addEventListener('keydown', (e) => { if (e.key === 'Enter') joinCode($('code').value); });
  $('tables').addEventListener('click', (e) => {
    const join = e.target.closest('[data-join]');
    if (join) return joinCode(join.dataset.join);
    if (e.target.closest('[data-create]')) openCreate();
  });
  $('btn-invite').addEventListener('click', copyInvite);
  $('btn-leave').addEventListener('click', leave);
  $('btn-rematch').addEventListener('click', rematch);
  $('btn-board').addEventListener('click', () => { hideResult = true; renderResult(); });
  $('btn-show-result').addEventListener('click', () => { hideResult = false; renderResult(); });
  $('btn-rematch-bar').addEventListener('click', rematch);
  $('btn-hall').addEventListener('click', () => send({ t: 'leave' }));
  $('btn-add-bot').addEventListener('click', addBot);
  $('btn-start').addEventListener('click', () => send({ t: 'start' }));
  $('btn-ready').addEventListener('click', () => send({ t: 'ready' }));
  $('sel-bet').addEventListener('change', (e) => send({ t: 'settings', bet: Number(e.target.value), map: $('sel-map').value }));
  $('sel-map').addEventListener('change', (e) => send(isBoss(state) ? { t: 'settings', map: e.target.value, boss: $('sel-boss').value }
    : { t: 'settings', bet: Number($('sel-bet').value), map: e.target.value }));
  $('sel-boss').addEventListener('change', (e) => send({ t: 'settings', boss: e.target.value, map: $('sel-map').value }));
  $('room').addEventListener('click', (e) => {
    const kick = e.target.closest('[data-kick]');
    if (kick) return send({ t: 'kick', id: kick.dataset.kick });
    const act = e.target.closest('[data-act]');
    if (!act) return;
    if (act.dataset.act === 'invite') copyInvite();
    if (act.dataset.act === 'bot') addBot();
  });

  // ---------- Bàn phím ----------

  const held = {};
  document.addEventListener('keydown', (e) => {
    if (!state || e.ctrlKey || e.metaKey || e.altKey) return;
    const t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    if (document.querySelector('dialog[open]')) return;
    if (state.phase === 'playing') {
      if (e.key === 'ArrowLeft') { e.preventDefault(); if (!held.l || Date.now() - held.l > 130) { held.l = Date.now(); step(-1); } return; }
      if (e.key === 'ArrowRight') { e.preventDefault(); if (!held.r || Date.now() - held.r > 130) { held.r = Date.now(); step(1); } return; }
      if (e.key === 'ArrowUp') { e.preventDefault(); nudgeAngle(1); return; }
      if (e.key === 'ArrowDown') { e.preventDefault(); nudgeAngle(-1); return; }
      if (e.key === ' ') { e.preventDefault(); if (!e.repeat) chargeStart(); return; }
      const n = Number(e.key);
      if (n >= 1 && n <= WEAPONS.length && myTurn()) {
        const w = WEAPONS[n - 1].id;
        aim.weapon = w; renderHud();
        return;
      }
    }
    if ((e.key === 'n' || e.key === 'N') && state.phase === 'over') rematch();
    else if (e.key === 'Escape' && state.phase === 'over' && !hideResult) { hideResult = true; renderResult(); }
  });
  document.addEventListener('keyup', (e) => {
    if (e.key === 'ArrowLeft') held.l = 0;
    if (e.key === 'ArrowRight') held.r = 0;
    if (e.key === ' ' && charge) { e.preventDefault(); chargeEnd(); }
  });

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(28);
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
      loadPower();
      A.api('/api/arena/maps').then((m) => { if (Array.isArray(m) && m.length) { maps = m; if (!state) renderHall(); } }, () => {});
    },
    (e) => A.toast(e.message),
  );
})();
