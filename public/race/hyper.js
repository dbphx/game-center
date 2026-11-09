// Siêu xe viễn tưởng dựng bằng code (three.js, DCity tự thiết kế — CC0): thân loft góc cạnh, buồng lái kính, vây lưỡi
// dao, cánh kính phát sáng, dải neon, mâm vành neon, xe bay. Chỉ học phong cách game đua arcade, không chép mẫu xe nào.
// Hệ toạ độ như mọi xe trong scene.js: mũi xe hướng +z, bề ngang ≈ 2 m, đáy bánh y = 0.
import * as THREE from 'three';
import { mergeGeometries, toCreasedNormals } from 'three/addons/utils/BufferGeometryUtils.js';

// ---------- Vật liệu ----------

// glowMat: vật liệu phát sáng không qua tone mapping; k > 1 cho bloom bắt sáng rõ.
export function glowMat(color, k = 1.6) {
  return new THREE.MeshBasicMaterial({ color: new THREE.Color(color).multiplyScalar(k), toneMapped: false });
}

// paintMat: sơn bóng có lớp phủ. chroma: sơn ánh kim đổi màu theo góc nhìn (lớp ngũ sắc), pearl: trắng ngọc trai.
export function paintMat(color, env, low, opt = {}) {
  if (low) return new THREE.MeshStandardMaterial({ color, metalness: opt.chroma ? 0.6 : 0.35, roughness: 0.38, envMap: env, envMapIntensity: 0.55 });
  // Phản xạ gương vừa phải: mặt phẳng lớn (cánh trước, nắp) không loá trắng khi đèn chiếu từ trên.
  const m = new THREE.MeshPhysicalMaterial({ color, metalness: 0.25, roughness: 0.42, specularIntensity: 0.55, clearcoat: 0.25, clearcoatRoughness: 0.4, envMap: env, envMapIntensity: 0.42 });
  if (opt.chroma) Object.assign(m, { metalness: 0.6, roughness: 0.34, iridescence: 1, iridescenceIOR: 1.9, iridescenceThicknessRange: [180, 820], envMapIntensity: 0.6 });
  if (opt.pearl) Object.assign(m, { iridescence: 0.6, iridescenceIOR: 1.5, iridescenceThicknessRange: [250, 520], envMapIntensity: 0.35, roughness: 0.45 });
  return m;
}

// Kết cấu cánh phát sáng (vẽ một lần trên canvas): mờ ở gốc, sáng dần lên mép, có vài vệt sáng dọc.
let FIN_TEX = null;
function finTexture() {
  if (FIN_TEX) return FIN_TEX;
  const cv = document.createElement('canvas');
  cv.width = cv.height = 128;
  const x = cv.getContext('2d');
  const g = x.createLinearGradient(0, 128, 0, 0);
  g.addColorStop(0, 'rgba(255,255,255,0.05)');
  g.addColorStop(0.55, 'rgba(255,255,255,0.32)');
  g.addColorStop(0.9, 'rgba(255,255,255,0.75)');
  g.addColorStop(1, 'rgba(255,255,255,1)');
  x.fillStyle = g;
  x.fillRect(0, 0, 128, 128);
  x.globalCompositeOperation = 'lighter';
  for (let i = 0; i < 7; i++) {
    const u = 10 + i * 17;
    const s = x.createLinearGradient(0, 128, 0, 0);
    s.addColorStop(0, 'rgba(255,255,255,0)');
    s.addColorStop(1, 'rgba(255,255,255,0.35)');
    x.strokeStyle = s;
    x.lineWidth = 2;
    x.beginPath();
    x.moveTo(u, 128);
    x.lineTo(u + 22, 0);
    x.stroke();
  }
  FIN_TEX = new THREE.CanvasTexture(cv);
  FIN_TEX.colorSpace = THREE.SRGBColorSpace;
  return FIN_TEX;
}
function finMat(color, opacity = 0.9) {
  return new THREE.MeshBasicMaterial({ color: new THREE.Color(color), map: finTexture(), transparent: true, opacity, side: THREE.DoubleSide, blending: THREE.AdditiveBlending, depthWrite: false, toneMapped: false });
}

// ---------- Hình học ----------

// loft nối các mặt cắt (trước → sau) thành vỏ kín. Mỗi mặt cắt {z, pts:[[x,y],…]} là nửa phải, đi từ đáy giữa (x = 0)
// lên đỉnh giữa (x = 0); nửa trái tự đối xứng. Mọi mặt cắt cùng số điểm. Nếp gấp > 28° giữ cạnh sắc.
function loft(stations, crease = 0.5) {
  const n = stations[0].pts.length;
  const M = 2 * n - 2;
  const pos = [];
  const uv = [];
  stations.forEach((st, s) => {
    const ring = st.pts.slice();
    for (let i = n - 2; i >= 1; i--) ring.push([-st.pts[i][0], st.pts[i][1]]);
    ring.forEach(([x, y], i) => { pos.push(x, y, st.z); uv.push(i / M, s / (stations.length - 1)); });
  });
  const idx = [];
  for (let s = 0; s < stations.length - 1; s++) for (let i = 0; i < M; i++) {
    const a = s * M + i, b = s * M + ((i + 1) % M), c = (s + 1) * M + i, d = (s + 1) * M + ((i + 1) % M);
    idx.push(a, c, b, b, c, d);
  }
  // Nắp trước / sau: quạt quanh tâm mặt cắt.
  for (const [s, front] of [[0, true], [stations.length - 1, false]]) {
    let cx = 0, cy = 0;
    for (let i = 0; i < M; i++) { cx += pos[(s * M + i) * 3]; cy += pos[(s * M + i) * 3 + 1]; }
    const ci = pos.length / 3;
    pos.push(cx / M, cy / M, stations[s].z);
    uv.push(0.5, s ? 1 : 0);
    for (let i = 0; i < M; i++) {
      const a = s * M + i, b = s * M + ((i + 1) % M);
      if (front) idx.push(ci, a, b); else idx.push(ci, b, a);
    }
  }
  const g = new THREE.BufferGeometry();
  g.setAttribute('position', new THREE.Float32BufferAttribute(pos, 3));
  g.setAttribute('uv', new THREE.Float32BufferAttribute(uv, 2));
  g.setIndex(idx);
  return toCreasedNormals(g, crease);
}

