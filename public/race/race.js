// Trường đua: sảnh, garage, phòng chờ, cuộc đua 3D. Server (games/race) giữ phòng, kiểm gói vị trí, xếp hạng và trả xu;
// máy người chơi tự mô phỏng xe mình (physics.js) và gửi vị trí ≈15 lần/giây.
import { Track, newCar, step, raceDistance, hitCar, ITEM_BOOST_MS, SHIELD_MS, NITRO_CHARGE } from './physics.js';
import { THREE, createRenderer, buildWorld, buildCar, createSparks, createSpeedLines, createItemFx, makeShield, isMoto, PAINT, creditOf, createShowroom } from './scene.js';

const A = window.Account;
const I = window.Icons;
const C = window.Character;
const $ = (id) => document.getElementById(id);
const xu = A.xu;
const esc = A.escapeHtml;

const BETS = [100, 500, 1000, 5000, 10000];
const TRACKS = [{ id: 'neon', name: 'Đường Neon' }, { id: 'city', name: 'Phố Đêm' }, { id: 'serpent', name: 'Đèo Rắn' }, { id: 'canyon', name: 'Hẻm Núi' }];
const MODES = [{ id: 'speed', name: 'Tốc độ' }, { id: 'items', name: 'Đạo cụ' }];
const ITEM_NAMES = { missile: 'Tên lửa', homing: 'Tên lửa đuổi', oil: 'Vũng dầu', shield: 'Khiên', boost: 'Tăng tốc' };
// Biểu tượng đạo cụ là ảnh PNG tự vẽ (public/race/img/item_*.png), không dùng emoji.
const ITEM_ICON = { missile: 'img/item_missile.png', homing: 'img/item_homing.png', oil: 'img/item_oil.png', shield: 'img/item_shield.png', boost: 'img/item_boost.png' };
const ITEM_BOX = 'img/item_box.png';
const itemImg = (k) => `<img src="${ITEM_ICON[k] || ITEM_BOX}" alt="" draggable="false">`;
const PICKUP_RANGE = 5.5; // m: chạy qua hộp gần chừng này thì báo nhặt (server cho tới 6 m)
const SEND_MS = 66; // ≈15 gói mỗi giây
const LERP_MS = 160; // xe người khác vẽ trễ một chút (≈2,5 gói) để mượt dù mạng chập chờn
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
let gFilter = 'car'; // garage: lọc Ô tô / Moto
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
  let mode = 'speed';
  chips($('track-grid'), TRACKS.map((t) => ({ v: t.id, label: t.name })), track, (v) => (track = v));
  chips($('mode-grid'), MODES.map((m) => ({ v: m.id, label: m.name })), mode, (v) => (mode = v));
  chips($('bet-grid'), BETS.map((b) => ({ v: b, label: `${xu(b)} xu` })), bet, (v) => (bet = Number(v)));
  const dlg = $('dlg-create');
  dlg.onclose = () => {
    if (dlg.returnValue !== 'ok') return;
    connect(() => send({ t: 'create', token: A.token(), bet, track, mode }));
  };
  dlg.returnValue = '';
  dlg.showModal();
}

function openPractice() {
  let track = 'neon';
  let mode = 'items';
  chips($('practice-grid'), TRACKS.map((t) => ({ v: t.id, label: t.name })), track, (v) => (track = v));
  chips($('practice-mode'), MODES.map((m) => ({ v: m.id, label: m.name })), mode, (v) => (mode = v));
  const dlg = $('dlg-practice');
  dlg.onclose = () => { if (dlg.returnValue === 'ok') startPractice(track, mode); };
  dlg.returnValue = '';
  dlg.showModal();
}

// ---------- Garage ----------
const curCar = () => (garage ? garage.cars.find((c) => c.id === garage.current) || garage.cars[0] : null);
const specOf = (car, number) => ({ car: car.id, levels: car.levels || {}, cosmetics: car.cosmetics || {}, number });

// Mảnh ghép từ nhiệm vụ nông trại (/api/quests): item → {qty, need, tier}. Lỗi thì bỏ qua (garage vẫn dùng được).
let shards = {};
async function loadShards() {
  try {
    const q = await A.api('/api/quests');
    shards = Object.fromEntries((q.shards || []).map((s) => [s.item, s]));
  } catch (e) {
    shards = {};
  }
}
const shardOf = (id) => shards[id] || null;
const shardFull = (s) => !!s && s.qty >= s.need;
// shardTag: biểu tượng mảnh + số mảnh đang có / cần (món chưa có).
const shardTag = (s) => (s ? `<span class="shard-tag tier-${s.tier}${shardFull(s) ? ' full' : ''}"><img src="/img/quests/shard_${s.tier}.png" alt="">${s.qty}/${s.need}</span>` : '');

async function loadGarage() {
  [garage] = await Promise.all([A.api('/api/race/garage'), loadShards()]);
  if (!gSel || !garage.cars.some((c) => c.id === gSel)) {
    gSel = garage.current || (garage.cars[0] && garage.cars[0].id);
    const c = garage.cars.find((x) => x.id === gSel);
    if (c) gFilter = kindOf(c);
  }
}

let gScene = null;
let gCam = null;
let gCarObj = null;
let gAngle = 0.6;
let gDrag = null;
let gToken = 0;
let gT = 0;

let gRoom = null;
// setupGarageScene dựng phòng trưng bày (scene.js createShowroom) một lần; kéo ngang để xoay xe.
function setupGarageScene() {
  if (gScene) return;
  gRoom = createShowroom(low);
  gScene = gRoom.scene;
  gCam = new THREE.PerspectiveCamera(36, 1, 0.1, 100);
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
  v.setBloom(0.55, 0.35, 0.92); // garage: toả sáng dịu, chỉ neon / cánh kính phát sáng
  renderGarage();
  await showGarageCar();
  let last = performance.now();
  const loop = (now) => {
    const dt = Math.min(0.05, (now - last) / 1000);
    last = now;
    if (gDrag == null) gAngle += dt * 0.3;
    gT += dt;
    const wide = innerWidth > 640;
    const r = wide ? 11.5 : 21; // điện thoại dọc: góc nhìn ngang hẹp nên lùi xa để thấy trọn xe
    // Máy tính: xe đứng giữa khoảng trống giữa cột danh sách xe (trái) và bảng nâng cấp (phải) — dời khung hình bằng
    // setViewOffset để xe không bị tên xe / bảng che; điện thoại: bảng ở dưới nên đẩy xe lên trên.
    gCenter(wide);
    const look = wide ? new THREE.Vector3(0, 0.6, 0) : new THREE.Vector3(0, -0.9, 0);
    gCam.position.set(Math.sin(gAngle) * r + look.x, wide ? 3.0 : 3.6, Math.cos(gAngle) * r + look.z);
    gCam.lookAt(look);
    if (gCarObj) gCarObj.update(0, 0, false, false, dt);
    gRoom.update(gT);
    v.render();
    gLoop = requestAnimationFrame(loop);
  };
  stopGarage();
  gLoop = requestAnimationFrame(loop);
}

