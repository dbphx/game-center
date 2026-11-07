// Trường đua: vật lý lái xe kiểu arcade và hình học đường đua. Không phụ thuộc three.js.
// Toạ độ mét, mặt đất là mặt phẳng xz; rot là góc quanh trục y, hướng tiến = (sin rot, cos rot).

export const NITRO_BOOST = 1.3; // ×1,3 tốc độ tối đa
export const NITRO_MS = 2000; // nitro kéo dài 2 giây
const WALL_EXTRA = 2.2; // tường cách mép đường (m): phần giữa là viền / cỏ làm xe chậm lại
const OFFROAD_SLOW = 0.6; // ra ngoài mép đường thì tốc độ tối đa ×0,6
const DRIFT_CHARGE = 0.24; // thanh nitro nạp mỗi giây khi drift
const DRIFT_MIN_SPEED = 11;

// Track bọc dữ liệu JSON đường đua với các phép tìm điểm gần nhất trên đường tâm.
export class Track {
  constructor(data) {
    this.data = data;
    this.path = data.path.map((p) => ({ x: p.x, z: p.z }));
    this.width = data.width || 16;
    this.half = this.width / 2;
    this.wall = this.half + WALL_EXTRA;
    this.cps = data.checkpoints;
    this.laps = data.laps || 3;
    const n = this.path.length;
    // Độ dài cộng dồn để tính tiến độ trên vòng.
    this.cum = new Float64Array(n + 1);
    for (let i = 0; i < n; i++) {
      const a = this.path[i];
      const b = this.path[(i + 1) % n];
      this.cum[i + 1] = this.cum[i] + Math.hypot(b.x - a.x, b.z - a.z);
    }
    this.length = this.cum[n];
    // Chỉ số điểm đường gần mỗi điểm kiểm tra, dùng để tính thứ hạng giữa cuộc đua.
    this.cpIndex = this.cps.map((c) => this.nearest(c.x, c.z, -1).i);
  }

  // nearest trả đoạn đường tâm gần (x, z) nhất: i (đầu đoạn), t (0–1), lateral (lệch sang phải, m),
  // dir (hướng đoạn, rad). hint ≥ 0 thì chỉ tìm quanh chỗ cũ cho nhanh.
  nearest(x, z, hint = -1) {
    const n = this.path.length;
    let best = { d2: Infinity, i: 0, t: 0 };
    const scan = (i) => {
      const a = this.path[i];
      const b = this.path[(i + 1) % n];
      const dx = b.x - a.x;
      const dz = b.z - a.z;
      const l2 = dx * dx + dz * dz || 1;
      let t = ((x - a.x) * dx + (z - a.z) * dz) / l2;
      t = t < 0 ? 0 : t > 1 ? 1 : t;
      const px = a.x + dx * t;
      const pz = a.z + dz * t;
      const d2 = (x - px) ** 2 + (z - pz) ** 2;
      if (d2 < best.d2) best = { d2, i, t, px, pz, dx, dz };
    };
    if (hint >= 0) for (let k = -12; k <= 12; k++) scan((hint + k + n) % n);
    else for (let i = 0; i < n; i++) scan(i);
    const len = Math.hypot(best.dx, best.dz) || 1;
    // Pháp tuyến phải của hướng (sin, cos) là (cos, −sin).
    const rx = best.dz / len;
    const rz = -best.dx / len;
    const lateral = (x - best.px) * rx + (z - best.pz) * rz;
    return { i: best.i, t: best.t, px: best.px, pz: best.pz, lateral, rx, rz, dir: Math.atan2(best.dx, best.dz) };
  }

  // progress là quãng đường đã chạy trên vòng hiện tại (m) tại đoạn i, t.
  progress(i, t) {
    const a = this.cum[i];
    return a + (this.cum[i + 1] - a) * t;
  }

  pointAt(dist) {
    const n = this.path.length;
    let d = ((dist % this.length) + this.length) % this.length;
    let lo = 0;
    let hi = n;
    while (hi - lo > 1) {
      const m = (lo + hi) >> 1;
      if (this.cum[m] <= d) lo = m;
      else hi = m;
    }
    const a = this.path[lo];
    const b = this.path[(lo + 1) % n];
    const t = (d - this.cum[lo]) / (this.cum[lo + 1] - this.cum[lo] || 1);
    return { x: a.x + (b.x - a.x) * t, z: a.z + (b.z - a.z) * t, dir: Math.atan2(b.x - a.x, b.z - a.z) };
  }
}

// newCar tạo trạng thái xe ở vị trí xuất phát.
export function newCar(spawn, stats) {
  return {
    x: spawn.x, z: spawn.z, rot: spawn.rot,
    vx: 0, vz: 0, speed: 0,
    stats: { ...stats },
    nitro: 0, // thanh nitro 0–1
    boostMs: 0, // còn bao lâu nitro đang chạy
    drift: false,
    lap: 0, // số vòng đã xong
    cp: 0, // điểm kiểm tra kế tiếp phải qua
    seg: -1, // đoạn đường gần nhất (gợi ý tìm kiếm)
    lateral: 0,
    offroad: false,
    hitWall: 0, // lực va tường lần gần nhất (để hiện tia lửa), tự giảm
    finished: false,
    steerVis: 0,
  };
}