// plate: tấm cắt theo đường viền (u = z dọc xe, v = y) rồi đùn dày theo x, tâm x = 0.
function plate(pts, depth = 0.04, bevel = 0.008) {
  const sh = new THREE.Shape(pts.map(([u, v]) => new THREE.Vector2(u, v)));
  const g = new THREE.ExtrudeGeometry(sh, { depth, steps: 1, bevelEnabled: bevel > 0, bevelThickness: bevel, bevelSize: bevel, bevelSegments: 1 });
  g.translate(0, 0, -depth / 2);
  g.rotateY(-Math.PI / 2);
  return g;
}

// finGeo: cánh mỏng hình ngọn lửa (u = z, v = y), gốc từ u = 0 tới −base, đỉnh lùi sweep và cao h; uv chuẩn hoá 0..1.
function finGeo(base, h, sweep, bend = 0.55, prongs = 1) {
  const s = new THREE.Shape();
  s.moveTo(0, 0);
  for (let k = 0; k < prongs; k++) {
    const f = (k + 1) / prongs;
    const tipU = -base * (0.35 + 0.65 * f) - sweep * (0.6 + 0.4 * f);
    const tipV = h * (1 - 0.22 * (prongs - 1 - k));
    s.quadraticCurveTo(-base * f * 0.35 - (k ? base * 0.3 * f : 0), tipV * bend, tipU, tipV);
    s.quadraticCurveTo(-base * f * 0.9 - sweep * 0.2, tipV * 0.42, -base * Math.min(1, f + 0.12), k === prongs - 1 ? 0 : tipV * 0.3);
  }
  s.lineTo(0, 0);
  const g = new THREE.ShapeGeometry(s, 12);
  g.computeBoundingBox();
  const bb = g.boundingBox;
  const p = g.attributes.position;
  const uv = g.attributes.uv;
  for (let i = 0; i < p.count; i++) uv.setXY(i, (p.getX(i) - bb.min.x) / (bb.max.x - bb.min.x || 1), (p.getY(i) - bb.min.y) / (bb.max.y - bb.min.y || 1));
  g.rotateY(-Math.PI / 2);
  return { geo: g, tip: [s.getPoints(12)] };
}

// tube: dải neon tròn chạy qua các điểm (đường gấp khúc giữ góc nhọn khi sharp).
function tube(points, r = 0.016, sharp = false) {
  const v = points.map((p) => new THREE.Vector3(...p));
  let curve;
  if (sharp) {
    curve = new THREE.CurvePath();
    for (let i = 0; i < v.length - 1; i++) curve.add(new THREE.LineCurve3(v[i], v[i + 1]));
  } else curve = new THREE.CatmullRomCurve3(v);
  return new THREE.TubeGeometry(curve, Math.max(8, v.length * (sharp ? 2 : 6)), r, 5, false);
}

// Bộ gom: mọi khối tĩnh được nướng vị trí vào hình học rồi gộp theo vật liệu (ít lệnh vẽ, nhẹ cho điện thoại).
class Kit {
  constructor() { this.parts = new Map(); }
  put(geo, mat, p = [0, 0, 0], r = [0, 0, 0], s = [1, 1, 1], mirror = false) {
    const m = new THREE.Matrix4().compose(new THREE.Vector3(...p), new THREE.Quaternion().setFromEuler(new THREE.Euler(...r)), new THREE.Vector3(...s));
    const add = (g) => {
      let ng = g.index ? g.toNonIndexed() : g.clone();
      for (const k of Object.keys(ng.attributes)) if (!['position', 'normal', 'uv'].includes(k)) ng.deleteAttribute(k);
      if (!ng.attributes.uv) ng.setAttribute('uv', new THREE.Float32BufferAttribute(new Float32Array(ng.attributes.position.count * 2), 2));
      if (!ng.attributes.normal) ng.computeVertexNormals();
      ng.clearGroups();
      return ng;
    };
    const one = add(geo);
    one.applyMatrix4(m);
    this.push(mat, one);
    if (mirror) {
      const two = add(geo);
      two.applyMatrix4(m);
      two.applyMatrix4(new THREE.Matrix4().makeScale(-1, 1, 1));
      flip(two);
      this.push(mat, two);
    }
  }
  // sym: đặt khối ở bên phải (x > 0) và bản đối xứng bên trái.
  sym(geo, mat, p, r, s) { this.put(geo, mat, p, r, s, true); }
  push(mat, g) {
    if (!this.parts.has(mat)) this.parts.set(mat, []);
    this.parts.get(mat).push(g);
  }
  build(group) {
    for (const [mat, list] of this.parts) {
      const mesh = new THREE.Mesh(mergeGeometries(list, false), mat);
      if (mat.transparent) mesh.renderOrder = 3;
      group.add(mesh);
    }
  }
}
// flip đảo chiều tam giác (sau khi lật gương) để mặt ngoài vẫn quay ra ngoài.
function flip(g) {
  for (const k of ['position', 'normal', 'uv']) {
    const a = g.attributes[k];
    if (!a) continue;
    const n = a.itemSize;
    for (let t = 0; t < a.count; t += 3) for (let c = 0; c < n; c++) {
      const i1 = (t + 1) * n + c, i2 = (t + 2) * n + c;
      const tmp = a.array[i1];
      a.array[i1] = a.array[i2];
      a.array[i2] = tmp;
    }
  }
}

// ---------- Bánh xe ----------

// wheel: lốp + mâm tối nhiều chấu xoắn + vành neon ở mặt ngoài. side 1 = bên phải (mặt ngoài +x). Trục quay là x.
function wheel(name, r, w, side, mats, tires) {
  const g = new THREE.Group();
  g.name = name;
  const k = new Kit();
  const tyre = new THREE.CylinderGeometry(r, r, w, 30, 1);
  tyre.rotateZ(Math.PI / 2);
  k.put(tyre, mats.rubber);
  // Vai lốp bo tròn.
  const lip = new THREE.TorusGeometry(r * 0.9, w * 0.16, 6, 30);
  lip.rotateY(Math.PI / 2);
  k.put(lip, mats.rubber, [side * w * 0.42, 0, 0]);
  const rim = new THREE.CylinderGeometry(r * 0.74, r * 0.74, w * 0.9, 30, 1);
  rim.rotateZ(Math.PI / 2);
  k.put(rim, mats.rim, [side * 0.01, 0, 0]);
  // Chấu xoắn kiểu tuabin.
  const n = 6;
  for (let i = 0; i < n; i++) {
    const a = (i / n) * Math.PI * 2;
    const sp = new THREE.BoxGeometry(0.03, r * 0.7, 0.07);
    sp.translate(0, r * 0.36, 0);
    k.put(sp, mats.spoke, [side * w * 0.47, 0, 0], [a, side * 0.35, 0]);
  }
  const hub = new THREE.CylinderGeometry(r * 0.16, r * 0.16, 0.04, 12);
  hub.rotateZ(Math.PI / 2);
  k.put(hub, mats.accent, [side * w * 0.5, 0, 0]);
  const ring = new THREE.TorusGeometry(r * 0.76, tires >= 4 ? 0.024 : 0.016, 6, 40);
  ring.rotateY(Math.PI / 2);
  k.put(ring, mats.accent, [side * w * 0.5, 0, 0]);
  k.build(g);
  return g;
}

