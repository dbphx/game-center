// Trường đua: dựng cảnh 3D (three.js) — đường đua đêm, rào mũi tên, neon, xe Kenney CC0 kèm đồ nâng cấp, hiệu ứng.
import * as THREE from 'three';
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';
import { EffectComposer } from 'three/addons/postprocessing/EffectComposer.js';
import { RenderPass } from 'three/addons/postprocessing/RenderPass.js';
import { UnrealBloomPass } from 'three/addons/postprocessing/UnrealBloomPass.js';
import { OutputPass } from 'three/addons/postprocessing/OutputPass.js';

const M = '/race/models/';
const IMG = '/race/img/';
const CAR_WIDTH = 2.0; // mọi xe thu phóng về cùng bề ngang (m)

// Mẫu xe server (games/race/cars.go) → mô hình Kenney. open: mui trần, cần thêm người lái.
export const CAR_MODELS = {
  starter: { file: 'car/kart-oozi.glb', open: true, seat: [0, 0.2, -0.15], hideChar: true },
  comet: { file: 'car/hatchback-sports.glb', open: false },
  bolt: { file: 'car/sedan-sports.glb', open: false },
  viper: { file: 'kit/raceCarRed.glb', open: true, seat: [0, 0.62, -0.25], paintMat: 'red' },
  phantom: { file: 'car/race.glb', open: true, seat: [0, 0.6, -0.35] },
  nova: { file: 'car/race-future.glb', open: true, seat: [0, 0.66, -0.3] },
  // Xe thể thao CC0 (Quaternius, scaranto qua Poly Pizza). Bánh liền khối nên không xoay / không thay mâm.
  sprint: { file: 'cc0/sports-car-white.glb', open: false, paintMat: ['White'], fixedWheels: true },
  raptor: { file: 'cc0/sports-car-orange.glb', open: false, paintMat: ['Orange', 'DarkOrange'], fixedWheels: true },
  zenith: { file: 'cc0/sports-car-striped.glb', open: false, tint: 0.55, fixedWheels: true },
  vortex: { file: 'cc0/sports-car-armored.glb', open: false, tint: 0.5, fixedWheels: true },
  apex: { file: 'kit/raceCarGreen.glb', open: true, seat: [0, 0.62, -0.25], paintMat: ['grass'] },
  blaze: { file: 'cc0/formula-car.glb', open: true, seat: [0, 0.5, -0.05], paintMat: ['Material.005'], fixedWheels: true, turn: Math.PI / 2 },
};
// Moto dựng bằng khối trong three.js (không có mô hình moto CC0 hợp phong cách): style chọn kiểu thân.
export const MOTO_MODELS = {
  moto_dash: { style: 'street' },
  moto_storm: { style: 'sport' },
  moto_neon: { style: 'neon' },
};
export const isMoto = (id) => !!MOTO_MODELS[id] || /^moto/.test(String(id || ''));
const ORDER = ['starter', 'comet', 'bolt', 'viper', 'phantom', 'nova'];
export const modelOf = (id) => CAR_MODELS[id] || CAR_MODELS[ORDER[Math.abs(hash(id)) % ORDER.length]];

export const PAINT = { red: 0xe8333c, blue: 0x2f7cf6, yellow: 0xffc928, green: 0x2fcf6a, white: 0xf4f6fb, black: 0x23252e, orange: 0xff7a1f, purple: 0x9b4dff, pink: 0xff5fb4, cyan: 0x2fe6ff };
const colorOf = (id, fallback) => {
  const k = Object.keys(PAINT).find((c) => String(id || '').includes(c));
  return k ? PAINT[k] : fallback;
};
function hash(s) {
  let h = 0;
  for (const ch of String(s || '')) h = (h * 31 + ch.charCodeAt(0)) | 0;
  return h;
}

// ---------- Tải mô hình (có bộ nhớ đệm) ----------
const loader = new GLTFLoader();
const cache = new Map();
export function loadModel(file) {
  if (!cache.has(file)) cache.set(file, loader.loadAsync(M + file).then((g) => g.scene));
  return cache.get(file);
}
const texLoader = new THREE.TextureLoader();
const texCache = new Map();
function tex(name, repeat) {
  const key = name + (repeat ? 'r' : '');
  if (!texCache.has(key)) {
    const t = texLoader.load(IMG + name);
    t.colorSpace = THREE.SRGBColorSpace;
    if (repeat) t.wrapS = t.wrapT = THREE.RepeatWrapping;
    t.anisotropy = 4;
    texCache.set(key, t);
  }
  return texCache.get(key);
}

// recenter đặt mô hình nằm giữa theo x/z, đáy chạm y = 0; trả kích thước gốc.
function recenter(obj) {
  const box = new THREE.Box3().setFromObject(obj);
  const size = box.getSize(new THREE.Vector3());
  const c = box.getCenter(new THREE.Vector3());
  obj.position.x -= c.x;
  obj.position.z -= c.z;
  obj.position.y -= box.min.y;
  return size;
}

// ---------- Renderer ----------
export function createRenderer(canvas, low) {
  const renderer = new THREE.WebGLRenderer({ canvas, antialias: !low, powerPreference: 'high-performance' });
  renderer.setPixelRatio(low ? Math.min(window.devicePixelRatio, 1) * 0.75 : Math.min(window.devicePixelRatio, 1.5));
  renderer.outputColorSpace = THREE.SRGBColorSpace;
  renderer.toneMapping = THREE.ACESFilmicToneMapping;
  renderer.toneMappingExposure = 1.05;
  let scene = new THREE.Scene();
  let camera = new THREE.PerspectiveCamera(72, 1, 0.3, 1500);
  let composer = null;
  let bloom = null;
  let pass = null;
  if (!low) {
    composer = new EffectComposer(renderer);
    pass = new RenderPass(scene, camera);
    composer.addPass(pass);
    bloom = new UnrealBloomPass(new THREE.Vector2(256, 256), 0.85, 0.45, 0.82);
    composer.addPass(bloom);
    composer.addPass(new OutputPass());
  }
  const resize = () => {
    const w = canvas.clientWidth || window.innerWidth;
    const h = canvas.clientHeight || window.innerHeight;
    renderer.setSize(w, h, false);
    if (composer) composer.setSize(w, h);
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
  };
  resize();
  return {
    renderer, resize,
    get scene() { return scene; },
    get camera() { return camera; },
    // use đổi cảnh đang vẽ (garage / đường đua) mà giữ nguyên renderer và bloom.
    use(s, c) {
      scene = s;
      camera = c;
      if (pass) { pass.scene = s; pass.camera = c; }
      resize();
    },
    render: () => (composer ? composer.render() : renderer.render(scene, camera)),
    dispose: () => { renderer.dispose(); if (composer) composer.dispose(); },
  };
}

// ---------- Đường đua ----------

