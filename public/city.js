// Bản đồ DCity nhìn từ trên cao xuống: lưới phố, mỗi ô phố là một khu (game / tiện ích / trang trí).
// Nhân vật đi theo đường ngắn nhất trên mạng lưới đường giữa các khu.
// City.mount(el, { look, name, level, at, onArrive, onDepart, onMove }) vẽ bản đồ; City.walkTo(zone) cho nhân vật đi.
(function () {
  'use strict';
  const C = window.Character;

  const W = 1600;
  const H = 1000;
  // Hai trục ngang và hai trục dọc; mặt đường rộng 64, vỉa hè 14 mỗi bên.
  const ROADS_H = [330, 700];
  const ROADS_V = [520, 1080];
  const HALF = 32;
  const WALK = 14;
  // Phần vẽ thêm quanh bản đồ: màn hình khác tỉ lệ 16:10 vẫn thấy cỏ và đường nối tiếp, không bị viền trống.
  const PAD = 900;

  const ZONES = {
    plaza: { name: 'Quảng trường', sub: 'Nhân vật của bạn', node: 'plaza' },
    casino: { name: 'Casino', sub: 'Tiến Lên Miền Nam', node: 'casino', game: 'tienlen' },
    bank: { name: 'Ngân hàng', sub: 'Ví xu · Thưởng ngày', node: 'bank' },
    shop: { name: 'Shop thời trang', sub: 'Đổi trang phục', node: 'shop' },
    site: { name: 'Công trường', sub: 'Game mới sắp mở', node: 'site', soon: true },
    farm: { name: 'Nông trại', sub: 'Trồng trọt · Sắp mở', node: 'farm', soon: true },
  };

  // Mạng lưới đường: ngã tư, điểm trước cửa mỗi khu, và điểm nhân vật đứng trong khu.
  const NODES = {
    x11: [520, 330], x12: [1080, 330], x21: [520, 700], x22: [1080, 700],
    h1a: [240, 330], h1b: [800, 330], h1c: [1360, 330],
    h2a: [240, 700], h2b: [800, 700], h2c: [1360, 700],
    casino: [240, 262], bank: [800, 262], shop: [1360, 262],
    plaza: [800, 418], site: [240, 772], farm: [1360, 772],
  };
  const EDGES = [
    ['h1a', 'x11'], ['x11', 'h1b'], ['h1b', 'x12'], ['x12', 'h1c'],
    ['h2a', 'x21'], ['x21', 'h2b'], ['h2b', 'x22'], ['x22', 'h2c'],
    ['x11', 'x21'], ['x12', 'x22'],
    ['casino', 'h1a'], ['bank', 'h1b'], ['shop', 'h1c'], ['plaza', 'h1b'], ['site', 'h2a'], ['farm', 'h2c'],
  ];
  const SPEED = 300; // đơn vị bản đồ / giây
  const SPRITE_SCALE = 1.35;

  const dist = (a, b) => Math.hypot(NODES[a][0] - NODES[b][0], NODES[a][1] - NODES[b][1]);
  const adj = {};
  for (const [a, b] of EDGES) {
    (adj[a] = adj[a] || []).push(b);
    (adj[b] = adj[b] || []).push(a);
  }

  // Dijkstra theo khoảng cách thật: đi đường ngắn nhất trên lưới phố.
  function route(from, to) {
    const d = { [from]: 0 };
    const prev = {};
    const todo = new Set(Object.keys(NODES));
    while (todo.size) {
      let n = null;
      for (const m of todo) if (d[m] !== undefined && (n === null || d[m] < d[n])) n = m;
      if (n === null || n === to) break;
      todo.delete(n);
      for (const m of adj[n] || []) {
        const nd = d[n] + dist(n, m);
        if (d[m] === undefined || nd < d[m]) { d[m] = nd; prev[m] = n; }
      }
    }
    const path = [to];
    while (path[0] !== from && prev[path[0]]) path.unshift(prev[path[0]]);
    return path[0] === from ? path : [from];
  }

  // ---------- Hình vẽ dùng chung ----------

  const tree = (x, y, r = 22, c = '#3f9c4c') => `<g class="tree">
      <circle cx="${x + 7}" cy="${y + 8}" r="${r}" fill="rgba(0,0,0,.2)"/>
      <circle cx="${x}" cy="${y}" r="${r}" fill="${c}"/>
      <circle cx="${x - r * 0.3}" cy="${y - r * 0.3}" r="${r * 0.55}" fill="#5cbf68" opacity=".8"/>
      <circle cx="${x + r * 0.35}" cy="${y + r * 0.1}" r="${r * 0.35}" fill="#2f8a3d" opacity=".6"/></g>`;

  // Xe nhìn từ trên xuống, đầu xe hướng sang phải (+x).
  const car = (color) => `<g>
      <rect x="-18" y="-10" width="36" height="20" rx="6" fill="rgba(0,0,0,.25)" transform="translate(3 3)"/>
      <rect x="-18" y="-10" width="36" height="20" rx="6" fill="${color}"/>
      <rect x="2" y="-8" width="9" height="16" rx="3" fill="#cfe9ff"/><rect x="-14" y="-8" width="7" height="16" rx="2" fill="#a9c9e6"/>
      <rect x="-5" y="-9" width="6" height="18" rx="2" fill="rgba(255,255,255,.18)"/></g>`;

  const parked = (x, y, color, rot = 90) => `<g transform="translate(${x} ${y}) rotate(${rot})">${car(color)}</g>`;

  // Mái nhà dốc hai bên: hai nửa khác sắc độ và đường nóc ở giữa.
  const house = (x, y, w, h, c1, c2) => `<g>
      <rect x="${x + 8}" y="${y + 8}" width="${w}" height="${h}" fill="rgba(0,0,0,.22)"/>
      <rect x="${x}" y="${y}" width="${w}" height="${h / 2}" fill="${c1}"/>
      <rect x="${x}" y="${y + h / 2}" width="${w}" height="${h / 2}" fill="${c2}"/>
      <rect x="${x}" y="${y + h / 2 - 2}" width="${w}" height="4" fill="rgba(0,0,0,.25)"/>
      <rect x="${x + w - 26}" y="${y + 8}" width="12" height="12" fill="#8d6e63"/></g>`;

  // ---------- Nền, đường, xe ----------

  function defs() {
    return `<defs>
      <radialGradient id="water" cx=".4" cy=".35"><stop offset="0" stop-color="#a6ecff"/><stop offset="1" stop-color="#2fa6dc"/></radialGradient>
      <linearGradient id="gold" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#ffe082"/><stop offset="1" stop-color="#e0a800"/></linearGradient>
      <filter id="glow" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="4" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
      <filter id="shadow" x="-20%" y="-20%" width="150%" height="150%"><feDropShadow dx="10" dy="12" stdDeviation="4" flood-opacity=".35"/></filter>
      <pattern id="tiles" width="24" height="24" patternUnits="userSpaceOnUse"><rect width="24" height="24" fill="#e7ddc6"/><path d="M0 0H24V24" fill="none" stroke="#d6caae" stroke-width="2"/></pattern>
      <pattern id="grass" width="40" height="40" patternUnits="userSpaceOnUse"><rect width="40" height="40" fill="#7cc576"/><circle cx="8" cy="10" r="2" fill="#6db867"/><circle cx="28" cy="26" r="2" fill="#8bd184"/></pattern>
      <pattern id="dirt" width="30" height="30" patternUnits="userSpaceOnUse"><rect width="30" height="30" fill="#c9a874"/><circle cx="7" cy="9" r="2" fill="#b8955f"/><circle cx="22" cy="21" r="3" fill="#d5b886"/></pattern>
    </defs>`;
  }

  function roads() {
    const out = [];
    // Vỉa hè trước, mặt đường đè lên.
    const x0 = -PAD, y0 = -PAD, w = W + 2 * PAD, h = H + 2 * PAD;
    for (const y of ROADS_H) out.push(`<rect x="${x0}" y="${y - HALF - WALK}" width="${w}" height="${(HALF + WALK) * 2}" fill="#d4ccb9"/>`);
    for (const x of ROADS_V) out.push(`<rect x="${x - HALF - WALK}" y="${y0}" width="${(HALF + WALK) * 2}" height="${h}" fill="#d4ccb9"/>`);
    for (const y of ROADS_H) out.push(`<rect x="${x0}" y="${y - HALF}" width="${w}" height="${HALF * 2}" fill="#4b5563"/>`);
    for (const x of ROADS_V) out.push(`<rect x="${x - HALF}" y="${y0}" width="${HALF * 2}" height="${h}" fill="#4b5563"/>`);
    // Vạch giữa đường, chừa ngã tư.
    const gap = HALF + WALK + 6;
    for (const y of ROADS_H) {
      const xs = [-PAD, ...ROADS_V, W + PAD];
      for (let i = 0; i < xs.length - 1; i++) {
        const a = xs[i] + (i ? gap : 0);
        const b = xs[i + 1] - (i + 1 < xs.length - 1 ? gap : 0);
        out.push(`<line x1="${a}" y1="${y}" x2="${b}" y2="${y}" stroke="#f3e9c6" stroke-width="3" stroke-dasharray="26 20"/>`);
      }
    }
    for (const x of ROADS_V) {
      const ys = [-PAD, ...ROADS_H, H + PAD];
      for (let i = 0; i < ys.length - 1; i++) {
        const a = ys[i] + (i ? gap : 0);
        const b = ys[i + 1] - (i + 1 < ys.length - 1 ? gap : 0);
        out.push(`<line x1="${x}" y1="${a}" x2="${x}" y2="${b}" stroke="#f3e9c6" stroke-width="3" stroke-dasharray="26 20"/>`);
      }
    }
    // Vạch qua đường ở bốn phía mỗi ngã tư.
    const stripes = (horizontal, cx, cy) => [-24, -12, 0, 12, 24]
      .map((o) => horizontal
        ? `<rect x="${cx + o - 4}" y="${cy - 8}" width="8" height="16" rx="1"/>`
        : `<rect x="${cx - 8}" y="${cy + o - 4}" width="16" height="8" rx="1"/>`)
      .join('');
    for (const x of ROADS_V) for (const y of ROADS_H) {
      const off = HALF + 10;
      out.push(`<g fill="#f5f5f5" opacity=".9">${stripes(true, x, y - off)}${stripes(true, x, y + off)}${stripes(false, x - off, y)}${stripes(false, x + off, y)}</g>`);
    }
    return out.join('');
  }

  // Xe chạy vòng trên các làn đường.
  function traffic() {
    const lanes = [
      [`M-60 ${330 + 16} H${W + 60}`, 13, '#e5484d'], [`M${W + 60} ${330 - 16} H-60`, 17, '#4f8ef7'],
      [`M-60 ${700 + 16} H${W + 60}`, 15, '#f6c744'], [`M${W + 60} ${700 - 16} H-60`, 11, '#ffffff'],
      [`M${520 + 16} -60 V${H + 60}`, 12, '#7e57c2'], [`M${1080 - 16} ${H + 60} V-60`, 14, '#26a69a'],
    ];
    return lanes
      .map(([d, dur, color], i) => `<g>${car(color)}<animateMotion path="${d}" dur="${dur}s" begin="-${i * 2.3}s" repeatCount="indefinite" rotate="auto"/></g>`)
      .join('');
  }

  // ---------- Các khu ----------

  function casino() {
    const stalls = Array.from({ length: 6 }, (_, i) => `<line x1="384" y1="${30 + i * 40}" x2="470" y2="${30 + i * 40}" stroke="#fff" stroke-width="3"/>`).join('');
    const bulbs = Array.from({ length: 20 }, (_, i) => `<circle cx="${44 + i * 16.5}" cy="34" r="3.5" class="bulb b${i % 2}"/>`).join('') +
      Array.from({ length: 20 }, (_, i) => `<circle cx="${44 + i * 16.5}" cy="220" r="3.5" class="bulb b${(i + 1) % 2}"/>`).join('');
    return `<g class="zone" data-zone="casino">
      <rect x="0" y="0" width="474" height="284" fill="#3b424b"/>
      ${stalls}
      ${parked(427, 50, '#e5484d', 0)}${parked(427, 130, '#f4f4f4', 0)}${parked(427, 210, '#263445', 0)}
      <g filter="url(#shadow)">
        <rect x="30" y="22" width="340" height="208" rx="12" fill="#2c1150"/>
        <rect x="40" y="32" width="320" height="188" rx="8" fill="#4a2378" stroke="url(#gold)" stroke-width="4"/>
      </g>
      ${bulbs}
      <rect x="70" y="86" width="260" height="70" rx="14" fill="#1a0b30" stroke="#ff4fd8" stroke-width="4" class="neon-frame"/>
      <text x="200" y="135" text-anchor="middle" class="neon-text" font-size="44" font-weight="800" letter-spacing="7">CASINO</text>
      <g font-size="26" text-anchor="middle" font-weight="700">
        <text x="80" y="200" fill="#f6c744">♠</text><text x="140" y="200" fill="#ff6b8b">♥</text>
        <text x="260" y="200" fill="#ff6b8b">♦</text><text x="320" y="200" fill="#f6c744">♣</text>
      </g>
      <g fill="#8b7aa8"><rect x="60" y="48" width="26" height="20" rx="3"/><rect x="314" y="48" width="26" height="20" rx="3"/></g>
      <rect x="196" y="226" width="88" height="44" rx="6" fill="#c62828" stroke="url(#gold)" stroke-width="3"/>
      <g stroke="#f6c744" stroke-width="2" opacity=".8"><line x1="206" y1="236" x2="274" y2="236"/><line x1="206" y1="260" x2="274" y2="260"/></g>
      <rect x="226" y="270" width="28" height="14" fill="#c62828"/>
      ${tree(170, 256, 14, '#2e8b57')}${tree(310, 256, 14, '#2e8b57')}
      </g>`;
  }

  function bank() {
    const cols = Array.from({ length: 9 }, (_, i) => `<circle cx="${660 + i * 35}" cy="226" r="9" fill="#f7f3ea" stroke="#cfc4ab" stroke-width="2"/>`).join('');
    const ribs = Array.from({ length: 8 }, (_, i) => `<line x1="800" y1="120" x2="${800 + 52 * Math.cos((i * Math.PI) / 4)}" y2="${120 + 52 * Math.sin((i * Math.PI) / 4)}" stroke="#4b958a" stroke-width="2"/>`).join('');
    return `<g class="zone" data-zone="bank">
      <rect x="566" y="0" width="468" height="284" fill="url(#grass)"/>
      <rect x="772" y="236" width="56" height="48" fill="#e7ddc6"/>
      <g filter="url(#shadow)"><rect x="620" y="26" width="360" height="196" rx="6" fill="#e9e2d2" stroke="#cfc4ab" stroke-width="4"/></g>
      <rect x="644" y="48" width="312" height="150" rx="4" fill="#ddd4c0"/>
      <circle cx="800" cy="120" r="58" fill="url(#gold)"/>
      <circle cx="800" cy="120" r="52" fill="#5fb3a6"/>${ribs}
      <circle cx="800" cy="120" r="14" fill="url(#gold)"/>
      <text x="800" y="126" text-anchor="middle" font-size="13" font-weight="800" fill="#7a5600">xu</text>
      <text x="800" y="194" text-anchor="middle" font-size="15" font-weight="800" fill="#6d6250" letter-spacing="4">NGÂN HÀNG</text>
      ${cols}
      <g fill="#cfc4ab"><rect x="740" y="238" width="120" height="6"/><rect x="730" y="248" width="140" height="6"/></g>
      ${tree(600, 40, 18)}${tree(1000, 40, 18)}${tree(600, 250, 16)}${tree(1000, 250, 16)}
      </g>`;
  }

  function shop() {
    const awning = Array.from({ length: 12 }, (_, i) => `<path d="M${1180 + i * 30} 208 h30 v22 a15 10 0 0 1 -30 0 Z" fill="${i % 2 ? '#fff' : '#ff6f9f'}"/>`).join('');
    const pots = [1150, 1570].map((x) => `${tree(x, 60, 14, '#43a047')}${tree(x, 160, 14, '#43a047')}`).join('');
    return `<g class="zone" data-zone="shop">
      <rect x="1126" y="0" width="474" height="284" fill="#f3d9e2"/>
      <g fill="#ecc7d4">${Array.from({ length: 10 }, (_, i) => `<rect x="${1126 + i * 48}" y="0" width="24" height="284"/>`).join('')}</g>
      <g filter="url(#shadow)"><rect x="1180" y="34" width="360" height="176" rx="10" fill="#f8bbd0" stroke="#d94b85" stroke-width="4"/></g>
      <rect x="1250" y="80" width="220" height="56" rx="12" fill="#fff" stroke="#d94b85" stroke-width="3"/>
      <text x="1360" y="117" text-anchor="middle" font-size="24" font-weight="800" fill="#d94b85" letter-spacing="2">THỜI TRANG</text>
      <path d="M1340 172 L1360 156 L1380 172 Z M1360 156 v-6 a5 5 0 1 1 5 -5" fill="none" stroke="#d94b85" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>
      ${awning}
      <g fill="#c2577f"><rect x="1200" y="52" width="22" height="18" rx="3"/><rect x="1498" y="52" width="22" height="18" rx="3"/></g>
      ${pots}
      </g>`;
  }

  function plaza() {
    const beds = [[640, 420], [960, 420], [640, 610], [960, 610]]
      .map(([x, y]) => `<circle cx="${x}" cy="${y}" r="26" fill="#6db867"/>${[0, 1, 2, 3, 4, 5]
        .map((i) => `<circle cx="${x + 14 * Math.cos(i)}" cy="${y + 14 * Math.sin(i)}" r="5" fill="${['#ff6f9f', '#ffd54a', '#fff'][i % 3]}"/>`)
        .join('')}`)
      .join('');
    const benches = [[800, 395, 0], [800, 635, 0], [690, 515, 90], [910, 515, 90]]
      .map(([x, y, r]) => `<rect x="${x - 22}" y="${y - 6}" width="44" height="12" rx="3" fill="#8a5a3b" transform="rotate(${r} ${x} ${y})"/>`)
      .join('');
    return `<g class="zone" data-zone="plaza">
      <rect x="566" y="376" width="468" height="278" fill="url(#tiles)"/>
      <circle cx="800" cy="515" r="126" fill="#efe6d1" stroke="#cbbd9c" stroke-width="5"/>
      <g stroke="#d9cdb0" stroke-width="10"><line x1="566" y1="515" x2="1034" y2="515"/><line x1="800" y1="376" x2="800" y2="654"/></g>
      <circle cx="800" cy="515" r="70" fill="#b9b2a2"/>
      <circle cx="800" cy="515" r="60" fill="url(#water)"/>
      <circle cx="800" cy="515" r="44" fill="none" stroke="#e6faff" stroke-width="3" class="ripple"/>
      <circle cx="800" cy="515" r="44" fill="none" stroke="#e6faff" stroke-width="3" class="ripple r2"/>
      <circle cx="800" cy="515" r="13" fill="#e9e3d5" stroke="#b9b2a2" stroke-width="3"/>
      <circle cx="800" cy="515" r="5" fill="#bdefff" class="spray"/>
      ${benches}${beds}
      ${tree(600, 400, 18)}${tree(1000, 400, 18)}${tree(600, 630, 18)}${tree(1000, 630, 18)}
      </g>`;
  }

  function park() {
    return `<g class="park">
      <rect x="0" y="376" width="474" height="278" fill="url(#grass)"/>
      <path d="M0 470 Q120 420 230 480 T474 450" fill="none" stroke="#d9cba8" stroke-width="16"/>
      <path d="M150 560 Q170 500 250 505 Q340 510 340 560 Q340 615 250 618 Q160 620 150 560 Z" fill="url(#water)"/>
      <ellipse cx="230" cy="545" rx="26" ry="8" fill="#e6faff" opacity=".6" class="spray"/>
      <rect x="210" y="597" width="40" height="34" rx="4" fill="#a1785a"/>
      ${tree(60, 420, 24)}${tree(110, 600, 26)}${tree(400, 560, 24)}${tree(420, 410, 20)}${tree(60, 520, 18)}
      </g>`;
  }

  function homes() {
    const lots = [[1140, 390], [1300, 390], [1460, 390], [1140, 525], [1300, 525], [1460, 525]];
    const roofs = [['#c0574a', '#a8473b'], ['#5d7b9b', '#4c6783'], ['#c79a3f', '#ad8432'], ['#6c8f5a', '#5a7a4a'], ['#9b5d8d', '#834c76'], ['#b0703c', '#975f31']];
    return `<g class="homes">
      <rect x="1126" y="376" width="474" height="278" fill="url(#grass)"/>
      ${lots.map(([x, y], i) => `<rect x="${x}" y="${y}" width="128" height="116" rx="6" fill="#9ed49a" stroke="#86bf82" stroke-width="2"/>
        ${house(x + 12, y + 14, 74, 62, ...roofs[i])}
        <rect x="${x + 96}" y="${y + 60}" width="22" height="56" fill="#d0c8b4"/>`).join('')}
      <rect x="1180" y="606" width="38" height="24" rx="5" fill="url(#water)" stroke="#fff" stroke-width="3"/>
      <rect x="1500" y="470" width="38" height="24" rx="5" fill="url(#water)" stroke="#fff" stroke-width="3"/>
      </g>`;
  }

  function court() {
    return `<g class="court">
      <rect x="566" y="746" width="468" height="254" fill="url(#grass)"/>
      <rect x="610" y="770" width="300" height="180" rx="6" fill="#d97b3f"/>
      <g fill="none" stroke="#fff" stroke-width="3">
        <rect x="620" y="780" width="280" height="160"/><line x1="760" y1="780" x2="760" y2="940"/><circle cx="760" cy="860" r="24"/>
        <path d="M620 820 h40 v80 h-40 M900 820 h-40 v80 h40"/>
      </g>
      <circle cx="790" cy="840" r="6" fill="#e65100" class="ball"/>
      ${tree(960, 790, 22)}${tree(990, 900, 26)}${tree(940, 970, 18)}
      </g>`;
  }

  function site() {
    const slabs = Array.from({ length: 3 }, (_, r) => Array.from({ length: 3 }, (_, c) =>
      `<rect x="${60 + c * 70}" y="${800 + r * 58}" width="58" height="46" fill="#bdbdbd" stroke="#9e9e9e" stroke-width="2"/>`).join('')).join('');
    return `<g class="zone soon" data-zone="site">
      <rect x="0" y="746" width="474" height="254" fill="url(#dirt)"/>
      <rect x="40" y="784" width="230" height="190" fill="none" stroke="#e08a1e" stroke-width="5" stroke-dasharray="14 8"/>
      ${slabs}
      ${parked(360, 960, '#f2b233', 30)}
      <g transform="translate(400 880)">
        <rect x="-14" y="-14" width="28" height="28" fill="#f2b233" stroke="#7a5600" stroke-width="3"/>
        <g class="crane-arm">
          <rect x="-150" y="-6" width="230" height="12" fill="#f2b233" stroke="#7a5600" stroke-width="2"/>
          <rect x="50" y="-12" width="28" height="24" fill="#5d6670"/>
          <rect x="-150" y="-3" width="16" height="6" fill="#333"/>
          <animateTransform attributeName="transform" type="rotate" values="-25;35;-25" dur="18s" repeatCount="indefinite"/>
        </g>
      </g>
      <rect x="300" y="770" width="140" height="56" rx="8" fill="#ffcc33" stroke="#222" stroke-width="3"/>
      <text x="370" y="795" text-anchor="middle" font-size="16" font-weight="800" fill="#222">SẮP MỞ</text>
      <text x="370" y="815" text-anchor="middle" font-size="12" font-weight="700" fill="#222">Game mới</text>
      <g fill="#ff7a1a">${[30, 80, 130].map((x) => `<circle cx="${x}" cy="764" r="7"/>`).join('')}</g>
      </g>`;
  }

  function farm() {
    const rows = (x, y, w, h, n, soil, crop) => `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="6" fill="${soil}"/>` +
      Array.from({ length: n }, (_, i) => `<rect x="${x + 8}" y="${y + 10 + i * ((h - 20) / (n - 1)) - 4}" width="${w - 16}" height="8" rx="4" fill="${crop}"/>`).join('');
    const sheep = [[1500, 960], [1530, 942], [1555, 968]]
      .map(([x, y]) => `<g class="sheep"><ellipse cx="${x}" cy="${y}" rx="10" ry="7" fill="#fff"/><circle cx="${x + 9}" cy="${y}" r="4" fill="#333"/></g>`)
      .join('');
    return `<g class="zone soon" data-zone="farm">
      <rect x="1126" y="746" width="474" height="254" fill="#a5d36b"/>
      ${rows(1140, 800, 120, 180, 7, '#8d6e4a', '#66bb6a')}
      ${rows(1270, 800, 110, 180, 7, '#8d6e4a', '#f6c744')}
      ${rows(1390, 800, 80, 110, 5, '#7a5a3a', '#a1887f')}
      <g filter="url(#shadow)">
        <rect x="1480" y="770" width="100" height="70" fill="#c62828"/>
        <rect x="1480" y="803" width="100" height="5" fill="#8e1b1b"/>
      </g>
      <g filter="url(#shadow)"><circle cx="1452" cy="955" r="18" fill="#cfd8dc"/><circle cx="1452" cy="955" r="9" fill="#90a4ae"/></g>
      <ellipse cx="1520" cy="890" rx="44" ry="26" fill="url(#water)"/>
      <rect x="1478" y="925" width="112" height="66" rx="4" fill="none" stroke="#a8784d" stroke-width="4"/>
      ${sheep}
      <g transform="translate(1415 945)"><circle r="12" fill="#f1e3c8" stroke="#bfa57a" stroke-width="3"/>
        <g class="windmill">${[0, 90, 180, 270].map((a) => `<rect x="-4" y="-46" width="8" height="40" rx="2" fill="#fff" stroke="#bfa57a" stroke-width="2" transform="rotate(${a})"/>`).join('')}<circle r="5" fill="#8a5a3b"/></g>
      </g>
      </g>`;
  }

  function labels() {
    const pos = { casino: [200, 22], bank: [800, 22], shop: [1360, 22], plaza: [690, 600], site: [150, 776], farm: [1500, 720] };
    return Object.entries(ZONES)
      .map(([id, z]) => {
        const [x, y] = pos[id];
        const w = Math.max(z.name.length, z.sub.length) * 8.6 + 34;
        return `<g class="zone-label${z.soon ? ' soon' : ''}" data-zone="${id}" transform="translate(${x} ${y + 24})">
          <rect x="${-w / 2}" y="-24" width="${w}" height="46" rx="12"/>
          <text y="-4" text-anchor="middle" class="zl-name">${z.name}</text>
          <text y="13" text-anchor="middle" class="zl-sub" id="zl-sub-${id}">${z.sub}</text>
        </g>`;
      })
      .join('');
  }

  // ---------- Nhân vật ----------

  let svgEl = null;
  let me = null; // { node, x, y, angle, look, walking, el, rot, body }
  let opts = {};
  let anim = null;

  function placeMe() {
    me.el.setAttribute('transform', `translate(${me.x} ${me.y})`);
    me.rot.setAttribute('transform', `rotate(${me.angle}) scale(${SPRITE_SCALE})`);
  }

  function drawMe() {
    me.body.innerHTML = C.topdown(me.look);
    me.body.setAttribute('class', 'td-char' + (me.walking ? ' walking' : ''));
  }

  function setTag(name, level) {
    const tag = svgEl.querySelector('#me-tag');
    tag.querySelector('.tag-name').textContent = name;
    tag.querySelector('.tag-lv').textContent = 'Lv ' + level;
    const w = name.length * 8 + 62;
    tag.querySelector('rect').setAttribute('x', -w / 2);
    tag.querySelector('rect').setAttribute('width', w);
    tag.querySelector('.tag-lv-bg').setAttribute('x', w / 2 - 40);
    tag.querySelector('.tag-lv').setAttribute('x', w / 2 - 22);
    tag.querySelector('.tag-name').setAttribute('x', -18);
  }

  function walkTo(zone) {
    const z = ZONES[zone];
    if (!z || !me) return;
    cancelAnimationFrame(anim);
    // Đang đi giữa chừng: đi tiếp từ nút gần nhất.
    const nearest = Object.keys(NODES).reduce((best, n) =>
      Math.hypot(NODES[n][0] - me.x, NODES[n][1] - me.y) < Math.hypot(NODES[best][0] - me.x, NODES[best][1] - me.y) ? n : best);
    const path = route(nearest, z.node).map((n) => NODES[n]);
    if (Math.hypot(path[0][0] - me.x, path[0][1] - me.y) < 1) path.shift();
    svgEl.querySelectorAll('.zone, .zone-label').forEach((el) => el.classList.toggle('target', el.dataset.zone === zone));

    if (!path.length) return arrive(zone);
    if (opts.onDepart) opts.onDepart(zone);
    me.walking = true;
    drawMe();
    let last = performance.now();
    const step = (now) => {
      let left = (SPEED * (now - last)) / 1000;
      last = now;
      while (left > 0 && path.length) {
        const [tx, ty] = path[0];
        const dx = tx - me.x;
        const dy = ty - me.y;
        const len = Math.hypot(dx, dy);
        if (len > 0.5) me.angle = (Math.atan2(dx, -dy) * 180) / Math.PI; // 0° = hướng bắc
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
      placeMe();
      if (opts.onMove) opts.onMove(me.x, me.y);
      if (path.length) anim = requestAnimationFrame(step);
      else {
        me.walking = false;
        drawMe();
        arrive(zone);
      }
    };
    anim = requestAnimationFrame(step);
  }

  function arrive(zone) {
    me.node = ZONES[zone].node;
    try { localStorage.setItem('city.at', zone); } catch (e) { /* bỏ qua */ }
    if (opts.onArrive) opts.onArrive(zone);
  }

  function mount(el, o) {
    opts = o;
    el.innerHTML = `<svg class="city-svg" viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMid meet" role="img" aria-label="Bản đồ DCity nhìn từ trên cao">
      ${defs()}<rect x="${-PAD}" y="${-PAD}" width="${W + 2 * PAD}" height="${H + 2 * PAD}" fill="url(#grass)"/>
      ${[[-120, 120], [-200, 560], [-90, 880], [1720, 140], [1800, 520], [1700, 900], [300, -120], [1300, -150], [700, 1120], [1400, 1130]]
        .map(([x, y]) => tree(x, y, 30)).join('')}
      ${park()}${homes()}${court()}${roads()}${traffic()}
      ${plaza()}${casino()}${bank()}${shop()}${site()}${farm()}
      ${labels()}
      <g id="me">
        <g class="me-rot"><g class="td-char"></g></g>
        <g id="me-tag" class="me-tag" transform="translate(0 -40)">
          <rect y="-15" height="28" rx="14"/><text class="tag-name" y="5" text-anchor="middle"></text>
          <rect class="tag-lv-bg" y="-11" width="36" height="20" rx="10"/><text class="tag-lv" y="4" text-anchor="middle"></text>
        </g>
      </g>
    </svg>`;
    svgEl = el.querySelector('svg');
    const start = ZONES[o.at] ? o.at : 'plaza';
    const [x, y] = NODES[ZONES[start].node];
    me = {
      node: ZONES[start].node, x, y, angle: 180, look: o.look, walking: false,
      el: svgEl.querySelector('#me'), rot: svgEl.querySelector('#me .me-rot'), body: svgEl.querySelector('#me .td-char'),
    };
    drawMe();
    placeMe();
    setTag(o.name, o.level);
    svgEl.querySelectorAll('[data-zone]').forEach((z) => z.addEventListener('click', () => walkTo(z.dataset.zone)));
    return start;
  }

  function setLook(look) {
    me.look = look;
    drawMe();
  }

  function setSub(zone, text) {
    const el = svgEl && svgEl.querySelector('#zl-sub-' + zone);
    if (el) el.textContent = text;
  }

  // Toạ độ x của khu, để đặt bảng thông tin không che khu đó.
  const zoneX = (zone) => NODES[ZONES[zone].node][0];

  window.City = { ZONES, mount, walkTo, setLook, setTag, setSub, zoneX, position: () => me && [me.x, me.y] };
})();