// ---------- Thân xe ----------

// sec: mặt cắt nửa phải 8 điểm. wb/yb đáy, ys mép trên vách dưới, wf/yf vai vè, wd/yd rãnh nắp, yc đỉnh giữa.
const sec = (z, wb, yb, ys, wf, yf, wd, yd, yc) => ({ z, pts: [[0, yb], [wb, yb], [wb + 0.03, ys], [wf, yf], [wf - 0.07, yf + 0.06], [wd, yd], [wd * 0.45, yc], [0, yc]] });

// bodyStations: thân siêu xe thấp rộng, vè bánh nhô cao, eo thắt giữa hai trục. P chỉnh độ cao / bề rộng theo mẫu.
function bodyStations(P) {
  const A = P.axle, h = P.h || 1, rr = P.rearRise || 0, fw = P.fw || 1;
  const y = (v) => v * h;
  const list = [
    sec(2.36, 0.72, 0.13, 0.15, 0.78, y(0.18), 0.4, y(0.22), y(0.22)),
    sec(2.12, 0.86, 0.12, 0.2, 0.93, y(0.32), 0.42, y(0.33), y(0.35)),
    sec(A + 0.46, 0.6, 0.14, y(0.66), 0.97, y(0.6), 0.44, y(0.46), y(0.44)),
    sec(A, 0.6, 0.14, y(0.74), 0.99, y(0.78), 0.44, y(0.53), y(0.5)),
    sec(A - 0.44, 0.6, 0.14, y(0.68), 0.95, y(0.72), 0.46, y(0.58), y(0.58)),
    sec(0.55, 0.66, 0.14, y(0.42), P.waist, y(0.64), 0.5, y(0.64), y(0.66)),
    sec(-0.2, 0.68, 0.14, y(0.4), P.waist - 0.02, y(0.66), 0.52, y(0.68), y(0.68)),
    sec(-A + 0.46, 0.62, 0.14, y(0.7 + rr), 1.0, y(0.8 + rr), 0.5, y(0.72 + rr), y(0.7 + rr)),
    sec(-A, 0.6, 0.14, y(0.76 + rr), 1.0, y(0.86 + rr), 0.5, y(0.76 + rr), y(0.72 + rr)),
    sec(-A - 0.42, 0.62, 0.16, y(0.7 + rr), 0.99, y(0.82 + rr), 0.52, y(0.74 + rr), y(0.7 + rr)),
    sec(-2.22, 0.8, 0.22, y(0.4), 0.95, y(0.74 + rr), 0.55, y(0.7 + rr), y(0.68 + rr)),
    sec(-2.36, 0.76, 0.26, y(0.42), 0.88, y(0.66 + rr), 0.5, y(0.62 + rr), y(0.6 + rr)),
  ];
  // fw < 1: co hẹp phần thân ngoài (xe công thức lộ bánh); giữ mũi và đuôi.
  if (fw !== 1) for (const st of list) for (const p of st.pts) if (p[0] > 0.45) p[0] = 0.45 + (p[0] - 0.45) * fw;
  return list;
}

// canopy: buồng lái giọt nước bằng kính (kiểu bubble hoặc jet dài thấp).
function canopyStations(kind, h) {
  const t = (base, w, top) => {
    const tp = base + (top - base) * h;
    return [[0, base], [w, base], [w * 0.96, base + 0.1], [w * 0.74, tp - 0.06], [w * 0.38, tp], [0, tp]];
  };
  if (kind === 'jet') return [
    { z: 1.25, pts: t(0.5, 0.42, 0.5) }, { z: 0.75, pts: t(0.6, 0.55, 0.86) }, { z: 0.05, pts: t(0.65, 0.6, 0.98) },
    { z: -0.7, pts: t(0.68, 0.56, 0.95) }, { z: -1.5, pts: t(0.72, 0.42, 0.84) }, { z: -2.05, pts: t(0.72, 0.22, 0.74) },
  ];
  return [
    { z: 1.0, pts: t(0.6, 0.48, 0.6) }, { z: 0.55, pts: t(0.62, 0.56, 0.9) }, { z: -0.05, pts: t(0.66, 0.6, 1.02) },
    { z: -0.6, pts: t(0.68, 0.57, 0.99) }, { z: -1.15, pts: t(0.7, 0.46, 0.86) }, { z: -1.6, pts: t(0.72, 0.28, 0.76) },
  ];
}

// spikes: răng cưa lưỡi dao (u = z, v = y) dọc từ z0 về z1, n răng cao dần (grow) và ngả về sau (lean).
function spikePts(z0, z1, n, y0, h, lean, grow = 0.5) {
  const pts = [[z0 + 0.1, y0 - 0.05]];
  const step = (z0 - z1) / n;
  for (let i = 0; i < n; i++) {
    const zb = z0 - i * step;
    const hh = h * (1 - grow + grow * ((i + 1) / n));
    pts.push([zb, y0], [zb - step * 0.25 - lean, y0 + hh], [zb - step * 0.62, y0 + hh * 0.18]);
  }
  pts.push([z1, y0], [z1, y0 - 0.05]);
  return pts;
}

