// Nông trại DCity: ruộng ô đất, kho giống, kho sản phẩm, chuồng gia súc, ao cá trong cùng một cảnh; nhân vật đi lại
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

  function renderHUD() {
    if (!user) return;
    $('balance').textContent = xu(user.balance);
    $('user-name').textContent = user.name;
    $('user-avatar').innerHTML = C.head({ gender: user.gender, outfit: user.outfit });
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
    if (me.look.gender !== user.gender || me.look.outfit !== user.outfit) {
      me.look = { gender: user.gender, outfit: user.outfit };
      me.drawn = '';
      drawMe();
    }
  }

  // ---------- Bố cục cảnh ----------
  // Toạ độ "thế giới" (px): cao cố định H, rộng theo số ô đất. Từ trái sang: cổng nông trại, kho sản phẩm, kho giống,
  // ruộng (4 hàng, thêm cột khi mở ô), chuồng, ao. Mọi vật đặt tuyệt đối, z-index = y chân để nhân vật đứng trước /
  // sau đúng chiều sâu.

  const H = 660;
  const SKY = 170; // dải trời + hàng rào phía trên
  const ROAD = 586; // đường đất chạy dọc chân cảnh
  const FY = 198; // mép trên ruộng
  const PLOT = 88;
  const PITCH = 96;
  const ROWS = 4;
  const FX = 760; // mép trái ruộng
  let W = 2400;
  let L = { cols: 6, FE: FX + 6 * PITCH, YX: 0, PX: 0 };
  let objs = {}; // key → { x, y (chỗ đứng), face, hx, hy (gợi ý), verb, run() }
  let obstacles = []; // [x1, y1, x2, y2] chân nhân vật không được vào

  function tiles() {
    const owned = farm.plots.length;
    const max = farm.maxPlots || 48;
    return { owned, max, buy: owned < max && !!farm.nextPlotPrice, left: max - owned - 1 };
  }

  function computeLayout() {
    const t = tiles();
    const n = t.owned + (t.buy ? 1 : 0) + (t.left > 0 ? 1 : 0);
    const cols = Math.max(6, Math.ceil(n / ROWS));
    const FE = FX + cols * PITCH; // mép phải ruộng
    const YX = FE + 90; // chuồng
    const PX = YX + 600; // ao
    W = PX + 540 + 260;
    L = { cols, rowsUsed: Math.ceil(n / cols), FE, YX, PX };
    obstacles = [
      [18, 636, 50, 652], // cột cổng trái
      [212, 636, 244, 652], // cột cổng phải
      [302, 404, 498, 470], // kho sản phẩm (nhà)
      [556, 420, 644, 470], // kho giống (silo)
      [YX, 196, YX + 560, 566], // chuồng (hàng rào)
      [PX + 20, 250, PX + 520, 560], // ao
      [W - 150, 462, W - 110, 478], // gốc cây cuối
      [W - 260, 374, W - 208, 392], // giếng
    ];
  }

  const pos = (x, y, z = y) => `left:${Math.round(x)}px;top:${Math.round(y)}px;z-index:${Math.round(z)}`;
  const capBar = (s) => `<span class="cap-mini"><span style="width:${Math.round((s.used / Math.max(1, s.capacity)) * 100)}%"></span></span>`;

  // Mỗi ô: nền đất (hoặc cỏ khi chưa mở), cây đứng trên ô, nhãn nhỏ phía dưới.
  function plotHTML(p, x, y) {
    const at = `style="${pos(x, y, y + PLOT)}"`;
    if (!p.crop) {
      return `<div class="plot empty" data-obj="plot${p.index}" ${at} title="Gieo hạt">
        <span class="soil"></span><span class="tag-pill add">${I.svg('plus')}Gieo hạt</span></div>`;
    }
    const c = find('crops', p.crop);
    const pr = progress(p.plantedAt, p.readyAt);
    if (pr >= 1) {
      return `<div class="plot ripe" data-obj="plot${p.index}" ${at} title="Thu hoạch ${esc(c.name)}">
        <span class="soil"></span><span class="crop">${art.crop(p.crop, 'ripe')}</span>
        <span class="harvest-arrow">${I.svg('chevronDown')}</span>
        <span class="twinkle t1">${I.svg('sparkles')}</span><span class="twinkle t2">${I.svg('sparkles')}</span>
        <span class="tag-pill go">Thu hoạch</span></div>`;
    }
    const stage = pr < 0.34 ? 1 : pr < 0.67 ? 2 : 3;
    return `<div class="plot growing" data-obj="plot${p.index}" ${at} data-start="${p.plantedAt}" data-ready="${p.readyAt}" data-stage="${stage}"
        title="${esc(c.name)}">
      <span class="soil wet"></span><span class="crop">${art.crop(p.crop, stage)}</span>
      <span class="tag-pill"><span data-left>${countdown(p.readyAt - nowS())}</span>
        <span class="bar"><span style="width:${Math.round(pr * 100)}%"></span></span></span></div>`;
  }

  function fieldHTML() {
    const t = tiles();
    const out = [];
    const cell = (i) => [FX + (i % L.cols) * PITCH, FY + Math.floor(i / L.cols) * PITCH];
    farm.plots.forEach((p, i) => {
      const [x, y] = cell(i);
      out.push(plotHTML(p, x, y));
      objs[`plot${p.index}`] = { x: x - 2, y: y + PLOT + 2, face: 'right', hx: x + PLOT / 2, hy: y - 6, verb: plotVerb(p), run: () => plotAction(p.index) };
    });
    let i = farm.plots.length;
    if (t.buy) {
      const [x, y] = cell(i++);
      out.push(`<div class="plot buy" data-obj="buyplot" style="${pos(x, y, y + PLOT)}" title="Mở thêm ô đất">
        <span class="soil grass"></span><span class="lock">${I.svg('lock')}</span>
        <span class="tag-pill price">${I.svg('coins')}${xu(farm.nextPlotPrice)}</span></div>`);
      objs.buyplot = { x: x - 2, y: y + PLOT + 2, face: 'right', hx: x + PLOT / 2, hy: y - 6, verb: 'Mở ô đất', run: buyPlot };
    }
    if (t.left > 0) {
      const [x, y] = cell(i);
      out.push(`<div class="plot locked" style="${pos(x, y, y + PLOT)}" title="Mở lần lượt từng ô">
        <span class="soil grass"></span><span class="lock">${I.svg('lock')}</span>
        <span class="tag-pill">Còn ${t.left} ô</span></div>`);
    }
    out.push(`<span class="plot-count" style="${pos(FX, FY - 30, 2000)}">${t.owned}/${t.max} ô đất</span>`);
    // Nền ruộng: một mảnh cỏ đậm bao quanh lưới ô.
    out.push(`<span class="field-bed" style="left:${FX - 12}px;top:${FY - 12}px;width:${L.cols * PITCH + 16}px;height:${L.rowsUsed * PITCH + 18}px;z-index:${FY - 20}"></span>`);
    return out.join('');
  }

  function plotVerb(p) {
    if (!p.crop) return tool === 'plant' && seedSel ? `Gieo ${find('crops', seedSel).name}` : 'Gieo hạt';
    return nowS() >= p.readyAt ? 'Thu hoạch' : 'Xem cây';
  }

  // Kho: ảnh công trình, nhãn tên + thanh sức chứa phía trên (giống bảng tên trong game).
  function storeObjHTML(kind, x, footY, image, h) {
    const s = store(kind);
    const name = kind === 'seeds' ? 'Kho giống' : 'Kho sản phẩm';
    const full = s.capacity && s.used >= s.capacity;
    return `<div class="obj building${full ? ' full' : ''}" data-obj="${kind}" style="${pos(x, footY)}" title="${name}">
      <span class="obj-label"><b>${name}</b>${capBar(s)}<em>${s.used}/${s.capacity}</em></span>
      ${art.img(image, { h })}</div>`;
  }

  // Vị trí ngẫu nhiên nhưng cố định theo id, để con vật không nhảy chỗ mỗi lần vẽ lại.
  function spot(id) {
    const r = (n) => (Math.sin(id * 9301 + n * 49297) + 1) / 2;
    return { x: 10 + r(1) * 76, y: 30 + r(2) * 70, dx: (r(3) - 0.5) * 120, dy: (r(4) - 0.5) * 16, dur: 6 + r(5) * 8, delay: -r(6) * 8 };
  }

  function critterHTML(item, list, swim) {
    const info = find(list, item.kind);
    const pr = progress(item.boughtAt, item.readyAt);
    const s = spot(item.id);
    const cls = `critter${swim ? ' fish-swim' : ''}${pr < 1 ? ' young' : ''}${s.dx < 0 ? ' flip' : ''}`;
    // Con nhỏ vẽ nhỏ hơn; bò / cừu vốn to nên giới hạn theo chiều cao.
    const h = swim ? (pr < 1 ? 20 : 28) : { cow: 70, sheep: 58, pig: 50, chicken: 42 }[item.kind] * (pr < 1 ? 0.7 : 1);
    const draw = swim ? art.fish(item.kind, h) : art.animal(item.kind, Math.round(h || 50));
    return `<button class="${cls}" data-${swim ? 'fish' : 'animal'}="${item.id}" title="${esc(info.name)}${pr >= 1 ? ' — đã lớn, bấm để thu hoạch' : ''}"
        style="left:${s.x}%;top:${s.y}%;--dx:${s.dx}px;--dy:${s.dy}px;--dur:${s.dur}s;--delay:${s.delay}s">
      ${draw}${pr >= 1 ? '<span class="ready">!</span>' : `<span class="mini-bar"><span style="width:${Math.round(pr * 100)}%"></span></span>`}
    </button>`;
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
    const items = (pen && pen.animals) || [];
    return `<div class="obj area yard${pen ? '' : ' ghost'}" data-obj="barn" style="left:${L.YX}px;top:196px;width:560px;height:370px;z-index:566">
      ${penLabel('barn', pen)}
      <span class="deco" style="left:14px;top:-36px">${art.img('barn', { h: 170 })}</span>
      <span class="deco" style="left:200px;top:40px">${art.img('hay', { h: 52 })}</span>
      <span class="deco" style="left:44%;bottom:12px">${art.img('trough_feed', { h: 34 })}</span>
      <span class="deco" style="left:18%;bottom:12px">${art.img('trough_water', { h: 34 })}</span>
      <span class="fence-top"></span><span class="fence-bottom"></span>
      <div class="roam">${items.map((it) => critterHTML(it, 'animals', false)).join('')}</div>
    </div>`;
  }

  function pondHTML() {
    const pen = farm.pond;
    const items = (pen && pen.fish) || [];
    return `<div class="obj area pond-area${pen ? '' : ' ghost'}" data-obj="pond" style="left:${L.PX}px;top:196px;width:540px;height:370px;z-index:566">
      ${penLabel('pond', pen)}
      <span class="deco" style="left:4px;bottom:10px">${art.img('flowers', { h: 52 })}</span>
      <span class="deco" style="right:6px;bottom:8px">${art.img('bush_3', { h: 44 })}</span>
      <div class="pond-water">${art.img('pond', { cls: 'pond-img' })}
        <div class="swim">${items.map((it) => critterHTML(it, 'fish', true)).join('')}</div></div>
    </div>`;
  }

  // Đồ trang trí đặt theo chân (x, y).
  const deco = (name, x, y, h, flip = false) =>
    `<span class="obj decor${flip ? ' flip' : ''}" style="${pos(x, y)}">${art.img(name, { h })}</span>`;

  function renderWorld() {
    computeLayout();
    objs = {};
    const world = $('world');
    world.style.width = `${W}px`;
    world.style.height = `${H}px`;
    world.style.transform = zoom === 1 ? '' : `scale(${zoom})`;
    const wrap = $('world-wrap');
    wrap.style.width = `${W * zoom}px`;
    wrap.style.height = `${H * zoom}px`;

    const parts = [];
    // Trời, mây, hàng rào, cây xa ở đường chân trời.
    parts.push(`<div class="sky" style="width:${W}px;height:${SKY + 10}px">
      ${[0.04, 0.3, 0.55, 0.8].map((k, i) => `<span class="cloud c${(i % 3) + 1}" style="left:${Math.round(W * k)}px"></span>`).join('')}
      <span class="fence-row"></span></div>`);
    for (let x = 300; x < W - 200; x += 520) {
      parts.push(`<span class="obj decor far" style="${pos(x + ((x / 520) % 2 ? 140 : 0), SKY + 16, 2)}">${art.img('tree_big', { h: 190 })}</span>`);
    }
    // Đường đất: sân trước hai kho và đường dọc chân cảnh qua cổng.
    parts.push(`<span class="dirt" style="left:280px;top:440px;width:420px;height:${ROAD - 430}px"></span>`);
    parts.push(`<span class="dirt road" style="left:0;top:${ROAD}px;width:${W}px;height:${H - ROAD}px"></span>`);

    // Cổng nông trại + biển tên.
    parts.push(`<div class="obj gate" data-obj="gate" style="${pos(130, 652)}" title="Về thành phố">
      ${art.img('farm_gate', { h: 230 })}<span class="gate-sign">Nông trại<b>${esc(user.name)}</b></span></div>`);
    objs.gate = { x: 130, y: 616, face: 'left', hx: 130, hy: 440, verb: 'Về thành phố', run: () => (location.href = '/') };

    parts.push(storeObjHTML('products', 400, 470, 'farmhouse', 210));
    parts.push(storeObjHTML('seeds', 600, 470, 'silo', 196));
    objs.products = { x: 400, y: 492, face: 'up', hx: 400, hy: 250, verb: 'Mở kho', run: () => openStore('products') };
    objs.seeds = { x: 600, y: 492, face: 'up', hx: 600, hy: 262, verb: 'Mở kho', run: () => openStore('seeds') };

    parts.push(fieldHTML());

    parts.push(yardHTML(), pondHTML());
    objs.barn = { x: L.YX + 280, y: 592, face: 'up', hx: L.YX + 280, hy: 214, verb: farm.barn ? 'Xem chuồng' : 'Mua chuồng', run: () => openPen('barn') };
    objs.pond = { x: L.PX + 270, y: 592, face: 'up', hx: L.PX + 270, hy: 214, verb: farm.pond ? 'Xem ao' : 'Mua ao', run: () => openPen('pond') };

    parts.push(deco('wheelbarrow', 700, 560, 64, true));
    parts.push(deco('flowers', 300, 560, 48));
    parts.push(deco('hay', L.FE + 46, 560, 54));
    parts.push(deco('tractor', L.PX - 30, 250, 90));
    parts.push(deco('well', W - 234, 392, 110));
    parts.push(deco('tree_big', W - 130, 478, 250));
    parts.push(deco('bush_2', W - 40, 560, 44));
    parts.push(deco('fence_piece', 24, 560, 50));
    parts.push(`<span class="fence-bottom-row" style="left:0;top:${H - 34}px;width:${W}px;z-index:${H}"></span>`);

    $('objects').innerHTML = parts.join('');
    // Nhân vật có thể đang đứng chỗ mới bị chặn (ruộng rộng ra đẩy chuồng sang phải): đưa về chỗ trống gần nhất.
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
      btn('data-go="seeds"', art.img('silo', { h: 42 }), 'Kho giống', badge(seeds.used)),
      btn('data-go="products"', art.img('farmhouse', { h: 38 }), 'Kho sản phẩm', badge(prods.used)),
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
    if (changed) render();
  }

  // ---------- Nhân vật đi lại ----------

  const SPEED = 230; // px/giây
  const SPRITE = 100;
  const me = { x: 320, y: 616, dir: 'down', walking: false, look: {}, path: [], pending: null, drawn: '' };
  const keys = new Set();
  let zoom = 1;
  let nearKey = null;

  const YMIN = SKY + 14;
  const blocked = (x, y) =>
    x < 18 || x > W - 18 || y < YMIN || y > H - 30 ||
    obstacles.some((r) => x > r[0] - 12 && x < r[2] + 12 && y > r[1] - 4 && y < r[3] + 6);

  function drawMe() {
    const el = $('me');
    el.style.left = `${Math.round(me.x)}px`;
    el.style.top = `${Math.round(me.y)}px`;
    el.style.zIndex = Math.round(me.y);
    const key = `${me.dir}|${me.walking}|${me.look.gender}|${me.look.outfit}`;
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
    return [260, 616];
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

  document.addEventListener('keydown', (e) => {
    if (typing(e) || dialogOpen() || e.ctrlKey || e.metaKey || e.altKey || !farm) return;
    if (KEYMAP[e.code]) {
      e.preventDefault();
      keys.add(KEYMAP[e.code]);
      me.path = [];
      me.pending = null;
      $('go-mark').classList.add('hidden');
    } else if ((e.code === 'KeyE' || e.code === 'Enter' || e.code === 'Space') && nearKey) {
      e.preventDefault();
      if (!e.repeat) goTo(nearKey);
    }
  });
  document.addEventListener('keyup', (e) => KEYMAP[e.code] && keys.delete(KEYMAP[e.code]));
  document.addEventListener('keydown', (e) => {
    if (e.code === 'Escape' && !dialogOpen() && (tool || !$('seed-pick').classList.contains('hidden'))) pickTool('none');
  });
  window.addEventListener('blur', () => keys.clear());

  let last = 0;
  function frame(ts) {
    const dt = Math.min(0.05, (ts - (last || ts)) / 1000);
    last = ts;
    if (farm) step(dt);
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
    const sh = ($('scene').clientHeight || innerHeight - 60) - (innerWidth < 600 ? 80 : 92);
    zoom = Math.max(0.55, Math.min(sh / H, innerWidth < 600 ? 1 : 1.35));
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
    const critter = e.target.closest('[data-animal], [data-fish]');
    if (critter) {
      const isFish = !!critter.dataset.fish;
      const id = +(critter.dataset.fish || critter.dataset.animal);
      return goTo(isFish ? 'pond' : 'barn', () => collect(isFish, id, critter));
    }
    const obj = e.target.closest('[data-obj]');
    if (obj) return goTo(obj.dataset.obj);
    const r = $('world-wrap').getBoundingClientRect();
    walkTo((e.clientX - r.left) / zoom, (e.clientY - r.top) / zoom);
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
    $('store-title').textContent = kind === 'seeds' ? 'Kho giống' : 'Kho sản phẩm';
    const head = `<div class="store-hero">${kind === 'seeds' ? art.img('silo', { h: 74 }) : art.img('farmhouse', { h: 74 })}
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
          const action = ready ? `<button class="primary" data-collect-${isBarn ? 'animal' : 'fish'}="${it.id}">${I.svg('sprout')}Thu hoạch</button>` : '';
          return `<li>${draw}<span class="grow"><span>${esc(info.name)}</span>
            <span class="t" data-left-at="${it.readyAt}">${ready ? 'Đã lớn' : countdown(it.readyAt - nowS())}</span></span>${action}</li>`;
        })
        .join('')
      : `<li class="empty">Chưa có ${isBarn ? 'vật nuôi' : 'cá'} — mua ở cửa hàng</li>`;
    $('pen-body').innerHTML = `
      <div class="cap"><span>${pen.used}/${pen.capacity} đơn vị</span><span class="cap-bar"><span style="width:${pct}%"></span></span>${expand}</div>
      <p class="muted">Con đã lớn bấm "Thu hoạch" để cất vào kho sản phẩm (+XP), rồi bán ở kho.</p>
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
      if (!confirm(`Mở rộng ${d.storeExpand === 'seeds' ? 'kho giống' : 'kho sản phẩm'} thêm ${units} chỗ với giá ${xu(s.nextExpandPrice)} xu?`)) return;
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
      me.look = { gender: u.gender, outfit: u.outfit };
      renderHUD();
      const manifest = await fetch(IMG + 'manifest.json').then((r) => r.json()).catch(() => ({}));
      for (const f of manifest.images || manifest.files || []) sizes[f.file.replace(/\.png$/, '')] = [f.w, f.h];
      cat = await A.api('/api/farm/catalog').catch((e) => (A.toast(e.message), null));
      if (await load()) {
        setZoom();
        // Vừa bước qua cổng nông trại.
        me.x = 320;
        me.y = 616;
        me.dir = 'right';
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
  window.FarmScene = { position: () => ({ x: Math.round(me.x), y: Math.round(me.y), dir: me.dir }), objects: () => objs, goTo, walkTo };
})();
