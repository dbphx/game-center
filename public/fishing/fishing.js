// Hồ câu cá DCity (khu ngoại ô): nhân vật đi ra đầu bến / bờ hồ, chọn mồi, quăng cần. Server quyết định giờ cá cắn
// (biteAt), khung giật (windowMs), độ khó và con cá; trang này vẽ phao, đếm giờ, chơi mini-game kéo cần rồi gửi kết quả.
// Giật trước biteAt = "giật sớm", quá khung = cá chạy mất. Cá vào giỏ (tối đa 30), bán lấy xu; sổ tay ghi kỷ lục cm.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const esc = A.escapeHtml;
  const xu = A.xu;

  const IMG = '/img/toon/fishing/';
  const W = 1400;
  const H = 760;
  const SPRITE = 112;
  const SPEED = 240;
  const lookOf = (u) => ({ gender: u.gender, outfit: u.outfit, hair: u.hair, wings: u.wings, face: u.face });

  const RARITY = {
    junk: { name: 'Rác', color: '#8d8d8d' },
    common: { name: 'Thường', color: '#9aa3ad' },
    uncommon: { name: 'Ít gặp', color: '#3aa94a' },
    rare: { name: 'Hiếm', color: '#2f86d6' },
    epic: { name: 'Sử thi', color: '#9b4dd6' },
    legendary: { name: 'Huyền thoại', color: '#e0a800' },
  };
  const fishImg = (id) => `${IMG}fish_${id}.png`;
  const baitImg = (id) => `${IMG}bait_${id}.png`;

  // ---------- Bố cục cảnh ----------
  // Mặt hồ: elip tâm (840, 477), bán trục 515 × 195. Bến gỗ chìa vào hồ từ bờ dưới.
  const LAKE = { cx: 840, cy: 477, rx: 520, ry: 198 };
  const DOCK = { x0: 668, x1: 852, y0: 486, y1: 752 };
  const OBJ = [
    { src: IMG + 'lake.png', x: 840, y: 692, h: 426, ground: true },
    { src: IMG + 'dock.png', x: 760, y: 752, h: 280, ground: true },
    { src: IMG + 'bamboo_bridge.png', x: 470, y: 345, h: 130, ground: true },
    { src: IMG + 'lilypad.png', x: 1100, y: 470, h: 30, ground: true },
    { src: IMG + 'lilypad.png', x: 520, y: 600, h: 26, ground: true },
    { src: IMG + 'lilypad.png', x: 1190, y: 600, h: 30, ground: true },
    { src: IMG + 'lilypad.png', x: 960, y: 640, h: 24, ground: true },
    { src: IMG + 'lotus.png', x: 990, y: 560, h: 66, ground: true },
    { src: IMG + 'lotus.png', x: 610, y: 430, h: 56, ground: true },
    { src: IMG + 'lotus.png', x: 1240, y: 420, h: 52, ground: true },
    { src: IMG + 'hyacinth.png', x: 1270, y: 530, h: 56, ground: true },
    { src: IMG + 'hyacinth.png', x: 430, y: 640, h: 48, ground: true },
    { src: IMG + 'willow.png', x: 170, y: 430, h: 320, foot: [30, 14], sway: true },
    { src: IMG + 'willow.png', x: 1345, y: 330, h: 250, foot: [26, 12], sway: true },
    { src: '/img/toon/farm/tree_tre.png', x: 60, y: 610, h: 220, foot: [24, 10], sway: true },
    { src: '/img/toon/farm/tree_tre.png', x: 1365, y: 745, h: 200, foot: [24, 10], sway: true },
    { src: '/img/toon/city/tree_phuong.png', x: 1180, y: 300, h: 200, foot: [20, 8], sway: true },
    { src: IMG + 'lantern.png', x: 652, y: 748, h: 150, foot: [10, 8] },
    { src: IMG + 'lantern.png', x: 868, y: 748, h: 150, foot: [10, 8] },
    { src: IMG + 'lantern.png', x: 255, y: 575, h: 140, foot: [10, 8] },
    { src: '/img/toon/city/bench.png', x: 130, y: 745, h: 56, foot: [50, 12] },
    { key: 'basket', src: IMG + 'basket.png', x: 935, y: 735, h: 62, foot: [34, 14], label: 'Giỏ cá' },
    { src: '/img/toon/farm/flower_pink.png', x: 330, y: 720, h: 30, ground: true },
    { src: '/img/toon/farm/flower_yellow.png', x: 1060, y: 735, h: 30, ground: true },
    { src: '/img/toon/farm/flower_purple.png', x: 1240, y: 720, h: 30, ground: true },
    { src: '/img/toon/farm/flower_red.png', x: 220, y: 680, h: 28, ground: true },
    { src: '/img/toon/farm/flower_white.png', x: 520, y: 740, h: 28, ground: true },
  ];
  // Chỗ câu: đứng ở đây quăng cần ra mặt nước phía trước.
  const SPOTS = {
    dock: { x: 760, y: 505, dir: 'up', tx: [660, 860], ty: [292, 345], label: 'Đầu bến' },
    bank: { x: 300, y: 505, dir: 'right', tx: [460, 560], ty: [430, 500], label: 'Bờ hồ' },
  };
  const STATIONS = {
    ...SPOTS,
    basket: { x: 935, y: 752, label: 'Giỏ cá' },
    exit: { x: 34, y: 700, label: 'Về thành phố' },
  };

  // ---------- Trạng thái ----------
  let user = null;
  let st = null; // trạng thái hồ câu (baits, basket, record, cast, fish)
  let offset = 0;
  const nowMs = () => Date.now() + offset;
  const me = { x: 220, y: 650, dir: 'right', walking: false, look: {}, drawn: '' };
  const keys = new Set();
  let target = null;
  let near = null;
  let bait = 'worm';
  try { bait = localStorage.getItem('fishing.bait') || 'worm'; } catch (e) { /* bỏ qua */ }
  let view = { s: 1, ox: 0, oy: 0 };
  // Lần câu đang diễn ra: { cast, spot, phase: 'fly'|'wait'|'bite'|'reel'|'done', bx, by }
  let fishing = null;

  async function call(path, body, quiet) {
    try {
      const data = await A.api(path, body === undefined ? {} : { method: 'POST', body });
      if (data.user) user = data.user;
      if (data.baits) {
        st = data;
        if (data.now) offset = data.now - Date.now();
      }
      renderHUD();
      renderBar();
      return data;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      if (!quiet) {
        const hp = e.status === 423 || (e.status === 403 && /HP/.test(e.message));
        A.toast(hp ? `${e.message} — ghé Bệnh viện trên bản đồ để hồi máu` : e.message);
      }
      return null;
    }
  }

  // ---------- HUD ----------
  function renderHUD() {
    if (!user) return;
    $('balance').textContent = xu(user.balance);
    $('user-name').textContent = user.name;
    $('user-avatar').innerHTML = C.head(lookOf(user));
    const pct = Math.min(100, Math.round(((user.xp - user.levelXP) / Math.max(1, user.nextXP - user.levelXP)) * 100));
    $('level-badge').innerHTML = `<span class="lv">Lv ${user.level}</span><span class="xp-bar"><span style="width:${pct}%"></span></span>`;
    if (user.maxHp) {
      const hpPct = Math.round((user.hp / user.maxHp) * 100);
      const pill = $('hp-pill');
      pill.className = `hp-pill${hpPct < 40 ? ' low' : ''}`;
      pill.innerHTML = `${I.svg('heart')}<span class="hp-bar"><span style="width:${hpPct}%"></span></span><b>${user.hp}/${user.maxHp}</b>`;
    }
    $('me-tag').innerHTML = `<span class="lv">Lv ${user.level}</span>${esc(user.name)}`;
    const look = lookOf(user);
    if (JSON.stringify(look) !== JSON.stringify(me.look)) {
      me.look = look;
      me.drawn = '';
      drawMe();
    }
  }
  const baitOf = (id) => (st && st.baits.find((b) => b.id === id)) || null;
  function renderBar() {
    if (!st) return;
    if (!baitOf(bait)) bait = (st.baits[0] && st.baits[0].id) || 'worm';
    const b = baitOf(bait);
    $('bait-img').src = baitImg(bait);
    $('bait-text').innerHTML = b ? `${esc(b.name)} <b>×${b.qty}</b>` : 'Mồi';
    $('basket-text').innerHTML = `Giỏ <b>${st.basket.length}/${st.basketMax || 30}</b>`;
    $('btn-cast').disabled = !!fishing;
    $('btn-cast').querySelector('span').textContent = fishing ? 'Đang câu…' : 'Quăng cần';
  }

  // ---------- Cảnh ----------
  function renderWorld() {
    // Dải trời + đồi xa phía trên (ảnh lặp ngang).
    const sky = [0, 700].map((x) => `<img class="toon sky" src="/img/toon/farm/bg_sky_hills.png" alt="" style="left:${x}px" draggable="false">`).join('');
    $('objects').innerHTML = sky + OBJ.map((o, i) => `<div class="obj${o.key ? ' click' : ''}${o.sway ? ' sway' : ''}${o.ground ? ' ground' : ''}" data-i="${i}"${o.key ? ` data-key="${o.key}"` : ''}
        style="left:${o.x}px;top:${o.y}px;z-index:${o.ground ? 1 + i : o.y}">${o.label ? `<span class="label">${o.label}</span>` : ''}<img class="toon" src="${o.src}" alt="" style="height:${o.h}px" draggable="false"></div>`).join('');
  }

  function drawMe() {
    const el = $('me');
    el.style.left = `${Math.round(me.x)}px`;
    el.style.top = `${Math.round(me.y)}px`;
    el.style.zIndex = Math.round(me.y) + 1;
    const rod = fishing ? ` rod-${me.dir}` : '';
    const key = `${me.dir}|${me.walking}|${rod}|${JSON.stringify(me.look)}`;
    if (key === me.drawn) return;
    me.drawn = key;
    $('me-body').className = `me-body${rod}`;
    $('me-body').innerHTML = `${C.sprite(me.look, { dir: me.dir, walking: me.walking, size: SPRITE })}${fishing ? `<img class="rod" src="${IMG}rod.png" alt="" draggable="false">` : ''}`;
    updateFade();
  }
  function updateFade() {
    document.querySelectorAll('#objects .obj:not(.ground)').forEach((el) => {
      const o = OBJ[el.dataset.i];
      const cover = o.y > me.y && Math.abs(o.x - me.x) < o.h * 0.4 && o.y - o.h < me.y - SPRITE * 0.4 && o.h > 90;
      el.classList.toggle('fade', cover);
    });
  }

  // ---------- Đi lại ----------
  const inWater = (x, y) => ((x - LAKE.cx) / LAKE.rx) ** 2 + ((y - LAKE.cy) / LAKE.ry) ** 2 < 1;
  const onDock = (x, y) => x > DOCK.x0 && x < DOCK.x1 && y > DOCK.y0 && y < DOCK.y1;
  function blocked(x, y) {
    if (x < 20 || x > W - 20 || y < 300 || y > H - 6) return true;
    if (inWater(x, y) && !onDock(x, y)) return true;
    return OBJ.some((o) => o.foot && Math.abs(x - o.x) < o.foot[0] && y > o.y - o.foot[1] && y < o.y + 6);
  }
  function stepMove(dt) {
    let dx = 0;
    let dy = 0;
    if (keys.has('left')) dx -= 1;
    if (keys.has('right')) dx += 1;
    if (keys.has('up')) dy -= 1;
    if (keys.has('down')) dy += 1;
    if (dx || dy) target = null;
    else if (target) {
      while (target.path && target.path.length > 1 && Math.hypot(target.path[0][0] - me.x, target.path[0][1] - me.y) < 6) target.path.shift();
      const wp = target.path && target.path.length > 1 ? target.path[0] : [target.x, target.y];
      const tx = wp[0] - me.x;
      const ty = wp[1] - me.y;
      const d = Math.hypot(tx, ty);
      if (d < 5) return arrive();
      dx = tx / d;
      dy = ty / d;
    }
    const moving = !!(dx || dy);
    if (moving) {
      const len = Math.hypot(dx, dy);
      const vx = (dx / len) * SPEED * dt;
      const vy = (dy / len) * SPEED * dt;
      let moved = false;
      if (!blocked(me.x + vx, me.y + vy)) { me.x += vx; me.y += vy; moved = true; }
      else if (!blocked(me.x + vx, me.y)) { me.x += vx; moved = Math.abs(vx) > 0.01; }
      else if (!blocked(me.x, me.y + vy)) { me.y += vy; moved = Math.abs(vy) > 0.01; }
      if (!moved && target && (target.stuck = (target.stuck || 0) + dt) > 0.4) return arrive(Math.hypot(target.x - me.x, target.y - me.y) < 140);
      me.dir = Math.abs(dx) > Math.abs(dy) ? (dx < 0 ? 'left' : 'right') : dy < 0 ? 'up' : 'down';
    }
    if (moving || moving !== me.walking) {
      me.walking = moving;
      drawMe();
    }
  }
  function arrive(ok = true) {
    const then = ok && target ? target.then : null;
    target = null;
    $('go-mark').classList.add('hidden');
    me.walking = false;
    drawMe();
    if (then) then();
  }
  // Tìm đường trên lưới 16px (BFS 8 hướng, không cắt góc vật cản), rồi làm mượt bằng đường thẳng đi được.
  const clearLine = (a, b) => {
    const n = Math.ceil(Math.hypot(b[0] - a[0], b[1] - a[1]) / 6);
    for (let i = 1; i <= n; i++) if (blocked(a[0] + ((b[0] - a[0]) * i) / n, a[1] + ((b[1] - a[1]) * i) / n)) return false;
    return true;
  };
  function findPath(x0, y0, x1, y1) {
    const G = 16;
    const cols = Math.ceil(W / G);
    const rows = Math.ceil(H / G);
    const free = (c, r) => c >= 0 && r >= 0 && c < cols && r < rows && !blocked(c * G + G / 2, r * G + G / 2);
    const key = (c, r) => r * cols + c;
    const sc = Math.floor(x0 / G);
    const sr = Math.floor(y0 / G);
    let tc = Math.floor(x1 / G);
    let tr = Math.floor(y1 / G);
    let exact = !blocked(x1, y1);
    if (!free(tc, tr)) {
      let best = null;
      let bd = Infinity;
      for (let r = 0; r < rows; r++) for (let c = 0; c < cols; c++) {
        const d = (c - tc) ** 2 + (r - tr) ** 2;
        if (d < bd && free(c, r)) { bd = d; best = [c, r]; }
      }
      if (!best) return null;
      [tc, tr] = best;
      exact = false;
    }
    const prev = new Map([[key(sc, sr), -1]]);
    const q = [[sc, sr]];
    for (let h = 0; h < q.length; h++) {
      const [c, r] = q[h];
      if (c === tc && r === tr) break;
      for (const [dc, dr] of [[1, 0], [-1, 0], [0, 1], [0, -1], [1, 1], [1, -1], [-1, 1], [-1, -1]]) {
        const nc = c + dc;
        const nr = r + dr;
        const k = key(nc, nr);
        if (prev.has(k) || !free(nc, nr) || (dc && dr && (!free(c + dc, r) || !free(c, r + dr)))) continue;
        prev.set(k, key(c, r));
        q.push([nc, nr]);
      }
    }
    if (!prev.has(key(tc, tr))) return null;
    const cells = [];
    for (let k = key(tc, tr); k !== -1; k = prev.get(k)) cells.push([(k % cols) * G + G / 2, Math.floor(k / cols) * G + G / 2]);
    cells.reverse();
    if (exact) cells.push([x1, y1]);
    const out = [];
    let from = [x0, y0];
    let i = 0;
    while (i < cells.length) {
      let j = cells.length - 1;
      while (j > i && !clearLine(from, cells[j])) j--;
      out.push(cells[j]);
      from = cells[j];
      i = j + 1;
    }
    return out;
  }

  function walkTo(x, y, then) {
    // Bấm vào nước thì đi tới mép bờ gần nhất theo đường thẳng (dừng khi bị chặn).
    target = { x: Math.max(26, Math.min(W - 26, x)), y: Math.max(306, Math.min(H - 8, y)), then };
    const path = findPath(me.x, me.y, target.x, target.y);
    if (path && path.length) {
      target.path = path;
      [target.x, target.y] = path[path.length - 1];
    }
    const m = $('go-mark');
    m.style.left = `${target.x}px`;
    m.style.top = `${target.y}px`;
    m.classList.remove('hidden');
  }
  // Đi tới đường dẫn qua điểm trung gian (vd ra đầu bến: đi tới chân bến rồi đi thẳng lên).
  function goStation(key, then) {
    const s = STATIONS[key];
    const finish = () => {
      if (SPOTS[key]) {
        me.dir = SPOTS[key].dir;
        drawMe();
      }
      if (then) then();
    };
    if (key === 'dock' && !onDock(me.x, me.y)) walkTo(760, 740, () => walkTo(s.x, s.y, finish));
    else walkTo(s.x, s.y, finish);
  }

  function updateNear() {
    let best = null;
    let bd = 80;
    for (const [k, s] of Object.entries(STATIONS)) {
      const d = Math.hypot(s.x - me.x, s.y - me.y);
      if (d < bd) { bd = d; best = k; }
    }
    near = best;
    const hint = $('hint');
    if (!near || document.querySelector('dialog[open]') || (fishing && fishing.phase !== 'done')) {
      if (fishing && (fishing.phase === 'wait' || fishing.phase === 'bite')) {
        hint.querySelector('span').textContent = fishing.phase === 'bite' ? 'GIẬT!' : 'Giật cần';
        hint.querySelector('kbd').textContent = 'Space';
        hint.classList.toggle('alarm', fishing.phase === 'bite');
        hint.style.left = `${me.x}px`;
        hint.style.top = `${me.y - SPRITE - 40}px`;
        return hint.classList.remove('hidden');
      }
      return hint.classList.add('hidden');
    }
    hint.classList.remove('alarm');
    hint.querySelector('kbd').textContent = 'E';
    hint.querySelector('span').textContent = SPOTS[near] ? `Quăng cần (${SPOTS[near].label})` : STATIONS[near].label;
    hint.style.left = `${me.x}px`;
    hint.style.top = `${me.y - SPRITE - 40}px`;
    hint.classList.remove('hidden');
  }
  function act(key) {
    if (key === 'exit') return (location.href = '/');
    if (key === 'basket') return openBasket();
    if (SPOTS[key]) {
      me.dir = SPOTS[key].dir;
      drawMe();
      return cast(key);
    }
  }

  // ---------- Khung nhìn ----------
  function layout() {
    const stage = $('stage');
    const vw = stage.clientWidth;
    const vh = stage.clientHeight;
    let s = Math.min(vw / W, vh / H);
    if (vw < 760) s = vh / H; // điện thoại: vừa chiều cao, cuộn ngang theo nhân vật
    view.s = s;
    follow(vw, vh);
  }
  function follow(vw = $('stage').clientWidth, vh = $('stage').clientHeight) {
    const s = view.s;
    const ww = W * s;
    view.ox = ww <= vw ? (vw - ww) / 2 : -Math.max(0, Math.min(ww - vw, me.x * s - vw / 2));
    view.oy = 0;
    $('world').style.transform = `translate(${view.ox}px, ${view.oy}px) scale(${s})`;
  }
  const toWorld = (cx, cy) => {
    const r = $('stage').getBoundingClientRect();
    return [(cx - r.left - view.ox) / view.s, (cy - r.top - view.oy) / view.s];
  };

  // ---------- Câu cá ----------
  function status(text, cls = '') {
    const el = $('status');
    el.className = `status ${cls}`;
    el.innerHTML = text;
    el.classList.toggle('hidden', !text);
  }

  async function cast(spotKey) {
    if (fishing) return;
    const spot = SPOTS[spotKey] || SPOTS[nearestSpot()];
    if (!spot) return;
    const b = baitOf(bait);
    if (!b || b.qty <= 0) {
      A.toast(`Hết mồi ${b ? b.name : ''} — mua thêm mồi nhé`);
      return openBait();
    }
    if (st.basket.length >= (st.basketMax || 30)) {
      A.toast('Giỏ cá đầy — bán bớt cá đã nhé');
      return openBasket();
    }
    const res = await call('/api/fishing/cast', { bait });
    if (!res || !res.cast) return;
    startCast(res.cast, spot);
  }
  function nearestSpot() {
    let best = 'dock';
    let bd = Infinity;
    for (const [k, s] of Object.entries(SPOTS)) {
      const d = Math.hypot(s.x - me.x, s.y - me.y);
      if (d < bd) { bd = d; best = k; }
    }
    return best;
  }

  // Bắt đầu hiển thị một lần thả câu (sau khi quăng hoặc khi tải lại trang giữa chừng).
  function startCast(c, spot) {
    const r = (a) => a[0] + Math.random() * (a[1] - a[0]);
    fishing = { cast: c, spot, phase: 'fly', bx: r(spot.tx), by: r(spot.ty), t0: performance.now() };
    me.dir = spot.dir;
    me.drawn = '';
    drawMe();
    renderBar();
    const bob = $('bobber');
    // phao bay từ ngọn cần ra mặt nước
    const [rx, ry] = rodTip();
    bob.style.transition = 'none';
    bob.style.left = `${rx}px`;
    bob.style.top = `${ry}px`;
    bob.className = 'bobber';
    void bob.offsetWidth;
    bob.style.transition = 'left .7s cubic-bezier(.3,.6,.4,1), top .7s cubic-bezier(.2,-0.6,.6,1)';
    bob.style.left = `${fishing.bx}px`;
    bob.style.top = `${fishing.by}px`;
    status('Quăng cần… chờ cá cắn câu. <small>Đừng giật sớm!</small>');
    setTimeout(() => {
      if (!fishing || fishing.phase !== 'fly') return;
      fishing.phase = 'wait';
      bob.classList.add('floating');
      splash(fishing.bx, fishing.by);
    }, 720);
  }
  function rodTip() {
    const dx = { up: 34, right: 58, left: -58, down: 30 }[me.dir] || 30;
    return [me.x + dx, me.y - SPRITE * 0.95];
  }
  function splash(x, y) {
    const rp = $('ripple');
    rp.style.left = `${x}px`;
    rp.style.top = `${y + 10}px`;
    rp.classList.remove('hidden', 'go');
    void rp.offsetWidth;
    rp.classList.add('go');
  }

  function tickFishing() {
    if (!fishing) return;
    const f = fishing;
    const c = f.cast;
    const now = nowMs();
    const bob = $('bobber');
    // Bóng cá lượn lại gần phao trước giờ cắn câu.
    const sh = $('fish-shadow');
    if (f.phase === 'wait' || f.phase === 'bite') {
      const left = c.biteAt - now;
      const k = Math.max(0, Math.min(1, 1 - left / 3500));
      const ang = now / 700;
      sh.style.left = `${f.bx + Math.cos(ang) * (90 - 70 * k)}px`;
      sh.style.top = `${f.by + 16 + Math.sin(ang) * (30 - 22 * k)}px`;
      sh.style.transform = `translate(-50%, -50%) scaleX(${Math.cos(ang + 1.57) > 0 ? -1 : 1})`;
      sh.style.opacity = String(0.15 + 0.45 * k);
      sh.classList.remove('hidden');
    } else sh.classList.add('hidden');
    if (f.phase === 'wait' && now >= c.biteAt) {
      f.phase = 'bite';
      bob.classList.add('bite');
      $('bang').classList.remove('hidden');
      splash(f.bx, f.by);
      status('<b>Cá cắn câu! GIẬT NGAY!</b> <kbd>Space</kbd>', 'alarm');
    }
    if (f.phase === 'bite' && now > c.biteAt + (c.windowMs || 1500)) {
      // Chậm tay: gửi thất bại để đóng lần thả câu (mồi đã mất).
      f.phase = 'done';
      reel(false, 'Chậm tay quá, cá đã ăn mồi rồi chạy mất!');
    }
    drawLine();
  }

  // Dây câu từ ngọn cần tới phao (nét vẽ giao diện).
  function drawLine() {
    const cv = $('line-cv');
    const ctx = cv.getContext('2d');
    ctx.clearRect(0, 0, W, H);
    if (!fishing) return;
    const [rx, ry] = rodTip();
    const bob = $('bobber');
    const bx = parseFloat(bob.style.left) || fishing.bx;
    const by = parseFloat(bob.style.top) || fishing.by;
    ctx.strokeStyle = 'rgba(255, 255, 255, .85)';
    ctx.lineWidth = 1.6;
    ctx.beginPath();
    ctx.moveTo(rx, ry);
    const sag = fishing.phase === 'reel' ? 6 : 40;
    ctx.quadraticCurveTo((rx + bx) / 2, Math.max(ry, by) + sag, bx, by - 14);
    ctx.stroke();
  }

  // Người chơi giật cần (Space / E / bấm).
  function strike() {
    const f = fishing;
    if (!f) return;
    if (f.phase === 'fly' || f.phase === 'wait') {
      f.phase = 'done';
      return reel(true, null); // server trả "giật sớm"
    }
    if (f.phase === 'bite') {
      f.phase = 'reel';
      $('bang').classList.add('hidden');
      $('bobber').classList.remove('bite');
      status('');
      startReelGame(f);
    }
  }

  // Mini-game kéo cần: giữ để nâng vùng xanh, giữ con cá trong vùng tới khi đầy thanh.
  const reelIn = { hold: false };
  function startReelGame(f) {
    const d = Math.max(1, Math.min(5, f.cast.difficulty || 1));
    const g = {
      zoneH: [0.38, 0.33, 0.28, 0.24, 0.2][d - 1],
      zone: 0.3, zv: 0,
      fishY: 0.5, fishT: 0.5, fishSpeed: 0.32 + 0.14 * d, retarget: 0,
      prog: 0.3, t: 0, last: performance.now(), d,
    };
    f.game = g;
    $('reel').classList.remove('hidden');
    $('reel-zone').style.height = `${g.zoneH * 100}%`;
    const loop = (ts) => {
      if (fishing !== f || f.phase !== 'reel') return;
      const dt = Math.min(0.05, (ts - g.last) / 1000);
      g.last = ts;
      g.t += dt;
      // cá đổi hướng ngẫu nhiên, càng khó càng lắt léo
      if ((g.retarget -= dt) <= 0) {
        g.fishT = 0.08 + Math.random() * 0.84;
        g.retarget = (1.3 - 0.18 * d) * (0.5 + Math.random());
      }
      const dir = Math.sign(g.fishT - g.fishY);
      g.fishY += dir * Math.min(Math.abs(g.fishT - g.fishY), g.fishSpeed * dt);
      // vùng xanh: giữ thì nổi lên, thả thì chìm
      g.zv += (reelIn.hold ? 2.2 : -2.0) * dt;
      g.zv *= Math.pow(0.3, dt);
      g.zone = Math.max(0, Math.min(1 - g.zoneH, g.zone + g.zv * dt));
      if (g.zone === 0 || g.zone === 1 - g.zoneH) g.zv *= -0.3;
      const inside = g.fishY >= g.zone && g.fishY <= g.zone + g.zoneH;
      g.prog = Math.max(0, Math.min(1, g.prog + (inside ? 0.34 : -0.22) * dt));
      // vẽ (toạ độ từ dưới lên)
      $('reel-zone').style.bottom = `${g.zone * 100}%`;
      $('reel-zone').classList.toggle('on', inside);
      $('reel-fish').style.bottom = `calc(${g.fishY * 100}% - 14px)`;
      $('reel-prog').style.height = `${g.prog * 100}%`;
      // phao giật theo cá
      $('bobber').style.transform = `translate(-50%, -100%) translate(${Math.sin(g.t * 13) * 4}px, ${inside ? 2 : 6}px)`;
      if (g.prog >= 1) return finishReel(true);
      if (g.prog <= 0 || g.t > 7) return finishReel(false);
      requestAnimationFrame(loop);
    };
    requestAnimationFrame(loop);
  }
  function finishReel(ok) {
    $('reel').classList.add('hidden');
    $('bobber').style.transform = '';
    fishing.phase = 'done';
    reel(ok, ok ? null : 'Dây chùng quá, cá giãy tuột mất rồi!');
  }

  async function reel(success, localMsg) {
    const f = fishing;
    if (!f) return;
    const res = await call('/api/fishing/reel', { castId: f.cast.id, success }, true);
    endCast();
    if (!res) return status('');
    const out = res.outcome;
    if (out === 'caught' && res.caught) {
      status('');
      showCatch(res.caught, res.message, res.gained);
    } else {
      const msg = out === 'early' ? 'Giật sớm quá, cá giật mình bơi mất! Đợi phao chìm hẳn rồi hãy giật.'
        : out === 'late' ? res.message || 'Cá đã ăn mồi chạy mất!' : localMsg || res.message || 'Tuột mất rồi!';
      status(msg, 'sad');
      setTimeout(() => !fishing && status(''), 3500);
    }
  }
  function endCast() {
    fishing = null;
    $('bobber').className = 'bobber hidden';
    $('fish-shadow').classList.add('hidden');
    $('bang').classList.add('hidden');
    $('reel').classList.add('hidden');
    drawLine();
    me.drawn = '';
    drawMe();
    renderBar();
  }

  function showCatch(c, msg, gained) {
    const r = RARITY[c.rarity] || RARITY.common;
    const el = $('catch');
    const junk = c.rarity === 'junk';
    el.className = `catch r-${c.rarity}`;
    el.innerHTML = `<div class="catch-card" style="--rc:${r.color}">
        <div class="catch-rar">${r.name}</div>
        <img class="catch-img" src="${fishImg(c.fishId)}" alt="">
        <h3>${esc(c.name)}</h3>
        ${junk ? `<p>${esc(msg || 'Vứt đi thôi!')}</p>` : `<p><b>${c.cm} cm</b> · ${xu(c.price)} xu${c.record ? ' · <span class="rec">Kỷ lục mới!</span>' : ''}</p>`}
        <p class="muted">+${(gained && gained.xp) || 0} XP${junk ? '' : ' · đã cho vào giỏ'}</p>
        <button type="button" class="primary" data-ok>Tuyệt! <kbd>Enter</kbd></button>
      </div>`;
    el.querySelector('[data-ok]').focus();
  }
  const closeCatch = () => $('catch').classList.add('hidden');

  // ---------- Hộp thoại: mồi, giỏ cá, sổ tay ----------
  let dlgMode = null;
  function openDlg(mode) {
    dlgMode = mode;
    renderDlg();
    const dlg = $('dlg');
    if (!dlg.open) dlg.showModal();
    const first = dlg.querySelector('.primary:not([disabled]), button:not([disabled])');
    if (first) first.focus();
  }
  const openBait = () => openDlg('bait');
  const openBasket = () => openDlg('basket');
  const openBook = () => openDlg('book');
  function head(title, icon) {
    return `<div class="dlg-head"><h2>${icon}${title}</h2><button type="button" class="link icon-only" data-close title="Đóng (Esc)">${I.svg('x')}</button></div>
      <div class="tabs">${[['bait', 'Mồi câu'], ['basket', 'Giỏ cá'], ['book', 'Sổ tay']].map(([k, n]) => `<button type="button" class="tab${k === dlgMode ? ' active' : ''}" data-mode="${k}">${n}</button>`).join('')}</div>`;
  }
  function renderDlg() {
    if (!st) return;
    const body = $('dlg-body');
    if (dlgMode === 'bait') {
      body.innerHTML = `${head('Mồi câu', `<img src="${baitImg('worm')}" alt="">`)}
        <p class="muted">Mồi tốt dễ gặp cá hiếm hơn. Chọn mồi để câu, mua thêm theo gói.</p>
        <div class="bait-list">${st.baits.map((b) => `<div class="bait-row${b.id === bait ? ' sel' : ''}">
            <img src="${baitImg(b.id)}" alt="">
            <div class="bait-info"><b>${esc(b.name)}</b><span class="muted">${xu(b.price)} xu / mồi · may mắn ${'★'.repeat(b.luck + 1)}</span><span>Đang có <b>${b.qty}</b></span></div>
            <div class="bait-act">
              <button type="button" data-use="${b.id}" ${b.id === bait ? 'disabled' : ''}>${b.id === bait ? 'Đang dùng' : 'Dùng'}</button>
              ${[1, 5, 10].map((n) => `<button type="button" class="primary" data-buy="${b.id}" data-qty="${n}">+${n} · ${xu(b.price * n)}</button>`).join('')}
            </div></div>`).join('')}</div>`;
    } else if (dlgMode === 'basket') {
      const total = st.basket.reduce((s, c) => s + c.price, 0);
      body.innerHTML = `${head(`Giỏ cá ${st.basket.length}/${st.basketMax || 30}`, `<img src="${IMG}basket.png" alt="">`)}
        ${st.basket.length ? `<div class="basket-top"><span>Tổng giá trị <b>${xu(total)} xu</b></span><button type="button" class="primary" data-sell-all>${I.svg('coins')}Bán tất cả</button></div>
        <div class="basket-list">${st.basket.slice().sort((a, b) => b.price - a.price).map((c) => {
          const r = RARITY[c.rarity] || RARITY.common;
          return `<div class="fish-row" style="--rc:${r.color}"><img src="${fishImg(c.fishId)}" alt="">
            <div><b>${esc(c.name)}</b><span class="rar">${r.name}</span><span class="muted">${c.cm} cm</span></div>
            <button type="button" data-sell="${c.id}">${xu(c.price)} xu</button></div>`;
        }).join('')}</div>` : '<p class="empty">Giỏ trống. Ra đầu bến quăng cần thôi!</p>'}`;
    } else {
      const rec = st.record || {};
      const found = st.fish.filter((f) => f.rarity !== 'junk' && rec[f.id]).length;
      const all = st.fish.filter((f) => f.rarity !== 'junk').length;
      body.innerHTML = `${head(`Sổ tay câu cá ${found}/${all}`, I.svg('scroll'))}
        <div class="book">${st.fish.map((f) => {
          const r = RARITY[f.rarity] || RARITY.common;
          const got = f.rarity === 'junk' || rec[f.id];
          return `<div class="book-card${got ? '' : ' unknown'}" style="--rc:${r.color}">
            <img src="${fishImg(f.id)}" alt="">
            <b>${got ? esc(f.name) : '???'}</b>
            <span class="rar">${r.name}</span>
            <span class="muted">${f.rarity === 'junk' ? 'rác' : rec[f.id] ? `Kỷ lục ${rec[f.id]} cm` : `${f.minCm}–${f.maxCm} cm`}</span>
          </div>`;
        }).join('')}</div>`;
    }
  }

  // ---------- Vòng lặp ----------
  let lastTs = 0;
  function frame(ts) {
    const dt = Math.min(0.05, (ts - lastTs) / 1000 || 0);
    lastTs = ts;
    const busy = document.querySelector('dialog[open]') || !$('catch').classList.contains('hidden');
    if (!busy && !(fishing && fishing.phase !== 'done')) {
      stepMove(dt);
      if (me.walking) follow();
    }
    tickFishing();
    updateNear();
    requestAnimationFrame(frame);
  }

  // ---------- Sự kiện ----------
  const KEY = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down', KeyA: 'left', KeyD: 'right', KeyW: 'up', KeyS: 'down' };
  const typing = (e) => e.target && /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName);
  window.addEventListener('keydown', (e) => {
    if (typing(e) || e.ctrlKey || e.metaKey || e.altKey) return;
    if (!$('catch').classList.contains('hidden')) {
      if (e.code === 'Enter' || e.code === 'Escape' || e.code === 'Space') { e.preventDefault(); closeCatch(); }
      return;
    }
    if (document.querySelector('dialog[open]')) return;
    if (fishing && fishing.phase !== 'done') {
      if (e.code === 'Space' || e.code === 'KeyE' || e.code === 'Enter') {
        e.preventDefault();
        if (fishing.phase === 'reel') reelIn.hold = true;
        else if (!e.repeat) strike();
      }
      return;
    }
    const k = KEY[e.code];
    if (k) {
      keys.add(k);
      e.preventDefault();
      return;
    }
    if (e.code === 'KeyE' || e.code === 'Enter' || e.code === 'Space') {
      e.preventDefault();
      if (near) act(near);
      else goStation('dock', () => cast('dock'));
    } else if (e.code === 'KeyB') openBasket();
    else if (e.code === 'KeyM') openBait();
    else if (e.code === 'KeyN') openBook();
  }, true);
  window.addEventListener('keyup', (e) => {
    const k = KEY[e.code];
    if (k) keys.delete(k);
    if (e.code === 'Space' || e.code === 'KeyE' || e.code === 'Enter') reelIn.hold = false;
  });
  window.addEventListener('blur', () => { keys.clear(); reelIn.hold = false; });

  $('stage').addEventListener('pointerdown', (e) => {
    if (fishing && fishing.phase !== 'done') {
      if (fishing.phase === 'reel') reelIn.hold = true;
      else strike();
      return;
    }
    if (e.target.closest('.hint')) return;
    const obj = e.target.closest('.obj.click');
    if (obj) return goStation(obj.dataset.key, () => act(obj.dataset.key));
    const [x, y] = toWorld(e.clientX, e.clientY);
    // Bấm xuống mặt nước: ra chỗ câu gần nhất rồi quăng cần.
    if (inWater(x, y) && !onDock(x, y)) {
      const k = Math.hypot(x - SPOTS.bank.x, y - SPOTS.bank.y) < Math.hypot(x - SPOTS.dock.x, y - SPOTS.dock.y) ? 'bank' : 'dock';
      return goStation(k, () => cast(k));
    }
    walkTo(x, y);
  });
  window.addEventListener('pointerup', () => (reelIn.hold = false));
  $('reel').addEventListener('pointerdown', (e) => { reelIn.hold = true; e.preventDefault(); });
  $('hint').addEventListener('click', (e) => {
    e.stopPropagation();
    if (fishing && fishing.phase !== 'done') return strike();
    if (near) act(near);
  });
  $('btn-cast').addEventListener('click', () => {
    if (near && SPOTS[near]) cast(near);
    else goStation(nearestSpot(), () => cast(nearestSpot()));
  });
  $('btn-bait').addEventListener('click', openBait);
  $('btn-basket').addEventListener('click', openBasket);
  $('btn-book').addEventListener('click', openBook);
  $('catch').addEventListener('click', (e) => e.target.closest('[data-ok]') && closeCatch());
  $('dlg').addEventListener('click', async (e) => {
    const t = e.target.closest('button');
    if (e.target === $('dlg') || (t && t.dataset.close !== undefined)) return $('dlg').close();
    if (!t) return;
    if (t.dataset.mode) return openDlg(t.dataset.mode);
    if (t.dataset.use) {
      bait = t.dataset.use;
      try { localStorage.setItem('fishing.bait', bait); } catch (err) { /* bỏ qua */ }
      renderBar();
      return renderDlg();
    }
    if (t.dataset.buy) {
      const res = await call('/api/fishing/bait/buy', { bait: t.dataset.buy, qty: Number(t.dataset.qty) });
      if (res) {
        bait = t.dataset.buy;
        renderBar();
        renderDlg();
        A.toast(`Đã mua ${t.dataset.qty} mồi`);
      }
      return;
    }
    if (t.dataset.sell) {
      const res = await call('/api/fishing/sell', { ids: [Number(t.dataset.sell)] });
      if (res) { renderDlg(); A.toast(`Bán được ${xu((res.gained && res.gained.coins) || 0)} xu`); }
      return;
    }
    if (t.dataset.sellAll !== undefined) {
      const total = st.basket.reduce((s, c) => s + c.price, 0);
      if (!(await A.confirm(`Bán cả ${st.basket.length} con cá trong giỏ được ${xu(total)} xu?`, { title: 'Bán cá', ok: 'Bán hết' }))) return;
      const res = await call('/api/fishing/sell', { all: true });
      if (res) { renderDlg(); A.toast(`Bán được ${xu((res.gained && res.gained.coins) || 0)} xu`); }
    }
  });
  $('dlg').addEventListener('keydown', (e) => {
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      const order = ['bait', 'basket', 'book'];
      const i = order.indexOf(dlgMode);
      openDlg(order[(i + (e.key === 'ArrowLeft' ? -1 : 1) + 3) % 3]);
      e.preventDefault();
    }
  });
  window.addEventListener('resize', layout);

  // ---------- Khởi động ----------
  $('logo').innerHTML = I.logo(28);
  I.render();
  A.me().then(
    async (u) => {
      if (!u) return A.requireLogin();
      user = u;
      me.look = lookOf(u);
      const res = await call('/api/fishing');
      $('zone').classList.remove('hidden');
      renderWorld();
      layout();
      drawMe();
      renderHUD();
      requestAnimationFrame(frame);
      // Còn lần thả câu dở (tải lại trang): chờ tiếp nếu chưa hết hạn, quá hạn thì đóng lại.
      if (res && res.cast) {
        if (nowMs() < res.cast.biteAt) {
          me.x = SPOTS.dock.x;
          me.y = SPOTS.dock.y;
          follow();
          startCast(res.cast, SPOTS.dock);
        } else {
          fishing = { cast: res.cast, phase: 'done' };
          reel(false, '');
          status('');
        }
      }
    },
    (e) => A.toast(e.message),
  );
})();