// ---------- Các mẫu siêu xe ----------
// accent / accent2: hai tông neon riêng của mẫu (neon gầm lắp thêm thì đè accent). canopy: 'bubble' | 'jet' | 'open'.
// fins: cánh kính phát sáng [{z, x, y, base, h, sweep, tilt, yaw, prongs, color: 1|2}], spikes: lưỡi dao răng cưa hai bên,
// boosters: số ống phản lực đuôi, hover: xe bay không bánh.
export const HYPER = {
  phantom: { accent: 0x38f2ff, accent2: 0x7a5cff, canopy: 'jet', waist: 0.8, h: 0.88, rearRise: 0.0,
    fins: [{ z: -1.25, x: 0.62, y: 0.72, base: 0.9, h: 0.95, sweep: 0.45, tilt: 0.28, yaw: 0.04, prongs: 1, color: 1 }],
    sideSpikes: 0, rearBlades: 3, splitter: true },
  vortex: { accent: 0x2fffc8, accent2: 0xb44dff, canopy: 'bubble', waist: 0.86, h: 1.0, hover: true,
    fins: [{ z: -1.5, x: 0.7, y: 0.78, base: 0.7, h: 0.6, sweep: 0.3, tilt: 0.75, yaw: 0.05, prongs: 2, color: 2 }],
    sideSpikes: 0, rearBlades: 2, splitter: true, mandibles: false },
  apex: { accent: 0xff3fd0, accent2: 0xb44dff, canopy: 'jet', waist: 0.84, h: 0.98, rearRise: 0.04, crease: 0.22,
    fins: [{ z: -1.6, x: 0.55, y: 0.82, base: 0.6, h: 0.55, sweep: 0.3, tilt: 0.25, yaw: 0, prongs: 3, color: 1 }],
    sideSpikes: 7, shoulderSpikes: 6, rearBlades: 4, splitter: true },
  blaze: { accent: 0xff7a1f, accent2: 0xffd23f, canopy: 'open', waist: 0.8, h: 0.92, rearRise: 0.06, fw: 0.45, pods: true,
    fins: [{ z: -1.3, x: 0.66, y: 0.8, base: 1.0, h: 0.75, sweep: 0.55, tilt: 0.35, yaw: 0.03, prongs: 3, color: 1 }],
    sideSpikes: 4, rearBlades: 2, boosters: 3, frontWing: true, mandibles: false },
  nova: { accent: 0xb04dff, accent2: 0x46f0ff, canopy: 'jet', waist: 0.84, h: 1.0, rearRise: 0.04,
    fins: [
      { z: -0.9, x: 0.66, y: 0.74, base: 1.2, h: 1.25, sweep: 0.7, tilt: 0.32, yaw: 0.06, prongs: 2, color: 1 },
      { z: -1.7, x: 0.78, y: 0.82, base: 0.8, h: 0.95, sweep: 0.5, tilt: 0.5, yaw: 0.04, prongs: 2, color: 1 },
      { z: 1.75, x: 0.84, y: 0.66, base: 0.55, h: 0.42, sweep: 0.4, tilt: 0.7, yaw: 0, prongs: 1, color: 2 },
    ],
    sideSpikes: 5, rearBlades: 3, splitter: true, halo: true },
  // Xe độc quyền (ghép từ mảnh nhiệm vụ): Cực Quang — trắng ngọc trai, vây băng, cánh pha lê cyan → tím lung linh.
  x_aurora: { accent: 0x5ff4ff, accent2: 0xb98cff, canopy: 'jet', waist: 0.84, h: 1.0, rearRise: 0.03, pearl: true, aurora: true,
    fins: [
      { z: -0.8, x: 0.64, y: 0.74, base: 1.3, h: 1.35, sweep: 0.75, tilt: 0.3, yaw: 0.06, prongs: 3, color: 1 },
      { z: -1.75, x: 0.8, y: 0.82, base: 0.85, h: 1.0, sweep: 0.55, tilt: 0.55, yaw: 0.04, prongs: 2, color: 2 },
      { z: 1.8, x: 0.86, y: 0.64, base: 0.6, h: 0.5, sweep: 0.42, tilt: 0.75, yaw: 0, prongs: 2, color: 1 },
    ],
    sideSpikes: 6, shoulderSpikes: 4, rearBlades: 3, splitter: true, halo: true, crystals: true },
};

