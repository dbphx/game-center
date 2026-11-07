// Trường đua: sảnh, garage, phòng chờ, cuộc đua 3D. Server (games/race) giữ phòng, kiểm gói vị trí, xếp hạng và trả xu;
// máy người chơi tự mô phỏng xe mình (physics.js) và gửi vị trí ≈15 lần/giây.
import { Track, newCar, step, raceDistance } from './physics.js';
import { THREE, createRenderer, buildWorld, buildCar, createSparks, createSpeedLines, PAINT } from './scene.js';

const A = window.Account;
const I = window.Icons;
const C = window.Character;
const $ = (id) => document.getElementById(id);
const xu = A.xu;
const esc = A.escapeHtml;

const BETS = [100, 500, 1000, 5000, 10000];
const TRACKS = [{ id: 'neon', name: 'Đường Neon' }, { id: 'city', name: 'Phố Đêm' }];
const SEND_MS = 66; // ≈15 gói mỗi giây
const LERP_MS = 110; // xe người khác vẽ trễ một chút để mượt
const ROOM_KEY = 'race.room';
const BASE_STATS = { maxSpeed: 30, accel: 12, grip: 0.7 };
const PART_NAMES = { engine: ['Động cơ / ống xả', 'Tốc độ'], turbo: ['Turbo', 'Tăng tốc'], tires: ['Lốp / mâm', 'Bám đường'] };
const KIND_NAMES = { spoiler: 'Cánh gió', neon: 'Neon gầm', paint: 'Sơn', decal: 'Decal' };

const touch = matchMedia('(pointer: coarse)').matches;
const low = touch || window.innerWidth < 700 || new URLSearchParams(location.search).has('low');
if (touch) document.body.classList.add('is-touch');

let user = null;
let garage = null;
let gSel = null; // xe đang xem trong garage
let gKind = 'spoiler';
let ws = null;
let room = null; // trạng thái phòng từ server
let mySeat = -1;
let view = null; // renderer dùng chung
let race = null; // cuộc đua đang chạy (mạng hoặc lái thử)
let busy = false;

const store = (k, v) => { try { v == null ? localStorage.removeItem(k) : localStorage.setItem(k, v); } catch (e) { /* bỏ qua */ } };
const load = (k) => { try { return localStorage.getItem(k); } catch (e) { return null; } };

// ---------- Màn hình ----------
function show(id) {
  for (const s of ['hall', 'garage', 'room']) $(s).classList.toggle('hidden', s !== id);
  const three = id === 'garage' || id === 'race';
  $('view').classList.toggle('hidden', !three);
  $('hud').classList.toggle('hidden', id !== 'race');
  document.querySelector('.topbar').classList.toggle('hidden', id === 'race');
  if (id !== 'garage') stopGarage();
}

function showBalance(n) { $('balance').textContent = xu(n); }
function showLevel(u) {
  const el = $('level-badge');
  const span = Math.max(1, u.nextXP - u.levelXP);
  const p = Math.min(100, Math.round(((u.xp - u.levelXP) / span) * 100));
  el.innerHTML = `<span class="lv">Lv ${u.level}</span><span class="xp-bar"><span style="width:${p}%"></span></span>`;
  el.classList.remove('hidden');
}
function setUser(u) {
  if (!u) return;
  user = u;
  showBalance(u.balance);
  showLevel(u);
}

function ensureView() {
  if (!view) {
    view = createRenderer($('view'), low);
    addEventListener('resize', () => view.resize());
  }
  return view;
}

// ---------- Sảnh ----------
async function loadRooms() {
  if ($('hall').classList.contains('hidden')) return;
  let list = [];
  try {
    list = await A.api('/api/race/rooms');
  } catch (e) {
    list = [];
  }
  list = Array.isArray(list) ? list : list.rooms || [];
  $('rooms').innerHTML = list.length
    ? list.map((r) => `<button class="room-card" data-code="${esc(r.code)}"><b>Phòng ${esc(r.code)}</b>
        <span class="muted">${esc(trackName(r.track))} · cược ${xu(r.bet || 0)} xu</span>
        <span class="muted">${(r.players || r.seats || []).length}/${r.max || 6} xe · ${r.phase === 'lobby' ? 'đang chờ' : 'đang đua'}</span></button>`).join('')
    : '<p class="muted">Chưa có phòng nào. Tạo phòng rồi mời bạn bè, hoặc thêm máy để đua ngay.</p>';
}
const trackName = (id) => ((room && room.tracks) || TRACKS).concat(TRACKS).find((t) => t.id === id)?.name || id || '';

function chips(el, items, sel, onPick) {
  el.innerHTML = items.map((it) => `<button type="button" data-v="${esc(String(it.v))}" class="${String(it.v) === String(sel) ? 'on' : ''}">${esc(it.label)}</button>`).join('');
  el.onclick = (e) => {
    const b = e.target.closest('button[data-v]');
    if (!b) return;
    el.querySelectorAll('button').forEach((x) => x.classList.toggle('on', x === b));
    onPick(b.dataset.v);
  };
}

function openCreate() {
  let track = 'neon';
  let bet = 100;
  chips($('track-grid'), TRACKS.map((t) => ({ v: t.id, label: t.name })), track, (v) => (track = v));
  chips($('bet-grid'), BETS.map((b) => ({ v: b, label: `${xu(b)} xu` })), bet, (v) => (bet = Number(v)));
  const dlg = $('dlg-create');
  dlg.onclose = () => {
    if (dlg.returnValue !== 'ok') return;
    connect(() => send({ t: 'create', token: A.token(), bet, track }));
  };
  dlg.returnValue = '';
  dlg.showModal();
}