// ribbon dựng dải dọc đường tâm, từ độ lệch l0 tới l1 (m, dương = bên phải), ở độ cao y; uv: u ngang, v dọc theo vRep m.
function ribbon(path, cum, l0, l1, y, vRep, y1 = y) {
  const n = path.length;
  const pos = [];
  const uv = [];
  const idx = [];
  for (let i = 0; i <= n; i++) {
    const p = path[i % n];
    const a = path[(i - 1 + n) % n];
    const b = path[(i + 1) % n];
    let dx = b.x - a.x;
    let dz = b.z - a.z;
    const len = Math.hypot(dx, dz) || 1;
    dx /= len;
    dz /= len;
    const rx = dz;
    const rz = -dx;
    pos.push(p.x + rx * l0, y, p.z + rz * l0, p.x + rx * l1, y1, p.z + rz * l1);
    const v = cum[i] / vRep;
    uv.push(0, v, 1, v);
    if (i < n) {
      const k = i * 2;
      idx.push(k, k + 2, k + 1, k + 1, k + 2, k + 3);
    }
  }
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3));
  g.setAttribute('uv', new THREE.Float32BufferAttribute(uv, 2));
  g.setIndex(idx);
  g.computeVertexNormals();
  return g;
}

// wall dựng tường đứng ở độ lệch l, cao h; mặt hướng vào trong đường. Đoạn cua phía ngoài dùng vật liệu mũi tên (nhóm 1).
function wall(path, cum, curv, l, h, side) {
  const n = path.length;
  const pos = [];
  const uv = [];
  const g = new THREE.BufferGeometry();
  let vtx = 0;
  let runStart = 0;
  let runMat = -1;
  const groups = [];
  for (let i = 0; i < n; i++) {
    const a = path[i];
    const b = path[(i + 1) % n];
    const p0 = path[(i - 1 + n) % n];
    const p2 = path[(i + 2) % n];
    const off = (p, q, r) => {
      let dx = r.x - q.x;
      let dz = r.z - q.z;
      const len = Math.hypot(dx, dz) || 1;
      return [p.x + (dz / len) * l, p.z + (-dx / len) * l];
    };
    const [ax, az] = off(a, p0, b);
    const [bx, bz] = off(b, a, p2);
    // Cua mà tường nằm ở phía ngoài thì là rào mũi tên.
    const outer = curv[i] * side < -0.004;
    const mat = outer ? 1 : 0;
    const u0 = cum[i] / 3.2;
    const u1 = cum[i + 1] / 3.2;
    // Mặt hướng vào tâm đường: thứ tự đỉnh theo phía.
    const quad = side > 0
      ? [ax, 0, az, bx, 0, bz, ax, h, az, bx, h, bz]
      : [bx, 0, bz, ax, 0, az, bx, h, bz, ax, h, az];
    const quv = side > 0 ? [u0, 0, u1, 0, u0, 1, u1, 1] : [u1, 0, u0, 0, u1, 1, u0, 1];
    pos.push(...quad.slice(0, 9), ...quad.slice(3, 6), ...quad.slice(9, 12), ...quad.slice(6, 9));
    uv.push(...quv.slice(0, 6), ...quv.slice(2, 4), ...quv.slice(6, 8), ...quv.slice(4, 6));
    if (mat !== runMat) {
      if (runMat >= 0) groups.push([runStart, vtx - runStart, runMat]);
      runStart = vtx;
      runMat = mat;
    }
    vtx += 6;
  }
  groups.push([runStart, vtx - runStart, runMat]);
  g.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3));
  g.setAttribute('uv', new THREE.Float32BufferAttribute(uv, 2));
  for (const [s, c, m] of groups) g.addGroup(s, c, m);
  g.computeVertexNormals();
  return g;
}

export async function buildWorld(scene, track, low) {
  const { path, cum } = track;
  const n = path.length;
  const night = track.data.id === 'city' ? 0x0d1430 : 0x0b0f2a;
  scene.background = new THREE.Color(night);
  scene.fog = new THREE.Fog(night, 90, low ? 260 : 420);

  scene.add(new THREE.HemisphereLight(0x5a74c8, 0x101418, 1.1));
  const moon = new THREE.DirectionalLight(0xbcd2ff, 1.4);
  moon.position.set(-60, 120, -40);
  scene.add(moon);
  scene.add(new THREE.AmbientLight(0x30365a, 0.6));

  // Độ cong tại từng điểm (dấu: rẽ phải âm/dương theo pháp tuyến) để biết phía ngoài khúc cua.
  const curv = new Float32Array(n);
  for (let i = 0; i < n; i++) {
    const a = path[(i - 2 + n) % n];
    const b = path[i];
    const c = path[(i + 2) % n];
    const h1 = Math.atan2(b.x - a.x, b.z - a.z);
    const h2 = Math.atan2(c.x - b.x, c.z - b.z);
    let d = h2 - h1;
    while (d > Math.PI) d -= Math.PI * 2;
    while (d < -Math.PI) d += Math.PI * 2;
    curv[i] = d / (Math.hypot(c.x - a.x, c.z - a.z) || 1);
  }

  const ground = new THREE.Mesh(
    new THREE.PlaneGeometry(1400, 1400),
    new THREE.MeshStandardMaterial({ map: (() => { const t = tex('ground.png', true); t.repeat.set(140, 140); return t; })(), roughness: 1 }),
  );
  ground.rotation.x = -Math.PI / 2;
  ground.position.y = -0.02;
  scene.add(ground);

  const half = track.half;
  const road = new THREE.Mesh(ribbon(path, cum, -half, half, 0.02, 14), new THREE.MeshStandardMaterial({ map: tex('asphalt.png', true), roughness: 0.62, metalness: 0.05 }));
  scene.add(road);
  const curbMat = new THREE.MeshStandardMaterial({ map: tex('curb.png', true), roughness: 0.8 });
  scene.add(new THREE.Mesh(ribbon(path, cum, half, half + 1.3, 0.05, 4), curbMat));
  scene.add(new THREE.Mesh(ribbon(path, cum, -half - 1.3, -half, 0.05, 4), curbMat));

  // Tường: phần thường màu tối có viền neon trên đỉnh, phần ngoài khúc cua là rào mũi tên vàng đen.
  const chevron = tex('chevron.png', true);
  const wallMats = [
    new THREE.MeshStandardMaterial({ color: 0x1b1f33, roughness: 0.7 }),
    new THREE.MeshStandardMaterial({ map: chevron, emissive: 0x3a2a00, emissiveMap: chevron, roughness: 0.5 }),
  ];
  const W = track.wall + 0.4;
  scene.add(new THREE.Mesh(wall(path, cum, curv, W, 1.3, 1), wallMats));
  scene.add(new THREE.Mesh(wall(path, cum, curv, -W, 1.3, -1), wallMats));
  const neonR = new THREE.MeshBasicMaterial({ color: track.data.id === 'city' ? 0xff3fd0 : 0x2fe6ff, toneMapped: false });
  const neonL = new THREE.MeshBasicMaterial({ color: track.data.id === 'city' ? 0x3fa0ff : 0xff3fd0, toneMapped: false });
  scene.add(new THREE.Mesh(ribbon(path, cum, W - 0.12, W + 0.12, 1.34, 4), neonR));
  scene.add(new THREE.Mesh(ribbon(path, cum, -W - 0.12, -W + 0.12, 1.34, 4), neonL));

  // Vạch xuất phát ô cờ.
  const ck = document.createElement('canvas');
  ck.width = 64;
  ck.height = 16;
  const cx = ck.getContext('2d');
  for (let i = 0; i < 16; i++) for (let j = 0; j < 4; j++) { cx.fillStyle = (i + j) % 2 ? '#111' : '#f4f4f4'; cx.fillRect(i * 4, j * 4, 4, 4); }
  const ckTex = new THREE.CanvasTexture(ck);
  ckTex.magFilter = THREE.NearestFilter;
  ckTex.colorSpace = THREE.SRGBColorSpace;
  const start = new THREE.Mesh(new THREE.PlaneGeometry(track.width, 2.4), new THREE.MeshStandardMaterial({ map: ckTex, roughness: 0.7 }));
  start.rotation.x = -Math.PI / 2;
  start.rotation.z = -Math.atan2(path[1].x - path[0].x, path[1].z - path[0].z);
  start.position.set(path[0].x, 0.04, path[0].z);
  scene.add(start);

  // Sao trên trời.
  const sp = [];
  for (let i = 0; i < (low ? 300 : 900); i++) {
    const th = Math.random() * Math.PI * 2;
    const ph = Math.random() * 0.45 + 0.08;
    const r = 900;
    sp.push(Math.cos(th) * Math.cos(ph) * r, Math.sin(ph) * r, Math.sin(th) * Math.cos(ph) * r);
  }
  const sg = new THREE.BufferGeometry();
  sg.setAttribute('position', new THREE.Float32BufferAttribute(sp, 3));
  const stars = new THREE.Points(sg, new THREE.PointsMaterial({ color: 0xcfe0ff, size: 2, sizeAttenuation: false, fog: false }));
  scene.add(stars);

  // Đồ trang trí Racing Kit (khán đài, đèn, biển...). Đầu đèn thêm quầng sáng để bloom.
  const glowTex = tex('spark.png');
  const glowMat = new THREE.SpriteMaterial({ map: glowTex, color: 0xfff1c8, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false });
  const pieces = track.data.pieces || [];
  const files = [...new Set(pieces.map((p) => p.model))];
  const proto = {};
  await Promise.all(files.map(async (m) => {
    try {
      const s = (await loadModel(`kit/${m}.glb`)).clone(true);
      const wrap = new THREE.Group();
      wrap.add(s);
      recenter(s);
      proto[m] = wrap;
    } catch (e) { /* thiếu mô hình thì bỏ qua đồ trang trí đó */ }
  }));
  for (const p of pieces) {
    const base = proto[p.model];
    if (!base) continue;
    const o = base.clone(true);
    o.scale.setScalar(p.scale || 10);
    o.position.set(p.x, 0, p.z);
    o.rotation.y = p.rot || 0;
    scene.add(o);
    if (/light/i.test(p.model) && !low) {
      const g = new THREE.Sprite(glowMat);
      g.scale.setScalar(p.model === 'overheadLights' ? 10 : 5);
      g.position.set(p.x, (p.scale || 10) * (p.model === 'overheadLights' ? 0.62 : 0.74), p.z);
      scene.add(g);
    }
  }
  return { curv };
}