// buildHyper dựng thân + bánh của siêu xe. Trả {group, wheels:[nhóm bánh], length, height, lights:true}.
// opt: {paint (hex), paintKind ('chroma' | ''), accent (đè tông neon chính), env, low, tires}.
export function buildHyper(id, opt) {
  const D = HYPER[id];
  const group = new THREE.Group();
  const A = 1.42;
  const R = 0.39;
  const accent = opt.accent != null ? opt.accent : D.accent;
  const env = opt.env;
  const body = D.pearl ? paintMat(0xc9d3e6, env, opt.low, { pearl: true }) : paintMat(opt.paint, env, opt.low, { chroma: opt.paintKind === 'chroma' });
  // Mảng phụ: xe ngọc trai lấy màu sơn cho vây; xe thường dùng carbon tối.
  const second = D.pearl ? paintMat(opt.paintKind === 'chroma' ? opt.paint : 0x8fdcff, env, opt.low, { chroma: opt.paintKind === 'chroma' })
    : new THREE.MeshStandardMaterial({ color: 0x15171f, metalness: 0.55, roughness: 0.32, envMap: env, envMapIntensity: 0.6 });
  const glass = new THREE.MeshPhysicalMaterial({ color: 0x0a0f1e, metalness: 0.25, roughness: 0.4, clearcoat: 0.3, clearcoatRoughness: 0.45, envMap: env, envMapIntensity: 0.35 });
  const g1 = glowMat(accent, 1.7);
  const g2 = glowMat(D.accent2, 1.6);
  const white = glowMat(0xd8ecff, 0.95);
  const red = glowMat(0xff2a3a, 1.5);
  const mats = { accent: g1, accent2: g2, fin1: finMat(accent, 0.95), fin2: finMat(D.accent2, 0.9) };
  const k = new Kit();

  // Thân chính.
  k.put(loft(bodyStations({ axle: A, waist: D.waist, h: D.h, rearRise: D.rearRise, fw: D.fw }), D.crease || 0.36), body);
  // Nắp capo: hai hốc gió tối và vạch neon hình mũi tên.
  const hy = (z) => (0.42 + (2.0 - z) * 0.17) * D.h; // cao độ gần đúng của nắp capo theo z
  k.sym(plate([[1.98, hy(1.98) + 0.005], [1.1, hy(1.1) + 0.005], [1.1, hy(1.1) + 0.035], [1.98, hy(1.98) + 0.03]], 0.2, 0.004), second, [0.27, 0.01, 0]);
  k.sym(tube([[0.38, hy(1.95) + 0.04, 1.95], [0.38, hy(1.15) + 0.04, 1.15]], 0.008), g1);
  k.sym(tube([[0.02, hy(1.6) + 0.035, 1.6], [0.16, hy(1.85) + 0.03, 1.85]], 0.01, true), g2);
  // Buồng lái: kính (hoặc mui trần có ghế đua tối màu).
  if (D.canopy === 'open') {
    const tub = loft([
      { z: 0.6, pts: [[0, 0.5], [0.42, 0.5], [0.44, 0.62], [0.38, 0.7], [0, 0.7]] },
      { z: -0.7, pts: [[0, 0.5], [0.42, 0.5], [0.44, 0.62], [0.38, 0.7], [0, 0.7]] },
    ]);
    k.put(tub, second, [0, 0.01, 0]);
    // Ốp sau lưng người lái nhô thành vây giữa.
    k.put(plate([[-0.55, 0.6], [-0.85, 1.05], [-1.9, 0.98], [-2.2, 0.7]], 0.12, 0.02), body);
    k.put(plate([[0.95, 0.66], [0.75, 0.82], [0.62, 0.84], [0.6, 0.66]], 0.7, 0.01), glass);
  } else {
    k.put(loft(canopyStations(D.canopy, 1), 0.7), glass);
    // Sống mui màu thân và viền neon quanh kính.
    k.put(plate([[0.7, 0.9], [0.05, 1.0], [-0.9, 0.95], [-1.6, 0.82], [-1.62, 0.78], [-0.9, 0.9], [0.05, 0.95], [0.7, 0.86]], 0.1, 0.01), body);
    k.sym(tube([[0.42, 0.62, 1.2], [0.58, 0.7, 0.5], [0.6, 0.72, -0.4], [0.5, 0.74, -1.3], [0.3, 0.74, -1.9]], 0.012), g1);
  }

  // Xe công thức: két nước hai bên giữa hai bánh (thân hẹp, bánh lộ ra ngoài).
  if (D.pods) {
    k.sym(loft([
      { z: 0.95, pts: [[0, 0.16], [0.18, 0.16], [0.2, 0.4], [0.1, 0.46], [0, 0.46]] },
      { z: 0.6, pts: [[0, 0.14], [0.3, 0.14], [0.32, 0.5], [0.16, 0.56], [0, 0.56]] },
      { z: -0.7, pts: [[0, 0.14], [0.3, 0.14], [0.3, 0.52], [0.15, 0.58], [0, 0.58]] },
      { z: -1.0, pts: [[0, 0.16], [0.12, 0.16], [0.12, 0.3], [0.05, 0.34], [0, 0.34]] },
    ], 0.4), body, [0.58, 0, 0]);
    k.sym(plate([[0.95, 0.18], [0.62, 0.52], [0.62, 0.18]], 0.3, 0.004), second, [0.6, 0, 0.01]);
    k.sym(tube([[0.92, 0.5, 0.6], [0.92, 0.5, -0.7]], 0.012), g1);
  }
  // Vách hông: hốc gió và tấm lưỡi dao chạy dọc sườn giữa hai bánh.
  if (!D.pods) k.sym(plate([[0.95, 0.18], [0.85, 0.42], [0.2, 0.5], [-0.6, 0.62], [-0.98, 0.5], [-0.98, 0.18]], 0.06, 0.01), second, [0.76, 0, 0], [0, 0, 0.12]);
  if (!D.pods) {
    k.sym(tube([[0.81, 0.22, 0.95], [0.83, 0.42, 0.8], [0.84, 0.5, 0.2], [0.85, 0.62, -0.6], [0.85, 0.52, -0.98]], 0.014), g1);
    k.sym(tube([[0.79, 0.16, 1.0], [0.8, 0.16, -1.0]], 0.012), g2);
  }
  if (D.sideSpikes && D.pods) k.sym(plate(spikePts(0.5, -0.9, D.sideSpikes, 0.56, 0.28, 0.12, 0.6), 0.03, 0.006), second, [0.75, 0, 0], [0, 0, 0.3]);
  else if (D.sideSpikes) k.sym(plate(spikePts(0.9, -1.0, D.sideSpikes, 0.24, 0.32, 0.12, 0.6), 0.03, 0.006), second, [0.84, 0, 0], [0, 0, 0.18]);
  if (D.shoulderSpikes) {
    k.sym(plate(spikePts(-0.4, -2.0, D.shoulderSpikes, 0.78, 0.36, 0.14, 0.7), 0.03, 0.006), body, [0.82, 0.02, 0], [0, 0, 0.42]);
    k.sym(plate(spikePts(1.95, 1.0, 3, 0.68, 0.22, 0.08, 0.6), 0.03, 0.006), second, [0.84, 0, 0], [0, 0, 0.5]);
  }

  // Mũi: lưỡi chia gió hai tầng, đèn pha xếch dạng khe.
  if (D.splitter || D.frontWing) {
    k.put(plate([[2.5, 0.08], [2.3, 0.16], [1.95, 0.16], [1.9, 0.08]], 2.0, 0.01), second);
    k.sym(plate([[2.46, 0.1], [2.62, 0.3], [2.3, 0.24], [1.95, 0.14]], 0.04, 0.006), second, [0.9, 0, 0]);
    k.put(tube([[-0.98, 0.17, 2.5], [0.98, 0.17, 2.5]], 0.012), g1);
  }
  if (D.frontWing) {
    k.put(plate([[2.75, 0.2], [2.6, 0.32], [2.25, 0.3], [2.3, 0.2]], 2.2, 0.01), body);
    k.sym(plate([[2.8, 0.14], [2.75, 0.5], [2.25, 0.42], [2.25, 0.14]], 0.04, 0.006), second, [1.1, 0, 0]);
  }
  k.sym(plate([[2.3, 0.3], [2.2, 0.345], [1.78, 0.5], [1.8, 0.455]], 0.1, 0.003), white, [0.62 * (D.fw || 1) + 0.08, 0.01, 0], [0, 0, 0.5]);
  // Hàm mũi: lưỡi dao chĩa về trước hai bên mũi, viền neon.
  if (D.mandibles !== false) {
    k.sym(plate([[1.7, 0.22], [2.78, 0.24], [2.15, 0.36], [1.75, 0.46]], 0.035, 0.006), second, [0.8, 0, 0], [0, -0.05, 0.1]);
    k.sym(tube([[0, 0.24, 2.78], [0, 0.36, 2.15], [0, 0.46, 1.75]], 0.011, true), g1, [0.8, 0, 0], [0, -0.05, 0.1]);
  }
  // Hốc hút gió sau bánh trước: mảng tối kèm hai lá dọc.
  if (!D.pods) k.sym(plate([[1.0, 0.24], [0.95, 0.58], [0.72, 0.6], [0.6, 0.3]], 0.05, 0.006), second, [0.86, 0, 0], [0, 0, 0.1]);
  k.sym(tube([[0.4, 0.27, 2.3], [0.78, 0.38, 2.12], [0.95, 0.56, 1.9]], 0.014), g2);

  // Đuôi: dải đèn hậu ngang, khuếch tán gió có lá dọc, vây lưỡi dao sau.
  // Mặt đuôi tối có dải đèn hậu đỏ dày chạy ngang, vuốt lên hai góc.
  const ty = (v) => v * D.h + (D.rearRise || 0);
  k.put(plate([[-2.355, ty(0.3)], [-2.4, ty(0.3)], [-2.4, ty(0.62)], [-2.355, ty(0.62)]], 1.62, 0.006), second);
  k.put(tube([[-0.78, ty(0.52), -2.41], [0.78, ty(0.52), -2.41]], 0.032), red);
  k.sym(tube([[0.78, ty(0.52), -2.41], [0.86, ty(0.62), -2.37], [0.95, ty(0.74), -2.22]], 0.026, true), red);
  k.put(tube([[-0.5, ty(0.38), -2.41], [0.5, ty(0.38), -2.41]], 0.012), g1);
  for (let i = 0; i < (D.rearBlades || 0); i++) {
    const x = 0.2 + i * (0.6 / Math.max(1, D.rearBlades - 1));
    k.sym(plate([[-1.9, 0.14], [-2.3, 0.14], [-2.62, 0.34], [-2.28, 0.4]], 0.035, 0.005), second, [x, 0, 0]);
  }
  k.put(plate([[-1.85, 0.12], [-2.35, 0.12], [-2.45, 0.24], [-1.9, 0.18]], 1.7, 0.006), second);

  // Ống phản lực (Hoả Tiễn).
  for (let i = 0; i < (D.boosters || 0); i++) {
    const x = (i - (D.boosters - 1) / 2) * 0.46;
    const noz = new THREE.CylinderGeometry(0.15, 0.19, 0.42, 14, 1, true);
    noz.rotateX(Math.PI / 2);
    k.put(noz, second, [x, 0.48, -2.42]);
    const core = new THREE.CircleGeometry(0.14, 14);
    core.rotateY(Math.PI);
    k.put(core, g2, [x, 0.48, -2.6]);
    const rim = new THREE.TorusGeometry(0.19, 0.02, 6, 18);
    k.put(rim, g1, [x, 0.48, -2.63]);
  }

  // Cánh kính phát sáng kèm sống cánh rắn và viền neon theo mép trước.
  const finsMeshes = [];
  for (const f of D.fins || []) {
    const { geo, tip } = finGeo(f.base, f.h, f.sweep, 0.55, f.prongs);
    const fm = f.color === 2 ? mats.fin2 : mats.fin1;
    k.sym(geo, fm, [f.x, f.y, f.z], [0, f.yaw, -f.tilt]);
    // Sống cánh: tấm rắn thấp ở gốc.
    k.sym(plate([[0.05, -0.02], [-f.base * 0.3, f.h * 0.28], [-f.base * 0.75, f.h * 0.2], [-f.base * 1.02, -0.02]], 0.035, 0.006), D.pearl ? second : body, [f.x, f.y, f.z], [0, f.yaw, -f.tilt]);
    // Viền neon theo mép đầu các mũi cánh.
    const pts = tip[0].map((p) => [0, p.y, p.x]);
    k.sym(tube(pts.filter((_, i) => i % 2 === 0), 0.011), f.color === 2 ? g2 : g1, [f.x, f.y, f.z], [0, f.yaw, -f.tilt]);
    finsMeshes.push(fm);
  }
  // Pha lê (Cực Quang): khối tinh thể xanh băng nhô trên vè sau và mũi.
  if (D.crystals) {
    const cm = new THREE.MeshPhysicalMaterial({ color: 0xbff4ff, metalness: 0, roughness: 0.05, transmission: opt.low ? 0 : 0.6, thickness: 0.3, transparent: true, opacity: 0.85, emissive: new THREE.Color(0x3fd8ff), emissiveIntensity: 0.6, envMap: env, envMapIntensity: 1.2 });
    const cry = new THREE.OctahedronGeometry(0.12, 0);
    for (const [x, y, z, s, r] of [[0.7, 0.92, -1.3, 1.3, 0.3], [0.62, 0.88, -1.65, 1.0, -0.2], [0.78, 0.86, -1.0, 0.8, 0.5], [0.5, 0.42, 2.05, 0.7, 0.1]]) k.sym(cry, cm, [x, y, z], [r, 0, 0.4], [s * 0.7, s * 1.8, s * 0.7]);
  }

  // Xe bay: bốn khối đẩy thay bánh, vòng sáng phía dưới.
  if (D.hover) {
    for (const z of [A, -A]) {
      const pod = new THREE.CylinderGeometry(0.26, 0.32, 0.16, 8);
      k.sym(pod, second, [0.74, 0.24, z], [0, Math.PI / 8, 0], [1, 1, 1.5]);
      const ring = new THREE.TorusGeometry(0.27, 0.025, 6, 8);
      ring.rotateX(Math.PI / 2);
      k.sym(ring, g1, [0.74, 0.16, z], [0, Math.PI / 8, 0], [1, 1, 1.5]);
      const core = new THREE.CircleGeometry(0.24, 8);
      core.rotateX(Math.PI / 2);
      k.sym(core, g2, [0.74, 0.155, z], [0, Math.PI / 8, 0], [1, 1, 1.5]);
    }
    // Vòng xoáy: hai vòng neon nghiêng ôm đuôi xe.
    for (const [r, y, c, a] of [[1.12, 0.98, g1, 0], [0.92, 1.06, g2, 2.2]]) {
      const tor = new THREE.TorusGeometry(r, 0.02, 6, 48, Math.PI * 1.25);
      tor.rotateX(Math.PI / 2);
      k.put(tor, c, [0, y, -1.55], [0.14, a, 0]);
    }
  }
  k.build(group);

  // Bánh (xoay được, bánh trước tên có "front" để bẻ lái).
  const wheels = [];
  if (!D.hover) {
    const wm = {
      rubber: new THREE.MeshStandardMaterial({ color: 0x111217, roughness: 0.85 }),
      rim: new THREE.MeshStandardMaterial({ color: 0x1c1f29, metalness: 0.7, roughness: 0.3, envMap: env, envMapIntensity: 0.7 }),
      spoke: new THREE.MeshStandardMaterial({ color: 0xb9c0d0, metalness: 0.85, roughness: 0.25, envMap: env, envMapIntensity: 0.9 }),
      accent: g1,
    };
    for (const [z, nm] of [[A, 'front'], [-A, 'rear']]) for (const side of [1, -1]) {
      const w = wheel(`wheel_${nm}_${side > 0 ? 'r' : 'l'}`, nm === 'rear' ? R + 0.02 : R, nm === 'rear' ? 0.36 : 0.32, side, wm, opt.tires || 0);
      w.position.set(side * 0.83, nm === 'rear' ? R + 0.02 : R, z);
      group.add(w);
      wheels.push(w);
    }
  }
  return { group, wheels, length: 5.0, height: 1.05, glows: [g1, mats.fin1], glows2: [g2, mats.fin2], fins: finsMeshes, aurora: !!D.aurora, halo: !!D.halo, accent, accent2: D.accent2 };
}

