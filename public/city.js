// Bản đồ DCity phong cách hoạt hình (giống game Avatar): ảnh trong /img/toon/city/ và /img/toon/farm/ (CC0, xem docs/ART.md).
// Mặt đất (cỏ, đường, vỉa hè, bờ sông, cầu, ruộng) vẽ lên <canvas> theo từng mảng; trời + đồi xa ở dải phía trên; toà nhà,
// cây, đèn, gia súc, xe, thuyền, biển tên khu và nhân vật là ảnh HTML xếp lớp theo chiều sâu (y) để nhân vật đi trước / sau đúng.
// Không vẽ hình minh hoạ bằng SVG.
//
// Thế giới không có mép: hai bên thành phố là ngoại ô sinh theo mảng (đường, sông, nhà, ruộng... lặp có biến đổi) chạy mãi,
// phía dưới là đồng cỏ. Camera đi theo nhân vật (mượt, có vùng chết), kéo chuột / vuốt để nhìn quanh; nhân vật đi tiếp ra
// ngoại ô được một đoạn trên đường / vỉa hè ngang và theo hai trục dọc xuống đồng cỏ.
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
  const MB = 900; // độ sâu dải đồng cỏ phía dưới thành phố (camera không xuống quá)
  const EXT = 50; // nhân vật đi tiếp ra ngoại ô tối đa EXT ô mỗi bên (trên đường / vỉa hè ngang)
  // Lưới đi được: cột GC0 … GC1 − 1, hàng 0 … GR1 − 1 (dưới thành phố theo hai trục dọc xuống đường đất ở đồng cỏ).
  const GC0 = -EXT;
  const GC1 = COLS + EXT;
  // Đường đất ngang ở đồng cỏ (hàng 41–42) chạy suốt mọi mảng, nối hai trục dọc của thành phố.
  const COUNTRY = [41, 42];
  const GR1 = COUNTRY[1] + 1;
  const GW = GC1 - GC0;
  const FAR = EXT * T + 4000; // xe, sông, thuyền chạy tới tận đây ở hai bên (xa hơn mọi khung nhìn)
  const NAV_PAD = 84; // px màn hình dành cho thanh đi nhanh ở đáy (bản đồ vẫn vẽ tiếp bên dưới thanh)

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
  // Vật cản trong các khu bờ Nam (cột, hàng, rộng, cao): đài phun nước, toà bệnh viện, bên trong hàng rào nông trại.
  const FOUNTAIN = [23, 27, 6, 3];
  const HOSPITAL = [4, 24, 6, 6]; // toà thu 0.8 (194×192), cửa giữa đáy (x 224, y 966); sân trước hàng 30–31
  // Nông trại là khu có hàng rào: hàng rào chạy ngang ở y FARM_FENCE_Y, cổng gỗ giữa (x FARM_GATE_X); bên trong
  // (ruộng, chuồng, gia súc) chỉ để nhìn — vào chơi qua cổng. Hàng 22–24 trước hàng rào là sân đất đi được.
  const FARM_FENCE_Y = 820;
  const FARM_GATE_X = 1440;
  const FARM_IN = [37, 25, 13, 7];
  const FARM_GATE_COLS = [43, 46]; // ô hàng 25 ngay dưới cổng vẫn đi được (bước vào khung cổng)

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
    casino: [224, 276], bank: [816, 276], shop: [1452, 276], // cửa Shop lệch phải trong ảnh toà nhà
    plaza: [816, 780], hospital: [224, 994], farm: [FARM_GATE_X, 790],
  };
  const SPEED = 240; // đơn vị bản đồ / giây
  const SPRITE_SIZE = 96; // chiều cao nhân vật chibi (khung 96×148, neo chân giữa đáy)
  const HURT_MS = 1500;
  const DOOR_RADIUS = T * 1.6; // đứng trong bán kính này quanh cửa thì gợi ý "vào khu"

  // ---------- Lưới đi được ----------

  const inRect = (c, r, [rc, rr, rw, rh]) => c >= rc && c < rc + rw && r >= rr && r < rr + rh;

  // Ô (c, r) có đi được không: đường, vỉa hè, cầu, công viên ven sông và các khu bờ Nam (trừ đài phun, toà bệnh viện,
  // bên trong hàng rào nông trại); nước sông và toà nhà bờ Bắc thì không. Ngoại ô: chỉ đường / vỉa hè ngang (hai bên),
  // hai trục dọc xuống đồng cỏ và đường đất ngang ở đồng cỏ.
  function cellWalkable(c, r) {
    if (c < GC0 || r < 0 || c >= GC1 || r >= GR1) return false;
    if (r >= ROWS) {
      if (COUNTRY.includes(r)) return true;
      return c >= 0 && c < COLS && r < COUNTRY[0] && (WALK_COLS.includes(c) || inRoadCol(c));
    }
    if (c < 0 || c >= COLS) return WALK_ROWS.includes(r) || inRoadRow(r);
    if (RIVER_ROWS.includes(r)) return onBridge(c);
    if (r < 8) return r >= 5 && (WALK_COLS.includes(c) || inRoadCol(c)); // bờ Bắc: chỉ vỉa hè / đường giữa các toà nhà, tới chân đồi
    if (inRect(c, r, FOUNTAIN) || inRect(c, r, HOSPITAL)) return false;
    if (inRect(c, r, FARM_IN) && !(r === FARM_IN[1] && c >= FARM_GATE_COLS[0] && c <= FARM_GATE_COLS[1])) return false;
    return true;
  }
  const walkable = (x, y) => cellWalkable(Math.floor(x / T), Math.floor(y / T));
  // Chân nhân vật chiếm một khoảng nhỏ: cả bốn điểm quanh chân phải đi được.
  const footFree = (x, y) => walkable(x - 9, y - 4) && walkable(x + 9, y - 4) && walkable(x - 9, y + 3) && walkable(x + 9, y + 3);

  const cellCenter = (c, r) => [c * T + T / 2, r * T + T / 2];

  // Ô đi được gần nhất với một điểm (tìm loang theo vòng).
  function nearestWalkableCell(x, y) {
    const c0 = Math.max(GC0, Math.min(GC1 - 1, Math.floor(x / T)));
    const r0 = Math.max(0, Math.min(GR1 - 1, Math.floor(y / T)));
    if (cellWalkable(c0, r0)) return [c0, r0];
    for (let d = 1; d < GW; d++) {
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
    const key = (c, r) => r * GW + (c - GC0);
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
        while (prev.has(cur)) { cur = prev.get(cur); path.unshift([(cur % GW) + GC0, Math.floor(cur / GW)]); }
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

  // ---------- Mặt đất trên canvas (theo mảng) ----------

  // Thế giới chia thành các mảng rộng W theo chiều ngang: mảng 0 là thành phố, các mảng khác là ngoại ô sinh ngẫu nhiên
  // cố định theo số mảng (lặp có biến đổi, chạy mãi hai bên). Mỗi mảng có một canvas mặt đất (từ dải trời tới hết đồng cỏ
  // phía dưới) và một lớp ảnh đồ vật; chỉ giữ các mảng quanh khung nhìn.
  const GROUND_IMAGES = [
    'grass_tile', 'road_h', 'road_v', 'road_plain', 'road_crosswalk_h', 'road_crosswalk_v', 'sidewalk_tile', 'water_tile',
    'river_bank_top', 'river_bank_bottom', 'bridge_deck', 'bridge_rail_v', 'farm/bg_sky_hills', 'farm/soil_plot',
    'farm/soil_plot_wet', 'farm/dirt_path_tile', 'farm/grass_plot',
  ];

  // Số ngẫu nhiên cố định theo hạt giống (cùng mảng luôn ra cùng cảnh).
  function rng(seed) {
    let s = (Math.imul(seed | 0, 2654435761) ^ 0x9e3779b9) >>> 0 || 1;
    return () => {
      s ^= s << 13; s >>>= 0;
      s ^= s >>> 17;
      s ^= s << 5; s >>>= 0;
      return s / 4294967296;
    };
  }
  const BLEED = 3;
  const pick = (r, list) => list[Math.floor(r() * list.length) % list.length];
  const between = (r, a, b) => a + (b - a) * r();
  const widthOf = (name, k) => (sizes[name] ? sizes[name][0] : 120) * k;
  // Không nằm trên hai trục đường dọc (và vỉa hè hai bên) của thành phố, cách thêm pad.
  const offRoad = (x, pad) => ![[14, 19], [32, 37]].some(([a, b]) => x > a * T - pad && x < b * T + pad);

  const HOUSES = ['house_1', 'house_2', 'house_3', 'house_4', 'house_5', 'house_6', 'house_7', 'house_8'];
  const TREES = ['tree_1', 'tree_big', 'farm/tree_big', 'farm/tree_med', 'tree_1', 'farm/tree_round_1'];
  const TREE_K = { tree_1: [0.6, 0.8], tree_big: [0.5, 0.68], 'farm/tree_big': [0.45, 0.6], 'farm/tree_med': [0.6, 0.8], 'farm/tree_round_1': [0.6, 0.8] };
  const BUSHES = ['bush_1', 'farm/bush_1', 'bush_3', 'farm/bush_2', 'bush_2', 'farm/bush_3'];
  const CROPS = ['corn', 'tomato', 'carrot', 'watermelon', 'strawberry', 'rice'];
  const ANIMALS = ['farm/animal_cow', 'farm/animal_sheep', 'farm/animal_pig', 'farm/animal_horse', 'farm/animal_chicken', 'farm/animal_chicken_white'];

  // Kế hoạch một mảng: ô đất vẽ lên canvas và danh sách ảnh đồ vật.
  function chunkPlan(kx) {
    const r = rng(kx * 7919 + 101);
    const x0 = kx * W;
    const plan = { plots: [], props: [] };
    const at = (img, x, y, k = 1, flip = false) => plan.props.push([img, x, y, k, flip, '']);
    const tree = (x, y, scale = 1) => {
      const t = pick(r, TREES);
      const [a, b] = TREE_K[t] || [0.6, 0.8];
      at(t, x, y, between(r, a, b) * scale, r() < 0.5);
    };

    // Một lô đất rộng lw từ x = lx, đồ vật đứng trong khoảng y0 … y1 (y là chân đồ vật).
    function lot(type, lx, lw, y0, y1) {
      if (type === 'houses') {
        const n = lw > 330 ? 2 : 1;
        for (let i = 0; i < n; i++) {
          const h = pick(r, HOUSES);
          const k = between(r, 0.6, 0.74);
          const cx = lx + (lw * (i + 0.5)) / n;
          at(h, cx, between(r, Math.min(y1, y0 + 150), y1), k, r() < 0.4);
          if (r() < 0.7) at('farm/flowers', cx + widthOf(h, k) / 2 - 6, y1 - between(r, 0, 20), 0.48);
        }
        if (r() < 0.8) tree(lx + between(r, 10, lw - 10), between(r, y0 + 40, y0 + 90));
      } else if (type === 'field') {
        const cols = Math.max(2, Math.floor((lw - 30) / 62));
        const rows = Math.max(1, Math.min(3, Math.floor((y1 - y0 - 10) / 62)));
        const top = between(r, y0, y1 - rows * 62);
        const crop = pick(r, CROPS);
        for (let i = 0; i < cols; i++) for (let j = 0; j < rows; j++) {
          const px0 = lx + 15 + i * 62;
          const py0 = top + j * 62;
          plan.plots.push([px0, py0, (i + j) % 2 === 1]);
          if (r() < 0.8) at(`farm/crop_${crop}_${pick(r, ['ripe', '2', 'ripe'])}`, px0 + 29, py0 + 50, 0.46);
        }
        if (r() < 0.5) at(pick(r, ['farm/hay', 'farm/wheelbarrow']), lx + lw - 20, Math.min(y1, top + rows * 62 + 14), 0.5);
      } else if (type === 'grove') {
        const n = 3 + Math.floor(r() * 4);
        for (let i = 0; i < n; i++) tree(lx + between(r, 20, lw - 20), between(r, y0 + 30, y1));
        for (let i = 0; i < 3; i++) at(pick(r, BUSHES), lx + between(r, 10, lw - 10), between(r, y0 + 20, y1), 0.7);
      } else if (type === 'pond') {
        at('farm/pond', lx + lw / 2, y1 - between(r, 0, 20), Math.min(0.75, lw / 380));
        at(pick(r, BUSHES), lx + 10, y1 - 30, 0.7);
        if (r() < 0.6) tree(lx + lw - 10, between(r, y0 + 50, y1 - 60));
      } else if (type === 'pasture') {
        // Bãi chăn thả: hàng rào thấp phía trên, vài con vật, rơm.
        const fy = y0 + between(r, 20, 50);
        for (let fx = lx + 30; fx < lx + lw - 30; fx += 59) at('farm/fence_piece', fx, fy, 0.9);
        const n = 2 + Math.floor(r() * 3);
        for (let i = 0; i < n; i++) at(pick(r, ANIMALS), lx + between(r, 30, lw - 30), between(r, fy + 70, y1), 0.5, r() < 0.5);
        if (r() < 0.6) at('farm/hay', lx + between(r, 30, lw - 30), between(r, fy + 60, y1), 0.5);
      } else {
        // Bãi cỏ: vài bụi, hoa, một hai cây, đôi khi gia súc thả rông.
        for (let i = 0; i < 4; i++) {
          const x = lx + between(r, 10, lw - 10);
          const y = between(r, y0 + 30, y1);
          const roll = r();
          if (roll < 0.35) tree(x, y);
          else if (roll < 0.7) at(pick(r, BUSHES), x, y, between(r, 0.6, 0.9));
          else if (roll < 0.85) at('farm/flowers', x, y, 0.5);
          else at(pick(r, ANIMALS), x, y, 0.5, r() < 0.5);
        }
      }
    }
    // Rải các lô nối tiếp trong khoảng xa … xb (lô không vượt ra ngoài mảng để ô đất không bị cắt).
    function band(xa, xb, y0, y1, types) {
      let lx = xa + between(r, 0, 30);
      while (lx < xb - 120) {
        const lw = Math.min(between(r, 240, 420), xb - 10 - lx);
        lot(pick(r, types), lx, lw, y0, y1);
        lx += lw + between(r, 12, 40);
      }
    }

    // Chân đồi: bụi / cây nhỏ lô nhô, không xếp thẳng hàng.
    for (let x = x0 + between(r, 10, 50); x < x0 + W; x += between(r, 64, 120)) {
      const y = HORIZON + between(r, 0, 16);
      if (r() < 0.22) at(pick(r, ['tree_3', 'tree_4', 'farm/tree_round_3']), x, y, between(r, 1, 1.4));
      else at(pick(r, ['bush_1', 'farm/bush_1', 'bush_3', 'farm/bush_2']), x, y, between(r, 0.6, 0.8));
    }

    if (kx === 0) {
      plan.props.push(...props());
    } else {
      // Bờ Bắc: dãy nhà dân hướng ra vỉa hè, xen cây và chậu hoa.
      let x = x0 + between(r, 0, 40);
      while (x < x0 + W - 50) {
        if (r() < 0.64) {
          const h = pick(r, HOUSES);
          const k = between(r, 0.62, 0.78);
          const w = widthOf(h, k);
          at(h, x + w / 2, 8 * T, k, r() < 0.3);
          if (r() < 0.5) at('farm/flowers', x + w + 8, 8 * T + 4, 0.5);
          x += w + between(r, 18, 56);
        } else {
          const w = between(r, 60, 110);
          tree(x + w / 2, 8 * T + 2);
          x += w + between(r, 4, 30);
        }
      }
      // Đèn đường hai bên đường ngang, ghế + cây ven sông.
      for (let i = 0; i < 5; i++) {
        at('lamp', x0 + 120 + i * 320, 9 * T - 2, 0.75);
        at('lamp', x0 + 150 + i * 320, 22 * T - 2, 0.75);
      }
      const kinds = ['tree_4', 'bush_2', 'tree_3', 'bush_3', 'tree_4', 'bush_4'];
      for (const y of [13 * T - 4, 18 * T - 4]) {
        for (let xx = x0 + between(r, 10, 60); xx < x0 + W; xx += between(r, 80, 140)) {
          if (r() < 0.14) { at('bench', xx, y + 2, 0.6); continue; }
          const kind = pick(r, kinds);
          at(kind, xx, y, kind.startsWith('tree') ? between(r, 1.1, 1.4) : between(r, 0.8, 1.1));
        }
      }
      // Bờ Nam: các lô nhà, ruộng, vườn cây, bãi chăn thả nối tiếp nhau.
      band(x0, x0 + W, 760, 1012, ['houses', 'field', 'grove', 'pasture', 'houses', 'field']);
    }

    // Đồng cỏ phía dưới: dải trên (giữa thành phố và đường đất), dải dưới đường đất; ở thành phố chừa hai trục dọc.
    const lane0 = COUNTRY[0] * T;
    const lane1 = (COUNTRY[1] + 1) * T;
    const MEADOW = ['field', 'grove', 'pasture', 'meadow', 'houses', 'meadow', 'pond', 'field'];
    const top = kx === 0 ? [[x0, 14 * T - 12], [19 * T + 12, 32 * T - 12], [37 * T + 12, x0 + W]] : [[x0, x0 + W]];
    for (const [a, b] of top) band(a, b, H + 50, lane0 - 24, MEADOW);
    band(x0, x0 + W, lane1 + 70, lane1 + 300, MEADOW);
    band(x0, x0 + W, lane1 + 330, H + MB - 10, MEADOW);
    // Ven đường đất: bụi, hoa thưa.
    for (let x = x0 + between(r, 0, 80); x < x0 + W; x += between(r, 110, 240)) {
      if (kx === 0 && !offRoad(x, 20)) continue;
      at(r() < 0.3 ? 'farm/flowers' : pick(r, BUSHES), x, lane0 - 2, between(r, 0.5, 0.7));
    }
    return plan;
  }

  // Vẽ mặt đất một mảng lên canvas của nó (gốc toạ độ = góc trên-trái bản đồ, như các ảnh HTML).
  function drawChunk(canvas, kx, plan) {
    const g = canvas.getContext('2d');
    g.imageSmoothingEnabled = true;
    g.imageSmoothingQuality = 'high';
    // Lấp hình chữ nhật bằng ảnh lặp, phóng to / thu nhỏ theo k; gốc ảnh lặp (ox, oy) tính theo toạ độ bản đồ để
    // hai mảng cạnh nhau nối liền.
    const fill = (name, x, y, w, h, k = 1, ox = x, oy = y) => {
      const img = images[name];
      if (!img) return;
      const pat = g.createPattern(img, 'repeat');
      pat.setTransform(new DOMMatrix([k, 0, 0, k, ox, oy]));
      g.fillStyle = pat;
      g.fillRect(x, y, w, h);
    };
    const put = (name, x, y, w, h) => images[name] && g.drawImage(images[name], x, y, w, h);
    const x0 = kx * W - BLEED; // vẽ tràn BLEED mỗi bên, hai mảng chồng lên nhau để không lộ đường nối khi thu phóng
    const x1 = kx * W + W + BLEED;
    const ww = x1 - x0;
    g.save();
    g.translate(-x0, SKY);

    // Trời + đồi xa: chân ảnh ở đường chân đồi; mảng lẻ lật ngang để hai mảng cạnh nhau nối liền mạch.
    const sky = images['farm/bg_sky_hills'];
    if (sky) {
      const sh = sky.height * (W / sky.width);
      g.save();
      if (kx % 2) {
        g.translate(x1 + x0, 0);
        g.scale(-1, 1);
      }
      g.drawImage(sky, x0, HORIZON - sh, ww, sh);
      g.restore();
    }
    // Cỏ dưới chân đồi tới hết đồng cỏ.
    fill('grass_tile', x0, HORIZON, ww, H + MB - HORIZON, 0.5, 0, 0);

    // Vỉa hè và đường ngang chạy suốt mọi mảng; đường dọc, ngã tư, vạch qua đường chỉ ở thành phố.
    for (const r of WALK_ROWS) fill('sidewalk_tile', x0, r * T, ww, T, 0.5, 0, r * T);
    for (const g3 of ROAD_ROWS) fill('road_h', x0, g3[0] * T, ww, 3 * T, 0.75, 0, g3[0] * T);
    if (kx === 0) {
      const top = 5 * T; // đường dọc bờ Bắc bắt đầu từ chân đồi, bờ Nam chạy tiếp xuống tới đường đất ở đồng cỏ
      const end = COUNTRY[0] * T + 8;
      for (const c of WALK_COLS) fill('sidewalk_tile', c * T, top, T, end - top, 0.5);
      for (const g3 of ROAD_COLS) fill('road_v', g3[0] * T, top, 3 * T, end - top, 0.75);
      for (const r3 of ROAD_ROWS) for (const c3 of ROAD_COLS) fill('road_plain', c3[0] * T, r3[0] * T, 3 * T, 3 * T, 0.75);
      for (const r3 of ROAD_ROWS) for (const c of WALK_COLS) put('road_crosswalk_h', c * T - 14, r3[0] * T + 4, T + 28, 3 * T - 8);
      for (const c3 of ROAD_COLS) for (const r of [8, 21]) put('road_crosswalk_v', c3[0] * T + 4, r * T - 14, 3 * T - 8, T + 28);
    }

    // Sông: nền nước, bờ trên / dưới (mặt nước gợn là lớp HTML), cầu gỗ ở thành phố.
    const ry = RIVER_ROWS[0] * T;
    const rh = RIVER_ROWS.length * T;
    fill('water_tile', x0, ry, ww, rh, 0.75, 0, ry);
    fill('river_bank_top', x0, ry, ww, 36, 0.75, 0, ry);
    fill('river_bank_bottom', x0, ry + rh - 36, ww, 36, 0.75, 0, ry + rh - 36);
    if (kx === 0) {
      for (const [a, b] of [[14, 19], [32, 37]]) {
        fill('bridge_deck', a * T, ry - 6, (b - a) * T, rh + 12, 0.75);
        g.fillStyle = 'rgba(70, 40, 10, .25)';
        g.fillRect(a * T, ry + rh + 6, (b - a) * T, 6);
        fill('bridge_rail_v', a * T - 6, ry - 18, 20, rh + 30, 1);
        fill('bridge_rail_v', b * T - 14, ry - 18, 20, rh + 30, 1);
      }
      // Nền từng lô bờ Nam: quảng trường lát gạch cả lô; bệnh viện có sân lát gạch trước cửa nối ra vỉa hè;
      // nông trại có sân đất trước hàng rào, lối đất từ cổng vào chuồng và ruộng bên trong.
      const [pc, pr, pw, ph] = BLOCKS.plaza;
      fill('sidewalk_tile', pc * T, pr * T, pw * T, ph * T, 0.75);
      fill('sidewalk_tile', 40, 944, 14 * T - 40, H - 944, 0.75);
      const [fc, fr, fw] = BLOCKS.farm;
      fill('farm/dirt_path_tile', fc * T, fr * T, fw * T, FARM_FENCE_Y - fr * T - 4, 0.5);
      fill('farm/dirt_path_tile', FARM_GATE_X - 36, FARM_FENCE_Y - 8, 72, H - FARM_FENCE_Y + 8, 0.5);
      for (let i = 0; i < 3; i++) for (let j = 0; j < 2; j++) {
        put(j ? 'farm/soil_plot_wet' : 'farm/soil_plot', 1204 + i * 64, 848 + j * 64, 60, 60);
      }
    }
    // Đường đất ngang ở đồng cỏ.
    fill('farm/dirt_path_tile', x0, COUNTRY[0] * T, ww, COUNTRY.length * T, 0.5, 0, COUNTRY[0] * T);
    for (const [x, y, wet] of plan.plots) put(wet ? 'farm/soil_plot_wet' : 'farm/soil_plot', x, y, 60, 60);
    g.restore();
  }

  // ---------- Toà nhà, cây, đồ vật của thành phố (ảnh HTML xếp theo chiều sâu) ----------

  // Mỗi món: [ảnh, x giữa đáy, y đáy, tỉ lệ, lật ngang?, class thêm].
  function props() {
    const list = [];
    const at = (img, x, y, k = 1, flip = false, cls = '') => list.push([img, x, y, k, flip, cls]);

    // Cây to nơi hai đường dọc biến mất vào xa (chân đồi).
    for (const c3 of ROAD_COLS) at('tree_big', (c3[0] + 1.5) * T, HORIZON + 14, 0.75);

    // Toà nhà bờ Bắc đặt giữa lô, cửa ở đáy (sát vỉa hè hàng 8); hai bên mỗi toà có cây và chậu hoa.
    at('building_casino', 224, 8 * T, 1);
    at('building_bank', 816, 8 * T, 0.86);
    at('building_shop', 1392, 8 * T, 1);
    for (const x of [40, 408, 1226, 1558]) at('tree_1', x, 8 * T + 2, 0.72);
    for (const x of [140, 308, 1272, 1512]) at('farm/flowers', x, 8 * T + 4, 0.55);

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

    // Bệnh viện: toà giữa lô, sân lát gạch trước cửa, xe cứu thương đậu trong sân, cây và hoa hai bên.
    at('building_hospital', 224, 966, 0.8);
    at('ambulance_side', 396, 1012, 0.7);
    at('bench', 74, 1008, 0.55);
    at('tree_1', 52, 30 * T - 24, 0.7);
    at('tree_1', 400, 30 * T - 24, 0.7);
    at('farm/flowers', 112, 30 * T - 2, 0.5);
    at('farm/flowers', 336, 30 * T - 2, 0.5);

    // Quảng trường: đài phun nước giữa lô, ghế, cây và chậu hoa ở góc.
    at('fountain', 816, 945, 1.1);
    at('bench', 690, 1004, 0.6);
    at('bench', 942, 1004, 0.6);
    at('tree_big', 650, 872, 0.55);
    at('tree_big', 982, 872, 0.55);
    at('tree_1', 630, 1018, 0.55);
    at('tree_1', 1002, 1018, 0.55);
    at('farm/flowers', 724, 812, 0.55);
    at('farm/flowers', 908, 812, 0.55);

    // Nông trại: hàng rào ngang có cổng gỗ, bụi rào hai bên; trong rào là ruộng, chuồng, rơm, gia súc.
    for (const x of [1211.7, 1267, 1322.3]) at('farm/fence_piece', x, FARM_FENCE_Y, 0.84);
    at('farm/fence_piece', 1565, FARM_FENCE_Y, 1.06);
    at('farm_gate', FARM_GATE_X, FARM_FENCE_Y + 1, 0.9);
    for (let y = 856; y <= 1016; y += 32) for (const x of [1198, 1590]) at('bush_2', x, y, 0.7);
    const crops = ['corn_ripe', 'tomato_ripe', 'carrot_2', 'watermelon_ripe', 'strawberry_2', 'rice_ripe'];
    crops.forEach((c, i) => at('farm/crop_' + c, 1204 + (i % 3) * 64 + 30, 848 + Math.floor(i / 3) * 64 + 52, 0.5));
    at('farm/barn', 1540, 965, 0.62);
    at('farm/hay', 1586, 1014, 0.5);
    at('farm/trough_water', 1516, 1016, 0.45);
    at('farm/animal_cow', 1250, 1018, 0.5, true);
    at('farm/animal_sheep', 1330, 1014, 0.45);
    at('farm/animal_pig', 1394, 1016, 0.5, true);
    at('farm/animal_chicken', 1424, 905, 0.5);
    at('farm/animal_chicken_white', 1460, 940, 0.5, true);

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

  // Mây trôi trên trời (ba đám mỗi mảng).
  function cloudsHtml(kx) {
    const r = rng(kx * 131 + 7);
    return [['farm/cloud_1', 40, -88, 0.5, 70], ['farm/cloud_2', 520, -92, 0.42, 95], ['farm/cloud_3', 1080, -80, 0.46, 80]]
      .map(([n, x, y, k, dur]) => [n, kx * W + x + between(r, -30, 200), y + between(r, -4, 6), k, dur])
      .filter(([n]) => sizes[n])
      .map(([n, x, y, k, dur]) => `<img class="cloud" src="${srcOf(n)}" alt="" draggable="false"
        style="left:${px(x)};top:${px(y)};width:${px(sizes[n][0] * k)};animation-duration:${dur}s">`)
      .join('');
  }

  // Nước sông gợn: các đoạn giữa hai cây cầu (hai đầu chảy tiếp ra ngoại ô), nền ảnh mặt nước trôi dần.
  function waterHtml() {
    const segs = [[-FAR / T, 14], [19, 32], [37, COLS + FAR / T]];
    return segs
      .map(([a, b]) => `<div class="river-flow" style="left:${px(a * T)};top:${px(RIVER_ROWS[0] * T + 34)};width:${px((b - a) * T)};height:${px(RIVER_ROWS.length * T - 68)}"></div>`)
      .join('');
  }

  // Xe chạy vòng trên các làn đường ngang (ảnh nghiêng, lật theo hướng). Đường dọc dành cho người đi bộ qua cầu.
  // Vị trí tính bằng JS (không dùng CSS animation) để phát hiện va chạm với nhân vật.
  // Mỗi làn PER_LANE xe rải (có lệch) suốt đoạn đường dài 2·FAR + W, cùng tốc độ trong một làn nên không đè nhau;
  // xe chạy liên tục ra vào khung nhìn.
  const LANES = [[11, 'east', 115], [9, 'west', 95], [20, 'east', 130], [18, 'west', 105]];
  const CAR_IMGS = ['car_red_side', 'car_blue_side', 'car_yellow_side', 'car_green_side', 'car_red_side', 'ambulance_side',
    'car_blue_side', 'car_green_side', 'car_yellow_side', 'car_blue_side'];
  const PER_LANE = 10;
  const CAR_SPAN = W + 2 * FAR + 240;
  const JITTER = [0, 0.3, 0.1, 0.42, 0.2, 0.05, 0.35, 0.15, 0.4, 0.25];
  const CARS = LANES.flatMap(([row, dir, speed], li) => Array.from({ length: PER_LANE }, (_, i) => ({
    img: CAR_IMGS[(i + li * 3) % CAR_IMGS.length], dir, y: roadCenter(row), speed,
    at: ((i + JITTER[(i + li * 4) % PER_LANE]) * CAR_SPAN) / PER_LANE,
  })));
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

  // Vị trí tâm xe hiện tại; xe chạy hết cả đoạn đường ngoại ô rồi mới quay vòng (không hiện ra / biến mất giữa màn hình).
  function carPos(car) {
    const span = carHorizontal(car) ? CAR_SPAN : H + 240;
    const p = ((car.at % span) + span) % span - (carHorizontal(car) ? FAR + 120 : 120);
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
    // ngoại ô
    { img: 'boat_2', from: -1500, to: -200, y: 15 * T + 6, speed: 35, at: 400 },
    { img: 'boat_1', from: -3200, to: -1800, y: 14 * T + 30, speed: 50, at: 90 },
    { img: 'boat_3', from: 1800, to: 3000, y: 14 * T + 30, speed: 40, at: 700 },
    { img: 'boat_2', from: 3300, to: 4600, y: 15 * T + 8, speed: 30, at: 50 },
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
    // Biển tên: bờ Bắc treo trên trời phía trên toà nhà; bờ Nam đặt ở đầu lô (dải cỏ / sân giữa vỉa hè và toà nhà).
    const labelAt = { casino: [224, -84], bank: [816, -84], shop: [1392, -84], plaza: [816, 712], hospital: [224, 706], farm: [1262, 712] };
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
      // Chân nhân vật nằm trong làn xe (quanh tâm làn) và trùng thân xe theo chiều ngang. Hộp va chạm rộng hơn
      // làn nên phải chặn thêm: chân đứng trên vỉa hè / lề đường thì không bị tông.
      const onRoad = carHorizontal(car) ? inRoadRow(Math.floor(me.y / T)) : inRoadCol(Math.floor(me.x / T));
      if (onRoad && Math.abs(me.x - x) < hw + 12 && me.y > y - up && me.y < y + down) hit('car', car);
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
    for (let d = 1; d < GW; d++) {
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
        cam.free = false;
        if (path) { path = null; pathZone = null; hideMarker(); }
        startMoving();
        stepFree(vx, vy, dt);
        afterMove();
      } else if (path) {
        cam.free = false;
        stepPath(dt);
        afterMove();
      } else if (me.walking) {
        stop();
      }
    }
    stepCamera(dt, false);
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

  // ---------- Camera và các mảng ----------

  // cam.x: tâm khung nhìn theo chiều ngang, cam.y: mép trên khung nhìn (đơn vị bản đồ). Thu phóng theo chiều cao:
  // dải trời + cả thành phố vừa khít phía trên thanh đi nhanh; chiều ngang không giới hạn. cam.free: đang kéo nhìn quanh
  // (camera đứng yên tới khi nhân vật đi tiếp).
  const cam = { x: W / 2, y: -SKY, free: false };
  const view = { w: W, h: H + SKY, cw: 0, ch: 0 };
  let lastCam = '';
  let worldEl = null; // .city-world (được dịch / thu phóng theo camera)
  let groundReady = false;
  const chunks = new Map(); // số mảng -> { canvas, props }
  const clamp = (v, a, b) => Math.min(b, Math.max(a, v));

  function fit() {
    if (!root) return;
    const cw = root.clientWidth;
    const ch = root.clientHeight;
    if (!cw || !ch) return;
    scale = Math.max(0.25, (ch - NAV_PAD) / (H + SKY));
    Object.assign(view, { w: cw / scale, h: ch / scale, cw, ch });
    lastCam = '';
    stepCamera(0, true);
  }

  // Vị trí camera muốn tới: nhân vật nằm trong vùng chết giữa khung (không lắc theo từng bước), chân luôn ở trên thanh
  // đi nhanh; còn lại ưu tiên thấy trọn dải trời.
  function camTarget() {
    const dz = view.w * 0.2;
    return [clamp(cam.x, me.x - dz, me.x + dz), Math.max(-SKY, me.y + (NAV_PAD + 70) / scale - view.h)];
  }

  function stepCamera(dt, snap) {
    if (!me || !root) return;
    if (!cam.free) {
      const [tx, ty] = camTarget();
      const k = snap ? 1 : 1 - Math.exp(-dt * 5);
      cam.x += (tx - cam.x) * k;
      cam.y += (ty - cam.y) * k;
    }
    cam.x = clamp(cam.x, -EXT * T, W + EXT * T);
    cam.y = clamp(cam.y, -SKY, Math.max(-SKY, H + MB - view.h));
    const t = `translate(${((view.w / 2 - cam.x) * scale).toFixed(1)}px, ${((-SKY - cam.y) * scale).toFixed(1)}px) scale(${scale})`;
    if (t === lastCam) return;
    lastCam = t;
    worldEl.style.transform = t;
    ensureChunks();
  }

  // Dựng các mảng trong (và cạnh) khung nhìn, bỏ các mảng đã xa.
  function ensureChunks() {
    if (!groundReady) return;
    const k0 = Math.floor((cam.x - view.w / 2) / W) - 1;
    const k1 = Math.floor((cam.x + view.w / 2) / W) + 1;
    for (let k = k0; k <= k1; k++) if (!chunks.has(k)) makeChunk(k);
    for (const [k, ch] of chunks) {
      if (k >= k0 - 1 && k <= k1 + 1) continue;
      ch.canvas.remove();
      ch.props.remove();
      chunks.delete(k);
    }
  }

  function makeChunk(kx) {
    const plan = chunkPlan(kx);
    const canvas = document.createElement('canvas');
    canvas.className = 'chunk-ground';
    canvas.width = W + 2 * BLEED;
    canvas.height = SKY + H + MB;
    canvas.style.cssText = `left:${px(kx * W - BLEED)};top:${px(-SKY)};width:${px(W + 2 * BLEED)};height:${px(SKY + H + MB)}`;
    drawChunk(canvas, kx, plan);
    world.querySelector('.city-ground').appendChild(canvas);
    const layer = document.createElement('div');
    layer.className = 'chunk-props';
    layer.innerHTML = cloudsHtml(kx) + plan.props.map(propHtml).join('');
    world.querySelector('.city-props').appendChild(layer);
    chunks.set(kx, { canvas, props: layer });
  }

  // Kéo chuột / vuốt trên bản đồ để nhìn quanh; bấm (không kéo) vẫn là đi tới điểm đó.
  function bindDrag(stage) {
    let start = null;
    let suppress = false;
    stage.addEventListener('pointerdown', (e) => {
      if (e.button !== 0) return;
      start = { id: e.pointerId, x: e.clientX, y: e.clientY, cx: cam.x, cy: cam.y, drag: false };
    });
    stage.addEventListener('pointermove', (e) => {
      if (!start || e.pointerId !== start.id) return;
      const dx = e.clientX - start.x;
      const dy = e.clientY - start.y;
      if (!start.drag) {
        if (Math.hypot(dx, dy) < 8) return;
        start.drag = true;
        try { stage.setPointerCapture(e.pointerId); } catch (err) { /* bỏ qua */ }
        root.classList.add('dragging');
      }
      cam.free = true;
      cam.x = start.cx - dx / scale;
      cam.y = start.cy - dy / scale;
      stepCamera(0, false);
    });
    const end = (e) => {
      if (!start || e.pointerId !== start.id) return;
      if (start.drag) {
        suppress = true;
        setTimeout(() => { suppress = false; }, 0);
        root.classList.remove('dragging');
      }
      start = null;
    };
    stage.addEventListener('pointerup', end);
    stage.addEventListener('pointercancel', end);
    // Kéo xong thì bỏ cú "click" đi kèm (không đi tới điểm thả chuột, không vào khu).
    stage.addEventListener('click', (e) => {
      if (!suppress) return;
      suppress = false;
      e.stopPropagation();
      e.preventDefault();
    }, true);
  }

  function mount(el, o) {
    opts = o;
    root = el;
    el.classList.add('city-toon');
    el.innerHTML = `<div class="city-stage">
      <div class="city-world" style="width:${px(W)};height:${px(H + SKY)}" role="img" aria-label="Bản đồ DCity">
        <div class="city-inner" style="top:${px(SKY)};width:${px(W)};height:${px(H)}">
          <div class="city-ground"></div>
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
    worldEl = el.querySelector('.city-world');
    // Nạp kích thước ảnh trước rồi dựng phần ảnh HTML (cần w/h) và vẽ mặt đất.
    loadManifests().then(() => {
      el.querySelector('.city-props').innerHTML = waterHtml() + boatsHtml() + signHtml() + trafficHtml() + zonesHtml();
      el.querySelectorAll('[data-zone]').forEach((z) => z.addEventListener('click', (e) => {
        e.stopPropagation();
        walkTo(z.dataset.zone);
      }));
      CARS.forEach((c) => { c.el = null; });
      BOATS.forEach((b) => { b.el = null; });
      return loadImages(GROUND_IMAGES).then(() => {
        groundReady = true;
        ensureChunks();
      });
    });

    // Lớp điều khiển (không cuộn theo bản đồ): gợi ý vào khu, hướng dẫn phím, cần điều khiển ảo.
    overlay = document.createElement('div');
    overlay.className = 'city-overlay';
    let helpHidden = false;
    try { helpHidden = localStorage.getItem('city.help') === 'off'; } catch (e) { /* bỏ qua */ }
    overlay.innerHTML = `
      <button class="enter-hint hidden" type="button"><span class="eh-key">E</span><span class="eh-key">Enter</span>Vào <b class="eh-name"></b></button>
      <div class="city-help${helpHidden ? ' hidden' : ''}">WASD / ↑↓←→ để đi · Bấm chuột để tới · E / Enter để vào
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

    // Bấm toà nhà / biển tên: đi tới cửa khu (gắn khi dựng biển); bấm chỗ khác: đi tới điểm đó; kéo để nhìn quanh.
    const stage = el.querySelector('.city-stage');
    bindDrag(stage);
    stage.addEventListener('click', (e) => {
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
    cam.free = false;
    stepCamera(0, true);
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

  // Vị trí ngang của khu trên màn hình, quy về thang 0–1600 (≥ 900 là nửa phải), để đặt bảng thông tin không che khu đó.
  const zoneX = (zone) => ((NODES[ZONES[zone].node][0] - cam.x) / view.w + 0.5) * W;

  window.City = {
    ZONES, mount, walkTo, setLook, setTag, setSub, setBadge, teleport, setLocked, zoneX,
    position: () => me && [me.x, me.y],
  };
})();
