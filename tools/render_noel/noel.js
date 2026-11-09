// Dựng đồ trang trí Noel + NPC nông trại bằng three.js rồi xuất ảnh (window.renderItem(id) → data URL PNG nền trong suốt).
// Ảnh tự dựng cho DCity (CC0). Hoạ tiết vẽ bằng canvas ngay trong script này, không đưa vào game.
// 1 đơn vị thế giới = 100 px hiển thị; dựng ở 4× (400 px / đơn vị), hậu kỳ (finish.py) thu về 2× và cắt sát hình.
import * as THREE from 'three';
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js';

const TILT = (26 * Math.PI) / 180; // góc nhìn từ trên xuống, gần với ảnh 3D sẵn có của nông trại
const PX = 400; // px ảnh thô trên một đơn vị

const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
renderer.setPixelRatio(1);
renderer.setClearColor(0x000000, 0);
renderer.outputColorSpace = THREE.SRGBColorSpace;
renderer.toneMapping = THREE.NeutralToneMapping;
renderer.toneMappingExposure = 1.05;
document.body.appendChild(renderer.domElement);
const pmrem = new THREE.PMREMGenerator(renderer);
const envTex = pmrem.fromScene(new RoomEnvironment(), 0.04).texture;

// ---------- tiện ích ----------
function rng(seed) {
  let s = seed >>> 0;
  return () => ((s = (s * 1664525 + 1013904223) >>> 0) / 4294967296);
}
const mat = (color, o = {}) => new THREE.MeshStandardMaterial({ color, roughness: 0.55, metalness: 0, envMapIntensity: 0.6, ...o });
const glossy = (color, o = {}) => new THREE.MeshPhysicalMaterial({ color, roughness: 0.18, metalness: 0.1, clearcoat: 1, clearcoatRoughness: 0.1, envMapIntensity: 1, ...o });
const metal = (color) => new THREE.MeshStandardMaterial({ color, roughness: 0.25, metalness: 0.85, envMapIntensity: 1.2 });
const glow = (color, k = 1) => new THREE.MeshStandardMaterial({ color, emissive: color, emissiveIntensity: k, roughness: 0.3 });
function mesh(geo, m, x = 0, y = 0, z = 0) {
  const o = new THREE.Mesh(geo, m);
  o.position.set(x, y, z);
  return o;
}
const sphere = (r, m, x, y, z, seg = 32) => mesh(new THREE.SphereGeometry(r, seg, Math.round(seg * 0.75)), m, x, y, z);
function canvasTex(w, h, draw) {
  const c = document.createElement('canvas');
  c.width = w; c.height = h;
  draw(c.getContext('2d'), w, h);
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  t.wrapS = t.wrapT = THREE.RepeatWrapping;
  t.anisotropy = 8;
  return t;
}
// Ụ tuyết gồ ghề (cục tuyết đọng): cầu dẹt, đỉnh lồi lõm nhẹ.
function snowLump(rx, ry, rz, seed = 1) {
  const g = new THREE.SphereGeometry(1, 28, 18);
  const p = g.attributes.position;
  const r = rng(seed);
  const ph = [r() * 6, r() * 6, r() * 6];
  for (let i = 0; i < p.count; i++) {
    const x = p.getX(i), y = p.getY(i), z = p.getZ(i);
    const n = 1 + 0.08 * Math.sin(x * 5 + ph[0]) * Math.cos(z * 4 + ph[1]) + 0.05 * Math.sin(y * 7 + ph[2]);
    p.setXYZ(i, x * rx * n, y * ry * n, z * rz * n);
  }
  g.computeVertexNormals();
  return new THREE.Mesh(g, SNOW);
}
const SNOW = new THREE.MeshStandardMaterial({ color: 0xf6faff, roughness: 0.8, envMapIntensity: 0.9 });