function openPractice() {
  let track = 'neon';
  chips($('practice-grid'), TRACKS.map((t) => ({ v: t.id, label: t.name })), track, (v) => (track = v));
  const dlg = $('dlg-practice');
  dlg.onclose = () => { if (dlg.returnValue === 'ok') startPractice(track); };
  dlg.returnValue = '';
  dlg.showModal();
}

// ---------- Garage ----------
const curCar = () => (garage ? garage.cars.find((c) => c.id === garage.current) || garage.cars[0] : null);
const specOf = (car, number) => ({ car: car.id, levels: car.levels || {}, cosmetics: car.cosmetics || {}, number });

async function loadGarage() {
  garage = await A.api('/api/race/garage');
  if (!gSel || !garage.cars.some((c) => c.id === gSel)) gSel = garage.current || (garage.cars[0] && garage.cars[0].id);
}

let gScene = null;
let gCam = null;
let gCarObj = null;
let gAngle = 0.6;
let gDrag = null;
let gToken = 0;

function setupGarageScene() {
  if (gScene) return;
  gScene = new THREE.Scene();
  gScene.background = new THREE.Color(0x0b0f2a);
  gScene.fog = new THREE.Fog(0x0b0f2a, 18, 40);
  gCam = new THREE.PerspectiveCamera(40, 1, 0.1, 100);
  gScene.add(new THREE.HemisphereLight(0x8aa2ff, 0x101418, 1.4));
  const key = new THREE.DirectionalLight(0xffffff, 2.2);
  key.position.set(4, 8, 6);
  gScene.add(key);
  const rim = new THREE.DirectionalLight(0x58f0ff, 1.6);
  rim.position.set(-6, 3, -6);
  gScene.add(rim);
  const floor = new THREE.Mesh(new THREE.CircleGeometry(4.2, 48), new THREE.MeshStandardMaterial({ color: 0x161b40, metalness: 0.6, roughness: 0.35 }));
  floor.rotation.x = -Math.PI / 2;
  gScene.add(floor);
  const ring = new THREE.Mesh(new THREE.RingGeometry(4.15, 4.35, 64), new THREE.MeshBasicMaterial({ color: 0x2fe6ff, toneMapped: false }));
  ring.rotation.x = -Math.PI / 2;
  ring.position.y = 0.01;
  gScene.add(ring);
  const cv = $('view');
  cv.addEventListener('pointerdown', (e) => { if (!$('garage').classList.contains('hidden')) gDrag = e.clientX; });
  addEventListener('pointermove', (e) => { if (gDrag != null) { gAngle -= (e.clientX - gDrag) * 0.01; gDrag = e.clientX; } });
  addEventListener('pointerup', () => (gDrag = null));
}

let gLoop = 0;
function stopGarage() { cancelAnimationFrame(gLoop); gLoop = 0; }

async function showGarageCar() {
  const car = garage.cars.find((c) => c.id === gSel);
  if (!car) return;
  const token = ++gToken;
  const obj = await buildCar(specOf(car, 7), low);
  if (token !== gToken) return;
  if (gCarObj) gScene.remove(gCarObj.root);
  gCarObj = obj;
  gScene.add(obj.root);
}

async function openGarage() {
  if (!user) return A.requireLogin();
  show('garage');
  try {
    await loadGarage();
  } catch (e) {
    A.toast(e.message);
    return show('hall');
  }
  const v = ensureView();
  setupGarageScene();
  v.use(gScene, gCam);
  renderGarage();
  await showGarageCar();
  let last = performance.now();
  const loop = (now) => {
    const dt = Math.min(0.05, (now - last) / 1000);
    last = now;
    if (gDrag == null) gAngle += dt * 0.35;
    const wide = innerWidth > 640;
    const r = wide ? 12 : 18;
    // Máy tính: bảng bên phải nên đẩy xe sang trái; điện thoại: bảng ở dưới nên đẩy xe lên trên.
    const look = wide ? new THREE.Vector3(Math.cos(gAngle) * 2.6, 0.7, -Math.sin(gAngle) * 2.6) : new THREE.Vector3(0, -0.6, 0);
    gCam.position.set(Math.sin(gAngle) * r + look.x, 3.6, Math.cos(gAngle) * r + look.z);
    gCam.lookAt(look);
    if (gCarObj) gCarObj.update(0, 0, false, false, dt);
    v.render();
    gLoop = requestAnimationFrame(loop);
  };
  stopGarage();
  gLoop = requestAnimationFrame(loop);
}

