// Dựng 12 chậu tầng mây bằng three.js rồi xuất ảnh (window.renderPot(id) → data URL PNG nền trong suốt).
// Ảnh tự dựng cho DCity (CC0). Hoạ tiết vẽ bằng canvas ngay trong script này, không đưa vào game.
import * as THREE from 'three';
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js';

// Khung hình chung (đơn vị thế giới): rộng 1,24 ↔ 120 px, cao 1,364 ↔ 132 px; máy quay nghiêng 25°.
// Tâm mặt đất (0, SOIL_Y, 0) rơi đúng y = 32 px của ảnh 120×132 (plantY).
export const TILT = (25 * Math.PI) / 180;
const FRAME = { left: -0.62, right: 0.62, top: 1.15, bottom: 1.15 - 1.364 };
const SCALE = 4; // dựng 480×528 rồi thu nhỏ khi hậu kỳ
const W = 120 * SCALE, H = 132 * SCALE;
const RIM_Y = 0.95; // đỉnh vành
const SOIL_Y = (FRAME.top - (32 / 132) * 1.364) / Math.cos(TILT); // ≈ 0,904

const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
renderer.setPixelRatio(1);
renderer.setSize(W, H);
renderer.setClearColor(0x000000, 0);
renderer.outputColorSpace = THREE.SRGBColorSpace;
renderer.toneMapping = THREE.NeutralToneMapping;
renderer.toneMappingExposure = 1.05;
document.body.appendChild(renderer.domElement);

const pmrem = new THREE.PMREMGenerator(renderer);
const envTex = pmrem.fromScene(new RoomEnvironment(), 0.04).texture;

const camera = new THREE.OrthographicCamera(FRAME.left, FRAME.right, FRAME.top, FRAME.bottom, 0.1, 50);
camera.position.set(0, Math.sin(TILT) * 10, Math.cos(TILT) * 10);
camera.up.set(0, Math.cos(TILT), -Math.sin(TILT));
camera.lookAt(0, 0, 0);

// ---------- tiện ích ----------
function rng(seed) {
  let s = seed >>> 0;
  return () => ((s = (s * 1664525 + 1013904223) >>> 0) / 4294967296);
}
function canvasTex(w, h, draw, { color = true, repeatX = 1 } = {}) {
  const c = document.createElement('canvas');
  c.width = w; c.height = h;
  const g = c.getContext('2d');
  draw(g, w, h);
  const t = new THREE.CanvasTexture(c);
  t.wrapS = THREE.RepeatWrapping;
  t.repeat.x = repeatX;
  t.anisotropy = 8;
  if (color) t.colorSpace = THREE.SRGBColorSpace;
  return t;
}
// Đường cong mượt qua các điểm (r, y) → mảng Vector2 cho LatheGeometry.
function curve(pts, n = 80) {
  return new THREE.SplineCurve(pts.map(([r, y]) => new THREE.Vector2(r, y))).getPoints(n);
}
// Thân chậu: thành ngoài theo profile, kín đáy; v của UV chạy từ đáy (0) lên miệng (1).
function body(outer, segments = 96) {
  const pts = [new THREE.Vector2(0.0001, outer[0][1]), ...curve(outer)];
  const g = new THREE.LatheGeometry(pts, segments);
  g.computeVertexNormals();
  return g;
}
// Thành trong + mặt đất: ống từ miệng xuống ngay dưới mặt đất.
function inner(r, mat) {
  const grp = new THREE.Group();
  const wall = new THREE.Mesh(new THREE.CylinderGeometry(r, r * 0.96, 0.12, 72, 1, true), mat);
  wall.material = mat.clone();
  wall.material.side = THREE.BackSide;
  wall.position.y = RIM_Y - 0.06;
  grp.add(wall);
  grp.add(soil(r));
  return grp;
}
function soil(r) {
  const grp = new THREE.Group();
  const g = new THREE.CircleGeometry(r, 64, 0, Math.PI * 2);
  g.rotateX(-Math.PI / 2);
  const pos = g.attributes.position;
  const rand = rng(7);
  for (let i = 0; i < pos.count; i++) {
    const x = pos.getX(i), z = pos.getZ(i);
    const d = Math.hypot(x, z) / r;
    const bump = (Math.sin(x * 37) * Math.cos(z * 41) + Math.sin(x * 83 + z * 61) * 0.5) * 0.008 + rand() * 0.006;
    pos.setY(i, (1 - d * d) * 0.04 + bump * (1 - d) * 0.6);
  }
  g.computeVertexNormals();
  const tex = canvasTex(256, 256, (c, w, h) => {
    c.fillStyle = '#4b2e19'; c.fillRect(0, 0, w, h);
    const rr = rng(3);
    for (let i = 0; i < 2200; i++) {
      const v = rr();
      c.fillStyle = v < 0.6 ? 'rgba(30,17,8,.45)' : v < 0.92 ? 'rgba(104,68,40,.45)' : 'rgba(140,100,62,.5)';
      c.beginPath(); c.arc(rr() * w, rr() * h, 0.6 + rr() * 1.6, 0, 7); c.fill();
    }
  });
  const m = new THREE.MeshStandardMaterial({ map: tex, roughness: 0.9, color: 0xd8c8b8 });
  const mesh = new THREE.Mesh(g, m);
  mesh.position.y = SOIL_Y - 0.012;
  grp.add(mesh);
  // Vài cục đất tơi.
  const rr = rng(11);
  const crumbGeo = new THREE.IcosahedronGeometry(1, 0);
  for (let i = 0; i < 34; i++) {
    const a = rr() * Math.PI * 2, d = Math.sqrt(rr()) * r * 0.85;
    const c = new THREE.Mesh(crumbGeo, new THREE.MeshStandardMaterial({ color: rr() < 0.6 ? 0x4a2d18 : 0x6a4426, roughness: 1, flatShading: true }));
    const s = 0.008 + rr() * 0.012;
    c.scale.set(s, s * 0.7, s);
    c.rotation.set(rr() * 3, rr() * 3, rr() * 3);
    c.position.set(Math.cos(a) * d, SOIL_Y - 0.005 + (1 - (d / r) ** 2) * 0.04, Math.sin(a) * d);
    grp.add(c);
  }
  return grp;
}
function torus(r, tube, mat, y, seg = 96) {
  const m = new THREE.Mesh(new THREE.TorusGeometry(r, tube, 24, seg), mat);
  m.rotation.x = Math.PI / 2;
  m.position.y = y;
  return m;
}
function glaze(color, opts = {}) {
  return new THREE.MeshPhysicalMaterial({ color, roughness: 0.22, clearcoat: 1, clearcoatRoughness: 0.08, envMapIntensity: 1, ...opts });
}
function gold(opts = {}) {
  return new THREE.MeshPhysicalMaterial({ color: 0xffc94a, metalness: 1, roughness: 0.22, envMapIntensity: 1.4, ...opts });
}
function gem(color) {
  return new THREE.MeshPhysicalMaterial({ color, roughness: 0.05, metalness: 0, clearcoat: 1, envMapIntensity: 2.2, emissive: color, emissiveIntensity: 0.18, flatShading: true });
}
// Điểm trên mặt ngoài thân ở độ cao y (nội suy bán kính theo profile).
function radiusAt(outer, y) {
  const pts = curve(outer, 200);
  let best = pts[0];
  for (const p of pts) if (Math.abs(p.y - y) < Math.abs(best.y - y)) best = p;
  return best.x;
}

// Vẽ dải hoạ tiết trên canvas UV của thân: v (0 đáy → 1 miệng) đổi thành toạ độ canvas.
const band = (h, v) => (1 - v) * h;

// ---------- hoạ tiết ----------
function flowerAt(c, x, y, s, petal, center) {
  c.fillStyle = petal;
  for (let k = 0; k < 5; k++) {
    const a = (k / 5) * Math.PI * 2 - Math.PI / 2;
    c.beginPath(); c.ellipse(x + Math.cos(a) * s * 0.55, y + Math.sin(a) * s * 0.55, s * 0.42, s * 0.3, a, 0, 7); c.fill();
  }
  c.fillStyle = center;
  c.beginPath(); c.arc(x, y, s * 0.28, 0, 7); c.fill();
}
function leaf(c, x, y, s, a, col) {
  c.save(); c.translate(x, y); c.rotate(a); c.fillStyle = col;
  c.beginPath(); c.moveTo(0, 0); c.quadraticCurveTo(s * 0.5, -s * 0.35, s, 0); c.quadraticCurveTo(s * 0.5, s * 0.35, 0, 0); c.fill();
  c.restore();
}
function swirl(c, x, y, s, col, lw) {
  c.strokeStyle = col; c.lineWidth = lw; c.lineCap = 'round';
  c.beginPath();
  for (let t = 0; t < 4 * Math.PI; t += 0.1) {
    const r = (s * t) / (4 * Math.PI);
    const px = x + Math.cos(t) * r, py = y + Math.sin(t) * r * 0.8;
    t === 0 ? c.moveTo(px, py) : c.lineTo(px, py);
  }
  c.stroke();
}