// ---------- Xe ----------

function metal(color) { return new THREE.MeshStandardMaterial({ color, metalness: 0.85, roughness: 0.3 }); }
function glow(color) { return new THREE.MeshBasicMaterial({ color, toneMapped: false }); }

// buildCar dựng xe theo mẫu và đồ đang lắp: spec {car, levels:{engine,turbo,tires}, cosmetics:{spoiler,neon,paint,decal}, number}.
export async function buildCar(spec, low = false) {
  if (isMoto(spec.car)) return buildMoto(spec, low);
  const info = modelOf(spec.car);
  const src = await loadModel(info.file);
  const body = src.clone(true);
  const root = new THREE.Group(); // gốc xe: tâm giữa hai trục, đáy y = 0, mũi xe hướng +z
  const holder = new THREE.Group();
  if (info.turn) body.rotation.y = info.turn; // mô hình nhìn ngược hướng thì quay lại cho mũi xe hướng +z
  holder.add(body);
  root.add(holder);
  const size = recenter(body);
  const s = CAR_WIDTH / Math.max(size.x, 0.01);
  holder.scale.setScalar(s);
  const L = size.z * s;
  const H = size.y * s;
  const lv = spec.levels || {};
  const cos = spec.cosmetics || {};
  const paint = colorOf(cos.paint, PAINT.red);

  // Sơn: vật liệu tên màu (Racing Kit) đổi hẳn màu; mô hình dùng bảng màu chung thì nhuộm phần thân.
  body.traverse((o) => {
    if (!o.isMesh) return;
    const isWheel = /wheel/i.test(o.name) || /wheel/i.test(o.parent && o.parent.name);
    if (isWheel || /character/i.test(o.name) || /character/i.test(o.parent && o.parent.name)) return;
    const mats = Array.isArray(o.material) ? o.material : [o.material];
    const next = mats.map((m) => {
      const c = m.clone();
      const pm = info.paintMat && [].concat(info.paintMat);
      if (pm) {
        if (pm.includes(m.name)) c.color.setHex(paint);
      } else {
        c.color.setHex(0xffffff).lerp(new THREE.Color(paint), info.tint ?? 0.72);
      }
      c.metalness = 0.35;
      c.roughness = 0.32;
      return c;
    });
    o.material = Array.isArray(o.material) ? next : next[0];
  });
  // Bánh xe chỉ giữ các nút bánh (không lặp con).
  const wheelNodes = [];
  if (!info.fixedWheels) body.traverse((o) => { if (/wheel/i.test(o.name) && !/wheel/i.test(o.parent && o.parent.name)) wheelNodes.push(o); });
  for (const w of wheelNodes) w.rotation.order = 'YXZ';

  // Lốp / mâm: cấp 2–3 đổi mâm tối, cấp 4–5 mâm đua có viền phát sáng.
  const tires = lv.tires || 0;
  if (tires >= 2) {
    const wfile = tires >= 4 ? 'car/wheel-racing.glb' : 'car/wheel-dark.glb';
    const wsrc = await loadModel(wfile);
    root.updateMatrixWorld(true);
    for (const w of wheelNodes) {
      const wb = new THREE.Box3().setFromObject(w);
      const wsz = wb.getSize(new THREE.Vector3());
      w.children.forEach((c) => (c.visible = false));
      if (w.isMesh) w.material = new THREE.MeshBasicMaterial({ visible: false });
      const nw = wsrc.clone(true);
      const nb = new THREE.Box3().setFromObject(nw).getSize(new THREE.Vector3());
      const k = (wsz.y / s / (nb.y || 1)) * s;
      // wsz đang ở hệ thế giới của mô hình đã thu phóng: quy về hệ nút bánh.
      nw.scale.setScalar(k / s);
      const side = w.position.x >= 0 ? 1 : -1;
      nw.rotation.y = side > 0 ? 0 : Math.PI;
      w.add(nw);
      if (tires >= 4) {
        const wr = (wsz.y / s) * 0.36; // bán kính viền sáng theo cỡ bánh (hệ nút bánh)
        const ring = new THREE.Mesh(new THREE.TorusGeometry(wr, wr * 0.1, 6, 20), glow(colorOf(cos.neon, 0x2fe6ff)));
        ring.rotation.y = Math.PI / 2;
        ring.position.x = side * (wsz.x / s) * 0.5;
        w.add(ring);
      }
    }
  }

  const extras = new THREE.Group(); // đồ gắn thêm theo hệ mét của xe
  root.add(extras);
  const rear = -L / 2;
  // Bánh liền khối (mô hình CC0 ngoài Kenney): lốp cấp 4–5 là vòng sáng ở vị trí bánh ước lượng.
  if (info.fixedWheels && tires >= 4) {
    const wr = H * 0.17;
    for (const zf of [0.32, -0.32]) for (const side of [-1, 1]) {
      const ring = new THREE.Mesh(new THREE.TorusGeometry(wr, wr * 0.12, 6, 20), glow(colorOf(cos.neon, 0x2fe6ff)));
      ring.rotation.y = Math.PI / 2;
      ring.position.set(side * CAR_WIDTH * 0.51, wr * 1.15, zf * L);
      extras.add(ring);
    }
  }

  // Người lái: mũ bảo hiểm và kính phát sáng (xe mui trần).
  if (info.open) {
    const [sx, sy, sz] = info.seat;
    // Mô hình kart có sẵn nhân vật đầu to: ẩn đi, dùng người lái chung cho cân đối.
    if (info.hideChar) body.traverse((o) => { if (/character/i.test(o.name)) o.visible = false; });
    const d = new THREE.Group();
    const suit = new THREE.Mesh(new THREE.BoxGeometry(0.34, 0.3, 0.26), new THREE.MeshStandardMaterial({ color: 0x1d2235, roughness: 0.6 }));
    suit.position.y = 0.15;
    // Mũ bảo hiểm màu tương phản với sơn, có sọc và kính phát sáng ôm phía trước.
    const shell = paint === PAINT.white || paint === PAINT.yellow ? 0x2f7cf6 : 0xf2f4f8;
    const helmet = new THREE.Mesh(new THREE.SphereGeometry(0.17, 16, 12), new THREE.MeshStandardMaterial({ color: shell, metalness: 0.2, roughness: 0.45 }));
    helmet.position.y = 0.42;
    helmet.scale.set(1, 0.95, 1.08);
    const stripe = new THREE.Mesh(new THREE.TorusGeometry(0.168, 0.018, 6, 24, Math.PI), new THREE.MeshStandardMaterial({ color: paint, roughness: 0.4 }));
    stripe.rotation.set(0, Math.PI / 2, Math.PI / 2);
    stripe.position.y = 0.42;
    const visor = new THREE.Mesh(new THREE.SphereGeometry(0.172, 16, 8, Math.PI * 0.18, Math.PI * 0.64, Math.PI * 0.36, Math.PI * 0.2), glow(0x58f0ff));
    visor.position.y = 0.42;
    d.add(suit, helmet, stripe, visor);
    d.position.set(sx * s, sy * H, sz * L * 0.5);
    extras.add(d);
  }

  // Động cơ / ống xả: ống to dần theo cấp, cấp 4+ có 4 ống. Lửa xanh khi nitro.
  const engine = lv.engine || 0;
  const pipes = engine >= 4 ? 4 : 2;
  const r = 0.05 + engine * 0.012;
  const flames = [];
  for (let i = 0; i < pipes; i++) {
    const x = (i - (pipes - 1) / 2) * 0.22;
    const pipe = new THREE.Mesh(new THREE.CylinderGeometry(r, r * 1.15, 0.3 + engine * 0.03, 10), metal(0xb8bcc8));
    pipe.rotation.x = Math.PI / 2;
    pipe.position.set(x, H * 0.28, rear + 0.05);
    extras.add(pipe);
    const flame = new THREE.Mesh(new THREE.ConeGeometry(r * 1.6, 1.1, 10, 1, true), new THREE.MeshBasicMaterial({ color: 0x46b8ff, transparent: true, opacity: 0.85, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }));
    flame.rotation.x = -Math.PI / 2;
    flame.position.set(x, H * 0.28, rear - 0.6);
    flame.visible = false;
    extras.add(flame);
    flames.push(flame);
  }

  // Turbo: bình nằm ngang sau lưng, cấp 3+ có hai bình, cấp 5 có vòng sáng.
  const turbo = lv.turbo || 0;
  if (turbo >= 1) {
    const n = turbo >= 3 ? 2 : 1;
    for (let i = 0; i < n; i++) {
      const can = new THREE.Mesh(new THREE.CylinderGeometry(0.1 + turbo * 0.012, 0.1 + turbo * 0.012, 0.3 + turbo * 0.05, 14), metal(0x9aa3b8));
      can.rotation.z = Math.PI / 2;
      can.position.set((i - (n - 1) / 2) * 0.4, H * (info.open ? 0.62 : 0.72), rear + 0.5);
      extras.add(can);
      if (turbo >= 5) {
        const ring = new THREE.Mesh(new THREE.TorusGeometry(0.13, 0.02, 6, 18), glow(0xff5a2a));
        ring.rotation.y = Math.PI / 2;
        ring.position.copy(can.position);
        extras.add(ring);
      }
    }
  }

  // Cánh gió (đồ trang trí). Xe có cánh sẵn thì ẩn cánh gốc khi lắp cánh mới.
  if (cos.spoiler) {
    body.traverse((o) => { if (/spoiler/i.test(o.name)) o.visible = false; });
    let wing;
    if (cos.spoiler === 'spoiler_gt') {
      wing = new THREE.Group();
      const m = new THREE.MeshStandardMaterial({ color: 0x15171f, metalness: 0.5, roughness: 0.35 });
      const blade = new THREE.Mesh(new THREE.BoxGeometry(2.1, 0.06, 0.5), m);
      blade.position.y = 0.55;
      const s1 = new THREE.Mesh(new THREE.BoxGeometry(0.06, 0.55, 0.2), m);
      s1.position.set(-0.6, 0.27, 0);
      const s2 = s1.clone();
      s2.position.x = 0.6;
      const e1 = new THREE.Mesh(new THREE.BoxGeometry(0.05, 0.3, 0.55), new THREE.MeshStandardMaterial({ color: paint, metalness: 0.4, roughness: 0.3 }));
      e1.position.set(-1.05, 0.62, 0);
      const e2 = e1.clone();
      e2.position.x = 1.05;
      wing.add(blade, s1, s2, e1, e2);
    } else {
      wing = (await loadModel(cos.spoiler === 'spoiler_mid' ? 'car/debris-spoiler-b.glb' : 'car/debris-spoiler-a.glb')).clone(true);
      recenter(wing);
      const g = new THREE.Group();
      g.add(wing);
      g.scale.setScalar(1.7);
      wing = g;
    }
    wing.position.set(0, H * (info.open ? 0.62 : 0.78), rear + 0.35);
    extras.add(wing);
  }

  // Đèn neon gầm: quầng sáng màu trên mặt đường.
  let underglow = null;
  if (cos.neon) {
    const c = colorOf(cos.neon, 0x2fe6ff);
    underglow = new THREE.Mesh(new THREE.PlaneGeometry(CAR_WIDTH * 1.5, L * 1.25), new THREE.MeshBasicMaterial({ map: tex('spark.png'), color: c, transparent: true, opacity: 0.5, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }));
    underglow.rotation.x = -Math.PI / 2;
    underglow.position.y = 0.06;
    root.add(underglow);
    const strip = new THREE.Mesh(new THREE.BoxGeometry(CAR_WIDTH * 0.92, 0.04, L * 0.8), glow(c));
    strip.position.y = 0.12;
    root.add(strip);
  }

  // Decal: số đua (chữ vẽ trên canvas) hoặc hình PNG dán trên nắp xe.
  if (cos.decal) {
    let map;
    if (cos.decal === 'decal_number') {
      const cv = document.createElement('canvas');
      cv.width = cv.height = 128;
      const x = cv.getContext('2d');
      x.fillStyle = '#ffffff';
      x.beginPath();
      x.arc(64, 64, 58, 0, Math.PI * 2);
      x.fill();
      x.fillStyle = '#111';
      x.font = 'bold 76px sans-serif';
      x.textAlign = 'center';
      x.textBaseline = 'middle';
      x.fillText(String(spec.number || 7), 64, 70);
      map = new THREE.CanvasTexture(cv);
      map.colorSpace = THREE.SRGBColorSpace;
    } else {
      map = tex(`${cos.decal}.png`);
    }
    const plate = new THREE.Mesh(new THREE.PlaneGeometry(cos.decal === 'decal_number' ? 0.7 : 1.1, cos.decal === 'decal_number' ? 0.7 : L * 0.45), new THREE.MeshBasicMaterial({ map, transparent: true, depthWrite: false, polygonOffset: true, polygonOffsetFactor: -4 }));
    plate.rotation.x = -Math.PI / 2;
    plate.position.set(0, H * (info.open ? 0.45 : 0.55), L * 0.22);
    extras.add(plate);
  }

  // Đèn pha phát sáng phía trước, đèn hậu đỏ.
  for (const x of [-0.6, 0.6]) {
    const hl = new THREE.Mesh(new THREE.BoxGeometry(0.28, 0.1, 0.05), glow(0xeaf6ff));
    hl.position.set(x, H * 0.32, L / 2 + 0.02);
    extras.add(hl);
    const tl = new THREE.Mesh(new THREE.BoxGeometry(0.26, 0.08, 0.05), glow(0xff2a3a));
    tl.position.set(x, H * 0.4, rear - 0.02);
    extras.add(tl);
  }

  // Bóng mềm dưới xe.
  const shadow = new THREE.Mesh(new THREE.PlaneGeometry(CAR_WIDTH * 1.4, L * 1.15), new THREE.MeshBasicMaterial({ map: tex('spark.png'), color: 0x000000, transparent: true, opacity: 0.55, depthWrite: false }));
  shadow.rotation.x = -Math.PI / 2;
  shadow.position.y = 0.035;
  root.add(shadow);

  let spin = 0;
  let t = 0;
  return {
    root, length: L, height: H, flames,
    // update xoay bánh theo tốc độ, bẻ bánh trước, lắc thân khi drift, bật lửa khi nitro.
    update(speed, steer, drift, boost, dt) {
      t += dt;
      spin += (speed / 0.42) * dt;
      for (const w of wheelNodes) {
        w.rotation.x = spin;
        w.rotation.y = /front/i.test(w.name) ? steer * 0.45 : 0;
      }
      holder.rotation.z = THREE.MathUtils.lerp(holder.rotation.z, (drift ? 0.06 : 0.02) * steer, Math.min(1, dt * 6));
      for (const f of flames) {
        f.visible = boost;
        if (boost) f.scale.set(1, 0.8 + Math.sin(t * 60 + f.id) * 0.25, 1);
      }
      if (underglow) underglow.material.opacity = 0.45 + Math.sin(t * 3) * 0.1;
    },
  };
}

