// Dựng ảnh cho bảng nhiệm vụ: rương ngày / tuần (đóng, mở), mảnh ghép 4 bậc, ảnh nhỏ các xe đua và đồ độc quyền.
// Ảnh tự dựng cho DCity (CC0). Rương + mảnh: three.js riêng (nền trong suốt). Xe: dùng chính mô hình trong game
// (public/race/scene.js) rồi tách nền bằng hai lần vẽ trên nền đen / trắng (renderer của game không có kênh alpha).
// window.renderItem(id) → {png (data URL), outline}; window.ITEM_IDS; window.questsReady.
import * as THREE from 'three';
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js';
import { createRenderer, buildCar } from '/race/scene.js';

const TILT = (24 * Math.PI) / 180;
const PX = 400; // px ảnh thô / đơn vị (finish.py thu về 2×)

const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
renderer.setPixelRatio(1);
renderer.setClearColor(0x000000, 0);
renderer.outputColorSpace = THREE.SRGBColorSpace;
renderer.toneMapping = THREE.NeutralToneMapping;
renderer.toneMappingExposure = 1.05;
document.body.appendChild(renderer.domElement);
const pmrem = new THREE.PMREMGenerator(renderer);
const envTex = pmrem.fromScene(new RoomEnvironment(), 0.04).texture;

const mat = (color, o = {}) => new THREE.MeshStandardMaterial({ color, roughness: 0.55, metalness: 0, envMapIntensity: 0.6, ...o });
const glossy = (color, o = {}) => new THREE.MeshPhysicalMaterial({ color, roughness: 0.2, metalness: 0.05, clearcoat: 1, clearcoatRoughness: 0.08, envMapIntensity: 1.1, ...o });
const metal = (color, o = {}) => new THREE.MeshStandardMaterial({ color, roughness: 0.28, metalness: 0.9, envMapIntensity: 1.3, ...o });
const glow = (color, k = 1.4) => new THREE.MeshStandardMaterial({ color, emissive: color, emissiveIntensity: k, roughness: 0.3 });
function mesh(geo, m, x = 0, y = 0, z = 0) {
  const o = new THREE.Mesh(geo, m);
  o.position.set(x, y, z);
  return o;
}
function canvasTex(w, h, draw) {
  const c = document.createElement('canvas');
  c.width = w; c.height = h;
  draw(c.getContext('2d'), w, h);
  const t = new THREE.CanvasTexture(c);
  t.colorSpace = THREE.SRGBColorSpace;
  t.anisotropy = 8;
  return t;
}
function rng(seed) {
  let s = seed >>> 0;
  return () => ((s = (s * 1664525 + 1013904223) >>> 0) / 4294967296);
}
// Hộp bo góc (đùn từ hình chữ nhật bo góc, vát mép).
function roundBox(w, h, d, r, bevel = 0.03) {
  const s = new THREE.Shape();
  const x = -w / 2 + bevel, y = -h / 2 + bevel, W = w - 2 * bevel, H = h - 2 * bevel, rr = Math.min(r, W / 2, H / 2);
  s.moveTo(x + rr, y);
  s.lineTo(x + W - rr, y); s.quadraticCurveTo(x + W, y, x + W, y + rr);
  s.lineTo(x + W, y + H - rr); s.quadraticCurveTo(x + W, y + H, x + W - rr, y + H);
  s.lineTo(x + rr, y + H); s.quadraticCurveTo(x, y + H, x, y + H - rr);
  s.lineTo(x, y + rr); s.quadraticCurveTo(x, y, x + rr, y);
  const g = new THREE.ExtrudeGeometry(s, { depth: d - 2 * bevel, bevelEnabled: true, bevelThickness: bevel, bevelSize: bevel, bevelSegments: 4, curveSegments: 10 });
  g.translate(0, 0, -(d - 2 * bevel) / 2);
  return g;
}

// ---------- Rương ----------
// Vân gỗ cho rương ngày.
const woodTex = canvasTex(256, 256, (c, w, h) => {
  c.fillStyle = '#a86a3a'; c.fillRect(0, 0, w, h);
  const r = rng(3);
  for (let i = 0; i < 40; i++) {
    const y = r() * h;
    c.strokeStyle = r() < 0.5 ? 'rgba(90,50,25,.35)' : 'rgba(200,140,90,.25)';
    c.lineWidth = 1 + r() * 3;
    c.beginPath(); c.moveTo(0, y);
    for (let x = 0; x <= w; x += 16) c.lineTo(x, y + Math.sin(x * 0.05 + i) * 3);
    c.stroke();
  }
  // khe ván
  c.fillStyle = 'rgba(60,30,12,.55)';
  for (let k = 1; k < 4; k++) c.fillRect(0, (k * h) / 4 - 1, w, 3);
});