// ---------- 12 chậu ----------
const POTS = {
  // Tầng 1: chậu đất nung, vành cuộn dày, dải hoa lá kem nổi.
  pot_1() {
    const outer = [[0.3, 0], [0.33, 0.06], [0.4, 0.3], [0.47, 0.6], [0.5, 0.82], [0.5, 0.86]];
    const deco = canvasTex(1024, 512, (c, w, h) => {
      c.fillStyle = '#d0703f'; c.fillRect(0, 0, w, h);
      // vân đất nung
      const r = rng(5);
      for (let i = 0; i < 2500; i++) { c.fillStyle = r() < 0.5 ? 'rgba(150,70,35,.12)' : 'rgba(240,160,110,.12)'; c.fillRect(r() * w, r() * h, 2 + r() * 6, 1 + r() * 2); }
      c.fillStyle = '#f6e3c4';
      c.fillRect(0, band(h, 0.72), w, 7); c.fillRect(0, band(h, 0.34), w, 7);
      for (let i = 0; i < 8; i++) {
        const x = (i + 0.5) * (w / 8), y = band(h, 0.53);
        leaf(c, x - 14, y + 4, 34, Math.PI * 0.85, '#7fae4e'); leaf(c, x + 14, y + 4, 34, Math.PI * 0.15, '#7fae4e');
        flowerAt(c, x, y - 6, 40, '#fbe9cf', '#f2a33a');
      }
      for (let i = 0; i < 40; i++) { c.beginPath(); c.arc((i + 0.5) * (w / 40), band(h, 0.18), 4, 0, 7); c.fillStyle = '#f6e3c4'; c.fill(); }
    });
    const bump = canvasTex(1024, 512, (c, w, h) => {
      c.fillStyle = '#000'; c.fillRect(0, 0, w, h); c.fillStyle = '#fff';
      c.fillRect(0, band(h, 0.72), w, 7); c.fillRect(0, band(h, 0.34), w, 7);
      for (let i = 0; i < 8; i++) { const x = (i + 0.5) * (w / 8), y = band(h, 0.53); flowerAt(c, x, y - 6, 40, '#fff', '#fff'); leaf(c, x - 14, y + 4, 34, Math.PI * 0.85, '#bbb'); leaf(c, x + 14, y + 4, 34, Math.PI * 0.15, '#bbb'); }
      for (let i = 0; i < 40; i++) { c.beginPath(); c.arc((i + 0.5) * (w / 40), band(h, 0.18), 4, 0, 7); c.fill(); }
    }, { color: false });
    const mat = new THREE.MeshPhysicalMaterial({ map: deco, bumpMap: bump, bumpScale: 3, roughness: 0.55, clearcoat: 0.35, clearcoatRoughness: 0.3 });
    const g = new THREE.Group();
    g.add(new THREE.Mesh(body(outer), mat));
    const rimMat = new THREE.MeshPhysicalMaterial({ color: 0xc8653a, roughness: 0.5, clearcoat: 0.4 });
    // vành loe dày: ống côn + gờ tròn trên
    const collar = new THREE.Mesh(new THREE.CylinderGeometry(0.56, 0.52, 0.11, 96, 1, true), rimMat);
    collar.position.y = 0.875;
    g.add(collar, torus(0.535, 0.045, rimMat, RIM_Y - 0.02), torus(0.52, 0.025, rimMat, 0.82));
    g.add(inner(0.5, rimMat));
    return { group: g, outline: '#5a2410' };
  },

  // Tầng 2: bát sứ men trắng bầu tròn, vành lam trời, vân mây lam, chân đế.
  pot_2() {
    const outer = [[0.24, 0], [0.27, 0.03], [0.24, 0.08], [0.36, 0.2], [0.5, 0.42], [0.54, 0.6], [0.52, 0.8], [0.49, 0.88]];
    const deco = canvasTex(1024, 512, (c, w, h) => {
      c.fillStyle = '#fbfdff'; c.fillRect(0, 0, w, h);
      c.fillStyle = '#4f9fd8'; c.fillRect(0, band(h, 0.82), w, 10); c.fillRect(0, band(h, 0.2), w, 8);
      for (let i = 0; i < 6; i++) {
        const x = (i + 0.5) * (w / 6), y = band(h, 0.52);
        swirl(c, x - 26, y, 26, '#5aa6dd', 7); swirl(c, x + 22, y + 8, 20, '#7cbbe6', 6);
        c.fillStyle = '#9fd0f0'; c.beginPath(); c.arc(x + 4, y - 28, 6, 0, 7); c.fill();
      }
    });
    const g = new THREE.Group();
    g.add(new THREE.Mesh(body(outer), glaze(0xffffff, { map: deco })));
    const blue = glaze(0x3f95d6);
    g.add(torus(0.5, 0.05, blue, RIM_Y - 0.035), torus(0.27, 0.025, blue, 0.025));
    g.add(inner(0.47, glaze(0xf4f8fb)));
    return { group: g, outline: '#1d4f78' };
  },

  // Tầng 3: thạp ngọc lục bích, vành và chân vàng, vân xoắn vàng.
  pot_3() {
    const outer = [[0.3, 0], [0.32, 0.05], [0.29, 0.1], [0.42, 0.28], [0.53, 0.5], [0.52, 0.7], [0.46, 0.84], [0.46, 0.88]];
    const deco = canvasTex(1024, 512, (c, w, h) => {
      const gr = c.createLinearGradient(0, 0, 0, h); gr.addColorStop(0, '#2fc493'); gr.addColorStop(1, '#0d7a5a');
      c.fillStyle = gr; c.fillRect(0, 0, w, h);
      const r = rng(9);
      for (let i = 0; i < 120; i++) { c.strokeStyle = 'rgba(200,255,230,.08)'; c.lineWidth = 3 + r() * 6; c.beginPath(); c.moveTo(r() * w, r() * h); c.bezierCurveTo(r() * w, r() * h, r() * w, r() * h, r() * w, r() * h); c.stroke(); }
      c.fillStyle = '#ffd25a'; c.fillRect(0, band(h, 0.75), w, 6); c.fillRect(0, band(h, 0.3), w, 6);
      for (let i = 0; i < 6; i++) { const x = (i + 0.5) * (w / 6), y = band(h, 0.53); swirl(c, x - 30, y, 24, '#ffd25a', 6); swirl(c, x + 30, y, 24, '#ffd25a', 6); c.save(); c.translate(x, y); c.rotate(Math.PI / 4); c.fillStyle = '#fff4c8'; c.fillRect(-9, -9, 18, 18); c.restore(); }
    });
    const g = new THREE.Group();
    g.add(new THREE.Mesh(body(outer), glaze(0xffffff, { map: deco, roughness: 0.12, sheen: 0.4, sheenColor: new THREE.Color(0x9fffe0) })));
    const au = gold();
    g.add(torus(0.48, 0.05, au, RIM_Y - 0.035), torus(0.32, 0.03, au, 0.03), torus(0.47, 0.018, au, 0.84));
    g.add(inner(0.45, glaze(0x138a64)));
    return { group: g, outline: '#06402e' };
  },

  // Tầng 4: bình vàng hoàng gia trên chân đế, dải đá quý, vành răng cưa.
  pot_4() {
    const outer = [[0.3, 0], [0.3, 0.06], [0.16, 0.12], [0.15, 0.18], [0.36, 0.3], [0.52, 0.5], [0.52, 0.7], [0.47, 0.84], [0.48, 0.88]];
    const au = gold();
    const g = new THREE.Group();
    const deco = canvasTex(1024, 512, (c, w, h) => {
      c.fillStyle = '#ffd15a'; c.fillRect(0, 0, w, h);
      c.strokeStyle = '#b8801c'; c.lineWidth = 5;
      for (let i = 0; i < 16; i++) { const x = (i + 0.5) * (w / 16); c.beginPath(); c.arc(x, band(h, 0.32), 28, Math.PI, 0); c.stroke(); }
      c.fillStyle = '#b8801c'; c.fillRect(0, band(h, 0.74), w, 4); c.fillRect(0, band(h, 0.6), w, 4);
    }, { color: false });
    g.add(new THREE.Mesh(body(outer), gold({ bumpMap: deco, bumpScale: 2 })));
    g.add(torus(0.3, 0.035, au, 0.04), torus(0.16, 0.03, au, 0.15));
    g.add(torus(0.49, 0.05, au, RIM_Y - 0.035));
    // răng vương miện trên vành
    for (let i = 0; i < 16; i++) {
      const a = (i / 16) * Math.PI * 2;
      const b = new THREE.Mesh(new THREE.SphereGeometry(0.028, 16, 12), au);
      b.position.set(Math.cos(a) * 0.49, RIM_Y + 0.025, Math.sin(a) * 0.49);
      g.add(b);
    }
    // dải đá quý quanh bụng
    const cols = [0xe0245e, 0x2f7bf6, 0x22b573, 0x9b4dff];
    const y = 0.67, r = radiusAt(outer, y);
    for (let i = 0; i < 10; i++) {
      const a = (i / 10) * Math.PI * 2 + Math.PI / 2;
      const big = i % 2 === 0;
      const m = new THREE.Mesh(new THREE.OctahedronGeometry(big ? 0.055 : 0.035, 1), gem(cols[i % 4]));
      m.scale.set(1, big ? 1.25 : 1, 0.55);
      m.position.set(Math.cos(a) * (r + 0.01), y, Math.sin(a) * (r + 0.01));
      m.lookAt(Math.cos(a) * 3, y, Math.sin(a) * 3);
      g.add(m);
      const setting = new THREE.Mesh(new THREE.TorusGeometry(big ? 0.06 : 0.04, 0.012, 8, 24), au);
      setting.position.copy(m.position); setting.lookAt(Math.cos(a) * 3, y, Math.sin(a) * 3);
      g.add(setting);
    }
    g.add(inner(0.46, gold({ roughness: 0.4 })));
    return { group: g, outline: '#6b4300' };
  },

  // Gốm trắng: chậu men trắng gân dọc, vành tròn mập.
  pot_skin_white() {
    const outer = [[0.32, 0], [0.34, 0.04], [0.42, 0.25], [0.5, 0.55], [0.52, 0.8], [0.5, 0.88]];
    const ribs = canvasTex(1024, 64, (c, w, h) => {
      for (let x = 0; x < w; x++) { const v = 128 + 127 * Math.cos((x / w) * Math.PI * 2 * 24); c.fillStyle = `rgb(${v},${v},${v})`; c.fillRect(x, 0, 1, h); }
    }, { color: false });
    const g = new THREE.Group();
    g.add(new THREE.Mesh(body(outer), glaze(0xfafafa, { bumpMap: ribs, bumpScale: 4, roughness: 0.18 })));
    const m = glaze(0xffffff);
    g.add(torus(0.505, 0.055, m, RIM_Y - 0.04));
    g.add(inner(0.47, glaze(0xeeeeee)));
    return { group: g, outline: '#7c8794' };
  },

  // Gốm hoa: chậu kem bầu, vành hồng, hoa vẽ tay nổi.
  pot_skin_flower() {
    const outer = [[0.28, 0], [0.3, 0.04], [0.42, 0.22], [0.53, 0.5], [0.52, 0.74], [0.48, 0.88]];
    const cols = ['#ff7aa8', '#ffb340', '#a985ff', '#5cb8ff', '#ff6b6b'];
    const draw = (bumpMode) => (c, w, h) => {
      c.fillStyle = bumpMode ? '#000' : '#fff6e6'; c.fillRect(0, 0, w, h);
      const r = rng(21);
      for (let i = 0; i < 40; i++) {
        const x = r() * w, y = band(h, 0.28 + r() * 0.46), s = 28 + r() * 20;
        if (!bumpMode) { leaf(c, x - 4, y + 6, s * 0.9, Math.PI * 0.75, '#79b65a'); leaf(c, x + 4, y + 6, s * 0.9, Math.PI * 0.25, '#8cc66b'); }
        flowerAt(c, x, y, s, bumpMode ? '#fff' : cols[i % cols.length], bumpMode ? '#ddd' : '#fff1a8');
        if (x < s) flowerAt(c, x + w, y, s, bumpMode ? '#fff' : cols[i % cols.length], bumpMode ? '#ddd' : '#fff1a8');
      }
      c.fillStyle = bumpMode ? '#fff' : '#ff8fb3'; c.fillRect(0, band(h, 0.18), w, 6);
    };
    const g = new THREE.Group();
    g.add(new THREE.Mesh(body(outer), glaze(0xffffff, { map: canvasTex(1024, 512, draw(false)), bumpMap: canvasTex(1024, 512, draw(true), { color: false }), bumpScale: 2.5 })));
    const pink = glaze(0xff8fb3);
    g.add(torus(0.49, 0.05, pink, RIM_Y - 0.04));
    g.add(inner(0.46, glaze(0xfff0dc)));
    return { group: g, outline: '#9c4364' };
  },

  // Gỗ mộc: thùng gỗ ván dọc, hai đai đồng có đinh tán.
  pot_skin_wood() {
    const outer = [[0.4, 0], [0.42, 0.05], [0.48, 0.3], [0.5, 0.48], [0.48, 0.7], [0.45, 0.9]];
    const planks = canvasTex(1024, 512, (c, w, h) => {
      const n = 18, pw = w / n, r = rng(4);
      for (let i = 0; i < n; i++) {
        const base = [[176, 112, 62], [160, 98, 52], [188, 124, 72]][i % 3];
        c.fillStyle = `rgb(${base})`; c.fillRect(i * pw, 0, pw, h);
        for (let k = 0; k < 14; k++) { c.strokeStyle = `rgba(90,50,20,${0.15 + r() * 0.2})`; c.lineWidth = 1 + r() * 2; const x = i * pw + r() * pw; c.beginPath(); c.moveTo(x, 0); c.bezierCurveTo(x + r() * 10 - 5, h * 0.3, x + r() * 10 - 5, h * 0.7, x, h); c.stroke(); }
        if (r() < 0.5) { c.fillStyle = 'rgba(80,40,15,.5)'; c.beginPath(); c.ellipse(i * pw + pw / 2, r() * h, 5, 9, 0, 0, 7); c.fill(); }
        c.fillStyle = 'rgba(50,25,8,.75)'; c.fillRect(i * pw, 0, 3, h);
      }
    });
    const seams = canvasTex(1024, 64, (c, w, h) => { c.fillStyle = '#fff'; c.fillRect(0, 0, w, h); c.fillStyle = '#000'; for (let i = 0; i < 18; i++) c.fillRect((i * w) / 18, 0, 4, h); }, { color: false });
    const g = new THREE.Group();
    g.add(new THREE.Mesh(body(outer), new THREE.MeshPhysicalMaterial({ map: planks, bumpMap: seams, bumpScale: 3, roughness: 0.7, clearcoat: 0.25 })));
    const brass = new THREE.MeshPhysicalMaterial({ color: 0xc9923e, metalness: 0.9, roughness: 0.35, envMapIntensity: 1.2 });
    for (const y of [0.2, 0.72]) {
      const r = radiusAt(outer, y) + 0.012;
      const band = new THREE.Mesh(new THREE.CylinderGeometry(r, r + 0.004, 0.07, 96, 1, true), brass);
      band.position.y = y; g.add(band);
      for (let i = 0; i < 12; i++) { const a = (i / 12) * Math.PI * 2; const s = new THREE.Mesh(new THREE.SphereGeometry(0.013, 10, 8), brass); s.position.set(Math.cos(a) * (r + 0.004), y, Math.sin(a) * (r + 0.004)); g.add(s); }
    }
    const rimWood = new THREE.MeshPhysicalMaterial({ color: 0x9a5f30, roughness: 0.6 });
    g.add(torus(0.44, 0.03, rimWood, 0.905));
    g.add(inner(0.43, rimWood));
    return { group: g, outline: '#3f210b' };
  },

  // Sứ thanh hoa: vò men trắng hoạ tiết lam (sóng, dây hoa, cánh sen), cổ thắt vành loe.
  pot_skin_bluewhite() {
    const outer = [[0.26, 0], [0.28, 0.04], [0.42, 0.2], [0.54, 0.45], [0.5, 0.66], [0.38, 0.8], [0.4, 0.86], [0.46, 0.9]];
    const deco = canvasTex(1024, 512, (c, w, h) => {
      c.fillStyle = '#fbfcff'; c.fillRect(0, 0, w, h);
      const blue = '#1f4fa8';
      // cánh sen dưới đáy
      for (let i = 0; i < 16; i++) { const x = (i + 0.5) * (w / 16), y = band(h, 0.08); c.strokeStyle = blue; c.lineWidth = 4; c.beginPath(); c.moveTo(x - 26, y); c.quadraticCurveTo(x, y - 70, x + 26, y); c.stroke(); c.fillStyle = 'rgba(31,79,168,.35)'; c.beginPath(); c.moveTo(x - 14, y); c.quadraticCurveTo(x, y - 46, x + 14, y); c.fill(); }
      c.fillStyle = blue; c.fillRect(0, band(h, 0.24), w, 5); c.fillRect(0, band(h, 0.68), w, 5);
      // dây hoa mẫu đơn quanh bụng
      c.strokeStyle = blue; c.lineWidth = 5; c.beginPath();
      for (let x = 0; x <= w; x += 4) { const y = band(h, 0.46) + Math.sin((x / w) * Math.PI * 8) * 26; x === 0 ? c.moveTo(x, y) : c.lineTo(x, y); }
      c.stroke();
      for (let i = 0; i < 8; i++) {
        const x = (i + 0.25) * (w / 8), y = band(h, 0.46) + Math.sin(((x / w) * Math.PI * 8)) * 26;
        c.fillStyle = blue; c.beginPath(); for (let k = 0; k < 8; k++) { const a = (k / 8) * Math.PI * 2; c.ellipse(x + Math.cos(a) * 14, y + Math.sin(a) * 14, 12, 7, a, 0, 7); } c.fill();
        c.fillStyle = '#fbfcff'; c.beginPath(); c.arc(x, y, 8, 0, 7); c.fill(); c.fillStyle = blue; c.beginPath(); c.arc(x, y, 4, 0, 7); c.fill();
        leaf(c, x + 30, y + 12, 30, 0.4, blue); leaf(c, x - 30, y - 10, 30, Math.PI + 0.4, blue);
      }
      // sóng ở vai
      c.strokeStyle = blue; c.lineWidth = 4;
      for (let i = 0; i < 24; i++) { const x = (i + 0.5) * (w / 24), y = band(h, 0.78); c.beginPath(); c.arc(x, y, 14, Math.PI, 0); c.stroke(); c.beginPath(); c.arc(x, y, 7, Math.PI, 0); c.stroke(); }
    });
    const g = new THREE.Group();
    g.add(new THREE.Mesh(body(outer), glaze(0xffffff, { map: deco, roughness: 0.1 })));
    const blue = glaze(0x1f4fa8);
    g.add(torus(0.465, 0.04, blue, RIM_Y - 0.03));
    g.add(inner(0.44, glaze(0xf2f5fb)));
    return { group: g, outline: '#14336e' };
  },

  // Ngọc lục bảo: chậu lục giác mài cạnh, trong veo, viền vàng.
  pot_skin_jade() {
    const outer = [[0.3, 0], [0.32, 0.05], [0.44, 0.3], [0.53, 0.62], [0.53, 0.8], [0.52, 0.88]];
    const g = new THREE.Group();
    const geo = body(outer, 8);
    geo.rotateY(Math.PI / 8);
    const jade = new THREE.MeshPhysicalMaterial({ color: 0x06764c, roughness: 0.08, clearcoat: 1, clearcoatRoughness: 0.02, sheen: 0.25, sheenColor: new THREE.Color(0x7dffc8), envMapIntensity: 1.1, flatShading: true, emissive: 0x05402a, emissiveIntensity: 0.35, iridescence: 0.3 });
    g.add(new THREE.Mesh(geo, jade));
    const au = gold();
    // cạnh đa giác 8 nằm ở góc: bán kính đỉnh = r_profile (Lathe đặt đỉnh trên đường tròn)
    const rim = new THREE.Mesh(new THREE.TorusGeometry(0.52, 0.035, 12, 8), au);
    rim.rotation.x = Math.PI / 2; rim.rotation.z = Math.PI / 8 + Math.PI / 2; rim.position.y = 0.88;
    const foot = new THREE.Mesh(new THREE.TorusGeometry(0.315, 0.025, 12, 8), au);
    foot.rotation.x = Math.PI / 2; foot.rotation.z = Math.PI / 8 + Math.PI / 2; foot.position.y = 0.03;
    g.add(rim, foot);
    // cạnh vàng mảnh dọc theo 6 cạnh
    for (let i = 0; i < 8; i++) {
      const a = (i / 8) * Math.PI * 2 + Math.PI / 8;
      const pts = curve(outer, 40).map((p) => new THREE.Vector3(Math.sin(a) * p.x * 1.004, p.y, Math.cos(a) * p.x * 1.004));
      g.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 40, 0.009, 6), au));
    }
    g.add(soil(0.48));
    return { group: g, outline: '#06422c' };
  },

  // Mây trắng: chậu bông mây mềm ghép từ nhiều cụm tròn, ánh hồng nhẹ.
  pot_skin_cloud() {
    const g = new THREE.Group();
    const puff = new THREE.MeshPhysicalMaterial({ color: 0xffffff, roughness: 0.55, sheen: 1, sheenRoughness: 0.4, sheenColor: new THREE.Color(0xffc8e6), envMapIntensity: 0.9 });
    const core = new THREE.Mesh(body([[0.3, 0.02], [0.4, 0.2], [0.47, 0.5], [0.47, 0.8], [0.45, 0.88]]), puff);
    g.add(core);
    const r = rng(17);
    const rings = [[0.12, 0.3, 9, 0.17], [0.36, 0.45, 11, 0.16], [0.62, 0.48, 12, 0.15], [0.85, 0.47, 14, 0.12]];
    for (const [y, rad, n, s] of rings) {
      for (let i = 0; i < n; i++) {
        const a = (i / n) * Math.PI * 2 + r() * 0.3;
        const m = new THREE.Mesh(new THREE.SphereGeometry(s * (0.85 + r() * 0.3), 32, 24), puff);
        m.position.set(Math.cos(a) * rad, y + (r() - 0.5) * 0.04, Math.sin(a) * rad);
        g.add(m);
      }
    }
    g.add(inner(0.44, puff));
    return { group: g, outline: '#8a8fb8' };
  },

  // Pha lê: chậu bát giác trong suốt mài cạnh xanh băng; trong lòng là sỏi pha lê trắng / xanh nhạt, chỉ một lớp đất mỏng trên cùng.
  pot_skin_crystal() {
    const outer = [[0.3, 0], [0.33, 0.05], [0.47, 0.35], [0.54, 0.7], [0.54, 0.9]];
    const g = new THREE.Group();
    const glass = new THREE.MeshPhysicalMaterial({ color: 0xb8e6ff, roughness: 0.02, metalness: 0, transparent: true, opacity: 0.4, clearcoat: 1, clearcoatRoughness: 0, envMapIntensity: 3.6, flatShading: true, iridescence: 1, iridescenceIOR: 1.35, iridescenceThicknessRange: [200, 600], side: THREE.DoubleSide, depthWrite: false });
    const mesh = new THREE.Mesh(body(outer, 8), glass);
    mesh.renderOrder = 2;
    g.add(mesh);
    // Sỏi pha lê lấp lòng chậu (thấy qua thành kính): trắng, xanh băng, tím nhạt, mài cạnh, ánh lấp lánh.
    const cols = [0xffffff, 0xd6f3ff, 0xa9e3ff, 0xe6dcff, 0xc2f0f0];
    const pebMats = cols.map((c) => new THREE.MeshPhysicalMaterial({ color: c, roughness: 0.08, clearcoat: 1, envMapIntensity: 2.4, flatShading: true, iridescence: 0.6, emissive: c, emissiveIntensity: 0.12 }));
    const pebGeo = new THREE.IcosahedronGeometry(1, 0);
    const r = rng(29);
    for (let y = 0.07; y < 0.86; y += 0.045) {
      const ring = radiusAt(outer, y) * 0.86;
      const n = Math.round(ring * 26);
      for (let i = 0; i < n; i++) {
        const a = (i / n) * Math.PI * 2 + r() * 0.4;
        const d = ring * (0.55 + r() * 0.45);
        const m = new THREE.Mesh(pebGeo, pebMats[Math.floor(r() * cols.length)]);
        const sc = 0.035 + r() * 0.03;
        m.scale.set(sc, sc * (0.7 + r() * 0.4), sc);
        m.rotation.set(r() * 3, r() * 3, r() * 3);
        m.position.set(Math.cos(a) * d, y + (r() - 0.5) * 0.02, Math.sin(a) * d);
        g.add(m);
      }
    }
    const edge = new THREE.MeshPhysicalMaterial({ color: 0xf2fbff, roughness: 0.02, clearcoat: 1, envMapIntensity: 4, transparent: true, opacity: 0.95, emissive: 0xbfeaff, emissiveIntensity: 0.25 });
    const rim = new THREE.Mesh(new THREE.TorusGeometry(0.54, 0.03, 12, 8), edge);
    rim.rotation.x = Math.PI / 2; rim.position.y = RIM_Y - 0.05;
    rim.rotation.z = Math.PI / 8;
    g.add(rim);
    for (let i = 0; i < 8; i++) {
      const a = (i / 8) * Math.PI * 2;
      const pts = curve(outer, 40).map((p) => new THREE.Vector3(Math.sin(a) * p.x * 1.003, p.y, Math.cos(a) * p.x * 1.003));
      const t = new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 40, 0.008, 6), edge);
      t.renderOrder = 3; g.add(t);
    }
    // Vệt sáng sắc trên hai mặt kính phía trước bên trái.
    const shine = new THREE.MeshBasicMaterial({ color: 0xffffff, transparent: true, opacity: 0.75, depthWrite: false });
    for (const [a, w, o] of [[-0.55, 0.035, 0.75], [-0.3, 0.018, 0.5]]) {
      const pts = curve(outer, 30).slice(4, 28).map((p) => new THREE.Vector3(Math.sin(a) * p.x * 1.02, p.y, Math.cos(a) * p.x * 1.02));
      const m = new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 30, w / 2, 4), shine.clone());
      m.material.opacity = o; m.scale.set(1, 1, 0.4); m.renderOrder = 4; g.add(m);
    }
    // Lớp đất mỏng trên cùng.
    g.add(soil(0.5));
    return { group: g, outline: '#4b86b0' };
  },

  // Vàng khảm đá: bình vàng chân đế cao, quai cuộn hai bên, đá quý lớn.
  pot_skin_gold() {
    const outer = [[0.32, 0], [0.32, 0.05], [0.17, 0.12], [0.15, 0.2], [0.38, 0.32], [0.52, 0.52], [0.51, 0.72], [0.45, 0.84], [0.5, 0.9]];
    const g = new THREE.Group();
    const au = gold({ roughness: 0.16 });
    const deco = canvasTex(1024, 512, (c, w, h) => {
      c.fillStyle = '#000'; c.fillRect(0, 0, w, h); c.strokeStyle = '#fff'; c.lineWidth = 6;
      for (let i = 0; i < 12; i++) { const x = (i + 0.5) * (w / 12); swirl(c, x - 20, band(h, 0.4), 18, '#fff', 5); swirl(c, x + 20, band(h, 0.4), 18, '#fff', 5); }
      c.fillStyle = '#fff'; c.fillRect(0, band(h, 0.78), w, 6);
    }, { color: false });
    g.add(new THREE.Mesh(body(outer), gold({ roughness: 0.16, bumpMap: deco, bumpScale: 2.5 })));
    g.add(torus(0.32, 0.035, au, 0.045), torus(0.16, 0.035, au, 0.16), torus(0.5, 0.045, au, RIM_Y - 0.03));
    // quai cuộn
    for (const s of [-1, 1]) {
      const pts = [[0.5, 0.82], [0.66, 0.84], [0.68, 0.66], [0.55, 0.58]].map(([x, y]) => new THREE.Vector3(s * x, y, 0));
      g.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 40, 0.028, 12), au));
      const knob = new THREE.Mesh(new THREE.SphereGeometry(0.035, 16, 12), au); knob.position.set(s * 0.555, 0.58, 0); g.add(knob);
    }
    const y = 0.62, r = radiusAt(outer, y);
    const cols = [0xff2d55, 0x1e6bff, 0x9b4dff, 0x14c48a];
    for (let i = 0; i < 8; i++) {
      const a = (i / 8) * Math.PI * 2 + Math.PI / 2;
      const big = i % 2 === 0;
      const m = new THREE.Mesh(new THREE.OctahedronGeometry(big ? 0.07 : 0.045, 1), gem(big ? cols[0] : cols[1 + ((i >> 1) % 3)]));
      m.scale.set(1, big ? 1.2 : 1, 0.55);
      const p = new THREE.Vector3(Math.cos(a) * (r + 0.012), y, Math.sin(a) * (r + 0.012));
      m.position.copy(p); m.lookAt(p.x * 4, y, p.z * 4); g.add(m);
      const setting = new THREE.Mesh(new THREE.TorusGeometry(big ? 0.075 : 0.05, 0.014, 8, 24), au);
      setting.position.copy(p); setting.lookAt(p.x * 4, y, p.z * 4); g.add(setting);
    }
    for (let i = 0; i < 24; i++) { const a = (i / 24) * Math.PI * 2; const b = new THREE.Mesh(new THREE.SphereGeometry(0.018, 10, 8), gem(cols[i % 4])); b.position.set(Math.cos(a) * 0.5, RIM_Y + 0.012, Math.sin(a) * 0.5); g.add(b); }
    g.add(inner(0.47, gold({ roughness: 0.4 })));
    return { group: g, outline: '#6b4300' };
  },
};