// ---------- Bộ đồ viễn tưởng gắn lên xe mô hình (hạng trung) ----------

// addKit gắn dải neon dọc sườn, lưỡi chia gió và (level 2) vây lưỡi dao + cánh kính nhỏ lên xe mô hình có sẵn.
export function addKit(extras, level, L, H, accent, accent2) {
  const k = new Kit();
  const g1 = glowMat(accent, 1.6);
  const g2 = glowMat(accent2, 1.5);
  const carbon = new THREE.MeshStandardMaterial({ color: 0x15171f, metalness: 0.55, roughness: 0.35 });
  const half = L / 2;
  // Dải neon dọc ngưỡng cửa (mép thân thấp) và vạch neon dưới cản trước.
  k.sym(tube([[1.0, H * 0.2, half * 0.55], [1.01, H * 0.2, -half * 0.6]], 0.014), g1);
  k.put(tube([[-0.85, H * 0.14, half - 0.06], [0.85, H * 0.14, half - 0.06]], 0.011), g2);
  if (level >= 2) {
    k.sym(plate(spikePts(half * 0.4, -half * 0.5, 5, H * 0.3, 0.2, 0.08, 0.6), 0.03, 0.006), carbon, [0.99, 0, 0], [0, 0, 0.1]);
    const { geo, tip } = finGeo(0.6, H * 0.55, 0.3, 0.55, 2);
    const fm = finMat(accent, 0.85);
    k.sym(geo, fm, [0.78, H * 0.62, -half * 0.55], [0, 0.04, -0.45]);
    k.sym(tube(tip[0].filter((_, i) => i % 2 === 0).map((p) => [0, p.y, p.x]), 0.01), g1, [0.78, H * 0.62, -half * 0.55], [0, 0.04, -0.45]);
    for (const x of [0.25, 0.55]) k.sym(plate([[-half + 0.3, 0.1], [-half - 0.02, 0.1], [-half - 0.22, 0.28], [-half + 0.02, 0.3]], 0.03, 0.005), carbon, [x, 0, 0]);
  }
  const g = new THREE.Group();
  k.build(g);
  extras.add(g);
  return { glows: [g1], glows2: [g2] };
}