// kind: 'daily' (gỗ, viền vàng) | 'weekly' (xanh hoàng gia, viền vàng, ngọc); open: nắp mở, ánh sáng + mảnh bay ra.
function chest(kind, open) {
  const g = new THREE.Group();
  const W = 1.1, D = 0.72, H = 0.52;
  const weekly = kind === 'weekly';
  const bodyMat = weekly ? glossy(0x2a4fd6, { roughness: 0.3 }) : mat(0xffffff, { map: woodTex, roughness: 0.7 });
  const trim = metal(0xf2c14e);
  const dark = mat(weekly ? 0x16245e : 0x4a2a14, { roughness: 0.8 });
  bodyMat.side = THREE.DoubleSide; // lòng nắp thấy được khi mở
  // Thân
  g.add(mesh(roundBox(W, H, D, 0.06), bodyMat, 0, H / 2, 0));
  // Lòng rương (thấy khi mở)
  if (open) {
    const inner = mesh(new THREE.BoxGeometry(W - 0.12, 0.02, D - 0.12), glow(0xffe08a, 1.2), 0, H - 0.04, 0);
    g.add(inner);
    // Mảnh nhô lên khỏi miệng rương
    const cols = weekly ? [0xffb423, 0xa35cff, 0x3d8bff] : [0x58c06a, 0x3d8bff, 0xffb423];
    cols.forEach((c, i) => {
      const p = piece(c, 0.26);
      p.position.set(-0.28 + i * 0.28, H + 0.12 + (i === 1 ? 0.12 : 0), 0.02);
      p.rotation.set(-0.2, (i - 1) * 0.5, (i - 1) * -0.35);
      g.add(p);
    });
  }
  // Nắp: nửa trụ nằm ngang, bản lề mép sau
  const lid = new THREE.Group();
  // vòm nắp: nửa trụ nằm ngang theo trục x, vòm hướng lên
  const dome = new THREE.Mesh(new THREE.CylinderGeometry(D / 2, D / 2, W, 40, 1, false, 0, Math.PI), bodyMat);
  dome.rotation.z = Math.PI / 2;
  dome.position.set(0, 0, D / 2);
  lid.add(dome);
  // nắp hai đầu vòm
  for (const sx of [-1, 1]) {
    const capG = new THREE.CircleGeometry(D / 2, 40, 0, Math.PI);
    const cap = new THREE.Mesh(capG, weekly ? bodyMat : dark);
    cap.rotation.y = sx * Math.PI / 2;
    cap.position.set(sx * W / 2, 0, D / 2);
    lid.add(cap);
  }
  // đai kim loại trên nắp
  for (const x of [-W * 0.36, W * 0.36]) {
    const band = new THREE.Mesh(new THREE.CylinderGeometry(D / 2 + 0.018, D / 2 + 0.018, 0.09, 40, 1, false, 0, Math.PI), trim);
    band.rotation.z = Math.PI / 2;
    band.position.set(x, 0, D / 2);
    lid.add(band);
  }
  // viền đáy nắp
  if (!open) lid.add(mesh(new THREE.BoxGeometry(W + 0.03, 0.05, D + 0.03), trim, 0, 0.0, D / 2));
  else for (const z of [0.02, D - 0.02]) lid.add(mesh(new THREE.BoxGeometry(W + 0.03, 0.05, 0.05), trim, 0, 0, z)); // chỉ còn viền mép
  lid.position.set(0, H, -D / 2);
  lid.rotation.x = open ? -1.95 : 0;
  g.add(lid);
  // Đai dọc thân + viền đáy
  for (const x of [-W * 0.36, W * 0.36]) g.add(mesh(new THREE.BoxGeometry(0.09, H + 0.01, D + 0.03), trim, x, H / 2, 0));
  g.add(mesh(new THREE.BoxGeometry(W + 0.03, 0.06, D + 0.03), trim, 0, 0.03, 0));
  g.add(mesh(new THREE.BoxGeometry(W + 0.03, 0.05, D + 0.03), trim, 0, H - 0.02, 0));
  // Ổ khoá
  const lock = mesh(roundBox(0.2, 0.24, 0.06, 0.04, 0.012), trim, 0, H - 0.06, D / 2 + 0.03);
  g.add(lock);
  g.add(mesh(new THREE.CylinderGeometry(0.025, 0.025, 0.02, 16), dark, 0, H - 0.08, D / 2 + 0.065).rotateX(Math.PI / 2));
  if (weekly) {
    // Ngọc trên ổ khoá và hai đầu
    const gem = new THREE.Mesh(new THREE.OctahedronGeometry(0.06), glossy(0xff3f8a, { emissive: 0xff2f7a, emissiveIntensity: 0.4 }));
    gem.position.set(0, H - 0.02, D / 2 + 0.07);
    g.add(gem);
    for (const sx of [-1, 1]) {
      const gm = new THREE.Mesh(new THREE.OctahedronGeometry(0.045), glossy(0x2fe6ff, { emissive: 0x2fe6ff, emissiveIntensity: 0.5 }));
      gm.position.set(sx * W * 0.36, H * 0.5, D / 2 + 0.03);
      g.add(gm);
    }
  }
  g.rotation.y = -0.55; // nhìn chéo 3/4 cho thấy hai đầu vòm nắp
  return { group: g, frame: [-0.95, 0.95, 1.75, -0.3], outline: weekly ? '#101c4a' : '#3a200c' };
}