// ---------- Chậu Noel VIP (chỉ ghép từ mảnh nhiệm vụ) ----------
// Phải nổi bật hẳn so với 12 chậu thường: vật liệu bóng cao cấp, chi tiết dày, đèn nhỏ phát sáng, ánh viền mạnh; bậc càng cao càng xịn.

// Hộp bo góc: hình vuông cạnh 2·half bo bán kính rad, đùn cao h từ y0 lên.
function roundBox(half, h, rad, y0 = 0, bevel = 0.025) {
  const sh = new THREE.Shape();
  const a = half - rad;
  sh.moveTo(-a, -half); sh.lineTo(a, -half); sh.quadraticCurveTo(half, -half, half, -a);
  sh.lineTo(half, a); sh.quadraticCurveTo(half, half, a, half); sh.lineTo(-a, half);
  sh.quadraticCurveTo(-half, half, -half, a); sh.lineTo(-half, -a); sh.quadraticCurveTo(-half, -half, -a, -half);
  const g = new THREE.ExtrudeGeometry(sh, { depth: h - bevel * 2, bevelEnabled: true, bevelThickness: bevel, bevelSize: bevel, bevelSegments: 4, curveSegments: 10 });
  g.rotateX(-Math.PI / 2);
  g.translate(0, y0 + bevel, 0);
  g.computeVertexNormals();
  return g;
}
// Đèn nhỏ phát sáng (bóng + quầng mờ).
function bulb(color, r = 0.022) {
  const grp = new THREE.Group();
  grp.add(new THREE.Mesh(new THREE.SphereGeometry(r, 16, 12), new THREE.MeshStandardMaterial({ color, emissive: color, emissiveIntensity: 2.2, roughness: 0.3 })));
  const halo = new THREE.Mesh(new THREE.SphereGeometry(r * 2.1, 16, 12), new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.22, depthWrite: false }));
  grp.add(halo);
  return grp;
}
// Ngôi sao 5 cánh nổi (đùn dày, vát cạnh).
function star3d(r, depth, mat) {
  const sh = new THREE.Shape();
  for (let i = 0; i < 10; i++) {
    const a = (i / 10) * Math.PI * 2 + Math.PI / 2;
    const rr = i % 2 === 0 ? r : r * 0.45;
    i === 0 ? sh.moveTo(Math.cos(a) * rr, Math.sin(a) * rr) : sh.lineTo(Math.cos(a) * rr, Math.sin(a) * rr);
  }
  const g = new THREE.ExtrudeGeometry(sh, { depth, bevelEnabled: true, bevelThickness: depth * 0.6, bevelSize: r * 0.12, bevelSegments: 3 });
  g.translate(0, 0, -depth / 2);
  return new THREE.Mesh(g, mat);
}
// Bông tuyết nổi (6 nhánh, mỗi nhánh 2 tia phụ) dựng bằng ống mảnh.
function snowflake3d(r, mat, tube = 0.006) {
  const grp = new THREE.Group();
  for (let i = 0; i < 6; i++) {
    const a = (i / 6) * Math.PI * 2;
    const dir = new THREE.Vector3(Math.cos(a), Math.sin(a), 0);
    const line = (p0, p1) => grp.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.LineCurve3(p0, p1), 1, tube, 6), mat));
    line(new THREE.Vector3(), dir.clone().multiplyScalar(r));
    for (const t of [0.45, 0.72]) {
      const base = dir.clone().multiplyScalar(r * t);
      for (const s of [-1, 1]) {
        const b = new THREE.Vector3(Math.cos(a + s * 0.75), Math.sin(a + s * 0.75), 0).multiplyScalar(r * 0.28 * (t === 0.45 ? 1 : 0.7));
        line(base, base.clone().add(b));
      }
    }
  }
  return grp;
}
// Rải hạt lấp lánh (kim tuyến) lên mặt ngoài thân lathe.
function glitter(outer, n, colors, seed, y0 = 0.05, y1 = 0.85, size = 0.008) {
  const grp = new THREE.Group();
  const r = rng(seed);
  const geo = new THREE.OctahedronGeometry(1, 0);
  const mats = colors.map((c) => new THREE.MeshStandardMaterial({ color: c, emissive: c, emissiveIntensity: 1.4, metalness: 0.6, roughness: 0.15 }));
  for (let i = 0; i < n; i++) {
    const y = y0 + r() * (y1 - y0), a = r() * Math.PI * 2, rad = radiusAt(outer, y) + 0.004;
    const m = new THREE.Mesh(geo, mats[i % mats.length]);
    const s = size * (0.6 + r() * 0.9);
    m.scale.setScalar(s); m.rotation.set(r() * 3, r() * 3, r() * 3);
    m.position.set(Math.cos(a) * rad, y, Math.sin(a) * rad);
    grp.add(m);
  }
  return grp;
}
// Nơ ruy băng 3D: hai quai phồng + nút thắt + hai đuôi, đặt tại p, quay mặt theo hướng yaw.
function bow3d(mat, size, edgeMat) {
  const grp = new THREE.Group();
  for (const s of [-1, 1]) {
    const pts = [[0, 0], [0.5, 0.42], [1.0, 0.5], [1.12, 0.1], [0.9, -0.28], [0.45, -0.22], [0, 0]].map(([x, y]) => new THREE.Vector3(s * x * size, y * size, 0));
    const curve = new THREE.CatmullRomCurve3(pts, true);
    const loop = new THREE.Mesh(new THREE.TubeGeometry(curve, 60, size * 0.2, 16, true), mat);
    loop.scale.set(1, 1, 0.55);
    grp.add(loop);
    const edge = new THREE.Mesh(new THREE.TubeGeometry(curve, 60, size * 0.05, 8, true), edgeMat);
    edge.position.z = size * 0.1;
    grp.add(edge);
    const tail = new THREE.CatmullRomCurve3([[0, 0], [0.25, -0.5], [0.4, -1.0], [0.62, -1.35]].map(([x, y]) => new THREE.Vector3(s * x * size, y * size, -0.02)));
    const t = new THREE.Mesh(new THREE.TubeGeometry(tail, 30, size * 0.13, 12), mat);
    t.scale.set(1, 1, 0.45);
    grp.add(t);
  }
  const knot = new THREE.Mesh(new THREE.SphereGeometry(size * 0.28, 24, 16), mat);
  knot.scale.set(1, 1.1, 0.75);
  grp.add(knot);
  return grp;
}