// ---------- Siêu moto ----------

// buildHyperBike dựng moto viễn tưởng: yếm góc cạnh, bánh không trục vành neon, vây và cánh kính ở đuôi.
// Trả {group, wheels:[nhóm quay], seatY}. R / WB: bán kính bánh và chiều dài cơ sở của khung moto (scene.js).
export const HYPER_BIKE = {
  moto_neon: { accent: 0xff3fd0, accent2: 0x46f0ff },
  x_frost: { accent: 0x5ff4ff, accent2: 0xb98cff, pearl: true, aurora: true },
};
export function buildHyperBike(id, opt, R, WB) {
  const D = HYPER_BIKE[id];
  const group = new THREE.Group();
  const env = opt.env;
  const accent = opt.accent != null ? opt.accent : D.accent;
  const body = D.pearl ? paintMat(0xc9d3e6, env, opt.low, { pearl: true }) : paintMat(opt.paint, env, opt.low, { chroma: opt.paintKind === 'chroma' });
  const second = D.pearl ? paintMat(opt.paintKind === 'chroma' ? opt.paint : 0x8fdcff, env, opt.low, { chroma: opt.paintKind === 'chroma' })
    : new THREE.MeshStandardMaterial({ color: 0x15171f, metalness: 0.55, roughness: 0.32, envMap: env, envMapIntensity: 0.6 });
  const glass = new THREE.MeshPhysicalMaterial({ color: 0x9fe6ff, metalness: 0.1, roughness: 0.05, transparent: true, opacity: 0.45, envMap: env });
  const g1 = glowMat(accent, 1.7);
  const g2 = glowMat(D.accent2, 1.6);
  const fin1 = finMat(accent, 0.95);
  const fin2 = finMat(D.accent2, 0.9);
  const k = new Kit();
  const y0 = R;
  // Yếm: thân loft hẹp, mũi nhọn thấp, gù bình xăng, đuôi vuốt cao.
  const s = (z, w, yb, yt) => ({ z, pts: [[0, yb], [w * 0.7, yb], [w, (yb + yt) / 2], [w * 0.8, yt - 0.04], [w * 0.35, yt], [0, yt]] });
  k.put(loft([
    s(1.12, 0.05, y0 + 0.28, y0 + 0.36), s(0.9, 0.2, y0 + 0.1, y0 + 0.62), s(0.45, 0.24, y0 + 0.02, y0 + 0.66), s(0.05, 0.22, y0 + 0.02, y0 + 0.7),
    s(-0.35, 0.16, y0 + 0.18, y0 + 0.6), s(-0.8, 0.12, y0 + 0.42, y0 + 0.72), s(-1.12, 0.03, y0 + 0.6, y0 + 0.78),
  ], 0.5), body);
  // Kính chắn gió.
  k.put(plate([[0.88, y0 + 0.6], [0.62, y0 + 0.84], [0.5, y0 + 0.8], [0.58, y0 + 0.64]], 0.3, 0.004), glass);
  // Lưỡi dao hai bên yếm + đuôi.
  k.sym(plate(spikePts(0.85, -0.6, 4, y0 + 0.08, 0.26, 0.1, 0.5), 0.025, 0.005), second, [0.2, 0, 0], [0, 0, 0.25]);
  k.sym(plate([[-0.5, y0 + 0.5], [-0.75, y0 + 0.9], [-1.25, y0 + 0.96], [-0.95, y0 + 0.6]], 0.025, 0.005), second, [0.12, 0, 0], [0, 0, 0.22]);
  // Dải neon dọc thân và đèn pha khe.
  k.sym(tube([[0.205, y0 + 0.3, 0.95], [0.25, y0 + 0.34, 0.4], [0.23, y0 + 0.4, -0.2], [0.15, y0 + 0.58, -0.8], [0.05, y0 + 0.7, -1.1]], 0.012), g1);
  k.sym(tube([[0.2, y0 + 0.12, 0.6], [0.22, y0 + 0.08, -0.1]], 0.01), g2);
  k.put(plate([[1.1, y0 + 0.32], [1.0, y0 + 0.4], [0.86, y0 + 0.44], [0.9, y0 + 0.36]], 0.22, 0.004), glowMat(0xeaf6ff, 1.4));
  k.put(tube([[-0.06, y0 + 0.7, -1.12], [0.06, y0 + 0.7, -1.12]], 0.02), glowMat(0xff2a3a, 1.5));
  // Cánh kính phát sáng ở đuôi (hai bên) và một cánh nhỏ ở mũi.
  const fins = [];
  for (const [f, fm, g] of [[{ z: -0.55, x: 0.14, y: y0 + 0.62, base: 0.55, h: 0.6, sweep: 0.4, tilt: 0.55, prongs: 2 }, fin1, g1], [{ z: 0.85, x: 0.2, y: y0 + 0.36, base: 0.35, h: 0.3, sweep: 0.25, tilt: 0.9, prongs: 1 }, fin2, g2]]) {
    const { geo, tip } = finGeo(f.base, f.h, f.sweep, 0.55, f.prongs);
    k.sym(geo, fm, [f.x, f.y, f.z], [0, 0.05, -f.tilt]);
    k.sym(tube(tip[0].filter((_, i) => i % 2 === 0).map((p) => [0, p.y, p.x]), 0.009), g, [f.x, f.y, f.z], [0, 0.05, -f.tilt]);
    fins.push(fm);
  }
  // Càng và gắp tối màu (bánh không trục nên chỉ là tay đỡ cong).
  k.sym(plate([[WB / 2 + 0.05, y0 + 0.02], [WB / 2 - 0.25, y0 + 0.5], [WB / 2 - 0.4, y0 + 0.45], [WB / 2 - 0.08, y0 - 0.04]], 0.05, 0.006), second, [0.1, 0, 0]);
  k.sym(plate([[-WB / 2 - 0.05, y0 + 0.02], [-WB / 2 + 0.6, y0 + 0.2], [-WB / 2 + 0.6, y0 + 0.08], [-WB / 2 + 0.05, y0 - 0.06]], 0.05, 0.006), second, [0.12, 0, 0]);
  if (D.pearl) {
    const cm = new THREE.MeshPhysicalMaterial({ color: 0xbff4ff, roughness: 0.05, transmission: opt.low ? 0 : 0.6, thickness: 0.2, transparent: true, opacity: 0.85, emissive: new THREE.Color(0x3fd8ff), emissiveIntensity: 0.6, envMap: env });
    const cry = new THREE.OctahedronGeometry(0.07, 0);
    for (const [x, y, z, sc] of [[0.12, y0 + 0.74, -0.75, 1.2], [0.1, y0 + 0.7, -0.45, 0.9], [0.16, y0 + 0.5, 0.75, 0.8]]) k.sym(cry, cm, [x, y, z], [0.3, 0, 0.3], [sc * 0.7, sc * 1.8, sc * 0.7]);
  }
  k.build(group);
  // Bánh không trục: lốp xuyến + vành neon kép, quay quanh x.
  const wheels = [];
  const rubber = new THREE.MeshStandardMaterial({ color: 0x111217, roughness: 0.85 });
  for (const z of [WB / 2, -WB / 2]) {
    const w = new THREE.Group();
    w.position.set(0, R, z);
    const spin = new THREE.Group();
    const wk = new Kit();
    const tyre = new THREE.TorusGeometry(R * 0.8, R * 0.22, 10, 30);
    tyre.rotateY(Math.PI / 2);
    wk.put(tyre, rubber);
    for (const x of [-0.06, 0.06]) {
      const ring = new THREE.TorusGeometry(R * 0.6, R * 0.035, 6, 32);
      ring.rotateY(Math.PI / 2);
      wk.put(ring, g1, [x, 0, 0]);
    }
    const inner = new THREE.TorusGeometry(R * 0.5, R * 0.025, 6, 32);
    inner.rotateY(Math.PI / 2);
    wk.put(inner, g2);
    wk.build(spin);
    w.add(spin);
    group.add(w);
    wheels.push(spin);
  }
  return { group, wheels, seatY: y0 + 0.62, glows: [g1, fin1], glows2: [g2, fin2], aurora: !!D.aurora, accent, accent2: D.accent2 };
}