// ---------- Mảnh ghép ----------
// Hình mảnh ghép: ô vuông, núm lồi phải + trên, hõm trái.
function pieceShape(s) {
  const sh = new THREE.Shape();
  const h = s / 2, k = s * 0.17; // bán kính núm
  sh.moveTo(-h, -h);
  sh.lineTo(h, -h);
  // phải: núm lồi
  sh.lineTo(h, -k * 0.9);
  sh.absarc(h + k * 0.75, 0, k, Math.PI * 1.25, Math.PI * 0.75, false);
  sh.lineTo(h, h);
  // trên: núm lồi
  sh.lineTo(k * 0.9, h);
  sh.absarc(0, h + k * 0.75, k, -Math.PI * 0.25, Math.PI * 1.25, false);
  sh.lineTo(-h, h);
  // trái: hõm
  sh.lineTo(-h, k * 0.9);
  sh.absarc(-h + k * 0.75, 0, k, Math.PI * 0.75, Math.PI * 1.25, true);
  sh.lineTo(-h, -h);
  return sh;
}
function piece(color, s = 0.5) {
  const g = new THREE.ExtrudeGeometry(pieceShape(s), { depth: s * 0.18, bevelEnabled: true, bevelThickness: s * 0.06, bevelSize: s * 0.05, bevelSegments: 5, curveSegments: 24 });
  g.center();
  return new THREE.Mesh(g, glossy(color, { emissive: color, emissiveIntensity: 0.12 }));
}
const TIERS = { common: 0x58c06a, rare: 0x3d8bff, epic: 0xa35cff, exclusive: 0xffb423 };
function shard(tier) {
  const g = new THREE.Group();
  const p = piece(TIERS[tier], 0.62);
  p.rotation.set(-0.35, 0.35, 0.12);
  p.position.y = 0.45;
  g.add(p);
  // Dấu xe đua dập nổi trên mặt mảnh: bánh lái tròn
  const ring = new THREE.Mesh(new THREE.TorusGeometry(0.11, 0.022, 12, 40), metal(0xffffff, { roughness: 0.2 }));
  ring.position.set(0, 0, 0.09);
  p.add(ring);
  for (let i = 0; i < 3; i++) {
    const sp = new THREE.Mesh(new THREE.BoxGeometry(0.02, 0.1, 0.02), metal(0xffffff));
    sp.rotation.z = (i * Math.PI * 2) / 3;
    sp.position.set(Math.sin(-sp.rotation.z) * 0.05, Math.cos(sp.rotation.z) * 0.05, 0.09);
    p.add(sp);
  }
  if (tier === 'exclusive' || tier === 'epic') {
    // Lấp lánh: vài ngôi sao nhỏ phát sáng
    const r = rng(tier.length);
    for (let i = 0; i < 4; i++) {
      const st = new THREE.Mesh(new THREE.OctahedronGeometry(0.035 + r() * 0.02), glow(0xffffff, 2));
      st.scale.set(1, 2.2, 0.4);
      st.position.set(-0.4 + r() * 0.8, 0.15 + r() * 0.65, 0.3);
      g.add(st);
    }
  }
  return { group: g, frame: [-0.6, 0.6, 1.05, -0.15], outline: '#1a1430' };
}

