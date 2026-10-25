// Công trường DCity (khu ngoại ô): nhân vật đội nón bảo hộ đi lại trong cảnh công trường, tới chỗ làm việc nhận ca.
// Mỗi ca là một mini-game ngắn (khuân gạch, trộn vữa, lái cần cẩu); hết giờ nộp số mục tiêu hoàn thành, server tính
// tiền công + XP (trần 6.000 xu / ngày) và đã trừ HP lúc nhận ca. Mục tiêu sinh từ seed của ca nên giống nhau mọi máy.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const esc = A.escapeHtml;
  const xu = A.xu;

  const IMG = '/img/toon/site/';
  const W = 1400; // kích thước thế giới (px trước khi phóng)
  const H = 760;
  const SPRITE = 112; // chiều cao nhân vật
  const SPEED = 240; // px / giây

  const lookOf = (u) => ({ gender: u.gender, outfit: u.outfit, hair: u.hair, wings: u.wings, face: u.face });

  // ---------- Bố cục cảnh ----------
  // x, y = chân vật (giữa đáy ảnh); h = chiều cao hiển thị; foot = [nửa rộng, sâu] vùng chân không đi xuyên được.
  const OBJ = [
    { src: '/img/toon/city/tree_phuong.png', x: 70, y: 335, h: 230, foot: [30, 14], sway: true },
    { src: '/img/toon/city/tree_banglang.png', x: 1340, y: 340, h: 220, foot: [30, 14], sway: true },
    { src: '/img/toon/farm/tree_tre.png', x: 600, y: 330, h: 200, foot: [26, 10], sway: true },
    { key: 'board', src: IMG + 'site_sign.png', x: 175, y: 430, h: 130, foot: [100, 14], label: 'Bảng việc' },
    { src: IMG + 'building_frame.png', x: 395, y: 440, h: 340, foot: [190, 70], label: 'Nhà đang xây' },
    { src: '/img/toon/city/lamp.png', x: 545, y: 470, h: 130, foot: [8, 8] },
    { key: 'crane', src: IMG + 'crane.png', x: 1070, y: 450, h: 500, foot: [52, 24], lowLabel: true },
    { key: 'bricks', src: IMG + 'bricks_pile.png', x: 680, y: 560, h: 100, foot: [78, 26] },
    { src: '/img/toon/farm/wheelbarrow.png', x: 775, y: 590, h: 66, foot: [36, 12] },
    { src: IMG + 'cement_bags.png', x: 860, y: 500, h: 76, foot: [66, 22] },
    { src: IMG + 'mixer.png', x: 900, y: 655, h: 136, foot: [78, 26] },
    { key: 'mortar', src: IMG + 'mortar_tub.png', x: 1040, y: 690, h: 86, foot: [76, 24] },
    { src: IMG + 'sand_pile.png', x: 1240, y: 600, h: 76, foot: [78, 22] },
    { src: IMG + 'barrier.png', x: 250, y: 740, h: 66, foot: [72, 10] },
    { src: IMG + 'barrier.png', x: 1290, y: 745, h: 66, foot: [72, 10] },
    { src: IMG + 'cone.png', x: 345, y: 742, h: 46, foot: [16, 8] },
    { src: IMG + 'cone.png', x: 1180, y: 742, h: 46, foot: [16, 8] },
    { src: IMG + 'cone.png', x: 600, y: 470, h: 46, foot: [16, 8] },
  ];
  const STATION = {
    bricks: { job: 'bricks', x: 680, y: 610, art: IMG + 'bricks_pile.png' },
    mortar: { job: 'mortar', x: 1040, y: 735, art: IMG + 'mortar_tub.png' },
    crane: { job: 'crane', x: 1070, y: 500, art: IMG + 'crane.png' },
    board: { job: null, x: 175, y: 480 },
    exit: { job: null, x: 40, y: 610 },
  };
  const ORDER = ['bricks', 'mortar', 'crane'];
  const JOB_ART = { bricks: IMG + 'bricks_pile.png', mortar: IMG + 'mortar_tub.png', crane: IMG + 'crane_load.png' };

  // ---------- Trạng thái ----------
  let user = null;
  let st = null; // trạng thái công trường từ server
  let offset = 0; // giờ server − giờ máy (ms)
  const nowMs = () => Date.now() + offset;
  const me = { x: 140, y: 580, dir: 'right', walking: false, look: {}, drawn: '' };
  const keys = new Set();
  let target = null; // { x, y, then }
  let near = null; // trạm đang đứng gần
  let view = { s: 1, ox: 0, oy: 0 };
  let playing = null; // mini-game đang chạy

  // ---------- Gọi API ----------
  async function call(path, body) {
    try {
      const data = await A.api(path, body === undefined ? {} : { method: 'POST', body });
      if (data.user) user = data.user;
      if (data.jobs) {
        st = data;
        if (data.now) offset = data.now - Date.now();
      }
      renderHUD();
      renderBoard();
      return data;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      const hp = e.status === 423 || (e.status === 403 && /HP/.test(e.message));
      A.toast(hp ? `${e.message} — ghé Bệnh viện trên bản đồ để hồi máu` : e.message);
      if (hp) A.me().then((u) => u && ((user = u), renderHUD())).catch(() => {});
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
      pill.title = 'Làm ở công trường tốn máu mỗi ca';
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

  const jobOf = (id) => (st && st.jobs.find((j) => j.id === id)) || null;
  const cooldownLeft = () => Math.max(0, Math.ceil(((st && st.cooldownUntil) || 0) - nowMs()) / 1000);

  function renderBoard() {
    if (!st || !user) return;
    const t = st.today || { earned: 0, cap: 6000 };
    const rows = ORDER.map((id, i) => {
      const j = jobOf(id);
      if (!j) return '';
      const locked = user.level < j.level;
      return `<button type="button" class="job-row${locked ? ' locked' : ''}" data-job="${id}">
          <span class="kb">${i + 1}</span><img src="${JOB_ART[id]}" alt=""><b>${esc(j.name)}</b>
          <span class="meta">${locked ? `Cần Lv ${j.level}` : `${xu(j.basePay)}–${xu(j.maxPay)} xu`}<br>−${j.hp} HP · ${j.seconds}s</span>
        </button>`;
    }).join('');
    const cd = Math.ceil(cooldownLeft());
    $('job-board').innerHTML = `<h2>${I.svg('hammer')}Bảng việc hôm nay</h2>
      <div class="today">Đã kiếm <b>${xu(t.earned)}</b> / ${xu(t.cap)} xu
        <div class="cap-bar"><span style="width:${Math.min(100, (t.earned / Math.max(1, t.cap)) * 100)}%"></span></div></div>
      <div class="job-rows">${rows}</div>
      ${cd > 0 ? `<div class="cooldown">Nghỉ tay ${cd}s rồi nhận ca tiếp</div>` : ''}`;
  }

  // ---------- Cảnh ----------
  function renderWorld() {
    // Dải trời + đồi xa phía trên (ảnh lặp ngang).
    const sky = [0, 700].map((x) => `<img class="toon sky" src="/img/toon/farm/bg_sky_hills.png" alt="" style="left:${x}px" draggable="false">`).join('');
    $('objects').innerHTML = sky + OBJ.map((o, i) => {
      const lbl = o.key && STATION[o.key] && STATION[o.key].job ? jobLabel(STATION[o.key].job) : o.label ? `<span class="label">${o.label}</span>` : '';
      return `<div class="obj${o.key ? ' click' : ''}${o.sway ? ' sway' : ''}${o.lowLabel ? ' low-label' : ''}" data-i="${i}"${o.key ? ` data-key="${o.key}"` : ''}
        style="left:${o.x}px;top:${o.y}px;z-index:${o.y}">${lbl}<img class="toon" src="${o.src}" alt="" style="height:${o.h}px" draggable="false"></div>`;
    }).join('');
  }
  function jobLabel(id) {
    const j = jobOf(id);
    if (!j) return '';
    const locked = user && user.level < j.level;
    return `<span class="label${locked ? ' locked' : ''}">${esc(j.name)}${locked ? `<span class="lock">Lv ${j.level}</span>` : ''}</span>`;
  }

  function drawMe() {
    const el = $('me');
    el.style.left = `${Math.round(me.x)}px`;
    el.style.top = `${Math.round(me.y)}px`;
    el.style.zIndex = Math.round(me.y) + 1;
    const key = `${me.dir}|${me.walking}|${JSON.stringify(me.look)}`;
    if (key === me.drawn) return;
    me.drawn = key;
    const w = Math.round((SPRITE * C.FRAME_W) / C.FRAME_H);
    // Nón bảo hộ chồng lên đầu (đầu chibi chiếm ~ nửa trên khung).
    const hatW = Math.round(w * 0.74);
    $('me-body').innerHTML = `${C.sprite(me.look, { dir: me.dir, walking: me.walking, size: SPRITE })}
      <img class="hat" src="${IMG}hardhat.png" alt="" style="width:${hatW}px;top:${Math.round(SPRITE * 0.035)}px" draggable="false">`;
    updateFade();
  }

  // Vật cao đứng trước nhân vật và che quá nửa người thì làm mờ.
  function updateFade() {
    document.querySelectorAll('#objects .obj').forEach((el) => {
      const o = OBJ[el.dataset.i];
      const w = o.h * 1.2;
      const cover = o.y > me.y && Math.abs(o.x - me.x) < w * 0.45 && o.y - o.h < me.y - SPRITE * 0.4 && o.h > 90;
      el.classList.toggle('fade', cover);
    });
  }

  // ---------- Đi lại ----------
  function blocked(x, y) {
    if (x < 24 || x > W - 24 || y < 360 || y > H - 8) return true;
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
      if (d < 5) {
        const then = target.then;
        target = null;
        $('go-mark').classList.add('hidden');
        me.walking = false;
        drawMe();
        if (then) then();
        return;
      }
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
      if (!moved && target) {
        target.stuck = (target.stuck || 0) + dt;
        if (target.stuck > 0.4) {
          const then = Math.hypot(target.x - me.x, target.y - me.y) < 120 ? target.then : null;
          target = null;
          $('go-mark').classList.add('hidden');
          if (then) then();
        }
      }
      me.dir = Math.abs(dx) > Math.abs(dy) ? (dx < 0 ? 'left' : 'right') : dy < 0 ? 'up' : 'down';
    }
    if (moving !== me.walking || moving) {
      me.walking = moving;
      drawMe();
    }
  }

  function updateNear() {
    let best = null;
    let bd = 95;
    for (const [k, s] of Object.entries(STATION)) {
      const d = Math.hypot(s.x - me.x, s.y - me.y);
      if (d < bd) { bd = d; best = k; }
    }
    near = best;
    const hint = $('hint');
    document.querySelectorAll('#objects .obj').forEach((el) => el.classList.toggle('focus', !!near && el.dataset.key === near));
    if (!near || playing || $('dlg-job').open) return hint.classList.add('hidden');
    const s = STATION[near];
    const text = near === 'exit' ? 'Về thành phố' : near === 'board' ? 'Xem bảng việc' : `Làm: ${(jobOf(s.job) || {}).name || ''}`;
    hint.querySelector('span').textContent = text;
    hint.style.left = `${me.x}px`;
    hint.style.top = `${me.y - SPRITE - 40}px`;
    hint.classList.remove('hidden');
  }

  function act(key) {
    if (key === 'exit') return (location.href = '/');
    if (key === 'board') return openJob(nextJob());
    openJob(STATION[key].job);
  }
  // Việc cao nhất đủ level (gợi ý mặc định khi xem bảng việc).
  function nextJob() {
    const ok = ORDER.filter((id) => { const j = jobOf(id); return j && user && user.level >= j.level; });
    return ok[ok.length - 1] || 'bricks';
  }
  function goStation(key) {
    const s = STATION[key];
    walkTo(s.x, s.y, () => act(key));
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
    target = { x: Math.max(30, Math.min(W - 30, x)), y: Math.max(370, Math.min(H - 10, y)), then };
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

  // ---------- Thẻ việc ----------
  let jobOpen = null;
  function openJob(id) {
    const j = jobOf(id);
    if (!j || !user) return;
    jobOpen = id;
    const t = st.today || { earned: 0, cap: 6000 };
    const cd = Math.ceil(cooldownLeft());
    let why = '';
    if (user.level < j.level) why = `Cần level ${j.level} để làm việc này.`;
    else if (user.hospitalUntil && user.hospitalUntil > user.now) why = 'Bạn đang nằm viện.';
    else if (user.hp < Math.max(30, j.hp)) why = `Cần ít nhất ${Math.max(30, j.hp)} HP — ghé Bệnh viện hoặc ngồi nghỉ ghế đá.`;
    else if (cd > 0) why = `Nghỉ tay thêm ${cd}s rồi nhận ca tiếp.`;
    const full = t.earned >= t.cap;
    const tabs = ORDER.map((k) => {
      const x = jobOf(k);
      return `<button type="button" class="tab${k === id ? ' active' : ''}" data-tab-job="${k}">${esc(x ? x.name : k)}</button>`;
    }).join('');
    $('job-body').innerHTML = `
      <div class="tabs">${tabs}</div>
      <div class="job-art"><img src="${JOB_ART[id]}" alt=""></div>
      <h2>${esc(j.name)}</h2>
      <p class="muted" style="margin:0;text-align:center">${HELP[id]}</p>
      <div class="job-stats">
        <div>Tiền công<b>${xu(j.basePay)}–${xu(j.maxPay)} xu</b></div>
        <div>Kinh nghiệm<b>+${j.xp} XP</b></div>
        <div>Tốn máu<b>−${j.hp} HP</b></div>
        <div>Thời gian<b>${j.seconds} giây</b></div>
      </div>
      <div class="today">Hôm nay đã kiếm ${xu(t.earned)} / ${xu(t.cap)} xu${full ? ' — <b>đã đủ trần, làm tiếp không có tiền công</b>' : ''}</div>
      ${why ? `<div class="why">${esc(why)}</div>` : ''}
      <div class="actions">
        <button type="button" data-close>Để sau</button>
        <button type="button" class="primary" data-start ${why ? 'disabled' : ''}>${I.svg('play')}Nhận ca (−${j.hp} HP)</button>
      </div>`;
    const dlg = $('dlg-job');
    if (!dlg.open) dlg.showModal();
    const btn = dlg.querySelector(why ? '[data-close]' : '[data-start]');
    if (btn) btn.focus();
    updateNear();
  }
  const HELP = {
    bricks: 'Gạch rơi từ giàn giáo — đẩy xe rùa (← → hoặc chuột) hứng cho trúng.',
    mortar: 'Giữ phím Space / chuột để đẩy kim, giữ kim trong vùng xanh cho vữa đều tay.',
    crane: 'Lái móc cẩu bằng ← →, nhấn Space để thả khối bê tông đúng ô đánh dấu.',
  };

  async function startJob() {
    const id = jobOpen;
    const res = await call('/api/site/start', { job: id });
    if (!res) return;
    $('dlg-job').close();
    const shift = res.shift || res.active;
    if (shift) runGame(shift);
  }

  // ---------- Mini-game ----------
  // Ngẫu nhiên có seed (mulberry32) để mục tiêu của ca giống nhau trên mọi máy.
  function rng(seed) {
    let a = (seed % 4294967296) >>> 0;
    return () => {
      a = (a + 0x6d2b79f5) >>> 0;
      let t = a;
      t = Math.imul(t ^ (t >>> 15), t | 1);
      t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }
  const pics = {};
  function pic(src) {
    if (!pics[src]) {
      const im = new Image();
      im.src = src;
      pics[src] = im;
    }
    return pics[src];
  }
  const drawPic = (ctx, src, cx, by, h, rot = 0) => {
    const im = pic(src);
    if (!im.complete || !im.naturalWidth) return;
    const w = (im.naturalWidth / im.naturalHeight) * h;
    ctx.save();
    ctx.translate(cx, by);
    if (rot) ctx.rotate(rot);
    ctx.drawImage(im, -w / 2, -h, w, h);
    ctx.restore();
  };

  const CW = 720;
  const CH = 420;
  const input = { left: false, right: false, hold: false, drop: false, mouseX: null };

  function runGame(shift) {
    const job = jobOf(shift.job) || { name: shift.job, seconds: shift.seconds };
    const R = rng(shift.seed || 1);
    const targets = shift.targets || Math.floor(shift.seconds / 2);
    const g = { shift, job, R, targets, score: 0, t0: performance.now(), last: performance.now(), done: false, fx: [] };
    g.endsAt = shift.endsAt || nowMs() + shift.seconds * 1000;
    g.startAt = g.endsAt - shift.seconds * 1000;
    ({ bricks: initBricks, mortar: initMortar, crane: initCrane }[shift.job] || initBricks)(g);
    playing = g;
    keys.clear();
    target = null;
    $('game-title').textContent = job.name;
    $('game-help').textContent = HELP[shift.job] || '';
    $('game').classList.remove('hidden');
    updateNear();
    requestAnimationFrame(gameFrame);
  }

  function gameFrame(ts) {
    const g = playing;
    if (!g || g.done) return;
    const dt = Math.min(0.05, (ts - g.last) / 1000);
    g.last = ts;
    const left = Math.max(0, g.endsAt - nowMs());
    g.t = (nowMs() - g.startAt) / 1000;
    const ctx = $('game-canvas').getContext('2d');
    ctx.clearRect(0, 0, CW, CH);
    g.step(dt, ctx);
    // hiệu ứng chữ bay
    g.fx = g.fx.filter((f) => (f.life -= dt) > 0);
    for (const f of g.fx) {
      ctx.globalAlpha = Math.min(1, f.life * 2);
      ctx.font = '900 22px "Baloo 2", sans-serif';
      ctx.fillStyle = f.color;
      ctx.strokeStyle = '#fff';
      ctx.lineWidth = 4;
      ctx.textAlign = 'center';
      const y = f.y - (1 - f.life) * 40;
      ctx.strokeText(f.text, f.x, y);
      ctx.fillText(f.text, f.x, y);
      ctx.globalAlpha = 1;
    }
    g.score = Math.min(g.score, g.targets);
    $('game-score').textContent = `${g.score}/${g.targets}`;
    $('game-time').textContent = `${Math.ceil(left / 1000)}s`;
    if (left <= 0) return endGame(g);
    requestAnimationFrame(gameFrame);
  }
  const pop = (g, text, x, y, color = '#2f7a1e') => g.fx.push({ text, x, y, color, life: 1 });

  // Khuân gạch: gạch rơi theo lịch từ seed, xe rùa hứng gạch.
  function initBricks(g) {
    const n = Math.round(g.targets * 1.4);
    const span = g.shift.seconds - 2.4;
    g.bricks = [];
    for (let i = 0; i < n; i++) {
      g.bricks.push({ t: 0.6 + (i * span) / n + g.R() * 0.35, x: 70 + g.R() * 580, state: 'wait' });
    }
    g.barrow = CW / 2;
    g.load = 0;
    g.step = (dt, ctx) => {
      // nền: nhà đang xây phía sau, đất
      ctx.fillStyle = '#e9d2a6';
      ctx.fillRect(0, 0, CW, CH);
      ctx.globalAlpha = 0.55;
      drawPic(ctx, IMG + 'building_frame.png', 160, 300, 300);
      drawPic(ctx, IMG + 'building_frame.png', 560, 300, 300);
      ctx.globalAlpha = 1;
      ctx.fillStyle = '#c99a5e';
      ctx.fillRect(0, 380, CW, 40);
      const sp = 520;
      if (input.mouseX != null) g.barrow += Math.max(-sp * dt, Math.min(sp * dt, input.mouseX - g.barrow));
      if (input.left) g.barrow -= sp * dt;
      if (input.right) g.barrow += sp * dt;
      g.barrow = Math.max(60, Math.min(CW - 60, g.barrow));
      const FALL = 1.7;
      for (const b of g.bricks) {
        if (b.state === 'wait' && g.t >= b.t) b.state = 'fall';
        if (b.state !== 'fall') continue;
        const k = (g.t - b.t) / FALL;
        const y = -30 + k * 375;
        if (k >= 1) {
          if (Math.abs(b.x - g.barrow) < 62) {
            b.state = 'caught';
            g.score++;
            g.load = (g.load + 1) % 6;
            pop(g, '+1', g.barrow, 300);
          } else {
            b.state = 'broke';
            b.y = 385;
            b.life = 0.8;
            pop(g, 'Vỡ!', b.x, 360, '#c92a2a');
          }
          continue;
        }
        drawPic(ctx, IMG + 'brick.png', b.x, y, 26, k * 3);
      }
      for (const b of g.bricks) {
        if (b.state === 'broke' && (b.life -= dt) > 0) {
          ctx.globalAlpha = b.life;
          drawPic(ctx, IMG + 'brick.png', b.x - 10, b.y, 16, -0.6);
          drawPic(ctx, IMG + 'brick.png', b.x + 10, b.y, 16, 0.5);
          ctx.globalAlpha = 1;
        }
      }
      drawPic(ctx, '/img/toon/farm/wheelbarrow.png', g.barrow, 392, 74);
      for (let i = 0; i < g.load; i++) drawPic(ctx, IMG + 'brick.png', g.barrow - 22 + (i % 3) * 18, 352 - Math.floor(i / 3) * 10, 16);
    };
  }

  // Trộn vữa: vùng xanh trôi theo seed; giữ để đẩy kim lên, thả thì kim rơi; mỗi 2 giây trong vùng = +1.
  function initMortar(g) {
    const ph = [g.R() * 6, g.R() * 6, g.R() * 6];
    const fr = [0.5 + g.R() * 0.4, 1.1 + g.R() * 0.6, 0.23 + g.R() * 0.2];
    g.zone = (t) => 0.5 + 0.22 * Math.sin(t * fr[0] + ph[0]) + 0.1 * Math.sin(t * fr[1] + ph[1]) + 0.06 * Math.sin(t * fr[2] + ph[2]);
    g.p = 0.5;
    g.v = 0;
    g.acc = 0;
    g.spin = 0;
    g.step = (dt, ctx) => {
      ctx.fillStyle = '#e9d2a6';
      ctx.fillRect(0, 0, CW, CH);
      drawPic(ctx, IMG + 'mixer.png', 130, 400, 220);
      drawPic(ctx, IMG + 'cement_bags.png', 610, 400, 110);
      g.v += (input.hold ? 2.6 : -2.6) * dt;
      g.v *= Math.pow(0.35, dt);
      g.p += g.v * dt;
      if (g.p < 0) { g.p = 0; g.v = Math.max(0, g.v); }
      if (g.p > 1) { g.p = 1; g.v = Math.min(0, g.v); }
      const z = Math.max(0.12, Math.min(0.88, g.zone(g.t)));
      const zw = 0.13;
      const inside = Math.abs(g.p - z) < zw;
      if (inside) {
        g.acc += dt;
        if (g.acc >= 2) {
          g.acc -= 2;
          g.score++;
          pop(g, '+1 mẻ vữa', CW / 2, 230);
        }
      }
      g.spin += (input.hold ? 6 : 2) * dt;
      // máng vữa + xẻng đảo
      drawPic(ctx, IMG + 'mortar_tub.png', CW / 2, 396, 150, Math.sin(g.spin) * 0.04);
      // thước đo (giao diện): rãnh, vùng xanh, kim
      const x0 = 110;
      const x1 = CW - 110;
      const y = 70;
      ctx.fillStyle = '#7a5a36';
      roundRect(ctx, x0 - 6, y - 6, x1 - x0 + 12, 40, 14);
      ctx.fillStyle = '#f3e2c0';
      roundRect(ctx, x0, y, x1 - x0, 28, 10);
      ctx.fillStyle = inside ? '#5fd35a' : '#8ee08a';
      roundRect(ctx, x0 + (z - zw) * (x1 - x0), y, zw * 2 * (x1 - x0), 28, 8);
      const nx = x0 + g.p * (x1 - x0);
      ctx.fillStyle = '#c92a2a';
      ctx.beginPath();
      ctx.moveTo(nx, y + 32);
      ctx.lineTo(nx - 11, y + 52);
      ctx.lineTo(nx + 11, y + 52);
      ctx.closePath();
      ctx.fill();
      ctx.fillRect(nx - 2, y - 4, 4, 36);
      // vòng tiến độ mẻ vữa
      ctx.strokeStyle = '#ffb21e';
      ctx.lineWidth = 10;
      ctx.beginPath();
      ctx.arc(CW / 2, 170, 34, -Math.PI / 2, -Math.PI / 2 + (g.acc / 2) * Math.PI * 2);
      ctx.stroke();
      ctx.fillStyle = '#4a2c12';
      ctx.font = '900 16px "Baloo 2", sans-serif';
      ctx.textAlign = 'center';
      ctx.fillText(inside ? 'Đều tay!' : input.hold ? 'Đang đẩy…' : 'Giữ Space', CW / 2, 176);
    };
  }

  // Lái cần cẩu: móc chạy ngang theo phím, khối lắc nhẹ; Space thả khối, trúng ô đánh dấu (±24px) = +1.
  function initCrane(g) {
    g.hx = CW / 2;
    g.hv = 0;
    g.ang = 0;
    g.av = 0;
    g.placed = [];
    const nextTarget = () => (g.tx = 110 + g.R() * 500);
    nextTarget();
    g.fall = null;
    g.wait = 0;
    g.step = (dt, ctx) => {
      ctx.fillStyle = '#cfe8f6';
      ctx.fillRect(0, 0, CW, 300);
      ctx.fillStyle = '#e9d2a6';
      ctx.fillRect(0, 300, CW, 120);
      ctx.globalAlpha = 0.6;
      drawPic(ctx, IMG + 'crane.png', 660, 420, 420);
      ctx.globalAlpha = 1;
      // ray cần cẩu (giao diện)
      ctx.fillStyle = '#e0a800';
      ctx.fillRect(0, 26, CW, 12);
      // sàn bê tông + khối đã đặt
      ctx.fillStyle = '#b8b8b2';
      ctx.fillRect(60, 372, CW - 120, 14);
      for (const p of g.placed) {
        ctx.globalAlpha = Math.max(0, p.life);
        drawPic(ctx, IMG + 'crane_load.png', p.x, 374, 62);
        ctx.globalAlpha = 1;
        p.life -= dt * 0.4;
      }
      g.placed = g.placed.filter((p) => p.life > 0);
      // ô đánh dấu
      ctx.setLineDash([8, 6]);
      ctx.strokeStyle = '#2f7fd6';
      ctx.lineWidth = 3;
      ctx.strokeRect(g.tx - 34, 314, 68, 58);
      ctx.setLineDash([]);
      ctx.fillStyle = 'rgba(47, 127, 214, .15)';
      ctx.fillRect(g.tx - 34, 314, 68, 58);
      // điều khiển móc
      const acc = ((input.right ? 1 : 0) - (input.left ? 1 : 0)) * 900 + (input.mouseX != null ? Math.max(-1, Math.min(1, (input.mouseX - g.hx) / 60)) * 900 : 0);
      g.hv += acc * dt;
      g.hv *= Math.pow(0.04, dt);
      g.hx = Math.max(40, Math.min(CW - 40, g.hx + g.hv * dt));
      // khối treo lắc như con lắc, bị kéo theo gia tốc của móc
      g.av += (-9 * Math.sin(g.ang) - acc * 0.004) * dt;
      g.av *= Math.pow(0.5, dt);
      g.ang += g.av * dt;
      const L = 150;
      const lx = g.hx + Math.sin(g.ang) * L;
      const ly = 40 + Math.cos(g.ang) * L;
      // dây cáp từ xe con xuống móc; khối bê tông treo dưới móc
      const hx = lx;
      const hy = ly - 58;
      ctx.strokeStyle = '#555';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(g.hx, 38);
      ctx.lineTo(hx, hy - 30);
      ctx.stroke();
      ctx.fillStyle = '#5a6068';
      ctx.fillRect(g.hx - 14, 30, 28, 14);
      if (!g.fall && g.wait <= 0) drawPic(ctx, IMG + 'crane_hook.png', hx, hy + 6, 42, g.ang * 0.6);
      if (g.wait > 0) g.wait -= dt;
      else if (!g.fall) {
        drawPic(ctx, IMG + 'crane_load.png', lx, ly, 62, g.ang * 0.6);
        if (input.drop) {
          input.drop = false;
          g.fall = { x: lx, y: ly, vy: 0 };
        }
      }
      if (g.fall) {
        g.fall.vy += 1400 * dt;
        g.fall.y += g.fall.vy * dt;
        drawPic(ctx, IMG + 'crane_load.png', g.fall.x, Math.min(374, g.fall.y), 62);
        if (g.fall.y >= 374) {
          const ok = Math.abs(g.fall.x - g.tx) < 24;
          if (ok) {
            g.score++;
            g.placed.push({ x: g.tx, life: 1 });
            pop(g, 'Chuẩn!', g.tx, 300);
          } else pop(g, 'Lệch rồi', g.fall.x, 300, '#c92a2a');
          g.fall = null;
          g.wait = 0.3;
          g.ang = 0;
          g.av = 0;
          nextTarget();
        }
      }
    };
  }

  function roundRect(ctx, x, y, w, h, r) {
    ctx.beginPath();
    ctx.moveTo(x + r, y);
    ctx.arcTo(x + w, y, x + w, y + h, r);
    ctx.arcTo(x + w, y + h, x, y + h, r);
    ctx.arcTo(x, y + h, x, y, r);
    ctx.arcTo(x, y, x + w, y, r);
    ctx.closePath();
    ctx.fill();
  }

  async function endGame(g) {
    g.done = true;
    const body = { shiftId: g.shift.id, score: Math.min(g.score, g.targets) };
    let res = null;
    for (let i = 0; i < 3 && !res; i++) {
      try {
        res = await A.api('/api/site/finish', { method: 'POST', body });
      } catch (e) {
        if (e.status === 409 && /chưa hết giờ/.test(e.message)) await new Promise((r) => setTimeout(r, 1200));
        else {
          A.toast(e.message);
          break;
        }
      }
    }
    playing = null;
    $('game').classList.add('hidden');
    if (res) {
      if (res.user) user = res.user;
      if (res.jobs) {
        st = res;
        if (res.now) offset = res.now - Date.now();
      }
      renderHUD();
      renderBoard();
      const pay = res.pay || 0;
      floatGain(`+${xu(pay)} xu · +${res.xp || 0} XP`);
      await A.alert(`${g.job.name}: hoàn thành ${body.score}/${g.targets}. Nhận ${xu(pay)} xu và ${res.xp || 0} XP.` +
        (pay === 0 && st && st.today.earned >= st.today.cap ? ' Hôm nay đã đủ trần tiền công.' : ''), { title: 'Hết ca!', ok: 'Nghỉ tay' });
    }
    updateNear();
  }

  function floatGain(text) {
    const el = document.createElement('div');
    el.className = 'float-gain';
    el.textContent = text;
    const r = $('me').getBoundingClientRect();
    el.style.left = `${r.left + r.width / 2}px`;
    el.style.top = `${r.top}px`;
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 1700);
  }

  // ---------- Vòng lặp ----------
  let lastTs = 0;
  function frame(ts) {
    const dt = Math.min(0.05, (ts - lastTs) / 1000 || 0);
    lastTs = ts;
    if (!playing && !$('dlg-job').open) {
      stepMove(dt);
      if (me.walking) follow();
    }
    updateNear();
    requestAnimationFrame(frame);
  }

  // ---------- Sự kiện ----------
  const KEY = { ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down', KeyA: 'left', KeyD: 'right', KeyW: 'up', KeyS: 'down' };
  const typing = (e) => e.target && /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName);
  window.addEventListener('keydown', (e) => {
    if (typing(e) || e.ctrlKey || e.metaKey || e.altKey) return;
    if (playing) {
      if (e.code === 'ArrowLeft' || e.code === 'KeyA') input.left = true;
      if (e.code === 'ArrowRight' || e.code === 'KeyD') input.right = true;
      if (e.code === 'Space' || e.code === 'ArrowUp' || e.code === 'KeyW') input.hold = true;
      if ((e.code === 'Space' || e.code === 'ArrowDown' || e.code === 'Enter') && !e.repeat) input.drop = true;
      input.mouseX = null;
      e.preventDefault();
      return;
    }
    if (document.querySelector('dialog[open]')) {
      if ($('dlg-job').open && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
        const i = ORDER.indexOf(jobOpen);
        openJob(ORDER[(i + (e.key === 'ArrowLeft' ? -1 : 1) + ORDER.length) % ORDER.length]);
        e.preventDefault();
      }
      return;
    }
    const k = KEY[e.code];
    if (k) {
      keys.add(k);
      e.preventDefault();
      return;
    }
    if ((e.code === 'KeyE' || e.code === 'Enter' || e.code === 'Space') && near) {
      e.preventDefault();
      act(near);
      return;
    }
    if (/^Digit[1-3]$/.test(e.code)) goStation(ORDER[Number(e.code.slice(5)) - 1]);
  }, true);
  window.addEventListener('keyup', (e) => {
    const k = KEY[e.code];
    if (k) keys.delete(k);
    if (e.code === 'ArrowLeft' || e.code === 'KeyA') input.left = false;
    if (e.code === 'ArrowRight' || e.code === 'KeyD') input.right = false;
    if (e.code === 'Space' || e.code === 'ArrowUp' || e.code === 'KeyW') input.hold = false;
  });
  window.addEventListener('blur', () => { keys.clear(); input.left = input.right = input.hold = false; });

  // Chuột / chạm trong mini-game: kéo ngang để điều khiển, giữ để đẩy, chạm nhả để thả khối.
  const cv = $('game-canvas');
  const cvX = (e) => {
    const r = cv.getBoundingClientRect();
    return ((e.clientX - r.left) / r.width) * CW;
  };
  cv.addEventListener('pointerdown', (e) => {
    input.hold = true;
    input.mouseX = cvX(e);
    cv.setPointerCapture(e.pointerId);
  });
  cv.addEventListener('pointermove', (e) => {
    if (e.pointerType === 'mouse' || input.hold) input.mouseX = cvX(e);
  });
  cv.addEventListener('pointerup', () => {
    input.hold = false;
    if (playing && playing.shift.job === 'crane') input.drop = true;
  });

  $('stage').addEventListener('pointerdown', (e) => {
    if (playing) return;
    const obj = e.target.closest('.obj.click');
    if (obj) return goStation(obj.dataset.key);
    if (e.target.closest('.hint')) return;
    const [x, y] = toWorld(e.clientX, e.clientY);
    walkTo(x, y);
  });
  $('hint').addEventListener('click', () => near && act(near));
  $('job-board').addEventListener('click', (e) => {
    const b = e.target.closest('[data-job]');
    if (b) goStation(b.dataset.job);
  });
  $('dlg-job').addEventListener('click', (e) => {
    if (e.target.closest('[data-close]')) $('dlg-job').close();
    else if (e.target.closest('[data-start]')) startJob();
    else if (e.target.closest('[data-tab-job]')) openJob(e.target.closest('[data-tab-job]').dataset.tabJob);
    else if (e.target === $('dlg-job')) $('dlg-job').close();
  });
  $('dlg-job').addEventListener('close', () => {
    jobOpen = null;
    document.activeElement && document.activeElement.blur && document.activeElement.blur();
  });
  window.addEventListener('resize', layout);
  setInterval(() => !playing && renderBoard(), 1000);

  // ---------- Khởi động ----------
  $('logo').innerHTML = I.logo(28);
  I.render();
  A.me().then(
    async (u) => {
      if (!u) return A.requireLogin();
      user = u;
      me.look = lookOf(u);
      const res = await call('/api/site');
      $('zone').classList.remove('hidden');
      renderWorld();
      layout();
      drawMe();
      renderHUD();
      requestAnimationFrame(frame);
      // Đang dở ca (tải lại trang giữa ca): chơi tiếp phần thời gian còn lại.
      if (res && res.active && res.active.endsAt > nowMs() - 25000) runGame(res.active);
    },
    (e) => A.toast(e.message),
  );
})();