// step tiến xe một khoảng dt giây theo điều khiển input {throttle −1..1, steer −1..1, drift, nitro}.
// Trả các sự kiện: 'checkpoint', 'lap', 'finish', 'wall', 'nitro'.
export function step(car, input, track, dt) {
  const ev = [];
  const st = car.stats;
  const boosting = car.boostMs > 0;
  if (boosting) car.boostMs = Math.max(0, car.boostMs - dt * 1000);

  // Nitro: dùng khi thanh đầy.
  if (input.nitro && car.nitro >= 1 && !boosting) {
    car.nitro = 0;
    car.boostMs = NITRO_MS;
    ev.push('nitro');
  }

  let max = st.maxSpeed * (car.boostMs > 0 ? NITRO_BOOST : 1);
  if (car.offroad) max *= OFFROAD_SLOW;
  const accel = st.accel * (car.boostMs > 0 ? 1.6 : 1);
  const th = input.throttle || 0;
  if (th > 0) {
    car.speed += accel * th * dt * (1 - Math.max(0, car.speed) / (max * 1.05));
  } else if (th < 0) {
    if (car.speed > 0.5) car.speed -= accel * 2.2 * -th * dt; // phanh
    else car.speed = Math.max(-8, car.speed - accel * 0.6 * -th * dt); // lùi chậm
  } else {
    car.speed -= car.speed * 0.35 * dt; // ma sát lăn
  }
  if (car.speed > max) car.speed = Math.max(max, car.speed - accel * 1.5 * dt);

  // Lái: xe chậm thì quay ít, nhanh thì bớt nhạy; drift quay gắt hơn.
  const sp = Math.abs(car.speed);
  car.drift = !!input.drift && sp > DRIFT_MIN_SPEED && Math.abs(input.steer || 0) > 0.2;
  const steer = input.steer || 0;
  const turn = 2.3 * steer * Math.min(1, sp / 7) * (1 - 0.35 * Math.min(1, sp / st.maxSpeed)) * (car.drift ? 1.5 : 1);
  car.rot += turn * dt * Math.sign(car.speed || 1); // steer > 0 là rẽ trái
  car.steerVis += (steer - car.steerVis) * Math.min(1, dt * 10);

  // Vận tốc bám theo hướng mũi xe; độ bám thấp hoặc đang drift thì trượt nhiều hơn.
  const fx = Math.sin(car.rot);
  const fz = Math.cos(car.rot);
  const k = car.drift ? st.grip * 2.2 : st.grip * 11;
  const a = 1 - Math.exp(-k * dt);
  car.vx += (fx * car.speed - car.vx) * a;
  car.vz += (fz * car.speed - car.vz) * a;
  if (car.drift) {
    car.nitro = Math.min(1, car.nitro + DRIFT_CHARGE * dt);
    car.speed *= 1 - 0.08 * dt; // drift mất chút tốc độ
  }
  car.x += car.vx * dt;
  car.z += car.vz * dt;

  // Va tường và ra ngoài đường.
  const near = track.nearest(car.x, car.z, car.seg);
  car.seg = near.i;
  car.lateral = near.lateral;
  car.offroad = Math.abs(near.lateral) > track.half;
  if (Math.abs(near.lateral) > track.wall) {
    const side = Math.sign(near.lateral);
    const push = Math.abs(near.lateral) - track.wall;
    car.x -= near.rx * side * push;
    car.z -= near.rz * side * push;
    // Bỏ phần vận tốc đâm vào tường, nảy nhẹ ra.
    const vn = car.vx * near.rx + car.vz * near.rz;
    if (vn * side > 0) {
      car.vx -= near.rx * vn * 1.4;
      car.vz -= near.rz * vn * 1.4;
      const hit = Math.abs(vn);
      car.speed *= hit > 6 ? 0.72 : 0.9;
      car.hitWall = Math.max(car.hitWall, hit);
      if (hit > 2) ev.push('wall');
    }
  }
  car.hitWall = Math.max(0, car.hitWall - dt * 20);

  // Điểm kiểm tra theo thứ tự; cái cuối là vạch đích, qua nó là xong một vòng.
  if (!car.finished) {
    const c = track.cps[car.cp];
    if (c && (car.x - c.x) ** 2 + (car.z - c.z) ** 2 <= c.r * c.r) {
      car.cp++;
      ev.push('checkpoint');
      if (car.cp >= track.cps.length) {
        car.cp = 0;
        car.lap++;
        ev.push('lap');
        if (car.lap >= track.laps) {
          car.finished = true;
          ev.push('finish');
        }
      }
    }
  }
  return ev;
}

// raceDistance là tổng quãng đường đã chạy (để xếp hạng giữa cuộc đua): vòng × chiều dài + tiến độ trong vòng.
export function raceDistance(track, lap, cp, x, z, seg = -1) {
  const near = track.nearest(x, z, seg);
  let d = track.progress(near.i, near.t);
  // Trước khi qua điểm kiểm tra đầu tiên, xe đứng sau vạch đích: tiến độ gần cuối vòng → coi là âm.
  if (cp === 0 && d > track.length * 0.5) d -= track.length;
  return lap * track.length + d;
}