function renderGarage() {
  const car = garage.cars.find((c) => c.id === gSel);
  $('g-cars').innerHTML = garage.cars.map((c) => `<button class="car-tab ${c.id === gSel ? 'on' : ''}" data-car="${esc(c.id)}">
      <b>${esc(c.name)}</b><small>${c.owned ? (c.id === garage.current ? 'Đang dùng' : 'Đã có') : c.price ? `${xu(c.price)} xu` : 'Miễn phí'}</small></button>`).join('');
  $('g-name').textContent = car.name;
  $('g-state').textContent = car.owned ? (car.id === garage.current ? 'Đang dùng' : 'Đã có') : 'Chưa mua';
  const st = car.stats || BASE_STATS;
  const bar = (v, lo, hi) => `<span class="bar"><span style="width:${Math.round(Math.max(0.08, Math.min(1, (v - lo) / (hi - lo))) * 100)}%"></span></span>`;
  $('g-stats').innerHTML = `
    <div class="stat-row"><span>Tốc độ</span>${bar(st.maxSpeed, 24, 37)}<b>${Math.round(st.maxSpeed * 3.6)}</b></div>
    <div class="stat-row"><span>Tăng tốc</span>${bar(st.accel, 9, 15)}<b>${st.accel.toFixed(1)}</b></div>
    <div class="stat-row"><span>Bám đường</span>${bar(st.grip, 0.55, 1)}<b>${Math.round(st.grip * 100)}</b></div>`;
  $('g-action').innerHTML = !car.owned
    ? `<button class="primary" data-act="buy">Mua xe — ${xu(car.price)} xu</button>`
    : car.id !== garage.current ? '<button class="primary" data-act="select">Dùng xe này</button>' : '';
  // 3 món nâng cấp có chỉ số.
  const max = garage.maxLevel || 5;
  const prices = (garage.upgradePrice || {})[car.id] || {};
  const parts = ['engine', 'turbo', 'tires'].map((p) => {
    const lv = (car.levels || {})[p] || 0;
    const price = prices[p] || 0;
    const btn = !car.owned ? '' : lv >= max ? '<span class="chip">Tối đa</span>' : `<button data-act="up" data-part="${p}">Nâng — ${xu(price)} xu</button>`;
    return `<div class="part"><b>${PART_NAMES[p][0]}</b>${btn}
      <span class="dots">${Array.from({ length: max }, (_, i) => `<i class="${i < lv ? 'on' : ''}"></i>`).join('')}</span>
      <small>Cấp ${lv}/${max} · tăng ${PART_NAMES[p][1].toLowerCase()} · hình trên xe đổi theo cấp</small></div>`;
  }).join('');
  $('g-action').insertAdjacentHTML('beforeend', parts);
  $('g-tabs').innerHTML = Object.entries(KIND_NAMES).map(([k, n]) => `<button data-kind="${k}" class="${k === gKind ? 'on' : ''}">${n}</button>`).join('');
  const on = (car.cosmetics || {})[gKind] || '';
  const items = (garage.items || []).filter((it) => it.kind === gKind);
  $('g-list').innerHTML = (gKind !== 'paint' ? [`<button class="item ${on ? '' : 'on'}" data-item="" data-kind="${gKind}"><b>Không lắp</b><span class="muted">—</span></button>`] : [])
    .concat(items.map((it) => {
      const sw = gKind === 'paint' || gKind === 'neon' ? `<span class="swatch" style="background:#${(PAINT[Object.keys(PAINT).find((c) => it.id.includes(c))] || 0x888888).toString(16).padStart(6, '0')}"></span>` : '';
      const state = it.id === on ? 'Đang lắp' : it.owned || !it.price ? 'Lắp' : `${xu(it.price)} xu`;
      return `<button class="item ${it.id === on ? 'on' : ''}" data-item="${esc(it.id)}" data-kind="${it.kind}"><b>${sw}${esc(it.name)}</b><span class="muted">${state}</span></button>`;
    })).join('');
}

async function garageAct(path, body, msg) {
  if (busy) return;
  busy = true;
  try {
    const r = await A.api(path, { method: 'POST', body });
    if (r.garage) garage = r.garage;
    if (r.user) setUser(r.user);
    if (msg) A.toast(msg);
    renderGarage();
    await showGarageCar();
  } catch (e) {
    A.toast(e.message);
  } finally {
    busy = false;
  }
}

function onGarageClick(e) {
  const tab = e.target.closest('[data-car]');
  if (tab) { gSel = tab.dataset.car; renderGarage(); showGarageCar(); return; }
  const kind = e.target.closest('[data-kind]:not([data-item])');
  if (kind) { gKind = kind.dataset.kind; renderGarage(); return; }
  const car = garage && garage.cars.find((c) => c.id === gSel);
  if (!car) return;
  const act = e.target.closest('[data-act]');
  if (act) {
    const a = act.dataset.act;
    if (a === 'buy') {
      A.confirm(`Mua ${car.name} giá ${xu(car.price)} xu?`, { title: 'Mua xe', ok: 'Mua' }).then((ok) => ok && garageAct('/api/race/car/buy', { car: car.id }, `Đã mua ${car.name}`));
    } else if (a === 'select') {
      garageAct('/api/race/car/select', { car: car.id }, `Đã chọn ${car.name}`);
    } else if (a === 'up') {
      garageAct('/api/race/upgrade', { car: car.id, part: act.dataset.part }, `Đã nâng ${PART_NAMES[act.dataset.part][0].toLowerCase()}`);
    }
    return;
  }
  const item = e.target.closest('[data-item]');
  if (item) {
    if (!car.owned) return A.toast('Mua xe trước rồi mới lắp đồ');
    const id = item.dataset.item;
    const it = (garage.items || []).find((x) => x.id === id);
    if (it && !it.owned && it.price) {
      A.confirm(`Mua ${it.name} giá ${xu(it.price)} xu rồi lắp lên ${car.name}?`, { title: 'Mua đồ', ok: 'Mua' }).then(async (ok) => {
        if (!ok) return;
        await garageAct('/api/race/item/buy', { item: id });
        garageAct('/api/race/equip', { car: car.id, kind: item.dataset.kind, item: id }, `Đã lắp ${it.name}`);
      });
    } else {
      garageAct('/api/race/equip', { car: car.id, kind: item.dataset.kind, item: id });
    }
  }
}