// ---------- Phụ kiện nâng cấp xe (Piston / Bộ tăng áp / Lốp đua) ----------
// Piston: đầu piston bạc có 3 rãnh xéc-măng, chốt và tay biên thép, chân biên tròn.
function piston() {
  const g = new THREE.Group();
  const steel = metal(0xd9dee6, { roughness: 0.22 });
  const dark = metal(0x5a6170, { roughness: 0.35 });
  const head = mesh(new THREE.CylinderGeometry(0.34, 0.34, 0.46, 40), steel, 0, 0.62, 0);
  g.add(head);
  g.add(mesh(new THREE.CylinderGeometry(0.33, 0.34, 0.04, 40), metal(0xf2f4f8, { roughness: 0.12 }), 0, 0.87, 0)); // mặt đỉnh
  for (const y of [0.74, 0.68, 0.62]) g.add(mesh(new THREE.TorusGeometry(0.343, 0.012, 8, 48), dark, 0, y, 0));
  g.children.slice(-3).forEach((r) => (r.rotation.x = Math.PI / 2));
  const pin = mesh(new THREE.CylinderGeometry(0.07, 0.07, 0.74, 20), metal(0xb8c0cc), 0, 0.5, 0);
  pin.rotation.z = Math.PI / 2;
  g.add(pin);
  const rod = mesh(new THREE.BoxGeometry(0.13, 0.62, 0.09), dark, 0, 0.16, 0);
  g.add(rod);
  const end = mesh(new THREE.TorusGeometry(0.16, 0.06, 14, 36), dark, 0, -0.18, 0);
  g.add(end);
  g.add(mesh(new THREE.CylinderGeometry(0.1, 0.1, 0.1, 20), metal(0xffb423, { roughness: 0.25 }), 0, -0.18, 0));
  g.children[g.children.length - 1].rotation.x = Math.PI / 2;
  g.rotation.set(0.15, 0.6, -0.32);
  return { group: g, frame: [-0.62, 0.62, 1.05, -0.5], outline: '#1a1430' };
}
// Bộ tăng áp (turbo): vỏ máy nén hình ốc xoắn (volute) nhôm bạc ánh xanh nhìn chính diện, cửa hút tròn thấy rõ cánh quạt
// xoắn và đai ốc trục, ống ra tiếp tuyến có bích vuông 4 bu-lông; phía sau là vỏ tua-bin gang sẫm ánh nhiệt tím xanh, ống xả
// bích tròn; giữa hai vỏ là thân ổ trục có đai vàng.
function volute(R0, R1, t0, t1, material, turns = 1.8, segs = 40) {
  const grp = new THREE.Group();
  for (let i = 0; i < segs; i++) {
    const k = i / segs;
    const step = (turns * Math.PI * 2) / segs;
    const tor = new THREE.Mesh(new THREE.TorusGeometry(R0 + (R1 - R0) * k, t0 + (t1 - t0) * k, 18, 4, step * 1.08), material);
    tor.rotation.z = k * turns * Math.PI * 2;
    grp.add(tor);
  }
  return grp;
}
function turboKit() {
  const g = new THREE.Group();
  const alu = metal(0xc9d6e6, { roughness: 0.18, envMapIntensity: 1.6 });
  const aluDark = metal(0x8fa3bd, { roughness: 0.3 });
  const iron = new THREE.MeshPhysicalMaterial({ color: 0x4a4f5c, metalness: 0.9, roughness: 0.35, iridescence: 0.8, iridescenceIOR: 1.6, envMapIntensity: 1.3 });
  const goldM = metal(0xe0a92a, { roughness: 0.22 });
  // Vỏ máy nén (trước, mặt hướng về người xem: trục theo z).
  const comp = new THREE.Group();
  comp.add(volute(0.26, 0.34, 0.1, 0.17, alu));
  const disc = mesh(new THREE.CylinderGeometry(0.33, 0.33, 0.24, 48), alu, 0, 0, 0);
  disc.rotation.x = Math.PI / 2; comp.add(disc);
  // Gờ đúc quanh mặt trước.
  const lip = mesh(new THREE.TorusGeometry(0.3, 0.03, 12, 48), aluDark, 0, 0, 0.12); comp.add(lip);
  // Cửa hút: ống ngắn loe miệng.
  const inlet = mesh(new THREE.CylinderGeometry(0.2, 0.2, 0.2, 40, 1, true), alu, 0, 0, 0.2);
  inlet.rotation.x = Math.PI / 2; inlet.material = alu.clone(); inlet.material.side = THREE.DoubleSide; comp.add(inlet);
  comp.add(mesh(new THREE.TorusGeometry(0.205, 0.03, 12, 48), alu, 0, 0, 0.3));
  // Lòng cửa hút tối để cánh quạt nổi lên.
  const back = mesh(new THREE.CircleGeometry(0.2, 40), mat(0x1c2230, { roughness: 0.8 }), 0, 0, 0.12); comp.add(back);
  // Cánh quạt nén xoắn (9 cánh) + đai ốc trục.
  const blade = metal(0xe8eef6, { roughness: 0.15, envMapIntensity: 2 });
  for (let i = 0; i < 9; i++) {
    const a = (i / 9) * Math.PI * 2;
    const b = mesh(new THREE.BoxGeometry(0.018, 0.16, 0.12), blade, 0, 0, 0.17);
    b.position.set(Math.cos(a) * 0.1, Math.sin(a) * 0.1, 0.17);
    b.rotation.set(0, 0, a - Math.PI / 2); b.rotateY(0.55);
    comp.add(b);
  }
  comp.add(mesh(new THREE.ConeGeometry(0.055, 0.12, 20), metal(0xffc13b, { roughness: 0.2 }), 0, 0, 0.24));
  comp.children[comp.children.length - 1].rotation.x = Math.PI / 2;
  // Ống ra tiếp tuyến lên trên bên phải + bích vuông 4 bu-lông.
  const outPipe = mesh(new THREE.CylinderGeometry(0.11, 0.13, 0.34, 32), alu, 0.4, 0.2, 0);
  outPipe.rotation.z = -0.35; comp.add(outPipe);
  const flange = mesh(new THREE.BoxGeometry(0.32, 0.05, 0.32), aluDark, 0.46, 0.37, 0);
  flange.rotation.z = -0.35; comp.add(flange);
  for (const [dx, dz] of [[-0.11, -0.11], [0.11, -0.11], [-0.11, 0.11], [0.11, 0.11]]) {
    const bolt = mesh(new THREE.CylinderGeometry(0.025, 0.025, 0.06, 6), metal(0x3a3f4a), 0.46 + dx * Math.cos(0.35), 0.37 + dx * Math.sin(0.35) + 0.01, dz);
    bolt.rotation.z = -0.35; comp.add(bolt);
  }
  comp.position.z = 0.18;
  g.add(comp);
  // Thân ổ trục + đai vàng.
  const core = mesh(new THREE.CylinderGeometry(0.17, 0.17, 0.22, 32), aluDark, 0, 0, -0.04);
  core.rotation.x = Math.PI / 2; g.add(core);
  const band = mesh(new THREE.TorusGeometry(0.175, 0.03, 12, 40), goldM, 0, 0, -0.04); g.add(band);
  // Vỏ tua-bin (sau): gang sẫm ánh nhiệt, nhỏ hơn, xoắn ngược.
  const turb = new THREE.Group();
  const tv = volute(0.22, 0.3, 0.09, 0.15, iron); tv.rotation.y = Math.PI; turb.add(tv);
  const td = mesh(new THREE.CylinderGeometry(0.29, 0.29, 0.2, 40), iron, 0, 0, 0); td.rotation.x = Math.PI / 2; turb.add(td);
  const ex = mesh(new THREE.CylinderGeometry(0.13, 0.13, 0.28, 28), iron, -0.36, -0.22, 0); ex.rotation.z = Math.PI / 2 - 0.4; turb.add(ex);
  const exF = mesh(new THREE.TorusGeometry(0.14, 0.035, 10, 32), metal(0x6b7180), -0.48, -0.28, 0); exF.rotation.y = Math.PI / 2; exF.rotation.x = 0.4; turb.add(exF);
  turb.position.z = -0.3;
  g.add(turb);
  g.rotation.set(0.28, -0.62, 0.05);
  g.position.y = 0.3;
  return { group: g, frame: [-0.72, 0.78, 1.0, -0.42], outline: '#1a1430' };
}
// Lốp đua: lốp đen gai sọc, vành đúc vàng 6 chấu, chữ sọc đỏ trên hông lốp.
function tireKit() {
  const g = new THREE.Group();
  const rubber = mat(0x22252c, { roughness: 0.85 });
  const tire = mesh(new THREE.TorusGeometry(0.42, 0.17, 24, 64), rubber);
  tire.scale.z = 1.35;
  g.add(tire);
  // Gai lốp.
  for (let i = 0; i < 36; i++) {
    const a = (i / 36) * Math.PI * 2;
    const k = mesh(new THREE.BoxGeometry(0.05, 0.035, 0.3), mat(0x15171c), Math.cos(a) * 0.585, Math.sin(a) * 0.585, 0);
    k.rotation.z = a;
    g.add(k);
  }
  const stripe = mesh(new THREE.TorusGeometry(0.43, 0.012, 8, 64), glow(0xff3b3b, 0.5), 0, 0, 0.215);
  g.add(stripe);
  const rimMat = metal(0xffc13b, { roughness: 0.2 });
  const rim = mesh(new THREE.CylinderGeometry(0.3, 0.3, 0.16, 40, 1, true), rimMat);
  rim.rotation.x = Math.PI / 2;
  g.add(rim);
  g.add(mesh(new THREE.TorusGeometry(0.3, 0.022, 10, 48), rimMat, 0, 0, 0.08));
  for (let i = 0; i < 6; i++) {
    const sp = mesh(new THREE.BoxGeometry(0.06, 0.28, 0.04), rimMat, 0, 0, 0.06);
    sp.rotation.z = (i / 6) * Math.PI * 2;
    sp.position.set(Math.sin(-sp.rotation.z) * 0.15, Math.cos(sp.rotation.z) * 0.15, 0.06);
    g.add(sp);
  }
  g.add(mesh(new THREE.CylinderGeometry(0.07, 0.07, 0.08, 20), metal(0x3a3f4a), 0, 0, 0.07));
  g.children[g.children.length - 1].rotation.x = Math.PI / 2;
  g.rotation.set(-0.15, 0.55, 0);
  g.position.y = 0.3;
  return { group: g, frame: [-0.72, 0.72, 1.0, -0.42], outline: '#1a1430' };
}