// ---------- cây thông Noel ----------
// lit: 'a' | 'b' — bộ đèn nào sáng (hai ảnh luân phiên tạo hiệu ứng nhấp nháy).
function xmasTree(lit) {
  const g = new THREE.Group();
  const needles = canvasTex(256, 256, (c, w, h) => {
    c.fillStyle = '#1d6e30'; c.fillRect(0, 0, w, h);
    const r = rng(5);
    for (let i = 0; i < 900; i++) {
      const x = r() * w, y = r() * h, l = 6 + r() * 10;
      c.strokeStyle = r() < 0.5 ? '#2c8c40' : '#145423';
      c.lineWidth = 2;
      c.beginPath(); c.moveTo(x, y); c.lineTo(x + l * 0.4, y + l); c.stroke();
    }
  });
  needles.repeat.set(3, 1);
  const green = mat(0xd8e8d8, { map: needles, roughness: 0.8, envMapIntensity: 0.4 });
  // Chậu gỗ + thân
  g.add(mesh(new THREE.CylinderGeometry(0.2, 0.17, 0.2, 28), mat(0xb5432f), 0, 0.1, 0));
  g.add(mesh(new THREE.CylinderGeometry(0.215, 0.215, 0.04, 28), mat(0xe9c46a), 0, 0.2, 0));
  g.add(mesh(new THREE.CylinderGeometry(0.06, 0.07, 0.2, 12), mat(0x6b4226), 0, 0.28, 0));
  // Bốn tầng tán: chóp nón có mép lượn sóng, mép đọng tuyết.
  const tiers = [[0.62, 0.5, 0.36], [0.52, 0.46, 0.66], [0.4, 0.42, 0.94], [0.28, 0.38, 1.2]];
  const r = rng(11);
  const bulbsA = [], bulbsB = [];
  tiers.forEach(([rad, hgt, y], ti) => {
    const geo = new THREE.ConeGeometry(rad, hgt, 40, 4, true);
    const p = geo.attributes.position;
    for (let i = 0; i < p.count; i++) {
      const x = p.getX(i), yy = p.getY(i), z = p.getZ(i);
      if (yy < -hgt / 2 + 0.001) {
        const a = Math.atan2(z, x);
        const k = 1 + 0.06 * Math.sin(a * 9 + ti);
        p.setX(i, x * k); p.setZ(i, z * k); p.setY(i, yy + 0.03 * Math.sin(a * 9 + ti));
      }
    }
    geo.computeVertexNormals();
    const cone = new THREE.Mesh(geo, green);
    cone.position.y = y + hgt / 2;
    g.add(cone);
    // Đáy kín
    const cap = mesh(new THREE.CircleGeometry(rad, 40), mat(0x1f6a2a), 0, y + 0.01, 0);
    cap.rotation.x = Math.PI / 2;
    g.add(cap);
    // Tuyết trên vai tầng: vài ụ quanh mép phía trên
    for (let k = 0; k < 7; k++) {
      const a = -Math.PI * 0.95 + (k / 6) * Math.PI * 0.9 + (r() - 0.5) * 0.2; // nửa trước
      const rr = rad * 0.72;
      const lump = snowLump(0.09 + r() * 0.03, 0.035, 0.07, ti * 10 + k);
      lump.position.set(Math.cos(a) * rr, y + hgt * 0.32, -Math.sin(a) * rr);
      lump.rotation.y = a;
      lump.rotation.z = 0.5;
      g.add(lump);
    }
    // Quả châu + bóng đèn quanh mép dưới
    const n = 8 - ti;
    for (let k = 0; k < n; k++) {
      const a = -Math.PI + ((k + 0.5) / n) * Math.PI; // chỉ phía trước (z > 0)
      const rr = rad * 0.94;
      const x = Math.cos(a) * rr, z = -Math.sin(a) * rr;
      if (k % 2 === 0) {
        const cols = [0xd62828, 0xf2b705, 0x2a6fdb, 0xc0c0c8];
        g.add(sphere(0.045, glossy(cols[(k / 2 + ti) % 4]), x, y + 0.04, z));
      } else {
        const on = (k + ti) % 4 < 2 ? 'a' : 'b';
        const cols = [0xffd23f, 0xff5e5b, 0x7bdff2, 0xb8f35a];
        const col = cols[(k + ti) % 4];
        const b = sphere(on === lit ? 0.034 : 0.026, on === lit ? glow(col, 3) : mat(0x555555, { color: new THREE.Color(col).multiplyScalar(0.45), roughness: 0.4 }), x * 1.02, y + 0.07, z * 1.02, 16);
        g.add(b);
        (on === 'a' ? bulbsA : bulbsB).push(b);
      }
    }
  });
  // Dây kim tuyến xoắn quanh tán (phía trước)
  const pts = [];
  for (let i = 0; i <= 120; i++) {
    const t = i / 120, y = 0.4 + t * 1.0, rad = 0.6 - t * 0.42;
    const a = -Math.PI * 0.95 + t * Math.PI * 4.4;
    pts.push(new THREE.Vector3(Math.cos(a) * rad, y - 0.06 * Math.sin(a * 0.5), -Math.sin(a) * rad));
  }
  const tinsel = new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 240, 0.014, 8), metal(0xf2c14e));
  g.add(tinsel);
  // Ngôi sao vàng trên đỉnh
  const s = new THREE.Shape();
  for (let i = 0; i < 10; i++) {
    const a = Math.PI / 2 + (i * Math.PI) / 5, rr = i % 2 ? 0.055 : 0.13;
    const x = Math.cos(a) * rr, y = Math.sin(a) * rr;
    i ? s.lineTo(x, y) : s.moveTo(x, y);
  }
  const star = new THREE.Mesh(new THREE.ExtrudeGeometry(s, { depth: 0.04, bevelEnabled: true, bevelSize: 0.012, bevelThickness: 0.015, bevelSegments: 2 }),
    new THREE.MeshStandardMaterial({ color: 0xffd23f, emissive: 0xffb000, emissiveIntensity: lit === 'a' ? 0.9 : 0.6, metalness: 0.5, roughness: 0.3 }));
  star.position.set(0, 1.66, -0.02);
  g.add(star);
  return { group: g, frame: [-0.8, 0.8, 1.95, -0.25], outline: '#1d4a22' };
}