// ---------- Kết nối phòng ----------
function connect(then) {
  if (ws && ws.readyState === WebSocket.OPEN) { then && then(); return; }
  if (ws && ws.readyState === WebSocket.CONNECTING) { ws.addEventListener('open', () => then && then(), { once: true }); return; }
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${proto}://${location.host}/ws/race?token=${encodeURIComponent(A.token() || '')}`);
  ws.onopen = () => then && then();
  ws.onmessage = (e) => {
    let m;
    try { m = JSON.parse(e.data); } catch (err) { return; }
    onMessage(m);
  };
  ws.onclose = () => {
    ws = null;
    if (room) {
      A.toast('Mất kết nối phòng đua');
      if (race && race.net) endRace();
      room = null;
      show('hall');
      loadRooms();
    }
  };
}
function send(m) { if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(m)); }

function onMessage(m) {
  switch (m.t) {
    case 'room': return onRoom(m.room || m);
    case 'cars': return race && race.net && onCars(m);
    case 'finish': return race && race.net && onFinish(m);
    case 'result':
      if (race && race.net) { if (race.resultShown) return null; race.resultShown = true; }
      return onResult(m);
    case 'error': return A.toast(m.msg || m.error || 'Lỗi');
    case 'session': if (m.code) store(ROOM_KEY, m.code); return null;
    case 'left':
      store(ROOM_KEY, null);
      room = null;
      if (!race) { show('hall'); loadRooms(); }
      return null;
    case 'chat': return null;
    default: return null;
  }
}

const seatsOf = (r) => r.seats || r.players || [];

function onRoom(r) {
  room = r;
  if (r.code) store(ROOM_KEY, r.code);
  mySeat = typeof r.seat === 'number' ? r.seat : seatsOf(r).findIndex((s) => s && user && s.id === user.id);
  if (r.phase === 'over' && r.result && race && race.net && !race.resultShown) {
    race.resultShown = true;
    onResult(r.result);
  }
  if (r.phase === 'lobby' || r.phase === 'over' || !r.phase) {
    if (race && race.net && r.phase === 'lobby' && $('result').classList.contains('hidden')) endRace();
    if (!race) { show('room'); renderRoom(); }
    return;
  }
  // Đếm ngược / đang đua: dựng cuộc đua nếu chưa có.
  if (!race || !race.net || race.code !== r.code) startNetRace(r);
  else race.room = r;
}

function renderRoom() {
  const r = room;
  $('room-code').textContent = r.code || '';
  $('room-bet').textContent = `Cược ${xu(r.bet || 0)} xu`;
  $('room-track').textContent = trackName(r.track);
  const seats = seatsOf(r);
  const host = r.host === mySeat || (seats[mySeat] && seats[mySeat].host);
  const max = r.max || 6;
  const cards = [];
  for (let i = 0; i < max; i++) {
    const s = seats[i];
    if (!s) { cards.push('<div class="seat empty">Ghế trống</div>'); continue; }
    const car = s.car && (s.car.name || s.car.id || s.car);
    const last = r.phase === 'over' && s.place ? ` · về hạng ${s.place}` : r.phase === 'over' && s.dnf ? ' · không về đích' : '';
    cards.push(`<div class="seat ${i === mySeat ? 'me' : ''}"><b>${esc(s.name || 'Máy')}${s.bot ? ' · máy' : ''}${s.host ? ' ★' : ''}</b>
      <small>${esc(typeof car === 'string' ? car : '')}${last}</small><small>${s.bot ? '' : s.online === false ? 'mất kết nối' : `${xu(s.coins || 0)} xu`}</small>
      ${host && i !== mySeat ? `<button class="kick" data-kick="${i}">Mời ra</button>` : ''}</div>`);
  }
  $('seats').innerHTML = cards.join('');
  $('room-host').innerHTML = host
    ? `<button data-h="addBot"><i data-icon="plus"></i>Thêm máy</button>
       <select id="sel-track">${(r.tracks && r.tracks.length ? r.tracks : TRACKS).map((t) => `<option value="${esc(t.id)}" ${t.id === r.track ? 'selected' : ''}>${esc(t.name)}</option>`).join('')}</select>
       <select id="sel-bet">${(r.betOptions && r.betOptions.length ? r.betOptions : BETS).map((b) => `<option value="${b}" ${b === r.bet ? 'selected' : ''}>${b ? `${xu(b)} xu` : 'Chơi vui'}</option>`).join('')}</select>
       <button class="primary" data-h="start">Bắt đầu</button>`
    : '';
  $('room-note').textContent = mySeat < 0 ? 'Bạn đang xem phòng này.' : host ? (r.phase === 'over' ? 'Bấm Bắt đầu để đua ván mới.' : 'Cần ít nhất 2 xe. Ai không đủ xu cược thì không bắt đầu được.') : 'Chờ chủ phòng bắt đầu…';
  I.render($('room-host'));
}

// ---------- Cuộc đua ----------
async function loadTrack(id) {
  const res = await fetch(`/race/tracks/${encodeURIComponent(id)}.json`);
  if (!res.ok) throw new Error('Không tải được đường đua');
  return new Track(await res.json());
}

const keys = new Set();
const touchKeys = new Set();
function input() {
  const k = (a, b) => keys.has(a) || keys.has(b) || touchKeys.has(a);
  const left = k('ArrowLeft', 'KeyA') || touchKeys.has('left');
  const right = k('ArrowRight', 'KeyD') || touchKeys.has('right');
  const up = k('ArrowUp', 'KeyW');
  const down = k('ArrowDown', 'KeyS') || touchKeys.has('brake');
  return {
    throttle: down ? -1 : up || touch ? 1 : 0, // điện thoại tự chạy
    steer: (left ? 1 : 0) - (right ? 1 : 0),
    drift: keys.has('ShiftLeft') || keys.has('ShiftRight') || touchKeys.has('drift'),
    nitro: keys.has('Space') || touchKeys.has('nitro'),
  };
}