// ---------- Moto (dựng bằng khối) ----------

// buildMoto dựng moto theo kiểu (street / sport / neon) với người lái cúi người, đồ nâng cấp như xe hơi
// (ống xả, bình turbo, mâm), neon gầm, sơn, decal. Không lắp cánh gió. Nghiêng vào cua khi lái.
export function buildMoto(spec) {
  const style = (MOTO_MODELS[spec.car] || { style: 'street' }).style;
  const lv = spec.levels || {};
  const cos = spec.cosmetics || {};
  const paint = colorOf(cos.paint, style === 'neon' ? PAINT.black : PAINT.red);
  const accent = colorOf(cos.neon, style === 'neon' ? 0xff3fd0 : 0x2fe6ff);
  const root = new THREE.Group();
  const lean = new THREE.Group(); // nghiêng quanh vệt bánh trên mặt đường
  const MOTO_SCALE = 1.3; // moto hẹp hơn ô tô nhiều: phóng to cho dễ nhìn (chỉ để vẽ)
  lean.scale.setScalar(MOTO_SCALE);
  root.add(lean);
  const bodyMat = new THREE.MeshStandardMaterial({ color: paint, metalness: 0.45, roughness: 0.28 });
  const darkMat = new THREE.MeshStandardMaterial({ color: 0x1a1c24, metalness: 0.4, roughness: 0.5 });
  const chrome = metal(0xc8ccd8);
  const R = 0.34; // bán kính bánh
  const WB = 1.42; // khoảng cách hai trục
  const L = 2.25 * MOTO_SCALE;
  const H = 1.45 * MOTO_SCALE;

  // Bánh: lốp xuyến + mâm. Lốp cấp 2–3 mâm tối, 4–5 mâm đua có viền sáng; neon: bánh không trục phát sáng.
  const tires = lv.tires || 0;
  const wheels = [];
  for (const z of [WB / 2, -WB / 2]) {
    const w = new THREE.Group();
    w.position.set(0, R, z);
    const tyre = new THREE.Mesh(new THREE.TorusGeometry(R * 0.78, R * 0.24, 10, 24), new THREE.MeshStandardMaterial({ color: 0x15161b, roughness: 0.85 }));
    tyre.rotation.y = Math.PI / 2;
    const spin = new THREE.Group();
    spin.add(tyre);
    if (style === 'neon') {
      const ring = new THREE.Mesh(new THREE.TorusGeometry(R * 0.58, R * 0.05, 6, 28), glow(accent));
      ring.rotation.y = Math.PI / 2;
      spin.add(ring);
    } else {
      const rim = new THREE.Mesh(new THREE.CylinderGeometry(R * 0.56, R * 0.56, 0.1, 12), tires >= 2 ? metal(0x2a2d38) : chrome);
      rim.rotation.z = Math.PI / 2;
      spin.add(rim);
      for (let k = 0; k < (tires >= 4 ? 6 : 4); k++) {
        const sp = new THREE.Mesh(new THREE.BoxGeometry(0.12, R * 1.05, 0.05), tires >= 2 ? metal(0x111218) : chrome);
        sp.rotation.x = (k / (tires >= 4 ? 6 : 4)) * Math.PI;
        spin.add(sp);
      }
      if (tires >= 4) {
        const ring = new THREE.Mesh(new THREE.TorusGeometry(R * 0.6, R * 0.04, 6, 24), glow(accent));
        ring.rotation.y = Math.PI / 2;
        spin.add(ring);
      }
    }
    w.add(spin);
    lean.add(w);
    wheels.push(spin);
  }
  // Càng trước và gắp sau.
  const fork = new THREE.Mesh(new THREE.CylinderGeometry(0.035, 0.035, 0.75, 8), chrome);
  fork.position.set(0, R + 0.32, WB / 2 - 0.12);
  fork.rotation.x = -0.4;
  lean.add(fork);
  const arm = new THREE.Mesh(new THREE.BoxGeometry(0.1, 0.07, 0.75), darkMat);
  arm.position.set(0, R + 0.08, -WB / 2 + 0.36);
  arm.rotation.x = 0.18;
  lean.add(arm);
  // Khối động cơ.
  const eng = new THREE.Mesh(new THREE.BoxGeometry(0.36, 0.34, 0.5), darkMat);
  eng.position.set(0, R + 0.18, 0.02);
  lean.add(eng);

  // Thân theo kiểu.
  const add = (geo, mat, x, y, z, rx = 0, ry = 0, rz = 0) => {
    const m = new THREE.Mesh(geo, mat);
    m.position.set(x, y, z);
    m.rotation.set(rx, ry, rz);
    lean.add(m);
    return m;
  };
  if (style === 'street') {
    add(new THREE.SphereGeometry(0.24, 14, 10), bodyMat, 0, R + 0.62, 0.22).scale.set(1, 0.72, 1.45); // bình xăng
    add(new THREE.BoxGeometry(0.28, 0.08, 0.62), darkMat, 0, R + 0.6, -0.3, 0.1); // yên
    add(new THREE.BoxGeometry(0.22, 0.1, 0.42), bodyMat, 0, R + 0.66, -0.66, -0.35); // đuôi
    add(new THREE.CylinderGeometry(0.11, 0.11, 0.06, 14), glow(0xeaf6ff), 0, R + 0.74, WB / 2 + 0.02, Math.PI / 2); // đèn tròn
  } else if (style === 'sport') {
    // Yếm ôm trước, kính chắn gió, đuôi nhọn.
    add(new THREE.BoxGeometry(0.42, 0.52, 0.8), bodyMat, 0, R + 0.5, 0.42, -0.35);
    add(new THREE.BoxGeometry(0.44, 0.24, 0.6), bodyMat, 0, R + 0.24, 0.3);
    const glass = add(new THREE.BoxGeometry(0.34, 0.02, 0.32), new THREE.MeshStandardMaterial({ color: 0x9fd4ff, transparent: true, opacity: 0.55, metalness: 0.1, roughness: 0.05 }), 0, R + 0.86, 0.6, -0.95);
    glass.renderOrder = 2;
    add(new THREE.SphereGeometry(0.22, 14, 10), bodyMat, 0, R + 0.64, 0.05).scale.set(1, 0.7, 1.3);
    add(new THREE.BoxGeometry(0.26, 0.08, 0.5), darkMat, 0, R + 0.62, -0.38, 0.15);
    add(new THREE.ConeGeometry(0.18, 0.62, 4), bodyMat, 0, R + 0.7, -0.78, -Math.PI / 2 - 0.22, Math.PI / 4);
    for (const x of [-0.09, 0.09]) add(new THREE.BoxGeometry(0.1, 0.05, 0.03), glow(0xeaf6ff), x, R + 0.62, 0.86);
  } else {
    // Neon: thân dài vuốt nhọn, dải đèn chạy dọc hai bên.
    add(new THREE.BoxGeometry(0.4, 0.3, 1.5), bodyMat, 0, R + 0.5, 0.05, -0.06);
    add(new THREE.ConeGeometry(0.22, 0.6, 4), bodyMat, 0, R + 0.56, 0.98, Math.PI / 2 - 0.1, Math.PI / 4);
    add(new THREE.ConeGeometry(0.2, 0.5, 4), bodyMat, 0, R + 0.62, -0.92, -Math.PI / 2 - 0.15, Math.PI / 4);
    for (const x of [-0.205, 0.205]) add(new THREE.BoxGeometry(0.02, 0.04, 1.6), glow(accent), x, R + 0.48, 0.05, -0.06);
    add(new THREE.BoxGeometry(0.3, 0.04, 0.05), glow(accent), 0, R + 0.6, 1.12);
    add(new THREE.BoxGeometry(0.26, 0.07, 0.55), darkMat, 0, R + 0.7, -0.32, 0.1);
  }
  // Ghi đông.
  add(new THREE.BoxGeometry(0.62, 0.035, 0.035), darkMat, 0, R + 0.86, WB / 2 - 0.28);

  // Người lái cúi người, đội mũ có kính phát sáng.
  const rider = new THREE.Group();
  const suit = new THREE.MeshStandardMaterial({ color: 0x1d2235, roughness: 0.6 });
  const torso = new THREE.Mesh(new THREE.BoxGeometry(0.36, 0.5, 0.26), suit);
  torso.position.set(0, 0.28, 0.08);
  torso.rotation.x = 0.95;
  const shell = paint === PAINT.white || paint === PAINT.yellow ? 0x2f7cf6 : 0xf2f4f8;
  const helmet = new THREE.Mesh(new THREE.SphereGeometry(0.15, 16, 12), new THREE.MeshStandardMaterial({ color: shell, metalness: 0.2, roughness: 0.4 }));
  helmet.position.set(0, 0.46, 0.34);
  const visor = new THREE.Mesh(new THREE.SphereGeometry(0.152, 16, 8, Math.PI * 0.18, Math.PI * 0.64, Math.PI * 0.36, Math.PI * 0.22), glow(0x58f0ff));
  visor.position.copy(helmet.position);
  const thigh = new THREE.Mesh(new THREE.BoxGeometry(0.12, 0.12, 0.42), suit);
  const t1 = thigh.clone(); t1.position.set(-0.17, 0.02, 0.0); t1.rotation.x = -0.25;
  const t2 = thigh.clone(); t2.position.set(0.17, 0.02, 0.0); t2.rotation.x = -0.25;
  const armL = new THREE.Mesh(new THREE.BoxGeometry(0.09, 0.09, 0.42), suit);
  const a1 = armL.clone(); a1.position.set(-0.22, 0.34, 0.36); a1.rotation.x = 0.5;
  const a2 = armL.clone(); a2.position.set(0.22, 0.34, 0.36); a2.rotation.x = 0.5;
  rider.add(torso, helmet, visor, t1, t2, a1, a2);
  rider.position.set(0, R + 0.62, -0.32);
  lean.add(rider);

  // Ống xả (Động cơ): to dần theo cấp, cấp 4+ hai ống; lửa xanh khi nitro.
  const engine = lv.engine || 0;
  const flames = [];
  const pipes = engine >= 4 ? 2 : 1;
  for (let i = 0; i < pipes; i++) {
    const x = pipes === 2 ? (i ? 0.16 : -0.16) : 0.16;
    const r = 0.045 + engine * 0.01;
    const pipe = add(new THREE.CylinderGeometry(r, r * 1.2, 0.5 + engine * 0.03, 10), chrome, x, R + 0.3, -0.72, Math.PI / 2 - 0.25);
    pipe.userData.keep = true;
    const flame = new THREE.Mesh(new THREE.ConeGeometry(r * 1.6, 0.9, 10, 1, true), new THREE.MeshBasicMaterial({ color: 0x46b8ff, transparent: true, opacity: 0.85, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }));
    flame.rotation.x = -Math.PI / 2;
    flame.position.set(x, R + 0.24, -1.35);
    flame.visible = false;
    lean.add(flame);
    flames.push(flame);
  }
  // Turbo: bình bên hông, cấp 3+ hai bình, cấp 5 vòng sáng.
  const turbo = lv.turbo || 0;
  if (turbo >= 1) {
    for (const x of turbo >= 3 ? [-0.24, 0.24] : [-0.24]) {
      const can = add(new THREE.CylinderGeometry(0.07 + turbo * 0.01, 0.07 + turbo * 0.01, 0.26 + turbo * 0.03, 12), metal(0x9aa3b8), x, R + 0.24, -0.05, Math.PI / 2);
      if (turbo >= 5) add(new THREE.TorusGeometry(0.1, 0.016, 6, 18), glow(0xff5a2a), x, can.position.y, can.position.z);
    }
  }
  // Decal trên bình xăng / thân.
  if (cos.decal) {
    let map;
    if (cos.decal === 'decal_number') {
      const cv = document.createElement('canvas');
      cv.width = cv.height = 128;
      const x = cv.getContext('2d');
      x.fillStyle = '#ffffff';
      x.beginPath(); x.arc(64, 64, 58, 0, Math.PI * 2); x.fill();
      x.fillStyle = '#111'; x.font = 'bold 76px sans-serif'; x.textAlign = 'center'; x.textBaseline = 'middle';
      x.fillText(String(spec.number || 7), 64, 70);
      map = new THREE.CanvasTexture(cv);
      map.colorSpace = THREE.SRGBColorSpace;
    } else map = tex(`${cos.decal}.png`);
    for (const side of [-1, 1]) {
      const plate = add(new THREE.PlaneGeometry(0.42, 0.26), new THREE.MeshBasicMaterial({ map, transparent: true, depthWrite: false, side: THREE.DoubleSide, polygonOffset: true, polygonOffsetFactor: -4 }), side * (style === 'sport' ? 0.225 : 0.21), R + 0.36, 0.25, 0, side * Math.PI / 2);
      plate.renderOrder = 3;
    }
  }
  // Neon gầm.
  let underglow = null;
  if (cos.neon) {
    underglow = new THREE.Mesh(new THREE.PlaneGeometry(1.2, L * 1.2), new THREE.MeshBasicMaterial({ map: tex('spark.png'), color: accent, transparent: true, opacity: 0.5, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }));
    underglow.rotation.x = -Math.PI / 2;
    underglow.position.y = 0.06;
    root.add(underglow);
  }
  // Đèn hậu và bóng mềm.
  add(new THREE.BoxGeometry(0.16, 0.05, 0.04), glow(0xff2a3a), 0, R + 0.66, -1.02);
  const shadow = new THREE.Mesh(new THREE.PlaneGeometry(0.9, L * 1.1), new THREE.MeshBasicMaterial({ map: tex('spark.png'), color: 0x000000, transparent: true, opacity: 0.55, depthWrite: false }));
  shadow.rotation.x = -Math.PI / 2;
  shadow.position.y = 0.035;
  root.add(shadow);

  let rot = 0;
  let t = 0;
  return {
    root, length: L, height: H, flames, moto: true,
    update(speed, steer, drift, boost, dt) {
      t += dt;
      rot += (speed / R) * dt;
      for (const w of wheels) w.rotation.x = rot;
      // Nghiêng vào trong cua (steer > 0 là rẽ trái), càng nhanh nghiêng càng nhiều.
      const want = steer * (drift ? 0.62 : 0.46) * Math.min(1, Math.abs(speed) / 18);
      lean.rotation.z = THREE.MathUtils.lerp(lean.rotation.z, want, Math.min(1, dt * 7));
      for (const f of flames) {
        f.visible = boost;
        if (boost) f.scale.set(1, 0.8 + Math.sin(t * 60) * 0.25, 1);
      }
      if (underglow) underglow.material.opacity = 0.45 + Math.sin(t * 3) * 0.1;
    },
  };
}

