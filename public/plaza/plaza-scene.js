// Quảng trường DCity: một cảnh công viên riêng (lối lát gạch, bãi cỏ, cây, đài phun nước, hồ sen + cầu gỗ,
// ghế ngồi được, xích đu, cầu trượt, xe kem, chim bồ câu, bướm). Vào trang là vào "phòng công viên" qua
// WebSocket /ws/plaza: thấy người chơi khác cũng đang ở đây, trò chuyện (bong bóng 4 giây), biểu cảm, xem hồ sơ,
// tặng xu, mời chơi. Rời trang là rời phòng.
// Ảnh: /img/toon/park/ (dựng riêng), /img/toon/city3d/ (cây, đèn, ghế, đài phun nước), nền /img/toon/city/.
(function () {
  const A = window.Account;
  const C = window.Character;
  const I = window.Icons;
  const $ = (id) => document.getElementById(id);
  const esc = (s) => A.escapeHtml(s);
  const xu = A.xu;
  const ic = (name) => {
    try { return (I && I.svg(name)) || ''; } catch (e) { return ''; }
  };

  // ---------- Kích thước cảnh (khớp SceneW / SceneH ở games/plaza) ----------
  const SCENE_W = 2400;
  const SCENE_H = 1000;
  const SKY_H = 250; // dải trời + đồi phía sau hàng rào; dưới đó là mặt đất đi được
  const WALK_TOP = 300; // chân nhân vật không lên quá hàng cây sau
  const SPRITE = 96; // chiều cao nhân vật
  const SPEED = 230; // đơn vị cảnh / giây
  const CELL = 20; // ô lưới tìm đường
  const BUBBLE_MS = 4000; // bong bóng chat hiện 4 giây
  const SEND_EVERY = 150;
  const GATE = { x: 120, y: 640 }; // cổng "Về thành phố" bên trái lối chính

  const EMOTES = [
    { kind: 'wave', name: 'Vẫy tay' },
    { kind: 'heart', name: 'Thả tim' },
    { kind: 'laugh', name: 'Cười' },
    { kind: 'clap', name: 'Vỗ tay' },
    { kind: 'dance', name: 'Nhảy' },
  ];
  const emoteImg = (kind) => `/img/toon/city/emote_${kind}.png`;
  const GIFT_STEPS = [10, 50, 100, 500];
  const GAME_NAME = { caro: 'Cờ Caro', baucua: 'Bầu cua', tienlen: 'Tiến Lên' };

  // ---------- Bố cục ----------
  // Lối lát gạch (x, y, rộng, cao) trên nền cỏ: lối chính chạy ngang, sân tròn quanh đài phun nước, lối lên sân chơi,
  // lối xuống hồ sen.
  const PATHS = [
    [0, 600, SCENE_W, 110], // lối chính từ cổng sang phải
    [980, 420, 440, 400], // sân đài phun nước
    [470, 430, 90, 180], // lối lên sân chơi
    [1680, 690, 90, 140], // lối xuống hồ
    [150, 690, 260, 60], // sân trước cổng
  ];

  // Vật trong cảnh: [ảnh, x chân, y chân, tỉ lệ, vật cản {w, h} quanh chân (null = không cản), thêm]
  // ảnh "park/..." = /img/toon/park/, còn lại = /img/toon/city3d/.
  const PROPS = [
    // hàng cây phía sau (sát hàng rào)
    ['tree_phuong', 160, 330, 0.85, { w: 60, h: 24 }],
    ['tree_sao', 420, 300, 0.8, { w: 40, h: 20 }],
    ['tree_banglang', 690, 325, 0.8, { w: 60, h: 24 }],
    ['tree_cau', 900, 300, 0.95, { w: 30, h: 16 }],
    ['tree_round', 1060, 310, 1, { w: 50, h: 20 }],
    ['tree_phuong', 1330, 325, 0.85, { w: 60, h: 24 }],
    ['tree_dua', 1560, 315, 0.85, { w: 40, h: 18 }],
    ['tree_banglang', 1800, 325, 0.8, { w: 60, h: 24 }],
    ['tree_sao', 2040, 300, 0.8, { w: 40, h: 20 }],
    ['tree_bang', 2280, 335, 0.75, { w: 60, h: 24 }],
    // sân chơi bên trái
    ['park/swing', 300, 470, 1.35, { w: 190, h: 30 }],
    ['park/slide', 640, 470, 1.35, { w: 150, h: 34 }],
    ['park/seesaw', 470, 560, 1.3, { w: 130, h: 22 }],
    ['tree_mai', 90, 470, 0.7, { w: 40, h: 18 }],
    ['tree_dao', 800, 560, 0.7, { w: 40, h: 18 }],
    // đài phun nước giữa sân
    ['fountain', 1200, 640, 1.8, { w: 330, h: 90 }],
    ['park/flowerbed', 1030, 470, 0.9, { w: 130, h: 50 }],
    ['park/flowerbed', 1370, 470, 0.9, { w: 130, h: 50 }],
    ['park/flowerbed', 1030, 800, 0.9, { w: 130, h: 50 }],
    ['park/flowerbed', 1370, 800, 0.9, { w: 130, h: 50 }],
    // xe kem + bong bóng cạnh lối chính
    ['park/icecream', 860, 600, 1.1, { w: 130, h: 30 }, { name: 'Xe kem' }],
    ['park/balloons', 760, 600, 1.0, { w: 24, h: 14 }],
    // hồ sen + cầu gỗ bên phải
    ['park/pond', 1960, 880, 1.25, null, { pond: true }],
    ['park/bridge_arch', 1960, 900, 1.15, null],
    ['tree_willow', 2280, 820, 0.75, { w: 50, h: 20 }],
    ['tree_willow', 1640, 960, 0.7, { w: 50, h: 20 }],
    ['lotus', 2240, 960, 1, null],
    // đèn, bụi hoa, thùng rác dọc lối chính
    ['lamp', 300, 590, 1, { w: 20, h: 12 }],
    ['lamp', 950, 590, 1, { w: 20, h: 12 }],
    ['lamp', 1460, 590, 1, { w: 20, h: 12 }],
    ['lamp', 2100, 590, 1, { w: 20, h: 12 }],
    ['lamp', 700, 730, 1, { w: 20, h: 12 }],
    ['lamp', 1700, 730, 1, { w: 20, h: 12 }],
    ['bush_flower', 40, 590, 0.9, { w: 70, h: 20 }],
    ['bush', 2360, 600, 0.9, { w: 70, h: 20 }],
    ['bush_flower', 560, 860, 1, { w: 80, h: 24 }],
    ['bush', 820, 950, 1, { w: 80, h: 24 }],
    ['bush_flower', 1500, 950, 1, { w: 80, h: 24 }],
    ['flower_clump', 260, 900, 1, null],
    ['flower_clump', 1150, 960, 1, null],
    ['flower_clump', 2200, 700, 1, null],
    ['park/trash_bin', 1000, 590, 1, { w: 22, h: 12 }],
    ['park/trash_bin', 1560, 730, 1, { w: 22, h: 12 }],
    ['tree_round_small', 400, 930, 1, { w: 40, h: 18 }],
    ['tree_xoai', 1300, 975, 0.7, { w: 50, h: 20 }],
    ['tree_cau', 640, 975, 0.9, { w: 30, h: 16 }],
    // cổng về thành phố
    ['park/park_gate', GATE.x, GATE.y + 70, 0.8, null, { gate: true }],
  ];

  // Ghế ngồi được: [ảnh, x, y, tỉ lệ]. Ngồi: đứng vào chỗ ngồi (ngay trước ghế), quay mặt xuống.
  const BENCHES = [
    ['bench', 380, 590, 1.1],
    ['bench', 1560, 590, 1.1],
    ['bench_wood', 2220, 590, 1.1],
    ['park/stone_bench', 1060, 715, 1.4],
    ['park/stone_bench', 1340, 715, 1.4],
    ['bench', 860, 760, 1.1],
    ['bench_wood', 1780, 800, 1.1],
    ['park/stone_bench', 300, 800, 1.4],
  ];

  // ---------- Trạng thái ----------
  let user = null;
  const look = (u) => ({ gender: u.gender, outfit: u.outfit, hair: u.hair, wings: u.wings, face: u.face });
  const me = { x: GATE.x + 140, y: 670, dir: 'right', walking: false, sitting: null, el: null, drawn: '' };
  let path = null; // các điểm đang đi tới
  let onArrive = null; // việc làm khi tới nơi (ngồi ghế, ra cổng)
  const keys = new Set();
  let scale = 1;
  let camX = 0;
  let camY = 0;
  let manualCam = false; // đang kéo xem: camera không bám nhân vật tới khi nhân vật đi
  let manifest = {};
  const props = []; // vật đã đặt: {el, x, y, w, h, top, left, block}
  const benches = []; // {x, y, seatX, seatY, el}

  // ---------- Ảnh ----------

  async function loadManifests() {
    for (const [dir, url] of [['', '/img/toon/city3d/manifest.json'], ['park/', '/img/toon/park/manifest.json']]) {
      try {
        const m = await (await fetch(url)).json();
        for (const it of m.images || []) {
          const key = dir + it.file.replace(/\.png$/, '');
          if (!manifest[key] || !it.replaces) manifest[key] = it;
        }
      } catch (e) { /* thiếu manifest: dùng cỡ mặc định */ }
    }
  }

  const srcOf = (name) => (name.startsWith('park/') ? `/img/toon/park/${name.slice(5)}.png` : `/img/toon/city3d/${name}.png`);

  // Đặt một ảnh theo điểm chân (x, y) với tỉ lệ s; trả hộp hiển thị.
  function placeImg(name, x, y, s, cls) {
    const m = manifest[name] || { w: 100, h: 100, foot: [50, 100], anchor: 'bottom-center' };
    const w = m.w * s;
    const h = m.h * s;
    const fx = (m.anchor === 'center' ? m.w / 2 : m.foot[0]) * s;
    const fy = (m.anchor === 'center' ? m.h / 2 : m.foot[1]) * s;
    const el = document.createElement('img');
    el.src = srcOf(name);
    el.alt = '';
    el.draggable = false;
    el.className = 'prop' + (cls ? ' ' + cls : '');
    el.style.cssText = `left:${x - fx}px;top:${y - fy}px;width:${w}px;height:${h}px;z-index:${Math.round(y)}`;
    $('things').appendChild(el);
    return { el, x, y, left: x - fx, top: y - fy, w, h };
  }

  function buildScene() {
    const ground = $('ground');
    ground.innerHTML = PATHS.map(([x, y, w, h]) => `<div class="path" style="left:${x}px;top:${y}px;width:${w}px;height:${h}px"></div>`).join('')
      + `<div class="hedge" style="top:${SKY_H - 30}px"></div>`;
    for (const [name, x, y, s, block, extra] of PROPS) {
      const p = placeImg(name, x, y, s, extra && extra.pond ? 'flat' : '');
      p.block = block;
      p.extra = extra || {};
      if (p.extra.pond) {
        p.el.style.zIndex = '2'; // hồ nằm sát mặt đất
        // vùng nước là hình elip: chặn đi bằng nhiều hộp nhỏ
        p.pond = { cx: x, cy: y, rx: p.w * 0.46, ry: p.h * 0.42 };
      }
      if (p.extra.gate) p.el.classList.add('gate');
      props.push(p);
    }
    for (const [name, x, y, s] of BENCHES) {
      const p = placeImg(name, x, y, s, 'bench');
      const b = { x, y, w: p.w, seatX: x, seatY: y + 4, el: p.el };
      p.block = { w: p.w * 0.9, h: 16 };
      props.push(p);
      benches.push(b);
      p.el.addEventListener('click', (e) => {
        e.stopPropagation();
        goSit(b);
      });
    }
    // Đối tượng của mình
    me.el = document.createElement('div');
    me.el.className = 'actor me';
    me.el.innerHTML = '<div class="tag"></div><div class="body"></div><div class="fx"></div>';
    $('things').appendChild(me.el);
    buildPigeons();
    buildButterflies();
    buildGrid();
  }

  // ---------- Lưới vật cản + tìm đường ----------
  const GW = Math.ceil(SCENE_W / CELL);
  const GH = Math.ceil(SCENE_H / CELL);
  let blocked = new Uint8Array(GW * GH);

  function buildGrid() {
    blocked = new Uint8Array(GW * GH);
    const mark = (x0, y0, x1, y1) => {
      for (let gy = Math.max(0, Math.floor(y0 / CELL)); gy <= Math.min(GH - 1, Math.floor(y1 / CELL)); gy++) {
        for (let gx = Math.max(0, Math.floor(x0 / CELL)); gx <= Math.min(GW - 1, Math.floor(x1 / CELL)); gx++) blocked[gy * GW + gx] = 1;
      }
    };
    mark(0, 0, SCENE_W, WALK_TOP); // phía sau hàng cây
    for (const p of props) {
      if (p.block) mark(p.x - p.block.w / 2, p.y - p.block.h, p.x + p.block.w / 2, p.y + 4);
      if (p.pond) {
        for (let gy = 0; gy < GH; gy++) for (let gx = 0; gx < GW; gx++) {
          const cx = gx * CELL + CELL / 2;
          const cy = gy * CELL + CELL / 2;
          if (((cx - p.pond.cx) / p.pond.rx) ** 2 + ((cy - p.pond.cy) / p.pond.ry) ** 2 <= 1) blocked[gy * GW + gx] = 1;
        }
      }
    }
  }

  const cellOf = (x, y) => [Math.max(0, Math.min(GW - 1, Math.floor(x / CELL))), Math.max(0, Math.min(GH - 1, Math.floor(y / CELL)))];
  const free = (x, y) => {
    if (x < 20 || x > SCENE_W - 20 || y < WALK_TOP || y > SCENE_H - 8) return false;
    const [gx, gy] = cellOf(x, y);
    return !blocked[gy * GW + gx];
  };

  // Ô trống gần nhất quanh (x, y) (bấm vào chỗ không đi được thì đi tới sát đó).
  function nearestFree(x, y) {
    if (free(x, y)) return [x, y];
    for (let r = CELL; r < 400; r += CELL) {
      for (let a = 0; a < 16; a++) {
        const px = x + Math.cos((a / 16) * Math.PI * 2) * r;
        const py = y + Math.sin((a / 16) * Math.PI * 2) * r;
        if (free(px, py)) return [px, py];
      }
    }
    return null;
  }

  // A* trên lưới 8 hướng; trả danh sách điểm (đã rút gọn đoạn thẳng).
  function findPath(x0, y0, x1, y1) {
    const t = nearestFree(x1, y1);
    if (!t) return null;
    [x1, y1] = t;
    const [sx, sy] = cellOf(x0, y0);
    const [tx, ty] = cellOf(x1, y1);
    const start = sy * GW + sx;
    const goal = ty * GW + tx;
    const g = new Float32Array(GW * GH).fill(Infinity);
    const from = new Int32Array(GW * GH).fill(-1);
    const open = [start];
    const f = new Float32Array(GW * GH).fill(Infinity);
    g[start] = 0;
    const hfun = (i) => Math.hypot((i % GW) - tx, Math.floor(i / GW) - ty);
    f[start] = hfun(start);
    const inOpen = new Uint8Array(GW * GH);
    inOpen[start] = 1;
    let steps = 0;
    while (open.length && steps++ < 20000) {
      let bi = 0;
      for (let k = 1; k < open.length; k++) if (f[open[k]] < f[open[bi]]) bi = k;
      const cur = open[bi];
      open.splice(bi, 1);
      inOpen[cur] = 0;
      if (cur === goal) break;
      const cx = cur % GW;
      const cy = Math.floor(cur / GW);
      for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) {
        if (!dx && !dy) continue;
        const nx = cx + dx;
        const ny = cy + dy;
        if (nx < 0 || ny < 0 || nx >= GW || ny >= GH) continue;
        const ni = ny * GW + nx;
        if (blocked[ni] && ni !== goal) continue;
        if (dx && dy && (blocked[cy * GW + nx] || blocked[ny * GW + cx])) continue; // không cắt góc
        const ng = g[cur] + (dx && dy ? 1.414 : 1);
        if (ng < g[ni]) {
          g[ni] = ng;
          from[ni] = cur;
          f[ni] = ng + hfun(ni);
          if (!inOpen[ni]) { open.push(ni); inOpen[ni] = 1; }
        }
      }
    }
    if (from[goal] === -1 && goal !== start) return null;
    const cells = [];
    for (let i = goal; i !== -1 && i !== start; i = from[i]) cells.push(i);
    cells.reverse();
    const pts = cells.map((i) => [(i % GW) * CELL + CELL / 2, Math.floor(i / GW) * CELL + CELL / 2]);
    if (pts.length) pts[pts.length - 1] = [x1, y1];
    else pts.push([x1, y1]);
    // rút gọn: bỏ điểm giữa khi đi thẳng được
    const out = [];
    let ax = x0;
    let ay = y0;
    for (let k = 0; k < pts.length; k++) {
      const nxt = pts[k + 1];
      if (nxt && lineFree(ax, ay, nxt[0], nxt[1])) continue;
      out.push(pts[k]);
      [ax, ay] = pts[k];
    }
    return out;
  }

  function lineFree(x0, y0, x1, y1) {
    const n = Math.ceil(Math.hypot(x1 - x0, y1 - y0) / (CELL / 2));
    for (let i = 1; i <= n; i++) if (!free(x0 + ((x1 - x0) * i) / n, y0 + ((y1 - y0) * i) / n)) return false;
    return true;
  }

  // ---------- Nhân vật ----------

  function drawActor(a) {
    const key = JSON.stringify([a.look, a.dir, a.walking, a.sitting ? 1 : 0, a.hospital]);
    if (a.drawn !== key) {
      a.drawn = key;
      const pose = a.hospital ? 'lie' : a.sitting ? 'sit' : undefined;
      a.el.querySelector('.body').innerHTML = C.sprite(a.look || {}, { dir: a.sitting ? 'down' : a.dir || 'down', walking: !!a.walking && !pose, size: SPRITE, pose });
    }
    a.el.style.transform = `translate(${a.x}px, ${a.y}px)`;
    a.el.style.zIndex = String(Math.round(a.y) + 2);
  }

  function setTag(a, name, level) {
    a.el.querySelector('.tag').innerHTML = `<span class="lv">Lv ${level || 1}</span>${esc(name || '')}`;
  }

  function showBubble(el, text, ms = BUBBLE_MS) {
    if (!el) return;
    let b = el.querySelector('.bubble');
    if (!b) {
      b = document.createElement('div');
      b.className = 'bubble';
      el.appendChild(b);
    }
    b.textContent = text;
    b.classList.remove('hidden');
    el.classList.add('talking');
    clearTimeout(b._t);
    b._t = setTimeout(() => {
      b.classList.add('hidden');
      el.classList.remove('talking');
    }, ms);
  }

  function showEmote(el, kind) {
    if (!el) return;
    const fx = el.querySelector('.fx');
    const img = document.createElement('img');
    img.className = 'emote';
    img.src = emoteImg(kind);
    img.alt = '';
    img.draggable = false;
    fx.appendChild(img);
    setTimeout(() => img.remove(), 2400);
    if (kind === 'dance') {
      el.classList.add('dancing');
      clearTimeout(el._dance);
      el._dance = setTimeout(() => el.classList.remove('dancing'), 2400);
    }
  }

  function floatText(el, text) {
    if (!el) return;
    const s = document.createElement('span');
    s.className = 'float';
    s.textContent = text;
    el.querySelector('.fx').appendChild(s);
    setTimeout(() => s.remove(), 2200);
  }

  // ---------- Di chuyển của mình ----------

  function walkTo(x, y, then) {
    standUp();
    const pts = findPath(me.x, me.y, x, y);
    if (!pts) return;
    path = pts;
    onArrive = then || null;
    manualCam = false;
  }

  function step(dt) {
    let vx = 0;
    let vy = 0;
    if (keys.has('left')) vx -= 1;
    if (keys.has('right')) vx += 1;
    if (keys.has('up')) vy -= 1;
    if (keys.has('down')) vy += 1;
    let moving = false;
    if (vx || vy) {
      path = null;
      onArrive = null;
      standUp();
      manualCam = false;
      const len = Math.hypot(vx, vy);
      const dx = (vx / len) * SPEED * dt;
      const dy = (vy / len) * SPEED * dt;
      if (free(me.x + dx, me.y + dy)) { me.x += dx; me.y += dy; } else if (free(me.x + dx, me.y)) me.x += dx; else if (free(me.x, me.y + dy)) me.y += dy;
      me.dir = Math.abs(vx) > Math.abs(vy) ? (vx < 0 ? 'left' : 'right') : vy < 0 ? 'up' : 'down';
      moving = true;
    } else if (path && path.length) {
      const [tx, ty] = path[0];
      const dx = tx - me.x;
      const dy = ty - me.y;
      const d = Math.hypot(dx, dy);
      const s = SPEED * dt;
      if (d <= s) {
        me.x = tx;
        me.y = ty;
        path.shift();
        if (!path.length) {
          path = null;
          const fn = onArrive;
          onArrive = null;
          if (fn) fn();
        }
      } else {
        me.x += (dx / d) * s;
        me.y += (dy / d) * s;
      }
      if (d > 0.5) me.dir = Math.abs(dx) > Math.abs(dy) ? (dx < 0 ? 'left' : 'right') : dy < 0 ? 'up' : 'down';
      moving = true;
    }
    me.walking = moving;
    drawActor(me);
    updateHint();
    fadeCovers();
  }

  // Vật đứng trước nhân vật mà che quá nhiều thì mờ đi (giống game Avatar).
  function fadeCovers() {
    const l = me.x - 26;
    const r = me.x + 26;
    const t = me.y - SPRITE + 10;
    const b = me.y - 4;
    for (const p of props) {
      if (p.extra && p.extra.pond) continue;
      const front = p.y > me.y + 2 && p.y - p.h < me.y;
      const ox = Math.max(0, Math.min(r, p.left + p.w) - Math.max(l, p.left));
      const oy = Math.max(0, Math.min(b, p.top + p.h) - Math.max(t, p.top));
      const cover = front && (ox * oy) / ((r - l) * (b - t)) > 0.35;
      p.el.classList.toggle('fade', cover);
    }
  }

  // ---------- Ngồi ghế (hồi máu ×2) ----------
  let sitTimer = null;

  function nearestBench(range = 9999) {
    let best = null;
    let bd = range;
    for (const b of benches) {
      const d = Math.hypot(b.seatX - me.x, b.seatY - me.y);
      if (d < bd) { bd = d; best = b; }
    }
    return best;
  }

  function goSit(b) {
    if (!b) return;
    const taken = [...players.values()].some((p) => p.sitting && Math.hypot(p.x - b.seatX, p.y - b.seatY) < 30);
    if (taken) return A.toast('Ghế này có người ngồi rồi');
    walkTo(b.seatX, b.seatY + 14, () => sitOn(b));
  }

  async function sitOn(b) {
    me.x = b.seatX;
    me.y = b.seatY;
    me.sitting = b;
    me.dir = 'down';
    drawActor(me);
    try {
      const u = await A.api('/api/me/sit', { method: 'POST', body: { sit: true } });
      setUser(u);
      A.toast('Ngồi nghỉ: hồi máu nhanh gấp đôi');
    } catch (e) {
      A.toast(e.message);
      me.sitting = null;
      drawActor(me);
      return;
    }
    clearInterval(sitTimer);
    sitTimer = setInterval(() => {
      if (!me.sitting) return clearInterval(sitTimer);
      A.api('/api/me/sit', { method: 'POST', body: { sit: true } }).then(setUser, () => {});
    }, 30000);
  }

  function standUp() {
    if (!me.sitting) return;
    me.sitting = null;
    me.y += 14;
    clearInterval(sitTimer);
    A.api('/api/me/sit', { method: 'POST', body: { sit: false } }).then(setUser, () => {});
  }

  // Gợi ý phím E: ngồi ghế gần / ra cổng.
  let hintAct = null;
  function updateHint() {
    const h = $('park-hint');
    let text = '';
    hintAct = null;
    if (!me.sitting && !path) {
      if (Math.hypot(me.x - GATE.x, me.y - GATE.y) < 120) {
        text = 'Về thành phố';
        hintAct = () => (location.href = '/');
      } else {
        const b = nearestBench(90);
        if (b) {
          text = 'Ngồi ghế (hồi máu ×2)';
          hintAct = () => goSit(b);
        }
      }
    } else if (me.sitting) {
      text = 'Đứng dậy';
      hintAct = () => { standUp(); drawActor(me); };
    }
    h.classList.toggle('hidden', !text);
    if (text && h.dataset.text !== text) {
      h.dataset.text = text;
      h.innerHTML = `<kbd>E</kbd>${esc(text)}`;
    }
  }

  // ---------- Camera ----------

  function layout() {
    const st = $('stage');
    const vw = st.clientWidth;
    const vh = st.clientHeight;
    // Chiều dọc vừa khung (điện thoại thì nới nhẹ để thấy rộng hơn), chiều ngang cuộn theo nhân vật.
    scale = Math.min(1.1, vh / SCENE_H);
    if (vw < 700) scale = Math.max(0.62, Math.min(scale, (vh / SCENE_H) * 0.9));
    $('world').style.width = SCENE_W + 'px';
    $('world').style.height = SCENE_H + 'px';
  }

  function camera(dt) {
    const st = $('stage');
    const vw = st.clientWidth / scale;
    const vh = st.clientHeight / scale;
    if (!manualCam) {
      const tx = Math.max(0, Math.min(SCENE_W - vw, me.x - vw / 2));
      camX += (tx - camX) * Math.min(1, dt * 6);
    }
    camX = Math.max(0, Math.min(Math.max(0, SCENE_W - vw), camX));
    // Khung cao hơn cảnh: đẩy cảnh xuống đáy (phía trên là trời); thấp hơn: bám theo nhân vật theo chiều dọc.
    if (vh >= SCENE_H) camY = SCENE_H - vh;
    else camY += (Math.max(0, Math.min(SCENE_H - vh, me.y - vh * 0.6)) - camY) * Math.min(1, dt * 6);
    $('world').style.transform = `scale(${scale}) translate(${-camX}px, ${-camY}px)`;
  }

  // ---------- Chim bồ câu, bướm ----------
  const pigeons = [];
  function buildPigeons() {
    for (let i = 0; i < 8; i++) {
      const el = document.createElement('img');
      el.className = 'pigeon';
      el.alt = '';
      el.draggable = false;
      el.src = '/img/toon/park/pigeon.png';
      $('things').appendChild(el);
      const p = { el, x: 1060 + Math.random() * 300, y: 700 + Math.random() * 90, tx: 0, ty: 0, wait: Math.random() * 3, peck: 0, fly: 0, flip: Math.random() < 0.5 };
      pickTarget(p);
      pigeons.push(p);
    }
  }
  function pickTarget(p) {
    for (let k = 0; k < 10; k++) {
      const x = 1000 + Math.random() * 420;
      const y = 690 + Math.random() * 120;
      if (free(x, y)) { p.tx = x; p.ty = y; return; }
    }
    p.tx = p.x;
    p.ty = p.y;
  }
  function stepPigeons(dt) {
    const all = [me, ...players.values()];
    for (const p of pigeons) {
      if (p.fly > 0) { // đang bay đi khi có người lại gần
        p.fly -= dt;
        p.y -= 160 * dt;
        p.x += (p.flip ? 120 : -120) * dt;
        p.el.style.opacity = String(Math.max(0, p.fly / 1.5));
        if (p.fly <= 0) { p.x = 1060 + Math.random() * 300; p.y = 700 + Math.random() * 90; p.el.style.opacity = '1'; p.wait = 4; }
      } else {
        if (all.some((a) => Math.hypot(a.x - p.x, a.y - p.y) < 70 && a.walking)) { p.fly = 1.5; p.flip = Math.random() < 0.5; }
        else if (p.wait > 0) {
          p.wait -= dt;
          p.peck -= dt;
          if (p.peck <= 0) { p.peck = 0.6 + Math.random(); p.el.dataset.peck = p.el.dataset.peck === '1' ? '0' : '1'; p.el.src = p.el.dataset.peck === '1' ? '/img/toon/park/pigeon_peck.png' : '/img/toon/park/pigeon.png'; }
        } else {
          const dx = p.tx - p.x;
          const dy = p.ty - p.y;
          const d = Math.hypot(dx, dy);
          if (d < 2) { p.wait = 1 + Math.random() * 3; pickTarget(p); } else {
            p.x += (dx / d) * 40 * dt;
            p.y += (dy / d) * 40 * dt;
            p.flip = dx > 0;
          }
        }
      }
      p.el.style.transform = `translate(${p.x - 18}px, ${p.y - 22}px) scaleX(${p.flip ? -1 : 1})`;
      p.el.style.zIndex = String(Math.round(p.y));
    }
  }
  function buildButterflies() {
    const spots = [[1030, 430], [1370, 430], [1030, 760], [1370, 760], [600, 820], [2200, 660]];
    spots.forEach(([x, y], i) => {
      const el = document.createElement('img');
      el.className = 'butterfly';
      el.src = '/img/toon/city/fx_butterfly.png';
      el.alt = '';
      el.style.left = x + 'px';
      el.style.top = y - 60 + 'px';
      el.style.animationDelay = `${-i * 1.7}s`;
      $('things').appendChild(el);
    });
  }

  // ---------- Người chơi khác ----------
  const players = new Map(); // id → {id, name, level, look, x, y, tx, ty, dir, walking, sitting, hospital, el, drawn}

  function addPlayer(p) {
    removePlayer(p.id);
    const a = { ...p, tx: p.x, ty: p.y, drawn: '' };
    a.el = document.createElement('div');
    a.el.className = 'actor other';
    a.el.dataset.actor = p.id;
    a.el.innerHTML = '<div class="tag"></div><div class="body"></div><div class="fx"></div>';
    a.el.addEventListener('click', (e) => {
      e.stopPropagation();
      openProfile(p.id);
    });
    a.el.addEventListener('pointerdown', (e) => e.stopPropagation());
    $('things').appendChild(a.el);
    setTag(a, a.name, a.level);
    players.set(p.id, a);
    drawActor(a);
    updateOnline();
  }
  function removePlayer(id) {
    const a = players.get(id);
    if (a) a.el.remove();
    players.delete(id);
    updateOnline();
  }
  function clearPlayers() {
    for (const id of [...players.keys()]) removePlayer(id);
  }
  function stepPlayers() {
    for (const a of players.values()) {
      const dx = a.tx - a.x;
      const dy = a.ty - a.y;
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) {
        if (a.x !== a.tx || a.y !== a.ty) { a.x = a.tx; a.y = a.ty; drawActor(a); }
        continue;
      }
      const far = Math.hypot(dx, dy) > 300;
      a.x = far ? a.tx : a.x + dx * 0.3;
      a.y = far ? a.ty : a.y + dy * 0.3;
      drawActor(a);
    }
  }
  function updateOnline() {
    const n = players.size + (you ? 1 : 0);
    $('park-count').textContent = you ? `${n} người trong công viên` : 'Đang vào công viên…';
  }
  const actorEl = (id) => (id === you ? me.el : players.get(id) && players.get(id).el);

  // ---------- Kết nối phòng công viên ----------
  let ws = null;
  let you = null;
  let retry = 0;
  let replaced = false; // phiên bị thay bởi tab khác của chính mình
  let lastSent = '';
  let unread = 0;

  function send(msg) {
    if (ws && ws.readyState === 1) ws.send(JSON.stringify(msg));
  }

  function connect() {
    if (ws || !A.token()) return;
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const sock = new WebSocket(`${proto}://${location.host}/ws/plaza`);
    ws = sock;
    sock.onopen = () => sock.send(JSON.stringify({ t: 'hello', token: A.token() }));
    sock.onmessage = (e) => {
      let m;
      try { m = JSON.parse(e.data); } catch (err) { return; }
      onMessage(m);
    };
    sock.onclose = () => {
      if (ws !== sock) return;
      ws = null;
      you = null;
      lastSent = '';
      clearPlayers();
      updateOnline();
      if (replaced) return; // tab khác của mình đã vào công viên: không tự nối lại để khỏi đá qua đá lại
      retry = Math.min(retry + 1, 6);
      setTimeout(connect, 1000 * Math.min(15, 2 ** retry));
    };
  }

  function onMessage(m) {
    switch (m.t) {
      case 'replaced':
        replaced = true;
        A.toast('Bạn đã mở Quảng trường ở nơi khác — tải lại trang để vào lại đây');
        break;
      case 'welcome':
        you = m.you;
        retry = 0;
        clearPlayers();
        (m.players || []).forEach(addPlayer);
        $('plaza-log').innerHTML = '';
        (m.chat || []).forEach((c) => logLine(c.name, c.text, false));
        lastSent = '';
        updateOnline();
        break;
      case 'join':
        addPlayer(m.player);
        logSystem(`${m.player.name} vừa vào công viên`);
        break;
      case 'leave':
        removePlayer(m.id);
        break;
      case 'moves':
        for (const v of m.list || []) {
          const a = players.get(v.id);
          if (!a) continue;
          a.tx = v.x;
          a.ty = v.y;
          if (a.dir !== v.dir || a.walking !== v.walking || a.sitting !== v.sitting) {
            Object.assign(a, { dir: v.dir, walking: v.walking, sitting: v.sitting });
            drawActor(a);
          }
        }
        break;
      case 'look': {
        const a = players.get(m.id);
        if (a) {
          Object.assign(a, { name: m.name, level: m.level, look: m.look, hospital: m.hospital });
          setTag(a, a.name, a.level);
          drawActor(a);
        }
        break;
      }
      case 'chat': {
        const mine = m.id === you;
        logLine(m.name, m.text, mine);
        showBubble(actorEl(m.id), m.text, BUBBLE_MS);
        if (!mine && !chatOpen()) setUnread(unread + 1);
        break;
      }
      case 'emote':
        showEmote(actorEl(m.id), m.kind);
        break;
      case 'gift':
        floatText(actorEl(m.to), `+${xu(m.amount)} xu`);
        if (m.to === you) A.toast(`${m.fromName} tặng bạn ${xu(m.amount)} xu`);
        else if (m.from === you) A.toast(`Đã tặng ${m.toName} ${xu(m.amount)} xu`);
        logSystem(`${m.fromName} tặng ${m.toName} ${xu(m.amount)} xu`);
        break;
      case 'balance':
        if (user) { user.balance = m.balance; renderHUD(); }
        break;
      case 'invite':
        showInvite(m);
        break;
      case 'error':
        A.toast(m.msg);
        break;
      case 'auth':
        A.requireLogin();
        break;
    }
  }

  function sendPosition() {
    if (!ws || !you) return;
    const msg = { t: 'move', x: Math.round(me.x * 10) / 10, y: Math.round(me.y * 10) / 10, dir: me.sitting ? 'down' : me.dir, walking: !!me.walking, sitting: !!me.sitting };
    const key = JSON.stringify(msg);
    if (key === lastSent) return;
    lastSent = key;
    send(msg);
  }

  // ---------- Chat ----------
  const chatOpen = () => !$('plaza-chat').classList.contains('collapsed');
  function openChat() {
    $('plaza-chat').classList.remove('collapsed');
    setUnread(0);
    $('plaza-input').focus();
    const log = $('plaza-log');
    log.scrollTop = log.scrollHeight;
  }
  function closeChat() {
    $('plaza-chat').classList.add('collapsed');
    $('plaza-input').blur();
  }
  function setUnread(n) {
    unread = n;
    const el = $('plaza-unread');
    el.textContent = n > 9 ? '9+' : String(n);
    el.classList.toggle('hidden', !n);
  }
  function logLine(name, text, mine) {
    const log = $('plaza-log');
    log.insertAdjacentHTML('beforeend', `<p class="${mine ? 'mine' : ''}"><b>${esc(name)}</b> ${esc(text)}</p>`);
    while (log.children.length > 60) log.firstElementChild.remove();
    log.scrollTop = log.scrollHeight;
  }
  function logSystem(text) {
    const log = $('plaza-log');
    log.insertAdjacentHTML('beforeend', `<p class="sys">${esc(text)}</p>`);
    log.scrollTop = log.scrollHeight;
  }

  function bindChat() {
    $('plaza-emotes').innerHTML = EMOTES.map((e, i) => `<button type="button" data-emote="${e.kind}" title="${e.name} (Shift+${i + 1})"><img src="${emoteImg(e.kind)}" alt="${e.name}" draggable="false"></button>`).join('');
    $('plaza-chat-toggle').onclick = () => (chatOpen() ? closeChat() : openChat());
    $('plaza-form').onsubmit = (e) => {
      e.preventDefault();
      const input = $('plaza-input');
      const text = input.value.trim();
      if (!text) return closeChat();
      if (!ws) return A.toast('Chưa kết nối được, thử lại sau');
      send({ t: 'chat', text });
      input.value = '';
    };
    $('plaza-input').addEventListener('keydown', (e) => {
      if (e.key === 'Escape') { e.preventDefault(); closeChat(); }
    });
    $('plaza-emotes').onclick = (e) => {
      const b = e.target.closest('[data-emote]');
      if (b) send({ t: 'emote', kind: b.dataset.emote });
    };
  }

  // ---------- Hồ sơ, tặng xu, mời chơi ----------
  function openProfile(id) {
    const p = players.get(id);
    if (!p) return;
    let dlg = $('plaza-profile');
    if (!dlg) {
      dlg = document.createElement('dialog');
      dlg.id = 'plaza-profile';
      dlg.className = 'modal plaza-profile';
      document.body.appendChild(dlg);
      dlg.addEventListener('click', (e) => { if (e.target === dlg) dlg.close(); });
    }
    const full = C && typeof C.svg === 'function' ? C.svg(p.look || {}, { size: 170 }) : ''; // ảnh toàn thân
    dlg.innerHTML = `<form method="dialog">
        <div class="pp-head">
          <div class="pp-art">${full}</div>
          <div class="pp-info">
            <h2>${esc(p.name)}</h2>
            <span class="pp-lv">Lv ${p.level || 1}</span>
            ${p.hospital ? '<span class="pp-tag">Đang nằm viện</span>' : p.sitting ? '<span class="pp-tag">Đang ngồi nghỉ</span>' : ''}
          </div>
        </div>
        <div class="pp-actions">
          <button type="button" data-act="wave"><img src="${emoteImg('wave')}" alt="">Vẫy tay</button>
          <button type="button" data-act="gift"><img src="${emoteImg('heart')}" alt="">Tặng xu</button>
          <button type="button" data-act="baucua">${ic('layers')}Mời Bầu cua</button>
          <button type="button" data-act="caro">${ic('play')}Mời Cờ Caro</button>
        </div>
        <div class="pp-gift hidden">
          <p class="muted">Tặng bao nhiêu xu? (tối đa 5.000 xu mỗi ngày)</p>
          <div class="pp-steps">${GIFT_STEPS.map((v) => `<button type="button" data-gift="${v}">${xu(v)} xu</button>`).join('')}</div>
        </div>
        <div class="actions"><button value="close">Đóng</button></div>
      </form>`;
    dlg.querySelector('.pp-actions').onclick = async (e) => {
      const b = e.target.closest('[data-act]');
      if (!b) return;
      const act = b.dataset.act;
      if (act === 'wave') {
        send({ t: 'emote', kind: 'wave' });
        send({ t: 'chat', text: `Chào ${p.name}!` });
        dlg.close();
      } else if (act === 'gift') {
        dlg.querySelector('.pp-gift').classList.toggle('hidden');
        const first = dlg.querySelector('[data-gift]');
        if (first) first.focus();
      } else if (act === 'baucua') {
        try {
          const r = await A.api('/api/baucua/rooms', { method: 'POST', body: {} });
          send({ t: 'invite', to: id, game: 'baucua', link: `/baucua/?room=${encodeURIComponent(r.code)}` });
          A.toast(`Đã mời ${p.name} — bàn ${r.code}`);
          dlg.close();
          location.href = `/baucua/?room=${encodeURIComponent(r.code)}`;
        } catch (err) {
          A.toast(err.message);
        }
      } else if (act === 'caro') {
        send({ t: 'invite', to: id, game: 'caro', link: '/caro/' });
        A.toast(`Đã mời ${p.name} chơi Cờ Caro`);
        dlg.close();
      }
    };
    dlg.querySelector('.pp-steps').onclick = async (e) => {
      const b = e.target.closest('[data-gift]');
      if (!b) return;
      const amount = Number(b.dataset.gift);
      dlg.close();
      if (await A.confirm(`Tặng ${p.name} ${xu(amount)} xu?`, { title: 'Tặng xu', ok: 'Tặng' })) send({ t: 'gift', to: id, amount });
    };
    dlg.showModal();
    const first = dlg.querySelector('[data-act]');
    if (first) first.focus();
  }

  function showInvite(m) {
    const box = $('plaza-invite');
    box.innerHTML = `<span><b>${esc(m.name)}</b> mời bạn chơi ${esc(GAME_NAME[m.game] || m.game)}</span>
      <a class="btn primary" href="${esc(m.link)}">Vào</a><button type="button" class="link" data-close>Bỏ qua</button>`;
    box.classList.remove('hidden');
    box.querySelector('[data-close]').onclick = () => box.classList.add('hidden');
    clearTimeout(box._t);
    box._t = setTimeout(() => box.classList.add('hidden'), 20000);
    logSystem(`${m.name} mời bạn chơi ${GAME_NAME[m.game] || m.game}`);
  }

  // ---------- HUD ----------
  function setUser(u) {
    if (!u) return;
    const changed = !user || JSON.stringify(look(user)) !== JSON.stringify(look(u)) || user.level !== u.level;
    user = u;
    renderHUD();
    if (changed) {
      me.look = look(u);
      me.drawn = '';
      setTag(me, u.name, u.level);
      drawActor(me);
      send({ t: 'look' });
    }
    if (!u.sitting && me.sitting && u.now) { /* server cho đứng (bị chữa / hết nhịp): đồng bộ */ }
  }

  function renderHUD() {
    if (!user) return;
    $('balance').textContent = xu(user.balance);
    $('user-name').textContent = user.name;
    $('user-avatar').innerHTML = C.head(look(user));
    const pct = Math.min(100, Math.round(((user.xp - user.levelXP) / Math.max(1, user.nextXP - user.levelXP)) * 100));
    $('level-badge').innerHTML = `<span class="lv">Lv ${user.level}</span><span class="xp-bar"><span style="width:${pct}%"></span></span>`;
    if (user.maxHp) {
      const hpPct = Math.round((user.hp / user.maxHp) * 100);
      const pill = $('hp-pill');
      pill.className = `hp-pill${hpPct < 40 ? ' low' : ''}${user.sitting ? ' regen' : ''}`;
      pill.title = user.sitting ? 'Đang ngồi nghỉ: hồi máu nhanh gấp đôi' : 'Máu';
      pill.innerHTML = `${ic('heart')}<span class="hp-bar"><span style="width:${hpPct}%"></span></span><b>${user.hp}/${user.maxHp}</b>${user.sitting ? '<em>×2</em>' : ''}`;
    }
  }

  // ---------- Điều khiển ----------
  const KEYMAP = {
    ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down',
    a: 'left', d: 'right', w: 'up', s: 'down', A: 'left', D: 'right', W: 'up', S: 'down',
  };
  const typing = (e) => {
    const t = e.target;
    return t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
  };

  function bindControls() {
    window.addEventListener('keydown', (e) => {
      if (typing(e) || e.ctrlKey || e.metaKey || e.altKey || document.querySelector('dialog[open]')) return;
      const k = KEYMAP[e.key];
      if (k && !e.shiftKey) {
        keys.add(k);
        e.preventDefault();
        return;
      }
      if ((e.key === 'e' || e.key === 'E' || e.key === 'Enter' || e.key === ' ') && hintAct) {
        e.preventDefault();
        hintAct();
        return;
      }
      if (e.key === 't' || e.key === 'T') {
        e.preventDefault();
        openChat();
        return;
      }
      const m = /^Digit([1-5])$/.exec(e.code || '');
      if (m && e.shiftKey) {
        e.preventDefault();
        send({ t: 'emote', kind: EMOTES[Number(m[1]) - 1].kind });
      }
    });
    window.addEventListener('keyup', (e) => {
      const k = KEYMAP[e.key];
      if (k) keys.delete(k);
    });
    window.addEventListener('blur', () => keys.clear());

    // Bấm để đi; kéo để xem ngang.
    const st = $('stage');
    let down = null;
    st.addEventListener('pointerdown', (e) => {
      down = { x: e.clientX, y: e.clientY, cam: camX, moved: false };
    });
    window.addEventListener('pointermove', (e) => {
      if (!down) return;
      const dx = e.clientX - down.x;
      if (Math.abs(dx) > 8) down.moved = true;
      if (down.moved) {
        manualCam = true;
        camX = down.cam - dx / scale;
      }
    });
    window.addEventListener('pointerup', (e) => {
      const d = down;
      down = null;
      if (!d || d.moved || e.target.closest('.actor.other, .bench, .park-bar, .plaza-chat')) return;
      if (!st.contains(e.target)) return;
      const r = st.getBoundingClientRect();
      const wx = (e.clientX - r.left) / scale + camX;
      const wy = (e.clientY - r.top) / scale + camY;
      if (Math.hypot(wx - GATE.x, wy - (GATE.y - 40)) < 90) return walkTo(GATE.x, GATE.y, () => (location.href = '/'));
      walkTo(wx, wy);
    });
    $('btn-sit').onclick = () => goSit(nearestBench());
    addEventListener('resize', layout);
  }

  // ---------- Vòng lặp ----------
  let last = performance.now();
  let sendAcc = 0;
  function frame(now) {
    const dt = Math.min(0.05, (now - last) / 1000);
    last = now;
    step(dt);
    stepPlayers();
    stepPigeons(dt);
    camera(dt);
    sendAcc += dt * 1000;
    if (sendAcc >= SEND_EVERY) { sendAcc = 0; sendPosition(); }
    requestAnimationFrame(frame);
  }

  async function start() {
    $('logo').innerHTML = I.logo ? I.logo(28) : '';
    if (I.render) I.render();
    let u;
    try { u = await A.me(); } catch (e) { A.toast(e.message); }
    if (!u) return A.requireLogin();
    if (u.hospitalUntil && u.hospitalUntil > u.now) {
      A.toast('Bạn đang nằm viện, chưa ra công viên được');
      setTimeout(() => (location.href = '/'), 1500);
      return;
    }
    $('park').classList.remove('hidden');
    await loadManifests();
    buildScene();
    setUser(u);
    if (u.sitting) A.api('/api/me/sit', { method: 'POST', body: { sit: false } }).then(setUser, () => {});
    bindChat();
    bindControls();
    layout();
    camX = Math.max(0, me.x - 400);
    drawActor(me);
    connect();
    requestAnimationFrame(frame);
    setTimeout(() => $('park-help').classList.add('faded'), 12000);
    // Máu tự hồi: cập nhật HUD định kỳ.
    setInterval(() => A.me().then(setUser, () => {}), 30000);
  }

  // Cho test.
  window.Park = { players, me, benches, goSit, walkTo, openProfile, send, get you() { return you; } };

  start();
})();