// ---------- người tuyết ----------
function snowman() {
  const g = new THREE.Group();
  g.add(snowLump(0.32, 0.27, 0.3, 3).translateY(0.25));
  g.add(snowLump(0.23, 0.21, 0.22, 4).translateY(0.62));
  g.add(snowLump(0.17, 0.16, 0.17, 5).translateY(0.92));
  const coal = mat(0x23262b, { roughness: 0.6 });
  for (const [x, y] of [[-0.055, 0.96], [0.055, 0.96]]) g.add(sphere(0.022, coal, x, y, 0.15));
  for (const y of [0.55, 0.65, 0.75]) g.add(sphere(0.022, coal, 0, y, 0.215 - Math.abs(y - 0.65) * 0.35));
  const mouth = [-0.06, -0.03, 0, 0.03, 0.06];
  mouth.forEach((x) => g.add(sphere(0.012, coal, x, 0.875 - (0.06 - Math.abs(x)) * 0.3, 0.155)));
  const nose = mesh(new THREE.ConeGeometry(0.03, 0.14, 16), mat(0xf08a24), 0, 0.92, 0.22);
  nose.rotation.x = Math.PI / 2;
  g.add(nose);
  // Khăn quàng đỏ sọc trắng
  const stripes = canvasTex(256, 32, (c, w, h) => { c.fillStyle = '#d62828'; c.fillRect(0, 0, w, h); c.fillStyle = '#fff'; for (let x = 0; x < w; x += 32) c.fillRect(x, 0, 10, h); });
  const scarfM = mat(0xffffff, { map: stripes, roughness: 0.8 });
  const scarf = mesh(new THREE.TorusGeometry(0.15, 0.045, 14, 40), scarfM, 0, 0.79, 0);
  scarf.rotation.x = Math.PI / 2;
  g.add(scarf);
  const tail = mesh(new THREE.BoxGeometry(0.07, 0.2, 0.03), scarfM, 0.08, 0.68, 0.18);
  tail.rotation.z = 0.25;
  g.add(tail);
  // Mũ đen viền đỏ
  const hatM = mat(0x2b2d33, { roughness: 0.5 });
  const hat = new THREE.Group();
  hat.add(mesh(new THREE.CylinderGeometry(0.2, 0.2, 0.025, 32), hatM));
  hat.add(mesh(new THREE.CylinderGeometry(0.12, 0.13, 0.2, 32), hatM, 0, 0.1, 0));
  hat.add(mesh(new THREE.CylinderGeometry(0.132, 0.132, 0.04, 32), mat(0xd62828), 0, 0.03, 0));
  hat.position.set(0.01, 1.06, 0);
  hat.rotation.z = -0.15;
  g.add(hat);
  // Tay cành cây
  const twig = mat(0x6b4226, { roughness: 0.9 });
  for (const s of [-1, 1]) {
    const arm = mesh(new THREE.CylinderGeometry(0.012, 0.016, 0.24, 8), twig, s * 0.3, 0.74, 0.02);
    arm.rotation.z = s * -0.85;
    g.add(arm);
    const fork = mesh(new THREE.CylinderGeometry(0.007, 0.009, 0.07, 6), twig, s * 0.37, 0.83, 0.02);
    fork.rotation.z = s * -0.2;
    g.add(fork);
  }
  return { group: g, frame: [-0.6, 0.6, 1.35, -0.2], outline: '#56708f' };
}