const VIP_POTS = {
  // Bậc 1 — Chậu Quà Noel: hộp quà đỏ bóng vừa bung nắp (nắp nghiêng tựa bên hông, nơ vàng lớn trên nắp), ruy băng vàng
  // xoắn bay lên từ góc hộp, sao vàng nhỏ và kim tuyến bung ra; đất nằm trong lòng hộp.
  pot_skin_vip_gift() {
    const g = new THREE.Group();
    const red = new THREE.MeshPhysicalMaterial({ color: 0xa8001a, roughness: 0.12, clearcoat: 1, clearcoatRoughness: 0.03, envMapIntensity: 1.3 });
    const au = gold({ roughness: 0.12, envMapIntensity: 2.2 });
    const satin = new THREE.MeshPhysicalMaterial({ color: 0xffcf3a, metalness: 0.8, roughness: 0.18, clearcoat: 1, envMapIntensity: 2.4, sheen: 1, sheenColor: new THREE.Color(0xfff2a8), side: THREE.DoubleSide });
    const half = 0.36, top = 0.9;
    g.add(new THREE.Mesh(roundBox(half, top, 0.07), red));
    // Ruy băng dọc bốn mặt.
    for (let i = 0; i < 4; i++) {
      const rib = new THREE.Mesh(new THREE.BoxGeometry(0.12, top - 0.02, 0.028), au);
      const a = (i / 4) * Math.PI * 2;
      rib.position.set(Math.sin(a) * (half + 0.035), top / 2, Math.cos(a) * (half + 0.035));
      rib.rotation.y = a; g.add(rib);
    }
    // Miệng hộp: viền vàng quanh đất.
    const lip = new THREE.Mesh(roundBox(half + 0.012, 0.05, 0.075, top - 0.05), au);
    g.add(lip);
    g.add(soil(0.31));
    // Nắp hộp nghiêng tựa vào hông phải phía trước, có nơ lớn.
    const lid = new THREE.Group();
    lid.add(new THREE.Mesh(roundBox(half + 0.04, 0.12, 0.08), red));
    for (const a of [0, Math.PI / 2]) { const b = new THREE.Mesh(new THREE.BoxGeometry(0.12, 0.125, 2 * half + 0.1), au); b.rotation.y = a; b.position.y = 0.06; lid.add(b); }
    const bow = bow3d(satin, 0.16, new THREE.MeshPhysicalMaterial({ color: 0xffe9a0, metalness: 0.9, roughness: 0.15 }));
    bow.position.set(0, 0.2, 0); bow.rotation.x = -0.2; lid.add(bow);
    lid.position.set(0.42, 0.0, 0.3); lid.rotation.set(0.25, -0.75, 1.15); lid.scale.setScalar(0.72);
    // Hộp quà nhỏ xanh lá ở chân bên trái.
    const mini = new THREE.Group();
    mini.add(new THREE.Mesh(roundBox(0.13, 0.22, 0.03), new THREE.MeshPhysicalMaterial({ color: 0x0b8a3e, roughness: 0.15, clearcoat: 1, envMapIntensity: 1.3 })));
    for (const a of [0, Math.PI / 2]) { const b = new THREE.Mesh(new THREE.BoxGeometry(0.05, 0.225, 0.3), au); b.rotation.y = a; b.position.y = 0.11; mini.add(b); }
    const mbow = bow3d(satin, 0.06, au); mbow.position.y = 0.25; mini.add(mbow);
    mini.position.set(-0.38, 0, 0.36); mini.rotation.y = 0.5; g.add(mini);
    g.add(lid);
    // Ruy băng xoắn bay lên từ hai góc.
    for (const [x, z, ph, h] of [[-0.3, 0.3, 0, 0.42], [-0.28, -0.32, 2.1, 0.36], [0.3, -0.3, 4.0, 0.3]]) {
      const pts = [];
      for (let t = 0; t <= 1; t += 0.05) pts.push(new THREE.Vector3(x + Math.cos(ph + t * 9) * 0.07 * (1 - t * 0.3) - t * 0.06 * Math.sign(x), top + t * h, z + Math.sin(ph + t * 9) * 0.07));
      const rib = new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 80, 0.016, 8), satin);
      rib.scale.set(1, 1, 1); g.add(rib);
    }
    // Sao vàng nhỏ + kim tuyến bung ra.
    const starMat = new THREE.MeshPhysicalMaterial({ color: 0xffe066, metalness: 1, roughness: 0.1, emissive: 0xffb300, emissiveIntensity: 0.8, envMapIntensity: 2.5 });
    const r = rng(43);
    for (const [x, y, z, s] of [[-0.42, 1.08, 0.05, 0.06], [0.08, 1.12, -0.36, 0.05], [-0.15, 1.05, 0.34, 0.04], [0.42, 0.98, -0.12, 0.045]]) {
      const st = star3d(s, 0.015, starMat); st.position.set(x, y, z); st.rotation.set(r() - 0.5, r() * 3, r() - 0.5); g.add(st);
    }
    const conf = [0xffd54a, 0xff3b5c, 0xffffff, 0x3ad17a];
    for (let i = 0; i < 40; i++) {
      const a = r() * Math.PI * 2, d = 0.15 + r() * 0.35;
      const m = new THREE.Mesh(new THREE.OctahedronGeometry(0.01 + r() * 0.008, 0), new THREE.MeshStandardMaterial({ color: conf[i % 4], emissive: conf[i % 4], emissiveIntensity: 0.9, metalness: 0.5, roughness: 0.2 }));
      m.position.set(Math.cos(a) * d, top + 0.04 + r() * 0.25, Math.sin(a) * d); m.rotation.set(r() * 3, r() * 3, r() * 3); g.add(m);
    }
    // Kim tuyến vàng rải trên mặt hộp.
    const gl = new THREE.MeshStandardMaterial({ color: 0xffe08a, emissive: 0xffc94a, emissiveIntensity: 1.2, metalness: 0.8, roughness: 0.1 });
    for (let i = 0; i < 60; i++) {
      const a = Math.floor(r() * 4) * Math.PI / 2, u = (r() - 0.5) * 0.6, y = 0.08 + r() * 0.74;
      if (Math.abs(u) < 0.08) continue;
      const m = new THREE.Mesh(new THREE.OctahedronGeometry(0.006 + r() * 0.006, 0), gl);
      m.position.copy(new THREE.Vector3(Math.sin(a), 0, Math.cos(a)).multiplyScalar(half + 0.03).add(new THREE.Vector3(Math.cos(a), 0, -Math.sin(a)).multiplyScalar(u)));
      m.position.y = y; g.add(m);
    }
    g.scale.setScalar(0.84);
    return { group: g, outline: '#5e0610', glow: '#ffcf5a', vip: true, y: SOIL_Y * (1 - 0.84) };
  },

  // Bậc 2 — Chậu Người Tuyết: người tuyết lấp lánh đội chiếc mũ chóp to — cây mọc từ miệng mũ. Mắt than, mũi cà rốt, má hồng,
  // tay cành cây đeo găng đỏ, khăn len đỏ trắng có đuôi bay, cúc than, bông tuyết nổi phát sáng.
  pot_skin_vip_snowman() {
    const g = new THREE.Group();
    const snow = new THREE.MeshPhysicalMaterial({ color: 0xffffff, roughness: 0.4, sheen: 1, sheenRoughness: 0.3, sheenColor: new THREE.Color(0xbfe4ff), clearcoat: 0.45, clearcoatRoughness: 0.2, envMapIntensity: 1.25, iridescence: 0.25 });
    const coal = new THREE.MeshPhysicalMaterial({ color: 0x1b1b22, roughness: 0.2, clearcoat: 1, envMapIntensity: 1.5 });
    const yaw0 = 0.35; // bù góc xoay chung để mặt nhìn thẳng
    const fwd = (yaw, rr, y) => new THREE.Vector3(Math.sin(yaw) * rr, y, Math.cos(yaw) * rr);
    // Thân + đầu.
    const bodyC = 0.13, bodyR = 0.3, headC = 0.5, headR = 0.215;
    const b1 = new THREE.Mesh(new THREE.SphereGeometry(bodyR, 48, 32), snow); b1.position.y = bodyC; b1.scale.y = 0.9; g.add(b1);
    const b2 = new THREE.Mesh(new THREE.SphereGeometry(headR, 48, 32), snow); b2.position.y = headC; g.add(b2);
    const sparkle = (cy, rad, n, seed) => {
      const r = rng(seed); const mats = [0xffffff, 0xcff0ff, 0xa8dcff].map((c) => new THREE.MeshStandardMaterial({ color: c, emissive: c, emissiveIntensity: 1.4, metalness: 0.6, roughness: 0.15 }));
      for (let i = 0; i < n; i++) { const u = r() * 2 - 1, a = r() * Math.PI * 2, s = Math.sqrt(1 - u * u); const m = new THREE.Mesh(new THREE.OctahedronGeometry(0.006 + r() * 0.005, 0), mats[i % 3]); m.position.set(Math.cos(a) * s * rad, cy + u * rad * 0.9, Math.sin(a) * s * rad); g.add(m); }
    };
    sparkle(bodyC, bodyR + 0.004, 140, 51); sparkle(headC, headR + 0.004, 90, 52);
    // Mặt.
    for (const s of [-1, 1]) {
      const e = new THREE.Mesh(new THREE.SphereGeometry(0.032, 20, 16), coal); e.position.copy(fwd(yaw0 + s * 0.32, headR - 0.005, headC + 0.06)); g.add(e);
      const hl = new THREE.Mesh(new THREE.SphereGeometry(0.01, 10, 8), new THREE.MeshBasicMaterial({ color: 0xffffff })); hl.position.copy(fwd(yaw0 + s * 0.32 - 0.06, headR + 0.02, headC + 0.075)); g.add(hl);
      const cp = fwd(yaw0 + s * 0.55, headR + 0.003, headC - 0.03);
      const cheek = new THREE.Mesh(new THREE.CircleGeometry(0.038, 24), new THREE.MeshBasicMaterial({ color: 0xff7aa0, transparent: true, opacity: 0.6, depthWrite: false }));
      cheek.position.copy(cp); cheek.lookAt(cp.x * 3, cp.y, cp.z * 3); g.add(cheek);
    }
    for (let i = -2; i <= 2; i++) { const m = new THREE.Mesh(new THREE.SphereGeometry(0.014, 12, 10), coal); m.position.copy(fwd(yaw0 + i * 0.16, headR - 0.003, headC - 0.1 + Math.abs(i) * 0.022)); g.add(m); }
    const np = fwd(yaw0, headR, headC);
    const nose = new THREE.Mesh(new THREE.ConeGeometry(0.035, 0.2, 20), new THREE.MeshPhysicalMaterial({ color: 0xff7a1a, roughness: 0.35, clearcoat: 0.6 }));
    nose.position.copy(fwd(yaw0, headR + 0.085, headC - 0.005)); nose.lookAt(np.x * 5, headC - 0.04, np.z * 5); nose.rotateX(Math.PI / 2); g.add(nose);
    // Cúc than.
    for (const y of [0.25, 0.12, -0.01]) { const m = new THREE.Mesh(new THREE.SphereGeometry(0.024, 14, 10), coal); m.position.copy(fwd(yaw0, Math.sqrt(Math.max(0, bodyR ** 2 - ((y - bodyC) / 0.9) ** 2)) + 0.005, y)); g.add(m); }
    // Khăn len đỏ trắng + đuôi bay.
    const knit = canvasTex(512, 64, (c, w, h) => { for (let i = 0; i < 16; i++) { c.fillStyle = i % 2 ? '#ffffff' : '#e3122d'; c.fillRect((i * w) / 16, 0, w / 16 + 1, h); } c.fillStyle = 'rgba(0,0,0,.12)'; for (let x = 0; x < w; x += 6) c.fillRect(x, 0, 2, h); });
    const knitMat = new THREE.MeshPhysicalMaterial({ map: knit, roughness: 0.8, sheen: 1, sheenColor: new THREE.Color(0xffd0d8) });
    const scarf = new THREE.Mesh(new THREE.TorusGeometry(0.2, 0.05, 18, 64), knitMat); scarf.rotation.x = Math.PI / 2; scarf.position.y = 0.33; g.add(scarf);
    const tailPts = [[0.0, 0.33], [0.12, 0.28], [0.26, 0.24], [0.4, 0.27], [0.5, 0.22]].map(([d, y]) => { const v = fwd(yaw0 + 0.95, 0.2 + d, y); return v; });
    g.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(tailPts), 30, 0.042, 12), knitMat));
    // Tay cành cây + găng đỏ.
    const wood = new THREE.MeshStandardMaterial({ color: 0x5b3a1e, roughness: 0.8 });
    const mitt = new THREE.MeshPhysicalMaterial({ color: 0xe3122d, roughness: 0.6, sheen: 1, sheenColor: new THREE.Color(0xffb0b8) });
    for (const s of [-1, 1]) {
      const a = yaw0 + s * Math.PI / 2;
      const p0 = fwd(a, bodyR - 0.04, 0.22), p1 = fwd(a, bodyR + 0.17, 0.36), p2 = fwd(a, bodyR + 0.27, 0.46);
      g.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3([p0, p1, p2]), 12, 0.014, 6), wood));
      const twig = new THREE.Mesh(new THREE.TubeGeometry(new THREE.LineCurve3(p1, p1.clone().add(new THREE.Vector3(0, 0.08, 0))), 1, 0.008, 6), wood); g.add(twig);
      const m = new THREE.Mesh(new THREE.SphereGeometry(0.045, 16, 12), mitt); m.position.copy(p2); m.scale.set(1, 1.2, 0.8); g.add(m);
    }
    // Mũ chóp to = chậu: vành ôm đỉnh đầu, chóp mở miệng chứa đất.
    const felt = new THREE.MeshPhysicalMaterial({ color: 0x1a1a24, roughness: 0.3, clearcoat: 0.9, clearcoatRoughness: 0.15, sheen: 0.8, sheenColor: new THREE.Color(0x6a6aa0), envMapIntensity: 1.2 });
    const brimY = 0.66, topY = 0.9, crownR = 0.31;
    const brim = new THREE.Mesh(new THREE.CylinderGeometry(0.44, 0.44, 0.035, 64), felt); brim.position.y = brimY; g.add(brim);
    g.add(torus(0.44, 0.018, felt, brimY));
    const crown = new THREE.Mesh(new THREE.CylinderGeometry(crownR + 0.015, crownR, topY - brimY, 64, 1, true), felt); crown.position.y = (topY + brimY) / 2; g.add(crown);
    g.add(torus(crownR + 0.015, 0.022, felt, topY));
    const band = new THREE.Mesh(new THREE.CylinderGeometry(crownR + 0.008, crownR + 0.006, 0.07, 64, 1, true), new THREE.MeshPhysicalMaterial({ color: 0xe3122d, roughness: 0.2, clearcoat: 1, side: THREE.DoubleSide })); band.position.y = brimY + 0.06; g.add(band);
    // Nhành ô rô + quả đỏ + bông tuyết trên dải mũ.
    const holly = new THREE.MeshPhysicalMaterial({ color: 0x0f7a34, roughness: 0.25, clearcoat: 1 });
    const hp = fwd(yaw0 + 0.7, crownR + 0.03, brimY + 0.07);
    for (const k of [-1, 1]) { const lf = new THREE.Mesh(new THREE.SphereGeometry(0.04, 12, 8), holly); lf.scale.set(1.6, 0.5, 0.6); lf.position.copy(hp).add(new THREE.Vector3(k * 0.04, 0.01, 0)); lf.rotation.z = k * 0.5; g.add(lf); }
    for (const [dx, dy] of [[0, 0.02], [0.025, 0.0], [-0.02, -0.005]]) { const b = new THREE.Mesh(new THREE.SphereGeometry(0.018, 12, 8), new THREE.MeshPhysicalMaterial({ color: 0xff1a2e, clearcoat: 1, emissive: 0x700010, emissiveIntensity: 0.6 })); b.position.copy(hp).add(new THREE.Vector3(dx, dy + 0.02, 0.02)); g.add(b); }
    const flakeMat = new THREE.MeshStandardMaterial({ color: 0xdff4ff, emissive: 0x8fd8ff, emissiveIntensity: 1.8, metalness: 0.3, roughness: 0.2 });
    const fp = fwd(yaw0 - 0.5, crownR + 0.02, brimY + 0.06); const fl = snowflake3d(0.05, flakeMat, 0.006); fl.position.copy(fp); fl.lookAt(fp.x * 3, fp.y, fp.z * 3); g.add(fl);
    // Tuyết đọng trên vành mũ.
    const snowM = new THREE.MeshPhysicalMaterial({ color: 0xffffff, roughness: 0.35, sheen: 1, sheenColor: new THREE.Color(0xd8f0ff), emissive: 0xffffff, emissiveIntensity: 0.1 });
    const rr = rng(61);
    for (let i = 0; i < 14; i++) { const a = rr() * Math.PI * 2; const m = new THREE.Mesh(new THREE.SphereGeometry(0.03 + rr() * 0.02, 12, 8), snowM); m.scale.set(1.4, 0.4, 1.1); m.position.set(Math.cos(a) * (0.36 + rr() * 0.05), brimY + 0.02, Math.sin(a) * (0.36 + rr() * 0.05)); g.add(m); }
    // Đế tuyết nhỏ.
    const base = new THREE.Mesh(new THREE.SphereGeometry(0.36, 32, 16, 0, Math.PI * 2, 0, Math.PI / 2), snowM); base.scale.set(1, 0.18, 1); base.position.y = -0.1; g.add(base);
    g.add(soil(crownR - 0.01));
    return { group: g, outline: '#2f5578', glow: '#9fe0ff', vip: true };
  },

  // Bậc 3 — Chậu Thông Vàng: xe trượt tuyết của Ông già Noel mạ vàng — thân cong cuộn đầu, đệm nhung đỏ, đất trên lòng ghế,
  // càng trượt vàng cuộn tròn, đá quý nạm hai bên, dây đèn ấm dọc mép, ngôi sao vàng phát sáng ở mũi, cây thông nhỏ mạ vàng
  // và túi quà phía sau, tuyết lấp lánh.
  pot_skin_vip_pine() {
    const g = new THREE.Group();
    const au = gold({ roughness: 0.1, envMapIntensity: 2.6, clearcoat: 1, clearcoatRoughness: 0.04 });
    const au2 = gold({ color: 0xffb52e, roughness: 0.18, envMapIntensity: 2.2 });
    const velvet = new THREE.MeshPhysicalMaterial({ color: 0xb00020, roughness: 0.55, sheen: 1, sheenRoughness: 0.35, sheenColor: new THREE.Color(0xff6b80), envMapIntensity: 0.9 });
    // Thân xe: mặt cắt hông (x dài, y cao) đùn theo z.
    const sh = new THREE.Shape();
    sh.moveTo(0.46, 0.3);
    sh.bezierCurveTo(0.52, 0.5, 0.5, 0.78, 0.56, 0.92); // lưng ghế
    sh.bezierCurveTo(0.6, 1.0, 0.5, 1.02, 0.46, 0.94); // cuộn lưng
    sh.lineTo(0.42, 0.86);
    sh.lineTo(-0.3, 0.84);
    sh.bezierCurveTo(-0.42, 0.86, -0.5, 0.96, -0.52, 1.02); // mũi vươn lên
    sh.bezierCurveTo(-0.55, 1.1, -0.66, 1.06, -0.63, 0.96); // cuộn mũi
    sh.bezierCurveTo(-0.6, 0.86, -0.52, 0.86, -0.55, 0.92);
    sh.bezierCurveTo(-0.62, 0.7, -0.5, 0.45, -0.36, 0.3);
    sh.lineTo(0.46, 0.3);
    const depth = 0.52;
    const sideGeo = new THREE.ExtrudeGeometry(sh, { depth, bevelEnabled: true, bevelThickness: 0.03, bevelSize: 0.025, bevelSegments: 5, curveSegments: 32 });
    sideGeo.translate(0, 0, -depth / 2);
    const bodyMesh = new THREE.Mesh(sideGeo, au);
    g.add(bodyMesh);
    // Đệm nhung đỏ phủ mặt ghế (đất nằm giữa).
    const cushion = new THREE.Mesh(roundBox(0.3, 0.06, 0.08, 0.84), velvet); cushion.scale.set(1.25, 1, 0.92); g.add(cushion);
    g.add(torus(0.3, 0.022, au2, 0.905));
    g.add(soil(0.29));
    // Ô nhung đỏ viền vàng hai bên hông + đá quý.
    for (const zs of [-1, 1]) {
      const panel = new THREE.Mesh(roundBox(0.18, 0.012, 0.05), velvet);
      panel.scale.set(1.9, 1, 1); panel.rotation.x = zs * Math.PI / 2; panel.position.set(0.02, 0.58, zs * (depth / 2 + 0.035));
      g.add(panel);
      const frame = new THREE.Mesh(new THREE.TorusGeometry(1, 0.012, 8, 64), au2); frame.scale.set(0.36, 0.2, 1); frame.position.set(0.02, 0.58, zs * (depth / 2 + 0.045)); g.add(frame);
      const gems = [0xff1744, 0x00c27a, 0x2f7bff, 0x00c27a, 0xff1744];
      gems.forEach((c, i) => { const m = new THREE.Mesh(new THREE.OctahedronGeometry(i === 2 ? 0.045 : 0.028, 1), gem(c)); m.scale.z = 0.6; m.position.set(-0.26 + i * 0.14, i === 2 ? 0.58 : 0.58 + (i % 2 ? 0.08 : -0.08) * 0, zs * (depth / 2 + 0.05)); if (i !== 2) m.position.y = 0.58; g.add(m); });
    }
    // Càng trượt cuộn tròn ở mũi + thanh chống.
    for (const zs of [-1, 1]) {
      const z = zs * 0.22;
      const rp = [[0.5, 0.06], [0.1, 0.03], [-0.4, 0.04], [-0.6, 0.14], [-0.66, 0.28], [-0.58, 0.36], [-0.52, 0.28]].map(([x, y]) => new THREE.Vector3(x, y, z));
      g.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(rp), 80, 0.03, 12), au));
      for (const x of [-0.25, 0.08, 0.38]) { const st = new THREE.Mesh(new THREE.CylinderGeometry(0.018, 0.018, 0.27, 10), au2); st.position.set(x, 0.17, z); g.add(st); }
    }
    // Dây đèn ấm dọc mép trên hai bên.
    const warm = [0xffe08a, 0xff7a7a, 0x9ad8ff, 0xfff6d6, 0x7dffb0];
    for (const zs of [-1, 1]) for (let i = 0; i < 9; i++) {
      const x = -0.3 + i * 0.085;
      const b = bulb(warm[(i + (zs > 0 ? 0 : 2)) % warm.length], 0.016); b.position.set(x, 0.885, zs * (depth / 2 + 0.03)); g.add(b);
    }
    // Ngôi sao vàng phát sáng ở mũi xe.
    const starMat = new THREE.MeshPhysicalMaterial({ color: 0xffe066, metalness: 1, roughness: 0.08, envMapIntensity: 3, emissive: 0xffb300, emissiveIntensity: 1.0, clearcoat: 1 });
    const st = star3d(0.085, 0.025, starMat); st.position.set(-0.6, 1.05, 0.0); st.rotation.y = 0.35; g.add(st);
    const halo = new THREE.Mesh(new THREE.CircleGeometry(0.15, 40), new THREE.MeshBasicMaterial({ color: 0xffd54a, transparent: true, opacity: 0.28, depthWrite: false })); halo.position.set(-0.605, 1.05, -0.02); halo.rotation.y = 0.35; g.add(halo);
    // Cây thông nhỏ mạ vàng + túi quà đỏ ở sau lưng ghế (cạnh đất, không che giữa).
    const tree = new THREE.Group();
    for (const [rr, y, h] of [[0.09, 0.0, 0.12], [0.075, 0.07, 0.11], [0.055, 0.14, 0.1]]) { const c = new THREE.Mesh(new THREE.ConeGeometry(rr, h, 14), au); c.position.y = y + h / 2; tree.add(c); }
    for (let i = 0; i < 7; i++) { const b = bulb(warm[i % warm.length], 0.008); const a = i * 2.2; b.position.set(Math.cos(a) * 0.06, 0.04 + i * 0.022, Math.sin(a) * 0.06); tree.add(b); }
    const ts = star3d(0.025, 0.01, starMat); ts.position.y = 0.26; tree.add(ts);
    tree.scale.setScalar(1.7); tree.position.set(0.42, 0.86, -0.13); g.add(tree);
    const sack = new THREE.Mesh(new THREE.SphereGeometry(0.09, 24, 16), velvet); sack.scale.set(1, 1.15, 0.9); sack.position.set(0.42, 0.97, 0.13); g.add(sack);
    g.add(torus(0.03, 0.012, au2, 1.07).translateX(0.42).translateY(0.13));
    // Tuyết lấp lánh trên càng và mũi.
    const snowM = new THREE.MeshPhysicalMaterial({ color: 0xffffff, roughness: 0.35, sheen: 1, sheenColor: new THREE.Color(0xd8f0ff), emissive: 0xffffff, emissiveIntensity: 0.12 });
    const rr = rng(97);
    for (let i = 0; i < 16; i++) { const m = new THREE.Mesh(new THREE.SphereGeometry(0.02 + rr() * 0.015, 10, 8), snowM); m.scale.set(1.4, 0.5, 1); m.position.set(-0.5 + rr() * 1.0, 0.07, (rr() < 0.5 ? -1 : 1) * 0.22); g.add(m); }
    const gl = [0xffffff, 0xfff1b0].map((c) => new THREE.MeshStandardMaterial({ color: c, emissive: c, emissiveIntensity: 1.5, metalness: 0.7, roughness: 0.1 }));
    for (let i = 0; i < 70; i++) { const m = new THREE.Mesh(new THREE.OctahedronGeometry(0.006 + rr() * 0.006, 0), gl[i % 2]); m.position.set(-0.45 + rr() * 0.9, 0.34 + rr() * 0.5, (rr() < 0.5 ? -1 : 1) * (depth / 2 + 0.03)); g.add(m); }
    g.scale.setScalar(0.8);
    return { group: g, outline: '#5a3a00', glow: '#ffd24a', vip: true, y: SOIL_Y * (1 - 0.8) };
  },
};
Object.assign(POTS, VIP_POTS);