// ---------- Đạo cụ: hộp "?", tên lửa, tên lửa đuổi, vũng dầu, khiên ----------

function questionTexture() {
  const cv = document.createElement('canvas');
  cv.width = cv.height = 128;
  const x = cv.getContext('2d');
  x.fillStyle = '#ffb21e';
  x.fillRect(0, 0, 128, 128);
  x.fillStyle = '#ff5fb4';
  x.fillRect(0, 0, 128, 18); x.fillRect(0, 110, 128, 18); x.fillRect(0, 0, 18, 128); x.fillRect(110, 0, 18, 128);
  x.fillStyle = '#ffffff';
  x.font = 'bold 92px sans-serif';
  x.textAlign = 'center';
  x.textBaseline = 'middle';
  x.fillText('?', 64, 70);
  const t = new THREE.CanvasTexture(cv);
  t.colorSpace = THREE.SRGBColorSpace;
  return t;
}

// createItemFx quản lý mọi thứ của chế độ đạo cụ trong cảnh: hộp (bật / tắt), đạn và dầu (theo id), khiên quanh xe.
export function createItemFx(scene, sparks) {
  const qTex = questionTexture();
  const boxMat = new THREE.MeshStandardMaterial({ map: qTex, emissive: 0xffa21e, emissiveMap: qTex, emissiveIntensity: 0.6, transparent: true, opacity: 0.92, roughness: 0.35 });
  const boxGeo = new THREE.BoxGeometry(1.5, 1.5, 1.5);
  const glowMat = new THREE.SpriteMaterial({ map: tex('spark.png'), color: 0xffd25a, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false, transparent: true, opacity: 0.8 });
  const boxes = new Map();
  const fx = new Map();
  let t = 0;

  const missileMesh = (homing) => {
    const g = new THREE.Group();
    const body = new THREE.Mesh(new THREE.CylinderGeometry(0.16, 0.16, 1.1, 10), new THREE.MeshStandardMaterial({ color: homing ? 0x9b4dff : 0xf2f4f8, metalness: 0.5, roughness: 0.3 }));
    body.rotation.x = Math.PI / 2;
    const nose = new THREE.Mesh(new THREE.ConeGeometry(0.16, 0.4, 10), new THREE.MeshStandardMaterial({ color: homing ? 0xff3fd0 : 0xe8333c, metalness: 0.3, roughness: 0.35 }));
    nose.rotation.x = Math.PI / 2;
    nose.position.z = 0.75;
    g.add(body, nose);
    for (let k = 0; k < 4; k++) {
      const fin = new THREE.Mesh(new THREE.BoxGeometry(0.03, 0.34, 0.26), new THREE.MeshStandardMaterial({ color: 0x23252e }));
      fin.position.z = -0.45;
      fin.rotation.z = (k * Math.PI) / 2;
      fin.position.x = Math.cos((k * Math.PI) / 2) * 0.16;
      fin.position.y = Math.sin((k * Math.PI) / 2) * 0.16;
      g.add(fin);
    }
    const flame = new THREE.Sprite(new THREE.SpriteMaterial({ map: tex('spark.png'), color: homing ? 0xff5fe0 : 0xff8a2a, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }));
    flame.scale.setScalar(1.4);
    flame.position.z = -0.75;
    g.add(flame);
    if (homing) {
      const ring = new THREE.Mesh(new THREE.TorusGeometry(0.42, 0.04, 6, 20), glow(0xff3fd0));
      g.add(ring);
      g.userData.ring = ring;
    }
    g.position.y = 0.9;
    return g;
  };
  const oilMesh = () => {
    const g = new THREE.Group();
    const pool = new THREE.Mesh(new THREE.CircleGeometry(2.6, 20), new THREE.MeshStandardMaterial({ color: 0x050608, metalness: 0.9, roughness: 0.12, transparent: true, opacity: 0.92, polygonOffset: true, polygonOffsetFactor: -3 }));
    pool.rotation.x = -Math.PI / 2;
    pool.scale.set(1, 0.75, 1);
    const sheen = new THREE.Mesh(new THREE.CircleGeometry(1.8, 16), new THREE.MeshBasicMaterial({ map: tex('spark.png'), color: 0x9b4dff, transparent: true, opacity: 0.35, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }));
    sheen.rotation.x = -Math.PI / 2;
    sheen.position.y = 0.01;
    g.add(pool, sheen);
    g.position.y = 0.05;
    return g;
  };

  return {
    // setBoxes dựng / cập nhật hộp từ danh sách [{id, x, z, active}].
    setBoxes(list) {
      for (const b of list || []) {
        let o = boxes.get(b.id);
        if (!o) {
          const g = new THREE.Group();
          const cube = new THREE.Mesh(boxGeo, boxMat);
          const halo = new THREE.Sprite(glowMat);
          halo.scale.setScalar(3.4);
          g.add(cube, halo);
          g.position.set(b.x, 1.2, b.z);
          scene.add(g);
          o = { g, cube, x: b.x, z: b.z, active: true };
          boxes.set(b.id, o);
        }
        o.active = b.active !== false;
        o.g.visible = o.active;
      }
    },
    setBox(id, active) {
      const o = boxes.get(id);
      if (!o) return;
      if (o.active && !active) sparks.emit(o.x, 1.2, o.z, 16, 6, 0xffd25a);
      o.active = active;
      o.g.visible = active;
    },
    boxes,
    // fx từ server: {id, kind, x, z, rot, dead}. Tên lửa nội suy về vị trí mới; dầu đứng yên.
    onFx(m) {
      let o = fx.get(m.id);
      if (m.dead) {
        if (o) {
          if (o.kind !== 'oil') sparks.emit(o.g.position.x, 1, o.g.position.z, 24, 9, 0xff8a2a);
          scene.remove(o.g);
          fx.delete(m.id);
        }
        return;
      }
      if (!o) {
        const g = m.kind === 'oil' ? oilMesh() : missileMesh(m.kind === 'homing');
        g.position.x = m.x;
        g.position.z = m.z;
        g.rotation.y = m.rot || 0;
        scene.add(g);
        o = { g, kind: m.kind, tx: m.x, tz: m.z, rot: m.rot || 0 };
        fx.set(m.id, o);
      }
      o.tx = m.x;
      o.tz = m.z;
      o.rot = m.rot || o.rot;
    },
    clearFx() { for (const o of fx.values()) scene.remove(o.g); fx.clear(); },
    fx,
    update(dt) {
      t += dt;
      for (const o of boxes.values()) {
        if (!o.active) continue;
        o.cube.rotation.y += dt * 1.6;
        o.cube.rotation.x = Math.sin(t * 1.3 + o.x) * 0.25;
        o.g.position.y = 1.2 + Math.sin(t * 2.4 + o.z) * 0.18;
      }
      for (const o of fx.values()) {
        if (o.kind === 'oil') continue;
        const k = 1 - Math.exp(-dt * 14);
        const px = o.g.position.x;
        const pz = o.g.position.z;
        o.g.position.x += (o.tx - px) * k;
        o.g.position.z += (o.tz - pz) * k;
        let d = o.rot - o.g.rotation.y;
        while (d > Math.PI) d -= Math.PI * 2;
        while (d < -Math.PI) d += Math.PI * 2;
        o.g.rotation.y += d * k;
        if (o.g.userData.ring) o.g.userData.ring.rotation.z += dt * 8;
        if (Math.random() < 0.6) sparks.emit(o.g.position.x - Math.sin(o.g.rotation.y) * 0.8, 0.9, o.g.position.z - Math.cos(o.g.rotation.y) * 0.8, 1, 2, o.kind === 'homing' ? 0xff5fe0 : 0xff8a2a);
      }
    },
  };
}