// ---------- hộp quà ----------
function gift(box, ribbon, [w, h, d], seed) {
  const g = new THREE.Group();
  const bm = glossy(box, { roughness: 0.35, clearcoat: 0.6 });
  const rm = glossy(ribbon, { roughness: 0.25 });
  g.add(mesh(new THREE.BoxGeometry(w, h, d), bm, 0, h / 2, 0));
  g.add(mesh(new THREE.BoxGeometry(w * 1.04, h * 0.18, d * 1.04), bm, 0, h * 0.94, 0)); // nắp
  g.add(mesh(new THREE.BoxGeometry(w * 0.16, h * 1.02, d * 1.06), rm, 0, h / 2 + 0.002, 0));
  g.add(mesh(new THREE.BoxGeometry(w * 1.06, h * 1.02, d * 0.16), rm, 0, h / 2 + 0.002, 0));
  for (const s of [-1, 1]) {
    const loop = mesh(new THREE.TorusGeometry(w * 0.16, w * 0.045, 10, 24), rm, s * w * 0.14, h + w * 0.12, 0);
    loop.rotation.y = s * 0.5;
    loop.rotation.z = s * 0.5;
    g.add(loop);
  }
  g.add(sphere(w * 0.06, rm, 0, h + w * 0.05, 0));
  g.rotation.y = (seed % 2 ? 1 : -1) * 0.5;
  return { group: g, frame: [-0.45, 0.45, 0.6, -0.15], outline: '#4a2b2b' };
}

// ---------- vòng hoa ----------
function wreath() {
  const g = new THREE.Group();
  const r = rng(21);
  const greens = [mat(0x2f8a3a, { roughness: 0.8 }), mat(0x3fa548, { roughness: 0.8 }), mat(0x226f2c, { roughness: 0.8 })];
  for (let i = 0; i < 70; i++) {
    const a = (i / 70) * Math.PI * 2, rr = 0.2 + (r() - 0.5) * 0.05;
    const o = mesh(new THREE.IcosahedronGeometry(0.05 + r() * 0.02, 0), greens[i % 3], Math.cos(a) * rr, Math.sin(a) * rr, (r() - 0.5) * 0.04);
    o.rotation.set(r() * 3, r() * 3, r() * 3);
    g.add(o);
  }
  for (let i = 0; i < 9; i++) {
    const a = (i / 9) * Math.PI * 2 + 0.3;
    g.add(sphere(0.022, glossy(0xd62828), Math.cos(a) * 0.2, Math.sin(a) * 0.2, 0.05, 14));
  }
  const rm = glossy(0xd62828, { roughness: 0.3 });
  for (const s of [-1, 1]) {
    const loop = mesh(new THREE.TorusGeometry(0.06, 0.02, 10, 24), rm, s * 0.06, -0.2, 0.06);
    loop.scale.set(1, 0.7, 1);
    g.add(loop);
    const tail = mesh(new THREE.BoxGeometry(0.04, 0.13, 0.01), rm, s * 0.04, -0.29, 0.06);
    tail.rotation.z = s * 0.3;
    g.add(tail);
  }
  g.add(sphere(0.03, rm, 0, -0.2, 0.08));
  // Treo trên tường: xoay để mặt vòng hướng về máy quay.
  g.rotation.x = -TILT;
  g.position.y = 0.3;
  return { group: g, frame: [-0.35, 0.35, 0.6, -0.1], outline: '#1d4a22' };
}