// newRace dựng cảnh, xe và vòng lặp. entries: [{seat, name, spec, stats, me, bot}].
async function newRace({ trackId, entries, net, code, startAt }) {
  $('loading').classList.remove('hidden');
  show('race');
  const v = ensureView();
  const track = await loadTrack(trackId);
  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(72, 1, 0.3, 1500);
  scene.add(camera);
  v.use(scene, camera);
  await buildWorld(scene, track, low);
  const sparks = createSparks(scene);
  const lines = createSpeedLines(camera);
  const cars = [];
  for (const e of entries) {
    const sp = track.data.spawn[e.seat % track.data.spawn.length];
    const obj = await buildCar(e.spec, low);
    scene.add(obj.root);
    const st = newCar(sp, e.stats || BASE_STATS);
    cars.push({ ...e, obj, st, snaps: [], dist: 0, place: 0, done: false });
  }
  // Đèn pha cho xe mình (máy tính).
  const me = cars.find((c) => c.me);
  if (me && !low) {
    const spot = new THREE.SpotLight(0xeaf6ff, 40, 60, 0.5, 0.6, 1.2);
    spot.position.set(0, 1.2, 1.6);
    spot.target.position.set(0, 0, 14);
    me.obj.root.add(spot, spot.target);
  }
  race = {
    net, code, track, scene, camera, cars, me, sparks, lines,
    startAt: startAt || Date.now() + 3500,
    lastSend: 0, finishMs: 0, ended: false, room: null,
    cam: { x: 0, y: 6, z: 0 },
  };
  drawMinimapBase(track);
  $('loading').classList.add('hidden');
  $('hud-lap').textContent = `1/${track.laps}`;
  let last = performance.now();
  const loop = (now) => {
    if (!race || race.scene !== scene) return;
    const dt = Math.min(0.05, (now - last) / 1000);
    last = now;
    tick(dt);
    v.render();
    race.raf = requestAnimationFrame(loop);
  };
  race.raf = requestAnimationFrame(loop);
}

function tick(dt) {
  const r = race;
  const now = Date.now();
  const left = r.startAt - now;
  const cd = $('countdown');
  if (left > 0) {
    cd.classList.remove('hidden');
    cd.textContent = String(Math.ceil(left / 1000));
  } else if (left > -900) {
    cd.classList.remove('hidden');
    cd.textContent = 'ĐUA!';
  } else cd.classList.add('hidden');
  const go = left <= 0;

  for (const c of r.cars) {
    if (c.me) {
      // window.__autopilot: script thử nghiệm cho xe mình tự lái như máy.
      const inp = go && !c.st.finished ? (window.__autopilot ? autoSteer(r.track, c.st, 1) : input()) : { throttle: c.st.finished ? 0.3 : 0, steer: 0 };
      if (c.st.finished) Object.assign(inp, autoSteer(r.track, c.st));
      const ev = fixedSteps(c, inp, r.track, dt);
      for (const e of ev) {
        if (e === 'wall') r.sparks.emit(c.st.x, 0.6, c.st.z, 14, 7);
        if (e === 'nitro') banner('NITRO!', 700);
        if (e === 'lap' && !c.st.finished) banner(`Vòng ${c.st.lap + 1}/${r.track.laps}`, 1200);
        if (e === 'finish' && !r.net) localFinish(c);
        if (e === 'finish' && r.net) banner('Về đích!', 2000);
      }
      if (r.net && go && now - r.lastSend >= SEND_MS) {
        r.lastSend = now;
        send({ t: 'state', ms: now - r.startAt, x: round(c.st.x), z: round(c.st.z), rot: round(c.st.rot, 3), speed: round(c.st.speed), lap: c.st.lap, cp: c.st.cp, nitro: c.st.boostMs > 0, drift: c.st.drift });
      }
    } else if (r.net) {
      lerpRemote(c, now);
    } else if (go) {
      const inp = autoSteer(r.track, c.st, c.skill);
      const ev = fixedSteps(c, inp, r.track, dt);
      if (ev.includes('finish')) localFinish(c);
    }
    const boost = c.me ? c.st.boostMs > 0 : !!c.st.boost;
    c.obj.root.position.set(c.st.x, 0, c.st.z);
    c.obj.root.rotation.y = c.st.rot;
    c.obj.update(c.st.speed, c.me ? c.st.steerVis : 0, c.st.drift, boost, dt);
    if (c.st.drift && Math.abs(c.st.speed) > 12 && Math.random() < 0.5) {
      const bx = c.st.x - Math.sin(c.st.rot) * c.obj.length * 0.45;
      const bz = c.st.z - Math.cos(c.st.rot) * c.obj.length * 0.45;
      r.sparks.emit(bx, 0.15, bz, 2, 3, 0xffb547);
    }
    c.dist = c.done ? Infinity : raceDistance(r.track, c.st.lap, c.st.cp, c.st.x, c.st.z, c.st.seg);
  }
  r.sparks.update(dt);
  // Thứ hạng: ai về đích xếp theo thứ tự về, còn lại theo quãng đường.
  const order = [...r.cars].sort((a, b) => (a.place && b.place ? a.place - b.place : a.place ? -1 : b.place ? 1 : b.dist - a.dist));
  // Người xem (không có ghế): HUD và camera theo xe dẫn đầu, ẩn nút lái.
  const me = r.me || order[0];
  $('hud').classList.toggle('watching', !r.me);
  if (!r.me && me) $('banner-watch').textContent = `Đang xem · ${me.name || 'xe dẫn đầu'}`;
  if (me) {
    $('hud-pos').textContent = `${order.indexOf(me) + 1}/${r.cars.length}`;
    $('hud-lap').textContent = `${Math.min(me.st.lap + 1, r.track.laps)}/${r.track.laps}`;
    $('hud-speed').textContent = Math.round(Math.abs(me.st.speed) * 3.6);
    $('hud-nitro').style.width = `${Math.round((me.st.boostMs > 0 ? me.st.boostMs / 2000 : me.st.nitro) * 100)}%`;
    $('hud-nitro-k').classList.toggle('ready', me.st.nitro >= 1);
    const t = go ? (r.me && me.st.finished && r.finishMs ? r.finishMs : now - r.startAt) : 0;
    $('hud-time').textContent = fmtTime(t);
    followCam(me, dt);
    r.lines.update(dt, me.st.boostMs > 0 ? 1 : 0, Math.max(10, me.st.speed));
  }
  drawMinimap(order);
}