function setLights(scene) {
  scene.environment = envTex;
  scene.add(new THREE.HemisphereLight(0xffffff, 0x8aa6c0, 0.9));
  const key = new THREE.DirectionalLight(0xfff4e0, 2.4);
  key.position.set(-2.5, 4, 3);
  scene.add(key);
  const rim = new THREE.DirectionalLight(0xcfe8ff, 1.4);
  rim.position.set(3, 2, -2.5);
  scene.add(rim);
  const fill = new THREE.DirectionalLight(0xffffff, 0.5);
  fill.position.set(2, 0.5, 3);
  scene.add(fill);
}

const ITEMS = {
  chest_daily: () => chest('daily', false),
  chest_daily_open: () => chest('daily', true),
  chest_weekly: () => chest('weekly', false),
  chest_weekly_open: () => chest('weekly', true),
  shard_common: () => shard('common'),
  shard_rare: () => shard('rare'),
  shard_epic: () => shard('epic'),
  shard_exclusive: () => shard('exclusive'),
  part_piston: piston,
  part_turbo_kit: turboKit,
  part_tire_kit: tireKit,
};

function renderSimple(id) {
  const scene = new THREE.Scene();
  setLights(scene);
  const { group, frame, outline } = ITEMS[id]();
  scene.add(group);
  const [l, r, t, b] = frame;
  const cam = new THREE.OrthographicCamera(l, r, t, b, 0.1, 50);
  cam.position.set(0, Math.sin(TILT) * 10, Math.cos(TILT) * 10);
  cam.up.set(0, Math.cos(TILT), -Math.sin(TILT));
  cam.lookAt(0, 0, 0);
  renderer.setSize(Math.round((r - l) * PX), Math.round((t - b) * PX));
  renderer.render(scene, cam);
  return { png: renderer.domElement.toDataURL('image/png'), outline };
}

