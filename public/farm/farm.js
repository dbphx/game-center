// Nông trại DCity: ruộng ô đất, kho giống, kho thóc (kho sản phẩm), chuồng gia súc, ao cá trong cùng một cảnh; nhân vật đi lại
// bằng phím (WASD / mũi tên) hoặc chuột, tới nơi mới thao tác — giống game "Avatar". Mọi luật do server quyết định
// (/api/farm/...), trang này chỉ hiển thị, đếm ngược và gửi thao tác.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const esc = A.escapeHtml;

  const IMG = '/img/toon/farm/';

  // ---------- Hình ----------
  // Ảnh hoạt hình CC0 trong public/img/toon/farm (xem manifest.json, CREDITS.csv) — phong cách game "Avatar".
  let sizes = {}; // tên file (không đuôi) → [w, h], đọc từ manifest.json

  const art = {
    // Ảnh theo chiều cao mong muốn (giữ tỉ lệ gốc). k = hệ số phóng so với kích thước gốc khi không truyền h.
    img(name, { h, k = 1, cls = '' } = {}) {
      const [w0, h0] = sizes[name] || [64, 64];
      const hh = h || Math.round(h0 * k);
      const ww = Math.round((w0 / h0) * hh);
      return `<img class="toon ${cls}" src="${IMG}${name}.png" width="${ww}" height="${hh}" alt="" draggable="false">`;
    },
    // Cây theo giai đoạn: 1 mầm, 2 đang lớn (3 = sắp chín, cùng hình 2 nhưng to hơn), 'ripe' chín.
    crop(id, stage) {
      if (stage === 'ripe') return art.img(`crop_${id}_ripe`, { cls: 'crop-img' });
      return art.img(`crop_${id}_${stage === 1 ? 1 : 2}`, { cls: `crop-img${stage === 3 ? ' big' : ''}` });
    },
    item: (id, h = 44) => art.img(`item_${id}`, { h }),
    animal: (kind, h = 56) => art.img(`animal_${kind}`, { h }),
    fish: (kind, h = 26) => art.img(`fish_${kind}`, { h }),
    // Hạt giống: túi hạt có hình nông sản.
    seed: (id, h = 44) => art.img(`seed_${id}`, { h }),
  };

  // ---------- Trạng thái ----------

  let user = null;
  let cat = null; // catalog
  let farm = null;
  let offset = 0; // giờ server − giờ máy (giây)
  let shopMode = { tab: 'crops', plot: null };
  let storeMode = 'seeds';
  let penMode = 'barn';
  let buyQty = 1; // số hạt mỗi lần mua
  let tool = null; // công cụ đang cầm: null | 'plant' | 'harvest'
  let seedSel = null; // hạt đang chọn khi cầm công cụ gieo

  const nowS = () => Date.now() / 1000 + offset;
  const xu = A.xu;
  const find = (list, id) => (cat ? cat[list].find((x) => x.id === id) : null) || { id, name: id, units: 1 };
  const EMPTY_STORE = { capacity: 0, used: 0, maxCapacity: 0, nextExpandPrice: 0, items: {} };
  const store = (kind) => (farm && farm.storage && farm.storage[kind]) || EMPTY_STORE;

  // Món trong kho sản phẩm: nông sản, vật nuôi hoặc cá.
  function product(id) {
    for (const list of ['crops', 'animals', 'fish']) {
      const it = cat && cat[list].find((x) => x.id === id);
      if (!it) continue;
      const draw = (h) => (list === 'crops' ? art.item(id, h) : list === 'animals' ? art.animal(id, h) : art.fish(id, Math.round(h * 0.6)));
      return { ...it, list, draw };
    }
    return { id, name: id, sellPrice: 0, list: 'crops', draw: (h) => art.item(id, h) };
  }

  function fmtDur(sec) {
    sec = Math.max(0, Math.round(sec));
    if (sec < 60) return `${sec} giây`;
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (!h) return `${m} phút`;
    return m ? `${h} giờ ${m} phút` : `${h} giờ`;
  }

  function countdown(sec) {
    sec = Math.max(0, Math.ceil(sec));
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    const pad = (n) => String(n).padStart(2, '0');
    return h ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
  }

  // Tiến độ 0..1 của một thứ đang lớn.
  const progress = (start, ready) => (ready <= start ? 1 : Math.min(1, Math.max(0, (nowS() - start) / (ready - start))));

  async function call(path, body) {
    try {
      const data = await A.api(path, body === undefined ? {} : { method: 'POST', body });
      if (data.user) user = data.user;
      if (data.farm) setFarm(data.farm);
      renderHUD();
      return data;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      // Thiếu máu (403 "cần ít nhất 40 HP") hoặc đang nằm viện (423): chỉ đường về Bệnh viện.
      const hp = e.status === 423 || (e.status === 403 && /HP/.test(e.message));
      A.toast(hp ? `${e.message} — ghé Bệnh viện trên bản đồ để hồi máu` : e.message);
      if (hp) A.me().then((u) => u && ((user = u), renderHUD())).catch(() => {});
      return null;
    }
  }

  function setFarm(f) {
    farm = f;
    offset = f.now - Date.now() / 1000;
    render();
  }

  // ---------- Topbar ----------

  // Diện mạo nhân vật: giới tính, trang phục, tóc, cánh.
  const lookOf = (u) => ({ gender: u.gender, outfit: u.outfit, hair: u.hair, wings: u.wings, face: u.face });

  function renderHUD() {
    if (!user) return;
    $('balance').textContent = xu(user.balance);
    $('user-name').textContent = user.name;
    $('user-avatar').innerHTML = C.head(lookOf(user));
    const pct = Math.min(100, Math.round(((user.xp - user.levelXP) / Math.max(1, user.nextXP - user.levelXP)) * 100));
    $('level-badge').innerHTML = `<span class="lv">Lv ${user.level}</span><span class="xp-bar"><span style="width:${pct}%"></span></span>`;
    $('level-badge').title = `${user.xp - user.levelXP} / ${user.nextXP - user.levelXP} XP để lên level ${user.level + 1}`;
    $('farm-name').textContent = `Nông trại của ${user.name}`;
    if (user.maxHp) {
      const hpPct = Math.round((user.hp / user.maxHp) * 100);
      const sick = user.hospitalUntil && user.hospitalUntil > user.now;
      const pill = $('hp-pill');
      pill.className = `hp-pill${hpPct < 40 || sick ? ' low' : ''}`;
      pill.title = sick ? 'Đang nằm viện' : 'Làm nông cần ít nhất 40 HP';
      pill.innerHTML = `${I.svg('heart')}<span class="hp-bar"><span style="width:${hpPct}%"></span></span><b>${user.hp}/${user.maxHp}</b>`;
    }
    $('me-tag').innerHTML = `<span class="lv">Lv ${user.level}</span>${esc(user.name)}`;
    if (JSON.stringify(me.look) !== JSON.stringify(lookOf(user))) {
      me.look = lookOf(user);
      me.drawn = '';
      drawMe();
    }
  }

  // ---------- Bố cục cảnh ----------
  // Toạ độ "thế giới" (px): cao cố định H. Từ trái sang phải như nông trại game Avatar:
  // CHUỒNG gia súc ở mép trái · hai kho ở rìa sát hàng rào sau + cổng "Vào" phía dưới · QUỸ ĐẤT 4 khối ruộng
  // (lưới 2×2, mỗi khối 12 ô = 3 hàng × 4 cột, đủ 48 ô; ô chưa mở hiện mờ) · AO CÁ ở mép phải. Nền đất be trơn.
  // Mọi vật đặt tuyệt đối, z-index = y chân để nhân vật đứng trước / sau đúng chiều sâu.

  const H = 740; // phần dưới y 610 là bụi cây + cỏ (thanh công cụ nổi đè lên)
  const SKY = 130; // dải trời + hàng rào phía sau
  const BOTTOM = 610; // mép dưới vùng đi lại
  const PLOT = 56;
  const PITCH = 60;
  const BLOCK_COLS = 4;
  const BLOCK_ROWS = 3;
  const PER_BLOCK = BLOCK_COLS * BLOCK_ROWS;
  const BLOCK_W = BLOCK_COLS * PITCH + 16;
  const BLOCK_H = BLOCK_ROWS * PITCH + 18;
  const BX = [744, 744 + BLOCK_W + 48]; // cột khối ruộng (lối đất giữa đủ rộng để đi)
  const BY = [158, 158 + BLOCK_H + 36]; // hàng khối ruộng
  const PEN = { x1: 18, y1: 164, x2: 400, y2: 600 }; // chuồng gia súc (hàng rào)
  const POND_BOX = { x: BX[1] + BLOCK_W + 40, y: 250, w: 320, h: 230 }; // lô cỏ có ao
  const W = POND_BOX.x + POND_BOX.w + 30;
  const CORE_W = W; // cả nông trại luôn vừa một khung nhìn theo chiều ngang (dọc cố định)
  let OY = 0; // khoảng trời thêm phía trên khi khung nhìn cao hơn cảnh
  let objs = {}; // key → { x, y (chỗ đứng), face, hx, hy (gợi ý), verb, run() }
  let obstacles = []; // [x1, y1, x2, y2] chân nhân vật không được vào

  function tiles() {
    const owned = farm.plots.length;
    const max = farm.maxPlots || 48;
    return { owned, max, buy: owned < max && !!farm.nextPlotPrice, left: max - owned };
  }

  function computeLayout() {
    obstacles = [
      [PEN.x1, PEN.y1, PEN.x2, PEN.y2], // chuồng (hàng rào)
      [416, 262, 586, 296], // kho thóc
      [592, 270, 708, 296], // kho giống
      [522, 432, 566, 450], // giếng
      [640, 432, 664, 450], // gốc cây ăn trái
      [520, 592, 544, 608], // cột cổng trái
      [628, 592, 652, 608], // cột cổng phải
      [POND_BOX.x + 14, POND_BOX.y + 22, POND_BOX.x + POND_BOX.w - 14, POND_BOX.y + POND_BOX.h - 12], // ao
    ];
    // 4 khối ruộng: không đi xuyên qua giữa các ô.
    for (const by of BY) for (const bx of BX) obstacles.push([bx, by, bx + BLOCK_W, by + BLOCK_H]);
  }

  const pos = (x, y, z = y) => `left:${Math.round(x)}px;top:${Math.round(y)}px;z-index:${Math.round(z)}`;
  const capBar = (s) => `<span class="cap-mini"><span style="width:${Math.round((s.used / Math.max(1, s.capacity)) * 100)}%"></span></span>`;

  // Mỗi ô: nền đất (hoặc cỏ khi chưa mở), cây đứng trên ô, nhãn nhỏ phía dưới.
  // Mỗi ô có 2 lớp: lớp MẶT ĐẤT (nền đất + nhãn, z thấp cố định — luôn nằm dưới nhân vật) và lớp CÂY
  // (z theo chân ô, để nhân vật đứng sau cây cao thì bị cây che đúng chiều sâu).
  function plotHTML(p, x, y) {
    const ground = `data-obj="plot${p.index}" style="${pos(x, y, 5)}"`;
    const top = (cls, inner) => `<div class="plot-top ${cls}" data-obj="plot${p.index}" style="${pos(x, y, y + PLOT)}">${inner}</div>`;
    if (!p.crop) {
      return `<div class="plot empty" ${ground} title="Gieo hạt">
        <span class="soil"></span><span class="tag-pill add">${I.svg('plus')}Gieo hạt</span></div>`;
    }
    const c = find('crops', p.crop);
    const pr = progress(p.plantedAt, p.readyAt);
    if (pr >= 1) {
      return `<div class="plot ripe" ${ground} title="Thu hoạch ${esc(c.name)}">
        <span class="soil"></span><span class="tag-pill go">Thu hoạch</span></div>
        ${top('ripe', `<span class="crop">${art.crop(p.crop, 'ripe')}</span>
        <span class="harvest-arrow">${I.svg('chevronDown')}</span>
        <span class="twinkle t1">${I.svg('sparkles')}</span><span class="twinkle t2">${I.svg('sparkles')}</span>`)}`;
    }
    const stage = pr < 0.34 ? 1 : pr < 0.67 ? 2 : 3;
    return `<div class="plot growing" ${ground} data-start="${p.plantedAt}" data-ready="${p.readyAt}" data-stage="${stage}"
        title="${esc(c.name)}">
      <span class="soil wet"></span>
      <span class="tag-pill"><span data-left>${countdown(p.readyAt - nowS())}</span>
        <span class="bar"><span style="width:${Math.round(pr * 100)}%"></span></span></span></div>
      ${top('growing', `<span class="crop">${art.crop(p.crop, stage)}</span>`)}`;
  }


  // Quỹ đất: 4 khối × 12 ô. Ô đã có trồng được; ô kế tiếp là chỗ mua; còn lại là đất để dành (mờ).
  function plotXY(i) {
    const b = Math.floor(i / PER_BLOCK);
    const k = i % PER_BLOCK;
    return [BX[b % 2] + 8 + (k % BLOCK_COLS) * PITCH, BY[Math.floor(b / 2)] + 10 + Math.floor(k / BLOCK_COLS) * PITCH];
  }

  // Chỗ đứng để làm một ô: mép khối gần ô nhất (trên / dưới / trái / phải), quay mặt về phía ô.
  function edgeSpot(i, x, y) {
    const b = Math.floor(i / PER_BLOCK);
    const bx = BX[b % 2];
    const by = BY[Math.floor(b / 2)];
    const cx = x + PLOT / 2;
    const cy = y + PLOT / 2;
    const c = [
      { x: cx, y: by - 8, face: 'down' },
      { x: cx, y: by + BLOCK_H + 10, face: 'up' },
      { x: bx - 16, y: cy + 14, face: 'right' },
      { x: bx + BLOCK_W + 16, y: cy + 14, face: 'left' },
    ];
    return c.reduce((best, s) => (Math.hypot(s.x - cx, s.y - cy) < Math.hypot(best.x - cx, best.y - cy) ? s : best));
  }

  function fieldHTML() {
    const t = tiles();
    const out = [];
    for (let b = 0; b < Math.ceil(t.max / PER_BLOCK); b++) {
      const used = t.owned > b * PER_BLOCK || (t.buy && t.owned === b * PER_BLOCK);
      const x = BX[b % 2];
      const y = BY[Math.floor(b / 2)];
      out.push(`<span class="field-bed${used ? '' : ' reserved'}" style="left:${x}px;top:${y}px;width:${BLOCK_W}px;height:${BLOCK_H}px;z-index:3"></span>`);
      if (!used) out.push(`<span class="block-sign" style="${pos(x + BLOCK_W / 2, y + BLOCK_H / 2 + 12, 4)}">Đất để dành</span>`);
    }
    farm.plots.forEach((p, i) => {
      const [x, y] = plotXY(i);
      out.push(plotHTML(p, x, y));
      objs[`plot${p.index}`] = { ...edgeSpot(i, x, y), hx: x + PLOT / 2, hy: y - 6, verb: plotVerb(p), run: () => plotAction(p.index) };
    });
    for (let i = t.owned; i < t.max; i++) {
      const [x, y] = plotXY(i);
      if (i === t.owned && t.buy) {
        out.push(`<div class="plot buy" data-obj="buyplot" style="${pos(x, y, y + PLOT)}" title="Mở thêm ô đất">
          <span class="soil grass"></span><span class="lock">${I.svg('lock')}</span>
          <span class="tag-pill price">${I.svg('coins')}${xu(farm.nextPlotPrice)}</span></div>`);
        objs.buyplot = { ...edgeSpot(i, x, y), hx: x + PLOT / 2, hy: y - 6, verb: 'Mở ô đất', run: buyPlot };
      } else {
        out.push(`<span class="plot reserved" style="${pos(x, y, y + PLOT)}"><span class="soil"></span></span>`);
      }
    }
    out.push(`<span class="plot-count" style="${pos(BX[0] + 4, BY[0] - 26, 2000)}">${t.owned}/${t.max} ô đất</span>`);
    return out.join('');
  }

  function plotVerb(p) {
    if (!p.crop) return tool === 'plant' && seedSel ? `Gieo ${find('crops', seedSel).name}` : 'Gieo hạt';
    return nowS() >= p.readyAt ? 'Thu hoạch' : 'Xem cây';
  }

  // Kho: ảnh công trình, nhãn tên + thanh sức chứa phía trên (giống bảng tên trong game).
  function storeObjHTML(kind, x, footY, image, h) {
    const s = store(kind);
    const name = kind === 'seeds' ? 'Kho giống' : 'Kho thóc';
    const full = s.capacity && s.used >= s.capacity;
    return `<div class="obj building${full ? ' full' : ''}" data-obj="${kind}" style="${pos(x, footY)}" title="${name}">
      <span class="obj-label"><b>${name}</b>${capBar(s)}<em>${s.used}/${s.capacity}</em></span>
      ${art.img(image, { h })}</div>`;
  }

  function penLabel(kind, pen) {
    const name = kind === 'barn' ? 'Chuồng gia súc' : 'Ao cá';
    if (!pen) {
      const cfg = cat ? cat[kind] : { price: 0, level: 1 };
      const locked = user.level < cfg.level;
      return `<span class="obj-label offer"><b>${name}</b><em>${locked ? `${I.svg('lock')}Cần Lv ${cfg.level}` : `${I.svg('coins')}Mua ${xu(cfg.price)}`}</em></span>`;
    }
    return `<span class="obj-label"><b>${name}</b>${capBar(pen)}<em>${pen.used}/${pen.capacity}</em></span>`;
  }

  function yardHTML() {
    const pen = farm.barn;
    const w = PEN.x2 - PEN.x1;
    const h = PEN.y2 - PEN.y1;
    // Nền cỏ chuồng nằm dưới cùng; chòi + hàng rào trước là vật riêng để con vật đứng trước / sau đúng chiều sâu.
    return `<div class="obj area yard${pen ? '' : ' ghost'}" data-obj="barn" style="left:${PEN.x1}px;top:${PEN.y1}px;width:${w}px;height:${h}px;z-index:3">
        <span class="fence-top"></span></div>
      <span class="pen-label" data-obj="barn" style="left:${PEN.x1 + w / 2}px;top:${PEN.y1 - 8}px;z-index:2000">${penLabel('barn', pen)}</span>
      <span class="obj decor pen-shed${pen ? '' : ' ghost'}" data-obj="barn" style="${pos(150, 300)}">${art.img('pen_shed', { h: 140 })}</span>
      <span class="obj decor" style="${pos(330, 300)}">${art.img('hay', { h: 44 })}</span>
      <span class="obj decor" style="${pos(280, 300)}">${art.img('trough_water', { h: 26 })}</span>
      <span class="fence-thick" style="left:${PEN.x1}px;top:${PEN.y2 - 52}px;width:${w}px;z-index:${PEN.y2}"></span>`;
  }

  function pondHTML() {
    const pen = farm.pond;
    const b = POND_BOX;
    return `<div class="obj area pond-area${pen ? '' : ' ghost'}" data-obj="pond" style="left:${b.x}px;top:${b.y}px;width:${b.w}px;height:${b.h}px;z-index:${b.y + 40}">
      ${penLabel('pond', pen)}
      <span class="deco" style="left:0;bottom:2px">${art.img('flowers', { h: 40 })}</span>
      <span class="deco" style="right:0;bottom:2px">${art.img('bush_3', { h: 34 })}</span>
      <div class="pond-water">${art.img('pond', { cls: 'pond-img' })}</div>
    </div>`;
  }

  // Đồ trang trí đặt theo chân (x, y).
  const deco = (name, x, y, h, flip = false, cls = '') =>
    `<span class="obj decor${flip ? ' flip' : ''}${cls ? ' ' + cls : ''}" data-tree="${/^tree_(mai|dao)/.test(name) ? name.slice(5) : ''}" style="${pos(x, y)}">${art.img(name, { h })}</span>`;

  // Khóm hoa nhỏ nhiều màu rải trên các mảng cỏ (ngẫu nhiên cố định theo seed) — lớp mặt đất dưới nhân vật.
  function flowersHTML() {
    let seed = 20240214;
    const r = () => ((seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296);
    const colors = ['red', 'yellow', 'pink', 'purple', 'white'];
    const zones = [
      [PEN.x1 + 24, 340, PEN.x2 - 24, PEN.y2 - 30, 10], // trong chuồng
      [480, 362, 700, 455, 5], // lô cỏ giữa
      [POND_BOX.x + 10, POND_BOX.y + POND_BOX.h - 26, POND_BOX.x + POND_BOX.w - 10, POND_BOX.y + POND_BOX.h - 8, 6], // bờ ao dưới
      [POND_BOX.x + 10, POND_BOX.y + 6, POND_BOX.x + POND_BOX.w - 10, POND_BOX.y + 18, 3], // bờ ao trên
      [16, SKY + 10, W - 16, SKY + 26, 18], // dải cỏ dưới hàng rào
    ];
    const out = [];
    for (const [x1, y1, x2, y2, n] of zones) {
      for (let i = 0; i < n; i++) {
        const x = x1 + r() * (x2 - x1);
        const y = y1 + r() * (y2 - y1);
        const c = colors[Math.floor(r() * colors.length)];
        out.push(`<span class="flower-clump" style="${pos(x, y, 4)}">${art.img(`flower_${c}`, { h: Math.round(26 + r() * 10) })}</span>`);
      }
    }
    return out.join('');
  }

  // Cánh mai / đào thỉnh thoảng rơi từ tán cây.
  function dropPetal() {
    const trees = document.querySelectorAll('[data-tree="mai"], [data-tree="dao"]');
    if (!trees.length || document.hidden) return;
    const t = trees[Math.floor(Math.random() * trees.length)];
    const img = t.querySelector('img');
    const x = t.offsetLeft + (Math.random() - 0.5) * img.width * 0.8;
    const y = t.offsetTop - img.height * (0.45 + Math.random() * 0.4);
    const el = document.createElement('img');
    el.className = 'petal';
    el.src = `${IMG}petal_${t.dataset.tree}.png`;
    el.alt = '';
    el.style.left = `${Math.round(x)}px`;
    el.style.top = `${Math.round(y)}px`;
    el.style.setProperty('--dx', `${Math.round((Math.random() - 0.3) * 70)}px`);
    el.style.setProperty('--fall', `${Math.round(img.height * (0.5 + Math.random() * 0.4))}px`);
    el.style.animationDuration = `${3 + Math.random() * 2}s`;
    $('pets').appendChild(el);
    setTimeout(() => el.remove(), 5200);
  }
  setInterval(dropPetal, 700);

  function renderWorld() {
    computeLayout();
    objs = {};
    const world = $('world');
    const sh = $('scene').clientHeight;
    OY = sh ? Math.max(0, Math.floor(sh / zoom - H)) : 0;
    world.style.width = `${W}px`;
    world.style.height = `${H + OY}px`;
    world.style.transform = zoom === 1 ? '' : `scale(${zoom})`;
    world.style.setProperty('--oy', `${OY}px`);
    $('layer').style.top = `${OY}px`;
    $('layer').style.height = `${H}px`;
    const wrap = $('world-wrap');
    wrap.style.width = `${W * zoom}px`;
    wrap.style.height = `${(H + OY) * zoom}px`;

    const parts = [];
    // Trời phía trên (khi khung cao) có thêm mây.
    if (OY > 60) {
      parts.push(`<div class="sky-top" style="top:${-OY}px;width:${W}px;height:${OY + 2}px">
        ${[0.12, 0.5, 0.83].map((k, i) => `<span class="cloud c${((i + 1) % 3) + 1}" style="left:${Math.round(W * k)}px;top:${Math.round(OY * 0.25 + i * 14)}px"></span>`).join('')}</div>`);
    }
    // Dải nền xa (đồi cây + đồng ruộng) kéo lên phần trời thừa, cao tối đa 170px.
    const E = Math.min(OY, 170);
    parts.push(`<div class="sky" style="top:${-E}px;width:${W}px;height:${SKY + 6 + E}px;--far:${Math.min(210, Math.round(E * 0.75 + 96))}px">
      ${[0.04, 0.36, 0.7].map((k, i) => `<span class="cloud c${(i % 3) + 1}" style="left:${Math.round(W * k)}px"></span>`).join('')}
      <span class="fence-row"></span></div>`);
    parts.push(`<span class="grass-edge" style="left:0;top:${SKY}px;width:${W}px;z-index:3"></span>`);
    for (const x of [300, 720, BX[1] + 40, W - 120]) parts.push(`<span class="obj decor far" style="${pos(x, SKY + 10, 2)}">${art.img('tree_big', { h: 150 })}</span>`);
    parts.push(`<span class="grass-edge bottom" style="left:0;top:${BOTTOM - 4}px;width:${W}px;height:${H - BOTTOM + 4}px;z-index:4"></span>`);
    parts.push(`<span class="bush-row" style="left:0;top:${BOTTOM - 10}px;width:${W}px;z-index:${H}"></span>`);

    // Chuồng gia súc ở mép trái.
    parts.push(yardHTML());
    objs.barn = { x: PEN.x2 + 22, y: 470, face: 'left', hx: 210, hy: PEN.y1 + 14, verb: farm.barn ? 'Xem chuồng' : 'Mua chuồng', run: () => openPen('barn') };

    // Hai kho ở rìa: sát hàng rào sau, cạnh chuồng.
    parts.push(storeObjHTML('products', 500, 296, 'hut_rice', 140));
    parts.push(storeObjHTML('seeds', 650, 296, 'hut_seed', 120));
    objs.products = { x: 500, y: 316, face: 'up', hx: 500, hy: 128, verb: 'Mở kho', run: () => openStore('products') };
    objs.seeds = { x: 650, y: 316, face: 'up', hx: 650, hy: 148, verb: 'Mở kho', run: () => openStore('seeds') };

    // Lô cỏ nhỏ giữa (giếng, cây ăn trái, hoa) và gà con đi dạo.
    parts.push(`<span class="patch" style="left:470px;top:350px;width:236px;height:112px;z-index:3"></span>`);
    parts.push(deco('well', 544, 450, 72));
    parts.push(deco('tree_mai', 654, 452, 132, false, 'sway blossom'));
    parts.push(deco('flowers', 494, 400, 30));
    for (const [x, y, k, d] of [[470, 520, 'chicken_white', 9], [600, 540, 'chicken_white', 12], [720, 330, 'chicken', 10]]) {
      parts.push(`<span class="obj decor stroll" style="${pos(x, y)};--dur:${d}s">${art.img(`animal_${k}`, { h: 30 })}</span>`);
    }

    // Cổng "Vào" dưới giữa (giữa kho và ruộng).
    // z của cổng thấp hơn chỗ đứng trước cổng (y 604) để nhân vật đứng TRƯỚC biển cổng.
    parts.push(`<div class="obj gate" data-obj="gate" style="${pos(586, 610, 592)}" title="Về thành phố">
      ${art.img('farm_gate', { h: 140 })}<span class="gate-sign">Nông trại<b>${esc(user.name)}</b></span></div>`);
    objs.gate = { x: 586, y: 604, face: 'down', hx: 586, hy: 460, verb: 'Về thành phố', run: () => (location.href = '/') };

    parts.push(fieldHTML());

    // Ao cá ở mép phải.
    parts.push(pondHTML());
    objs.pond = { x: POND_BOX.x - 18, y: POND_BOX.y + 130, face: 'right', hx: POND_BOX.x + POND_BOX.w / 2, hy: POND_BOX.y + 8, verb: farm.pond ? 'Xem ao' : 'Mua ao', run: () => openPen('pond') };

    // Cây kiểu Việt ở rìa: mai, đào hai bên cổng, đào trong chuồng, chuối + tre sau ao, mai góc phải.
    parts.push(deco('tree_mai', 448, 606, 150, false, 'sway blossom'));
    parts.push(deco('tree_dao', 688, 606, 112, true, 'sway blossom'));
    parts.push(deco('tree_dao', 352, 322, 150, false, 'sway blossom'));
    parts.push(deco('tree_chuoi', POND_BOX.x + 60, 246, 136, false, 'sway'));
    parts.push(deco('tree_tre', POND_BOX.x + POND_BOX.w - 30, 246, 160, false, 'sway slow'));
    parts.push(deco('tree_mai', W - 26, 606, 140, true, 'sway blossom'));
    parts.push(flowersHTML());

    $('objects').innerHTML = parts.join('');
    syncPets();
    // Nhân vật có thể đang đứng chỗ mới bị chặn: đưa về chỗ trống gần nhất.
    if (blocked(me.x, me.y)) {
      const [x, y] = nearestFree(me.x, me.y);
      me.x = x;
      me.y = y;
    }
    drawMe();
    updateHint();
  }

  // ---------- Thanh công cụ ----------

  function renderToolbar() {
    const seeds = store('seeds');
    const prods = store('products');
    const ripe = farm ? farm.plots.filter((p) => p.crop && nowS() >= p.readyAt).length : 0;
    const badge = (n) => (n ? `<span class="badge">${n}</span>` : '');
    const btn = (attr, pic, label, extra = '', on = false) =>
      `<button class="tool${on ? ' on' : ''}" ${attr} title="${label}"><span class="ic">${pic}</span><b>${label}</b>${extra}</button>`;
    const seedPic = seedSel ? art.seed(seedSel, 40) : art.seed('carrot', 40);
    $('toolbar').innerHTML = [
      btn('data-tool="plant"', seedPic, tool === 'plant' && seedSel ? find('crops', seedSel).name : 'Gieo hạt', '', tool === 'plant'),
      btn('data-tool="harvest"', art.item('corn', 40), 'Thu hoạch', badge(ripe), tool === 'harvest'),
      btn('data-go="seeds"', art.img('hut_seed', { h: 42 }), 'Kho giống', badge(seeds.used)),
      btn('data-go="products"', art.img('hut_rice', { h: 36 }), 'Kho thóc', badge(prods.used)),
      btn('data-shop', art.img('wheelbarrow', { h: 36 }), 'Cửa hàng'),
      btn('data-go="gate"', art.img('farm_gate', { h: 38 }), 'Về thành phố'),
    ].join('');
    $('scene').classList.toggle('tool-plant', tool === 'plant');
    $('scene').classList.toggle('tool-harvest', tool === 'harvest');
    const tip = $('tool-tip');
    tip.classList.toggle('hidden', !tool);
    if (tool === 'plant' && seedSel) {
      const have = seeds.items[seedSel] || 0;
      const c = find('crops', seedSel);
      tip.innerHTML = `${art.seed(seedSel, 26)}<span>Đang cầm hạt <b>${esc(c.name)}</b> ${have ? `(còn ${have} trong kho)` : `(hết hạt — mỗi ô mua ${xu(c.seedPrice || 0)} xu)`} · bấm ô đất trống để gieo</span><button class="link" data-tool="none">Bỏ</button>`;
    } else if (tool === 'harvest') {
      tip.innerHTML = `${art.item('corn', 26)}<span>Đang cầm liềm · bấm ô cây chín để thu hoạch (${ripe} ô chín)</span><button class="link" data-tool="none">Bỏ</button>`;
    }
  }

  function closeSeedPick() {
    $('seed-pick').classList.add('hidden');
  }

  function pickTool(t) {
    if (t === 'none' || (t === tool && t !== 'plant')) {
      tool = null;
      closeSeedPick();
    } else if (t === 'plant') {
      // Mở bảng chọn hạt (hạt có trong kho lên đầu).
      const pick = $('seed-pick');
      if (!pick.classList.contains('hidden')) return closeSeedPick();
      renderSeedPick();
      pick.classList.remove('hidden');
      return;
    } else {
      tool = t;
      closeSeedPick();
    }
    renderToolbar();
  }

  function renderSeedPick() {
    if (!cat) return;
    const seeds = store('seeds');
    const crops = cat.crops.filter((c) => user.level >= c.level).sort((a, b) => (seeds.items[b.id] || 0) - (seeds.items[a.id] || 0));
    $('seed-pick').innerHTML = `<p>Chọn hạt để gieo</p><div class="chips">${crops
      .map((c) => {
        const have = seeds.items[c.id] || 0;
        return `<button class="chip${seedSel === c.id && tool === 'plant' ? ' on' : ''}" data-seed-sel="${c.id}">${art.seed(c.id, 40)}
          <b>${esc(c.name)}</b><em>${have ? `×${have}` : `${xu(c.seedPrice)} xu`}</em></button>`;
      })
      .join('')}</div>`;
  }

  function selectSeed(id) {
    seedSel = id;
    tool = 'plant';
    closeSeedPick();
    renderToolbar();
  }

  function renderHarvestAll() {
    const ripe = farm.plots.filter((p) => p.crop && nowS() >= p.readyAt).length;
    $('btn-harvest-all').classList.toggle('hidden', ripe < 2);
    $('btn-harvest-all').querySelector('span').textContent = `Thu hoạch tất cả (${ripe})`;
  }

  function render() {
    if (!farm || !user) return;
    renderWorld();
    renderHarvestAll();
    renderToolbar();
    updateHint();
    if (!$('seed-pick').classList.contains('hidden')) renderSeedPick();
    if ($('dlg-shop').open) renderShop();
    if ($('dlg-store').open) renderStore();
    if ($('dlg-pen').open) renderPen();
  }

  // Mỗi giây: cập nhật đếm ngược, thanh tiến độ; vẽ lại khi có thứ vừa chín / lớn.
  function tick() {
    if (!farm) return;
    let changed = false;
    document.querySelectorAll('.plot.growing').forEach((el) => {
      const start = +el.dataset.start;
      const ready = +el.dataset.ready;
      const pr = progress(start, ready);
      const stage = pr < 0.34 ? 1 : pr < 0.67 ? 2 : 3;
      if (pr >= 1 || String(stage) !== el.dataset.stage) changed = true;
      el.querySelector('[data-left]').textContent = countdown(ready - nowS());
      el.querySelector('.bar > span').style.width = `${Math.round(pr * 100)}%`;
    });
    document.querySelectorAll('[data-left-at]').forEach((el) => {
      const left = +el.dataset.leftAt - nowS();
      if (left <= 0 && el.textContent !== 'Đã lớn') changed = true;
      else if (left > 0) el.textContent = countdown(left);
    });
    const young = [...((farm.barn && farm.barn.animals) || []), ...((farm.pond && farm.pond.fish) || [])];
    if (young.some((a) => a.readyAt > nowS() - 1 && a.readyAt <= nowS())) changed = true;
    pets.forEach(drawBadges);
    if (card) renderCard();
    if (changed) render();
  }

  // ---------- Vật nuôi và cá sống động ----------
  // Ảnh có khối (pet_<loài>.png, fish3d_<loài>.png — sprite sheet nhiều khung, gấp 2 lần cỡ hiển thị).
  // Con vật đi lang thang trong chuồng, tránh nhau, quay đầu theo hướng đi, xa nhỏ / gần to; cá bơi trong ao.
  // Bấm vào con → nhân vật đi tới, con dừng nhìn, nhảy + kêu + tim bay, hiện thẻ (lớn tới đâu, đói / no, cho ăn, thu hoạch).

  const PET = {
    chicken: { fw: 137, fh: 142, face: 'left', speed: 46, say: 'Cục tác!' },
    chicken_white: { fw: 137, fh: 142, face: 'left', speed: 46, say: 'Cục tác!' },
    pig: { fw: 172, fh: 154, face: 'left', speed: 30, say: 'Ụt ịt!' },
    sheep: { fw: 218, fh: 162, face: 'right', speed: 26, say: 'Be be!' },
    cow: { fw: 252, fh: 190, face: 'left', speed: 22, say: 'Ò ó!' },
  };
  const FISH_SHEET = {
    tilapia: { fw: 118, fh: 80 },
    catfish: { fw: 155, fh: 80 },
    carp: { fw: 93, fh: 80 },
    koi: { fw: 121, fh: 80 },
  };
  const FRAME = { stand: 0, step: 1, breathe: 2, peck: 3, hop: 4 };
  const ROAM = { x1: PEN.x1 + 34, y1: 330, x2: PEN.x2 - 30, y2: PEN.y2 - 24 }; // chỗ con vật đi lại trong chuồng
  const POND = { cx: POND_BOX.x + POND_BOX.w / 2, cy: POND_BOX.y + 104, rx: 98, ry: 42 }; // mặt nước ao

  const pets = new Map(); // id → con vật / cá đang hiện
  let card = null; // { id, fish } thẻ đang mở

  const rnd = (a, b) => a + Math.random() * (b - a);
  const isHungry = (a) => {
    if (a.readyAt <= nowS()) return false;
    if (typeof a.fedAt !== 'number' && typeof a.hungry === 'boolean') return a.hungry;
    const cd = (cat && cat.feedCooldown) || 600;
    return nowS() - Math.max(a.fedAt || 0, a.boughtAt) >= cd;
  };
  const sheetOf = (p) => (p.fish ? FISH_SHEET[p.kind] || FISH_SHEET.tilapia : PET[p.look] || PET.chicken);
  // Hệ số hiển thị: ảnh gấp 2; xa (y nhỏ) thì nhỏ lại, gần thì to; con chưa lớn nhỏ hơn.
  function scaleOf(p) {
    const young = p.data.readyAt > nowS() ? 0.78 : 1;
    if (p.fish) return 0.56 * young * (0.9 + 0.2 * ((p.y - (POND.cy - POND.ry)) / (2 * POND.ry)));
    return 0.62 * young * (0.84 + 0.26 * ((p.y - ROAM.y1) / (ROAM.y2 - ROAM.y1)));
  }

  function newTarget(p) {
    if (p.fish) {
      const a = rnd(0, Math.PI * 2);
      const r = Math.sqrt(Math.random());
      p.tx = POND.cx + Math.cos(a) * POND.rx * r;
      p.ty = POND.cy + Math.sin(a) * POND.ry * r;
    } else {
      p.tx = rnd(ROAM.x1, ROAM.x2);
      p.ty = rnd(ROAM.y1, ROAM.y2);
    }
    p.state = 'walk';
  }

  function petHTML(p) {
    const sh = sheetOf(p);
    const img = p.fish ? `fish3d_${p.kind}` : `pet_${p.look}`;
    return `<span class="pet-jump"><span class="pet-body" style="background-image:url('${IMG}${img}.png')"></span></span>
      <span class="pet-badges"></span>`;
  }

  // Đồng bộ danh sách con vật / cá với trạng thái server, giữ nguyên vị trí con đang có.
  function syncPets() {
    const layer = $('pets');
    const live = new Set();
    const add = (a, fish) => {
      live.add(a.id);
      let p = pets.get(a.id);
      if (!p) {
        p = { id: a.id, kind: a.kind, fish, look: a.kind === 'chicken' && a.id % 2 ? 'chicken_white' : a.kind, dir: Math.random() < 0.5 ? -1 : 1, frame: 0, ft: 0, data: a };
        if (fish) {
          const ang = rnd(0, Math.PI * 2);
          p.x = POND.cx + Math.cos(ang) * POND.rx * 0.6;
          p.y = POND.cy + Math.sin(ang) * POND.ry * 0.6;
        } else {
          p.x = rnd(ROAM.x1, ROAM.x2);
          p.y = rnd(ROAM.y1, ROAM.y2);
        }
        newTarget(p);
        p.state = 'idle';
        p.until = rnd(0.2, 2);
        p.el = document.createElement('button');
        p.el.className = `pet${fish ? ' fish' : ''}`;
        p.el.dataset.pet = a.id;
        p.el.innerHTML = petHTML(p);
        p.body = p.el.querySelector('.pet-body');
        p.badges = p.el.querySelector('.pet-badges');
        layer.appendChild(p.el);
        pets.set(a.id, p);
      }
      p.data = a;
      p.el.title = find(fish ? 'fish' : 'animals', a.kind).name;
      drawBadges(p);
    };
    ((farm.barn && farm.barn.animals) || []).forEach((a) => add(a, false));
    ((farm.pond && farm.pond.fish) || []).forEach((a) => add(a, true));
    for (const [id, p] of pets) {
      if (!live.has(id)) {
        p.el.remove();
        pets.delete(id);
        if (card && card.id === id) closeCard();
      }
    }
    if (card) renderCard();
  }

  // Bong bóng "Đói bụng" / lấp lánh khi lớn / thanh lớn nhỏ.
  function drawBadges(p) {
    const a = p.data;
    const ready = nowS() >= a.readyAt;
    const hungry = isHungry(a);
    const key = `${ready}|${hungry}|${Math.round(progress(a.boughtAt, a.readyAt) * 20)}`;
    if (key === p.badgeKey) return;
    p.badgeKey = key;
    const pr = progress(a.boughtAt, a.readyAt);
    p.badges.innerHTML = ready
      ? `<span class="pet-ready">!</span><span class="twinkle t1">${I.svg('sparkles')}</span><span class="twinkle t2">${I.svg('sparkles')}</span>`
      : `${hungry ? '<span class="bubble-hungry">Đói bụng</span>' : ''}<span class="mini-bar"><span style="width:${Math.round(pr * 100)}%"></span></span>`;
    p.el.classList.toggle('ready', ready);
    p.el.classList.toggle('hungry', hungry);
  }

  function setFrame(p, name) {
    const f = FRAME[name] || 0;
    if (p.fish) p.frame = name === 'step' || name === 'peck' ? 1 : 0;
    else p.frame = f;
  }

  // Mỗi khung hình: di chuyển, tránh nhau, chọn khung ảnh, đặt vị trí + cỡ theo chiều sâu.
  function stepPets(dt) {
    const list = [...pets.values()];
    for (const p of list) {
      p.ft += dt;
      p.until -= dt;
      const sp = p.fish ? 34 : (PET[p.look] || PET.chicken).speed;
      if (p.state === 'walk' || p.state === 'food') {
        const dx = p.tx - p.x;
        const dy = p.ty - p.y;
        const d = Math.hypot(dx, dy);
        const v = sp * (p.state === 'food' ? 1.8 : 1) * dt;
        if (d <= v + 0.5) {
          p.x = p.tx;
          p.y = p.ty;
          if (p.state === 'food') {
            p.state = 'peck';
            p.until = 1.6;
          } else {
            p.state = p.fish ? 'walk' : Math.random() < 0.55 && /chicken|pig/.test(p.look) ? 'peck' : 'idle';
            p.until = rnd(1.2, 3.5);
            if (p.fish) newTarget(p);
          }
        } else {
          p.x += (dx / d) * v;
          p.y += (dy / d) * v;
          if (Math.abs(dx) > 1) p.dir = dx > 0 ? 1 : -1;
        }
      } else if (p.until <= 0 && p.state !== 'look') {
        newTarget(p);
      } else if (p.state === 'look' && p.until <= 0) {
        p.state = 'idle';
        p.until = rnd(0.5, 1.5);
      }
    }
    // Tránh nhau: đẩy nhẹ hai con quá gần.
    for (let i = 0; i < list.length; i++) {
      for (let j = i + 1; j < list.length; j++) {
        const a = list[i];
        const b = list[j];
        if (a.fish !== b.fish) continue;
        const min = a.fish ? 24 : 34 * (scaleOf(a) + scaleOf(b));
        const dx = b.x - a.x;
        const dy = (b.y - a.y) * 1.6;
        const d = Math.hypot(dx, dy) || 0.01;
        if (d < min) {
          const push = ((min - d) / 2) * Math.min(1, dt * 8);
          a.x -= (dx / d) * push;
          a.y -= (dy / d) * push * 0.6;
          b.x += (dx / d) * push;
          b.y += (dy / d) * push * 0.6;
        }
      }
    }
    for (const p of list) {
      if (p.fish) {
        const ex = (p.x - POND.cx) / POND.rx;
        const ey = (p.y - POND.cy) / POND.ry;
        const r = Math.hypot(ex, ey);
        if (r > 1) {
          p.x = POND.cx + (ex / r) * POND.rx;
          p.y = POND.cy + (ey / r) * POND.ry;
        }
      } else {
        p.x = Math.min(ROAM.x2, Math.max(ROAM.x1, p.x));
        p.y = Math.min(ROAM.y2, Math.max(ROAM.y1, p.y));
      }
      // Khung ảnh theo trạng thái.
      let name = 'stand';
      if (p.state === 'walk' || p.state === 'food') name = Math.floor(p.ft / (p.fish ? 0.22 : 0.16)) % 2 ? 'step' : 'stand';
      else if (p.state === 'peck') name = Math.floor(p.ft / 0.26) % 2 ? 'peck' : 'stand';
      else if (p.state === 'hop') name = 'hop';
      else name = Math.floor(p.ft / 0.7) % 2 ? 'breathe' : 'stand';
      setFrame(p, name);
      const sh = sheetOf(p);
      const s = scaleOf(p);
      const w = sh.fw * s;
      const h = sh.fh * s;
      const n = p.fish ? 2 : 5;
      const el = p.el;
      el.style.left = `${(p.x - w / 2).toFixed(1)}px`;
      el.style.top = `${(p.y - h + (p.fish ? h / 2 : 8 * s)).toFixed(1)}px`;
      el.style.width = `${w.toFixed(1)}px`;
      el.style.height = `${h.toFixed(1)}px`;
      el.style.zIndex = p.fish ? 330 + Math.round(p.y - POND.cy + 40) : Math.round(p.y);
      const face = p.fish ? 1 : (PET[p.look] || PET.chicken).face === 'left' ? -1 : 1;
      p.body.style.backgroundSize = `${(sh.fw * n * s).toFixed(1)}px ${h.toFixed(1)}px`;
      p.body.style.backgroundPositionX = `${(-p.frame * sh.fw * s).toFixed(1)}px`;
      p.body.style.transform = p.dir === face ? '' : 'scaleX(-1)';
    }
    if (card) placeCard();
  }

  // Phản ứng khi được bấm: dừng, quay về phía nhân vật, nhảy, kêu, tim bay.
  function react(p) {
    p.state = 'look';
    p.until = 6;
    p.dir = me.x > p.x ? 1 : -1;
    if (p.fish) return;
    p.el.classList.remove('hop');
    void p.el.offsetWidth;
    p.el.classList.add('hop');
    p.state = 'hop';
    setTimeout(() => {
      if (p.state === 'hop') {
        p.state = 'look';
        p.until = 5;
      }
      p.el.classList.remove('hop');
    }, 650);
    const say = document.createElement('span');
    say.className = 'pet-say';
    say.textContent = (PET[p.look] || PET.chicken).say;
    p.el.appendChild(say);
    for (let i = 0; i < 3; i++) {
      const h = document.createElement('span');
      h.className = 'pet-heart';
      h.style.left = `${30 + i * 20}%`;
      h.style.animationDelay = `${i * 0.15}s`;
      h.innerHTML = I.svg('heart');
      p.el.appendChild(h);
    }
    setTimeout(() => p.el.querySelectorAll('.pet-say, .pet-heart').forEach((x) => x.remove()), 1600);
  }

  // Điểm đứng ngoài hàng rào gần con vật nhất (cạnh phải chuồng); cá thì bờ trái ao.
  function standPoint(p) {
    if (p.fish) return { x: POND_BOX.x - 18, y: Math.min(POND_BOX.y + 200, Math.max(POND_BOX.y + 30, p.y)), face: 'right' };
    return { x: PEN.x2 + 22, y: Math.min(PEN.y2 - 16, Math.max(PEN.y1 + 150, p.y)), face: 'left' };
  }

  function clickPet(id) {
    const p = pets.get(id);
    if (!p) return;
    // Bấm lúc đang đi thì con dừng lại nhìn nhân vật ngay.
    p.state = 'look';
    p.until = 8;
    p.dir = me.x > p.x ? 1 : -1;
    const st = standPoint(p);
    const job = { key: '_pet', face: st.face, work: false, run: () => { react(p); openCard(p); } };
    if (Math.hypot(me.x - st.x, me.y - st.y) < 16) return arrive(job);
    walkTo(st.x, st.y, job);
  }

  // ---------- Thẻ thông tin con vật ----------

  function openCard(p) {
    card = { id: p.id, fish: p.fish };
    renderCard();
  }

  function closeCard() {
    card = null;
    $('pet-card').classList.add('hidden');
  }

  function renderCard() {
    const p = card && pets.get(card.id);
    if (!p) return closeCard();
    const a = p.data;
    const info = find(p.fish ? 'fish' : 'animals', a.kind);
    const ready = nowS() >= a.readyAt;
    const hungry = isHungry(a);
    const pr = progress(a.boughtAt, a.readyAt);
    const price = info.feedPrice || Math.max(5, Math.round((info.price || 50) / 10));
    const el = $('pet-card');
    el.innerHTML = `<button class="link icon-only pc-close" data-card="close" title="Đóng">${I.svg('x')}</button>
      <b>${esc(info.name)}</b>
      <span class="pc-bar"><span style="width:${Math.round(pr * 100)}%"></span></span>
      <span class="pc-time">${ready ? 'Đã lớn — thu hoạch được' : `Còn ${countdown(a.readyAt - nowS())}`}</span>
      ${ready ? '' : `<span class="pc-state ${hungry ? 'hungry' : 'full'}">${hungry ? 'Đói bụng' : 'No'}</span>`}
      <span class="pc-btns">${ready
        ? `<button class="primary" data-card="collect">${I.svg('sprout')}Thu hoạch</button>`
        : hungry
          ? `<button class="primary" data-card="feed">${I.svg('coins')}Cho ăn (${xu(price)} xu)</button>`
          : `<span class="muted">Cho ăn lại sau ${countdown(((cat && cat.feedCooldown) || 600) - (nowS() - Math.max(a.fedAt || 0, a.boughtAt)))}</span>`}</span>`;
    el.classList.remove('hidden');
    placeCard();
  }

  function placeCard() {
    const p = pets.get(card.id);
    if (!p) return;
    const el = $('pet-card');
    const s = scaleOf(p);
    const h = sheetOf(p).fh * s;
    el.style.left = `${Math.round(p.x)}px`;
    el.style.top = `${Math.round(p.y - (p.fish ? 0 : h) - 10)}px`;
  }

  async function feedPet(p) {
    const res = await call(`/api/farm/${p.fish ? 'fish' : 'animals'}/feed`, { id: p.id });
    if (!res) return;
    dropFood(p.fish ? 'feed_pellet' : 'feed_grain', p.x + (p.fish ? 0 : -p.dir * 6), p.y + (p.fish ? 0 : 2), p.fish);
    p.state = 'peck';
    p.until = 2.2;
    floatGain(p.el, res.gained, 'Ăn ngon!');
  }

  // Thả thức ăn: thóc rải trên đất / cám rơi xuống ao, mờ dần.
  function dropFood(img, x, y, water) {
    const n = water ? 5 : 1;
    for (let i = 0; i < n; i++) {
      const f = document.createElement('span');
      f.className = `food${water ? ' pellet' : ''}`;
      f.innerHTML = `<img src="${IMG}${img}.png" alt="">`;
      f.style.left = `${x + (water ? rnd(-14, 14) : 0)}px`;
      f.style.top = `${y + (water ? rnd(-6, 6) : 0)}px`;
      f.style.zIndex = water ? 331 : Math.round(y) - 1;
      f.style.animationDelay = `${i * 0.08}s`;
      $('pets').appendChild(f);
      setTimeout(() => f.remove(), 2600);
    }
  }

  // Bấm vào mặt ao: nhân vật ra bờ, thả cám, cá bơi tới đớp; cá đang đói được cho ăn (trả xu mỗi con).
  async function feedPond(x, y) {
    const ex = (x - POND.cx) / POND.rx;
    const ey = (y - POND.cy) / POND.ry;
    const r = Math.hypot(ex, ey);
    if (r > 0.85) {
      x = POND.cx + (ex / r) * POND.rx * 0.85;
      y = POND.cy + (ey / r) * POND.ry * 0.85;
    }
    dropFood('feed_pellet', x, y, true);
    const fish = [...pets.values()].filter((p) => p.fish);
    fish.forEach((p, i) => {
      p.tx = x + rnd(-10, 10);
      p.ty = y + rnd(-5, 5);
      p.state = 'food';
      p.ft = i * 0.1;
    });
    const hungry = fish.filter((p) => isHungry(p.data));
    if (!fish.length) return A.toast('Ao chưa có cá — mua cá giống ở cửa hàng');
    if (!hungry.length) return A.toast('Cá đang no, chưa cần cho ăn');
    let xp = 0;
    let coins = 0;
    for (const p of hungry) {
      const res = await call('/api/farm/fish/feed', { id: p.id });
      if (!res) break;
      xp += (res.gained && res.gained.xp) || 0;
      coins += find('fish', p.kind).feedPrice || 0;
    }
    if (xp) A.toast(`Đã cho ${hungry.length} con cá ăn${coins ? ` (−${xu(coins)} xu)` : ''} · +${xp} XP`);
  }

  // ---------- Nhân vật đi lại ----------

  const SPEED = 230; // px/giây
  const SPRITE = 88;
  const me = { x: 586, y: 560, dir: 'down', walking: false, look: {}, path: [], pending: null, drawn: '' };
  const keys = new Set();
  let zoom = 1;
  let nearKey = null;

  const YMIN = SKY + 14;
  const blocked = (x, y) =>
    x < 18 || x > W - 18 || y < YMIN || y > BOTTOM - 6 ||
    obstacles.some((r) => x > r[0] - 12 && x < r[2] + 12 && y > r[1] - 4 && y < r[3] + 6);

  // Vật cao (kho, chuồng, cổng, cây, cây trồng) đứng TRƯỚC nhân vật và che hơn 40% thân → làm mờ như game Avatar.
  let fadeKey = '';
  function updateFade() {
    const key = `${Math.round(me.x)}|${Math.round(me.y)}|${zoom}`;
    if (key === fadeKey) return;
    fadeKey = key;
    const body = $('me-body').getBoundingClientRect();
    const area = Math.max(1, body.width * body.height);
    document.querySelectorAll('.obj.building, .obj.gate, .obj.decor, .pen-shed, .plot-top').forEach((el) => {
      let hide = false;
      if ((+el.style.zIndex || 0) > me.y) {
        const r = (el.querySelector('img') || el).getBoundingClientRect();
        const w = Math.min(r.right, body.right) - Math.max(r.left, body.left);
        const h = Math.min(r.bottom, body.bottom) - Math.max(r.top, body.top);
        hide = w > 0 && h > 0 && (w * h) / area > 0.4;
      }
      el.classList.toggle('see-through', hide);
    });
  }

  function drawMe() {
    const el = $('me');
    el.style.left = `${Math.round(me.x)}px`;
    el.style.top = `${Math.round(me.y)}px`;
    el.style.zIndex = Math.round(me.y);
    fadeKey = '';
    requestAnimationFrame(updateFade);
    const key = `${me.dir}|${me.walking}|${JSON.stringify(me.look)}`;
    if (key !== me.drawn) {
      me.drawn = key;
      $('me-body').innerHTML = C.sprite(me.look, { dir: me.dir, walking: me.walking, size: SPRITE });
    }
  }

  // Đoạn thẳng có đi được không (lấy mẫu mỗi 6px).
  function lineFree(x1, y1, x2, y2) {
    const n = Math.ceil(Math.hypot(x2 - x1, y2 - y1) / 6);
    for (let i = 1; i <= n; i++) {
      if (blocked(x1 + ((x2 - x1) * i) / n, y1 + ((y2 - y1) * i) / n)) return false;
    }
    return true;
  }

  const CELL = 16;
  function nearestFree(x, y) {
    if (!blocked(x, y)) return [x, y];
    for (let r = 1; r < 60; r++) {
      for (let a = 0; a < 16; a++) {
        const nx = x + Math.cos((a / 16) * Math.PI * 2) * r * 8;
        const ny = y + Math.sin((a / 16) * Math.PI * 2) * r * 8;
        if (!blocked(nx, ny)) return [nx, ny];
      }
    }
    return [586, 560];
  }

  // Tìm đường A* trên lưới 16px, rồi rút gọn bằng đường thẳng nhìn thấy nhau.
  function findPath(tx, ty) {
    if (lineFree(me.x, me.y, tx, ty)) return [[tx, ty]];
    const cols = Math.ceil(W / CELL);
    const rows = Math.ceil(H / CELL);
    const idx = (c, r) => r * cols + c;
    const free = (c, r) => c >= 0 && r >= 0 && c < cols && r < rows && !blocked(c * CELL + CELL / 2, r * CELL + CELL / 2);
    const sc = Math.floor(me.x / CELL);
    const sr = Math.floor(me.y / CELL);
    const gc = Math.floor(tx / CELL);
    const gr = Math.floor(ty / CELL);
    const g = new Map([[idx(sc, sr), 0]]);
    const from = new Map();
    const open = [[0, sc, sr]];
    const seen = new Set();
    const h = (c, r) => Math.hypot(c - gc, r - gr);
    while (open.length) {
      // Hàng đợi nhỏ (vài nghìn ô): tìm min tuyến tính là đủ nhanh.
      let bi = 0;
      for (let i = 1; i < open.length; i++) if (open[i][0] < open[bi][0]) bi = i;
      const [, c, r] = open.splice(bi, 1)[0];
      const k = idx(c, r);
      if (seen.has(k)) continue;
      seen.add(k);
      if (c === gc && r === gr) {
        const cells = [];
        for (let cur = k; from.has(cur); cur = from.get(cur)) cells.push(cur);
        const pts = cells.reverse().map((q) => [(q % cols) * CELL + CELL / 2, Math.floor(q / cols) * CELL + CELL / 2]);
        pts[pts.length - 1] = [tx, ty];
        // Rút gọn: bỏ điểm giữa nếu nhìn thẳng được tới điểm sau.
        const out = [];
        let ax = me.x;
        let ay = me.y;
        for (let i = 0; i < pts.length; i++) {
          const nxt = pts[i + 1];
          if (nxt && lineFree(ax, ay, nxt[0], nxt[1])) continue;
          out.push(pts[i]);
          [ax, ay] = pts[i];
        }
        return out;
      }
      for (let dc = -1; dc <= 1; dc++) {
        for (let dr = -1; dr <= 1; dr++) {
          if (!dc && !dr) continue;
          const nc = c + dc;
          const nr = r + dr;
          if (!free(nc, nr) && !(nc === gc && nr === gr)) continue;
          if (dc && dr && (!free(c + dc, r) || !free(c, r + dr))) continue; // không cắt góc
          const nk = idx(nc, nr);
          const ng = g.get(k) + (dc && dr ? Math.SQRT2 : 1);
          if (ng < (g.has(nk) ? g.get(nk) : Infinity)) {
            g.set(nk, ng);
            from.set(nk, k);
            open.push([ng + h(nc, nr), nc, nr]);
          }
        }
      }
    }
    return null;
  }

  function walkTo(x, y, pending = null) {
    [x, y] = nearestFree(x, y);
    const path = findPath(x, y);
    if (!path) return A.toast('Không đi tới đó được');
    me.path = path;
    me.pending = pending;
    const mark = $('go-mark');
    mark.style.left = `${Math.round(x)}px`;
    mark.style.top = `${Math.round(y)}px`;
    mark.classList.toggle('hidden', !!pending);
    mark.classList.remove('pop');
    void mark.offsetWidth;
    mark.classList.add('pop');
  }

  // Đi tới một đối tượng rồi thao tác (đứng sẵn gần thì làm luôn).
  function goTo(key, run) {
    const o = objs[key];
    if (!o) return;
    const pm = /^plot(\d+)$/.exec(key);
    if (pm && !run) {
      // Ghi ý định lúc bấm: tới nơi mà ô đã đổi (vd vừa "thu hoạch tất cả") thì thôi, không mở nhầm hộp thoại.
      const p = farm.plots[+pm[1]];
      const intent = !p.crop ? 'plant' : nowS() >= p.readyAt ? 'harvest' : 'view';
      run = () => plotAction(+pm[1], intent);
    }
    const job = { key, run: run || o.run, work: !!pm || (/^barn|^pond/.test(key) && !!run) };
    if (Math.hypot(me.x - o.x, me.y - o.y) < 14) return arrive(job);
    walkTo(o.x, o.y, job);
  }

  function arrive(job) {
    const o = objs[job.key];
    if (o) me.dir = o.face;
    else if (job.face) me.dir = job.face;
    me.walking = false;
    drawMe();
    if (!job.work) return job.run();
    // Nhún / vung tay một nhịp như đang làm đất, rồi ô mới thay đổi.
    const el = $('me');
    el.classList.remove('work');
    void el.offsetWidth;
    el.classList.add('work');
    setTimeout(() => {
      el.classList.remove('work');
      job.run();
    }, 420);
  }

  const typing = (e) => {
    const t = e.target;
    return t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName));
  };
  const dialogOpen = () => !!document.querySelector('dialog[open]');
  const KEYMAP = { ArrowUp: 'up', KeyW: 'up', ArrowDown: 'down', KeyS: 'down', ArrowLeft: 'left', KeyA: 'left', ArrowRight: 'right', KeyD: 'right' };
  // Dự phòng theo e.key (bàn phím / bộ gõ không báo e.code chuẩn).
  const KEYNAME = { w: 'up', s: 'down', a: 'left', d: 'right', Up: 'up', Down: 'down', Left: 'left', Right: 'right' };
  const dirKey = (e) => KEYMAP[e.code] || KEYNAME[e.key] || KEYNAME[String(e.key).toLowerCase()];
  const isAct = (e) => e.code === 'KeyE' || e.code === 'Enter' || e.code === 'Space' || /^[eE ]$/.test(e.key);

  // Bắt ở pha capture trên window: đi lại được ngay khi mở trang, không cần bấm vào cảnh trước,
  // dù focus đang ở nút nào (trừ ô nhập liệu / hộp thoại).
  window.addEventListener('keydown', (e) => {
    if (e.code === 'Escape' && !dialogOpen() && (tool || !$('seed-pick').classList.contains('hidden'))) return pickTool('none');
    if (typing(e) || dialogOpen() || e.ctrlKey || e.metaKey || e.altKey || !farm) return;
    const dir = dirKey(e);
    if (dir) {
      e.preventDefault();
      keys.add(dir);
      me.path = [];
      me.pending = null;
      $('go-mark').classList.add('hidden');
    } else if (isAct(e) && nearKey) {
      e.preventDefault();
      if (!e.repeat) goTo(nearKey);
    }
  }, true);
  window.addEventListener('keyup', (e) => {
    const dir = dirKey(e);
    if (dir) keys.delete(dir);
  }, true);
  document.addEventListener('visibilitychange', () => keys.clear());
  // Đóng hộp thoại: bỏ focus khỏi nút để phím Space / Enter không bấm lại nút cũ.
  // (Không xoá phím đang giữ ở đây: sự kiện close đến muộn, có thể sau khi người chơi đã bấm đi tiếp.)
  document.querySelectorAll('dialog').forEach((d) => d.addEventListener('close', () => {
    if (document.activeElement && document.activeElement !== document.body) document.activeElement.blur();
  }));
  window.addEventListener('blur', () => keys.clear());

  let last = 0;
  function frame(ts) {
    const dt = Math.min(0.05, (ts - (last || ts)) / 1000);
    last = ts;
    if (farm) {
      step(dt);
      stepPets(dt);
    }
    requestAnimationFrame(frame);
  }

  function step(dt) {
    let vx = 0;
    let vy = 0;
    if (keys.size && !dialogOpen()) {
      vx = (keys.has('right') ? 1 : 0) - (keys.has('left') ? 1 : 0);
      vy = (keys.has('down') ? 1 : 0) - (keys.has('up') ? 1 : 0);
    } else if (keys.size) keys.clear();
    let moving = false;
    if (vx || vy) {
      const n = Math.hypot(vx, vy);
      const nx = me.x + (vx / n) * SPEED * dt;
      const ny = me.y + (vy / n) * SPEED * dt;
      // Trượt theo vật cản: thử từng trục.
      if (!blocked(nx, me.y)) me.x = nx;
      if (!blocked(me.x, ny)) me.y = ny;
      moving = true;
    } else if (me.path.length) {
      const [tx, ty] = me.path[0];
      const dx = tx - me.x;
      const dy = ty - me.y;
      const d = Math.hypot(dx, dy);
      const s = SPEED * dt;
      if (d <= s) {
        me.x = tx;
        me.y = ty;
        me.path.shift();
      } else {
        me.x += (dx / d) * s;
        me.y += (dy / d) * s;
      }
      vx = dx;
      vy = dy;
      moving = true;
      if (!me.path.length) {
        $('go-mark').classList.add('hidden');
        const job = me.pending;
        me.pending = null;
        if (job) arrive(job);
        else me.walking = false;
        drawMe();
        updateHint();
        return;
      }
    }
    if (moving) {
      if (Math.abs(vx) > Math.abs(vy)) me.dir = vx > 0 ? 'right' : 'left';
      else if (vy) me.dir = vy > 0 ? 'down' : 'up';
    }
    if (moving !== me.walking || moving) {
      me.walking = moving;
      drawMe();
      if (moving) {
        updateHint();
        follow();
      }
    }
  }

  // Gợi ý "E" trên đối tượng gần nhất (trong 60px).
  function updateHint() {
    let best = null;
    let bd = 60;
    for (const [k, o] of Object.entries(objs)) {
      const d = Math.hypot(me.x - o.x, me.y - o.y);
      if (d < bd) {
        bd = d;
        best = k;
      }
    }
    nearKey = best;
    const hint = $('hint');
    hint.classList.toggle('hidden', !best);
    if (!best) return;
    const o = objs[best];
    hint.style.left = `${Math.round(o.hx)}px`;
    hint.style.top = `${Math.round(o.hy)}px`;
    const pm = /^plot(\d+)$/.exec(best);
    hint.querySelector('span').textContent = pm && farm.plots[+pm[1]] ? plotVerb(farm.plots[+pm[1]]) : o.verb;
  }

  // Camera: chỉ cuộn ngang để nhân vật nằm trong vùng giữa; chiều dọc vừa khung nên không cuộn.
  function follow(snap) {
    const sc = $('scene');
    const vw = sc.clientWidth;
    const cx = me.x * zoom;
    const mx = snap ? vw / 2 : vw * 0.32;
    let tx = sc.scrollLeft;
    if (cx - tx < mx) tx = cx - mx;
    else if (cx - tx > vw - mx) tx = cx - vw + mx;
    sc.scrollLeft = tx;
  }

  // Phóng cảnh cho chiều cao thế giới vừa khung nhìn (điện thoại giữ ≤ 1 để thấy rộng hơn; phần dư phía trên là trời).
  function setZoom() {
    // Chừa chỗ cho thanh công cụ dưới cùng (phần đó là cỏ).
    const sc = $('scene');
    const sh = sc.clientHeight || innerHeight - 60;
    // Máy tính: cả cổng, kho, ruộng 6 cột, chuồng, ao phải vừa một khung nhìn (theo cả chiều ngang).
    const fitW = innerWidth < 600 ? Infinity : (sc.clientWidth || innerWidth) / CORE_W;
    zoom = Math.max(0.5, Math.min(sh / H, fitW, innerWidth < 600 ? 1 : 1.35));
    if (farm) renderWorld();
  }

  // Kéo chuột để xem ngang tự do (điện thoại dùng vuốt cuộn sẵn có); lăn chuột dọc cũng cuộn ngang.
  let drag = null;
  let dragged = false;
  $('scene').addEventListener('pointerdown', (e) => {
    if (e.pointerType !== 'mouse' || e.button !== 0) return;
    drag = { x: e.clientX, left: $('scene').scrollLeft };
    dragged = false;
  });
  addEventListener('pointermove', (e) => {
    if (!drag) return;
    const dx = e.clientX - drag.x;
    if (!dragged && Math.abs(dx) < 6) return;
    dragged = true;
    $('scene').classList.add('dragging');
    $('scene').scrollLeft = drag.left - dx;
  });
  addEventListener('pointerup', () => {
    drag = null;
    $('scene').classList.remove('dragging');
  });
  $('scene').addEventListener('wheel', (e) => {
    if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return;
    e.preventDefault();
    $('scene').scrollLeft += e.deltaY;
  }, { passive: false });
  // Nút xem trái / phải / về nhân vật.
  document.querySelectorAll('[data-pan]').forEach((b) => b.addEventListener('click', () => {
    const sc = $('scene');
    if (b.dataset.pan === 'me') return sc.scrollTo({ left: me.x * zoom - sc.clientWidth / 2, behavior: 'smooth' });
    sc.scrollBy({ left: +b.dataset.pan * sc.clientWidth * 0.7, behavior: 'smooth' });
  }));

  // Bấm trong cảnh: vào đối tượng thì đi tới rồi thao tác, chỗ trống thì đi tới đó. Vừa kéo xong thì bỏ qua.
  $('world').addEventListener('click', (e) => {
    if (!farm) return;
    if (dragged) {
      dragged = false;
      return;
    }
    if (e.target.closest('[data-hint]')) return nearKey && goTo(nearKey);
    const r = $('world-wrap').getBoundingClientRect();
    const wx = (e.clientX - r.left) / zoom;
    const wy = (e.clientY - r.top) / zoom - OY;
    // Thẻ con vật: cho ăn / thu hoạch / đóng.
    const act = e.target.closest('[data-card]');
    if (act) {
      const p = card && pets.get(card.id);
      if (act.dataset.card === 'close' || !p) return closeCard();
      if (act.dataset.card === 'feed') return feedPet(p);
      if (act.dataset.card === 'collect') {
        closeCard();
        return collect(p.fish, p.id, p.el);
      }
      return;
    }
    if (e.target.closest('#pet-card')) return;
    closeCard();
    const pet = e.target.closest('[data-pet]');
    if (pet) return clickPet(+pet.dataset.pet);
    // Bấm mặt ao (không phải nhãn): ra bờ thả cám cho cá.
    if (e.target.closest('.pond-water') && farm.pond) return goTo('pond', () => feedPond(wx, wy));
    const obj = e.target.closest('[data-obj]');
    if (obj) return goTo(obj.dataset.obj);
    walkTo(wx, wy);
  });

  // ---------- Hiệu ứng ----------

  function floatGain(anchor, gained, extra = '') {
    if (!gained) return;
    const r = anchor ? anchor.getBoundingClientRect() : { left: innerWidth / 2, top: innerHeight / 2, width: 0 };
    const el = document.createElement('span');
    el.className = 'gain';
    const lines = [];
    if (gained.coins) lines.push(`+${xu(gained.coins)} xu`);
    if (extra) lines.push(`<span class="items">${extra}</span>`);
    if (gained.xp) lines.push(`<span class="xp">+${gained.xp} XP</span>`);
    el.innerHTML = lines.join('');
    el.style.left = `${r.left + r.width / 2}px`;
    el.style.top = `${r.top}px`;
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 1700);
  }

  // Nông sản / con nuôi bay từ chỗ thu hoạch về kho sản phẩm.
  function flyToStore(from, items) {
    const target = document.querySelector('[data-obj="products"] img');
    if (!from || !target || !items) return;
    const a = from.getBoundingClientRect();
    const b = target.getBoundingClientRect();
    let n = 0;
    for (const [id, qty] of Object.entries(items)) {
      for (let i = 0; i < Math.min(qty, 4) && n < 8; i++, n++) {
        const el = document.createElement('span');
        el.className = 'fly';
        el.innerHTML = product(id).draw(40);
        el.style.left = `${a.left + a.width / 2 + (n % 3) * 12 - 12}px`;
        el.style.top = `${a.top + a.height / 3}px`;
        document.body.appendChild(el);
        const dx = b.left + b.width / 2 - (a.left + a.width / 2);
        const dy = b.top + b.height / 2 - (a.top + a.height / 3);
        setTimeout(() => {
          el.style.transform = `translate(${dx}px, ${dy}px) scale(.45)`;
          el.style.opacity = '.2';
        }, 30 + n * 90);
        setTimeout(() => el.remove(), 1100 + n * 90);
      }
    }
    setTimeout(() => {
      const st = document.querySelector('[data-obj="products"]');
      if (!st) return;
      st.classList.remove('bump');
      void st.offsetWidth;
      st.classList.add('bump');
    }, 900);
  }

  const itemsText = (items) =>
    Object.entries(items || {})
      .map(([id, n]) => `+${n} ${esc(product(id).name)}`)
      .join(' · ');

  // ---------- Thao tác ----------

  async function plotAction(index, intent) {
    const p = farm.plots[index];
    if (!p) return;
    if ((intent === 'harvest' && !p.crop) || (intent === 'plant' && p.crop)) return;
    if (!p.crop) {
      if (tool === 'harvest') return A.toast('Ô này chưa trồng gì');
      if (tool === 'plant' && seedSel) return call('/api/farm/plant', { plot: index, crop: seedSel });
      return openShop('crops', index);
    }
    if (nowS() < p.readyAt) return A.toast(`${find('crops', p.crop).name} còn ${fmtDur(p.readyAt - nowS())} nữa mới chín`);
    const el = document.querySelector(`[data-obj="plot${index}"]`);
    const res = await call('/api/farm/harvest', { plot: index });
    if (res) {
      flyToStore(el, res.gained && res.gained.items);
      floatGain(el, res.gained, itemsText(res.gained && res.gained.items));
    }
  }

  async function buyPlot() {
    if (!confirm(`Mở thêm ô đất với giá ${xu(farm.nextPlotPrice)} xu?`)) return;
    await call('/api/farm/plots/buy', {});
  }

  async function collect(isFish, id, el) {
    const items = (isFish ? farm.pond && farm.pond.fish : farm.barn && farm.barn.animals) || [];
    const it = items.find((x) => x.id === id);
    if (!it) return;
    const info = find(isFish ? 'fish' : 'animals', it.kind);
    if (nowS() < it.readyAt) return A.toast(`${info.name} còn ${fmtDur(it.readyAt - nowS())} nữa mới lớn`);
    const anchor = el && el.isConnected ? el : document.querySelector(`[data-obj="${isFish ? 'pond' : 'barn'}"]`);
    const res = await call(`/api/farm/${isFish ? 'fish' : 'animals'}/collect`, { id });
    if (res) {
      const got = (res.gained && res.gained.items) || { [it.kind]: 1 };
      flyToStore(anchor, got);
      floatGain(anchor, res.gained, itemsText(got));
    }
  }

  async function plant(cropId) {
    let plot = shopMode.plot;
    if (plot === null) {
      const empty = farm.plots.find((p) => !p.crop);
      if (!empty) return A.toast('Hết ô trống');
      plot = empty.index;
    }
    const res = await call('/api/farm/plant', { plot, crop: cropId });
    if (res && shopMode.plot !== null) $('dlg-shop').close();
  }

  // ---------- Hộp thoại chung ----------

  function show(id) {
    keys.clear();
    if (!$(id).open) $(id).showModal();
  }

  function metaLine(parts) {
    return `<span class="meta">${parts.filter(Boolean).map(([icon, text]) => `<span>${I.svg(icon)}${text}</span>`).join('')}</span>`;
  }

  const qtyPicker = () =>
    `<div class="qty-pick"><span>Mua mỗi lần</span>${[1, 5, 10]
      .map((n) => `<button class="${n === buyQty ? 'active' : ''}" data-qty="${n}">×${n}</button>`)
      .join('')}</div>`;

  // Thanh sức chứa + nút mở rộng của một kho.
  function capRow(s, kind, units) {
    const pct = Math.round((s.used / Math.max(1, s.capacity)) * 100);
    const expand = s.nextExpandPrice
      ? `<button data-store-expand="${kind}" title="Thêm ${units} chỗ">${I.svg('plus')}+${units} · ${xu(s.nextExpandPrice)}</button>`
      : '<span class="tag">Tối đa</span>';
    return `<div class="cap"><span>${s.used}/${s.capacity} chỗ</span><span class="cap-bar"><span style="width:${pct}%"></span></span>${expand}</div>`;
  }

  // ---------- Kho ----------

  function openStore(kind) {
    storeMode = kind;
    renderStore();
    show('dlg-store');
  }

  function renderStore() {
    const kind = storeMode;
    const s = store(kind);
    const units = (cat && cat.storage && cat.storage[kind] && cat.storage[kind].expandUnits) || 25;
    $('store-title').textContent = kind === 'seeds' ? 'Kho giống' : 'Kho thóc';
    const head = `<div class="store-hero">${kind === 'seeds' ? art.img('hut_seed', { h: 74 }) : art.img('hut_rice', { h: 74 })}
      <div><p class="muted">${kind === 'seeds'
        ? 'Hạt mua sẵn để gieo; gieo vào ô trống sẽ lấy hạt trong kho trước. Mỗi hạt chiếm 1 chỗ.'
        : 'Nông sản thu hoạch, gia súc và cá đã lớn được cất ở đây — bán lấy xu. Mỗi món chiếm 1 chỗ.'}</p>
      ${capRow(s, kind, units)}</div></div>`;
    if (!cat) {
      $('store-body').innerHTML = head;
      return;
    }
    let body;
    if (kind === 'seeds') {
      const cards = cat.crops.map((c) => {
        const have = s.items[c.id] || 0;
        const locked = user.level < c.level;
        const cost = c.seedPrice * buyQty;
        const room = s.capacity - s.used >= buyQty;
        const btn = locked
          ? `<button disabled>${I.svg('lock')}Lv ${c.level}</button>`
          : `<button class="primary" data-buy-seed="${c.id}" ${user.balance < cost || !room ? 'disabled' : ''} title="${room ? '' : 'Kho giống đầy'}">${I.svg('coins')}+${buyQty} · ${xu(cost)}</button>`;
        return `<div class="store-card${have ? ' has' : ''}${locked ? ' locked' : ''}">
          ${have ? `<span class="count">×${have}</span>` : ''}
          <span class="pic">${art.seed(c.id, 58)}</span><b>${esc(c.name)}</b>
          <span class="sub">${xu(c.seedPrice)} xu/hạt · ${fmtDur(c.growSeconds)}</span>${btn}</div>`;
      });
      body = `${qtyPicker()}<div class="store-grid">${cards.join('')}</div>`;
    } else {
      const entries = Object.entries(s.items).filter(([, n]) => n > 0);
      const total = entries.reduce((sum, [id, n]) => sum + (product(id).sellPrice || 0) * n, 0);
      const cards = entries.map(([id, n]) => {
        const p = product(id);
        return `<div class="store-card has"><span class="count">×${n}</span>
          <span class="pic${p.list === 'fish' ? ' water' : ''}">${p.draw(52)}</span><b>${esc(p.name)}</b>
          <span class="sub">${xu(p.sellPrice)} xu/cái</span>
          <span class="btns"><button data-sell-item="${id}" data-sell-qty="1">Bán 1</button>
          <button class="primary" data-sell-item="${id}" data-sell-qty="0">Bán hết · ${xu(p.sellPrice * n)}</button></span></div>`;
      });
      body = entries.length
        ? `<div class="store-sellall"><span>Tổng giá trị: <b>${xu(total)} xu</b></span>
            <button class="primary" data-sell-all>${I.svg('coins')}Bán tất cả · ${xu(total)}</button></div>
          <div class="store-grid">${cards.join('')}</div>`
        : '<p class="store-empty muted">Kho trống — thu hoạch cây chín, gia súc và cá đã lớn để cất vào đây.</p>';
    }
    $('store-body').innerHTML = head + body;
  }

  // ---------- Chuồng và ao ----------

  function openPen(kind) {
    penMode = kind;
    renderPen();
    show('dlg-pen');
  }

  function renderPen() {
    const kind = penMode;
    const isBarn = kind === 'barn';
    const pen = isBarn ? farm.barn : farm.pond;
    const cfg = cat ? cat[kind] : { price: 0, level: 1 };
    const list = isBarn ? 'animals' : 'fish';
    $('pen-title').textContent = isBarn ? 'Chuồng gia súc' : 'Ao cá';
    if (!pen) {
      const locked = user.level < cfg.level;
      $('pen-body').innerHTML = `<div class="pen-offer">
        <div class="offer-art">${isBarn ? art.img('barn', { h: 130 }) + art.img('hay', { h: 50 }) : art.img('pond', { h: 110 })}</div>
        <h3>${isBarn ? 'Chưa có chuồng' : 'Chưa có ao'}</h3>
        <p class="muted">${isBarn
          ? `Nuôi gà, heo, cừu, bò. Mỗi con chiếm chỗ theo đơn vị (gà, heo 1 · cừu, bò 2); chuồng chứa ${cfg.capacity || 5} đơn vị, mở rộng được.`
          : `Thả cá rô phi, cá trê, cá chép, cá Koi; mỗi loại chiếm số đơn vị khác nhau, ao chứa ${cfg.capacity || 5} đơn vị, mở rộng được.`}</p>
        <button class="primary" data-buy-pen="${kind}" ${locked ? 'disabled' : ''}>
          ${locked ? `${I.svg('lock')}Cần Lv ${cfg.level}` : `${I.svg('coins')}Mua ${isBarn ? 'chuồng' : 'ao'} ${xu(cfg.price)} xu`}</button></div>`;
      return;
    }
    const items = (isBarn ? pen.animals : pen.fish) || [];
    const pct = Math.round((pen.used / Math.max(1, pen.capacity)) * 100);
    const expand = pen.nextExpandPrice
      ? `<button data-expand="${kind}" title="Thêm ${cfg.expandUnits || 5} đơn vị">${I.svg('plus')}+${cfg.expandUnits || 5} · ${xu(pen.nextExpandPrice)}</button>`
      : '<span class="tag">Tối đa</span>';
    const rows = items.length
      ? items
        .map((it) => {
          const info = find(list, it.kind);
          const ready = nowS() >= it.readyAt;
          const draw = isBarn ? art.animal(it.kind, 30) : art.fish(it.kind, 18);
          const hungry = !ready && isHungry(it);
          const action = ready
            ? `<button class="primary" data-collect-${isBarn ? 'animal' : 'fish'}="${it.id}">${I.svg('sprout')}Thu hoạch</button>`
            : hungry
              ? `<button data-feed-${isBarn ? 'animal' : 'fish'}="${it.id}" title="Cho ăn">${I.svg('coins')}Cho ăn ${xu(info.feedPrice || 0)}</button>`
              : '';
          return `<li>${draw}<span class="grow"><span>${esc(info.name)}</span>
            <span class="t"><span data-left-at="${it.readyAt}">${ready ? 'Đã lớn' : countdown(it.readyAt - nowS())}</span>${hungry ? ' · <b class="hungry-tag">Đói bụng</b>' : ''}</span></span>${action}</li>`;
        })
        .join('')
      : `<li class="empty">Chưa có ${isBarn ? 'vật nuôi' : 'cá'} — mua ở cửa hàng</li>`;
    $('pen-body').innerHTML = `
      <div class="cap"><span>${pen.used}/${pen.capacity} đơn vị</span><span class="cap-bar"><span style="width:${pct}%"></span></span>${expand}</div>
      <p class="muted">Con đã lớn bấm "Thu hoạch" để cất vào kho thóc (+XP), rồi bán ở kho.</p>
      <ul class="critter-list">${rows}</ul>
      <button class="primary wide" data-open-shop="${list}">${I.svg('plus')}Mua ${isBarn ? 'con giống' : 'cá giống'}</button>`;
  }

  // ---------- Cửa hàng ----------

  function openShop(tab, plot = null) {
    shopMode = { tab, plot };
    renderShop();
    show('dlg-shop');
  }

  function renderShop() {
    const { tab, plot } = shopMode;
    document.querySelectorAll('#shop-tabs .tab').forEach((b) => b.classList.toggle('active', b.dataset.tab === tab));
    $('shop-tabs').classList.toggle('hidden', plot !== null);
    $('shop-title').textContent = plot !== null ? `Gieo hạt vào ô ${plot + 1}` : 'Cửa hàng';
    if (!cat) {
      $('shop-list').innerHTML = '<p class="muted">Đang tải cửa hàng...</p>';
      return;
    }
    const rows = [];
    if (tab === 'crops') {
      const seeds = store('seeds');
      if (plot !== null) {
        $('shop-note').textContent = 'Chọn hạt để gieo — hạt có sẵn trong kho giống được dùng trước.';
        // Hạt đang có trong kho lên đầu.
        const crops = [...cat.crops].sort((a, b) => (seeds.items[b.id] ? 1 : 0) - (seeds.items[a.id] ? 1 : 0));
        for (const c of crops) {
          const have = seeds.items[c.id] || 0;
          const locked = user.level < c.level;
          let btn;
          if (locked) btn = `<button disabled>${I.svg('lock')}Cần Lv ${c.level}</button>`;
          else if (have) btn = `<button class="primary" data-plant="${c.id}">${I.svg('sprout')}Gieo (còn ${have})</button>`;
          else btn = `<button data-plant="${c.id}" ${user.balance < c.seedPrice ? 'disabled' : ''}>${I.svg('coins')}Mua 1 hạt ${xu(c.seedPrice)} xu</button>`;
          rows.push(`<div class="shop-row${locked ? ' locked' : ''}${have ? ' feature' : ''}"><span class="thumb">${have ? art.seed(c.id, 46) : art.item(c.id, 40)}</span>
            <div><b>${esc(c.name)}</b>${have ? `<span class="have">×${have} trong kho</span>` : ''}${metaLine([['timer', fmtDur(c.growSeconds)], ['coins', `bán ${xu(c.sellPrice)}`], ['trophy', `+${c.xp} XP`]])}</div>${btn}</div>`);
        }
      } else {
        $('shop-note').innerHTML = `Hạt mua được cất vào <b>Kho giống</b> (${seeds.used}/${seeds.capacity} chỗ).`;
        rows.push(qtyPicker());
        for (const c of cat.crops) {
          const locked = user.level < c.level;
          const cost = c.seedPrice * buyQty;
          const have = seeds.items[c.id] || 0;
          const btn = locked
            ? `<button disabled>${I.svg('lock')}Cần Lv ${c.level}</button>`
            : `<button class="primary" data-buy-seed="${c.id}" ${user.balance < cost || seeds.capacity - seeds.used < buyQty ? 'disabled' : ''}>${I.svg('coins')}+${buyQty} hạt · ${xu(cost)}</button>`;
          rows.push(`<div class="shop-row${locked ? ' locked' : ''}"><span class="thumb">${art.item(c.id, 40)}</span>
            <div><b>${esc(c.name)}</b>${have ? `<span class="have">×${have} trong kho</span>` : ''}${metaLine([['coins', `${xu(c.seedPrice)}/hạt`], ['timer', fmtDur(c.growSeconds)], ['trophy', `+${c.xp} XP`], ['sparkles', `Lv ${c.level}`]])}</div>${btn}</div>`);
        }
      }
    } else {
      const isBarn = tab === 'animals';
      const kind = isBarn ? 'barn' : 'pond';
      const pen = farm[kind];
      const cfg = cat[kind];
      $('shop-note').textContent = pen
        ? `${isBarn ? 'Chuồng' : 'Ao'} đang dùng ${pen.used}/${pen.capacity} đơn vị.`
        : `Cần mua ${isBarn ? 'chuồng' : 'ao'} trước.`;
      // Dòng đầu: mua chuồng / ao, hoặc mở rộng.
      if (!pen) {
        const locked = user.level < cfg.level;
        rows.push(`<div class="shop-row feature${locked ? ' locked' : ''}"><span class="thumb">${isBarn ? art.img('barn', { h: 46 }) : art.img('pond', { h: 34 })}</span>
          <div><b>${isBarn ? 'Chuồng gia súc' : 'Ao cá'}</b>${metaLine([['users', `chứa ${cfg.capacity} đơn vị`], ['plus', `mở rộng tới ${cfg.maxCapacity}`], ['sparkles', `Lv ${cfg.level}`]])}</div>
          ${locked ? `<button disabled>${I.svg('lock')}Cần Lv ${cfg.level}</button>` : `<button class="primary" data-buy-pen="${kind}">${I.svg('coins')}${xu(cfg.price)}</button>`}</div>`);
      } else if (pen.nextExpandPrice) {
        rows.push(`<div class="shop-row feature"><span class="thumb">${isBarn ? art.img('fence_piece', { h: 34 }) : art.img('pond', { h: 34 })}</span>
          <div><b>Mở rộng ${isBarn ? 'chuồng' : 'ao'} +${cfg.expandUnits}</b>${metaLine([['users', `${pen.capacity} → ${pen.capacity + cfg.expandUnits} đơn vị`], ['layers', `tối đa ${pen.maxCapacity}`]])}</div>
          <button class="primary" data-expand="${kind}">${I.svg('coins')}${xu(pen.nextExpandPrice)}</button></div>`);
      }
      for (const a of cat[isBarn ? 'animals' : 'fish']) {
        const locked = user.level < a.level;
        const full = pen && pen.used + a.units > pen.capacity;
        let btn;
        if (locked) btn = `<button disabled>${I.svg('lock')}Cần Lv ${a.level}</button>`;
        else if (!pen) btn = `<button disabled>Cần ${isBarn ? 'chuồng' : 'ao'}</button>`;
        else if (full) btn = `<button disabled>Hết chỗ</button>`;
        else btn = `<button class="primary" data-buy-${isBarn ? 'animal' : 'fish'}="${a.id}" ${user.balance < a.price ? 'disabled' : ''}>${I.svg('coins')}${xu(a.price)}</button>`;
        rows.push(`<div class="shop-row${locked ? ' locked' : ''}"><span class="thumb${isBarn ? '' : ' water'}">${isBarn ? art.animal(a.id, 40) : art.fish(a.id, 24)}</span>
          <div><b>${esc(a.name)}</b>${metaLine([['users', `${a.units} đơn vị`], ['timer', fmtDur(a.growSeconds)], ['coins', `bán ${xu(a.sellPrice)}`], ['trophy', `+${a.xp} XP`], ['sparkles', `Lv ${a.level}`]])}</div>${btn}</div>`);
      }
    }
    $('shop-list').innerHTML = rows.join('');
  }

  // ---------- Nút trong thanh tiêu đề và hộp thoại ----------

  async function onClick(e) {
    if (e.target.closest('#world')) return; // cảnh xử lý riêng
    const t = e.target.closest('button');
    if (!t) return;
    if (t.closest('.toolbar, .pan-btns')) t.blur(); // để Space / Enter về sau dùng cho nhân vật
    const d = t.dataset;
    if (d.close !== undefined) return t.closest('dialog').close();
    if (d.go) {
      closeSeedPick();
      return goTo(d.go);
    }
    if (d.tool) return pickTool(d.tool);
    if (d.seedSel) return selectSeed(d.seedSel);
    if (d.shop !== undefined) {
      closeSeedPick();
      return openShop(shopMode.tab);
    }
    if (d.plant) return plant(d.plant);
    if (d.qty) {
      buyQty = +d.qty;
      if ($('dlg-shop').open) renderShop();
      if ($('dlg-store').open) renderStore();
      return;
    }
    if (d.buySeed) {
      const c = find('crops', d.buySeed);
      const res = await call('/api/farm/seeds/buy', { crop: d.buySeed, qty: buyQty });
      if (res) A.toast(`Đã cất ${buyQty} hạt ${c.name} vào kho giống`);
      return;
    }
    if (d.storeExpand) {
      const s = store(d.storeExpand);
      const units = (cat && cat.storage && cat.storage[d.storeExpand] && cat.storage[d.storeExpand].expandUnits) || 25;
      if (!confirm(`Mở rộng ${d.storeExpand === 'seeds' ? 'kho giống' : 'kho thóc'} thêm ${units} chỗ với giá ${xu(s.nextExpandPrice)} xu?`)) return;
      return call(`/api/farm/storage/${d.storeExpand}/expand`, {});
    }
    if (d.sellItem) {
      const res = await call('/api/farm/products/sell', { item: d.sellItem, qty: +d.sellQty });
      if (res) floatGain(t, res.gained);
      return;
    }
    if (d.sellAll !== undefined) {
      const res = await call('/api/farm/products/sell-all', {});
      if (res) floatGain(t, res.gained);
      return;
    }
    if (d.buyPen) {
      const cfg = cat[d.buyPen];
      if (!confirm(`Mua ${d.buyPen === 'barn' ? 'chuồng gia súc' : 'ao cá'} với giá ${xu(cfg.price)} xu?`)) return;
      return call(`/api/farm/${d.buyPen}/buy`, {});
    }
    if (d.expand) {
      const pen = farm[d.expand];
      if (!confirm(`Mở rộng thêm ${cat[d.expand].expandUnits} đơn vị với giá ${xu(pen.nextExpandPrice)} xu?`)) return;
      return call(`/api/farm/${d.expand}/expand`, {});
    }
    if (d.buyAnimal) return call('/api/farm/animals/buy', { kind: d.buyAnimal });
    if (d.buyFish) return call('/api/farm/fish/buy', { kind: d.buyFish });
    if (d.feedAnimal || d.feedFish) {
      const p = pets.get(+(d.feedFish || d.feedAnimal));
      if (p) return feedPet(p);
      return call(`/api/farm/${d.feedFish ? 'fish' : 'animals'}/feed`, { id: +(d.feedFish || d.feedAnimal) });
    }
    if (d.collectAnimal || d.collectFish) {
      const isFish = !!d.collectFish;
      return collect(isFish, +(d.collectFish || d.collectAnimal), document.querySelector(`[data-obj="${isFish ? 'pond' : 'barn'}"]`));
    }
    if (d.openShop) return openShop(d.openShop);
  }

  async function load() {
    const res = await call('/api/farm');
    if (res) $('farm').classList.remove('hidden');
    return res;
  }

  // ---------- Sự kiện ----------

  document.addEventListener('click', onClick);
  $('dlg-shop').addEventListener('close', () => (shopMode.plot = null));
  document.querySelectorAll('#shop-tabs .tab').forEach((b) => b.addEventListener('click', (e) => {
    e.stopPropagation();
    shopMode = { tab: b.dataset.tab, plot: null };
    renderShop();
  }));
  $('btn-harvest-all').addEventListener('click', async (e) => {
    const btn = e.currentTarget;
    const ripe = farm.plots.filter((p) => p.crop && nowS() >= p.readyAt).map((p) => document.querySelector(`[data-obj="plot${p.index}"]`));
    const res = await call('/api/farm/harvest-all', {});
    if (!res) return;
    const items = (res.gained && res.gained.items) || {};
    // Mỗi ô chín bay một món về kho.
    const flat = Object.entries(items).flatMap(([id, n]) => Array(n).fill(id));
    ripe.slice(0, 8).forEach((el, i) => flat[i] && flyToStore(el, { [flat[i]]: 1 }));
    floatGain(btn, res.gained, itemsText(items));
  });
  document.addEventListener('visibilitychange', () => document.visibilityState === 'visible' && load());
  window.addEventListener('resize', () => setZoom());

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(28);
  A.me().then(
    async (u) => {
      if (!u) return A.requireLogin();
      user = u;
      me.look = lookOf(u);
      renderHUD();
      const manifest = await fetch(IMG + 'manifest.json').then((r) => r.json()).catch(() => ({}));
      for (const f of manifest.images || manifest.files || []) sizes[f.file.replace(/\.png$/, '')] = [f.w, f.h];
      cat = await A.api('/api/farm/catalog').catch((e) => (A.toast(e.message), null));
      if (await load()) {
        setZoom();
        // Vừa bước qua cổng nông trại.
        // Vừa bước qua cổng "Vào".
        me.x = 586;
        me.y = 560;
        me.dir = 'up';
        drawMe();
        updateHint();
        requestAnimationFrame(() => follow(true));
      }
      requestAnimationFrame(frame);
      setInterval(tick, 1000);
      setInterval(load, 15000);
    },
    (e) => A.toast(e.message),
  );

  // Cho kiểm thử tự động / gỡ lỗi.
  window.FarmScene = {
    position: () => ({ x: Math.round(me.x), y: Math.round(me.y), dir: me.dir }),
    objects: () => objs,
    goTo,
    walkTo,
    blocked: (x, y) => blocked(x, y),
    // Toạ độ thế giới → toạ độ màn hình.
    screen: (x, y) => {
      const r = $('world-wrap').getBoundingClientRect();
      return { x: r.left + x * zoom, y: r.top + (y + OY) * zoom };
    },
  };
})();
