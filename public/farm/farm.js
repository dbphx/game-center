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
  const sheets = {}; // sprite sheet: tên → { fw, fh, n, frames } (manifest frameW / frameH / frames)

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
  const find = (list, id) =>
    (cat ? (cat[list] || []).find((x) => x.id === id) || (list === 'animals' ? (cat.poultry || []).find((x) => x.id === id) : null) : null) ||
    { id, name: id, units: 1 };
  const allStock = () => [...((cat && cat.animals) || []), ...((cat && cat.poultry) || [])];
  const EMPTY_STORE = { capacity: 0, used: 0, maxCapacity: 0, nextExpandPrice: 0, items: {} };
  const store = (kind) => (farm && farm.storage && farm.storage[kind]) || EMPTY_STORE;

  // Món trong kho sản phẩm: nông sản, vật nuôi hoặc cá.
  function product(id) {
    // Trứng / sữa / lông / thịt: lấy từ catalog.animals[i].product / .meat, hình item_<id>.png.
    for (const a of allStock()) {
      for (const it of [a.product, a.meat]) {
        if (it && it.id === id) return { ...it, list: 'goods', draw: (h) => art.item(id, h) };
      }
    }
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

  // Co cỡ chữ cho vừa khung (giảm dần tới min px); vẫn dài thì cắt "…" và để tên đầy đủ ở title.
  function fitText(el, max, min = 10) {
    if (!el) return;
    el.title = el.textContent;
    for (let f = max; f >= min; f -= 0.5) {
      el.style.fontSize = `${f}px`;
      if (el.scrollWidth <= el.clientWidth + 0.5) return;
    }
  }

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
    fitText($('farm-name'), innerWidth < 600 ? 16 : 20, 10);
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
  const BY = [158, 158 + BLOCK_H + 36]; // hàng khối ruộng
  // Chuồng và ao to theo sức chứa (5 → 30 đơn vị, rộng tới ~2.2 lần); mọi thứ bên phải chuồng dời theo OX.
  const PEN = { x1: 18, y1: 164, x2: 318, y2: 600 }; // chuồng gia súc (hàng rào)
  const BX = [0, 0]; // cột khối ruộng (lối đất giữa đủ rộng để đi)
  const POND_BOX = { x: 0, y: 250, w: 260, h: 200 }; // lô cỏ có ao
  let OX = 0; // độ dời ngang phần bên phải chuồng (chuồng nới rộng)
  let W = 1700;
  let CORE_W = 1700; // bề rộng cảnh khi chuồng + ao nhỏ nhất — luôn vừa một khung nhìn; to hơn thì kéo ngang
  const capK = (c) => Math.min(1, Math.max(0, ((c || 5) - 5) / 25));
  function sizeLayout() {
    const barnCap = farm && farm.barn ? farm.barn.capacity : 5;
    const pondCap = farm && farm.pond ? farm.pond.capacity : 5;
    PEN.x2 = PEN.x1 + Math.round(300 + 360 * capK(barnCap));
    OX = PEN.x2 - 400;
    BX[0] = 804 + OX;
    BX[1] = BX[0] + BLOCK_W + 48;
    POND_BOX.w = Math.round(260 + 310 * capK(pondCap));
    POND_BOX.h = Math.round(200 + 80 * capK(pondCap));
    POND_BOX.x = BX[1] + BLOCK_W + 40;
    POND_BOX.y = Math.round(372 - POND_BOX.h / 2);
    W = POND_BOX.x + POND_BOX.w + 40;
    CORE_W = 804 - 82 + 2 * BLOCK_W + 48 + 40 + 260 + 40;
    Object.assign(ROAM, { x1: PEN.x1 + 34, y1: 330, x2: PEN.x2 - 30, y2: PEN.y2 - 24 });
    const ww = POND_BOX.w - 30;
    Object.assign(POND, { cx: POND_BOX.x + POND_BOX.w / 2, cy: POND_BOX.y + 18 + ww * 0.305, rx: ww * 0.34, ry: ww * 0.15 });
    NESTS.splice(0, NESTS.length, [584 + OX, 330], [620 + OX, 342], [656 + OX, 330]);
  }
  let OY = 0; // khoảng trời thêm phía trên khi khung nhìn cao hơn cảnh
  let objs = {}; // key → { x, y (chỗ đứng), face, hx, hy (gợi ý), verb, run() }
  let obstacles = []; // [x1, y1, x2, y2] chân nhân vật không được vào

  function tiles() {
    const owned = farm.plots.length;
    const max = farm.maxPlots || 48;
    return { owned, max, buy: owned < max && !!farm.nextPlotPrice, left: max - owned };
  }

  function computeLayout() {
    sizeLayout();
    const X = (n) => n + OX;
    obstacles = [
      // Chuồng: hàng rào 4 phía, chừa cổng ở hông phải (y 410–488) để đi vào trong.
      [PEN.x1 - 4, PEN.y1, PEN.x1 + 12, PEN.y2], // rào trái
      [PEN.x2 - 10, PEN.y1, PEN.x2 + 6, 404], // rào phải (trên cổng)
      [PEN.x2 - 10, 494, PEN.x2 + 6, PEN.y2], // rào phải (dưới cổng)
      [PEN.x1, PEN.y1, PEN.x2, PEN.y1 + 48], // rào sau
      [PEN.x1, PEN.y2 - 14, PEN.x2, PEN.y2], // rào trước
      [52, 262, 250, 300], // chòi chuồng
      [PEN.x2 - 140, 536, PEN.x2 - 14, 566], // cây rơm trong chuồng
      [X(404), 262, X(604), 296], // kho thóc
      [X(640), 270, X(772), 296], // kho giống
      [POND_BOX.x + 14, POND_BOX.y - 24, POND_BOX.x + 70, POND_BOX.y - 6], // giếng (cạnh cây đa sau ao)
      [POND_BOX.x + POND_BOX.w / 2 - 20, POND_BOX.y - 14, POND_BOX.x + POND_BOX.w / 2 + 40, POND_BOX.y + 4], // gốc cây đa
      [X(588), 476, X(612), 490], // gốc cây mai lô giữa
      [X(520), 592, X(544), 608], // cột cổng trái
      [X(628), 592, X(652), 608], // cột cổng phải
      [POND_BOX.x + 14, POND_BOX.y + 22, POND_BOX.x + POND_BOX.w - 14, POND_BOX.y + POND_BOX.h - 12], // ao
    ];
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

  // z khi đứng trong khối ruộng: nổi trên cây của chính ô đang đứng (cây hàng sau vẫn bị che đúng chiều sâu),
  // để nhân vật / gà vịt không bị cây trồng dưới chân che mất.
  function fieldZ(x, y) {
    for (let b = 0; b < 4; b++) {
      const bx = BX[b % 2];
      const by = BY[Math.floor(b / 2)];
      if (x < bx || x > bx + BLOCK_W || y < by || y > by + BLOCK_H) continue;
      const row = Math.min(BLOCK_ROWS - 1, Math.max(0, Math.floor((y - by - 10) / PITCH)));
      return Math.max(Math.round(y), by + 10 + row * PITCH + PLOT + 1);
    }
    return Math.round(y);
  }

  // Chỗ đứng để làm một ô: ngay trên ô (nhân vật đi vào trong khối ruộng được), quay mặt lên.
  const onSpot = (x, y) => ({ x: x + PLOT / 2, y: y + PLOT - 10, face: 'up' });

  function fieldHTML() {
    const t = tiles();
    const out = [];
    for (let b = 0; b < Math.ceil(t.max / PER_BLOCK); b++) {
      const used = t.owned > b * PER_BLOCK || (t.buy && t.owned === b * PER_BLOCK);
      const x = BX[b % 2];
      const y = BY[Math.floor(b / 2)];
      out.push(`<span class="field-bed${used ? '' : ' reserved'}" style="left:${x}px;top:${y}px;width:${BLOCK_W}px;height:${BLOCK_H}px;z-index:3"></span>`);
      if (!used) out.push(`<span class="block-sign" style="${pos(x + BLOCK_W / 2, y + BLOCK_H / 2 + 12, 4)}">Đất chưa mở</span>`);
    }
    farm.plots.forEach((p, i) => {
      const [x, y] = plotXY(i);
      out.push(plotHTML(p, x, y));
      objs[`plot${p.index}`] = { ...onSpot(x, y), hx: x + PLOT / 2, hy: y - 6, verb: plotVerb(p), run: () => plotAction(p.index) };
    });
    for (let i = t.owned; i < t.max; i++) {
      const [x, y] = plotXY(i);
      if (i === t.owned && t.buy) {
        out.push(`<div class="plot buy" data-obj="buyplot" style="${pos(x, y, y + PLOT)}" title="Mở thêm ô đất">
          <span class="soil grass"></span><span class="lock">${I.svg('lock')}</span>
          <span class="tag-pill price">${I.svg('coins')}${xu(farm.nextPlotPrice)}</span></div>`);
        objs.buyplot = { ...onSpot(x, y), hx: x + PLOT / 2, hy: y - 6, verb: 'Mở ô đất', run: buyPlot };
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

  // Lô đất trống chờ mua (chưa có chuồng / ao): mảng cỏ + biển "Mua …".
  function emptyLot(kind, x, y, w, h) {
    const cfg = cat ? cat[kind] : { price: 0, level: 1 };
    const locked = user.level < cfg.level;
    const name = kind === 'barn' ? 'Mua chuồng' : 'Mua ao';
    return `<div class="obj area empty-lot" data-obj="${kind}" style="left:${x}px;top:${y}px;width:${w}px;height:${h}px;z-index:3">
      <span class="lot-sign">${art.img('fence_piece', { h: 34 })}<b>${name}</b><em>${locked ? `${I.svg('lock')}Cần Lv ${cfg.level}` : `${I.svg('coins')}${xu(cfg.price)} xu`}</em></span></div>`;
  }

  function yardHTML() {
    const pen = farm.barn;
    const w = PEN.x2 - PEN.x1;
    const h = PEN.y2 - PEN.y1;
    if (!pen) return emptyLot('barn', PEN.x1, PEN.y1, w, h);
    // Nền cỏ chuồng nằm dưới cùng; chòi + hàng rào là vật riêng để con vật đứng trước / sau đúng chiều sâu.
    return `<div class="obj area yard" data-obj="barn" style="left:${PEN.x1}px;top:${PEN.y1}px;width:${w}px;height:${h}px;z-index:3">
        <span class="fence-top"></span></div>
      <span class="pen-label" data-obj="barn" style="left:${PEN.x1 + w / 2}px;top:${PEN.y1 - 8}px;z-index:2000">${penLabel('barn', pen)}</span>
      <span class="obj decor pen-shed" data-obj="barn" style="${pos(150, 300)}">${art.img('pen_shed', { h: 190 })}</span>
      <span class="obj decor haystack" style="${pos(PEN.x2 - 76, 566)}">${art.img('haystack', { h: 176 })}</span>
      <span class="obj decor" style="${pos(Math.min(PEN.x2 - 40, 290), 318)}">${art.img('trough_water', { h: 26 })}</span>
      <span class="fence-thick" style="left:${PEN.x1}px;top:${PEN.y2 - 52}px;width:${w}px;z-index:${PEN.y2}"></span>
      <span class="obj decor fence-side" style="left:${PEN.x1 - 22}px;top:${PEN.y1 - 6}px;z-index:${PEN.y1 + 60}">${art.img('fence_side_full', { h: 440 })}</span>
      <span class="obj decor fence-side" style="left:${PEN.x2 - 24}px;top:${PEN.y1 - 6}px;z-index:${PEN.y1 + 60}">${art.img('fence_side', { h: 440 })}</span>`;
  }

  function pondHTML() {
    const pen = farm.pond;
    const b = POND_BOX;
    if (!pen) return emptyLot('pond', b.x, b.y, b.w, b.h);
    return `<div class="obj area pond-area" data-obj="pond" style="left:${b.x}px;top:${b.y}px;width:${b.w}px;height:${b.h}px;z-index:${b.y + 40}">
      ${penLabel('pond', pen)}
      <span class="deco" style="left:0;bottom:2px">${art.img('flowers', { h: 40 })}</span>
      <span class="deco" style="right:0;bottom:2px">${art.img('bush_3', { h: 34 })}</span>
      <div class="pond-water" style="width:${b.w - 30}px">${art.img('pond', { cls: 'pond-img' })}</div>
    </div>`;
  }

  // Hiệu ứng nới rộng khi vừa mở rộng chuồng / ao: rào dời ra, nước loang ra.
  const lastCap = { barn: 0, pond: 0 };
  function growFx() {
    for (const [kind, sel] of [['barn', '.yard'], ['pond', '.pond-area']]) {
      const cap = farm[kind] ? farm[kind].capacity : 0;
      const old = lastCap[kind];
      lastCap[kind] = cap;
      if (!old || cap <= old) continue;
      const el = document.querySelector(sel);
      if (!el) continue;
      const k = kind === 'barn' ? (300 + 360 * capK(old)) / (300 + 360 * capK(cap)) : (260 + 310 * capK(old)) / (260 + 310 * capK(cap));
      el.animate([{ transform: `scaleX(${k.toFixed(3)})`, transformOrigin: kind === 'barn' ? '0 50%' : '50% 50%' }, { transform: 'none' }], { duration: 900, easing: 'cubic-bezier(.3,1.4,.5,1)' });
      A.toast(kind === 'barn' ? `Chuồng đã rộng thêm — ${cap} đơn vị` : `Ao đã rộng thêm — ${cap} đơn vị`);
    }
  }

  // Đồ trang trí đặt theo chân (x, y).
  const deco = (name, x, y, h, flip = false, cls = '') =>
    `<span class="obj decor${flip ? ' flip' : ''}${cls ? ' ' + cls : ''}" data-tree="${/^tree_(mai|dao)/.test(name) ? name.slice(5) : ''}" style="${pos(x, y)}">${art.img(name, { h })}</span>`;

  function nestsHTML() {
    const eggs = NESTS.map(() => []);
    for (const p of pets.values()) {
      if (LAYERS.has(p.kind) && prodReady(p.data)) eggs[p.id % NESTS.length].push(p.kind === 'duck' ? 'duck_egg' : 'egg');
    }
    return NESTS.map(([x, y], i) => `<span class="obj nest" data-obj="nest" style="${pos(x, y, 6)}">${art.img('nest', { h: 30 })}
      <span class="nest-eggs">${eggs[i].slice(0, 4).map((e) => art.item(e, 15)).join('')}</span></span>`).join('');
  }
  function refreshNests() {
    const el = document.querySelectorAll('.obj.nest');
    if (!el.length) return;
    const wrap = document.createElement('div');
    wrap.innerHTML = nestsHTML();
    el.forEach((n, i) => wrap.children[i] && n.replaceWith(wrap.children[i].cloneNode(true)));
  }

  // Khóm hoa nhỏ nhiều màu rải trên các mảng cỏ (ngẫu nhiên cố định theo seed) — lớp mặt đất dưới nhân vật.
  function flowersHTML() {
    let seed = 20240214;
    const r = () => ((seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296);
    const colors = ['red', 'yellow', 'pink', 'purple', 'white'];
    const zones = [
      [PEN.x1 + 24, 340, PEN.x2 - 24, PEN.y2 - 30, 10], // trong chuồng
      [480 + OX, 362, 700 + OX, 455, 5], // lô cỏ giữa
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
    const X = (n) => n + OX;
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
    for (const x of [Math.round(PEN.x2 * 0.65), BX[0] + 140, BX[1] + 160]) parts.push(`<span class="obj decor far" style="${pos(x, SKY + 10, 2)}">${art.img('tree_big', { h: 250 })}</span>`);
    parts.push(`<span class="grass-edge bottom" style="left:0;top:${BOTTOM - 4}px;width:${W}px;height:${H - BOTTOM + 4}px;z-index:4"></span>`);
    parts.push(`<span class="bush-row" style="left:0;top:${BOTTOM - 10}px;width:${W}px;z-index:${H}"></span>`);

    // Chuồng gia súc ở mép trái.
    parts.push(yardHTML());
    objs.barn = { x: PEN.x2 + 26, y: 450, face: 'left', hx: (PEN.x1 + PEN.x2) / 2, hy: PEN.y1 + 14, verb: farm.barn ? 'Xem chuồng' : 'Mua chuồng', run: () => openPen('barn') };

    // Hai kho ở rìa: sát hàng rào sau, cạnh chuồng.
    parts.push(storeObjHTML('products', X(504), 296, 'hut_rice', 190));
    parts.push(storeObjHTML('seeds', X(706), 296, 'hut_seed', 168));
    objs.products = { x: X(504), y: 316, face: 'up', hx: X(504), hy: 84, verb: 'Mở kho', run: () => openStore('products') };
    objs.seeds = { x: X(706), y: 316, face: 'up', hx: X(706), hy: 104, verb: 'Mở kho', run: () => openStore('seeds') };

    // Lô cỏ nhỏ giữa (giếng, cây ăn trái, hoa) và gà con đi dạo.
    parts.push(`<span class="patch" style="left:${X(458)}px;top:340px;width:250px;height:156px;z-index:3"></span>`);
    parts.push(deco('tree_mai', X(600), 490, 196, false, 'sway blossom'));
    parts.push(deco('flowers', X(470), 488, 30));

    // Ổ rơm (ổ đẻ) trên lô cỏ giữa: trứng của gà / vịt đã lớn hiện trong ổ.
    parts.push(nestsHTML());
    objs.nest = { x: X(620), y: 360, face: 'up', hx: X(620), hy: 300, verb: 'Ổ đẻ · gia cầm', run: nestAction };

    // Cổng "Vào" dưới giữa (giữa kho và ruộng).
    // z của cổng thấp hơn chỗ đứng trước cổng (y 604) để nhân vật đứng TRƯỚC biển cổng.
    parts.push(`<div class="obj gate" data-obj="gate" style="${pos(X(586), 610, 592)}" title="Về thành phố">
      ${art.img('farm_gate', { h: 140 })}<span class="gate-sign">Nông trại<b>${esc(user.name)}</b></span></div>`);
    objs.gate = { x: X(586), y: 604, face: 'down', hx: X(586), hy: 460, verb: 'Về thành phố', run: () => (location.href = '/') };

    parts.push(fieldHTML());

    // Ao cá ở mép phải.
    parts.push(pondHTML());
    objs.pond = { x: POND_BOX.x - 18, y: POND_BOX.y + 130, face: 'right', hx: POND_BOX.x + POND_BOX.w / 2, hy: POND_BOX.y + 8, verb: farm.pond ? 'Xem ao' : 'Mua ao', run: () => openPen('pond') };

    // Cây kiểu Việt ở rìa: mai, đào hai bên cổng, đào trong chuồng, chuối + tre sau ao, mai góc phải.
    parts.push(deco('tree_dao', X(444), 606, 190, false, 'sway blossom'));
    parts.push(deco('tree_dao', X(724), 606, 186, true, 'sway blossom'));
    parts.push(deco('tree_chuoi', BX[0] - 40, SKY + 26, 220, false, 'sway'));
    // Cây đa to sau ao (tán vươn lên trời, không che ruộng / lối).
    parts.push(deco('tree_da', POND_BOX.x + POND_BOX.w / 2 + 10, POND_BOX.y + 4, 400, false, 'sway slow big'));
    parts.push(deco('well', POND_BOX.x + 42, POND_BOX.y - 4, 76));
    parts.push(deco('tree_mai', W - 34, 606, 190, true, 'sway blossom'));
    // Dừa cạnh ao + góc phải; luỹ tre dọc hàng rào sau.
    parts.push(deco('tree_dua', POND_BOX.x + POND_BOX.w - 6, Math.min(590, POND_BOX.y + POND_BOX.h + 60), 330, true, 'sway slow'));
    parts.push(deco('tree_dua', W - 8, 262, 300, true, 'sway slow'));
    for (const [x, h] of [[44, 300], [BX[1] + BLOCK_W + 22, 300]]) parts.push(deco('tree_tre', x, SKY + 34, h, x % 2 === 1, 'sway slow'));
    // Luỹ tre rậm sau hai nhà kho: nhiều bụi chồng nhau, cao hơn mái, đung đưa lệch pha (z thấp hơn kho).
    for (let i = 0; i < 13; i++) {
      const x = X(400) + i * 30 + (i % 2) * 8;
      const y = SKY + 40 + (i % 3) * 14;
      const h = 290 + ((i * 37) % 5) * 14;
      parts.push(deco('tree_tre', x, y, h, i % 2 === 1, 'sway slow').replace('style="', `style="--sway-delay:-${(i * 0.73) % 6}s;`));
    }
    parts.push(flowersHTML());

    $('objects').innerHTML = parts.join('');
    fitText(document.querySelector('.gate-sign b'), 16, 8);
    syncPets();
    growFx();
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
    let n = 0;
    const btn = (attr, pic, label, extra = '', on = false) =>
      `<button class="tool${on ? ' on' : ''}" ${attr} title="${label} (phím ${++n})"><span class="ic">${pic}</span><b>${label}</b>${extra}<kbd class="tool-key">${n}</kbd></button>`;
    const seedPic = seedSel ? art.seed(seedSel, 40) : art.seed('carrot', 40);
    $('toolbar').innerHTML = [
      btn('data-tool="plant"', seedPic, tool === 'plant' && seedSel ? find('crops', seedSel).name : 'Gieo hạt', '', tool === 'plant'),
      btn('data-tool="harvest"', art.item('corn', 40), 'Thu hoạch', badge(ripe), tool === 'harvest'),
      btn('data-go="seeds"', art.img('hut_seed', { h: 42 }), 'Kho giống', badge(seeds.used)),
      btn('data-go="products"', art.img('hut_rice', { h: 36 }), 'Kho thóc', badge(prods.used)),
      btn('data-shop', art.img('wheelbarrow', { h: 36 }), 'Cửa hàng'),
      btn('data-go="gate"', art.img('farm_gate', { h: 38 }), 'Về thành phố'),
      btn('data-quick-menu', art.img('nest', { h: 26 }) , 'Làm nhanh', badge(quickCount())),
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

  // ---------- Làm nhanh: gieo / thu hoạch / cho ăn / lấy sản phẩm tất cả ----------

  function quickCount() {
    if (!farm) return 0;
    const ripe = farm.plots.filter((p) => p.crop && nowS() >= p.readyAt).length;
    let n = ripe;
    for (const p of pets.values()) if (isHungry(p.data) || (!p.fish && prodReady(p.data))) n++;
    return n;
  }

  function renderQuick() {
    const empty = farm.plots.filter((p) => !p.crop).length;
    const ripe = farm.plots.filter((p) => p.crop && nowS() >= p.readyAt).length;
    const all = [...pets.values()];
    const hungry = all.filter((p) => isHungry(p.data)).length;
    const goods = all.filter((p) => !p.fish && prodReady(p.data)).length;
    const row = (act, pic, label, n, key, dis) =>
      `<button class="chip quick" data-quick="${act}" ${dis ? 'disabled' : ''}>${pic}<b>${label}</b><em>${n}</em><kbd>${key}</kbd></button>`;
    $('quick-menu').innerHTML = `<p>Làm nhanh</p><div class="chips">
      ${row('plant-all', art.seed(seedSel || 'carrot', 36), 'Gieo tất cả', `${empty} ô trống`, 'Shift+1', !empty)}
      ${row('harvest-all', art.item('corn', 36), 'Thu hoạch tất cả', `${ripe} ô chín`, 'H', !ripe)}
      ${row('feed-all', art.img('feed_grain', { h: 22 }), 'Cho ăn tất cả', `${hungry} con đói`, 'Shift+F', !hungry)}
      ${row('produce-all', art.item('egg', 34), 'Trứng & sản phẩm', `${goods} con`, 'Shift+P', !goods)}
      ${row('yard', art.img('nest', { h: 24 }), 'Đàn gia cầm', farm.yard ? `${farm.yard.used}/${farm.yard.capacity}` : '—', 'G', false)}
    </div>`;
  }

  function toggleQuick() {
    const m = $('quick-menu');
    if (!m.classList.contains('hidden')) return m.classList.add('hidden');
    closeSeedPick();
    renderQuick();
    m.classList.remove('hidden');
    focusFirst(m);
  }

  // Nhân vật làm một nhịp nhanh (vung tay) khi làm hàng loạt.
  function workFx(times = 2) {
    const el = $('me');
    let i = 0;
    const once = () => {
      el.classList.remove('work');
      void el.offsetWidth;
      el.classList.add('work');
      if (++i < times) setTimeout(once, 430);
    };
    once();
  }

  // Gọi API làm hàng loạt; server chưa có endpoint (404 / 405) → trả null để làm từng việc.
  async function bulk(path, body) {
    try {
      const data = await A.api(path, { method: 'POST', body: body || {} });
      if (data.user) user = data.user;
      if (data.farm) setFarm(data.farm);
      renderHUD();
      return data;
    } catch (e) {
      if (e.status === 404 || e.status === 405) return null;
      if (e.status === 401) A.requireLogin();
      else A.toast(e.message);
      return false;
    }
  }

  function plantAll() {
    $('quick-menu').classList.add('hidden');
    const empty = farm.plots.filter((p) => !p.crop).length;
    if (!empty) return A.toast('Không còn ô trống để gieo');
    pickAll = true;
    renderSeedPick();
    $('seed-pick').classList.remove('hidden');
    focusFirst($('seed-pick'));
  }

  async function plantAllWith(crop) {
    const before = farm.plots.filter((p) => !p.crop).map((p) => p.index);
    workFx(3);
    let res = await bulk('/api/farm/plant-all', { crop });
    if (res === null) {
      for (const i of before) if (!(await call('/api/farm/plant', { plot: i, crop }))) break;
      res = true;
    }
    if (!res) return;
    const n = before.filter((i) => farm.plots[i] && farm.plots[i].crop).length;
    A.toast(`Đã gieo ${find('crops', crop).name} vào ${n} ô`);
  }

  async function feedAll() {
    $('quick-menu').classList.add('hidden');
    const hungry = [...pets.values()].filter((p) => isHungry(p.data));
    if (!hungry.length) return A.toast('Không con nào đang đói');
    workFx(2);
    let res = await bulk('/api/farm/feed-all');
    let xp = res && res.gained ? res.gained.xp : 0;
    if (res === null) {
      xp = 0;
      for (const p of hungry) {
        const r = await call(`/api/farm/${apiOf(p)}/feed`, { id: p.id });
        if (!r) break;
        xp += (r.gained && r.gained.xp) || 0;
      }
      res = true;
    }
    if (!res) return;
    for (const p of hungry) {
      if (!pets.has(p.id)) continue;
      dropFood(p.fish ? 'feed_pellet' : 'feed_grain', p.x, p.y + 2, p.fish);
      p.state = 'peck';
      p.until = 2.2;
    }
    A.toast(`Đã cho ${hungry.length} con ăn${xp ? ` · +${xp} XP` : ''}`);
  }

  async function produceAll() {
    $('quick-menu').classList.add('hidden');
    const ready = [...pets.values()].filter((p) => !p.fish && prodReady(p.data));
    if (!ready.length) return A.toast('Chưa có trứng / sữa / lông nào');
    workFx(3);
    let res = await bulk('/api/farm/produce-all');
    let got = (res && res.gained && res.gained.items) || {};
    if (res === null) {
      got = {};
      for (const p of ready) {
        const r = await producePet(p, true);
        if (!r) break;
        for (const [k, v] of Object.entries((r.gained && r.gained.items) || {})) got[k] = (got[k] || 0) + v;
      }
      res = true;
    }
    if (!res) return;
    flyToStore($('me'), got);
    floatGain($('me'), (res && res.gained) || {}, itemsText(got));
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
      focusFirst(pick);
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
    const empty = farm.plots.filter((p) => !p.crop).length;
    $('seed-pick').innerHTML = `<p>${pickAll ? `Gieo tất cả — chọn hạt cho ${empty} ô trống` : 'Chọn hạt để gieo'}</p><div class="chips">${crops
      .map((c) => {
        const have = seeds.items[c.id] || 0;
        return `<button class="chip${seedSel === c.id && tool === 'plant' ? ' on' : ''}" data-seed-sel="${c.id}">${art.seed(c.id, 40)}
          <b>${esc(c.name)}</b><em>${have ? `×${have}` : `${xu(c.seedPrice)} xu`}</em></button>`;
      })
      .join('')}</div>`;
  }

  let pickAll = false; // bảng chọn hạt đang mở để "Gieo tất cả"
  function selectSeed(id) {
    seedSel = id;
    closeSeedPick();
    if (pickAll) {
      pickAll = false;
      renderToolbar();
      return plantAllWith(id);
    }
    tool = 'plant';
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
    const sig = focusSig();
    if (!$('seed-pick').classList.contains('hidden')) renderSeedPick();
    if ($('dlg-shop').open) renderShop();
    if ($('dlg-store').open) renderStore();
    if ($('dlg-pen').open) renderPen();
    restoreFocus(sig, document.querySelector('dialog[open]') || (!$('seed-pick').classList.contains('hidden') ? $('seed-pick') : null));
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
    const eggSig = [...pets.values()].filter((p) => LAYERS.has(p.kind) && prodReady(p.data)).map((p) => p.id).join(',');
    if (eggSig !== tick.eggs) {
      tick.eggs = eggSig;
      refreshNests();
    }
    updateHint();
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
    duck: { fw: 137, fh: 142, face: 'left', speed: 40, say: 'Cạp cạp!' },
    dog: { fw: 200, fh: 170, face: 'left', speed: 62, say: 'Gâu gâu!' },
  };
  // Bù cỡ khi ảnh render nhỏ hơn bạn cùng loài (gà trắng render nhỏ hơn gà nâu).
  const SIZE_FIX = { chicken_white: 1.5 };
  // Gà, vịt thả rông khắp trang trại; heo, cừu, bò ở trong chuồng.
  const FREE = new Set(['chicken', 'duck']);
  const FISH_SHEET = {
    tilapia: { fw: 118, fh: 80 },
    catfish: { fw: 155, fh: 80 },
    carp: { fw: 93, fh: 80 },
    koi: { fw: 121, fh: 80 },
  };
  const ROAM = { x1: 52, y1: 330, x2: 288, y2: 576 }; // chỗ con vật đi lại trong chuồng (tính lại theo sức chứa)
  const POND = { cx: 0, cy: 0, rx: 98, ry: 42 }; // mặt nước ao (tính lại theo sức chứa)

  const pets = new Map(); // id → con vật / cá đang hiện
  let card = null; // { id, fish } thẻ đang mở

  const rnd = (a, b) => a + Math.random() * (b - a);
  // API theo nơi nuôi: cá → /fish, gia cầm trong sân (state.yard) → /poultry, gia súc → /animals.
  const apiOf = (p) => (p.fish ? 'fish' : p.yard ? 'poultry' : 'animals');
  // Con đã lớn có sản phẩm (trứng / sữa / lông) sẵn sàng chưa (tính theo giờ server, không cần tải lại).
  const yieldOf = (kind) => allStock().find((x) => x.id === kind) || {};
  const prodReady = (a) => a.readyAt <= nowS() && !!yieldOf(a.kind).product && ((a.productReadyAt && nowS() >= a.productReadyAt) || (!a.productReadyAt && a.productReady));
  const PRODUCE_VERB = { egg: 'Lấy trứng', duck_egg: 'Lấy trứng', milk: 'Vắt sữa', wool: 'Cắt lông' };
  const LAYERS = new Set(['chicken', 'duck']); // đẻ trứng vào ổ rơm
  const NESTS = [[492, 404], [534, 424], [496, 452]]; // ổ rơm trên lô cỏ giữa (dời theo OX)
  const isHungry = (a) => {
    if (a.readyAt <= nowS()) return false;
    if (typeof a.fedAt !== 'number' && typeof a.hungry === 'boolean') return a.hungry;
    const cd = (cat && cat.feedCooldown) || 600;
    return nowS() - Math.max(a.fedAt || 0, a.boughtAt) >= cd;
  };
  // Ảnh 3D (pet3d_*, sprite sheet nhiều hoạt ảnh) nếu manifest có; không thì ảnh cũ.
  const imgOf = (p) => {
    const n3 = `pet3d_${p.fish ? p.kind : p.look}`;
    if (sheets[n3]) return n3;
    return p.fish ? `fish3d_${p.kind}` : `pet_${p.look === 'duck' ? 'chicken_white' : p.look}`;
  };
  function sheetOf(p) {
    const m = sheets[imgOf(p)];
    const base = p.fish ? FISH_SHEET[p.kind] || FISH_SHEET.tilapia : PET[p.look] || PET.chicken;
    return m ? { ...base, fw: m.fw, fh: m.fh, n: m.n, frames: m.frames, animations: m.animations, cols: m.cols, face: m.face || base.face } : { ...base, n: p.fish ? 2 : 5 };
  }
  // Hệ số hiển thị: ảnh gấp 2; xa (y nhỏ) thì nhỏ lại, gần thì to; con chưa lớn nhỏ hơn.
  function scaleOf(p) {
    const young = p.data.readyAt > nowS() ? 0.78 : 1;
    if (p.fish) return 0.56 * young * (0.9 + 0.2 * ((p.y - (POND.cy - POND.ry)) / (2 * POND.ry)));
    const fix = SIZE_FIX[p.look] && sheets[imgOf(p)] && imgOf(p).startsWith('pet3d_') ? SIZE_FIX[p.look] : 1;
    if (p.free) return 0.62 * fix * young * (0.84 + 0.26 * ((p.y - SKY) / (BOTTOM - SKY)));
    return 0.62 * fix * young * (0.84 + 0.26 * ((p.y - ROAM.y1) / (ROAM.y2 - ROAM.y1)));
  }

  function newTarget(p) {
    if (p.fish) {
      const a = rnd(0, Math.PI * 2);
      const r = Math.sqrt(Math.random());
      p.tx = POND.cx + Math.cos(a) * POND.rx * r;
      p.ty = POND.cy + Math.sin(a) * POND.ry * r;
    } else if (p.free && LAYERS.has(p.kind) && prodReady(p.data)) {
      // Có trứng: về ổ rơm ngồi đẻ.
      const [nx, ny] = NESTS[p.id % NESTS.length];
      p.tx = nx + rnd(-6, 6);
      p.ty = ny - 2;
      p.goal = 'nest';
    } else if (p.free) {
      // Thả rông: chỗ trống bất kỳ ngoài chuồng, không quá xa chỗ đang đứng; vịt hay ra mép ao (bơi được).
      for (let i = 0; i < 30; i++) {
        let x;
        let y;
        if (p.look === 'duck' && Math.random() < 0.45) {
          const a = rnd(0, Math.PI * 2);
          const r = rnd(0.7, 0.95);
          x = POND.cx + Math.cos(a) * POND.rx * r;
          y = POND.cy + Math.sin(a) * POND.ry * r;
        } else if (p.kind === 'dog') {
          x = rnd(PEN.x2 + 40, BX[0] - 10);
          y = rnd(330, BOTTOM - 20);
        } else {
          x = p.x + rnd(-260, 260);
          y = p.y + rnd(-150, 150);
        }
        if (canStand(p, x, y)) {
          p.tx = x;
          p.ty = y;
          break;
        }
      }
    } else {
      p.tx = rnd(ROAM.x1, ROAM.x2);
      p.ty = rnd(ROAM.y1, ROAM.y2);
    }
    p.state = 'walk';
  }

  // Gà / vịt đứng được ở đâu: trong cảnh, ngoài chuồng, không vào vật cản (kho, gốc cây, ao — riêng vịt được xuống nước).
  const inPond = (x, y) => ((x - POND.cx) / POND.rx) ** 2 + ((y - POND.cy) / POND.ry) ** 2 < 1;
  const inPen = (x, y) => x > PEN.x1 - 14 && x < PEN.x2 + 14 && y > PEN.y1 - 10 && y < PEN.y2 + 6;
  function canStand(p, x, y) {
    if (x < 24 || x > W - 24 || y < SKY + 20 || y > BOTTOM - 8 || inPen(x, y)) return false;
    if (p.look === 'duck' && inPond(x, y)) return true;
    return !blocked(x, y);
  }
  // Chỗ thả gà / vịt lúc mới hiện: quanh lối đất trước chuồng và giữa trang trại.
  function freeSpawn(p) {
    for (let i = 0; i < 60; i++) {
      const x = rnd(430, W - 60);
      const y = rnd(SKY + 40, BOTTOM - 20);
      if (canStand(p, x, y)) return [x, y];
    }
    return [440, 520];
  }

  function petHTML(p) {
    const sh = sheetOf(p);
    const img = imgOf(p);
    return `<span class="pet-jump"><span class="pet-body" style="background-image:url('${IMG}${img}.png')"></span></span>
      <span class="pet-badges"></span>`;
  }

  // Đồng bộ danh sách con vật / cá với trạng thái server, giữ nguyên vị trí con đang có.
  function syncPets() {
    const layer = $('pets');
    const live = new Set();
    const add = (a, fish, yard) => {
      live.add(a.id);
      let p = pets.get(a.id);
      if (!p) {
        p = { id: a.id, kind: a.kind, fish, look: a.kind === 'chicken' && a.id % 2 ? 'chicken_white' : PET[a.kind] ? a.kind : 'chicken', dir: Math.random() < 0.5 ? -1 : 1, frame: 0, ft: 0, data: a };
        p.yard = !!yard;
        p.free = !fish && (p.yard || FREE.has(a.kind) || a.kind === 'dog');
        if (p.free) [p.x, p.y] = freeSpawn(p);
        else if (fish) {
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
        p.el.className = `pet${fish ? ' fish' : ''}${p.free ? ' free' : ''}${PET[p.look] && PET[p.look].cls ? ' ' + PET[p.look].cls : ''}`;
        p.el.dataset.pet = a.id;
        p.el.innerHTML = petHTML(p);
        p.body = p.el.querySelector('.pet-body');
        p.badges = p.el.querySelector('.pet-badges');
        layer.appendChild(p.el);
        pets.set(a.id, p);
      }
      p.data = a;
      if (a.kind !== 'dog') {
        p.el.title = find(fish ? 'fish' : 'animals', a.kind).name;
        drawBadges(p);
      }
    };
    ((farm.barn && farm.barn.animals) || []).forEach((a) => add(a, false));
    ((farm.yard && farm.yard.animals) || []).forEach((a) => add(a, false, true));
    ((farm.pond && farm.pond.fish) || []).forEach((a) => add(a, true));
    // Chó trang trí: đi loanh quanh cổng / kho, bấm thì vẫy nhảy + "Gâu gâu!".
    if (!pets.has(-1)) {
      add({ id: -1, kind: 'dog', boughtAt: 0, readyAt: 0, fedAt: 0 }, false, false);
      const d = pets.get(-1);
      d.decor = true;
      d.free = true;
      d.el.classList.add('decor-pet');
      d.el.title = 'Chó giữ nhà';
    }
    live.add(-1);
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
    const pr0 = prodReady(a);
    const key = `${ready}|${hungry}|${pr0}|${Math.round(progress(a.boughtAt, a.readyAt) * 20)}`;
    if (key === p.badgeKey) return;
    p.badgeKey = key;
    const pr = progress(a.boughtAt, a.readyAt);
    const prod = yieldOf(a.kind).product;
    p.el.classList.toggle('woolly', pr0 && a.kind === 'sheep');
    p.badges.innerHTML = ready && pr0 && prod && !LAYERS.has(a.kind)
      ? `<span class="pet-goods">${prod.id === 'wool' ? art.img('icon_scissors', { h: 24 }) : art.item(prod.id, 26)}</span>`
      : ready
      ? `<span class="pet-ready">!</span><span class="twinkle t1">${I.svg('sparkles')}</span><span class="twinkle t2">${I.svg('sparkles')}</span>`
      : `${hungry ? '<span class="bubble-hungry">Đói bụng</span>' : ''}<span class="mini-bar"><span style="width:${Math.round(pr * 100)}%"></span></span>`;
    p.el.classList.toggle('ready', ready);
    p.el.classList.toggle('hungry', hungry);
  }

  // ---------- Bộ phát hoạt ảnh (đọc từ manifest) ----------
  // manifest có thể khai "animations": [{ name, start, frames, fps }] (sprite sheet nhiều khung từ mô hình 3D):
  // idle, walk, peck / eat, turn, flap / hop, swim... Không có thì dựng từ danh sách khung đơn "frames"
  // (stand, step, breathe, peck, hop) của ảnh hiện tại. Tên thiếu thì lùi về tên gần nghĩa.
  const ANIM_FALLBACK = { look: ['look', 'idle', 'swim'], eat: ['eat', 'peck', 'idle', 'swim'], peck: ['peck', 'eat', 'idle'], hop: ['hop', 'flap', 'jump', 'idle', 'swim'],
    flap: ['flap', 'hop', 'idle'], swim: ['swim', 'walk', 'idle'], walk: ['walk', 'run', 'idle', 'swim'], idle: ['idle', 'stand', 'swim'], turn: ['turn', 'idle'] };
  const animCache = new Map();
  function animsOf(p) {
    const img = imgOf(p);
    if (animCache.has(img)) return animCache.get(img);
    const sh = sheetOf(p);
    const out = {};
    if (Array.isArray(sh.animations) && sh.animations.length) {
      for (const a of sh.animations) {
        const start = a.start != null ? a.start : (a.row || 0) * (sh.cols || 8);
        out[a.name] = { seq: Array.from({ length: a.frames || 1 }, (_, k) => start + k), fps: a.fps || 8, loop: a.loop !== false && a.name !== 'hop' };
      }
    } else if (p.fish) {
      out.idle = { seq: [0, 1], fps: 2.5 };
      out.swim = out.walk = { seq: [0, 1], fps: 4.5 };
      out.eat = { seq: [0, 1], fps: 8 };
    } else {
      const fr = sh.frames || ['stand', 'step', 'breathe', 'peck', 'hop'];
      const k = (n) => Math.max(0, fr.indexOf(n));
      out.idle = { seq: [k('stand'), k('breathe')], fps: 1.4 };
      out.walk = { seq: [k('stand'), k('step')], fps: 6 };
      out.peck = { seq: [k('stand'), k('peck')], fps: 3.8 };
      out.hop = { seq: [k('hop')], fps: 1 };
    }
    animCache.set(img, out);
    return out;
  }
  function playAnim(p, name) {
    const all = animsOf(p);
    for (const n of ANIM_FALLBACK[name] || [name, 'idle']) {
      if (all[n]) {
        if (p.anim !== n) {
          p.anim = n;
          p.at = 0;
        }
        const a = all[n];
        // Đi: tốc độ khung khớp tốc độ bước (con đi nhanh thì chân bước nhanh hơn).
        const fps = n === 'walk' && p.speedK ? a.fps * p.speedK : a.fps;
        const i = Math.floor(p.at * fps);
        p.frame = a.seq[a.loop === false ? Math.min(i, a.seq.length - 1) : i % a.seq.length];
        return;
      }
    }
    p.frame = 0;
  }

  // Mỗi khung hình: di chuyển, tránh nhau, chọn khung ảnh, đặt vị trí + cỡ theo chiều sâu.
  function stepPets(dt) {
    const list = [...pets.values()];
    for (const p of list) {
      p.ft += dt;
      p.at = (p.at || 0) + dt;
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
          } else if (p.goal === 'nest') {
            p.goal = null;
            p.state = 'nest';
            p.until = 9999; // ngồi ổ tới khi được nhặt trứng
          } else {
            p.state = p.fish ? 'walk' : Math.random() < 0.55 && /chicken|pig/.test(p.look) ? 'peck' : 'idle';
            p.until = rnd(1.2, 3.5);
            if (p.fish) newTarget(p);
          }
        } else {
          const nx = p.x + (dx / d) * v;
          const ny = p.y + (dy / d) * v;
          if (p.free && !canStand(p, nx, ny)) {
            newTarget(p); // vướng kho / gốc cây / nước → đổi hướng
          } else {
            p.x = nx;
            p.y = ny;
          }
          if (Math.abs(dx) > 1) p.dir = dx > 0 ? 1 : -1;
        }
      } else if (p.state === 'nest') {
        if (!prodReady(p.data)) {
          p.state = 'idle';
          p.until = rnd(0.5, 1.5);
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
    // Con vật né nhân vật khi nhân vật đi vào chuồng.
    for (const p of list) {
      if (p.fish || p.state === 'look' || p.state === 'hop') continue;
      const dx = p.x - me.x;
      const dy = (p.y - me.y) * 1.6;
      const d = Math.hypot(dx, dy) || 0.01;
      if (d < 40) {
        const push = (40 - d) * Math.min(1, dt * 6);
        p.x += (dx / d) * push;
        p.y += (dy / d) * push * 0.6;
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
      } else if (!p.free) {
        p.x = Math.min(ROAM.x2, Math.max(ROAM.x1, p.x));
        p.y = Math.min(ROAM.y2, Math.max(ROAM.y1, p.y));
      }
      // Vịt đang ở dưới nước: chỉ hiện nửa trên (đang bơi).
      p.el.classList.toggle('swimming', p.look === 'duck' && inPond(p.x, p.y));
      // Hoạt ảnh theo hành vi: đi → walk (dưới nước → swim), mổ / ăn → eat, bị bấm → hop / flap, đứng → idle.
      const swimming = p.look === 'duck' && inPond(p.x, p.y);
      let name = 'idle';
      if (p.state === 'walk' || p.state === 'food') name = p.fish || swimming ? 'swim' : 'walk';
      else if (p.state === 'peck') name = p.fish ? 'eat' : 'eat';
      else if (p.state === 'hop') name = 'hop';
      else if (p.state === 'nest') name = 'idle';
      else if (p.state === 'idle' || p.state === 'look') {
        if (p.state === 'look') name = 'look';
        else {
          if (!p.idleAnim || p.idleUntil < p.ft) {
            const r = Math.random();
            p.idleAnim = r < 0.5 ? 'idle' : r < 0.8 ? 'eat' : 'look';
            p.idleUntil = p.ft + rnd(1.5, 3.5);
          }
          name = p.idleAnim;
        }
      }
      p.speedK = p.state === 'food' ? 1.8 : 1;
      playAnim(p, name);
      const sh = sheetOf(p);
      const s = scaleOf(p);
      const w = sh.fw * s;
      const h = sh.fh * s;
      const n = sh.n || (p.fish ? 2 : 5);
      const el = p.el;
      el.style.left = `${(p.x - w / 2).toFixed(1)}px`;
      el.style.top = `${(p.y - h + (p.fish ? h / 2 : 8 * s)).toFixed(1)}px`;
      el.style.width = `${w.toFixed(1)}px`;
      el.style.height = `${h.toFixed(1)}px`;
      el.style.zIndex = p.fish ? 330 + Math.round(p.y - POND.cy + 40) : p.free ? fieldZ(p.x, p.y) : Math.round(p.y);
      const face = p.fish ? 1 : (sh.face || (PET[p.look] || PET.chicken).face) === 'left' ? -1 : 1;
      // Sprite sheet 1 hàng hoặc lưới nhiều hàng (manifest "columns").
      const cols = sh.cols && sh.cols < n ? sh.cols : n;
      const rows = Math.ceil(n / cols);
      p.body.style.backgroundSize = `${(sh.fw * cols * s).toFixed(1)}px ${(sh.fh * rows * s).toFixed(1)}px`;
      p.body.style.backgroundPosition = `${(-(p.frame % cols) * sh.fw * s).toFixed(1)}px ${(-Math.floor(p.frame / cols) * sh.fh * s).toFixed(1)}px`;
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
    const side = me.x > p.x ? 1 : -1;
    if (p.free) {
      const [x, y] = nearestFree(p.x + side * 44, p.y + 2);
      return { x, y, face: side > 0 ? 'left' : 'right' };
    }
    const [x, y] = nearestFree(Math.min(PEN.x2 - 30, Math.max(PEN.x1 + 30, p.x + side * 44)), p.y + 2);
    return { x, y, face: side > 0 ? 'left' : 'right' };
  }

  function clickPet(id) {
    const p = pets.get(id);
    if (!p) return;
    // Bấm lúc đang đi thì con dừng lại nhìn nhân vật ngay.
    p.state = 'look';
    p.until = 8;
    p.dir = me.x > p.x ? 1 : -1;
    const st = standPoint(p);
    const job = { key: '_pet', face: st.face, work: false, run: () => { react(p); if (!p.decor) openCard(p); } };
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
    const y = yieldOf(a.kind);
    const el = $('pet-card');
    let btns;
    if (card.confirm) {
      const meat = y.meat ? y.meat.name : 'thịt';
      btns = `<span class="pc-ask">Lấy ${esc(meat.toLowerCase())}? ${esc(info.name)} sẽ không còn nữa.</span>
        <button class="primary danger" data-card="meat-yes">${I.svg('check')}Lấy thịt <kbd>Enter</kbd></button>
        <button data-card="meat-no">Huỷ <kbd>Esc</kbd></button>`;
    } else if (p.fish) {
      btns = ready
        ? `<button class="primary" data-card="collect">${I.svg('sprout')}Thu hoạch <kbd>H</kbd></button>`
        : hungry ? `<button class="primary" data-card="feed">${I.svg('coins')}Cho ăn (${xu(price)} xu) <kbd>F</kbd></button>` : '';
    } else if (ready) {
      const list = [];
      if (y.product) {
        const verb = PRODUCE_VERB[y.product.id] || `Lấy ${y.product.name.toLowerCase()}`;
        list.push(prodReady(a)
          ? `<button class="primary" data-card="produce">${art.item(y.product.id, 18)}${verb} <kbd>P</kbd></button>`
          : `<button disabled>${art.item(y.product.id, 18)}${verb} sau ${countdown((a.productReadyAt || nowS()) - nowS())}</button>`);
      }
      list.push(`<button data-card="meat">${y.meat ? art.item(y.meat.id, 18) : ''}Lấy thịt <kbd>M</kbd></button>`);
      btns = list.join('');
    } else {
      btns = hungry
        ? `<button class="primary" data-card="feed">${I.svg('coins')}Cho ăn (${xu(price)} xu) <kbd>F</kbd></button>`
        : `<span class="muted">Cho ăn lại sau ${countdown(((cat && cat.feedCooldown) || 600) - (nowS() - Math.max(a.fedAt || 0, a.boughtAt)))}</span>`;
    }
    el.innerHTML = `<button class="link icon-only pc-close" data-card="close" title="Đóng">${I.svg('x')}</button>
      <b>${esc(info.name)}</b>
      <span class="pc-bar"><span style="width:${Math.round(pr * 100)}%"></span></span>
      <span class="pc-time">${ready ? (p.fish ? 'Đã lớn — thu hoạch được' : y.product ? `Đã lớn — cho ${esc(y.product.name.toLowerCase())}` : 'Đã lớn') : `Còn ${countdown(a.readyAt - nowS())}`}</span>
      ${ready ? '' : `<span class="pc-state ${hungry ? 'hungry' : 'full'}">${hungry ? 'Đói bụng' : 'No'}</span>`}
      <span class="pc-btns">${btns}</span>`;
    el.classList.remove('hidden');
    placeCard();
  }

  // Lấy trứng / vắt sữa / cắt lông một con.
  async function producePet(p, quiet) {
    const res = await call(`/api/farm/${apiOf(p)}/produce`, { id: p.id });
    if (!res) return null;
    const items = (res.gained && res.gained.items) || {};
    if (!quiet) {
      flyToStore(p.el, items);
      floatGain(p.el, res.gained, itemsText(items));
      p.state = 'hop';
      p.el.classList.remove('hop');
      void p.el.offsetWidth;
      p.el.classList.add('hop');
      setTimeout(() => {
        p.el.classList.remove('hop');
        p.state = 'idle';
        p.until = 1;
      }, 650);
    }
    return res;
  }

  // Ổ rơm: có trứng thì nhặt, không thì mở bảng đàn gia cầm.
  function nestAction() {
    const any = [...pets.values()].some((p) => LAYERS.has(p.kind) && prodReady(p.data));
    if (any) return collectEggs();
    return openPen(farm.yard ? 'yard' : 'barn');
  }

  // Nhặt trứng ở ổ rơm: mọi gà / vịt đang có trứng.
  async function collectEggs() {
    const layers = [...pets.values()].filter((p) => LAYERS.has(p.kind) && prodReady(p.data));
    if (!layers.length) return A.toast('Ổ chưa có trứng — gà / vịt đã lớn sẽ về ổ đẻ');
    const me0 = $('me');
    me0.classList.remove('work');
    void me0.offsetWidth;
    me0.classList.add('work');
    const nestEl = document.querySelector('[data-obj="nest"]');
    let xp = 0;
    const got = {};
    for (const p of layers) {
      const res = await producePet(p, true);
      if (!res) break;
      xp += (res.gained && res.gained.xp) || 0;
      for (const [k, v] of Object.entries((res.gained && res.gained.items) || {})) got[k] = (got[k] || 0) + v;
    }
    if (Object.keys(got).length) {
      flyToStore(nestEl, got);
      floatGain(nestEl, { xp }, itemsText(got));
    }
  }

  // Lấy thịt (con vật mất) — đã hỏi xác nhận trước.
  async function meatPet(p) {
    closeCard();
    await collect(false, p.id, p.el);
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
    const res = await call(`/api/farm/${apiOf(p)}/feed`, { id: p.id });
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
  const me = { x: 660, y: 560, dir: 'down', walking: false, look: {}, path: [], pending: null, drawn: '' };
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
      if ((+el.style.zIndex || 0) > fieldZ(me.x, me.y)) {
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
    el.style.zIndex = fieldZ(me.x, me.y);
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
    return [660 + OX, 560];
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
  // ---------- Điều khiển hoàn toàn bằng bàn phím ----------
  // Cảnh: WASD / mũi tên đi · E / Space / Enter tương tác · Tab / Q đổi mục tiêu · 1–6 thanh công cụ ·
  // H thu hoạch tất cả · [ ] đổi hạt khi cầm công cụ gieo · ? bảng phím tắt · Esc bỏ công cụ / đóng.
  // Hộp thoại / bảng chọn hạt: mũi tên di chuyển giữa các nút, Enter bấm, số 1–9 chọn nhanh hạt, Esc đóng.
  // Thẻ con vật: F cho ăn · H / E thu hoạch · ← → con kế bên · Esc đóng.
  const TOOL_KEYS = ['[data-tool="plant"]', '[data-tool="harvest"]', '[data-go="seeds"]', '[data-go="products"]', '[data-shop]', '[data-go="gate"]', '[data-quick-menu]'];

  // Chuyển focus theo hướng mũi tên giữa các nút trong một vùng (hộp thoại, bảng chọn hạt).
  function moveFocus(root, dir) {
    const items = [...root.querySelectorAll('button:not([disabled]), [tabindex="0"]')].filter((el) => el.offsetParent !== null);
    if (!items.length) return;
    const cur = root.contains(document.activeElement) ? document.activeElement : null;
    if (!cur || !items.includes(cur)) return items[0].focus();
    const a = cur.getBoundingClientRect();
    const ax = a.left + a.width / 2;
    const ay = a.top + a.height / 2;
    let best = null;
    let bs = Infinity;
    for (const el of items) {
      if (el === cur) continue;
      const b = el.getBoundingClientRect();
      const dx = b.left + b.width / 2 - ax;
      const dy = b.top + b.height / 2 - ay;
      const main = dir === 'left' ? -dx : dir === 'right' ? dx : dir === 'up' ? -dy : dy;
      if (main <= 4) continue;
      const side = dir === 'left' || dir === 'right' ? Math.abs(dy) : Math.abs(dx);
      const score = main + side * 2.5;
      if (score < bs) {
        bs = score;
        best = el;
      }
    }
    if (best) {
      best.focus();
      best.scrollIntoView({ block: 'nearest' });
    }
  }

  // Focus nút hợp lý đầu tiên khi mở hộp thoại / bảng.
  function focusFirst(root) {
    // Ưu tiên theo thứ tự (không theo thứ tự trong trang): gieo / chọn hạt, bán tất cả, mua hạt, thu hoạch, cho ăn...
    const order = ['[data-plant]', '[data-seed-sel]', '[data-sell-all]', '[data-buy-seed]', '[data-collect-animal]', '[data-collect-fish]',
      '[data-feed-animal]', '[data-feed-fish]', '[data-sell-item]', '.shop-list button', '.pen-body button', '#store-body button', 'button'];
    for (const sel of order) {
      const el = [...root.querySelectorAll(sel)].find((x) => !x.disabled && x.offsetParent !== null);
      if (el) return el.focus();
    }
  }

  // Dấu hiệu nút đang focus để khôi phục sau khi vẽ lại hộp thoại.
  function focusSig() {
    const el = document.activeElement;
    if (!el || el === document.body || !el.closest('dialog, #seed-pick')) return null;
    const attr = [...el.attributes].find((x) => x.name.startsWith('data-'));
    return attr ? `[${attr.name}="${CSS.escape(attr.value)}"]` : null;
  }
  function restoreFocus(sig, root) {
    if (!sig) return;
    const el = (root || document).querySelector(`${sig}:not([disabled])`);
    if (el) el.focus();
    else if (root) focusFirst(root);
  }

  // Shift + đi khi cầm công cụ: ô dưới chân (chưa làm trong lượt giữ Shift này) được gieo / thu hoạch liền.
  let shiftHeld = false;
  const swept = new Set();
  let sweepBusy = false;
  addEventListener('keydown', (e) => e.key === 'Shift' && (shiftHeld = true), true);
  addEventListener('keyup', (e) => {
    if (e.key !== 'Shift') return;
    shiftHeld = false;
    swept.clear();
  }, true);
  addEventListener('blur', () => ((shiftHeld = false), swept.clear()));
  // Ô đi qua được xếp hàng, làm lần lượt (không bỏ ô khi đang chờ server).
  const sweepQueue = [];
  function shiftSweep() {
    if (tool === 'plant' && !seedSel) return;
    for (const [k, o] of Object.entries(objs)) {
      const m = /^plot(\d+)$/.exec(k);
      if (!m || swept.has(+m[1]) || Math.abs(me.x - o.x) > PITCH / 2 || Math.abs(me.y - o.y) > PITCH / 2) continue;
      swept.add(+m[1]);
      const p = farm.plots[+m[1]];
      const ok = tool === 'plant' ? p && !p.crop : p && p.crop && nowS() >= p.readyAt;
      if (ok) sweepQueue.push([+m[1], tool === 'plant' ? 'plant' : 'harvest']);
    }
    drainSweep();
  }
  async function drainSweep() {
    if (sweepBusy) return;
    sweepBusy = true;
    try {
      while (sweepQueue.length) {
        const [i, intent] = sweepQueue.shift();
        await plotAction(i, intent);
      }
    } finally {
      sweepBusy = false;
    }
  }

  function cycleSeed(step) {
    if (!cat) return;
    const crops = cat.crops.filter((c) => user.level >= c.level);
    const k = Math.max(0, crops.findIndex((c) => c.id === seedSel));
    selectSeed(crops[(k + step + crops.length) % crops.length].id);
    A.toast(`Đang cầm hạt ${find('crops', seedSel).name}`);
  }

  function cardKeys(e) {
    const p = pets.get(card.id);
    if (!p) return false;
    const k = e.key.toLowerCase();
    if (card.confirm) {
      if (k === 'enter' || k === ' ') meatPet(p);
      else if (k === 'escape') {
        card.confirm = false;
        renderCard();
      }
      return k === 'enter' || k === ' ' || k === 'escape';
    }
    if (k === 'escape') return closeCard(), true;
    if (k === 'p' && !p.fish) {
      if (prodReady(p.data)) producePet(p).then(renderCard);
      else A.toast(yieldOf(p.kind).product ? 'Chưa có sản phẩm — đợi thêm' : `${find('animals', p.kind).name} không cho trứng / sữa / lông`);
      return true;
    }
    if (k === 'm' && !p.fish) {
      if (nowS() >= p.data.readyAt) {
        card.confirm = true;
        renderCard();
      } else A.toast('Con này chưa lớn');
      return true;
    }
    if (k === 'f') {
      if (isHungry(p.data)) feedPet(p);
      else A.toast(nowS() >= p.data.readyAt ? 'Con này đã lớn — bấm H để thu hoạch' : 'Con này đang no');
      return true;
    }
    if (k === 'h' || k === 'e' || k === 'enter' || k === ' ') {
      if (p.fish && nowS() >= p.data.readyAt) {
        closeCard();
        collect(true, p.id, p.el);
      } else if (!p.fish && prodReady(p.data)) producePet(p).then(renderCard);
      else if (!p.fish && nowS() >= p.data.readyAt) {
        card.confirm = true;
        renderCard();
      } else if (isHungry(p.data)) feedPet(p);
      else A.toast(`${find(p.fish ? 'fish' : 'animals', p.kind).name} chưa lớn`);
      return true;
    }
    if (k === 'arrowleft' || k === 'arrowright' || k === 'a' || k === 'd') {
      // Con kế bên (cùng chuồng / ao), sắp theo x.
      const list = [...pets.values()].filter((q) => q.fish === p.fish).sort((a, b) => a.x - b.x || a.id - b.id);
      const i = list.indexOf(p);
      const q = list[(i + (k === 'arrowleft' || k === 'a' ? -1 : 1) + list.length) % list.length];
      if (q && q !== p) {
        q.state = 'look';
        q.until = 8;
        react(q);
        openCard(q);
      }
      return true;
    }
    return false;
  }

  window.addEventListener('keydown', (e) => {
    if (typing(e) || e.ctrlKey || e.metaKey || e.altKey || !farm) return;
    const dlg = document.querySelector('dialog[open]');
    const arrow = { ArrowUp: 'up', ArrowDown: 'down', ArrowLeft: 'left', ArrowRight: 'right' }[e.key];
    if (dlg) {
      // Trong hộp thoại: mũi tên chuyển nút; số chọn nhanh hạt khi đang gieo vào ô.
      if (arrow) {
        e.preventDefault();
        return moveFocus(dlg, arrow);
      }
      if (/^[1-9]$/.test(e.key) && dlg.id === 'dlg-shop') {
        const btn = dlg.querySelectorAll('[data-plant]:not([disabled]), [data-buy-seed]:not([disabled])')[+e.key - 1];
        if (btn) {
          e.preventDefault();
          btn.click();
        }
      }
      return; // Enter / Space bấm nút đang focus, Esc đóng (mặc định của trình duyệt)
    }
    const qm = $('quick-menu');
    if (!qm.classList.contains('hidden')) {
      if (arrow) {
        e.preventDefault();
        return moveFocus(qm, arrow);
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        return qm.classList.add('hidden');
      }
      if ((e.key === 'Enter' || e.key === ' ') && qm.contains(document.activeElement)) return;
    }
    const pick = $('seed-pick');
    if (!pick.classList.contains('hidden')) {
      if (arrow) {
        e.preventDefault();
        return moveFocus(pick, arrow);
      }
      if (/^[1-9]$/.test(e.key)) {
        const btn = pick.querySelectorAll('[data-seed-sel]')[+e.key - 1];
        if (btn) {
          e.preventDefault();
          return btn.click();
        }
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        pickAll = false;
        return pickTool('none');
      }
      if ((e.key === 'Enter' || e.key === ' ') && document.activeElement && pick.contains(document.activeElement)) return;
    }
    if (card && cardKeys(e)) return e.preventDefault();
    if (e.key === 'Escape') {
      if ($('kb-help') && !$('kb-help').classList.contains('hidden')) return $('kb-help').classList.add('hidden');
      if (tool) return pickTool('none');
      return;
    }
    if (e.key === '?' || (e.code === 'Slash' && e.shiftKey)) {
      e.preventDefault();
      return $('kb-help').classList.toggle('hidden');
    }
    if (e.key === 'Tab' || e.key.toLowerCase() === 'q') {
      e.preventDefault();
      return cycleTarget(e.shiftKey ? -1 : 1);
    }
    if (e.shiftKey && !e.repeat && (e.code === 'Digit1' || e.code === 'KeyF' || e.code === 'KeyP')) {
      e.preventDefault();
      return e.code === 'Digit1' ? plantAll() : e.code === 'KeyF' ? feedAll() : produceAll();
    }
    if (e.key.toLowerCase() === 'g' && !e.repeat) {
      e.preventDefault();
      return openPen(farm.yard ? 'yard' : 'barn');
    }
    if (/^[1-7]$/.test(e.key)) {
      e.preventDefault();
      const b = $('toolbar').querySelector(TOOL_KEYS[+e.key - 1]);
      if (b) b.click();
      return;
    }
    if (e.key.toLowerCase() === 'h' && !e.repeat) {
      e.preventDefault();
      const b = $('btn-harvest-all');
      if (!b.classList.contains('hidden')) b.click();
      else A.toast('Chưa có ô nào chín');
      return;
    }
    if ((e.key === '[' || e.key === ']') && tool === 'plant') {
      e.preventDefault();
      return cycleSeed(e.key === ']' ? 1 : -1);
    }
    const dir = dirKey(e);
    if (dir) {
      e.preventDefault();
      keys.add(dir);
      me.path = [];
      me.pending = null;
      $('go-mark').classList.add('hidden');
    } else if (isAct(e)) {
      e.preventDefault();
      if (!e.repeat) actTarget();
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
    if (moving && shiftHeld && (tool === 'plant' || tool === 'harvest')) shiftSweep();
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
  // Mục tiêu tương tác bằng phím: các đối tượng gần (≤ 60px tới chỗ đứng) và con vật / cá gần (≤ 170px).
  // Tab / Q chuyển mục tiêu khi có nhiều; mục tiêu đang chọn có viền sáng + gợi ý phím.
  let targets = [];
  let targetIdx = 0;
  let targetSig = '';
  const targetId = (t) => (t ? (t.pet ? `pet${t.pet}` : t.key) : '');

  function updateHint() {
    const list = [];
    const fv = { up: [0, -1], down: [0, 1], left: [-1, 0], right: [1, 0] }[me.dir] || [0, 0];
    const fx = me.x + fv[0] * 16;
    const fy = me.y + fv[1] * 16;
    for (const [k, o] of Object.entries(objs)) {
      const d = Math.hypot(me.x - o.x, me.y - o.y);
      // Ô đất: tính khoảng cách tới điểm trước mặt để E thao tác ô đang đứng / ô đối diện.
      if (d < 60) list.push({ key: k, d: /^plot|^buyplot/.test(k) ? Math.hypot(fx - o.x, fy - o.y) : d });
    }
    for (const p of pets.values()) {
      const d = Math.hypot(me.x - p.x, me.y - p.y);
      if (d < (p.free || me.x < PEN.x2 ? 90 : 170)) list.push({ pet: p.id, d: d + 10 });
    }
    list.sort((a, b) => a.d - b.d);
    const sig = list.map(targetId).join(',');
    if (sig !== targetSig) {
      // Danh sách / thứ tự gần xa đổi (vừa đi) → chọn lại đối tượng gần nhất; Tab chỉ giữ khi đứng yên.
      targetSig = sig;
      targets = list;
      targetIdx = 0;
    }
    showTarget();
  }

  function cycleTarget(step) {
    if (targets.length < 2) return;
    targetIdx = (targetIdx + step + targets.length) % targets.length;
    showTarget();
  }

  function showTarget() {
    const t = targets[targetIdx];
    nearKey = t && t.key ? t.key : null;
    document.querySelectorAll('.kb-target').forEach((el) => el.classList.remove('kb-target'));
    const hint = $('hint');
    hint.classList.toggle('hidden', !t);
    if (!t) return;
    let x;
    let y;
    let verb;
    if (t.pet) {
      const p = pets.get(t.pet);
      if (!p) return hint.classList.add('hidden');
      p.el.classList.add('kb-target');
      x = p.x;
      y = p.y - sheetOf(p).fh * scaleOf(p) - 18;
      verb = p.decor ? 'Chó giữ nhà' : `${find(p.fish ? 'fish' : 'animals', p.kind).name}`;
    } else {
      const o = objs[t.key];
      x = o.hx;
      y = o.hy;
      const pm = /^plot(\d+)$/.exec(t.key);
      verb = pm && farm.plots[+pm[1]] ? plotVerb(farm.plots[+pm[1]]) : o.verb;
      document.querySelectorAll(`[data-obj="${t.key}"]`).forEach((el) => el.classList.add('kb-target'));
    }
    hint.style.left = `${Math.round(x)}px`;
    hint.style.top = `${Math.round(y)}px`;
    hint.querySelector('span').textContent = targets.length > 1 ? `${verb}  ·  Tab đổi (${targetIdx + 1}/${targets.length})` : verb;
  }

  function actTarget() {
    const t = targets[targetIdx];
    if (!t) return;
    if (t.pet) return clickPet(t.pet);
    goTo(t.key);
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
    const fitW = innerWidth < 600 ? Infinity : ((sc.clientWidth || innerWidth) - 2) / CORE_W;
    zoom = Math.max(0.5, Math.min(sh / H, fitW, innerWidth < 600 ? 1 : 1.35));
    if (farm) renderWorld();
  }

  // Kéo chuột để xem ngang tự do (điện thoại dùng vuốt cuộn sẵn có); lăn chuột dọc cũng cuộn ngang.
  let drag = null;
  let dragged = false;
  // Đang cầm công cụ gieo / thu hoạch: nhấn giữ chuột trên ô rồi kéo qua nhiều ô → nhân vật làm lần lượt từng ô.
  let sweep = null;
  const plotIdx = (el) => {
    const o = el && el.closest('[data-obj^="plot"]');
    const m = o && /^plot(\d+)$/.exec(o.dataset.obj);
    return m ? +m[1] : null;
  };
  function runSweep(list) {
    const i = list.shift();
    if (i === undefined) return;
    goTo(`plot${i}`, async () => {
      await plotAction(i, tool === 'harvest' ? 'harvest' : 'plant');
      runSweep(list);
    });
  }
  $('scene').addEventListener('pointerdown', (e) => {
    if (e.pointerType !== 'mouse' || e.button !== 0) return;
    const i = (tool === 'plant' || tool === 'harvest') && plotIdx(e.target);
    if (i !== null && i !== false) {
      sweep = [i];
      return;
    }
    drag = { x: e.clientX, left: $('scene').scrollLeft };
    dragged = false;
  });
  addEventListener('pointermove', (e) => {
    if (!sweep) return;
    const i = plotIdx(document.elementFromPoint(e.clientX, e.clientY));
    if (i !== null && !sweep.includes(i)) {
      sweep.push(i);
      const el = document.querySelector(`.plot[data-obj="plot${i}"]`);
      if (el) el.classList.add('kb-target');
    }
  });
  addEventListener('pointerup', () => {
    if (!sweep) return;
    const list = sweep;
    sweep = null;
    if (list.length > 1) {
      dragged = true; // bỏ qua cú click ngay sau khi kéo
      runSweep(list);
    }
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
    if (e.target.closest('[data-hint]')) return actTarget();
    const r = $('world-wrap').getBoundingClientRect();
    const wx = (e.clientX - r.left) / zoom;
    const wy = (e.clientY - r.top) / zoom - OY;
    // Thẻ con vật: cho ăn / thu hoạch / đóng.
    const act = e.target.closest('[data-card]');
    if (act) {
      const p = card && pets.get(card.id);
      if (act.dataset.card === 'close' || !p) return closeCard();
      const c = act.dataset.card;
      if (c === 'feed') return feedPet(p);
      if (c === 'produce') return producePet(p).then(renderCard);
      if (c === 'meat') return ((card.confirm = true), renderCard());
      if (c === 'meat-no') return ((card.confirm = false), renderCard());
      if (c === 'meat-yes') return meatPet(p);
      if (c === 'collect') {
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
    if (!(await ask(`Mở thêm ô đất với giá ${xu(farm.nextPlotPrice)} xu?`, { title: 'Mở ô đất', ok: 'Mở ô' }))) return;
    await call('/api/farm/plots/buy', {});
  }

  async function collect(isFish, id, el) {
    const items = (isFish ? farm.pond && farm.pond.fish : [...((farm.barn && farm.barn.animals) || []), ...((farm.yard && farm.yard.animals) || [])]) || [];
    const it = items.find((x) => x.id === id);
    if (!it) return;
    const info = find(isFish ? 'fish' : 'animals', it.kind);
    if (nowS() < it.readyAt) return A.toast(`${info.name} còn ${fmtDur(it.readyAt - nowS())} nữa mới lớn`);
    const anchor = el && el.isConnected ? el : document.querySelector(`[data-obj="${isFish ? 'pond' : 'barn'}"]`);
    const pp = pets.get(id);
    const res = await call(`/api/farm/${pp ? apiOf(pp) : isFish ? 'fish' : 'animals'}/collect`, { id });
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

  // Hỏi xác nhận bằng modal kiểu game (không dùng confirm()): dùng A.confirm chung nếu có, không thì hộp riêng.
  function ask(message, opts = {}) {
    if (typeof A.confirm === 'function') return A.confirm(message, opts);
    return new Promise((resolve) => {
      const dlg = $('dlg-ask');
      $('ask-title').textContent = opts.title || 'Xác nhận';
      $('ask-msg').textContent = message;
      const ok = $('ask-ok');
      ok.innerHTML = `${opts.ok || 'Đồng ý'} <kbd>Enter</kbd>`;
      ok.classList.toggle('danger', !!opts.danger);
      $('ask-cancel').innerHTML = `${opts.cancel || 'Huỷ'} <kbd>Esc</kbd>`;
      let done = false;
      const finish = (v) => {
        if (done) return;
        done = true;
        if (dlg.open) dlg.close();
        resolve(v);
      };
      ok.onclick = () => finish(true);
      $('ask-cancel').onclick = () => finish(false);
      dlg.onclose = () => finish(false);
      dlg.showModal();
      ok.focus();
    });
  }

  function show(id) {
    keys.clear();
    if (!$(id).open) {
      $(id).showModal();
      focusFirst($(id));
    }
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
    const isYard = kind === 'yard';
    const isBarn = kind === 'barn' || isYard;
    const pen = isYard ? farm.yard : kind === 'barn' ? farm.barn : farm.pond;
    const cfg = (cat && cat[kind]) || { price: 0, level: 1 };
    const list = isBarn ? 'animals' : 'fish';
    $('pen-title').textContent = isYard ? 'Đàn gia cầm (thả rông)' : isBarn ? 'Chuồng gia súc' : 'Ao cá';
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
          const draw = isBarn ? art.animal(it.kind === 'duck' ? 'chicken_white' : it.kind, 30) : art.fish(it.kind, 18);
          const hungry = !ready && isHungry(it);
          const yd = yieldOf(it.kind);
          const action = ready && isBarn
            ? `${yd.product && prodReady(it) ? `<button class="primary" data-produce="${it.id}">${art.item(yd.product.id, 16)}${PRODUCE_VERB[yd.product.id] || 'Lấy'}</button>` : ''}<button data-collect-animal="${it.id}" title="Lấy thịt (con vật mất)">Lấy thịt</button>`
            : ready
            ? `<button class="primary" data-collect-fish="${it.id}">${I.svg('sprout')}Thu hoạch</button>`
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
      <p class="muted">${isYard ? 'Gà, vịt đi lại khắp trang trại; đã lớn thì về ổ rơm cạnh kho đẻ trứng — nhặt ở ổ (Shift+P lấy hết sản phẩm).' : 'Con đã lớn: lấy trứng / sữa / lông hoặc lấy thịt, cất vào kho thóc rồi bán.'}</p>
      <ul class="critter-list">${rows}</ul>
      <button class="primary wide" data-open-shop="${isYard ? 'poultry' : list}">${I.svg('plus')}Mua ${isYard ? 'gà, vịt' : isBarn ? 'con giống' : 'cá giống'}</button>`;
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
    } else if (tab === 'poultry') {
      // Gia cầm thả rông: sân gia cầm (state.yard, có sẵn) — backend cũ thì nuôi chung chuồng.
      const yard = farm.yard;
      const holder = yard || farm.barn;
      const cfg = (cat && cat.yard) || {};
      const list = (cat && cat.poultry) || ((cat && cat.animals) || []).filter((a) => LAYERS.has(a.id));
      $('shop-note').textContent = holder
        ? `Gà, vịt đi lại khắp trang trại, đẻ trứng ở ổ rơm cạnh kho · ${yard ? 'sân' : 'chuồng'} ${holder.used}/${holder.capacity} đơn vị.`
        : 'Cần mua chuồng trước.';
      if (yard && yard.nextExpandPrice) {
        rows.push(`<div class="shop-row feature"><span class="thumb">${art.img('nest', { h: 26 })}</span>
          <div><b>Mở rộng sân gia cầm +${cfg.expandUnits || 5}</b>${metaLine([['users', `${yard.capacity} → ${yard.capacity + (cfg.expandUnits || 5)} đơn vị`], ['layers', `tối đa ${yard.maxCapacity}`]])}</div>
          <button class="primary" data-expand="yard">${I.svg('coins')}${xu(yard.nextExpandPrice)}</button></div>`);
      }
      for (const a of list) {
        const locked = user.level < a.level;
        const full = holder && holder.used + (a.units || 1) > holder.capacity;
        let btn;
        if (locked) btn = `<button disabled>${I.svg('lock')}Cần Lv ${a.level}</button>`;
        else if (!holder) btn = '<button disabled>Cần chuồng</button>';
        else if (full) btn = '<button disabled>Hết chỗ</button>';
        else btn = `<button class="primary" ${yard ? 'data-buy-poultry' : 'data-buy-animal'}="${a.id}" ${user.balance < a.price ? 'disabled' : ''}>${I.svg('coins')}${xu(a.price)}</button>`;
        rows.push(`<div class="shop-row${locked ? ' locked' : ''}"><span class="thumb">${art.animal(a.id === 'duck' ? 'chicken_white' : a.id, 40)}</span>
          <div><b>${esc(a.name)}</b>${metaLine([['timer', fmtDur(a.growSeconds)], a.product && ['sparkles', `${esc(a.product.name)} mỗi ${fmtDur(a.product.every)}`], ['trophy', `+${a.xp} XP`], ['sparkles', `Lv ${a.level}`]])}</div>${btn}</div>`);
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
      for (const a of cat[isBarn ? 'animals' : 'fish'].filter((x) => !isBarn || !farm.yard || !LAYERS.has(x.id))) {
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
    if (d.quickMenu !== undefined) return toggleQuick();
    if (d.quick) {
      $('quick-menu').classList.add('hidden');
      if (d.quick === 'plant-all') return plantAll();
      if (d.quick === 'harvest-all') return $('btn-harvest-all').click();
      if (d.quick === 'feed-all') return feedAll();
      if (d.quick === 'produce-all') return produceAll();
      if (d.quick === 'yard') return openPen(farm.yard ? 'yard' : 'barn');
    }
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
      if (!(await ask(`Mở rộng ${d.storeExpand === 'seeds' ? 'kho giống' : 'kho thóc'} thêm ${units} chỗ với giá ${xu(s.nextExpandPrice)} xu?`, { title: 'Mở rộng kho', ok: 'Mở rộng' }))) return;
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
      if (!(await ask(`Mua ${d.buyPen === 'barn' ? 'chuồng gia súc' : 'ao cá'} với giá ${xu(cfg.price)} xu?`, { title: d.buyPen === 'barn' ? 'Mua chuồng' : 'Mua ao', ok: 'Mua' }))) return;
      return call(`/api/farm/${d.buyPen}/buy`, {});
    }
    if (d.expand) {
      const pen = farm[d.expand];
      const units = (cat[d.expand] && cat[d.expand].expandUnits) || 5;
      if (!(await ask(`Mở rộng ${d.expand === 'yard' ? 'sân gia cầm' : d.expand === 'barn' ? 'chuồng' : 'ao'} thêm ${units} đơn vị với giá ${xu(pen.nextExpandPrice)} xu?`, { title: 'Mở rộng', ok: 'Mở rộng' }))) return;
      return call(`/api/farm/${d.expand}/expand`, {});
    }
    if (d.buyAnimal) return call('/api/farm/animals/buy', { kind: d.buyAnimal });
    if (d.buyPoultry) return call('/api/farm/poultry/buy', { kind: d.buyPoultry });
    if (d.buyFish) return call('/api/farm/fish/buy', { kind: d.buyFish });
    if (d.feedAnimal || d.feedFish) {
      const p = pets.get(+(d.feedFish || d.feedAnimal));
      if (p) return feedPet(p);
      return call(`/api/farm/${d.feedFish ? 'fish' : 'animals'}/feed`, { id: +(d.feedFish || d.feedAnimal) });
    }
    if (d.produce) {
      const p = pets.get(+d.produce);
      return p ? producePet(p) : call('/api/farm/animals/produce', { id: +d.produce });
    }
    if (d.collectAnimal && !(await ask('Lấy thịt con này? Con vật sẽ không còn nữa.', { title: 'Lấy thịt', ok: 'Lấy thịt', danger: true }))) return;
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
      for (const f of manifest.images || manifest.files || []) {
        const name = f.file.replace(/\.png$/, '');
        sizes[name] = [f.w, f.h];
        if (f.frameW && f.frameH) sheets[name] = { fw: f.frameW, fh: f.frameH, n: f.frameCount || (f.frames && f.frames.length) || Math.round(f.w / f.frameW) * Math.max(1, Math.round(f.h / f.frameH)), frames: f.frames, animations: f.animations, cols: f.framesPerRow || f.columns || f.cols || Math.round(f.w / f.frameW), face: f.face };
      }
      cat = await A.api('/api/farm/catalog').catch((e) => (A.toast(e.message), null));
      if (await load()) {
        setZoom();
        // Vừa bước qua cổng nông trại.
        // Vừa bước qua cổng "Vào".
        me.x = 690 + OX;
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
    pets: () => [...pets.values()].map((p) => ({ id: p.id, kind: p.kind, free: !!p.free, x: Math.round(p.x), y: Math.round(p.y), grown: nowS() >= p.data.readyAt, product: prodReady(p.data), state: p.state })),
    // Đặt con vật đứng yên tại (x, y) — cho kiểm thử sắp lớp.
    placePet: (id, x, y) => {
      const p = pets.get(id);
      if (!p) return;
      p.x = p.tx = x;
      p.y = p.ty = y;
      p.state = 'look';
      p.until = 30;
    },
    // Toạ độ thế giới → toạ độ màn hình.
    screen: (x, y) => {
      const r = $('world-wrap').getBoundingClientRect();
      return { x: r.left + x * zoom, y: r.top + (y + OY) * zoom };
    },
  };
})();