// ---------- dây đèn nháy (ảnh lặp ngang, rộng đúng 1,28 = 128 px) ----------
function lights(lit) {
  const g = new THREE.Group();
  const W = 1.28;
  const wireY = (x) => 0.16 - 0.06 * Math.sin((Math.PI * x) / (W / 2)) ** 2; // võng 2 nhịp mỗi ô
  const pts = [];
  for (let i = 0; i <= 64; i++) { const x = -W / 2 - 0.02 + ((W + 0.04) * i) / 64; pts.push(new THREE.Vector3(x, wireY(x + W / 2), 0)); }
  g.add(new THREE.Mesh(new THREE.TubeGeometry(new THREE.CatmullRomCurve3(pts), 128, 0.008, 6), mat(0x1f4d2b)));
  const cols = [0xff4d4d, 0xffd23f, 0x4dd2ff, 0x7ddf4d];
  for (let i = 0; i < 4; i++) {
    const x = -W / 2 + W * (i + 0.5) / 4;
    const y = wireY(x + W / 2);
    const on = (i % 2 === 0 ? 'a' : 'b') === lit;
    g.add(mesh(new THREE.CylinderGeometry(0.016, 0.016, 0.03, 10), mat(0x2b5b3a), x, y - 0.018, 0));
    const b = mesh(new THREE.SphereGeometry(0.04, 18, 14), on ? glow(cols[i], 3) : mat(new THREE.Color(cols[i]).multiplyScalar(0.55).getHex(), { roughness: 0.3 }), x, y - 0.065, 0);
    b.scale.set(0.8, 1.25, 0.8);
    g.add(b);
  }
  return { group: g, frame: [-W / 2, W / 2, 0.24, -0.04], outline: null, noCrop: true };
}

// ---------- chấm "!" nhiệm vụ ----------
function quest() {
  const g = new THREE.Group();
  const m = new THREE.MeshStandardMaterial({ color: 0xffc928, emissive: 0xff9f00, emissiveIntensity: 0.35, roughness: 0.3, metalness: 0.2 });
  const bar = mesh(new THREE.CapsuleGeometry(0.045, 0.16, 8, 20), m, 0, 0.24, 0);
  bar.scale.set(1, 1, 0.8);
  g.add(bar);
  g.add(sphere(0.05, m, 0, 0.06, 0));
  g.rotation.x = -TILT;
  return { group: g, frame: [-0.15, 0.15, 0.42, -0.06], outline: '#7a4b00' };
}