function lights(scene) {
  scene.environment = envTex;
  scene.add(new THREE.HemisphereLight(0xffffff, 0x8aa6c0, 0.9));
  const key = new THREE.DirectionalLight(0xfff4e0, 2.4);
  key.position.set(-2.5, 4, 3);
  scene.add(key);
  const rim = new THREE.DirectionalLight(0xcfe8ff, 1.6);
  rim.position.set(3, 2, -2.5);
  scene.add(rim);
  const fill = new THREE.DirectionalLight(0xffffff, 0.5);
  fill.position.set(2, 0.5, 3);
  scene.add(fill);
}

window.POT_IDS = Object.keys(POTS);
window.SOIL_Y = SOIL_Y;
window.renderPot = (id) => {
  const scene = new THREE.Scene();
  lights(scene);
  const { group, outline, y = 0, glow, vip } = POTS[id]();
  if (vip) {
    // Chậu VIP: thêm ánh viền mạnh hai bên để nổi khối, sáng hơn chậu thường.
    const rimL = new THREE.DirectionalLight(0xfff0c8, 2.2); rimL.position.set(-3.5, 1.5, -2); scene.add(rimL);
    const rimR = new THREE.DirectionalLight(0xd8ecff, 2.0); rimR.position.set(3.5, 1.2, -1.5); scene.add(rimR);
    const top = new THREE.DirectionalLight(0xffffff, 0.8); top.position.set(0, 5, 1); scene.add(top);
  }
  group.position.y = y;
  group.rotation.y = -0.35; // xoay nhẹ để hoạ tiết không đối xứng cứng
  scene.add(group);
  renderer.render(scene, camera);
  return { png: renderer.domElement.toDataURL('image/png'), outline, glow, vip: !!vip };
};
window.potsReady = true;