// addBikeKit gắn lên moto mô hình (hạng cao): dải neon hai bên, lưỡi dao sườn và cặp cánh kính nhỏ ở đuôi. h: cao thân.
export function addBikeKit(lean, h, accent, accent2) {
  const k = new Kit();
  const g1 = glowMat(accent, 1.6);
  const g2 = glowMat(accent2, 1.5);
  const carbon = new THREE.MeshStandardMaterial({ color: 0x15171f, metalness: 0.55, roughness: 0.35 });
  k.sym(tube([[0.17, h * 0.4, 0.75], [0.19, h * 0.42, 0.1], [0.15, h * 0.55, -0.7]], 0.011), g1);
  k.sym(plate(spikePts(0.6, -0.5, 3, h * 0.22, 0.2, 0.08, 0.5), 0.022, 0.005), carbon, [0.2, 0, 0], [0, 0, 0.2]);
  const { geo, tip } = finGeo(0.45, h * 0.42, 0.3, 0.55, 2);
  const fm = finMat(accent2, 0.85);
  k.sym(geo, fm, [0.12, h * 0.62, -0.5], [0, 0.05, -0.55]);
  k.sym(tube(tip[0].filter((_, i) => i % 2 === 0).map((p) => [0, p.y, p.x]), 0.009), g2, [0.12, h * 0.62, -0.5], [0, 0.05, -0.55]);
  const g = new THREE.Group();
  k.build(g);
  lean.add(g);
  return { glows: [g1] };
}