// ---------- Ảnh nhỏ xe ----------
const CARS = ['starter', 'comet', 'moto_dash', 'sprint', 'bolt', 'raptor', 'viper', 'moto_storm', 'zenith', 'phantom',
  'moto_neon', 'vortex', 'apex', 'blaze', 'nova', 'x_aurora', 'x_frost'];
// Đồ độc quyền: chụp trên một xe mẫu đang lắp món đó.
const ITEM_CARS = {
  paint_x_chroma: { car: 'phantom', cosmetics: { paint: 'paint_x_chroma' } },
  decal_x_snow: { car: 'sprint', cosmetics: { paint: 'paint_blue', decal: 'decal_x_snow' } },
  neon_x_rainbow: { car: 'nova', cosmetics: { paint: 'paint_black', neon: 'neon_x_rainbow' } },
  spoiler_x_glow: { car: 'sprint', cosmetics: { paint: 'paint_white', spoiler: 'spoiler_x_glow' } },
};
const TW = 640, TH = 400; // ảnh thô xe (finish.py thu về 320×200)
const carCanvas = document.createElement('canvas');
carCanvas.width = TW; carCanvas.height = TH;
carCanvas.style.cssText = `width:${TW}px;height:${TH}px`;
document.body.appendChild(carCanvas);
const view = createRenderer(carCanvas, false);
view.renderer.setPixelRatio(1);
view.renderer.setSize(TW, TH, false);
view.renderer.preserveDrawingBuffer = true;

