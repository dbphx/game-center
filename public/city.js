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
    // Các khu ở ngoại ô, bờ Nam: Công trường bên trái, Hồ câu cá bên phải, Khu danh vọng giữa Công trường và thành phố;
    // cửa vào trên vỉa hè hàng 21 trước cổng.
    site: [-42, 22, 16, 10], fishing: [68, 22, 18, 10], fame: [-20, 22, 14, 10],
  };
  const SITE_X = -1088; // tâm cổng Công trường
  const FISH_X = 2464; // tâm cổng Hồ câu cá
  const FAME_X = -416; // tâm cổng Khu danh vọng
  const ZONE_FENCE_Y = 772; // hàng rào / cổng của các khu ngoại ô
  // Khoảng x dành riêng cho các khu ngoại ô (không rải lô nhà / ruộng vào đây).
  const RESERVED = [[-42 * T - 16, -26 * T + 16], [68 * T - 16, 86 * T + 16], [-20 * T - 16, -6 * T + 16]];
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
    site: { name: 'Công trường', sub: 'Xây dựng · Kiếm xu', node: 'site' },
    fishing: { name: 'Hồ câu cá', sub: 'Câu cá giải trí', node: 'fishing' },
    fame: { name: 'Khu danh vọng', sub: 'Top 10 đại gia', node: 'fame' },
  };

  // Điểm đứng trước cửa / trong mỗi khu (đơn vị bản đồ).
  const NODES = {
    casino: [224, 276], bank: [816, 276], shop: [1452, 276], // cửa Shop lệch phải trong ảnh toà nhà
    plaza: [816, 780], hospital: [224, 994], farm: [FARM_GATE_X, 790],
    site: [SITE_X, 690], fishing: [FISH_X, 690], fame: [FAME_X, 690],
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

  // Bộ ảnh: mỗi tên (vd 'tree_1', 'farm/barn') → { src, w, h, ax, ay }: kích thước hiển thị gốc và điểm chân (neo) trong ảnh.
  // Đọc từ manifest.json của /img/toon/city/ và /img/toon/farm/ (khoá "farm/x"). Một bộ ảnh thay thế (vd ảnh 3D ở
  // /img/toon/city3d/) ghi đè theo tên: mỗi ảnh trong manifest của bộ đó có "replaces": "tên cũ" (hoặc mảng), hoặc manifest
  // có bảng "map": { "tên cũ": "file mới.png" }; neo đọc từ "anchor" ("bottom-center", "center", [x, y] hay {x, y}) hoặc
  // "foot": [x, y]; "scale" (mặc định 1) đổi kích thước hiển thị. Mặc định dùng bộ 3D; ?art=2d (hoặc localStorage
  // 'city.art' = '2d') để xem bộ hoạt hình phẳng cũ.
  const ART_SETS = { city3d: '/img/toon/city3d/' };
  const art = {};
  const sizes = {}; // tên → [w, h] hiển thị (dùng cho xe, thuyền, mây...)
  const images = {};
  const srcOf = (name) => (art[name] ? art[name].src : (name.startsWith('farm/') ? FARM_IMG + name.slice(5) : IMG + name) + '.png');

  function anchorOf(it, w, h) {
    const a = it.foot || it.anchor;
    if (Array.isArray(a)) return [a[0], a[1]];
    if (a && typeof a === 'object') return [a.x, a.y];
    if (a === 'center') return [w / 2, h / 2];
    if (a === 'top-left') return [0, 0];
    return [w / 2, h];
  }
  function register(name, dir, it, file) {
    const k = it.scale || 1;
    const [ax, ay] = anchorOf(it, it.w, it.h);
    art[name] = { src: dir + (file || it.file), w: it.w * k, h: it.h * k, ax: ax * k, ay: ay * k, pw: it.w };
    sizes[name] = [it.w * k, it.h * k];
  }
  function artSet() {
    let set = null;
    try { set = new URLSearchParams(location.search).get('art') || localStorage.getItem('city.art'); } catch (e) { /* bỏ qua */ }
    if (set === '2d') return null;
    return ART_SETS[set] || ART_SETS.city3d;
  }
  let art3d = false; // đang dùng bộ ảnh 3D (đặt sau khi đọc manifest)
  // Ảnh có trong bộ đang dùng thì lấy, không thì dùng ảnh thay thế (ảnh chỉ bộ 3D mới có: tree_cau, flower_pot...).
  const pickArt = (name, fallback) => (art[name] ? name : fallback);

  async function loadManifests() {
    await loadArt();
    await measureBoxes();
  }

  // Khung pixel không trong suốt (alpha > 100) của các ảnh cần giữ khỏi lòng đường, đơn vị hiển thị: art[n].box = {l, t, r, b}.
  const BOX_RE = /(^|\/)(tree_|bush|building_|house_|lamp|bench|flower)/;
  async function measureBoxes() {
    const bySrc = {};
    await Promise.all(Object.keys(art).filter((n) => BOX_RE.test(n)).map((n) => {
      const src = art[n].src;
      if (!bySrc[src]) {
        bySrc[src] = new Promise((resolve) => {
          const img = new Image();
          img.onload = () => {
            try {
              const c = document.createElement('canvas');
              c.width = img.naturalWidth;
              c.height = img.naturalHeight;
              const g = c.getContext('2d');
              g.drawImage(img, 0, 0);
              const d = g.getImageData(0, 0, c.width, c.height).data;
              let l = c.width;
              let t = c.height;
              let r = 0;
              let b = 0;
              for (let y = 0; y < c.height; y++) {
                for (let x = 0; x < c.width; x += 2) {
                  if (d[(y * c.width + x) * 4 + 3] <= 100) continue;
                  if (x < l) l = x;
                  if (x > r) r = x;
                  if (y < t) t = y;
                  b = y;
                }
              }
              resolve(r >= l ? { l, t, r: r + 2, b: b + 1, w: c.width } : null);
            } catch (e) { resolve(null); }
          };
          img.onerror = () => resolve(null);
          img.src = src;
        });
      }
      return bySrc[src].then((px) => {
        if (!px) return;
        const u = art[n].w / px.w;
        art[n].box = { l: px.l * u, t: px.t * u, r: px.r * u, b: px.b * u };
      });
    }));
  }

  async function loadArt() {
    const read = async (dir) => {
      try { return await (await fetch(dir + 'manifest.json')).json(); } catch (e) { return null; }
    };
    await Promise.all([[IMG, ''], [FARM_IMG, 'farm/']].map(async ([dir, prefix]) => {
      const m = await read(dir);
      for (const it of (m && m.images) || []) register(prefix + it.file.replace(/\.png$/, ''), dir, it);
    }));
    const dir = artSet();
    const m = dir && (await read(dir));
    if (!m) return;
    art3d = true;
    const byFile = {};
    for (const it of m.images || []) {
      byFile[it.file] = it;
      if (it.frames && it.frameW) boatMeta[it.file.replace(/\.png$/, '')] = { w: it.w, frameW: it.frameW, foot: it.foot || [it.frameW / 2, it.h] };
      const names = [].concat(it.replaces || []);
      for (const n of names.length ? names : [it.file.replace(/\.png$/, '')]) register(n, dir, it);
    }
    for (const [old, file] of Object.entries(m.map || {})) if (byFile[file]) register(old, dir, byFile[file], file);
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
    'farm/soil_plot_wet', 'farm/dirt_path_tile', 'farm/grass_plot', 'fishing_lake', 'fishing_pier',
  'fame_carpet',
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
  // Cây: phượng, bằng lăng, bàng, sao (cây phố), xoài, đa, dừa, mai, đào, tre, chuối (vườn) và cây xanh tán tròn.
  const TREES = ['tree_1', 'tree_phuong', 'tree_banglang', 'tree_bang', 'tree_sao', 'farm/tree_xoai', 'farm/tree_dua', 'tree_big',
    'farm/tree_mai', 'farm/tree_dao', 'farm/tree_tre', 'farm/tree_chuoi', 'farm/tree_big', 'farm/tree_med', 'farm/tree_da', 'tree_lieu', 'tree_cau'];
  const TREE_K = {
    tree_1: [0.6, 0.8], tree_big: [0.5, 0.68], 'farm/tree_big': [0.45, 0.6], 'farm/tree_med': [0.6, 0.8], 'farm/tree_round_1': [0.6, 0.8],
    tree_phuong: [0.4, 0.5], tree_banglang: [0.42, 0.52], tree_bang: [0.4, 0.5], tree_sao: [0.42, 0.52], tree_lieu: [0.42, 0.52],
    'farm/tree_xoai': [0.36, 0.44], 'farm/tree_dua': [0.4, 0.5], 'farm/tree_da': [0.32, 0.4], 'farm/tree_mai': [0.5, 0.6],
    'farm/tree_dao': [0.5, 0.6], 'farm/tree_tre': [0.5, 0.62], 'farm/tree_chuoi': [0.5, 0.6], tree_cau: [0.6, 0.75],
  };
  // Cây phố (vỉa hè, quảng trường): xen kẽ phượng, bằng lăng, bàng, sao, dừa.
  const STREET_TREES = ['tree_phuong', 'tree_banglang', 'tree_bang', 'tree_sao', 'farm/tree_dua', 'tree_phuong', 'tree_banglang', 'tree_1'];
  const treeK = (name, r) => { const [a, b] = TREE_K[name] || [0.6, 0.8]; return r ? a + (b - a) * r : (a + b) / 2; };
  const BUSHES = ['bush_1', 'farm/bush_1', 'bush_3', 'farm/bush_2', 'bush_2', 'farm/bush_3'];
  const CROPS = ['corn', 'tomato', 'carrot', 'watermelon', 'strawberry', 'rice'];
  const ANIMALS = ['farm/animal_cow', 'farm/animal_sheep', 'farm/animal_pig', 'farm/animal_horse', 'farm/animal_chicken', 'farm/animal_chicken_white'];

  // Các đoạn [a, b] trong [xa, xb] không chạm khu dành riêng.
  function freeSpans(xa, xb) {
    let spans = [[xa, xb]];
    for (const [ra, rb] of RESERVED) {
      spans = spans.flatMap(([a, b]) => (rb <= a || ra >= b ? [[a, b]] : [[a, ra], [rb, b]].filter(([u, v]) => v - u > 60)));
    }
    return spans;
  }

  // Công trường, Hồ câu cá, Khu danh vọng: hàng rào + cổng ở y ZONE_FENCE_Y (cửa vào trên vỉa hè ngay trước cổng), bên trong chỉ để nhìn.
  function zoneProps(kx, at, plan) {
    const x0 = kx * W;
    const inChunk = (x) => x >= x0 && x < x0 + W;
    if (inChunk(SITE_X)) {
      const [c, , w] = BLOCKS.site;
      const L = c * T;
      const Rr = L + w * T;
      plan.ground.push(['farm/dirt_path_tile', L, ZONE_FENCE_Y + 4, Rr - L, H - ZONE_FENCE_Y - 4, 0.5]);
      for (const x of [L + 44, L + 122, SITE_X + 132, SITE_X + 210]) at('site_barrier', x, ZONE_FENCE_Y, 0.75);
      at('site_gate', SITE_X, ZONE_FENCE_Y + 1, 0.9);
      at('site_building', L + 140, 1004, 0.8);
      at('site_crane_mast', SITE_X + 130, 992, 0.75);
      plan.fx.push(['crane', SITE_X + 130, 992 - 270 + 4]);
      at('site_bricks', SITE_X + 30, 1012, 0.8);
      at('site_sand', Rr - 60, 1004, 0.8);
      // Bộ 3D có thêm: giàn giáo, máy trộn, bao xi măng.
      if (art.scaffold) at('scaffold', Rr - 110, 930, 0.62);
      if (art.mixer) at('mixer', SITE_X - 60, 1010, 0.7);
      if (art.cement_bags) at('cement_bags', Rr - 150, 1014, 0.7);
      for (const x of [SITE_X - 50, SITE_X + 50, L + 250]) at('site_cone', x, x === L + 250 ? 990 : 806, 0.8);
      plan.fx.push(['beacon', SITE_X - 71, ZONE_FENCE_Y - 128], ['beacon', SITE_X + 71, ZONE_FENCE_Y - 128]);
      at('fx_flag', L + 30, ZONE_FENCE_Y - 6, 0.7);
    }
    if (inChunk(FISH_X)) {
      const [c, , w] = BLOCKS.fishing;
      const L = c * T;
      const Rr = L + w * T;
      // Hồ (vẽ lên mặt đất) + bến gỗ, sen, lục bình, liễu, đèn lồng dưới xà cổng.
      plan.ground.push(['fishing_lake', L + 14, 792, 548, 266]);
      plan.ground.push(['fishing_pier', FISH_X + 110, 902, 120, 120]);
      for (const x of [L + 40, L + 96, L + 152, Rr - 40, Rr - 96, Rr - 152]) at('farm/fence_piece', x, ZONE_FENCE_Y, 0.85);
      at('fishing_gate', FISH_X, ZONE_FENCE_Y + 1, 0.9);
      at('tree_lieu', L + 30, 870, 0.5);
      at('tree_lieu', Rr - 24, 900, 0.46, true);
      at(pickArt('bench_wood', 'bench'), L + 90, 1012, 0.75, false, 'deco');
      // Ghế gỗ mộc trên vỉa hè trước cổng hồ (ngồi được), cầu tre ven hồ, đèn lồng cột tre bên bờ.
      if (art.bench_wood) for (const x of [FISH_X - 150, FISH_X + 150]) at('bench_wood', x, 22 * T - 4, 0.62);
      if (art.bamboo_bridge) at('bamboo_bridge', L + 470, 1000, 0.7);
      if (art.lantern) for (const x of [L + 100, Rr - 130]) at('lantern', x, 846, 0.5);
      at('farm/tree_dua', Rr - 70, 1018, 0.4);
      plan.fx.push(['lantern', FISH_X - 40, ZONE_FENCE_Y - 92], ['lantern', FISH_X + 40, ZONE_FENCE_Y - 92]);
      for (const [x, y, n] of [[L + 140, 880, 'fishing_lotus'], [L + 260, 960, 'fishing_lotus'], [L + 380, 880, 'fishing_hyacinth'], [L + 200, 1000, 'fishing_hyacinth']]) {
        plan.fx.push(['drift', x, y, n]);
      }
      for (let i = 0; i < 7; i++) plan.fx.push(['sparkle', L + 120 + ((i * 67) % 330), 860 + ((i * 41) % 130)]);
      plan.fx.push(['basket', L + 230, 930]);
    }
    if (inChunk(FAME_X)) {
      const [c, , w] = BLOCKS.fame;
      const L = c * T;
      const Rr = L + w * T;
      // Sân lát đá, thảm đỏ từ cổng tới cửa Đại sảnh; bục vinh danh bên trái, cúp vàng bên phải.
      plan.ground.push(['sidewalk_tile', L, ZONE_FENCE_Y + 4, Rr - L, H - ZONE_FENCE_Y - 4, 0.75]);
      plan.ground.push(['fame_carpet', FAME_X - 24, ZONE_FENCE_Y + 4, 48, 200, 0.75]);
      [L + 30, L + 86, L + 142, Rr - 142, Rr - 86, Rr - 30].forEach((x, i) => at(['bush_1', 'bush_3', 'farm/bush_2'][i % 3], x, ZONE_FENCE_Y, 0.6));
      at('fame_gate', FAME_X, ZONE_FENCE_Y + 1, 0.85);
      at('fame_hall', FAME_X, 992, 0.8);
      at('fame_podium', L + 74, 950, 0.55);
      at('fame_trophy', Rr - 60, 950, 0.55);
      for (const x of [FAME_X - 70, FAME_X + 70]) at('lamp', x, 840, 0.8);
      at('fx_flag', L + 20, 1000, 0.7);
      at('fx_flag', Rr - 20, 1000, 0.7, true);
      for (let i = 0; i < 6; i++) plan.fx.push(['sparkle', Rr - 90 + ((i * 23) % 60), 880 + ((i * 17) % 50)]);
      for (let i = 0; i < 4; i++) plan.fx.push(['sparkle', L + 50 + ((i * 29) % 50), 900 + ((i * 13) % 30)]);
    }
  }

  // ---------- Không lấn lòng đường ----------

  // Mặt nhựa (đường ngang, đường dọc thành phố tới đường đất), cầu, và chỗ đứng trước cửa khu: [trái, trên, phải, dưới].
  const ASPHALT = [
    ...ROAD_ROWS.map((g) => [-1e6, g[0] * T, 1e6, (g[2] + 1) * T]),
    ...ROAD_COLS.map((g) => [g[0] * T, 5 * T, (g[2] + 1) * T, COUNTRY[0] * T + 8]),
  ];
  const BRIDGES = [[14 * T, 13 * T - 18, 19 * T, 17 * T + 6], [32 * T, 13 * T - 18, 37 * T, 17 * T + 6]];
  const DOORS = Object.values(NODES).map(([x, y]) => [x - 30, y - 70, x + 30, y + 10]);
  const hits = (b, r) => b[0] < r[2] && b[2] > r[0] && b[1] < r[3] && b[3] > r[1];
  const isPlant = (name) => isTree(name) || /(^|\/)bush/.test(name);
  const isHouse = (name) => /^(building_|house_)/.test(name);

  // Khung ảnh thật của một món đã đặt (x, y = điểm chân, k = tỉ lệ, flip = lật ngang).
  function propBox(name, x, y, k, flip) {
    const a = art[name];
    if (!a) return null;
    const b = a.box || { l: 0, t: 0, r: a.w, b: a.h };
    const left = flip ? x - (a.w - a.ax) * k : x - a.ax * k;
    const [l, r] = flip ? [left + (a.w - b.r) * k, left + (a.w - b.l) * k] : [left + b.l * k, left + b.r * k];
    return [l, y - (a.ay - b.t) * k, r, y + (b.b - a.ay) * k];
  }
  // Điểm chân để đáy ảnh toà / nhà bờ Bắc vừa chạm mép trên vỉa hè (không lấn xuống vỉa hè, lòng đường).
  function northFoot(name, k) {
    const a = art[name];
    if (!a) return 8 * T;
    const below = ((a.box ? a.box.b : a.h) - a.ay) * k;
    return 8 * T - Math.max(0, below);
  }
  // Lý do một món lấn đường ('' = ổn): cây / bụi — cả gốc và tán không chạm mặt nhựa, cầu, chỗ cửa khu;
  // toà / nhà bờ Bắc — đáy ảnh không quá mép trên vỉa hè; món khác — đáy ảnh không xuống mặt nhựa.
  function violation([name, x, y, k, flip]) {
    const b = propBox(name, x, y, k, flip);
    if (!b) return '';
    if (isPlant(name)) {
      if (ASPHALT.some((r) => hits(b, r))) return 'cây chạm mặt đường';
      if (BRIDGES.some((r) => hits(b, r))) return 'cây đè cầu';
      if (DOORS.some((r) => hits(b, r))) return 'cây che cửa khu';
      if (RIVER_ROWS.includes(Math.floor(y / T))) return 'gốc cây dưới sông';
      return '';
    }
    if (isHouse(name) && y <= 8 * T + 8) return b[3] > 8 * T + 0.5 ? 'nhà lấn vỉa hè' : '';
    const base = [b[0], y - 2, b[2], b[3]];
    return ASPHALT.some((r) => hits(base, r)) ? 'chân lấn mặt đường' : '';
  }
  // Sửa một món lấn đường: lùi đáy lên / gốc xuống khỏi mặt nhựa, thu nhỏ, dời ngang; không được thì bỏ (chỉ bỏ cây / bụi).
  function settle(list) {
    const out = [];
    for (const p of list) {
      if (!violation(p)) { out.push(p); continue; }
      const [name, x, y, k, flip, cls] = p;
      const cands = [];
      const b = propBox(name, x, y, k, flip);
      if (isHouse(name) && y <= 8 * T + 8) cands.push([name, x, y - (b[3] - 8 * T), k, flip, cls]);
      for (const kk of [k, k * 0.82, k * 0.66]) {
        const bb = propBox(name, x, y, kk, flip);
        for (const r of [...ASPHALT, ...BRIDGES, ...DOORS]) {
          if (!hits(bb, r)) continue;
          cands.push([name, x, y + (r[3] - bb[1]) + 2, kk, flip, cls]); // gốc xuống dưới (tán khỏi đường phía trên)
          cands.push([name, x, y - (bb[3] - r[1]) - 2, kk, flip, cls]); // đáy lên trên
          cands.push([name, x + (r[2] - bb[0]) + 2, y, kk, flip, cls]); // sang phải
          cands.push([name, x - (bb[2] - r[0]) - 2, y, kk, flip, cls]); // sang trái
        }
        cands.push([name, x, y, kk, flip, cls]);
      }
      // Chỉ nhận chỗ mới gần chỗ cũ (≤ 140 đơn vị) và không rơi vào sông.
      const ok = cands.find((c) => !violation(c) && Math.hypot(c[1] - x, c[2] - y) <= 140 && !RIVER_ROWS.includes(Math.floor(c[2] / T)));
      if (ok) out.push(ok);
      else if (!isPlant(name)) out.push(p);
    }
    return out;
  }

  // Kế hoạch một mảng: ô đất vẽ lên canvas và danh sách ảnh đồ vật.
  function chunkPlan(kx) {
    const r = rng(kx * 7919 + 101);
    const x0 = kx * W;
    const plan = { plots: [], props: [], ground: [], fx: [] };
    const at = (img, x, y, k = 1, flip = false, cls = '') => plan.props.push([img, x, y, k, flip, cls]);
    const tree = (x, y, scale = 1) => {
      let t = pick(r, TREES);
      if (!art[t]) t = 'tree_1';
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
      plan.props.push(...props(plan.fx));
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
      for (const [a, b] of freeSpans(x0, x0 + W)) band(a, b, 760, 1012, ['houses', 'field', 'grove', 'pasture', 'houses', 'field']);
      zoneProps(kx, at, plan);
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
    plan.props = settle(plan.props);
    // Hiệu ứng suy ra từ đồ vật: đèn đường sáng và cửa sổ sáng ban đêm, khói bếp trên mái nhà, bướm quanh hoa;
    // đàn chim bay ngang trời.
    for (const [img, x, y, k] of plan.props) {
      const a = art[img];
      if (!a) continue;
      if (img === 'lamp') plan.fx.push(['lampglow', x, y - a.h * k + 12]);
      else if (img.startsWith('house_') || img.startsWith('building_')) {
        plan.fx.push(['window', x, y - a.h * k * 0.42, a.w * k * 0.8, a.h * k * 0.55]);
        if (img.startsWith('house_') && r() < 0.55) plan.fx.push(['smoke', x - a.w * k * between(r, 0.05, 0.25), y - a.h * k + 6]);
      } else if (img === 'farm/flowers' && r() < 0.4) plan.fx.push(['butterfly', x, y - 34]);
    }
    plan.fx.push(['birds', x0 + between(r, 0, 400), between(r, -80, -30)]);
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
    // Nền riêng (sân đất công trường, hồ câu, bến gỗ): ảnh lặp (có tỉ lệ k) hoặc ảnh đặt nguyên (w × h).
    for (const [name, x, y, w, h, k] of plan.ground) {
      if (k) fill(name, x, y, w, h, k);
      else put(name, x, y, w, h);
    }
    g.restore();
  }

  // ---------- Toà nhà, cây, đồ vật của thành phố (ảnh HTML xếp theo chiều sâu) ----------

  // Mỗi món: [ảnh, x giữa đáy, y đáy, tỉ lệ, lật ngang?, class thêm]. fx nhận các hiệu ứng động riêng của thành phố.
  function props(fx) {
    const list = [];
    const at = (img, x, y, k = 1, flip = false, cls = '') => list.push([img, x, y, k, flip, cls]);
    const tree = (img, x, y, k, flip = false) => at(img, x, y, k || treeK(img), flip);

    // Cây to nơi hai đường dọc biến mất vào xa (chân đồi).
    for (const c3 of ROAD_COLS) at('tree_big', (c3[0] + 1.5) * T, HORIZON + 14, 0.75);

    // Toà nhà bờ Bắc đặt giữa lô, cửa ở đáy (sát vỉa hè hàng 8); hai bên mỗi toà có cây phố và chậu hoa.
    const BK = art3d ? CITY3D_K : { casino: 1, bank: 0.86, shop: 1, hospital: 0.8 };
    const footC = northFoot('building_casino', BK.casino);
    const footB = northFoot('building_bank', BK.bank);
    const footS = northFoot('building_shop', BK.shop);
    at('building_casino', 224, footC, BK.casino);
    at('building_bank', 816, footB, BK.bank);
    at('building_shop', art3d ? 1402 : 1392, footS, BK.shop);
    tree('tree_phuong', 36, 8 * T + 2, 0.4);
    tree('tree_banglang', 412, 8 * T + 2, 0.42, true);
    tree('tree_sao', 1226, 8 * T + 2, 0.42);
    tree('tree_bang', 1560, 8 * T + 2, 0.4);
    for (const x of [140, 308, 1272, 1512]) at(pickArt('flower_pot', 'farm/flowers'), x, 8 * T + 4, art3d ? 0.7 : 0.55);

    // Vỉa hè: đèn đường hai bên đường ngang; cây phố dọc vỉa hè bờ Nam (chừa vạch qua đường và biển tên khu).
    for (const x of [96, 400, 700, 940, 1250, 1540]) {
      at('lamp', x, 9 * T - 2, 0.75);
      at('lamp', x + 30, 22 * T - 2, 0.75);
    }
    tree('tree_sao', 40, 22 * T - 6, 0.4);
    tree('farm/tree_dua', 632, 22 * T - 6, 0.38, true);
    tree('farm/tree_dua', 1004, 22 * T - 6, 0.38);
    if (art3d) for (const x of [560, 1080]) tree('tree_cau', x, 22 * T - 4, 0.62);

    // Công viên ven sông (vỉa hè hai bờ): dừa, mai, cây nhỏ, bụi, ghế; chừa chỗ hai cây cầu.
    const free = (x) => !(x > 13 * T && x < 20 * T) && !(x > 31 * T && x < 38 * T);
    const kinds = [['farm/tree_dua', 0.3], ['bush_2', 0.9], ['tree_3', 1.3], ['farm/tree_mai', 0.36], ['tree_4', 1.3], ['bush_4', 0.9], ['farm/tree_dao', 0.36]];
    let k = 0;
    for (let x = 40; x < W; x += 104) {
      if (!free(x)) continue;
      const [kind, kk] = kinds[k++ % kinds.length];
      at(kind, x, 13 * T - 4, kk, k % 2 === 0);
      if (free(x + 52)) {
        const [k2, kk2] = kinds[(k + 3) % kinds.length];
        at(k2, x + 52, 18 * T - 4, kk2 * 1.05, k % 3 === 0);
      }
    }
    for (const x of [120, 300, 760, 900, 1300, 1480]) at('bench', x, 13 * T - 2, 0.75);
    for (const x of [180, 360, 700, 860, 1360, 1540]) at('bench', x, 18 * T - 2, 0.75);

    // Bệnh viện: toà giữa lô, sân lát gạch trước cửa, xe cứu thương đậu trong sân, cây và hoa hai bên.
    at('building_hospital', 224, 966, BK.hospital);
    at('ambulance_side', 396, 1012, 0.7);
    at('bench', 80, 1008, 0.72);
    tree('tree_bang', 52, 30 * T - 24, 0.36);
    tree('tree_banglang', 400, 30 * T - 24, 0.4);
    at('farm/flowers', 112, 30 * T - 2, 0.5);
    at('farm/flowers', 336, 30 * T - 2, 0.5);

    // Quảng trường: đài phun nước giữa lô, ghế quanh đài, phượng + bằng lăng ở góc trên, mai + đào ở góc dưới.
    at('fountain', 816, 945, 1.1);
    for (const [x, y] of [[690, 1004], [942, 1004], [668, 905], [964, 905]]) at('bench', x, y, 0.75);
    tree('tree_phuong', 652, 816, 0.4);
    tree('tree_banglang', 980, 816, 0.42, true);
    tree('farm/tree_mai', 630, 1018, 0.48);
    tree('farm/tree_dao', 1002, 1018, 0.48, true);
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

    // Hiệu ứng động: đèn biển CASINO chạy vòng, đồng xu Ngân hàng, neon Thời trang, chữ thập Bệnh viện, đài phun, cờ.
    if (fx) {
      const [sx, sy] = casinoSignAt();
      if (art3d) {
        const C = (ix, iy) => onBuilding('building_casino', 224, footC, BK.casino, ix, iy);
        const B = (ix, iy) => onBuilding('building_bank', 816, footB, BK.bank, ix, iy);
        const S = (ix, iy) => onBuilding('building_shop', 1402, footS, BK.shop, ix, iy);
        const Hs = (ix, iy) => onBuilding('building_hospital', 224, 966, BK.hospital, ix, iy);
        fx.push(['bulbs', sx, sy - 22], ['coin', ...B(170, 112)], ['coin', ...B(360, 112)], ['neon', ...S(151, 130)],
          ['cross', ...Hs(175, 51)], ['spray', 816, 862], ['flag', ...C(64, 30)], ['flag', ...C(318, 30), true],
          ['flag', ...B(265, 8)], ['bunting', ...S(151, 196)]);
      } else {
        fx.push(['bulbs', 224, 0], ['coin', 712, 112], ['coin', 920, 112], ['neon', 1392, 36], ['cross', 224, 822],
          ['spray', 816, 846], ['flag', 112, 40], ['flag', 336, 40, true], ['flag', 816, 66], ['bunting', 1392, 200]);
      }
      for (let i = 0; i < 6; i++) fx.push(['sparkle', 760 + ((i * 37) % 112), 836 + ((i * 23) % 60)]);
    }
    return list;
  }

  // ---------- Phần động (HTML) ----------

  const px = (n) => `${Math.round(n)}px`;

  const isTree = (name) => /(^|\/)tree_/.test(name);
  // Lệch pha hoạt ảnh cố định theo vị trí (cây lắc không đều nhau).
  const phase = (x, y) => -(((Math.abs(Math.round(x * 7 + y * 13)) % 997) / 997) * 6).toFixed(2);

  // Ảnh đồ vật đặt theo điểm chân (neo trong manifest, mặc định giữa đáy), z-index theo y để nhân vật đi trước / sau đúng.
  // Cây lắc theo gió (lệch pha) và mờ đi khi che nhân vật; ghế băng bấm / đến gần để ngồi (trừ ghế chỉ để trang trí).
  function propHtml([name, x, y, k, flip, cls]) {
    const a = art[name];
    if (!a) return '';
    const w = a.w * k;
    const h = a.h * k;
    const left = flip ? x - (a.w - a.ax) * k : x - a.ax * k;
    const top = y - a.ay * k;
    let extra = '';
    if (isTree(name)) cls = `${cls} tree${h > 70 ? ' sway' : ''}`;
    if (art3d && name.startsWith('house_')) cls += ` tint${Math.abs(Math.round(x / 7 + y)) % 5}`;
    if ((name === 'bench' || name === 'bench_wood') && !/\bdeco\b/.test(cls)) {
      cls += ' bench';
      extra = ` data-bench="${Math.round(x)},${Math.round(y)}" title="Ngồi nghỉ (hồi máu ×2)"`;
    }
    return `<img class="prop${flip ? ' flip' : ''}${cls ? ' ' + cls.trim() : ''}" src="${srcOf(name)}" alt="" draggable="false"${extra}
      style="left:${px(left)};top:${px(top)};width:${px(w)};height:${px(h)};z-index:${100 + Math.round(y)};--d:${phase(x, y)}s">`;
  }

  // Hiệu ứng động (ảnh PNG + CSS animation / transform / opacity). Mỗi món: [loại, x, y, ...].
  function fxHtml([kind, x, y, p3, p4]) {
    const img = (name, cls, w, h, left, top, z, style = '') => (art[name]
      ? `<img class="fx ${cls}" src="${srcOf(name)}" alt="" draggable="false" style="left:${px(left)};top:${px(top)};width:${px(w)};height:${px(h)};z-index:${z};--d:${phase(left, top)}s;${style}">`
      : '');
    const z = 100 + Math.round(y);
    switch (kind) {
      case 'crane': // cần ngang quay qua lại trên đỉnh tháp (trục ở x = 120 / 400 của ảnh)
        return img('site_crane_jib', 'fx-crane', 300, 52, x - 90, y - 6, z + 700, 'transform-origin:90px 4px');
      case 'beacon':
        return img('fx_glow_orange', 'fx-blink glow', 70, 70, x - 35, y - 50, 2600) + img('site_beacon', 'fx-beacon', 24, 24, x - 12, y - 24, z + 800);
      case 'lantern':
        return img('fx_glow_red', 'fx-pulse glow', 54, 54, x - 27, y + 8, 2600) + img('lantern', 'fx-swing', 24, 42, x - 12, y, z + 800, 'transform-origin:12px 0');
      case 'drift': {
        const a = art[p3];
        return a ? img(p3, 'fx-drift', a.w * 0.8, a.h * 0.8, x - a.w * 0.4, y - a.h * 0.8, z) : '';
      }
      case 'basket': { // thuyền thúng có người câu, trôi chậm trên hồ
        const html = boatSheetHtml('basket', 'east', BOAT_K.basket);
        return html ? `<div class="fx fx-basket" style="left:${px(x)};top:${px(y)};z-index:${z}">${html}</div>` : '';
      }
      case 'sparkle':
        return img('fx_sparkle', 'fx-twinkle', 16, 16, x - 8, y - 8, z + 2);
      case 'smoke':
        return [0, 1, 2].map((i) => img('fx_smoke', 'fx-smoke', 26, 26, x - 13, y - 26, z, `animation-delay:${-i * 1.4}s`)).join('');
      case 'butterfly':
        return `<div class="fx fx-butterfly" style="left:${px(x - 10)};top:${px(y - 9)};z-index:${z + 40};--d:${phase(x, y)}s"><i style="background-image:url(${srcOf('fx_butterfly')})"></i></div>`;
      case 'birds':
        return `<div class="fx fx-birds" style="left:${px(x)};top:${px(y)};z-index:5;--d:${phase(x, y)}s">${[[0, 0], [26, 10], [50, -6], [70, 14], [92, 2]]
          .map(([dx, dy], i) => `<i style="left:${dx}px;top:${dy}px;background-image:url(${srcOf('fx_bird')});animation-delay:${-i * 0.13}s"></i>`).join('')}</div>`;
      case 'bulbs': { // bóng đèn chạy vòng quanh viền biển CASINO (170 × 44, tâm trên x)
        let out = '';
        let i = 0;
        const L = x - 86;
        for (let bx = L; bx <= x + 86; bx += 13) out += img('fx_bulb', 'fx-chase', 9, 9, bx - 4.5, y - 3, 400, `animation-delay:${(i++ * 0.08).toFixed(2)}s`);
        for (let bx = x + 86; bx >= L; bx -= 13) out += img('fx_bulb', 'fx-chase', 9, 9, bx - 4.5, y + 43, 400, `animation-delay:${(i++ * 0.08).toFixed(2)}s`);
        return out;
      }
      case 'coin':
        return img('fx_glow', 'fx-pulse glow', 46, 46, x - 23, y - 23, 2600) + img('fx_coin', 'fx-spin', 26, 26, x - 13, y - 13, 400);
      case 'neon':
        return img('fx_glow_pink', 'fx-neon glow', 190, 70, x - 95, y - 35, 2600);
      case 'cross':
        return img('fx_glow_red', 'fx-pulse glow', 80, 80, x - 40, y - 40, 2600);
      case 'spray':
        return img('fx_spray', 'fx-spray', 54, 80, x - 27, y - 80, 100 + 946);
      case 'flag':
        return img('fx_flag', `fx-wave${p3 ? ' flip' : ''}`, 30, 42, x - 4, y - 42, z);
      case 'bunting':
        return img('fx_bunting', 'fx-bunting', 200, 36, x - 100, y, 400);
      case 'lampglow':
        return img('fx_glow', 'fx-night glow', 90, 90, x - 45, y - 45, 2600);
      case 'window':
        return img('fx_glow', 'fx-night fx-window glow', p3, p4, x - p3 / 2, y - p4 / 2, 2600);
      default:
        return '';
    }
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
      const west = car.dir === 'west' && art[car.img.replace(/_side$/, '_west')];
      const name = west ? car.img.replace(/_side$/, '_west') : car.img;
      const a = art[name];
      const [w, h] = sizes[name] || [120, 52];
      // Bộ 3D: chân bánh xe (neo) nằm dưới tâm làn 14 đơn vị; bộ 2D giữ cách đặt cũ (CSS).
      const place = art3d && a ? `;transform:translate(${px(-a.ax * CAR_SCALE)}, ${px(-a.ay * CAR_SCALE + 14)})` : '';
      return `<div class="car car-${car.dir}${west ? ' own-west' : ''}" data-car="${i}"><img src="${srcOf(name)}" alt="" draggable="false"
        style="width:${px(w * CAR_SCALE)};height:${px(h * CAR_SCALE)}${place}"></div>`;
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
  // kind: loại thuyền của bộ 3D (xuồng ba lá, thuyền máy, đò, sà lan — sà lan chạy đoạn gần Công trường); img: ảnh bộ 2D.
  // y là đường mặt nước (điểm chân thuyền).
  const BOATS = [
    { kind: 'sampan', img: 'boat_1', from: 2 * T, to: 12 * T, y: 15 * T + 2, speed: 40, at: 0 },
    { kind: 'ferry', img: 'boat_2', from: 21 * T, to: 30 * T, y: 15 * T + 16, speed: 24, at: 120 },
    { kind: 'motor', img: 'boat_3', from: 39 * T, to: 48 * T, y: 15 * T + 4, speed: 60, at: 260 },
    // ngoại ô
    { kind: 'barge', img: 'boat_2', from: -1500, to: -200, y: 15 * T + 16, speed: 22, at: 400 },
    { kind: 'sampan', img: 'boat_1', from: -3200, to: -1800, y: 15 * T + 2, speed: 40, at: 90 },
    { kind: 'motor', img: 'boat_3', from: 1800, to: 3000, y: 15 * T + 6, speed: 60, at: 700 },
    { kind: 'ferry', img: 'boat_2', from: 3300, to: 4600, y: 15 * T + 16, speed: 24, at: 50 },
  ];
  const BOAT_K = { sampan: 0.8, motor: 0.78, ferry: 0.74, barge: 0.74, basket: 0.62 };
  const BOAT_WAKE = { sampan: 'wake', ferry: 'wake', barge: 'wake', motor: 'wake_small' };

  // Thuyền bộ 3D: dải 3 khung (lắc / khua chèo ~3 khung mỗi giây), ảnh riêng cho hướng tây, vệt sóng ở đuôi.
  // Đặt theo điểm chân (đường mặt nước) của từng khung.
  function boatSheetHtml(kind, dir, k, cls = '') {
    const a = art[`boat_${kind}_${dir}`];
    const meta = boatMeta[`boat_${kind}_${dir}`];
    if (!a || !meta) return '';
    const fw = meta.frameW * (a.w / meta.w) * k;
    const fh = a.h * k;
    const fx = meta.foot[0] * (a.w / meta.w) * k;
    const fy = meta.foot[1] * (a.w / meta.w) * k;
    let wake = '';
    const wn = BOAT_WAKE[kind] && `${BOAT_WAKE[kind]}_${dir}`;
    const wa = wn && art[wn];
    if (wa) {
      const kw = k * (kind === 'motor' ? 1 : 0.9);
      const stern = (dir === 'east' ? -1 : 1) * fw * 0.4; // đuôi thuyền so với điểm chân
      wake = `<img class="wake wake-${dir}" src="${srcOf(wn)}" alt="" draggable="false" style="left:${px(stern - wa.ax * kw)};top:${px(-wa.ay * kw)};width:${px(wa.w * kw)};height:${px(wa.h * kw)}">`;
    }
    return `<div class="boat-dir boat-${dir}${cls ? ' ' + cls : ''}">${wake}<i class="boat-sheet" style="left:${px(-fx)};top:${px(-fy)};width:${px(fw)};height:${px(fh)};background-image:url(${srcOf(`boat_${kind}_${dir}`)});background-size:${px(fw * 3)} ${px(fh)};--sw:${px(fw * 3)}"></i></div>`;
  }
  const boatMeta = {}; // tên dải khung → { w, frameW, foot } (đọc từ manifest bộ 3D)

  function boatsHtml() {
    return BOATS.map((b, i) => {
      const k = BOAT_K[b.kind] || 0.8;
      const three = art3d && boatSheetHtml(b.kind, 'east', k);
      if (three) {
        return `<div class="boat boat3d" data-boat="${i}">${three}${boatSheetHtml(b.kind, 'west', k)}</div>`;
      }
      const [w, h] = sizes[b.img] || [80, 42];
      return `<div class="boat" data-boat="${i}"><img src="${srcOf(b.img)}" alt="" draggable="false" style="width:${px(w * 0.85)};height:${px(h * 0.85)}"></div>`;
    }).join('');
  }

  // Biển CASINO nhấp nháy đè đúng lên biển trên nóc toà Casino.
  // Tâm biển CASINO rời: bộ 3D đặt trên mép mái trước của toà (điểm (175, 58) trong ảnh toà), bộ 2D đè lên biển vẽ sẵn.
  // Tỉ lệ các toà bộ 3D (mặt tiền song song đường) để vừa lô đất.
  const CITY3D_K = { casino: 1.12, bank: 0.88, shop: 1.24, hospital: 0.95 };
  // Toạ độ bản đồ của điểm (ix, iy) tính bằng px trong ảnh gốc của toà đặt chân ở (fx, fy) với tỉ lệ k.
  function onBuilding(name, fx, fy, k, ix, iy) {
    const a = art[name];
    if (!a) return [fx, fy];
    const u = a.w / (a.pw || a.w);
    return [fx + (ix * u - a.ax) * k, fy + (iy * u - a.ay) * k];
  }
  // Tâm biển CASINO rời: bộ 3D đặt trên mái trước của toà (điểm (191, 66) trong ảnh toà), bộ 2D đè lên biển vẽ sẵn.
  function casinoSignAt() {
    if (!art3d || !art.building_casino) {
      const [, bh] = sizes.building_casino || [262, 260];
      const [, sh] = sizes.sign_casino || [170, 44];
      return [224, 8 * T - bh + 4 + sh / 2];
    }
    const k = CITY3D_K.casino;
    return onBuilding('building_casino', 224, northFoot('building_casino', k), k, 191, 66);
  }
  function signHtml() {
    const [sw, sh] = sizes.sign_casino || [170, 44];
    const [cx, cy] = casinoSignAt();
    return `<img class="casino-sign" src="${srcOf('sign_casino')}" alt="" draggable="false"
      style="left:${px(cx - sw / 2)};top:${px(cy - sh / 2)};width:${px(sw)};height:${px(sh)};z-index:${100 + 8 * T + 1}">`;
  }

  // Vùng bấm của từng khu và biển tên (HTML).
  function zonesHtml() {
    // Biển tên: bờ Bắc treo trên trời phía trên toà nhà; bờ Nam đặt ở đầu lô (dải cỏ / sân giữa vỉa hè và toà nhà).
    const labelAt = { casino: [224, -84], bank: [816, -84], shop: [1392, -84], plaza: [816, 712], hospital: [224, 706], farm: [1262, 712],
      site: [SITE_X - 174, 712], fishing: [FISH_X - 174, 712], fame: [FAME_X - 174, 712] };
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
  let sitting = null; // ghế đang ngồi { x, y } (ngồi hồi máu ×2, báo ra ngoài qua opts.onSit)
  let nearBench = null; // ghế ở gần (gợi ý "E Ngồi nghỉ")
  let pathBench = null; // ghế sẽ ngồi khi đi hết path
  let hintKey = null; // nội dung gợi ý đang hiện

  function placeMe() {
    me.el.style.transform = `translate(${me.x}px, ${me.y}px)`;
    me.el.style.zIndex = String(100 + Math.round(me.y));
  }

  function drawMe() {
    const look = me.look || {};
    const hurt = performance.now() < hurtUntil;
    if (C && typeof C.sprite === 'function') {
      me.body.innerHTML = C.sprite(look, { dir: me.dir, walking: me.walking && !locked, size: SPRITE_SIZE, pose: locked ? 'lie' : sitting ? 'sit' : undefined, hurt });
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

  // Gợi ý khi đứng gần cửa khu ("Vào …"), gần ghế ("Ngồi nghỉ") hoặc đang ngồi ("Đứng dậy").
  function updateNear() {
    let found = null;
    for (const [id, z] of Object.entries(ZONES)) {
      const [nx, ny] = NODES[z.node];
      if (Math.hypot(nx - me.x, ny - me.y) <= DOOR_RADIUS) found = id;
    }
    if (locked) found = null;
    nearZone = found;
    nearBench = !found && !sitting && !locked ? findNearBench() : null;
    const key = locked ? '' : sitting ? 'stand' : found ? 'zone:' + found : nearBench ? 'bench' : '';
    if (key === hintKey) return;
    hintKey = key;
    const hint = overlay.querySelector('.enter-hint');
    hint.classList.toggle('hidden', !key);
    hint.classList.toggle('sit', key === 'bench' || key === 'stand');
    hint.querySelector('.eh-verb').textContent = sitting ? 'Đứng dậy' : found ? 'Vào' : 'Ngồi nghỉ';
    hint.querySelector('.eh-name').textContent = found ? ZONES[found].name : nearBench ? 'hồi máu ×2' : '';
  }

  // Phím E / Enter hoặc bấm gợi ý: đứng dậy, vào khu, hoặc ngồi ghế gần đó.
  function act() {
    if (locked) return;
    if (sitting) standUp();
    else if (nearZone) enter(nearZone);
    else if (nearBench) sitAt(nearBench);
  }

  // ---------- Ngồi ghế ----------

  const SIT_RADIUS = 48;
  // Chỗ đứng cạnh ghế: trước ghế nếu đi được (ghế ven sông thì phía trước là nước, đứng phía sau).
  function benchSpot(b) {
    for (const dy of [14, -10, -22, 26]) if (footFree(b.x, b.y + dy)) return [b.x, b.y + dy];
    return cellCenter(...nearestWalkableCell(b.x, b.y));
  }
  function findNearBench() {
    let best = null;
    let bd = SIT_RADIUS;
    for (const ch of chunks.values()) {
      for (const b of ch.benches || []) {
        const d = Math.hypot(b.x - me.x, b.y - me.y);
        if (d < bd) { bd = d; best = b; }
      }
    }
    return best;
  }
  function sitAt(b) {
    if (locked || !b) return;
    path = null;
    pathZone = null;
    pathBench = null;
    hideMarker();
    me.x = b.x;
    me.y = b.y + 2;
    me.dir = 'down';
    me.walking = false;
    moving = false;
    sitting = { x: b.x, y: b.y };
    me.el.classList.add('sitting');
    drawMe();
    placeMe();
    updateNear();
    savePos();
    if (opts.onMove) opts.onMove(me.x, me.y);
    if (opts.onSit) opts.onSit(true);
  }
  // Đứng dậy khỏi ghế (notify = false khi server đã tự cho đứng, vd bị xe tông / vào viện).
  function standUp(notify = true) {
    if (!sitting) return;
    const b = sitting;
    sitting = null;
    me.el.classList.remove('sitting');
    [me.x, me.y] = benchSpot(b);
    drawMe();
    placeMe();
    updateNear();
    if (notify && opts.onSit) opts.onSit(false);
  }
  function walkToBench(b) {
    if (locked || !b) return;
    if (sitting) {
      if (sitting.x === b.x && sitting.y === b.y) return;
      standUp();
    }
    hideMarker();
    follow(findPath([me.x, me.y], benchSpot(b)), null);
    pathBench = b;
    if (!path) sitAt(b);
  }

  // Cây đứng trước và che nhân vật thì mờ đi.
  function fadeTrees() {
    for (const ch of chunks.values()) {
      for (const t of ch.trees || []) {
        const cover = me.x > t.l + t.w * 0.15 && me.x < t.l + t.w * 0.85 && me.y < t.t + t.h - 4 && me.y > t.t + 20;
        if (cover !== !!t.faded) {
          t.faded = cover;
          t.el.classList.toggle('fade', cover);
        }
      }
    }
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
    const [x, y] = sitting ? benchSpot(sitting) : [me.x, me.y];
    try { localStorage.setItem('city.pos', JSON.stringify([Math.round(x), Math.round(y)])); } catch (e) { /* bỏ qua */ }
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
    standUp();
    pathBench = null;
    hideMarker();
    follow(findPath([me.x, me.y], NODES[z.node]), zone);
  }

  // Bấm chuột / chạm vào một điểm trên bản đồ.
  function walkToPoint(x, y) {
    if (locked) return;
    standUp();
    pathBench = null;
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
      const bench = pathBench;
      path = null;
      pathZone = null;
      pathBench = null;
      hideMarker();
      stop();
      if (zone) enter(zone);
      else if (bench) sitAt(bench);
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
    standUp(false);
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
        standUp();
        pathBench = null;
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
    stepActors(dt);
    stepCamera(dt, false);
    requestAnimationFrame(tick);
  }

  function afterMove() {
    placeMe();
    updateNear();
    fadeTrees();
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
    } else if ((e.key === 'e' || e.key === 'E' || e.key === 'Enter') && (sitting || nearZone || nearBench) && !locked) {
      e.preventDefault();
      act();
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
    let made = false;
    for (let k = k0; k <= k1; k++) {
      if (chunks.has(k)) continue;
      makeChunk(k);
      made = true;
    }
    if (made && me && overlay) updateNear(); // ghế của mảng mới dựng có thể ở ngay cạnh
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
    layer.innerHTML = cloudsHtml(kx) + plan.props.map(propHtml).join('') + plan.fx.map(fxHtml).join('');
    world.querySelector('.city-props').appendChild(layer);
    // Ghế ngồi được và cây (để làm mờ khi che nhân vật) của mảng này.
    const benches = [...layer.querySelectorAll('[data-bench]')].map((el) => {
      const [bx, by] = el.dataset.bench.split(',').map(Number);
      return { x: bx, y: by, el };
    });
    const trees = [...layer.querySelectorAll('.prop.tree')].map((el) => ({
      el, l: parseFloat(el.style.left), t: parseFloat(el.style.top), w: parseFloat(el.style.width), h: parseFloat(el.style.height),
    }));
    chunks.set(kx, { canvas, props: layer, benches, trees, plan });
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
          <div class="city-dusk" aria-hidden="true"></div>
          <div class="walk-target hidden"></div>
          <div id="me" class="me">
            <div id="me-tag" class="me-tag"><span class="tag-name"></span><span class="tag-lv"></span></div>
            <div class="me-body"></div>
          </div>
        </div>
      </div>
      <div class="city-light" aria-hidden="true"></div>
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
      <button class="enter-hint hidden" type="button"><span class="eh-key">E</span><span class="eh-key">Enter</span><span class="eh-verb">Vào</span> <b class="eh-name"></b></button>
      <div class="city-help${helpHidden ? ' hidden' : ''}">WASD / ↑↓←→ để đi · Bấm chuột để tới · E / Enter để vào
        <button class="link icon-only city-help-close" type="button" title="Ẩn">×</button></div>
      <div class="joystick" aria-label="Cần điều khiển"><div class="joy-knob"></div></div>`;
    el.parentElement.appendChild(overlay);
    overlay.querySelector('.enter-hint').addEventListener('click', act);
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
      const seat = e.target.closest && e.target.closest('[data-bench]');
      if (seat) {
        const [bx, by] = seat.dataset.bench.split(',').map(Number);
        walkToBench({ x: bx, y: by });
        return;
      }
      const r = world.getBoundingClientRect();
      walkToPoint((e.clientX - r.left) / scale, (e.clientY - r.top) / scale);
    });

    window.addEventListener('keydown', onKeyDown);
    window.addEventListener('keyup', onKeyUp);
    window.addEventListener('blur', () => keys.clear());
    fit();
    window.addEventListener('resize', fit);
    applyTime();
    setInterval(applyTime, 60000);
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
    standUp(false);
    path = null;
    pathZone = null;
    pathBench = null;
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
    if (on) standUp(false);
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

  // ---------- Người chơi khác (quảng trường), bong bóng chat, biểu cảm ----------

  const actors = new Map(); // id → { x, y, tx, ty, dir, walking, sitting, look, el, auto, drawn }
  const EMOTES = ['wave', 'heart', 'laugh', 'clap', 'dance'];

  function actorEl(id) {
    const el = document.createElement('div');
    el.className = 'me actor';
    el.dataset.actorId = id;
    el.innerHTML = '<div class="me-tag"><span class="tag-name"></span><span class="tag-lv"></span></div><div class="me-body"></div>';
    el.addEventListener('click', (e) => {
      e.stopPropagation();
      // Không ai đăng ký thì giao cho quảng trường (plaza.js) mở thẻ hồ sơ.
      if (opts.onActorClick) opts.onActorClick(id);
      else if (window.Plaza) window.Plaza.openProfile(id);
    });
    world.appendChild(el);
    return el;
  }
  function placeActor(a) {
    a.el.style.transform = `translate(${a.x}px, ${a.y}px)`;
    a.el.style.zIndex = String(100 + Math.round(a.y));
    const walking = !a.sitting && (a.walking || a.auto);
    const dir = a.dir || 'down';
    const key = `${dir}|${walking}|${a.sitting}|${JSON.stringify(a.look || {})}`;
    if (key === a.drawn || !C || typeof C.sprite !== 'function') return;
    a.drawn = key;
    a.el.querySelector('.me-body').innerHTML = C.sprite(a.look || {}, { dir: a.sitting ? 'down' : dir, walking, size: SPRITE_SIZE, pose: a.sitting ? 'sit' : undefined });
    a.el.classList.toggle('sitting', !!a.sitting);
  }
  // Đi mượt tới vị trí mới nhận được (nội suy); xa quá thì nhảy luôn.
  function stepActors(dt) {
    for (const a of actors.values()) {
      const dx = a.tx - a.x;
      const dy = a.ty - a.y;
      const d = Math.hypot(dx, dy);
      const was = a.auto;
      if (d > 0.5) {
        const step = Math.max(SPEED * 1.1 * dt, d * (1 - Math.exp(-dt * 4)));
        if (step >= d) { a.x = a.tx; a.y = a.ty; } else { a.x += (dx / d) * step; a.y += (dy / d) * step; }
        a.auto = d > 3;
        if (a.autoDir) a.dir = dirOf(dx, dy);
      } else {
        a.auto = false;
      }
      if (d > 0.5 || was !== a.auto) placeActor(a);
    }
  }
  function showBubble(el, text, ms) {
    if (!el) return;
    let b = el.querySelector('.say');
    if (!b) {
      b = document.createElement('div');
      b.className = 'say';
      el.appendChild(b);
    }
    b.textContent = String(text || '').slice(0, 120);
    el.classList.add('talking'); // đang nói: nổi lên trên cây / biển tên để đọc được
    clearTimeout(b.timer);
    b.timer = setTimeout(() => {
      b.remove();
      el.classList.remove('talking');
    }, ms || 5000);
  }
  function showEmote(el, kind) {
    if (!el || !EMOTES.includes(kind)) return;
    const i = document.createElement('img');
    i.className = 'emote';
    i.alt = '';
    i.src = srcOf('emote_' + kind);
    el.appendChild(i);
    EMOTES.forEach((k) => el.classList.remove('emo-' + k));
    void el.offsetWidth;
    el.classList.add('emo-' + kind);
    setTimeout(() => {
      i.remove();
      el.classList.remove('emo-' + kind);
    }, 2200);
  }

  const Actors = {
    // Thêm / cập nhật người chơi khác: { look, name, level, x, y, dir, walking, sitting } (trường nào thiếu thì giữ nguyên).
    upsert(id, st = {}) {
      if (!world) return;
      let a = actors.get(id);
      if (!a) {
        a = { x: st.x || 0, y: st.y || 0, tx: st.x || 0, ty: st.y || 0, dir: 'down', el: actorEl(id) };
        actors.set(id, a);
      }
      if (typeof st.x === 'number') a.tx = st.x;
      if (typeof st.y === 'number') a.ty = st.y;
      if (Math.hypot(a.tx - a.x, a.ty - a.y) > 640) { a.x = a.tx; a.y = a.ty; }
      for (const k of ['look', 'walking', 'sitting']) if (st[k] !== undefined) a[k] = st[k];
      if (st.dir) { a.dir = st.dir; a.autoDir = false; } else if (a.autoDir === undefined) a.autoDir = true;
      if (st.name !== undefined) a.el.querySelector('.tag-name').textContent = st.name;
      if (st.level !== undefined) a.el.querySelector('.tag-lv').textContent = 'Lv ' + st.level;
      placeActor(a);
    },
    remove(id) {
      const a = actors.get(id);
      if (!a) return;
      a.el.remove();
      actors.delete(id);
    },
    bubble(id, text, ms) { const a = actors.get(id); if (a) showBubble(a.el, text, ms); },
    emote(id, kind) { const a = actors.get(id); if (a) showEmote(a.el, kind); },
    ids: () => [...actors.keys()],
  };

  // Ban đêm (18h–6h theo giờ máy, hoặc ?time=night|day / localStorage 'city.time'): bản đồ tối ấm, đèn đường và cửa sổ sáng.
  function applyTime() {
    if (!root) return;
    let mode = null;
    try { mode = new URLSearchParams(location.search).get('time') || localStorage.getItem('city.time'); } catch (e) { /* bỏ qua */ }
    const h = new Date().getHours();
    const night = mode === 'night' || (mode !== 'day' && (h >= 18 || h < 6));
    root.classList.toggle('night', night);
    root.classList.toggle('dusk', !night && mode !== 'day' && (h === 17 || h === 6));
  }

  window.City = {
    ZONES, mount, walkTo, setLook, setTag, setSub, setBadge, teleport, setLocked, zoneX,
    position: () => me && [me.x, me.y],
    // Trạng thái nhân vật mình (để gửi cho người khác).
    state: () => me && { x: me.x, y: me.y, dir: me.dir, walking: !!me.walking, sitting: !!sitting, zone: nearZone },
    isSitting: () => !!sitting,
    // Kiểm tự động: các món (toà, nhà, cây, đèn, ghế...) lấn lòng đường / vỉa hè trong các mảng đang dựng.
    audit: () => [...chunks.entries()].flatMap(([kx, ch]) => (ch.plan ? ch.plan.props : [])
      .map((p) => [kx, p[0], Math.round(p[1]), Math.round(p[2]), violation(p)]).filter((v) => v[4])),
    // Đồng bộ theo server (vd server đã cho đứng): false = đứng dậy, không báo lại qua onSit.
    setSitting: (on) => { if (!on) standUp(false); },
    actors: Actors,
    bubbleMe: (text, ms) => me && showBubble(me.el, text, ms),
    emoteMe: (kind) => me && showEmote(me.el, kind),
  };
})();