// fixedSteps chạy vật lý theo bước cố định 1/60 s (tối đa 0,25 s mỗi khung) để tốc độ xe không phụ thuộc FPS.
const STEP = 1 / 60;
function fixedSteps(c, inp, track, dt) {
  c.acc = Math.min(0.25, (c.acc || 0) + dt);
  const ev = [];
  while (c.acc >= STEP) {
    c.acc -= STEP;
    ev.push(...step(c.st, inp, track, STEP));
  }
  return ev;
}

// Máy đua (chỉ khi lái thử): nhắm điểm phía trước trên đường tâm, thỉnh thoảng drift ở cua.
function autoSteer(track, st, skill = 0.9) {
  const near = track.nearest(st.x, st.z, st.seg);
  const ahead = track.pointAt(track.progress(near.i, near.t) + 10 + Math.abs(st.speed) * 0.55);
  const want = Math.atan2(ahead.x - st.x, ahead.z - st.z);
  let d = want - st.rot;
  while (d > Math.PI) d -= Math.PI * 2;
  while (d < -Math.PI) d += Math.PI * 2;
  const steer = Math.max(-1, Math.min(1, d * 2.2));
  return { throttle: Math.abs(d) > 0.6 && st.speed > 18 ? 0.2 : skill, steer, drift: Math.abs(d) > 0.35 && st.speed > 16, nitro: st.nitro >= 1 && Math.abs(d) < 0.1 };
}

function followCam(c, dt) {
  const r = race;
  const fx = Math.sin(c.st.rot);
  const fz = Math.cos(c.st.rot);
  const back = 5.6 + (c.obj.height || 1.5) * 0.9 + Math.max(0, c.st.speed) * 0.03;
  const tx = c.st.x - fx * back;
  const tz = c.st.z - fz * back;
  const k = 1 - Math.exp(-dt * 8);
  r.cam.x += (tx - r.cam.x) * k;
  r.cam.z += (tz - r.cam.z) * k;
  const h = c.obj.height || 1.5;
  r.cam.y += (h + 1.7 - r.cam.y) * k;
  r.camera.position.set(r.cam.x, r.cam.y, r.cam.z);
  r.camera.lookAt(c.st.x + fx * 6, h * 0.55 + 0.6, c.st.z + fz * 6);
  const fov = 66 + Math.max(0, c.st.speed) * 0.25 + (c.st.boostMs > 0 ? 8 : 0);
  r.camera.fov += (fov - r.camera.fov) * Math.min(1, dt * 4);
  r.camera.updateProjectionMatrix();
}

// Xe người khác: nội suy giữa hai gói gần nhất, vẽ trễ LERP_MS.
function onCars(m) {
  const at = Date.now();
  for (const s of m.list || []) {
    const c = race.cars.find((x) => x.seat === s.seat);
    if (!c || c.me) continue;
    c.snaps.push({ at, ...s });
    if (c.snaps.length > 6) c.snaps.shift();
  }
}
function lerpRemote(c, now) {
  const t = now - LERP_MS;
  const sn = c.snaps;
  if (!sn.length) return;
  let a = sn[0];
  let b = sn[sn.length - 1];
  for (let i = 0; i < sn.length - 1; i++) if (sn[i].at <= t && sn[i + 1].at >= t) { a = sn[i]; b = sn[i + 1]; break; }
  const k = b.at === a.at ? 1 : Math.max(0, Math.min(1, (t - a.at) / (b.at - a.at)));
  let dr = b.rot - a.rot;
  while (dr > Math.PI) dr -= Math.PI * 2;
  while (dr < -Math.PI) dr += Math.PI * 2;
  Object.assign(c.st, { x: a.x + (b.x - a.x) * k, z: a.z + (b.z - a.z) * k, rot: a.rot + dr * k, speed: b.speed || 0, lap: b.lap || 0, cp: b.cp || 0, drift: !!b.drift, boost: !!b.nitro });
}

function onFinish(m) {
  const c = race.cars.find((x) => x.seat === m.seat);
  if (!c) return;
  c.place = m.place;
  c.done = true;
  if (c.me) {
    race.finishMs = m.timeMs;
    banner(`Về đích hạng ${m.place}!`, 3000);
  }
}