// ---------- NPC: Ông già Noel / Bác nông dân ----------
// pose 0: đứng; 1: giơ tay vẫy; 2: vẫy nghiêng tay. Mọi khung cùng khung hình để thay ảnh không giật.
const NPC_FRAME = [-0.62, 0.62, 1.42, -0.16];
function person(kind, pose) {
  const santa = kind === 'santa';
  const g = new THREE.Group();
  const skin = mat(0xf2c09b, { roughness: 0.6 });
  const coat = santa ? mat(0xc8202a, { roughness: 0.6 }) : mat(0x3a5a8c, { roughness: 0.75 });
  const trim = mat(0xf7f7f2, { roughness: 0.9 });
  const pants = santa ? mat(0xc8202a, { roughness: 0.6 }) : mat(0x3b2f2a, { roughness: 0.8 });
  const boot = mat(0x1f1f24, { roughness: 0.4 });
  const bob = pose ? 0.012 : 0;
  // Chân + ủng
  for (const s of [-1, 1]) {
    g.add(mesh(new THREE.CylinderGeometry(0.065, 0.06, 0.22, 16), pants, s * 0.08, 0.17, 0));
    const b = mesh(new THREE.CapsuleGeometry(0.06, 0.06, 6, 14), santa ? boot : skin, s * 0.085, 0.05, 0.03);
    b.rotation.x = Math.PI / 2;
    g.add(b);
    if (!santa) g.add(mesh(new THREE.CylinderGeometry(0.068, 0.068, 0.04, 16), mat(0x2d241f), s * 0.08, 0.27, 0)); // ống quần xắn
  }
  // Thân áo (hơi phình)
  const body = new THREE.LatheGeometry([[0.001, 0.26], [0.2, 0.26], [0.235, 0.36], [0.235, 0.5], [0.2, 0.64], [0.12, 0.72], [0.001, 0.73]].map(([x, y]) => new THREE.Vector2(x, y)), 40);
  g.add(mesh(body, coat, 0, bob, 0));
  if (santa) {
    g.add(mesh(new THREE.TorusGeometry(0.205, 0.035, 12, 40), trim, 0, 0.27 + bob, 0).rotateX(Math.PI / 2));
    g.add(mesh(new THREE.BoxGeometry(0.06, 0.4, 0.03), trim, 0, 0.47 + bob, 0.225).rotateX(0.05));
    const belt = mesh(new THREE.CylinderGeometry(0.238, 0.238, 0.06, 40), boot, 0, 0.42 + bob, 0);
    g.add(belt);
    g.add(mesh(new THREE.BoxGeometry(0.09, 0.075, 0.02), metal(0xf2c14e), 0, 0.42 + bob, 0.24));
  } else {
    // Áo bà ba: hàng nút + túi
    for (const y of [0.4, 0.5, 0.6]) g.add(sphere(0.012, mat(0xe8e2d0), 0, y + bob, 0.232 - (y - 0.5) ** 2));
    g.add(mesh(new THREE.BoxGeometry(0.1, 0.07, 0.01), mat(0x31507e), -0.1, 0.36 + bob, 0.218));
    g.add(mesh(new THREE.TorusGeometry(0.11, 0.025, 10, 30), mat(0xd9c39a), 0, 0.71 + bob, 0).rotateX(Math.PI / 2)); // khăn rằn quấn cổ
  }
  // Đầu
  const hy = 0.86 + bob;
  g.add(sphere(0.16, skin, 0, hy, 0));
  const eye = mat(0x1d1d22, { roughness: 0.3 });
  for (const s of [-1, 1]) {
    g.add(sphere(0.018, eye, s * 0.055, hy + 0.02, 0.145, 14));
    g.add(sphere(0.006, trim, s * 0.055 + 0.006, hy + 0.028, 0.16, 8));
    g.add(sphere(0.03, mat(0xf29a8a, { roughness: 0.8 }), s * 0.095, hy - 0.025, 0.12, 14)); // má hồng
  }
  g.add(sphere(0.03, mat(0xe9a07f), 0, hy - 0.005, 0.16, 16)); // mũi
  if (santa) {
    // Râu trắng bồng + ria
    const beard = snowLump(0.15, 0.15, 0.1, 9);
    beard.material = trim;
    beard.position.set(0, hy - 0.1, 0.07);
    g.add(beard);
    for (const s of [-1, 1]) {
      const m = mesh(new THREE.CapsuleGeometry(0.025, 0.06, 6, 12), trim, s * 0.045, hy - 0.04, 0.15);
      m.rotation.z = s * 1.2;
      g.add(m);
    }
    // Mũ đỏ chóp cong + viền lông + quả bông
    const hat = new THREE.Group();
    hat.add(mesh(new THREE.TorusGeometry(0.15, 0.04, 14, 40), trim).rotateX(Math.PI / 2));
    const cone = mesh(new THREE.ConeGeometry(0.15, 0.3, 32), coat, 0, 0.14, 0);
    hat.add(cone);
    cone.rotation.z = -0.35;
    cone.position.x = 0.05;
    hat.add(sphere(0.045, trim, 0.13, 0.25, 0));
    hat.position.set(0, hy + 0.1, 0);
    hat.rotation.x = -0.15;
    g.add(hat);
  } else {
    // Ria đen + nón lá
    for (const s of [-1, 1]) {
      const m = mesh(new THREE.CapsuleGeometry(0.012, 0.04, 6, 10), eye, s * 0.03, hy - 0.05, 0.152);
      m.rotation.z = s * 1.3;
      g.add(m);
    }
    const strawTex = canvasTex(512, 64, (c, w, h) => {
      c.fillStyle = '#e9cf8a'; c.fillRect(0, 0, w, h);
      for (let x = 0; x < w; x += 16) { c.fillStyle = 'rgba(150,110,40,.35)'; c.fillRect(x, 0, 2, h); }
      for (let y = 8; y < h; y += 14) { c.fillStyle = 'rgba(255,245,210,.5)'; c.fillRect(0, y, w, 2); }
    });
    const non = mesh(new THREE.ConeGeometry(0.3, 0.17, 48, 1, true), mat(0xffffff, { map: strawTex, side: THREE.DoubleSide, roughness: 0.85 }), 0, hy + 0.2, -0.03);
    non.rotation.x = -0.2; // nón hất ra sau để thấy mặt
    g.add(non);
  }
  // Tay: trái buông (giữ bao / giỏ), phải buông hoặc vẫy.
  const arm = (side, raise, swing) => {
    const a = new THREE.Group();
    a.position.set(side * 0.22, 0.64 + bob, 0);
    const up = mesh(new THREE.CapsuleGeometry(0.05, 0.2, 6, 14), coat, 0, -0.13, 0);
    a.add(up);
    if (santa) a.add(mesh(new THREE.TorusGeometry(0.05, 0.022, 10, 24), trim, 0, -0.26, 0).rotateX(Math.PI / 2));
    a.add(sphere(0.055, santa ? mat(0x1f1f24) : skin, 0, -0.3, 0, 18));
    a.rotation.z = side * (0.18 + raise);
    a.rotation.x = swing;
    return a;
  };
  g.add(arm(-1, 0, 0.1));
  const wave = pose === 1 ? [2.5, -0.1] : pose === 2 ? [2.85, 0.15] : [0.05, -0.05];
  const right = arm(1, wave[0], wave[1]);
  g.add(right);
  // Bao quà (Noel) / giỏ rau (nông dân) bên trái
  if (santa) {
    const sack = snowLump(0.17, 0.2, 0.15, 13);
    sack.material = mat(0x8a5a2b, { roughness: 0.9 });
    sack.position.set(-0.36, 0.2, -0.05);
    g.add(sack);
    g.add(mesh(new THREE.CylinderGeometry(0.05, 0.08, 0.08, 16), sack.material, -0.36, 0.42, -0.05));
    g.add(mesh(new THREE.TorusGeometry(0.05, 0.012, 8, 20), mat(0xd62828), -0.36, 0.4, -0.05).rotateX(Math.PI / 2));
    const peek = gift(0x2a9d4b, 0xf2c14e, [0.12, 0.1, 0.12], 1).group;
    peek.position.set(-0.4, 0.43, -0.02);
    peek.rotation.z = 0.3;
    g.add(peek);
    const peek2 = gift(0x2a6fdb, 0xffffff, [0.1, 0.09, 0.1], 2).group;
    peek2.position.set(-0.3, 0.44, -0.08);
    peek2.rotation.z = -0.25;
    g.add(peek2);
  } else {
    const basket = new THREE.Group();
    const weave = canvasTex(256, 64, (c, w, h) => {
      c.fillStyle = '#b5833f'; c.fillRect(0, 0, w, h);
      for (let y = 0; y < h; y += 10) for (let x = (y / 10) % 2 ? 0 : 12; x < w; x += 24) { c.fillStyle = '#8a5f28'; c.fillRect(x, y, 12, 8); }
    });
    basket.add(mesh(new THREE.CylinderGeometry(0.15, 0.11, 0.17, 28, 1, true), mat(0xffffff, { map: weave, side: THREE.DoubleSide, roughness: 0.9 }), 0, 0.085, 0));
    basket.add(mesh(new THREE.CircleGeometry(0.11, 28), mat(0x8a5f28), 0, 0.005, 0).rotateX(-Math.PI / 2));
    basket.add(mesh(new THREE.TorusGeometry(0.15, 0.015, 8, 30), mat(0x8a5f28), 0, 0.17, 0).rotateX(Math.PI / 2));
    // Rau củ: bắp cải, cà rốt, cà chua
    basket.add(sphere(0.07, mat(0x7cc35a, { roughness: 0.7 }), -0.04, 0.19, 0));
    basket.add(sphere(0.05, glossy(0xe63b2e), 0.06, 0.19, 0.04));
    const carrot = mesh(new THREE.ConeGeometry(0.03, 0.16, 12), mat(0xf08a24), 0.05, 0.24, -0.04);
    carrot.rotation.z = 2.7;
    basket.add(carrot);
    basket.position.set(-0.36, 0, 0.02);
    g.add(basket);
  }
  return { group: g, frame: NPC_FRAME, outline: santa ? '#5a1218' : '#2a2a3a' };
}

