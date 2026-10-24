// Bản đồ DCity phong cách hoạt hình (giống game Avatar): ảnh trong /img/toon/city/ và /img/toon/farm/ (CC0, xem docs/ART.md).
// Mặt đất (cỏ, đường, vỉa hè, bờ sông, cầu, ruộng) vẽ một lần lên <canvas>; trời + đồi xa ở dải phía trên; toà nhà, cây, đèn,
// gia súc, xe, thuyền, biển tên khu và nhân vật là ảnh HTML xếp lớp theo chiều sâu (y) để nhân vật đi trước / sau đúng.
// Không vẽ hình minh hoạ bằng SVG.
//
// Điều khiển: WASD / phím mũi tên đi tự do (va chạm theo lưới đi được), bấm chuột / chạm để đi tới điểm đó (A*),
// bấm toà nhà / biển tên để đi tới cửa khu, đứng trước cửa nhấn E / Enter để vào; điện thoại có cần điều khiển ảo.
// Xe đâm vào nhân vật thì gọi onHit('car'), hất nhân vật lên vỉa hè và miễn va chạm một lúc.
// City.mount(el, { look, name, level, at, onArrive, onDepart, onMove, onHit }) dựng bản đồ; City.walkTo(zone) cho nhân vật đi;
// City.teleport(zone) đặt nhân vật ngay trước cửa khu; City.setLocked(true) khoá mọi điều khiển (ví dụ khi nằm viện);
// City.setBadge(zone, text) gắn nhãn phụ (ví dụ yêu cầu HP) lên biển tên khu.
(function () {
  'use strict';
  const C = window.Character;

  const IMG = '/img/toon/city/';
  const FARM_IMG = '/img/toon/farm/';
  const T = 32; // một ô lưới = 32 đơn vị bản đồ
  const COLS = 50;
  const ROWS = 32;
  const W = COLS * T; // 1600
  const H = ROWS * T; // 1024
  const SKY = 96; // dải trời + đồi xa phía trên bản đồ (toạ độ y âm), biển tên các toà bờ Bắc nằm ở đây
  const HORIZON = 150; // đường chân đồi: trên đó là trời / đồi, dưới là cỏ

  // Lưới phố (theo ô): hai trục ngang, hai trục dọc rộng 3 ô, vỉa hè 1 ô mỗi bên; sông 4 ô chảy ngang giữa,
  // sát vỉa hè hai bờ.
  const ROAD_ROWS = [[9, 10, 11], [18, 19, 20]];
  const ROAD_COLS = [[15, 16, 17], [33, 34, 35]];
  const WALK_ROWS = [8, 12, 17, 21];
  const WALK_COLS = [14, 18, 32, 36];
  const RIVER_ROWS = [13, 14, 15, 16];
  const inRoadRow = (r) => ROAD_ROWS.some((g) => g.includes(r));
  const inRoadCol = (c) => ROAD_COLS.some((g) => g.includes(c));
  const onBridge = (c) => (c >= 14 && c <= 18) || (c >= 32 && c <= 36);
  const roadCenter = (n) => n * T + T / 2;

  // Các ô phố (theo ô: cột, hàng, rộng, cao): bờ Bắc hàng 0–7, bờ Nam hàng 22–31 (toà nhà + sân trước cửa).
  const BLOCKS = {
    casino: [0, 0, 14, 8], bank: [19, 0, 13, 8], shop: [37, 0, 13, 8],
    hospital: [0, 22, 14, 10], plaza: [19, 22, 13, 10], farm: [37, 22, 13, 10],
  };
  // Vật cản trong các khu bờ Nam (cột, hàng, rộng, cao): đài phun nước, nhà kho nông trại, toà bệnh viện.
  const FOUNTAIN = [22, 25, 7, 4];
  const BARN = [44, 24, 5, 3];
  const HOSPITAL = [3, 22, 8, 8]; // toà 242×240, cửa giữa đáy (x 224, y 960); sân trước hàng 30–31

  const ZONES = {
    plaza: { name: 'Quảng trường', sub: 'Nhân vật của bạn', node: 'plaza' },
    casino: { name: 'Casino', sub: 'Tiến Lên Miền Nam', node: 'casino', game: 'tienlen' },
    bank: { name: 'Ngân hàng', sub: 'Ví xu · Thưởng ngày', node: 'bank' },
    shop: { name: 'Shop thời trang', sub: 'Đổi trang phục', node: 'shop' },
    hospital: { name: 'Bệnh viện', sub: 'Chữa trị · Hồi máu', node: 'hospital' },
    farm: { name: 'Nông trại', sub: 'Trồng trọt · Chăn nuôi', node: 'farm' },
  };

  // Điểm đứng trước cửa / trong mỗi khu (đơn vị bản đồ).
  const NODES = {
    casino: [224, 276], bank: [816, 276], shop: [1392, 276],
    plaza: [816, 736], hospital: [224, 984], farm: [1392, 736],
  };
  const SPEED = 240; // đơn vị bản đồ / giây
  const SPRITE_SIZE = 96; // chiều cao nhân vật chibi (khung 96×148, neo chân giữa đáy)
  const HURT_MS = 1500;
  const DOOR_RADIUS = T * 1.6; // đứng trong bán kính này quanh cửa thì gợi ý "vào khu"

  // ---------- Lưới đi được ----------

  const inRect = (c, r, [rc, rr, rw, rh]) => c >= rc && c < rc + rw && r >= rr && r < rr + rh;

  // Ô (c, r) có đi được không: đường, vỉa hè, cầu, công viên ven sông và các khu bờ Nam (trừ đài phun, nhà kho);
  // nước sông và toà nhà bờ Bắc thì không.
  function cellWalkable(c, r) {
    if (c < 0 || r < 0 || c >= COLS || r >= ROWS) return false;
    if (RIVER_ROWS.includes(r)) return onBridge(c);
    if (r < 8) return r >= 5 && (WALK_COLS.includes(c) || inRoadCol(c)); // bờ Bắc: chỉ vỉa hè / đường giữa các toà nhà, tới chân đồi
    if (inRect(c, r, FOUNTAIN) || inRect(c, r, BARN) || inRect(c, r, HOSPITAL)) return false;
    return true;
  }
  const walkable = (x, y) => cellWalkable(Math.floor(x / T), Math.floor(y / T));
  // Chân nhân vật chiếm một khoảng nhỏ: cả bốn điểm quanh chân phải đi được.
  const footFree = (x, y) => walkable(x - 9, y - 4) && walkable(x + 9, y - 4) && walkable(x - 9, y + 3) && walkable(x + 9, y + 3);

  const cellCenter = (c, r) => [c * T + T / 2, r * T + T / 2];

  // Ô đi được gần nhất với một điểm (tìm loang theo vòng).
  function nearestWalkableCell(x, y) {
    const c0 = Math.max(0, Math.min(COLS - 1, Math.floor(x / T)));
    const r0 = Math.max(0, Math.min(ROWS - 1, Math.floor(y / T)));
    if (cellWalkable(c0, r0)) return [c0, r0];
    for (let d = 1; d < Math.max(COLS, ROWS); d++) {
      let best = null;
      for (let dc = -d; dc <= d; dc++) for (let dr = -d; dr <= d; dr++) {
        if (Math.max(Math.abs(dc), Math.abs(dr)) !== d || !cellWalkable(c0 + dc, r0 + dr)) continue;
        const [cx, cy] = cellCenter(c0 + dc, r0 + dr);
        const dd = Math.hypot(cx - x, cy - y);
        if (!best || dd < best[2]) best = [c0 + dc, r0 + dr, dd];
      }
      if (best) return [best[0], best[1]];
    }
    return [c0, r0];
  }

  // A* trên lưới ô, đi 8 hướng (không cắt góc tường), trả về danh sách ô.
  function astar([sc, sr], [tc, tr]) {
    const key = (c, r) => r * COLS + c;
    const h = (c, r) => Math.hypot(c - tc, r - tr);
    const open = new Map([[key(sc, sr), [sc, sr]]]);
    const g = new Map([[key(sc, sr), 0]]);
    const f = new Map([[key(sc, sr), h(sc, sr)]]);
    const prev = new Map();
    while (open.size) {
      let k = null;
      for (const kk of open.keys()) if (k === null || f.get(kk) < f.get(k)) k = kk;
      const [c, r] = open.get(k);
      open.delete(k);
      if (c === tc && r === tr) {
        const path = [[c, r]];
        let cur = k;
        while (prev.has(cur)) { cur = prev.get(cur); path.unshift([cur % COLS, Math.floor(cur / COLS)]); }
        return path;
      }
      for (let dc = -1; dc <= 1; dc++) for (let dr = -1; dr <= 1; dr++) {
        if (!dc && !dr) continue;
        const nc = c + dc;
        const nr = r + dr;
        if (!cellWalkable(nc, nr)) continue;
        if (dc && dr && (!cellWalkable(c + dc, r) || !cellWalkable(c, r + dr))) continue;
        const nk = key(nc, nr);
        const ng = g.get(k) + (dc && dr ? Math.SQRT2 : 1);
        if (!g.has(nk) || ng < g.get(nk)) {
          g.set(nk, ng);
          f.set(nk, ng + h(nc, nr));
          prev.set(nk, k);
          open.set(nk, [nc, nr]);
        }
      }
    }
    return null;
  }

  // Có đi thẳng từ a tới b được không (kiểm tra từng đoạn ngắn trên đường thẳng).
  function clearLine([ax, ay], [bx, by]) {
    const n = Math.ceil(Math.hypot(bx - ax, by - ay) / 8);
    for (let i = 1; i <= n; i++) {
      const x = ax + ((bx - ax) * i) / n;
      const y = ay + ((by - ay) * i) / n;
      if (!footFree(x, y)) return false;
    }
    return true;
  }

  // Đường đi từ điểm hiện tại tới đích: A* trên lưới rồi rút gọn các điểm thẳng hàng cho đi mượt.
  function findPath(from, to) {
    if (clearLine(from, to)) return [to];
    const cells = astar(nearestWalkableCell(...from), nearestWalkableCell(...to));
    if (!cells) return [to];
    const pts = cells.map(([c, r]) => cellCenter(c, r));
    pts[pts.length - 1] = to;
    const out = [];
    let cur = from;
    let i = 0;
    while (i < pts.length) {
      let j = pts.length - 1;
      while (j > i && !clearLine(cur, pts[j])) j--;
      out.push(pts[j]);
      cur = pts[j];
      i = j + 1;
    }
    return out;
  }

  // ---------- Ảnh ----------

  // Kích thước gốc của mọi ảnh (đọc từ manifest.json của hai thư mục); khoá "farm/x" cho ảnh nông trại.
  const sizes = {};
  const images = {};
  const srcOf = (name) => (name.startsWith('farm/') ? FARM_IMG + name.slice(5) : IMG + name) + '.png';

  async function loadManifests() {
    await Promise.all([[IMG, ''], [FARM_IMG, 'farm/']].map(async ([dir, prefix]) => {
      try {
        const m = await (await fetch(dir + 'manifest.json')).json();
        for (const it of m.images || []) sizes[prefix + it.file.replace(/\.png$/, '')] = [it.w, it.h];
      } catch (e) { /* bỏ qua */ }
    }));
  }

  function loadImages(names) {
    return Promise.all(names.filter((n) => sizes[n]).map((n) => new Promise((resolve) => {
      if (images[n]) return resolve();
      const img = new Image();
      img.onload = () => { images[n] = img; resolve(); };
      img.onerror = () => resolve(); // thiếu ảnh thì bỏ qua, không vẽ thay bằng hình khối
      img.src = srcOf(n);
    })));
  }

  // ---------- Mặt đất trên canvas ----------

  const GROUND_IMAGES = [
    'grass_tile', 'road_h', 'road_v', 'road_plain', 'road_crosswalk_h', 'road_crosswalk_v', 'sidewalk_tile',
    'river_bank_top', 'river_bank_bottom', 'bridge_deck', 'bridge_rail_v', 'farm/bg_sky_hills', 'farm/soil_plot',
    'farm/soil_plot_wet', 'farm/dirt_path_tile', 'farm/grass_plot',
  ];

  function drawStatic(canvas) {
    const g = canvas.getContext('2d');
    g.imageSmoothingEnabled = true;
    g.imageSmoothingQuality = 'high';
    // Lấp hình chữ nhật bằng ảnh lặp, phóng to / thu nhỏ theo k.
    const fill = (name, x, y, w, h, k = 1, ox = 0, oy = 0) => {
      const img = images[name];
      if (!img) return;
      const pat = g.createPattern(img, 'repeat');
      pat.setTransform(new DOMMatrix([k, 0, 0, k, x + ox, y + oy]));
      g.fillStyle = pat;
      g.fillRect(x, y, w, h);
    };
    const put = (name, x, y, w, h) => images[name] && g.drawImage(images[name], x, y, w, h);
    g.save();
    g.translate(0, SKY);

    // Trời + đồi xa: chân ảnh nằm ở đường chân đồi.
    const sky = images['farm/bg_sky_hills'];
    if (sky) {
      const k = W / sky.width;
      g.drawImage(sky, 0, HORIZON - sky.height * k, W, sky.height * k);
    }
    // Cỏ toàn bản đồ dưới chân đồi.
    fill('grass_tile', 0, HORIZON, W, H - HORIZON, 0.5);

    // Vỉa hè (hàng / cột vỉa hè), mặt đường ngang và dọc, ngã tư, vạch qua đường.
    const top = 5 * T; // đường dọc bờ Bắc bắt đầu từ chân đồi
    for (const r of WALK_ROWS) fill('sidewalk_tile', 0, r * T, W, T, 0.5);
    for (const c of WALK_COLS) fill('sidewalk_tile', c * T, top, T, H - top, 0.5);
    for (const g3 of ROAD_ROWS) fill('road_h', 0, g3[0] * T, W, 3 * T, 0.75);
    for (const g3 of ROAD_COLS) fill('road_v', g3[0] * T, top, 3 * T, H - top, 0.75);
    for (const r3 of ROAD_ROWS) for (const c3 of ROAD_COLS) fill('road_plain', c3[0] * T, r3[0] * T, 3 * T, 3 * T, 0.75);
    for (const r3 of ROAD_ROWS) for (const c of WALK_COLS) put('road_crosswalk_h', c * T - 14, r3[0] * T + 4, T + 28, 3 * T - 8);
    for (const c3 of ROAD_COLS) for (const r of [8, 21]) put('road_crosswalk_v', c3[0] * T + 4, r * T - 14, 3 * T - 8, T + 28);

    // Sông: bờ trên / dưới (nước ở giữa là lớp HTML gợn sóng), cầu gỗ có lan can.
    const ry = RIVER_ROWS[0] * T;
    const rh = RIVER_ROWS.length * T;
    fill('water_tile', 0, ry, W, rh, 0.75);
    fill('river_bank_top', 0, ry, W, 36, 0.75);
    fill('river_bank_bottom', 0, ry + rh - 36, W, 36, 0.75);
    for (const [a, b] of [[14, 19], [32, 37]]) {
      fill('bridge_deck', a * T, ry - 6, (b - a) * T, rh + 12, 0.75);
      g.fillStyle = 'rgba(70, 40, 10, .25)';
      g.fillRect(a * T, ry + rh + 6, (b - a) * T, 6);
      fill('bridge_rail_v', a * T - 6, ry - 18, 20, rh + 30, 1);
      fill('bridge_rail_v', b * T - 14, ry - 18, 20, rh + 30, 1);
    }

    // Nền riêng các khu bờ Nam: quảng trường và sân bệnh viện lát gạch, nông trại có đường đất + ruộng.
    const [pc, pr, pw, ph] = BLOCKS.plaza;
    fill('sidewalk_tile', pc * T, pr * T, pw * T, ph * T, 0.75);
    fill('sidewalk_tile', 3 * T, 29 * T, 8 * T, 3 * T, 0.75);
    const [fc, fr] = BLOCKS.farm;
    fill('farm/dirt_path_tile', fc * T, 23 * T, 13 * T, 40, 0.5);
    for (let i = 0; i < 4; i++) for (let j = 0; j < 2; j++) {
      put(j ? 'farm/soil_plot_wet' : 'farm/soil_plot', (fc + 1) * T + i * 60, (fr + 3) * T + j * 60, 58, 58);
    }
    g.restore();
  }

  // ---------- Toà nhà, cây, đồ vật (ảnh HTML xếp theo chiều sâu) ----------

  // Mỗi món: [ảnh, x giữa đáy, y đáy, tỉ lệ, lật ngang?, class thêm].
  function props() {
    const list = [];
    const at = (img, x, y, k = 1, flip = false, cls = '') => list.push([img, x, y, k, flip, cls]);

    // Hàng cây / bụi dọc chân đồi, che chỗ đường dọc biến mất vào xa.
    for (let x = 30; x < W; x += 92) at(['bush_1', 'farm/bush_1', 'bush_3'][(x / 92) % 3 | 0], x, HORIZON + 6, 0.7);
    for (const c3 of ROAD_COLS) at('tree_big', (c3[0] + 1.5) * T, HORIZON + 14, 0.75);

    // Toà nhà bờ Bắc, cửa ở đáy (sát vỉa hè hàng 8).
    at('building_casino', 224, 8 * T, 1);
    at('building_bank', 816, 8 * T, 0.86);
    at('building_shop', 1392 - (165 - 105.5) * 1, 8 * T, 1);
    at('house_2', 1532, 8 * T, 0.72);
    at('tree_1', 40, 8 * T + 2, 0.8);
    at('tree_1', 410, 8 * T + 2, 0.72);
    at('farm/flowers', 150, 8 * T + 4, 0.6);
    at('farm/flowers', 300, 8 * T + 4, 0.6);

    // Vỉa hè: đèn đường hai bên đường ngang.
    for (const x of [96, 400, 700, 940, 1250, 1540]) {
      at('lamp', x, 9 * T - 2, 0.75);
      at('lamp', x + 30, 22 * T - 2, 0.75);
    }

    // Công viên ven sông (vỉa hè hai bờ): cây nhỏ, bụi, ghế; chừa chỗ hai cây cầu.
    const free = (x) => !(x > 13 * T && x < 20 * T) && !(x > 31 * T && x < 38 * T);
    const kinds = ['tree_4', 'bush_2', 'tree_3', 'bush_3', 'tree_4', 'bush_4'];
    let k = 0;
    for (let x = 40; x < W; x += 104) {
      if (!free(x)) continue;
      const kind = kinds[k++ % kinds.length];
      at(kind, x, 13 * T - 4, kind.startsWith('tree') ? 1.3 : 0.9);
      if (free(x + 52)) at(kinds[(k + 2) % kinds.length], x + 52, 18 * T - 4, 1.1);
    }
    for (const x of [200, 760, 1300]) {
      at('bench', x, 13 * T - 2, 0.6);
      at('bench', x + 60, 18 * T - 2, 0.6);
    }

    // Bệnh viện: toà nhà, xe cứu thương đậu trước sân, cây, ghế chờ.
    at('building_hospital', 224, 30 * T, 1);
    at('ambulance_side', 392, 31 * T + 8, 0.75);
    at('bench', 70, 31 * T + 4, 0.6);
    at('tree_1', 36, 26 * T, 0.8);
    at('tree_1', 412, 26 * T, 0.7);
    at('farm/flowers', 110, 30 * T, 0.55);
    at('farm/flowers', 340, 30 * T, 0.55);

    // Quảng trường: đài phun nước ở giữa, ghế, cây và chậu hoa ở góc.
    at('fountain', 816, 900, 1.2);
    at('bench', 690, 960, 0.65);
    at('bench', 942, 960, 0.65);
    at('tree_big', 640, 800, 0.62);
    at('tree_big', 992, 800, 0.62);
    at('tree_1', 628, 1010, 0.6);
    at('tree_1', 1004, 1010, 0.6);
    at('farm/flowers', 720, 780, 0.6);
    at('farm/flowers', 912, 780, 0.6);

    // Nông trại: hàng rào, nhà kho, rơm, cây, cây trồng trên ruộng, gia súc thả rông.
    for (let x = 1190; x < 1430; x += 64) at('farm/fence_piece', x + 32, 23 * T + 46, 0.9);
    at('farm/barn', 1500, 27 * T, 0.75);
    at('farm/hay', 1420, 27 * T + 10, 0.6);
    at('farm/silo', 1584, 27 * T - 6, 0.5);
    at('farm/tree_big', 1582, 31 * T + 18, 0.55);
    const crops = ['corn_ripe', 'tomato_ripe', 'carrot_2', 'watermelon_ripe', 'strawberry_2', 'rice_ripe', 'corn_2', 'tomato_2'];
    crops.forEach((c, i) => at('farm/crop_' + c, 38 * T + (i % 4) * 60 + 29, 25 * T + Math.floor(i / 4) * 60 + 50, 0.55));
    at('farm/animal_cow', 1300, 31 * T + 14, 0.6, true);
    at('farm/animal_sheep', 1220, 30 * T + 10, 0.55);
    at('farm/animal_pig', 1440, 31 * T + 12, 0.6, true);
    at('farm/animal_chicken', 1380, 30 * T + 4, 0.6);
    at('farm/animal_chicken_white', 1410, 30 * T + 16, 0.6, true);

    return list;
  }

  // ---------- Phần động (HTML) ----------

  const px = (n) => `${Math.round(n)}px`;

  // Ảnh đồ vật đặt theo giữa đáy, z-index theo y để nhân vật đi trước / sau đúng.
  function propHtml([name, x, y, k, flip, cls]) {
    const sz = sizes[name];
    if (!sz) return '';
    const w = sz[0] * k;
    const h = sz[1] * k;
    return `<img class="prop${flip ? ' flip' : ''}${cls ? ' ' + cls : ''}" src="${srcOf(name)}" alt="" draggable="false"
      style="left:${px(x - w / 2)};top:${px(y - h)};width:${px(w)};height:${px(h)};z-index:${100 + Math.round(y)}">`;
  }

  // Mây trôi trên trời.
  function cloudsHtml() {
    return [['farm/cloud_1', 40, -88, 0.5, 70], ['farm/cloud_2', 520, -92, 0.42, 95], ['farm/cloud_3', 1080, -80, 0.46, 80]]
      .filter(([n]) => sizes[n])
      .map(([n, x, y, k, dur]) => `<img class="cloud" src="${srcOf(n)}" alt="" draggable="false"
        style="left:${px(x)};top:${px(y)};width:${px(sizes[n][0] * k)};animation-duration:${dur}s">`)
      .join('');
  }

  // Nước sông gợn: các đoạn giữa hai cây cầu, nền ảnh mặt nước trôi dần.
  function waterHtml() {
    const segs = [[0, 14], [19, 32], [37, COLS]];
    return segs
      .map(([a, b]) => `<div class="river-flow" style="left:${px(a * T)};top:${px(RIVER_ROWS[0] * T + 34)};width:${px((b - a) * T)};height:${px(RIVER_ROWS.length * T - 68)}"></div>`)
      .join('');
  }

  // Xe chạy vòng trên các làn đường ngang (ảnh nghiêng, lật theo hướng). Đường dọc dành cho người đi bộ qua cầu.
  // Vị trí tính bằng JS (không dùng CSS animation) để phát hiện va chạm với nhân vật.
  const CARS = [
    { img: 'car_red_side', dir: 'east', y: roadCenter(11), speed: 115, at: 0 },
    { img: 'car_blue_side', dir: 'west', y: roadCenter(9), speed: 95, at: 700 },
    { img: 'car_yellow_side', dir: 'east', y: roadCenter(11), speed: 115, at: 950 },
    { img: 'car_green_side', dir: 'east', y: roadCenter(20), speed: 130, at: 400 },
    { img: 'ambulance_side', dir: 'west', y: roadCenter(18), speed: 105, at: 1200 },
    { img: 'car_red_side', dir: 'west', y: roadCenter(18), speed: 105, at: 300 },
  ];
  // Hộp va chạm quanh tâm làn (đơn vị bản đồ): [nửa rộng, trên, dưới].
  const CAR_BOX = { h: [44, 34, 34], v: [0, 0, 0] };
  const CAR_SCALE = 0.8;
  const YIELD_DIST = 170; // xe dừng nhường khi nhân vật đi tự động băng qua đường phía trước trong khoảng này
  const carHorizontal = (car) => car.dir === 'east' || car.dir === 'west';

  function trafficHtml() {
    return CARS.map((car, i) => {
      const [w, h] = sizes[car.img] || [120, 52];
      return `<div class="car car-${car.dir}" data-car="${i}"><img src="${srcOf(car.img)}" alt="" draggable="false"
        style="width:${px(w * CAR_SCALE)};height:${px(h * CAR_SCALE)}"></div>`;
    }).join('');
  }

  // Vị trí tâm xe hiện tại.
  function carPos(car) {
    const span = carHorizontal(car) ? W + 240 : H + 240;
    const p = ((car.at % span) + span) % span - 120;
    if (car.dir === 'east') return [p, car.y];
    if (car.dir === 'west') return [W - p, car.y];
    if (car.dir === 'south') return [car.x, p];
    return [car.x, H - p];
  }

  // Thuyền: mỗi chiếc chạy qua lại trong một đoạn sông giữa hai cây cầu.
  const BOATS = [
    { img: 'boat_1', from: 2 * T, to: 12 * T, y: 14 * T + 30, speed: 55, at: 0 },
    { img: 'boat_2', from: 21 * T, to: 30 * T, y: 15 * T + 10, speed: 28, at: 120 },
    { img: 'boat_3', from: 39 * T, to: 48 * T, y: 14 * T + 34, speed: 45, at: 260 },
  ];
  function boatsHtml() {
    return BOATS.map((b, i) => {
      const [w, h] = sizes[b.img] || [80, 42];
      return `<div class="boat" data-boat="${i}"><img src="${srcOf(b.img)}" alt="" draggable="false" style="width:${px(w * 0.85)};height:${px(h * 0.85)}"></div>`;
    }).join('');
  }

  // Biển CASINO nhấp nháy đè đúng lên biển trên nóc toà Casino.
  function signHtml() {
    const [bw, bh] = sizes.building_casino || [262, 260];
    const [sw, sh] = sizes.sign_casino || [170, 44];
    return `<img class="casino-sign" src="${srcOf('sign_casino')}" alt="" draggable="false"
      style="left:${px(224 - sw / 2)};top:${px(8 * T - bh + 4)};width:${px(sw)};height:${px(sh)};z-index:${100 + 8 * T + 1}">`;
  }

  // Vùng bấm của từng khu và biển tên (HTML).
  function zonesHtml() {
    const labelAt = { casino: [224, -84], bank: [816, -84], shop: [1392, -84], plaza: [700, 712], hospital: [78, 742], farm: [1290, 712] };
    return Object.entries(ZONES)
      .map(([id, z]) => {
        const [c, r, w, h] = BLOCKS[id];
        const [lx, ly] = labelAt[id];
        const top = r * T - (r === 0 ? SKY : 0);
        return `<div class="zone${z.soon ? ' soon' : ''}" data-zone="${id}" style="left:${px(c * T)};top:${px(top)};width:${px(w * T)};height:${px(h * T + (r === 0 ? SKY : 0))}"></div>
          <div class="zone-label${z.soon ? ' soon' : ''}" data-zone="${id}" style="left:${px(lx)};top:${px(ly)}">
            <span class="zl-name">${z.name}</span><span class="zl-sub" id="zl-sub-${id}">${z.sub}</span>
            <span class="zl-badge hidden" id="zl-badge-${id}"></span>
          </div>`;
      })
      .join('');
  }

  // ---------- Nhân vật và điều khiển ----------

  let root = null;
  let world = null;
  let overlay = null;
  let scale = 1;
  let me = null; // { x, y, dir, look, walking, el, body }
  let opts = {};
  let path = null; // các điểm đang đi tới (bấm chuột / đi tới khu)
  let pathZone = null; // khu sẽ "vào" khi đi hết path
  let nearZone = null; // khu đang đứng trước cửa
  let moving = false; // đang di chuyển (để gọi onDepart một lần khi bắt đầu đi)
  const keys = new Set();
  const stick = { x: 0, y: 0 }; // cần điều khiển ảo
  let locked = false; // khoá điều khiển (ví dụ đang nằm viện)
  let hurtUntil = 0; // đang nháy đỏ / miễn va chạm tới thời điểm này (ms)

  function placeMe() {
    me.el.style.transform = `translate(${me.x}px, ${me.y}px)`;
    me.el.style.zIndex = String(100 + Math.round(me.y));
  }

  function drawMe() {
    const look = me.look || {};
    const hurt = performance.now() < hurtUntil;
    if (C && typeof C.sprite === 'function') {
      me.body.innerHTML = C.sprite(look, { dir: me.dir, walking: me.walking && !locked, size: SPRITE_SIZE, pose: locked ? 'lie' : undefined, hurt });
    } else if (C && typeof C.svg === 'function') {
      me.body.innerHTML = `<div class="me-fallback">${C.svg(look, { size: SPRITE_SIZE })}</div>`;
    }
    me.body.className = 'me-body dir-' + me.dir + (me.walking ? ' walking' : '') + (hurt ? ' hurt' : '');
  }

  function setWalking(on, dir) {
    if (on === me.walking && (!dir || dir === me.dir)) return;
    me.walking = on;
    if (dir) me.dir = dir;
    drawMe();
  }

  function setTag(name, level) {
    if (!root) return;
    root.querySelector('#me-tag .tag-name').textContent = name;
    root.querySelector('#me-tag .tag-lv').textContent = 'Lv ' + level;
  }

  const dirOf = (dx, dy) => (Math.abs(dx) > Math.abs(dy) ? (dx < 0 ? 'left' : 'right') : dy < 0 ? 'up' : 'down');

  // Gợi ý "vào khu" khi đứng gần cửa.
  function updateNear() {
    let found = null;
    for (const [id, z] of Object.entries(ZONES)) {
      const [nx, ny] = NODES[z.node];
      if (Math.hypot(nx - me.x, ny - me.y) <= DOOR_RADIUS) found = id;
    }
    if (locked) found = null;
    if (found === nearZone) return;
    nearZone = found;
    const hint = overlay.querySelector('.enter-hint');
    hint.classList.toggle('hidden', !found);
    if (found) hint.querySelector('.eh-name').textContent = ZONES[found].name;
  }

  function startMoving() {
    if (!moving) {
      moving = true;
      hideHelp();
      if (opts.onDepart) opts.onDepart(null);
    }
  }

  // Dòng hướng dẫn phím: ẩn khi người chơi đã bắt đầu đi (hoặc sau 15 giây) để không che bản đồ.
  function hideHelp() {
    const el = overlay && overlay.querySelector('.city-help');
    if (el) el.classList.add('hidden');
  }

  function savePos() {
    try { localStorage.setItem('city.pos', JSON.stringify([Math.round(me.x), Math.round(me.y)])); } catch (e) { /* bỏ qua */ }
  }

  function enter(zone) {
    try { localStorage.setItem('city.at', zone); } catch (e) { /* bỏ qua */ }
    savePos();
    if (opts.onArrive) opts.onArrive(zone);
  }

  // Đi theo các điểm cho trước; tới nơi thì vào khu nếu là đi tới khu.
  function follow(points, zone) {
    path = points.slice();
    pathZone = zone || null;
    root.querySelectorAll('.zone, .zone-label').forEach((el) => el.classList.toggle('target', !!zone && el.dataset.zone === zone));
    if (!path.length) {
      path = null;
      if (zone) enter(zone);
      return;
    }
    startMoving();
  }

  function walkTo(zone) {
    const z = ZONES[zone];
    if (!z || !me || locked) return;
    hideMarker();
    follow(findPath([me.x, me.y], NODES[z.node]), zone);
  }

  // Bấm chuột / chạm vào một điểm trên bản đồ.
  function walkToPoint(x, y) {
    if (locked) return;
    let target = [x, y];
    if (!footFree(x, y)) target = cellCenter(...nearestWalkableCell(x, y));
    showMarker(target);
    follow(findPath([me.x, me.y], target), null);
  }

  function showMarker([x, y]) {
    const m = world.querySelector('.walk-target');
    m.style.transform = `translate(${x}px, ${y}px)`;
    m.classList.remove('hidden');
    m.classList.remove('pulse');
    void m.offsetWidth; // chạy lại hiệu ứng
    m.classList.add('pulse');
  }
  function hideMarker() {
    world.querySelector('.walk-target').classList.add('hidden');
  }

  // Đi tự do theo một vector (bàn phím / cần điều khiển), trượt dọc tường khi va chạm.
  function stepFree(vx, vy, dt) {
    const len = Math.hypot(vx, vy);
    if (len < 0.15) return false;
    const k = (SPEED * dt) / Math.max(1, len);
    const dx = vx * k;
    const dy = vy * k;
    let moved = false;
    if (footFree(me.x + dx, me.y)) { me.x += dx; moved = true; }
    if (footFree(me.x, me.y + dy)) { me.y += dy; moved = true; }
    setWalking(true, dirOf(vx, vy));
    return moved;
  }

  function stepPath(dt) {
    let left = SPEED * dt;
    while (left > 0 && path.length) {
      const [tx, ty] = path[0];
      const dx = tx - me.x;
      const dy = ty - me.y;
      const len = Math.hypot(dx, dy);
      if (len > 0.5) setWalking(true, dirOf(dx, dy));
      if (len <= left) {
        me.x = tx;
        me.y = ty;
        left -= len;
        path.shift();
      } else {
        me.x += (dx / len) * left;
        me.y += (dy / len) * left;
        left = 0;
      }
    }
    if (!path.length) {
      const zone = pathZone;
      path = null;
      pathZone = null;
      hideMarker();
      stop();
      if (zone) enter(zone);
    }
  }

  function stop() {
    if (moving) savePos();
    moving = false;
    setWalking(false, 'down' === me.dir ? 'down' : me.dir);
  }

  function inputVector() {
    let vx = 0;
    let vy = 0;
    if (keys.has('left')) vx -= 1;
    if (keys.has('right')) vx += 1;
    if (keys.has('up')) vy -= 1;
    if (keys.has('down')) vy += 1;
    return [vx + stick.x, vy + stick.y];
  }

  // Di chuyển xe / thuyền và kiểm tra xe đâm nhân vật.
  // Xe có nên dừng nhường không: nhân vật đang đi tự động (bấm chuột / thanh đi nhanh), đang ở trên đường
  // hoặc sát lề, cùng làn và ở ngay phía trước xe.
  function mustYield(car) {
    if (!me || !path || locked) return false;
    const c = Math.floor(me.x / T);
    const r = Math.floor(me.y / T);
    const nearRoad = [-1, 0, 1].some((d) => inRoadRow(r + d) || inRoadCol(c + d));
    if (!nearRoad) return false;
    const [x, y] = carPos(car);
    switch (car.dir) {
      case 'east': return Math.abs(me.y - 12 - y) < 44 && me.x > x && me.x - x < YIELD_DIST;
      case 'west': return Math.abs(me.y - 12 - y) < 44 && me.x < x && x - me.x < YIELD_DIST;
      case 'south': return Math.abs(me.x - x) < 40 && me.y > y && me.y - y < YIELD_DIST;
      default: return Math.abs(me.x - x) < 40 && me.y < y && y - me.y < YIELD_DIST + 20;
    }
  }

  function stepTraffic(dt, now) {
    CARS.forEach((car, i) => {
      if (!mustYield(car)) car.at += car.speed * dt;
      const [x, y] = carPos(car);
      car.el = car.el || world.querySelector(`[data-car="${i}"]`);
      if (!car.el) return; // ảnh chưa dựng xong
      car.el.style.transform = `translate(${x}px, ${y}px)`;
      car.el.style.zIndex = String(100 + Math.round(y));
      // Đi tự động thì xe đã nhường, không tính va chạm (tránh bị gián đoạn khi đi bằng thanh đi nhanh).
      if (!me || locked || path || now < hurtUntil) return;
      const [hw, up, down] = carHorizontal(car) ? CAR_BOX.h : CAR_BOX.v;
      // Chân nhân vật nằm trong làn xe (quanh tâm làn) và trùng thân xe theo chiều ngang.
      if (Math.abs(me.x - x) < hw + 12 && me.y > y - up && me.y < y + down) hit('car', car);
    });
    BOATS.forEach((b, i) => {
      b.at += b.speed * dt;
      const span = b.to - b.from;
      const k = b.at % (2 * span);
      const x = k < span ? b.from + k : b.to - (k - span);
      b.el = b.el || world.querySelector(`[data-boat="${i}"]`);
      if (!b.el) return;
      b.el.style.transform = `translate(${x}px, ${b.y}px)`;
      b.el.classList.toggle('back', k >= span);
    });
  }

  // Bị xe đâm: báo ra ngoài một lần, hất lên vỉa hè gần nhất, nháy đỏ và miễn va chạm một lúc.
  function hit(kind, car) {
    hurtUntil = performance.now() + HURT_MS;
    path = null;
    pathZone = null;
    hideMarker();
    const [x, y] = safeSpot(me.x, me.y, car);
    me.x = x;
    me.y = y;
    me.walking = false;
    drawMe();
    afterMove();
    savePos();
    setTimeout(() => drawMe(), HURT_MS + 20);
    if (opts.onHit) opts.onHit(kind);
  }

  // Ô đi được gần nhất không nằm trên đường xe chạy (vỉa hè, công viên...).
  function safeSpot(x, y) {
    const c0 = Math.floor(x / T);
    const r0 = Math.floor(y / T);
    const ok = (c, r) => cellWalkable(c, r) && !inRoadRow(r) && !inRoadCol(c);
    for (let d = 1; d < COLS; d++) {
      let best = null;
      for (let dc = -d; dc <= d; dc++) for (let dr = -d; dr <= d; dr++) {
        if (Math.max(Math.abs(dc), Math.abs(dr)) !== d || !ok(c0 + dc, r0 + dr)) continue;
        const [cx, cy] = cellCenter(c0 + dc, r0 + dr);
        const dd = Math.hypot(cx - x, cy - y);
        if (!best || dd < best[2]) best = [cx, cy, dd];
      }
      if (best) return [best[0], best[1]];
    }
    return [x, y];
  }

  let last = 0;
  function tick(now) {
    const dt = Math.min(0.05, (now - (last || now)) / 1000);
    last = now;
    if (world) stepTraffic(dt, now);
    if (me && locked) {
      if (me.walking) { me.walking = false; drawMe(); }
    } else if (me) {
      const [vx, vy] = inputVector();
      if (Math.hypot(vx, vy) >= 0.15) {
        if (path) { path = null; pathZone = null; hideMarker(); }
        startMoving();
        stepFree(vx, vy, dt);
        afterMove();
      } else if (path) {
        stepPath(dt);
        afterMove();
      } else if (me.walking) {
        stop();
      }
    }
    requestAnimationFrame(tick);
  }

  function afterMove() {
    placeMe();
    updateNear();
    if (opts.onMove) opts.onMove(me.x, me.y);
  }

  const KEYMAP = {
    ArrowLeft: 'left', ArrowRight: 'right', ArrowUp: 'up', ArrowDown: 'down',
    a: 'left', d: 'right', w: 'up', s: 'down', A: 'left', D: 'right', W: 'up', S: 'down',
  };
  // Bỏ qua phím khi đang gõ chữ hoặc đang mở hộp thoại / bản đồ đang ẩn.
  function ignoreKeys(e) {
    const t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return true;
    if (document.querySelector('dialog[open]')) return true;
    return !root || !root.offsetParent;
  }

  function onKeyDown(e) {
    if (ignoreKeys(e) || e.ctrlKey || e.metaKey || e.altKey) return;
    const k = KEYMAP[e.key];
    if (k) {
      keys.add(k);
      e.preventDefault();
    } else if ((e.key === 'e' || e.key === 'E' || e.key === 'Enter') && nearZone && !locked) {
      e.preventDefault();
      enter(nearZone);
    }
  }
  function onKeyUp(e) {
    const k = KEYMAP[e.key];
    if (k) keys.delete(k);
  }

  // Cần điều khiển ảo cho điện thoại.
  function bindJoystick(pad) {
    const knob = pad.querySelector('.joy-knob');
    let id = null;
    const set = (e) => {
      const r = pad.getBoundingClientRect();
      const max = r.width / 2;
      let dx = e.clientX - (r.left + max);
      let dy = e.clientY - (r.top + max);
      const len = Math.hypot(dx, dy);
      if (len > max) { dx = (dx / len) * max; dy = (dy / len) * max; }
      knob.style.transform = `translate(${dx}px, ${dy}px)`;
      stick.x = dx / max;
      stick.y = dy / max;
    };
    pad.addEventListener('pointerdown', (e) => { id = e.pointerId; pad.setPointerCapture(id); set(e); e.preventDefault(); });
    pad.addEventListener('pointermove', (e) => e.pointerId === id && set(e));
    const end = (e) => {
      if (e.pointerId !== id) return;
      id = null;
      stick.x = stick.y = 0;
      knob.style.transform = '';
    };
    pad.addEventListener('pointerup', end);
    pad.addEventListener('pointercancel', end);
  }

  // Thu phóng cả bản đồ cho vừa khung: máy tính thấy trọn bản đồ, điện thoại lấp đầy chiều cao và cuộn ngang.
  function fit() {
    if (!root) return;
    const cw = root.clientWidth;
    const ch = root.clientHeight;
    if (!cw || !ch) return;
    const HH = H + SKY;
    scale = cw < 860 ? ch / HH : Math.min(cw / W, ch / HH);
    const stage = root.querySelector('.city-stage');
    stage.style.width = px(W * scale);
    stage.style.height = px(HH * scale);
    root.querySelector('.city-world').style.transform = `scale(${scale})`;
    // Ngoài mép bản đồ: trời phía trên chân đồi, cỏ phía dưới (khớp với bản đồ khi màn hình rộng hơn bản đồ).
    const top = Math.max(0, (ch - HH * scale) / 2);
    const horizon = Math.round(top + (SKY + HORIZON) * scale);
    root.style.backgroundImage = `linear-gradient(#7fc6f0 0, #c4e9fb ${horizon - 20}px, #8fcf63 ${horizon - 20}px, transparent ${horizon}px), url(${IMG}grass_tile.png)`;
    root.style.backgroundSize = `100% 100%, ${64 * scale}px ${64 * scale}px`;
  }

  function mount(el, o) {
    opts = o;
    root = el;
    el.classList.add('city-toon');
    el.innerHTML = `<div class="city-stage">
      <div class="city-world" style="width:${px(W)};height:${px(H + SKY)}" role="img" aria-label="Bản đồ DCity">
        <canvas class="city-canvas" width="${W}" height="${H + SKY}"></canvas>
        <div class="city-inner" style="top:${px(SKY)};width:${px(W)};height:${px(H)}">
          <div class="city-props"></div>
          <div class="walk-target hidden"></div>
          <div id="me" class="me">
            <div id="me-tag" class="me-tag"><span class="tag-name"></span><span class="tag-lv"></span></div>
            <div class="me-body"></div>
          </div>
        </div>
      </div>
    </div>`;
    world = el.querySelector('.city-inner');
    // Nạp kích thước ảnh trước rồi dựng phần ảnh HTML (cần w/h) và vẽ mặt đất.
    loadManifests().then(() => {
      el.querySelector('.city-props').innerHTML =
        cloudsHtml() + waterHtml() + boatsHtml() + props().map(propHtml).join('') + signHtml() + trafficHtml() + zonesHtml();
      el.querySelectorAll('[data-zone]').forEach((z) => z.addEventListener('click', (e) => {
        e.stopPropagation();
        walkTo(z.dataset.zone);
      }));
      CARS.forEach((c) => { c.el = null; });
      BOATS.forEach((b) => { b.el = null; });
      return loadImages(GROUND_IMAGES).then(() => drawStatic(el.querySelector('.city-canvas')));
    });

    // Lớp điều khiển (không cuộn theo bản đồ): gợi ý vào khu, hướng dẫn phím, cần điều khiển ảo.
    overlay = document.createElement('div');
    overlay.className = 'city-overlay';
    let helpHidden = false;
    try { helpHidden = localStorage.getItem('city.help') === 'off'; } catch (e) { /* bỏ qua */ }
    overlay.innerHTML = `
      <button class="enter-hint hidden" type="button"><span class="eh-key">E</span>Vào <b class="eh-name"></b></button>
      <div class="city-help${helpHidden ? ' hidden' : ''}">WASD / ↑↓←→ để đi · Bấm chuột để tới · E để vào
        <button class="link icon-only city-help-close" type="button" title="Ẩn">×</button></div>
      <div class="joystick" aria-label="Cần điều khiển"><div class="joy-knob"></div></div>`;
    el.parentElement.appendChild(overlay);
    overlay.querySelector('.enter-hint').addEventListener('click', () => nearZone && !locked && enter(nearZone));
    overlay.querySelector('.city-help-close').addEventListener('click', () => {
      overlay.querySelector('.city-help').classList.add('hidden');
      try { localStorage.setItem('city.help', 'off'); } catch (e) { /* bỏ qua */ }
    });
    bindJoystick(overlay.querySelector('.joystick'));
    setTimeout(hideHelp, 15000);

    // Vị trí bắt đầu: chỗ đứng lần trước nếu còn hợp lệ, không thì trước cửa khu đã ở.
    const start = ZONES[o.at] ? o.at : 'plaza';
    let [x, y] = NODES[ZONES[start].node];
    try {
      const saved = JSON.parse(localStorage.getItem('city.pos') || 'null');
      if (Array.isArray(saved) && footFree(saved[0], saved[1])) [x, y] = saved;
    } catch (e) { /* bỏ qua */ }
    me = { x, y, dir: 'down', look: o.look, walking: false, el: el.querySelector('#me'), body: el.querySelector('#me .me-body') };
    drawMe();
    placeMe();
    setTag(o.name, o.level);

    // Bấm toà nhà / biển tên: đi tới cửa khu (gắn khi dựng biển); bấm chỗ khác: đi tới điểm đó.
    el.querySelector('.city-world').addEventListener('click', (e) => {
      const r = world.getBoundingClientRect();
      walkToPoint((e.clientX - r.left) / scale, (e.clientY - r.top) / scale);
    });

    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('keyup', onKeyUp);
    window.addEventListener('blur', () => keys.clear());
    fit();
    window.addEventListener('resize', fit);
    if (window.ResizeObserver) new ResizeObserver(fit).observe(el);
    updateNear();
    requestAnimationFrame(tick);
    return start;
  }

  function setLook(look) {
    me.look = look;
    drawMe();
  }

  function setSub(zone, text) {
    const el = root && root.querySelector('#zl-sub-' + zone);
    if (el) el.textContent = text;
  }

  // Nhãn phụ trên biển tên khu (ví dụ "Cần 30 HP"); text rỗng thì ẩn.
  function setBadge(zone, text) {
    const el = root && root.querySelector('#zl-badge-' + zone);
    if (!el) return;
    el.textContent = text || '';
    el.classList.toggle('hidden', !text);
  }

  // Đặt nhân vật ngay trước cửa khu (không đi bộ), cập nhật camera.
  function teleport(zone) {
    const z = ZONES[zone];
    if (!z || !me) return;
    path = null;
    pathZone = null;
    hideMarker();
    [me.x, me.y] = NODES[z.node];
    me.dir = 'down';
    me.walking = false;
    moving = false;
    drawMe();
    afterMove();
    savePos();
    try { localStorage.setItem('city.at', zone); } catch (e) { /* bỏ qua */ }
  }

  // Khoá / mở điều khiển. Khi khoá nhân vật đứng yên (nằm nếu ảnh nhân vật hỗ trợ).
  function setLocked(on) {
    locked = !!on;
    keys.clear();
    stick.x = stick.y = 0;
    if (locked) {
      path = null;
      pathZone = null;
      if (world) hideMarker();
    }
    if (overlay) overlay.classList.toggle('locked', locked);
    if (me) {
      me.walking = false;
      drawMe();
      if (overlay) updateNear();
    }
  }

  // Toạ độ x của khu, để đặt bảng thông tin không che khu đó.
  const zoneX = (zone) => NODES[ZONES[zone].node][0];

  window.City = {
    ZONES, mount, walkTo, setLook, setTag, setSub, setBadge, teleport, setLocked, zoneX,
    position: () => me && [me.x, me.y],
  };
})();