// gCenter dời tâm khung hình garage tới giữa vùng trống (máy tính); điện thoại bỏ dời.
function gCenter(wide) {
  const W = innerWidth, H = innerHeight;
  if (!wide) return gCam.view && gCam.clearViewOffset();
  const list = $('g-cars').getBoundingClientRect(), panel = document.querySelector('.garage-panel').getBoundingClientRect();
  const left = list.width ? list.right : 0, right = panel.width ? panel.left : W;
  const d = Math.round((left + right) / 2 - W / 2);
  if (gCam.view && gCam.view.offsetX === -d && gCam.view.fullWidth === W && gCam.view.fullHeight === H) return;
  gCam.setViewOffset(W, H, -d, 0, W, H);
}

const kindOf = (c) => c.kind || (isMoto(c.id) ? 'moto' : 'car');
// needLevel: xe chưa có và nhân vật chưa đủ cấp để mua (0 = mua được).
const needLevel = (c) => (!c.owned && c.level && user && (user.level || 1) < c.level ? c.level : 0);
function renderGarage() {
  const car = garage.cars.find((c) => c.id === gSel);
  const moto = kindOf(car) === 'moto';
  if (moto && gKind === 'spoiler') gKind = 'neon';
  $('g-filter').innerHTML = [['car', 'Ô tô'], ['moto', 'Moto']].map(([k, n]) => `<button data-filter="${k}" class="${k === gFilter ? 'on' : ''}">${n} <small>${garage.cars.filter((c) => kindOf(c) === k).length}</small></button>`).join('');
  $('g-cars').innerHTML = garage.cars.filter((c) => kindOf(c) === gFilter).map((c) => `<button class="car-tab ${c.id === gSel ? 'on' : ''} ${c.exclusive ? 'exclusive' : ''}" data-car="${esc(c.id)}">
      <b>${esc(c.name)}</b><small>${c.owned ? (c.id === garage.current ? 'Đang dùng' : 'Đã có') : c.exclusive ? 'Độc quyền' : needLevel(c) ? `${I.svg('lock')}Cấp ${c.level}` : c.price ? `${xu(c.price)} xu` : 'Miễn phí'}</small>${c.owned ? '' : shardTag(shardOf(c.id))}</button>`).join('');
  $('g-name').innerHTML = esc(car.name) + (car.exclusive ? ' <span class="excl-badge">Độc quyền</span>' : '');
  $('g-credit').textContent = creditOf(car.id);
  $('g-state').textContent = car.owned ? (car.id === garage.current ? 'Đang dùng' : 'Đã có') : car.exclusive ? 'Chưa có' : 'Chưa mua';
  const st = car.stats || BASE_STATS;
  const bar = (v, lo, hi) => `<span class="bar"><span style="width:${Math.round(Math.max(0.08, Math.min(1, (v - lo) / (hi - lo))) * 100)}%"></span></span>`;
  $('g-stats').innerHTML = `
    <div class="stat-row"><span>Tốc độ</span>${bar(st.maxSpeed, 24, 37)}<b>${Math.round(st.maxSpeed * 3.6)}</b></div>
    <div class="stat-row"><span>Tăng tốc</span>${bar(st.accel, 9, 15)}<b>${st.accel.toFixed(1)}</b></div>
    <div class="stat-row"><span>Bám đường</span>${bar(st.grip, 0.55, 1)}<b>${Math.round(st.grip * 100)}</b></div>`;
  // Xe chưa có: mua bằng xu (xe thường) hoặc ghép từ mảnh nhiệm vụ nông trại (mọi xe có mảnh; xe độc quyền chỉ ghép).
  // Ghép không cần đủ cấp nhân vật.
  const sh = car.owned ? null : shardOf(car.id);
  const assemble = sh
    ? `<button class="${car.exclusive ? 'exclusive-btn' : 'assemble-btn'}${shardFull(sh) ? ' ready' : ''}" data-assemble="${esc(car.id)}"${car.exclusive ? ` data-exclusive="car" data-id="${esc(car.id)}"` : ''} ${shardFull(sh) ? '' : 'disabled'} title="Mảnh ghép nhận từ nhiệm vụ ở nông trại">${shardFull(sh) ? 'Ghép xe' : 'Ghép từ mảnh'} ${shardTag(sh)}</button>`
    : car.exclusive && !car.owned ? `<button class="exclusive-btn" data-exclusive="car" data-id="${esc(car.id)}" disabled>Ghép từ mảnh</button>` : '';
  $('g-action').innerHTML = !car.owned && car.exclusive
    ? assemble
    : !car.owned
    ? (needLevel(car)
      ? `<button class="need-level" disabled title="Chưa đủ cấp nhân vật">${I.svg('lock')}Cần cấp ${car.level} — ${xu(car.price)} xu</button>`
      : `<button class="primary" data-act="buy">Mua xe — ${xu(car.price)} xu</button>`) + assemble
    : car.id !== garage.current ? '<button class="primary" data-act="select">Dùng xe này</button>' : '';
  if (sh && !shardFull(sh)) $('g-action').insertAdjacentHTML('beforeend', '<small class="shard-hint">Nhận mảnh từ nhiệm vụ của Ông già Noel ở nông trại</small>');
  // 3 món nâng cấp có chỉ số.
  const max = garage.maxLevel || 5;
  const prices = (garage.upgradePrice || {})[car.id] || {};
  // Phụ kiện nâng cấp (chỉ có từ nhiệm vụ): lên cấp n cần n phụ kiện đúng loại + xu.
  const have = garage.parts || {};
  const info = Object.fromEntries((garage.partInfo || []).map((x) => [x.id, x]));
  const needs = (garage.partNeed || {})[car.id] || {};
  const partImg = (id) => `<img class="part-ic" src="/img/quests/part_${esc(id)}.png" alt="">`;
  const stock = car.owned && garage.partInfo ? `<div class="part-stock">${garage.partInfo.map((x) => `<span title="${esc(x.name)}">${partImg(x.id)}${esc(x.name)} <b>${have[x.id] || 0}</b></span>`).join('')}
      <small>Phụ kiện chỉ có từ nhiệm vụ ở Ông già Noel trong nông trại</small></div>` : '';
  const parts = stock + ['engine', 'turbo', 'tires'].map((p) => {
    const lv = (car.levels || {})[p] || 0;
    const price = prices[p] || 0;
    const pid = (garage.partOf || {})[p];
    const need = needs[p] || 0;
    const pname = (info[pid] || {}).name || 'phụ kiện';
    const short = need && pid && (have[pid] || 0) < need;
    const btn = !car.owned ? '' : lv >= max ? '<span class="chip">Tối đa</span>'
      : short ? `<button class="up-btn need-part" disabled title="Làm nhiệm vụ ở Ông già Noel trong nông trại"><b>${partImg(pid)}Cần ${need} ${esc(pname)}</b><small>(có ${have[pid] || 0}) · ${xu(price)} xu</small></button>`
      : `<button class="up-btn" data-act="up" data-part="${p}"><b>Nâng — ${xu(price)} xu</b>${need && pid ? `<small>${partImg(pid)}${need} ${esc(pname)}</small>` : ''}</button>`;
    return `<div class="part"><b>${PART_NAMES[p][0]}</b>${btn}
      <span class="dots">${Array.from({ length: max }, (_, i) => `<i class="${i < lv ? 'on' : ''}"></i>`).join('')}</span>
      <small>Cấp ${lv}/${max} · tăng ${PART_NAMES[p][1].toLowerCase()} · hình trên xe đổi theo cấp</small></div>`;
  }).join('');
  $('g-action').insertAdjacentHTML('beforeend', parts);
  $('g-tabs').innerHTML = Object.entries(KIND_NAMES).filter(([k]) => !(moto && k === 'spoiler')).map(([k, n]) => `<button data-kind="${k}" class="${k === gKind ? 'on' : ''}">${n}</button>`).join('');
  const on = (car.cosmetics || {})[gKind] || '';
  const items = (garage.items || []).filter((it) => it.kind === gKind);
  $('g-list').innerHTML = (gKind !== 'paint' ? [`<button class="item ${on ? '' : 'on'}" data-item="" data-kind="${gKind}"><b>Không lắp</b><span class="muted">—</span></button>`] : [])
    .concat(items.map((it) => {
      const special = it.id === 'paint_x_chroma' ? 'chroma' : it.id === 'neon_x_rainbow' ? 'rainbow' : '';
      const sw = gKind === 'paint' || gKind === 'neon' ? `<span class="swatch ${special}" style="${special ? '' : `background:#${(PAINT[Object.keys(PAINT).find((c) => it.id.includes(c))] || 0x888888).toString(16).padStart(6, '0')}`}"></span>` : '';
      const locked = it.exclusive && !it.owned;
      const ish = locked ? shardOf(it.id) : null;
      const state = it.id === on ? 'Đang lắp' : locked ? (shardFull(ish) ? 'Ghép ngay' : 'Ghép từ mảnh') + ' ' + shardTag(ish) : it.owned || !it.price ? 'Lắp' : `${xu(it.price)} xu`;
      return `<button class="item ${it.id === on ? 'on' : ''} ${it.exclusive ? 'exclusive' : ''}" data-item="${esc(it.id)}" data-kind="${it.kind}"${locked ? ' data-exclusive="item"' : ''}><b>${sw}${esc(it.name)}</b><span class="muted">${state}</span></button>`;
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

// assembleFromShards ghép xe / đồ từ mảnh (POST /api/quests/assemble) rồi tải lại garage.
async function assembleFromShards(id) {
  const s = shardOf(id);
  const it = (garage.cars || []).find((c) => c.id === id) || (garage.items || []).find((x) => x.id === id);
  const name = it ? it.name : id;
  if (!shardFull(s)) return A.toast(`${name}: có ${s ? s.qty : 0}/${s ? s.need : '?'} mảnh — làm nhiệm vụ ở nông trại để nhận thêm`);
  const ok = await A.confirm(`Dùng ${s.need} mảnh để ghép ${name}?`, { title: 'Ghép từ mảnh', ok: 'Ghép' });
  if (!ok || busy) return;
  busy = true;
  try {
    await A.api('/api/quests/assemble', { method: 'POST', body: { item: id } });
    await loadGarage();
    A.toast(`Đã ghép ${name}!`);
    renderGarage();
    await showGarageCar();
  } catch (e) {
    A.toast(e.message);
  } finally {
    busy = false;
  }
}

function onGarageClick(e) {
  const asm = e.target.closest('[data-assemble]');
  if (asm) return assembleFromShards(asm.dataset.assemble);
  const tab = e.target.closest('[data-car]');
  if (tab) { gSel = tab.dataset.car; renderGarage(); showGarageCar(); return; }
  const fl = e.target.closest('[data-filter]');
  if (fl) {
    gFilter = fl.dataset.filter;
    const first = garage.cars.find((c) => kindOf(c) === gFilter && c.id === garage.current) || garage.cars.find((c) => kindOf(c) === gFilter);
    if (first) gSel = first.id;
    renderGarage();
    showGarageCar();
    return;
  }
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
    const id = item.dataset.item;
    if (item.dataset.exclusive) return assembleFromShards(id); // đồ độc quyền chưa có: ghép từ mảnh (không cần có xe)
    if (!car.owned) return A.toast('Mua xe trước rồi mới lắp đồ');
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
    case 'error':
      // Nhặt hộp trượt (server thấy xe còn xa / hộp vừa bị người khác nhặt) thì im lặng, không làm phiền khi đang đua.
      if (race && race.net && /hộp|đạo cụ|món/i.test(m.msg || '')) {
        if (/đủ/.test(m.msg || '')) flashItems();
        return null;
      }
      return A.toast(m.msg || m.error || 'Lỗi');
    case 'session': if (m.code) store(ROOM_KEY, m.code); return null;
    case 'left':
      store(ROOM_KEY, null);
      room = null;
      if (!race) { show('hall'); loadRooms(); }
      return null;
    case 'items': if (race && race.net) { if ((m.list || []).length > race.items.length) race.stats.got++; race.items = m.list || []; renderItems(); } return null;
    case 'box': if (race && race.net && race.ifx) race.ifx.setBox(m.id, !!m.active); return null;
    case 'fx': if (race && race.net && race.ifx) race.ifx.onFx(m); return null;
    case 'hit': if (race && race.net) onHit(m); return null;
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
  else {
    race.room = r;
    if (race.ifx && r.boxes && r.boxes.length) race.ifx.setBoxes(r.boxes);
  }
}

function renderRoom() {
  const r = room;
  $('room-code').textContent = r.code || '';
  $('room-bet').textContent = `Cược ${xu(r.bet || 0)} xu`;
  $('room-track').textContent = trackName(r.track);
  $('room-mode').textContent = `Chế độ ${r.mode === 'items' ? 'Đạo cụ' : 'Tốc độ'}`;
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
    ? `<select id="sel-track">${(r.tracks && r.tracks.length ? r.tracks : TRACKS).map((t) => `<option value="${esc(t.id)}" ${t.id === r.track ? 'selected' : ''}>${esc(t.name)}</option>`).join('')}</select>
       <select id="sel-mode">${MODES.map((m) => `<option value="${m.id}" ${m.id === (r.mode || 'speed') ? 'selected' : ''}>${m.name}</option>`).join('')}</select>
       <select id="sel-bet">${(r.betOptions && r.betOptions.length ? r.betOptions : BETS).map((b) => `<option value="${b}" ${b === r.bet ? 'selected' : ''}>${b ? `${xu(b)} xu` : 'Chơi vui'}</option>`).join('')}</select>
       <button class="primary" data-h="start">Bắt đầu</button>`
    : '';
  $('room-note').textContent = mySeat < 0 ? 'Bạn đang xem phòng này.' : host ? (r.phase === 'over' ? 'Bấm Bắt đầu để đua ván mới.' : 'Cần ít nhất 2 người chơi (không đua với máy). Ai không đủ xu cược thì không bắt đầu được.') : 'Chờ chủ phòng bắt đầu…';
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
// Dùng đạo cụ: Ctrl / E hoặc nút cảm ứng, chỉ tính lúc vừa bấm.
// Lần bấm được giữ lại tới khung kế tiếp, nên chạm rất nhanh (bấm rồi nhả trong cùng một khung) vẫn được tính.
let itemQueued = false;
function wantItem() {
  const fire = itemQueued;
  itemQueued = false;
  return fire;
}

// newRace dựng cảnh, xe và vòng lặp. entries: [{seat, name, spec, stats, me, bot}].
async function newRace({ trackId, entries, net, code, startAt, mode = 'speed', nitroCharge = NITRO_CHARGE, boxes = null }) {
  $('loading').classList.remove('hidden');
  show('race');
  const v = ensureView();
  const track = await loadTrack(trackId);
  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(72, 1, 0.3, 1500);
  scene.add(camera);
  v.use(scene, camera);
  v.setBloom(); // đường đua đêm: toả sáng mặc định
  await buildWorld(scene, track, low);
  const sparks = createSparks(scene);
  const lines = createSpeedLines(camera);
  const cars = [];
  for (const e of entries) {
    const sp = track.data.spawn[e.seat % track.data.spawn.length];
    const obj = await buildCar(e.spec, low);
    scene.add(obj.root);
    const st = newCar(sp, e.stats || BASE_STATS, nitroCharge);
    const shield = makeShield(obj.length);
    obj.root.add(shield);
    cars.push({ ...e, obj, st, shield, snaps: [], dist: 0, place: 0, done: false, items: [], spinVis: 0 });
  }
  const items = mode === 'items';
  let ifx = null;
  if (items) {
    ifx = createItemFx(scene, sparks);
    ifx.setBoxes(boxes && boxes.length ? boxes : (track.data.itemBoxes || []).map((b, id) => ({ id, x: b.x, z: b.z, active: true })));
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
    net, code, track, scene, camera, cars, me, sparks, lines, mode, ifx, items: [], pending: new Map(), fxSeq: 0, localFx: [], stats: { got: 0, used: 0, given: 0, taken: 0, blocked: 0 },
    startAt: startAt || Date.now() + 3500,
    lastSend: 0, finishMs: 0, ended: false, room: null,
    cam: { x: 0, y: 6, z: 0 },
  };
  drawMinimapBase(track);
  $('hud').classList.toggle('items-mode', items);
  renderItems();
  $('loading').classList.add('hidden');
  $('hud-lap').textContent = `1/${track.laps}`;
  let last = performance.now();
  const loop = (now) => {
    if (!race || race.scene !== scene) return;
    const dt = Math.min(0.05, (now - last) / 1000);
    last = now;
    // Giờ của khung theo đồng hồ Date nhưng lấy từ mốc rAF (cùng đồng hồ với vật lý): callback chạy trễ (máy bận)
    // thì Date.now() đã trôi xa hơn vị trí xe, đóng mốc theo nó làm xe người khác chậm rồi vụt.
    race.frameDate = performance.timeOrigin + now;
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
      c.inp = inp;
      const ev = fixedSteps(c, inp, r.track, dt);
      for (const e of ev) {
        if (e === 'wall') r.sparks.emit(c.st.x, 0.6, c.st.z, 14, 7);
        if (e === 'nitro') banner('NITRO!', 700);
        if (e === 'lap' && !c.st.finished) banner(`Vòng ${c.st.lap + 1}/${r.track.laps}`, 1200);
        if (e === 'finish' && !r.net) localFinish(c);
        if (e === 'finish' && r.net) banner('Về đích!', 2000);
      }
      if (r.ifx && go && !c.st.finished) itemTick(c, now);
      if (r.net && go && now - r.lastSend >= SEND_MS) sendState(c, now);
    } else if (r.net) {
      lerpRemote(c, now, dt);
    } else if (go) {
      const inp = autoSteer(r.track, c.st, c.skill);
      const ev = fixedSteps(c, inp, r.track, dt);
      if (ev.includes('finish')) localFinish(c);
      if (r.ifx && !c.st.finished) botItems(c, now);
    }
    const boost = c.me || !r.net ? c.st.boostMs > 0 || c.st.itemBoostMs > 0 : !!c.st.boost;
    // Bị trúng: xe quay tròn tại chỗ (chỉ để vẽ), khiên phát sáng quanh xe.
    const spinning = c.me || !r.net ? c.st.spinMs > 0 : !!c.st.spin;
    c.spinVis = spinning ? c.spinVis + dt * 13 : c.spinVis % (Math.PI * 2) > 0.05 ? c.spinVis + Math.min(dt * 13, Math.PI * 2 - (c.spinVis % (Math.PI * 2))) : 0;
    c.shield.visible = c.me || !r.net ? c.st.shieldMs > 0 : !!c.st.shield;
    if (c.shield.visible) c.shield.rotation.y += dt * 1.5;
    c.pose = posed(c);
    c.obj.root.position.set(c.pose.x, 0, c.pose.z);
    c.obj.root.rotation.y = c.pose.rot + c.spinVis;
    c.obj.update(c.st.speed, c.me ? c.st.steerVis : 0, c.st.drift, boost, dt);
    if (c.st.drift && Math.abs(c.st.speed) > 12 && Math.random() < 0.5) {
      const bx = c.pose.x - Math.sin(c.pose.rot) * c.obj.length * 0.45; // tia lửa drift theo vị trí đang vẽ
      const bz = c.pose.z - Math.cos(c.pose.rot) * c.obj.length * 0.45;
      r.sparks.emit(bx, 0.15, bz, 2, 3, 0xffb547);
    }
    c.dist = c.done ? Infinity : raceDistance(r.track, c.st.lap, c.st.cp, c.st.x, c.st.z, c.st.seg);
  }
  r.sparks.update(dt);
  if (r.ifx) {
    r.ifx.update(dt);
    if (!r.net) localFxTick(dt, now);
  }
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
    $('hud').classList.toggle('spun', !!r.me && me.st.spinMs > 0);
    $('hud-nitro-k').classList.toggle('ready', me.st.nitro >= 1);
    const t = go ? (r.me && me.st.finished && r.finishMs ? r.finishMs : now - r.startAt) : 0;
    $('hud-time').textContent = fmtTime(t);
    followCam(me, dt);
    r.lines.update(dt, me.st.boostMs > 0 ? 1 : 0, Math.max(10, me.st.speed));
  }
  drawMinimap(order);
}

function sendState(c, now) {
  race.lastSend = now;
  // ms: lúc vị trí này đúng, tính từ xuất phát, đếm theo số bước vật lý đã chạy (simMs) chứ không theo giờ hệ thống:
  // khung bị trễ / dồn (máy bận, dt bị kẹp) thì giờ hệ thống chạy trước vị trí xe → xe người khác chậm rồi vụt.
  // Lệch giờ thật quá 250 ms (tab bị treo lâu) thì đồng bộ lại.
  const wall = (race.frameDate || now) - (c.acc || 0) * 1000 - race.startAt;
  if (c.simMs == null || Math.abs(wall - c.simMs) > 250) c.simMs = wall;
  send({ t: 'state', ms: Math.round(c.simMs), x: round(c.st.x), z: round(c.st.z), rot: round(c.st.rot, 3), speed: round(c.st.speed), lap: c.st.lap, cp: c.st.cp, nitro: c.st.boostMs > 0, drift: c.st.drift,
    steer: round((c.inp && c.inp.steer) || 0, 2), throttle: round((c.inp && c.inp.throttle) || 0, 2) });
}

// fixedSteps chạy vật lý theo bước cố định 1/60 s (tối đa 0,25 s mỗi khung) để tốc độ xe không phụ thuộc FPS.
const STEP = 1 / 60;
// Màn hình 90 / 120 Hz (hoặc 60 Hz lệch nhịp) có khung không chạy bước nào, khung chạy 2 bước → xe đứng rồi nhảy;
// nên vẽ xe ở vị trí nội suy giữa hai bước vật lý gần nhất theo phần thời gian còn dư (c.pose, xem posed).
function fixedSteps(c, inp, track, dt) {
  c.acc = Math.min(0.25, (c.acc || 0) + dt);
  const ev = [];
  while (c.acc >= STEP) {
    c.acc -= STEP;
    c.prev = { x: c.st.x, z: c.st.z, rot: c.st.rot };
    ev.push(...step(c.st, inp, track, STEP));
    if (c.simMs != null) c.simMs += STEP * 1000;
  }
  c.alpha = c.acc / STEP;
  return ev;
}
// posed: vị trí để vẽ / camera bám. Xe tự mô phỏng: nội suy prev → st theo alpha; xe người khác: đã nội suy ở lerpRemote.
function posed(c) {
  const p = c.prev;
  if (!p || c.alpha == null) return { x: c.st.x, z: c.st.z, rot: c.st.rot };
  const a = c.alpha;
  let dr = c.st.rot - p.rot;
  while (dr > Math.PI) dr -= Math.PI * 2;
  while (dr < -Math.PI) dr += Math.PI * 2;
  return { x: p.x + (c.st.x - p.x) * a, z: p.z + (c.st.z - p.z) * a, rot: p.rot + dr * a };
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
  const ps = c.pose || c.st; // bám vị trí đang vẽ để camera và xe không lệch nhịp nhau
  const fx = Math.sin(ps.rot);
  const fz = Math.cos(ps.rot);
  const back = 5.6 + (c.obj.height || 1.5) * 0.9 + Math.max(0, c.st.speed) * 0.03;
  const tx = ps.x - fx * back;
  const tz = ps.z - fz * back;
  const k = 1 - Math.exp(-dt * 8);
  r.cam.x += (tx - r.cam.x) * k;
  r.cam.z += (tz - r.cam.z) * k;
  const h = c.obj.height || 1.5;
  r.cam.y += (h + 1.7 - r.cam.y) * k;
  r.camera.position.set(r.cam.x, r.cam.y, r.cam.z);
  r.camera.lookAt(ps.x + fx * 6, h * 0.55 + 0.6, ps.z + fz * 6);
  const fov = 66 + Math.max(0, c.st.speed) * 0.25 + (c.st.boostMs > 0 ? 8 : 0);
  r.camera.fov += (fov - r.camera.fov) * Math.min(1, dt * 4);
  r.camera.updateProjectionMatrix();
}

// Xe người khác: nội suy theo mốc thời gian server (`at` của từng xe), vẽ trễ LERP_MS.
// Mốc lấy từ server chứ không phải lúc gói tới, nên mạng dồn / trễ gói không làm xe giật; gói trùng (server phát lại
// vị trí cũ khi chưa có gói mới) bị bỏ; hết dữ liệu (mạng dồn gói theo cụm) thì đoán tiếp theo vận tốc tối đa EXTRAP_MS rồi mới đứng.
const EXTRAP_MS = 300;
function onCars(m) {
  // Độ lệch đồng hồ máy − server: tiến về mức nhỏ nhất (tin tới nhanh nhất). Vài giây đầu cuộc đua lấy ngay (tin đầu
  // thường bị xử lý trễ lúc đang dựng cảnh — giữ nó thì mọi xe người khác hiện chậm cả cuộc đua); sau đó hạ 2 ms mỗi tin
  // để thời gian vẽ không nhảy (lệch > 150 ms bất thường thì hạ 10% mỗi tin); nới rất chậm để theo kịp khi đồng hồ trôi.
  if (m.ms) {
    const off = Date.now() - m.ms;
    if (race.clockOff == null || (off < race.clockOff && m.ms - race.startAt + race.clockOff < 3000)) race.clockOff = off;
    else if (off < race.clockOff) race.clockOff -= race.clockOff - off > 150 ? (race.clockOff - off) / 10 : Math.min(2, race.clockOff - off);
    else race.clockOff += 0.02;
  }
  const arrive = Date.now();
  for (const s of m.list || []) {
    const c = race.cars.find((x) => x.seat === s.seat);
    if (!c || c.me) continue;
    const at = s.at || m.ms || arrive - (race.clockOff || 0);
    const last = c.snaps[c.snaps.length - 1];
    if (last && at <= last.at) {
      Object.assign(last, { lap: s.lap, cp: s.cp, shield: s.shield, boost: s.boost, spin: s.spin, nitro: s.nitro });
      continue; // vị trí cũ phát lại
    }
    c.snaps.push({ ...s, at });
    if (c.snaps.length > 24) c.snaps.shift();
    // Độ trễ của mẫu khi tới máy mình (ms, theo đồng hồ server), riêng từng xe, giữ ~2,5 giây gần nhất: chọn độ trễ vẽ.
    if (race.clockOff != null) {
      const lates = (c.lates ||= []);
      lates.push([arrive, arrive - race.clockOff - at]);
      while (lates.length && arrive - lates[0][0] > 2500) lates.shift();
    }
  }
}
// lerpDelay: độ trễ vẽ của xe c — mức trễ 80% của các mẫu ~2,5 giây gần nhất + 10 ms, kẹp 90–300 ms; phần mẫu tới muộn
// hơn thì ngoại suy bằng vật lý xe bù (đo A/B: trễ hiển thị giảm ~45% so với 95% + 30 ms mà độ mượt như cũ). Đổi mượt: tăng ≤ 100 ms/giây, giảm ≤ 50 ms/giây — thời gian
// vẽ của xe đó chỉ chậm / nhanh ≤ 10% / 5%, không nhảy.
function lerpDelay(c, dt) {
  if (c.delay == null) c.delay = LERP_MS;
  const lates = c.lates;
  if (lates && lates.length >= 8) {
    const sorted = lates.map((l) => l[1]).sort((a, b) => a - b);
    const want = Math.max(90, Math.min(300, sorted[Math.floor(sorted.length * 0.8)] + 10));
    c.delay += Math.max(-dt * 50, Math.min(dt * 100, want - c.delay));
  }
  return c.delay;
}
// extrapolate: hết dữ liệu (t vượt gói cuối b) thì chạy tiếp chính vật lý xe (physics.js, cùng chỉ số, bước cố định)
// từ trạng thái gói cuối với điều khiển cuối, tối đa EXTRAP_MS — bám cua / tường như xe thật thay cho đường thẳng.
// Gói cũ (máy khách / server chưa gửi điều khiển) thì trả null để đoán tuyến tính.
function extrapolate(c, b, ms) {
  if (b.throttle == null || b.steer == null) return null;
  let e = c.ext;
  if (!e || e.at !== b.at) {
    const sp = b.speed || 0;
    const st = newCar({ x: b.x, z: b.z, rot: b.rot }, c.stats || BASE_STATS, NITRO_CHARGE);
    Object.assign(st, { speed: sp, vx: Math.sin(b.rot) * sp, vz: Math.cos(b.rot) * sp, drift: !!b.drift, finished: true,
      boostMs: b.nitro || b.boost ? 400 : 0, spinMs: b.spin ? 400 : 0 });
    e = c.ext = { at: b.at, st, sim: 0, prev: { x: b.x, z: b.z, rot: b.rot } };
  }
  const want = Math.min(ms, EXTRAP_MS);
  const inp = { throttle: b.throttle, steer: b.steer, drift: !!b.drift, nitro: false };
  // Chạy tới bước đầu tiên ở sau `want` rồi nội suy lùi giữa hai bước kề nhau (không thì xe chỉ nhích theo từng
  // bước 16,7 ms → khựng / nhảy như lỗi bước cố định).
  while (e.sim < want) {
    e.prev = { x: e.st.x, z: e.st.z, rot: e.st.rot };
    step(e.st, inp, race.track, STEP);
    e.sim += STEP * 1000;
  }
  if (e.sim === 0) return { x: b.x, z: b.z, rot: b.rot };
  const a = Math.max(0, Math.min(1, (want - (e.sim - STEP * 1000)) / (STEP * 1000)));
  const p = e.prev;
  let dr = e.st.rot - p.rot;
  while (dr > Math.PI) dr -= Math.PI * 2;
  while (dr < -Math.PI) dr += Math.PI * 2;
  return { x: p.x + (e.st.x - p.x) * a, z: p.z + (e.st.z - p.z) * a, rot: p.rot + dr * a };
}
const SMOOTH_S = 0.1; // sai số đoán (gói mới khác chỗ đã vẽ) tắt dần theo e^(−dt/τ), không phụ thuộc FPS
function lerpRemote(c, now, dt) {
  const sn = c.snaps;
  if (!sn.length) return;
  const t = now - (race.clockOff || 0) - lerpDelay(c, dt); // thời điểm cần vẽ xe này, theo đồng hồ server
  c.renderT = t;
  let a = sn[0];
  let b = sn[0];
  let k = 0;
  let pose = null;
  const lastSn = sn[sn.length - 1];
  if (t >= lastSn.at) {
    // Hết dữ liệu: đoán tiếp bằng vật lý xe; không có điều khiển thì theo hai gói cuối (tuyến tính).
    b = lastSn;
    a = sn.length > 1 ? sn[sn.length - 2] : b;
    pose = extrapolate(c, b, t - b.at);
    k = b.at === a.at ? 1 : 1 + Math.min(t - b.at, EXTRAP_MS) / (b.at - a.at);
  } else if (t > sn[0].at) {
    for (let i = 0; i < sn.length - 1; i++) if (sn[i + 1].at >= t) { a = sn[i]; b = sn[i + 1]; break; }
    k = (t - a.at) / (b.at - a.at);
  }
  if (!pose) {
    let dr = b.rot - a.rot;
    while (dr > Math.PI) dr -= Math.PI * 2;
    while (dr < -Math.PI) dr += Math.PI * 2;
    pose = { x: a.x + (b.x - a.x) * k, z: a.z + (b.z - a.z) * k, rot: a.rot + dr * k };
  }
  // Làm mượt sai số: giữ độ lệch giữa chỗ đã vẽ và đích mới (khi đích nhảy vì gói mới / đoán sai) rồi cho nó tắt dần;
  // đích chạy liền thì xe bám đúng đích, không trễ. Lệch xa (bị trúng, hồi vị trí) thì nhảy luôn.
  const tg = c.target;
  let ex = 0, ez = 0, er = 0;
  if (tg && c.lerpInit && Math.hypot(pose.x - c.st.x, pose.z - c.st.z) <= 8) {
    const fade = Math.exp(-dt / SMOOTH_S);
    ex = (c.st.x - tg.x) * fade;
    ez = (c.st.z - tg.z) * fade;
    er = c.st.rot - tg.rot;
    while (er > Math.PI) er -= Math.PI * 2;
    while (er < -Math.PI) er += Math.PI * 2;
    er *= fade;
  }
  c.target = pose;
  c.lerpInit = true;
  Object.assign(c.st, { x: pose.x + ex, z: pose.z + ez, rot: pose.rot + er, speed: b.speed || 0, lap: b.lap || 0, cp: b.cp || 0, drift: !!b.drift, boost: !!b.nitro || !!b.boost, spin: !!b.spin, shield: !!b.shield });
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
  if (race.ifx) race.ifx.clearFx();
  $('hud').classList.remove('items-mode', 'spun');
  $('hud-items').innerHTML = '';
  race.scene.traverse((o) => { if (o.geometry) o.geometry.dispose(); });
  race = null;
  $('countdown').classList.add('hidden');
  $('banner').classList.add('hidden');
}

async function startPractice(trackId, mode = 'items') {
  if (race) endRace();
  let car = null;
  if (user) {
    try { await loadGarage(); car = curCar(); } catch (e) { car = null; }
  }
  const mine = car ? specOf(car, 7) : { car: 'starter', levels: {}, cosmetics: { paint: 'paint_red' }, number: 7 };
  // Lái thử chạy một mình (không đua với máy); ?bots=a,b,c (tối đa 5) chỉ để thử mẫu xe trên đường khi phát triển.
  const botIds = (new URLSearchParams(location.search).get('bots') || '').split(',').filter(Boolean).slice(0, 5);
  const bots = botIds.map((id, i) => ({
    seat: i + 1, name: `Máy ${i + 1}`, bot: true, skill: 0.82 + (i % 3) * 0.05,
    spec: { car: id, levels: { engine: i % 3, turbo: i % 3, tires: (i % 3) * 2 }, cosmetics: { paint: ['paint_blue', 'paint_yellow', 'paint_green', 'paint_purple', 'paint_white'][i], neon: ['neon_pink', 'neon_blue', 'neon_green', '', ''][i], spoiler: i === 1 ? 'spoiler_gt' : '' }, number: i + 2 },
    stats: { maxSpeed: 29 + (i % 3), accel: 11.5 + (i % 3) * 0.4, grip: 0.7 },
  }));
  try {
    await newRace({ trackId, net: false, mode, entries: [{ seat: 0, name: user ? user.name : 'Bạn', me: true, spec: mine, stats: (car && car.stats) || BASE_STATS }, ...bots] });
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
    await newRace({ trackId: r.track || 'neon', entries, net: true, code: r.code, startAt: r.startAt ? Date.now() + (r.startAt - (r.now || Date.now())) : 0,
      mode: r.mode || 'speed', boxes: r.boxes, nitroCharge: r.nitroCharge || NITRO_CHARGE });
    race.room = r;
  } catch (e) {
    A.toast(e.message);
    $('loading').classList.add('hidden');
  }
}

// ---------- Đạo cụ ----------
// Mạng: server quyết định hộp, món, đạn và trúng / trượt; client báo "pickup" khi chạy qua hộp và "use" khi bấm.
// Lái thử: mô phỏng gọn ngay trên máy (cùng tốc độ đạn, tầm trúng như server) để quen tay.
const LOCAL = { missileSpeed: 70, missileMs: 3000, homingSpeed: 55, homingMs: 6000, oilMs: 20000, oilSafeMs: 1500, hit: 3, boxOffMs: 3000 };

const nameOfSeat = (seat) => {
  const c = race && race.cars.find((x) => x.seat === seat);
  return c ? (c.me ? 'bạn' : c.name) : 'ai đó';
};

// Đã giữ đủ 2 món: ô đạo cụ nhấp nháy nhẹ thay cho thông báo.
function flashItems() {
  const el = $('hud-items');
  if (!el) return;
  el.classList.remove('flash');
  void el.offsetWidth;
  el.classList.add('flash');
}

function renderItems() {
  const el = $('hud-items');
  if (!race || !race.ifx || !race.me) { el.innerHTML = ''; return; }
  const list = race.net ? race.items : race.me.items;
  el.innerHTML = [0, 1].map((i) => {
    const k = list[i];
    return `<span class="item-slot ${k ? 'full' : 'empty'} ${i === 0 ? 'next' : ''}" title="${k ? ITEM_NAMES[k] : 'Trống'}">${k ? `${itemImg(k)}<small>${ITEM_NAMES[k]}</small>` : `<img class="ghost" src="${ITEM_BOX}" alt="" draggable="false">`}</span>`;
  }).join('');
  // Nút đạo cụ trên điện thoại hiện món sẽ dùng.
  const btn = document.querySelector('.touch .item-btn');
  if (btn) {
    const k = list[0];
    const html = k ? itemImg(k) : `<img class="ghost" src="${ITEM_BOX}" alt="" draggable="false">`;
    if (btn.dataset.k0 !== (k || '')) { btn.dataset.k0 = k || ''; btn.innerHTML = html; }
  }
}

// itemTick: xe mình chạy qua hộp thì nhặt; bấm dùng thì dùng món đầu.
function itemTick(c, now) {
  const r = race;
  for (const [id, b] of r.ifx.boxes) {
    if (!b.active) continue;
    if ((c.st.x - b.x) ** 2 + (c.st.z - b.z) ** 2 > PICKUP_RANGE * PICKUP_RANGE) continue;
    if (r.net) {
      const at = r.pending.get(id) || 0;
      if (now - at < 1200) continue;
      r.pending.set(id, now);
      // Gửi vị trí mới nhất ngay trước khi báo nhặt để server thấy xe đã tới hộp.
      sendState(c, now);
      send({ t: 'pickup', box: id });
    } else {
      localPickup(c, id, b, now);
    }
  }
  if (!wantItem()) return;
  if (r.net) {
    const k = r.items[0];
    if (!k) return;
    send({ t: 'use' });
    r.stats.used++;
    // Khiên và tăng tốc có tác dụng ngay trên máy mình; đạn và dầu chờ server.
    if (k === 'shield') c.st.shieldMs = SHIELD_MS;
    if (k === 'boost') { c.st.itemBoostMs = ITEM_BOOST_MS; banner('TĂNG TỐC!', 700); }
  } else {
    localUse(c, now);
  }
}

function rollLocal(c) {
  const order = [...race.cars].sort((a, b) => b.dist - a.dist);
  const rank = order.indexOf(c) + 1;
  const w = rank <= 1 ? { oil: 45, shield: 35, missile: 15, boost: 5 } : rank >= order.length ? { homing: 35, boost: 35, missile: 20, shield: 10 } : { missile: 35, oil: 20, boost: 20, shield: 15, homing: 10 };
  let x = Math.random() * Object.values(w).reduce((a, b) => a + b, 0);
  for (const [k, v] of Object.entries(w)) { if (x < v) return k; x -= v; }
  return 'missile';
}

function localPickup(c, id, b, now) {
  race.ifx.setBox(id, false);
  setTimeout(() => race && race.ifx && race.ifx.setBox(id, true), LOCAL.boxOffMs);
  if (c.items.length >= 2) return;
  c.items.push(rollLocal(c));
  if (c.me) race.stats.got++;
  c.useAt = now + 1500 + Math.random() * 3000;
  if (c.me) renderItems();
}

function localUse(c, now) {
  const k = c.items.shift();
  if (!k) return;
  if (c.me) renderItems();
  if (k === 'shield') { c.st.shieldMs = SHIELD_MS; return; }
  if (k === 'boost') { c.st.itemBoostMs = ITEM_BOOST_MS; if (c.me) banner('TĂNG TỐC!', 700); return; }
  const t = race.track;
  const near = t.nearest(c.st.x, c.st.z, c.st.seg);
  const s = t.progress(near.i, near.t);
  const f = { id: ++race.fxSeq, kind: k, owner: c, s, born: now, until: now + (k === 'oil' ? LOCAL.oilMs : k === 'homing' ? LOCAL.homingMs : LOCAL.missileMs) };
  if (k === 'oil') {
    const p = t.pointAt(s - 4);
    Object.assign(f, { x: p.x, z: p.z, rot: p.dir });
  } else {
    const p = t.pointAt(s + 3);
    Object.assign(f, { x: p.x, z: p.z, rot: p.dir });
    if (k === 'homing') {
      const order = [...race.cars].filter((x) => !x.done).sort((a, b) => b.dist - a.dist);
      const i = order.indexOf(c);
      f.target = i > 0 ? order[i - 1] : null;
    }
  }
  race.localFx.push(f);
  race.ifx.onFx({ id: f.id, kind: k, x: f.x, z: f.z, rot: f.rot });
}

function localFxTick(dt, now) {
  const r = race;
  for (const f of r.localFx) {
    if (f.dead) continue;
    if (now > f.until) { f.dead = true; continue; }
    if (f.kind === 'missile' || (f.kind === 'homing' && !f.target)) {
      f.s += LOCAL.missileSpeed * dt;
      const p = r.track.pointAt(f.s);
      Object.assign(f, { x: p.x, z: p.z, rot: p.dir });
    } else if (f.kind === 'homing') {
      const tg = f.target.st;
      const dx = tg.x - f.x;
      const dz = tg.z - f.z;
      const d = Math.hypot(dx, dz) || 1;
      const step = Math.min(d, LOCAL.homingSpeed * dt);
      f.x += (dx / d) * step;
      f.z += (dz / d) * step;
      f.rot = Math.atan2(dx, dz);
    }
    for (const c of r.cars) {
      if (c.done) continue;
      if (c === f.owner && (f.kind !== 'oil' || now - f.born < LOCAL.oilSafeMs)) continue;
      if ((c.st.x - f.x) ** 2 + (c.st.z - f.z) ** 2 > LOCAL.hit * LOCAL.hit) continue;
      f.dead = true;
      onHit({ seat: c.seat, by: f.owner.seat, kind: f.kind, blocked: c.st.shieldMs > 0 });
      break;
    }
    if (!f.dead && f.kind !== 'oil') r.ifx.onFx({ id: f.id, kind: f.kind, x: f.x, z: f.z, rot: f.rot });
  }
  for (const f of r.localFx) if (f.dead && !f.gone) { f.gone = true; r.ifx.onFx({ id: f.id, dead: true }); }
  r.localFx = r.localFx.filter((f) => !f.gone);
}

// botItems (lái thử): máy nhặt hộp và dùng món sau vài giây.
function botItems(c, now) {
  for (const [id, b] of race.ifx.boxes) {
    if (b.active && (c.st.x - b.x) ** 2 + (c.st.z - b.z) ** 2 <= PICKUP_RANGE * PICKUP_RANGE) localPickup(c, id, b, now);
  }
  if (c.items.length && now >= (c.useAt || 0)) {
    localUse(c, now);
    c.useAt = now + 2500 + Math.random() * 3000;
  }
}

// onHit: xe ở ghế seat bị món kind của ghế by trúng (blocked: khiên chặn).
function onHit(m) {
  const r = race;
  const c = r.cars.find((x) => x.seat === m.seat);
  if (!c) return;
  const x = c.st.x;
  const z = c.st.z;
  const byMe = r.me && m.by === r.me.seat;
  if (c.me) m.blocked ? r.stats.blocked++ : r.stats.taken++;
  if (byMe && !m.blocked && !c.me) r.stats.given++;
  if (m.blocked) {
    r.sparks.emit(x, 1, z, 26, 8, 0x58f0ff);
    c.st.shieldMs = 0;
    if (c.me) banner(`Khiên chặn ${ITEM_NAMES[m.kind] || 'đòn'} của ${nameOfSeat(m.by)}`, 1600);
    else if (byMe) banner(`${c.name} có khiên chặn`, 1200);
    return;
  }
  r.sparks.emit(x, 0.9, z, 36, 10, 0xff8a2a);
  if (c.me || !r.net) hitCar(c.st);
  if (c.me) banner(`Bị trúng ${(ITEM_NAMES[m.kind] || 'đạo cụ').toLowerCase()} của ${nameOfSeat(m.by)}!`, 1800);
  else if (byMe) banner(`Trúng ${c.name}!`, 1200);
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
  if (!e.repeat && ['KeyE', 'ControlLeft', 'ControlRight'].includes(e.code)) itemQueued = true;
  keys.add(e.code);
});
addEventListener('keyup', (e) => keys.delete(e.code));
addEventListener('blur', () => { keys.clear(); touchKeys.clear(); });
for (const b of document.querySelectorAll('#touch button')) {
  const k = b.dataset.k;
  const on = (e) => { e.preventDefault(); touchKeys.add(k); b.classList.add('down'); if (k === 'item') itemQueued = true; };
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
  if (e.target.id === 'sel-mode') send({ t: 'setMode', mode: e.target.value });
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
window.__raceCam = () => race && [race.camera.position.x, race.camera.position.y, race.camera.position.z];
// __raceCars: vị trí đang vẽ của từng xe (đo độ mượt / đủ xe lúc xuất phát trong test trình duyệt).
window.__raceCars = () => race && race.cars.map((c) => ({ rt: c.renderT, off: race.clockOff, seat: c.seat, me: !!c.me, bot: !!c.bot, x: c.obj.root.position.x, z: c.obj.root.position.z, vis: c.obj.root.visible !== false && !!c.obj.root.parent, snaps: c.snaps.length, delay: c.delay }));
window.__race = () => race && { me: race.me && { ...race.me.st, place: race.me.place }, cars: race.cars.length, startAt: race.startAt, mode: race.mode,
  items: race.net ? race.items : race.me ? race.me.items : [], stats: race.stats, fx: race.ifx ? race.ifx.fx.size : 0, boxes: race.ifx ? [...race.ifx.boxes.values()].filter((b) => b.active).length : 0 };