function localFinish(c) {
  if (c.place) return;
  const placed = race.cars.filter((x) => x.place).length;
  c.place = placed + 1;
  c.done = true;
  c.timeMs = Date.now() - race.startAt;
  if (c.me) {
    race.finishMs = c.timeMs;
    banner(`Về đích hạng ${c.place}!`, 3000);
    setTimeout(() => {
      if (!race || race.net) return;
      const list = [...race.cars].sort((a, b) => (a.place || 99) - (b.place || 99) || b.dist - a.dist);
      showResult('Lái thử xong', list.map((x) => ({ name: x.name, me: x.me, time: x.timeMs, money: null })));
    }, 2500);
  }
}

function onResult(m) {
  const seats = room ? seatsOf(room) : [];
  const list = (m.places || []).map((p) => ({
    name: (seats[p.seat] && seats[p.seat].name) || `Xe ${p.seat + 1}`,
    me: p.seat === mySeat,
    time: p.timeMs,
    dnf: p.dnf,
    money: m.money ? pick(m.money, p.seat, seats) : null,
    tax: m.tax ? pick(m.tax, p.seat, seats) : 0,
  }));
  const mine = list.findIndex((x) => x.me);
  showResult(mine === 0 ? 'Bạn về nhất!' : mine > 0 ? `Bạn về hạng ${mine + 1}` : 'Kết quả', list);
  A.me().then(setUser, () => {});
}
// money / tax có thể là mảng theo ghế hoặc map theo id người chơi.
function pick(v, seat, seats) {
  if (Array.isArray(v)) return v[seat] || 0;
  const s = seats[seat];
  return (s && (v[s.id] ?? v[String(seat)])) || 0;
}

function showResult(title, list) {
  $('res-title').textContent = title;
  $('res-list').innerHTML = list.map((x) => `<li class="${x.me ? 'me' : ''}">${esc(x.name)}${x.time ? ` · ${fmtTime(x.time)}` : x.dnf ? ' · không về đích' : ' · chưa về'}
    ${x.money != null ? `<span class="money ${x.money >= 0 ? 'plus' : 'minus'}">${x.money >= 0 ? '+' : '−'}${xu(Math.abs(x.money))} xu${x.tax ? ` (thuế ${xu(x.tax)})` : ''}</span>` : ''}</li>`).join('');
  $('result').classList.remove('hidden');
}

function endRace() {
  if (!race) return;
  cancelAnimationFrame(race.raf);
  race.scene.traverse((o) => { if (o.geometry) o.geometry.dispose(); });
  race = null;
  $('countdown').classList.add('hidden');
  $('banner').classList.add('hidden');
}

async function startPractice(trackId) {
  if (race) endRace();
  let car = null;
  if (user) {
    try { await loadGarage(); car = curCar(); } catch (e) { car = null; }
  }
  const mine = car ? specOf(car, 7) : { car: 'starter', levels: {}, cosmetics: { paint: 'paint_red' }, number: 7 };
  const bots = ['viper', 'phantom', 'comet'].map((id, i) => ({
    seat: i + 1, name: `Máy ${i + 1}`, bot: true, skill: 0.82 + i * 0.05,
    spec: { car: id, levels: { engine: i, turbo: i, tires: i * 2 }, cosmetics: { paint: ['paint_blue', 'paint_yellow', 'paint_green'][i], neon: ['neon_pink', 'neon_blue', 'neon_green'][i], spoiler: i === 1 ? 'spoiler_gt' : '' }, number: i + 2 },
    stats: { maxSpeed: 29 + i, accel: 11.5 + i * 0.4, grip: 0.7 },
  }));
  try {
    await newRace({ trackId, net: false, entries: [{ seat: 0, name: user ? user.name : 'Bạn', me: true, spec: mine, stats: (car && car.stats) || BASE_STATS }, ...bots] });
  } catch (e) {
    A.toast(e.message);
    $('loading').classList.add('hidden');
    show('hall');
  }
}

async function startNetRace(r) {
  if (race) endRace();
  const seats = seatsOf(r);
  const entries = [];
  seats.forEach((s, i) => {
    if (!s) return;
    const seat = typeof s.seat === 'number' ? s.seat : i;
    const car = s.car || {};
    const id = typeof car === 'string' ? car : car.id || 'starter';
    entries.push({
      seat, name: s.name || `Máy ${seat + 1}`, me: seat === mySeat, bot: !!s.bot,
      spec: { car: id, levels: car.levels || s.levels || {}, cosmetics: car.cosmetics || s.cosmetics || {}, number: seat + 1 },
      stats: car.stats || s.stats || BASE_STATS,
    });
  });
  try {
    await newRace({ trackId: r.track || 'neon', entries, net: true, code: r.code, startAt: r.startAt ? Date.now() + (r.startAt - (r.now || Date.now())) : 0 });
    race.room = r;
  } catch (e) {
    A.toast(e.message);
    $('loading').classList.add('hidden');
  }
}

// ---------- Tiện ích HUD ----------
const round = (v, d = 2) => Math.round(v * 10 ** d) / 10 ** d;
function fmtTime(ms) {
  if (!ms || ms < 0) return '0:00.0';
  const s = ms / 1000;
  return `${Math.floor(s / 60)}:${(s % 60).toFixed(1).padStart(4, '0')}`;
}
let bannerTimer = 0;
function banner(text, ms) {
  const b = $('banner');
  b.textContent = text;
  b.classList.remove('hidden');
  clearTimeout(bannerTimer);
  bannerTimer = setTimeout(() => b.classList.add('hidden'), ms);
}