const ITEMS = {
  tree_a: () => xmasTree('a'),
  tree_b: () => xmasTree('b'),
  snowman,
  gift_red: () => gift(0xd62828, 0xf2c14e, [0.3, 0.24, 0.3], 1),
  gift_green: () => gift(0x2a9d4b, 0xd62828, [0.24, 0.2, 0.24], 2),
  gift_blue: () => gift(0x2a6fdb, 0xffffff, [0.2, 0.28, 0.2], 3),
  wreath,
  lights_a: () => lights('a'),
  lights_b: () => lights('b'),
  quest_mark: quest,
  npc_santa_0: () => person('santa', 0),
  npc_santa_1: () => person('santa', 1),
  npc_santa_2: () => person('santa', 2),
  npc_farmer_0: () => person('farmer', 0),
  npc_farmer_1: () => person('farmer', 1),
  npc_farmer_2: () => person('farmer', 2),
};

function setLights(scene) {
  scene.environment = envTex;
  scene.add(new THREE.HemisphereLight(0xffffff, 0x8aa6c0, 0.9));
  const key = new THREE.DirectionalLight(0xfff4e0, 2.3);
  key.position.set(-2.5, 4, 3);
  scene.add(key);
  const rim = new THREE.DirectionalLight(0xcfe8ff, 1.4);
  rim.position.set(3, 2, -2.5);
  scene.add(rim);
  const fill = new THREE.DirectionalLight(0xffffff, 0.5);
  fill.position.set(2, 0.5, 3);
  scene.add(fill);
}

window.ITEM_IDS = Object.keys(ITEMS);
window.renderItem = (id) => {
  const scene = new THREE.Scene();
  setLights(scene);
  const { group, frame, outline, noCrop } = ITEMS[id]();
  scene.add(group);
  const [l, r, t, b] = frame;
  const cam = new THREE.OrthographicCamera(l, r, t, b, 0.1, 50);
  if (id.startsWith('lights')) {
    cam.position.set(0, 0, 10); // dây đèn nhìn thẳng để lặp ngang khít
  } else {
    cam.position.set(0, Math.sin(TILT) * 10, Math.cos(TILT) * 10);
    cam.up.set(0, Math.cos(TILT), -Math.sin(TILT));
  }
  cam.lookAt(0, 0, 0);
  const W = Math.round((r - l) * PX), H = Math.round((t - b) * PX);
  renderer.setSize(W, H);
  renderer.render(scene, cam);
  // Gốc (0,0,0) — chỗ chân chạm đất — trên ảnh thô.
  return { png: renderer.domElement.toDataURL('image/png'), outline, noCrop: !!noCrop, origin: [Math.round(-l * PX), Math.round(t * PX)], size: [W, H] };
};
window.noelReady = true;