// makeShield là quả cầu khiên trong suốt gắn lên xe (ẩn / hiện theo trạng thái).
export function makeShield(length) {
  const r = Math.max(1.6, length * 0.68);
  const m = new THREE.Mesh(new THREE.SphereGeometry(r, 20, 14), new THREE.MeshBasicMaterial({ color: 0x58f0ff, transparent: true, opacity: 0.22, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false }));
  const wire = new THREE.Mesh(new THREE.IcosahedronGeometry(r * 1.01, 1), new THREE.MeshBasicMaterial({ color: 0x9ff6ff, wireframe: true, transparent: true, opacity: 0.45, toneMapped: false }));
  const g = new THREE.Group();
  g.add(m, wire);
  g.position.y = 0.7;
  g.visible = false;
  return g;
}

// ---------- Hiệu ứng: tia lửa, vệt tốc độ ----------

export function createSparks(scene, max = 240) {
  const pos = new Float32Array(max * 3);
  const vel = new Float32Array(max * 3);
  const life = new Float32Array(max);
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.BufferAttribute(pos, 3));
  const mat = new THREE.PointsMaterial({ map: tex('spark.png'), color: 0xffb547, size: 0.5, transparent: true, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false });
  const pts = new THREE.Points(g, mat);
  pts.frustumCulled = false;
  scene.add(pts);
  let head = 0;
  return {
    emit(x, y, z, n, spread = 6, color) {
      if (color) mat.color.setHex(color);
      for (let i = 0; i < n; i++) {
        const k = head++ % max;
        pos[k * 3] = x; pos[k * 3 + 1] = y; pos[k * 3 + 2] = z;
        vel[k * 3] = (Math.random() - 0.5) * spread;
        vel[k * 3 + 1] = Math.random() * spread * 0.7;
        vel[k * 3 + 2] = (Math.random() - 0.5) * spread;
        life[k] = 0.35 + Math.random() * 0.3;
      }
    },
    update(dt) {
      for (let k = 0; k < max; k++) {
        if (life[k] <= 0) { pos[k * 3 + 1] = -100; continue; }
        life[k] -= dt;
        vel[k * 3 + 1] -= 18 * dt;
        pos[k * 3] += vel[k * 3] * dt;
        pos[k * 3 + 1] = Math.max(0.05, pos[k * 3 + 1] + vel[k * 3 + 1] * dt);
        pos[k * 3 + 2] += vel[k * 3 + 2] * dt;
      }
      g.attributes.position.needsUpdate = true;
    },
  };
}