let mini = null;
function drawMinimapBase(track) {
  const cv = $('minimap');
  const p = track.path;
  let x0 = Infinity; let x1 = -Infinity; let z0 = Infinity; let z1 = -Infinity;
  for (const q of p) { x0 = Math.min(x0, q.x); x1 = Math.max(x1, q.x); z0 = Math.min(z0, q.z); z1 = Math.max(z1, q.z); }
  const s = 140 / Math.max(x1 - x0, z1 - z0);
  const map = (x, z) => [10 + (x1 - x) * s + (140 - (x1 - x0) * s) / 2, 10 + (z1 - z) * s + (140 - (z1 - z0) * s) / 2];
  const base = document.createElement('canvas');
  base.width = base.height = 160;
  const g = base.getContext('2d');
  g.strokeStyle = 'rgba(200, 220, 255, .85)';
  g.lineWidth = 5;
  g.lineJoin = 'round';
  g.beginPath();
  p.forEach((q, i) => { const [x, y] = map(q.x, q.z); i ? g.lineTo(x, y) : g.moveTo(x, y); });
  g.closePath();
  g.stroke();
  mini = { cv, base, map };
}
function drawMinimap(order) {
  if (!mini) return;
  const g = mini.cv.getContext('2d');
  g.clearRect(0, 0, 160, 160);
  g.drawImage(mini.base, 0, 0);
  for (const c of order) {
    const [x, y] = mini.map(c.st.x, c.st.z);
    g.fillStyle = c.me ? '#ffd23f' : '#ff4fa0';
    g.beginPath();
    g.arc(x, y, c.me ? 5 : 4, 0, Math.PI * 2);
    g.fill();
  }
}

// ---------- Sự kiện ----------
addEventListener('keydown', (e) => {
  if (!race) return;
  if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Space'].includes(e.code)) e.preventDefault();
  keys.add(e.code);
});
addEventListener('keyup', (e) => keys.delete(e.code));
addEventListener('blur', () => { keys.clear(); touchKeys.clear(); });
for (const b of document.querySelectorAll('#touch button')) {
  const k = b.dataset.k;
  const on = (e) => { e.preventDefault(); touchKeys.add(k); b.classList.add('down'); };
  const off = (e) => { e.preventDefault(); touchKeys.delete(k); b.classList.remove('down'); };
  b.addEventListener('pointerdown', on);
  b.addEventListener('pointerup', off);
  b.addEventListener('pointercancel', off);
  b.addEventListener('pointerleave', off);
}

$('btn-create').onclick = () => (user ? openCreate() : A.requireLogin());
$('btn-join').onclick = () => {
  const code = $('code').value.trim().toUpperCase();
  if (!code) return A.toast('Nhập mã phòng');
  connect(() => send({ t: 'join', code, token: A.token() }));
};
$('rooms').onclick = (e) => {
  const b = e.target.closest('[data-code]');
  if (b) connect(() => send({ t: 'join', code: b.dataset.code, token: A.token() }));
};
$('btn-garage').onclick = openGarage;
$('btn-practice').onclick = openPractice;
$('btn-garage-back').onclick = () => { show('hall'); loadRooms(); };
$('garage').addEventListener('click', onGarageClick);
$('btn-leave').onclick = () => { send({ t: 'leave' }); store(ROOM_KEY, null); room = null; show('hall'); loadRooms(); };
$('btn-invite').onclick = () => {
  const link = `${location.origin}/race/?room=${room && room.code}`;
  (navigator.clipboard ? navigator.clipboard.writeText(link) : Promise.reject()).then(() => A.toast('Đã sao chép link mời'), () => A.toast(link));
};
$('room-host').addEventListener('click', (e) => {
  const b = e.target.closest('[data-h]');
  if (b) send({ t: b.dataset.h });
});
$('room-host').addEventListener('change', (e) => {
  if (e.target.id === 'sel-track') send({ t: 'setTrack', track: e.target.value });
  if (e.target.id === 'sel-bet') send({ t: 'setBet', bet: Number(e.target.value) });
});
$('seats').addEventListener('click', (e) => {
  const b = e.target.closest('[data-kick]');
  if (b) send({ t: 'kick', seat: Number(b.dataset.kick) });
});
$('btn-quit').onclick = async () => {
  if (race && race.net && !(race.me && race.me.st.finished)) {
    const ok = await A.confirm('Thoát giữa cuộc đua sẽ bị tính về bét và vẫn mất cược. Vẫn thoát?', { title: 'Thoát đua', ok: 'Thoát', cancel: 'Đua tiếp', danger: true });
    if (!ok) return;
    send({ t: 'leave' });
    room = null;
    store(ROOM_KEY, null);
  }
  endRace();
  show(room ? 'room' : 'hall');
  if (room) renderRoom(); else loadRooms();
};
$('btn-res-ok').onclick = () => {
  $('result').classList.add('hidden');
  endRace();
  if (room) { show('room'); renderRoom(); } else { show('hall'); loadRooms(); }
};

// ---------- Khởi động ----------
$('logo').innerHTML = I.logo(28);
I.render();
show('hall');
setInterval(loadRooms, 4000);
A.me().then(
  (u) => {
    if (!u) {
      loadRooms();
      return;
    }
    setUser(u);
    $('user-name').textContent = u.name;
    $('user-avatar').innerHTML = C.head(u);
    loadRooms();
    const want = new URLSearchParams(location.search).get('room') || load(ROOM_KEY);
    if (want) connect(() => send({ t: 'join', code: want.toUpperCase(), token: A.token() }));
    if (new URLSearchParams(location.search).has('practice')) startPractice(new URLSearchParams(location.search).get('practice') || 'neon');
  },
  () => loadRooms(),
);
// Cho script thử nghiệm đọc trạng thái.
window.__race = () => race && { me: race.me && { ...race.me.st, place: race.me.place }, cars: race.cars.length, startAt: race.startAt };