async function renderCar(id) {
  const spec = ITEM_CARS[id] ? { ...ITEM_CARS[id], levels: { engine: 5, turbo: 5, tires: 5 } } : { car: id, levels: { engine: 3, turbo: 3, tires: 3 }, cosmetics: {} };
  const obj = await buildCar({ ...spec, number: 7 });
  // chờ texture (decal / sơn) tải xong
  await new Promise((r) => setTimeout(r, 400));
  const scene = new THREE.Scene();
  scene.add(new THREE.HemisphereLight(0xffffff, 0x8899bb, 1.2));
  const key = new THREE.DirectionalLight(0xffffff, 2.2);
  key.position.set(-6, 9, 8);
  scene.add(key);
  const rim = new THREE.DirectionalLight(0xbfe4ff, 1.6);
  rim.position.set(7, 4, -6);
  scene.add(rim);
  scene.add(obj.root);
  obj.update(0, 0.25, false, false, 0.016);
  // Khung xe theo hộp bao: camera phối cảnh 3/4 trước trái.
  const box = new THREE.Box3().setFromObject(obj.root);
  const size = box.getSize(new THREE.Vector3());
  const c = box.getCenter(new THREE.Vector3());
  const cam = new THREE.PerspectiveCamera(30, TW / TH, 0.1, 200);
  const dist = Math.max(size.z, size.x * 1.5, size.y * 2) * 1.05 + 0.6;
  cam.position.set(c.x - dist * 0.62, c.y + dist * 0.38, c.z + dist * 0.72);
  cam.lookAt(c.x, c.y * 0.8, c.z);
  const shot = (bg) => {
    scene.background = new THREE.Color(bg);
    view.renderer.render(scene, cam);
    const cv = document.createElement('canvas');
    cv.width = TW; cv.height = TH;
    cv.getContext('2d').drawImage(carCanvas, 0, 0, TW, TH);
    return cv.getContext('2d').getImageData(0, 0, TW, TH);
  };
  const B = shot(0x000000), Wt = shot(0xffffff);
  // alpha = 1 − (trắng − đen); màu = đen / alpha.
  const out = new ImageData(TW, TH);
  for (let i = 0; i < B.data.length; i += 4) {
    const d = ((Wt.data[i] - B.data[i]) + (Wt.data[i + 1] - B.data[i + 1]) + (Wt.data[i + 2] - B.data[i + 2])) / 3;
    const a = Math.max(0, Math.min(255, 255 - d));
    out.data[i + 3] = a;
    for (let k = 0; k < 3; k++) out.data[i + k] = a ? Math.min(255, Math.round((B.data[i + k] * 255) / a)) : 0;
  }
  const cv = document.createElement('canvas');
  cv.width = TW; cv.height = TH;
  cv.getContext('2d').putImageData(out, 0, 0);
  return { png: cv.toDataURL('image/png'), outline: null, car: true };
}

window.ITEM_IDS = [...Object.keys(ITEMS), ...CARS.map((c) => `car_${c}`), ...Object.keys(ITEM_CARS).map((c) => `car_${c}`)];
window.renderItem = async (id) => (ITEMS[id] ? renderSimple(id) : renderCar(id.replace(/^car_/, '')));
window.questsReady = true;