// Vệt tốc độ: các đoạn thẳng trôi về phía camera khi chạy nhanh / nitro (gắn vào camera).
export function createSpeedLines(camera, count = 60) {
  const pos = new Float32Array(count * 6);
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.BufferAttribute(pos, 3));
  const mat = new THREE.LineBasicMaterial({ color: 0xbfe8ff, transparent: true, opacity: 0, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false });
  const lines = new THREE.LineSegments(g, mat);
  lines.frustumCulled = false;
  camera.add(lines);
  const seed = (i, far) => {
    const a = Math.random() * Math.PI * 2;
    const r = 1.6 + Math.random() * 2.4;
    const z = far ? -24 - Math.random() * 12 : -Math.random() * 36;
    pos[i * 6] = Math.cos(a) * r; pos[i * 6 + 1] = Math.sin(a) * r * 0.6; pos[i * 6 + 2] = z;
    pos[i * 6 + 3] = pos[i * 6]; pos[i * 6 + 4] = pos[i * 6 + 1]; pos[i * 6 + 5] = z - 3;
  };
  for (let i = 0; i < count; i++) seed(i, false);
  return {
    update(dt, amount, speed) {
      mat.opacity += (amount * 0.55 - mat.opacity) * Math.min(1, dt * 5);
      lines.visible = mat.opacity > 0.02;
      if (!lines.visible) return;
      for (let i = 0; i < count; i++) {
        pos[i * 6 + 2] += speed * 1.6 * dt;
        pos[i * 6 + 5] = pos[i * 6 + 2] - 1.2 - amount * 1.6;
        if (pos[i * 6 + 2] > -1) seed(i, true);
      }
      g.attributes.position.needsUpdate = true;
    },
  };
}

export { THREE };
