// Bản đồ DCity: lưới phố pixel art (ảnh Kenney CC0 trong /img/city/), mỗi ô phố là một khu (game / tiện ích / trang trí).
// Nền tĩnh (mặt đất, đường, sông, cầu, toà nhà, đồ vật) vẽ một lần lên <canvas>; xe, nước gợn, biển tên khu và nhân vật
// là phần tử HTML đặt chồng lên. Không vẽ hình minh hoạ bằng SVG (xem docs/ART.md).
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

  const IMG = '/img/city/';
  const PX = 2; // ảnh pixel phóng to 2 lần
  const T = 16 * PX; // một ô lưới = 32 đơn vị bản đồ
  const COLS = 50;
  const ROWS = 32;
  const W = COLS * T; // 1600
  const H = ROWS * T; // 1024

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
  const FOUNTAIN = [BLOCKS.plaza[0] + 4, BLOCKS.plaza[1] + 3, 5, 3];
  const BARN = [BLOCKS.farm[0] + 8, BLOCKS.farm[1] + 2, 3, 3];
  const HOSPITAL = [1, 22, 12, 8]; // ảnh 384×256 ở cột 1–12, cửa giữa đáy (x 224, y 960); sân trước hàng 30–31

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
  const SPRITE_SCALE = 3; // nhân vật 16px × 3 = 48 đơn vị, to hơn ô lưới một chút cho dễ nhìn
  const HURT_MS = 1500;
  const DOOR_RADIUS = T * 1.6; // đứng trong bán kính này quanh cửa thì gợi ý "vào khu"

  // ---------- Lưới đi được ----------

  const inRect = (c, r, [rc, rr, rw, rh]) => c >= rc && c < rc + rw && r >= rr && r < rr + rh;

  // Ô (c, r) có đi được không: đường, vỉa hè, cầu, công viên ven sông và các khu bờ Nam (trừ đài phun, nhà kho);
  // nước sông và toà nhà bờ Bắc thì không.
  function cellWalkable(c, r) {
    if (c < 0 || r < 0 || c >= COLS || r >= ROWS) return false;
    if (RIVER_ROWS.includes(r)) return onBridge(c);
    if (r < 8) return WALK_COLS.includes(c) || inRoadCol(c); // bờ Bắc: chỉ vỉa hè / đường giữa các toà nhà
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

  const images = {};
  // Lọc theo manifest.json: chỉ nạp ảnh đã có; manifest lỗi thì thử nạp hết.
  async function loadAvailable(names) {
    try {
      const m = await (await fetch(IMG + 'manifest.json')).json();
      const have = new Set((m.images || []).map((it) => it.file.replace(/\.png$/, '')));
      names = names.filter((n) => have.has(n));
    } catch (e) { /* bỏ qua */ }
    return loadImages(names);
  }
  function loadImages(names) {
    return Promise.all(names.map((n) => new Promise((resolve) => {
      if (images[n]) return resolve();
      const img = new Image();
      img.onload = () => { images[n] = img; resolve(); };
      img.onerror = () => resolve(); // thiếu ảnh thì bỏ qua, không vẽ thay bằng hình khối
      img.src = IMG + n + '.png';
    })));
  }

  // Bộ sinh số giả ngẫu nhiên cố định để cỏ lần nào cũng như nhau.
  function rng(seed) {
    let s = seed;
    return () => ((s = (s * 1103515245 + 12345) % 2147483648) / 2147483648);
  }

  // ---------- Bản đồ tĩnh trên canvas ----------

  // Ảnh nền của từng ô lưới.
  function groundTile(c, r, rand) {
    const roadR = inRoadRow(r);
    const roadC = inRoadCol(c);
    const walkR = WALK_ROWS.includes(r);
    const walkC = WALK_COLS.includes(c);
    if (roadR && roadC) return 'road_asphalt';
    if (roadR) {
      if (walkC) return 'crosswalk_v';
      return r === 10 || r === 19 ? 'road_dash_h' : 'road_asphalt';
    }
    if (roadC) {
      if (walkR) return 'crosswalk_h';
      return c === 16 || c === 34 ? 'road_dash_v' : 'road_asphalt';
    }
    if (walkC && RIVER_ROWS.includes(r)) return 'paving_light'; // vỉa hè trên cầu
    if (walkR || walkC) return 'paving_grey';
    if (RIVER_ROWS.includes(r)) return 'river';
    return rand() < 0.22 ? 'ground_grass_2' : 'ground_grass';
  }

  // Nền riêng của từng khu (vẽ đè lên cỏ).
  function blockGround(id, c, r) {
    switch (id) {
      case 'plaza':
      case 'shop':
        return 'paving_beige';
      case 'hospital':
        return c === 0 || c === 13 ? null : 'paving_light';
      case 'farm':
        return r % 2 === 1 && c > 37 && c < 45 && r > 24 ? 'ground_dirt' : 'ground_grass_2';
      default:
        return null;
    }
  }

  // Đồ vật: [ảnh, x giữa đáy, y đáy] theo đơn vị bản đồ; vẽ theo thứ tự y để vật ở trước che vật ở sau.
  function props() {
    const list = [];
    const at = (img, x, y) => list.push([img, x, y]);
    const B = (id) => BLOCKS[id].map((v) => v * T);

    // Toà nhà bờ Bắc, cửa ở đáy giữa ảnh.
    const [cx, cy, cw, ch] = B('casino');
    at('building_casino', cx + cw / 2, cy + ch);
    const [bx, by, bw, bh] = B('bank');
    at('building_bank', bx + bw / 2, by + bh);
    const [sx, sy, sw, sh] = B('shop');
    at('building_shop', sx + sw / 2, sy + sh);
    at('planter_teal', sx + 24, sy + sh);
    at('planter_orange', sx + sw - 24, sy + sh);
    at('planter_green', bx + 20, by + bh);
    at('planter_green', bx + bw - 20, by + bh);

    // Vỉa hè: đèn đường, thùng rác, trụ cứu hoả.
    for (const x of [96, 400, 688, 944, 1248, 1536]) {
      at('lamp_street', x, 9 * T - 2);
      at('lamp_street', x, 22 * T - 2);
    }
    at('bin', 320, 9 * T - 4);
    at('hydrant', 1000, 9 * T - 4);
    at('bin', 1480, 22 * T - 4);
    at('hydrant', 464, 22 * T - 4);

    // Vỉa hè ven sông: chậu cây, bụi, ghế (đồ thấp để không che mặt đường), chừa chỗ hai cây cầu.
    const free = (x) => !(x > 13 * T && x < 19 * T) && !(x > 31 * T && x < 37 * T);
    const treeKinds = ['planter_green', 'planter_teal', 'bush_green', 'planter_orange', 'planter_green', 'bush_teal'];
    let k = 0;
    for (let x = 48; x < W; x += 112) {
      if (!free(x) || !free(x + 56)) continue;
      at(treeKinds[k++ % treeKinds.length], x, 13 * T - 6);
      at(treeKinds[(k + 2) % treeKinds.length], x + 56, 18 * T - 6);
    }
    for (const x of [200, 760, 1300]) {
      at('bench', x, 13 * T - 2);
      at('bench', x + 60, 18 * T - 2);
    }
    for (const x of [376, 960, 1520]) {
      at('bush_green', x, 13 * T - 2);
      at('bush_teal', x - 40, 18 * T - 2);
    }

    // Quảng trường: đài phun nước ở giữa, ghế, chậu cây, cây ở bốn góc.
    const [px0, py0, pw, ph] = B('plaza');
    at('bench', px0 + 2 * T, py0 + 5 * T);
    at('bench', px0 + pw - 2 * T, py0 + 5 * T);
    at('bench', px0 + pw / 2, py0 + ph - 8);
    at('planter_orange', px0 + 3 * T, py0 + 3 * T);
    at('planter_teal', px0 + pw - 3 * T, py0 + 3 * T);
    at('tree_green', px0 + T, py0 + 2 * T);
    at('tree_teal', px0 + pw - T, py0 + 2 * T);
    at('tree_small_green', px0 + T, py0 + ph - 4);
    at('tree_small_teal', px0 + pw - T, py0 + ph - 4);
    at('food_cart', px0 + 3 * T, py0 + 7 * T);

    // Bệnh viện: toà nhà (ảnh building_hospital nếu có, chưa có thì tạm ghép hai nhà sáng màu), xe cứu thương, ghế chờ.
    const hx = (HOSPITAL[0] + HOSPITAL[2] / 2) * T;
    const hy = (HOSPITAL[1] + HOSPITAL[3]) * T;
    if (images.building_hospital) at('building_hospital', hx, hy);
    else {
      at('house_grey', hx - 80, hy);
      at('house_grey', hx + 80, hy);
    }
    if (images.ambulance_side) at('ambulance_side', hx + 5 * T, hy + 58);
    at('bench', hx - 4 * T, hy + 54);
    at('planter_green', hx - 2 * T, hy + 12);
    at('planter_green', hx + 2 * T, hy + 12);
    at('tree_small_green', T / 2 + 4, (BLOCKS.hospital[1] + 3) * T);
    at('tree_small_teal', 13 * T + T / 2 - 4, (BLOCKS.hospital[1] + 3) * T);

    // Nông trại: nhà kho, hàng rào gỗ, thùng nông sản dọc các luống đất.
    const [fx, fy] = B('farm');
    at('building_barn', (BARN[0] + BARN[2] / 2) * T, (BARN[1] + BARN[3]) * T);
    for (let x = fx + 16; x < fx + 7 * T; x += 80) at('fence_wood', x + 40, fy + T - 2);
    const crates = ['crate_tomato', 'crate_greens', 'crate_fruit', 'crate_veg'];
    for (let i = 0; i < 4; i++) at(crates[i], fx + (2 + i * 2) * T, fy + 8 * T - 4);
    at('tree_orange', fx + 12 * T + 16, fy + 7 * T);
    at('bush_orange', fx + 11 * T, fy + 7 * T + 20);

    return list.sort((a, b) => a[2] - b[2]);
  }

  // Tên mọi ảnh cần nạp.
  const ALL_IMAGES = [
    'ground_grass', 'ground_grass_2', 'ground_dirt', 'ground_gravel', 'paving_grey', 'paving_beige', 'paving_light',
    'road_asphalt', 'road_dash_h', 'road_dash_v', 'crosswalk_h', 'crosswalk_v', 'road_manhole', 'river_strip',
    'water_tl', 'water_t', 'water_tr', 'water_l', 'water_c', 'water_r', 'water_bl', 'water_b', 'water_br',
    'tree_green', 'tree_orange', 'tree_teal', 'tree_small_green', 'tree_small_teal', 'planter_green', 'planter_orange', 'planter_teal',
    'bush_green', 'bush_orange', 'bush_teal', 'lamp_street', 'lamp_small', 'bench', 'fence_wood', 'fence_chain',
    'crate_tomato', 'crate_greens', 'crate_fruit', 'crate_veg', 'food_cart', 'bin', 'hydrant', 'house_grey',
    'building_casino', 'building_bank', 'building_shop', 'building_barn', 'bridge_rail_v', 'bridge_post',
    // Có thể chưa có (agent bổ sung hình đang làm): thiếu thì bỏ qua.
    'building_hospital', 'ambulance_side',
  ];

  function drawStatic(canvas) {
    const g = canvas.getContext('2d');
    g.imageSmoothingEnabled = false;
    const tile = (name, c, r) => {
      if (name === 'river') {
        const img = images.river_strip;
        if (img) g.drawImage(img, 0, (r - RIVER_ROWS[0]) * 16, 16, 16, c * T, r * T, T, T);
        return;
      }
      if (images[name]) g.drawImage(images[name], c * T, r * T, T, T);
    };
    const rand = rng(7);
    for (let r = 0; r < ROWS; r++) for (let c = 0; c < COLS; c++) tile(groundTile(c, r, rand), c, r);
    for (const [c, r] of [[6, 9], [26, 11], [44, 18], [10, 20], [16, 4], [34, 28]]) tile('road_manhole', c, r);

    for (const [id, [bc, br, bw, bh]] of Object.entries(BLOCKS)) {
      for (let r = br; r < br + bh; r++) for (let c = bc; c < bc + bw; c++) {
        const name = blockGround(id, c, r);
        if (name) tile(name, c, r);
      }
    }

    // Đài phun nước: hồ ghép từ 9 mảnh nước có viền.
    const [fc, fr, fw, fh] = FOUNTAIN;
    for (let r = 0; r < fh; r++) for (let c = 0; c < fw; c++) {
      const v = r === 0 ? 't' : r === fh - 1 ? 'b' : '';
      const h = c === 0 ? 'l' : c === fw - 1 ? 'r' : '';
      tile('water_' + (v + h || 'c'), fc + c, fr + r);
    }

    // Lan can hai mép cầu (cột vỉa hè ngoài cùng), trụ ở đầu cầu.
    for (const c of [14, 18, 32, 36]) {
      for (const r of RIVER_ROWS) tile('bridge_rail_v', c, r);
      tile('bridge_post', c, 12);
      tile('bridge_post', c, 17);
    }

    for (const [name, x, y] of props()) {
      const img = images[name];
      if (!img) continue;
      const w = img.width * PX;
      const h = img.height * PX;
      g.drawImage(img, Math.round(x - w / 2), Math.round(y - h), w, h);
    }
  }

  // ---------- Phần động (HTML) ----------

  const px = (n) => `${Math.round(n)}px`;
  const spriteImg = (name) => `<img class="pix" src="${IMG}${name}.png" alt="" draggable="false">`;

  // Nước gợn: các đoạn sông giữa hai cây cầu, nền ảnh dòng sông trôi dần; mặt hồ đài phun cũng gợn.
  function waterHtml() {
    const segs = [[0, 14], [19, 32], [37, COLS]];
    const [fc, fr, fw, fh] = FOUNTAIN;
    return segs
      .map(([a, b]) => `<div class="river-flow" style="left:${px(a * T)};top:${px(RIVER_ROWS[0] * T)};width:${px((b - a) * T)};height:${px(4 * T)}"></div>`)
      .join('') +
      `<div class="fountain-water" style="left:${px((fc + 1) * T)};top:${px((fr + 1) * T)};width:${px((fw - 2) * T)};height:${px((fh - 2) * T)}"></div>`;
  }

  // Xe chạy vòng: đường ngang dùng ảnh nhìn ngang, đường dọc dùng ảnh nhìn đầu / đuôi xe.
  // Vị trí tính bằng JS (không dùng CSS animation) để phát hiện va chạm với nhân vật.
  const CARS = [
    { img: 'car_green_side', dir: 'east', y: roadCenter(11), speed: 115, at: 0 },
    { img: 'car_grey_side', dir: 'west', y: roadCenter(9), speed: 95, at: 700 },
    { img: 'car_orange_side', dir: 'east', y: roadCenter(20), speed: 130, at: 400 },
    { img: 'car_green_side', dir: 'west', y: roadCenter(18), speed: 90, at: 1200 },
    { img: 'car_orange_front', dir: 'south', x: roadCenter(15), speed: 105, at: 300 },
    { img: 'car_grey_back', dir: 'north', x: roadCenter(17), speed: 100, at: 800 },
    { img: 'car_green_front', dir: 'south', x: roadCenter(33), speed: 90, at: 100 },
    { img: 'car_orange_back', dir: 'north', x: roadCenter(35), speed: 120, at: 600 },
  ];
  // Hộp va chạm quanh tâm xe (đơn vị bản đồ, nhỏ hơn hình một chút): [nửa rộng, trên, dưới].
  // Hình xe vẽ lệch lên trên tâm (translate -60%) nên phần trên dài hơn phần dưới.
  const CAR_BOX = { h: [30, 24, 14], v: [16, 30, 18] };
  const YIELD_DIST = 150; // xe dừng nhường khi nhân vật đi tự động băng qua đường phía trước trong khoảng này
  const carHorizontal = (car) => car.dir === 'east' || car.dir === 'west';

  function trafficHtml() {
    return CARS.map((car, i) => `<div class="car car-${car.dir}" data-car="${i}">${spriteImg(car.img)}</div>`).join('');
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
    { img: 'boat_motor', from: 1 * T, to: 13 * T, y: 14 * T + 24, speed: 60, at: 0 },
    { img: 'boat_row', from: 20 * T, to: 31 * T, y: 15 * T + 20, speed: 28, at: 120 },
    { img: 'boat_motor', from: 38 * T, to: 49 * T, y: 14 * T + 30, speed: 50, at: 260 },
  ];
  function boatsHtml() {
    return BOATS.map((b, i) => `<div class="boat" data-boat="${i}">${spriteImg(b.img)}</div>`).join('');
  }

  // Biển neon CASINO đè lên biển hiệu trên mái (ảnh bật / tắt xen kẽ).
  function signHtml() {
    return `<div class="casino-sign" style="left:${px(224)};top:${px(140)}">${spriteImg('sign_casino_off')}<span class="sign-on">${spriteImg('sign_casino')}</span></div>`;
  }

  // Vùng bấm của từng khu và biển tên (HTML).
  function zonesHtml() {
    const labelAt = { casino: [224, 10], bank: [816, 10], shop: [1392, 10], plaza: [690, 712], hospital: [224, 712], farm: [1240, 712] };
    return Object.entries(ZONES)
      .map(([id, z]) => {
        const [c, r, w, h] = BLOCKS[id];
        const [lx, ly] = labelAt[id];
        return `<div class="zone${z.soon ? ' soon' : ''}" data-zone="${id}" style="left:${px(c * T)};top:${px(r * T)};width:${px(w * T)};height:${px(h * T)}"></div>
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
      me.body.innerHTML = C.sprite(look, { dir: me.dir, walking: me.walking && !locked, scale: SPRITE_SCALE, pose: locked ? 'lie' : undefined, hurt });
    } else if (C && typeof C.svg === 'function') {
      // Tạm dùng hình nhân vật cũ cho tới khi có ảnh nhân vật pixel (Character.sprite).
      me.body.innerHTML = `<div class="me-fallback">${C.svg(look, { walking: me.walking })}</div>`;
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
      car.el.style.transform = `translate(${x}px, ${y}px)`;
      car.el.style.zIndex = String(100 + Math.round(y));
      // Đi tự động thì xe đã nhường, không tính va chạm (tránh bị gián đoạn khi đi bằng thanh đi nhanh).
      if (!me || locked || path || now < hurtUntil) return;
      const [hw, up, down] = carHorizontal(car) ? CAR_BOX.h : CAR_BOX.v;
      // Hộp nhân vật: rộng ±10 quanh chân, cao 28 phía trên chân.
      if (Math.abs(me.x - x) < hw + 10 && me.y > y - up && me.y - 28 < y + down) hit('car', car);
    });
    BOATS.forEach((b, i) => {
      b.at += b.speed * dt;
      const span = b.to - b.from;
      const k = b.at % (2 * span);
      const x = k < span ? b.from + k : b.to - (k - span);
      b.el = b.el || world.querySelector(`[data-boat="${i}"]`);
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
    scale = cw < 860 ? ch / H : Math.min(cw / W, ch / H);
    const stage = root.querySelector('.city-stage');
    stage.style.width = px(W * scale);
    stage.style.height = px(H * scale);
    world.style.transform = `scale(${scale})`;
    root.style.backgroundSize = `${T * scale}px ${T * scale}px`;
  }

  function mount(el, o) {
    opts = o;
    root = el;
    el.classList.add('city-pixel');
    el.style.backgroundImage = `url(${IMG}ground_grass.png)`;
    el.innerHTML = `<div class="city-stage">
      <div class="city-world" style="width:${px(W)};height:${px(H)}" role="img" aria-label="Bản đồ DCity">
        <canvas class="city-canvas" width="${W}" height="${H}"></canvas>
        ${waterHtml()}${boatsHtml()}${signHtml()}${trafficHtml()}${zonesHtml()}
        <div class="walk-target hidden"></div>
        <div id="me" class="me">
          <div id="me-tag" class="me-tag"><span class="tag-name"></span><span class="tag-lv"></span></div>
          <div class="me-body"></div>
        </div>
      </div>
    </div>`;
    world = el.querySelector('.city-world');
    loadAvailable(ALL_IMAGES).then(() => drawStatic(el.querySelector('.city-canvas')));

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

    // Bấm toà nhà / biển tên: đi tới cửa khu; bấm chỗ khác: đi tới điểm đó.
    el.querySelectorAll('[data-zone]').forEach((z) => z.addEventListener('click', (e) => {
      e.stopPropagation();
      walkTo(z.dataset.zone);
    }));
    world.addEventListener('click', (e) => {
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
