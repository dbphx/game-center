// Bản đồ thành phố DCity: mỗi khu là một game / tiện ích, nhân vật đi bộ theo đường phố giữa các khu.
// City.mount(el, { look, name, level, at, onArrive }) vẽ bản đồ; City.walkTo(zone) cho nhân vật đi tới.
(function () {
  'use strict';
  const C = window.Character;

  // Khu vực. node là điểm nhân vật đứng khi tới khu (trước cửa).
  const ZONES = {
    plaza: { name: 'Quảng trường', sub: 'Nhân vật của bạn', node: 'plaza' },
    casino: { name: 'Casino', sub: 'Tiến Lên Miền Nam', node: 'casino', game: 'tienlen' },
    shop: { name: 'Shop thời trang', sub: 'Đổi trang phục', node: 'shop' },
    bank: { name: 'Ngân hàng', sub: 'Ví xu · Thưởng ngày', node: 'bank' },
    site: { name: 'Công trường', sub: 'Game mới sắp mở', node: 'site', soon: true },
    farm: { name: 'Nông trại', sub: 'Trồng trọt · Sắp mở', node: 'farm', soon: true },
  };

  // Mạng lưới đường: các nút trên trục đường chính (y = 560) và lối vào từng khu.
  const ROAD_Y = 560;
  const NODES = {
    r260: [260, ROAD_Y], r650: [650, ROAD_Y], r800: [800, ROAD_Y], r990: [990, ROAD_Y], r1300: [1300, ROAD_Y], r1370: [1370, ROAD_Y],
    casino: [260, 508], shop: [650, 508], bank: [990, 508], site: [1370, 508],
    plaza: [800, 668], farm: [1300, 668],
  };
  const EDGES = [
    ['r260', 'r650'], ['r650', 'r800'], ['r800', 'r990'], ['r990', 'r1300'], ['r1300', 'r1370'],
    ['r260', 'casino'], ['r650', 'shop'], ['r990', 'bank'], ['r1370', 'site'], ['r800', 'plaza'], ['r1300', 'farm'],
  ];
  const SPEED = 320; // đơn vị bản đồ / giây
  const SPRITE_SCALE = 0.9;

  const adj = {};
  for (const [a, b] of EDGES) {
    (adj[a] = adj[a] || []).push(b);
    (adj[b] = adj[b] || []).push(a);
  }

  // Đường đi ngắn nhất (theo số đoạn) giữa hai nút.
  function route(from, to) {
    const prev = { [from]: null };
    const queue = [from];
    while (queue.length) {
      const n = queue.shift();
      if (n === to) break;
      for (const m of adj[n] || []) if (!(m in prev)) { prev[m] = n; queue.push(m); }
    }
    if (!(to in prev)) return [from];
    const path = [];
    for (let n = to; n !== null; n = prev[n]) path.unshift(n);
    return path;
  }

  // ---------- Vẽ bản đồ ----------

  const tree = (x, y, s = 1) => `<g transform="translate(${x} ${y}) scale(${s})">
      <ellipse cx="0" cy="34" rx="22" ry="6" fill="rgba(0,0,0,.18)"/>
      <rect x="-5" y="8" width="10" height="26" rx="3" fill="#7a5134"/>
      <circle cx="0" cy="-4" r="26" fill="#3f9c4c"/><circle cx="-12" cy="4" r="17" fill="#4caf57"/><circle cx="12" cy="2" r="18" fill="#47a652"/>
      <circle cx="-6" cy="-14" r="10" fill="#6cc874" opacity=".7"/></g>`;

  const lamp = (x) => `<g transform="translate(${x} 598)">
      <rect x="-2" y="-62" width="4" height="62" fill="#3b4652"/><rect x="-10" y="-66" width="20" height="6" rx="3" fill="#3b4652"/>
      <circle cx="0" cy="-58" r="6" class="lamp-light" fill="#ffe8a3"/></g>`;

  function ground() {
    return `
      <defs>
        <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#8fd3ff"/><stop offset="1" stop-color="#c9ecff"/></linearGradient>
        <radialGradient id="water" cx=".5" cy=".4"><stop offset="0" stop-color="#9be7ff"/><stop offset="1" stop-color="#3fb4e6"/></radialGradient>
        <linearGradient id="casino-wall" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#4a2378"/><stop offset="1" stop-color="#2c1150"/></linearGradient>
        <linearGradient id="gold" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ffe082"/><stop offset="1" stop-color="#e0a800"/></linearGradient>
        <filter id="glow" x="-50%" y="-50%" width="200%" height="200%"><feGaussianBlur stdDeviation="4" result="b"/><feMerge><feMergeNode in="b"/><feMergeNode in="SourceGraphic"/></feMerge></filter>
        <pattern id="tiles" width="28" height="28" patternUnits="userSpaceOnUse"><rect width="28" height="28" fill="#e6dcc4"/><path d="M0 0H28V28" fill="none" stroke="#d3c7aa" stroke-width="2"/></pattern>
      </defs>
      <rect width="1600" height="1000" fill="#7cc576"/>
      <rect width="1600" height="190" fill="url(#sky)"/>
      <g fill="#b7d9ee" opacity=".9">
        <rect x="40" y="90" width="70" height="100"/><rect x="120" y="60" width="50" height="130"/><rect x="520" y="80" width="90" height="110"/>
        <rect x="760" y="50" width="60" height="140"/><rect x="830" y="100" width="80" height="90"/><rect x="1180" y="70" width="70" height="120"/>
        <rect x="1440" y="90" width="110" height="100"/>
      </g>
      <g fill="#fff" opacity=".85"><ellipse cx="300" cy="60" rx="60" ry="18"/><ellipse cx="340" cy="50" rx="40" ry="16"/>
        <ellipse cx="1050" cy="45" rx="70" ry="18"/><ellipse cx="1100" cy="38" rx="40" ry="14"/></g>
      <rect y="186" width="1600" height="30" fill="#5fae62"/>
      <g fill="#8fd18c" opacity=".55"><ellipse cx="200" cy="700" rx="160" ry="40"/><ellipse cx="1450" cy="930" rx="200" ry="50"/><ellipse cx="600" cy="950" rx="180" ry="40"/></g>`;
  }

  function roads() {
    const v = (x, y1, y2) => `<rect x="${x - 28}" y="${y1}" width="56" height="${y2 - y1}" fill="#cfc6b1"/>`;
    return `
      <rect x="0" y="512" width="1600" height="96" fill="#d8cfba"/>
      <rect x="0" y="522" width="1600" height="76" fill="#4b5663"/>
      <line x1="0" y1="${ROAD_Y}" x2="1600" y2="${ROAD_Y}" stroke="#f6f1e1" stroke-width="4" stroke-dasharray="34 26"/>
      <rect x="0" y="520" width="1600" height="3" fill="#a59c86"/><rect x="0" y="597" width="1600" height="3" fill="#a59c86"/>
      ${v(800, 598, 660)}${v(1300, 598, 655)}
      <g fill="#fff" opacity=".85">${[740, 760, 780, 800, 820, 840].map((x) => `<rect x="${x - 6}" y="528" width="10" height="64" rx="2"/>`).join('')}</g>`;
  }

  function casino() {
    const bulbs = Array.from({ length: 17 }, (_, i) => `<circle cx="${82 + i * 22.5}" cy="232" r="4.5" class="bulb b${i % 2}"/>`).join('');
    const windows = [110, 175, 345, 410]
      .map((x) => `<path d="M${x - 22} 340 V300 a22 22 0 0 1 44 0 V340 Z" fill="#ffd27a" stroke="url(#gold)" stroke-width="3"/>`)
      .join('');
    return `<g class="zone" data-zone="casino">
      <rect x="70" y="236" width="380" height="270" rx="6" fill="url(#casino-wall)"/>
      <rect x="58" y="218" width="404" height="26" rx="8" fill="url(#gold)"/>
      ${bulbs}
      <rect x="138" y="150" width="244" height="62" rx="14" fill="#1a0b30" stroke="#ff4fd8" stroke-width="4" class="neon-frame"/>
      <text x="260" y="194" text-anchor="middle" class="neon-text" font-size="38" font-weight="800" letter-spacing="6">CASINO</text>
      ${windows}
      <g font-size="30" text-anchor="middle" font-weight="700">
        <text x="110" y="400" fill="#f6c744">♠</text><text x="410" y="400" fill="#ff6b8b">♥</text>
        <text x="110" y="460" fill="#ff6b8b">♦</text><text x="410" y="460" fill="#f6c744">♣</text>
      </g>
      <path d="M205 506 V420 a55 55 0 0 1 110 0 V506 Z" fill="#2a0f45" stroke="url(#gold)" stroke-width="5"/>
      <path d="M215 506 V422 a45 45 0 0 1 90 0 V506 Z" fill="#ffcf6a" opacity=".9" class="door-light"/>
      <rect x="230" y="506" width="60" height="20" fill="#c62828"/>
      </g>`;
  }

  function shop() {
    const stripes = Array.from({ length: 8 }, (_, i) => `<path d="M${530 + i * 30} 352 h30 l-4 34 a11 11 0 0 1 -22 0 Z" fill="${i % 2 ? '#fff' : '#ff6f9f'}"/>`).join('');
    return `<g class="zone" data-zone="shop">
      <rect x="540" y="300" width="220" height="206" rx="6" fill="#fde7ef"/>
      <rect x="530" y="290" width="240" height="22" rx="6" fill="#d94b85"/>
      <rect x="560" y="316" width="180" height="34" rx="8" fill="#fff" stroke="#d94b85" stroke-width="3"/>
      <text x="650" y="340" text-anchor="middle" font-size="17" font-weight="800" fill="#d94b85" letter-spacing="1">THỜI TRANG</text>
      ${stripes}
      <rect x="556" y="400" width="66" height="90" rx="4" fill="#bfe8ff" stroke="#c2577f" stroke-width="3"/>
      <rect x="678" y="400" width="66" height="90" rx="4" fill="#bfe8ff" stroke="#c2577f" stroke-width="3"/>
      <g transform="translate(570 404) scale(.42)">${C.group({ gender: 'female', outfit: 'aodai' })}</g>
      <g transform="translate(692 404) scale(.42)">${C.group({ gender: 'male', outfit: 'suit' })}</g>
      <rect x="625" y="420" width="50" height="86" rx="4" fill="#8c3a5d"/><circle cx="665" cy="465" r="3" fill="#f6c744"/>
      </g>`;
  }

  function bank() {
    const cols = [880, 935, 1045, 1100].map((x) => `<rect x="${x - 11}" y="352" width="22" height="130" fill="#f7f3ea" stroke="#d6cdb8" stroke-width="2"/>`).join('');
    return `<g class="zone" data-zone="bank">
      <rect x="850" y="340" width="280" height="166" fill="#ece5d6"/>
      <path d="M836 342 L990 248 L1144 342 Z" fill="#f4eee1" stroke="#cfc4ab" stroke-width="3"/>
      <circle cx="990" cy="304" r="22" fill="url(#gold)" stroke="#b98a00" stroke-width="3"/>
      <text x="990" y="312" text-anchor="middle" font-size="22" font-weight="800" fill="#7a5600">xu</text>
      <rect x="846" y="340" width="288" height="16" fill="#d9cfb8"/>
      <text x="990" y="353" text-anchor="middle" font-size="12" font-weight="800" fill="#6d6250" letter-spacing="3">NGÂN HÀNG</text>
      ${cols}
      <rect x="960" y="400" width="60" height="106" rx="4" fill="#6b4a2f"/><rect x="988" y="400" width="4" height="106" fill="#4f351f"/>
      <rect x="836" y="482" width="308" height="12" fill="#d9cfb8"/><rect x="826" y="494" width="328" height="12" fill="#cfc4ab"/>
      </g>`;
  }

  function site() {
    return `<g class="zone soon" data-zone="site">
      <rect x="1220" y="360" width="300" height="146" fill="#d9c7a3"/>
      <g stroke="#e08a1e" stroke-width="5" fill="none">
        <rect x="1240" y="300" width="120" height="206"/><path d="M1240 300 L1360 404 M1360 300 L1240 404 M1240 404 L1360 506 M1360 404 L1240 506"/>
      </g>
      <g stroke="#f2b233" stroke-width="7" stroke-linecap="round">
        <path d="M1450 506 V230"/><path d="M1380 236 H1560"/><path d="M1540 236 V300"/>
      </g>
      <rect x="1530" y="300" width="20" height="18" fill="#3b4652"/>
      <rect x="1412" y="230" width="22" height="22" fill="#3b4652"/>
      <rect x="1290" y="420" width="160" height="60" rx="8" fill="#ffcc33" stroke="#222" stroke-width="3"/>
      <text x="1370" y="448" text-anchor="middle" font-size="16" font-weight="800" fill="#222">SẮP MỞ</text>
      <text x="1370" y="468" text-anchor="middle" font-size="12" font-weight="700" fill="#222">Game mới</text>
      <g fill="#ff7a1a">${[1250, 1300, 1440, 1490].map((x) => `<path d="M${x} 506 l8 -24 l8 24 Z"/>`).join('')}</g>
      </g>`;
  }

  function plaza() {
    return `<g class="zone" data-zone="plaza">
      <circle cx="800" cy="800" r="150" fill="url(#tiles)" stroke="#c9bd9f" stroke-width="6"/>
      <circle cx="800" cy="800" r="62" fill="#b9b2a2"/>
      <circle cx="800" cy="800" r="52" fill="url(#water)"/>
      <circle cx="800" cy="800" r="40" fill="none" stroke="#e6faff" stroke-width="3" class="ripple"/>
      <circle cx="800" cy="800" r="40" fill="none" stroke="#e6faff" stroke-width="3" class="ripple r2"/>
      <rect x="792" y="752" width="16" height="44" rx="4" fill="#d9d2c2"/>
      <path d="M800 750 q-26 -6 -34 22 M800 750 q26 -6 34 22 M800 750 q0 -26 0 -30" stroke="#bdefff" stroke-width="4" fill="none" stroke-linecap="round" class="spray"/>
      ${[650, 950].map((x) => `<rect x="${x - 26}" y="900" width="52" height="10" rx="3" fill="#8a5a3b"/><rect x="${x - 22}" y="910" width="6" height="12" fill="#5d3c27"/><rect x="${x + 16}" y="910" width="6" height="12" fill="#5d3c27"/>`).join('')}
      </g>`;
  }

  function farm() {
    // Ruộng: các luống rau xanh và lúa vàng.
    const rows = (x, y, w, n, color, crop) =>
      Array.from({ length: n }, (_, i) => `<rect x="${x}" y="${y + i * 22}" width="${w}" height="14" rx="7" fill="${color}"/>` +
        Array.from({ length: Math.floor(w / 26) }, (_, j) => `<circle cx="${x + 13 + j * 26}" cy="${y + 7 + i * 22}" r="6" fill="${crop}"/>`).join('')).join('');
    const fence = (x1, x2, y) => `<g stroke="#a8784d" stroke-width="4"><line x1="${x1}" y1="${y}" x2="${x2}" y2="${y}"/><line x1="${x1}" y1="${y + 12}" x2="${x2}" y2="${y + 12}"/></g>` +
      Array.from({ length: Math.floor((x2 - x1) / 30) + 1 }, (_, i) => `<rect x="${x1 + i * 30 - 3}" y="${y - 8}" width="6" height="28" rx="2" fill="#c08f5f"/>`).join('');
    const chicken = (x, y) => `<g transform="translate(${x} ${y})" class="chicken"><ellipse rx="9" ry="7" fill="#fff"/><circle cx="7" cy="-6" r="5" fill="#fff"/><path d="M11 -6 l5 1 l-5 2 Z" fill="#f2a516"/><path d="M5 -11 q2 -4 4 0" fill="#e53935"/></g>`;
    return `<g class="zone soon" data-zone="farm">
      <rect x="1120" y="640" width="470" height="350" rx="16" fill="#9ccc65"/>
      <rect x="1140" y="760" width="200" height="210" rx="10" fill="#8d6e4a"/>
      ${rows(1150, 772, 180, 9, '#7a5a3a', '#66bb6a')}
      <rect x="1360" y="830" width="210" height="140" rx="10" fill="#8d6e4a"/>
      ${rows(1370, 842, 190, 6, '#7a5a3a', '#f6c744')}
      <!-- Chuồng đỏ và silo -->
      <rect x="1420" y="690" width="120" height="110" fill="#c62828"/>
      <path d="M1410 692 L1480 640 L1550 692 Z" fill="#8e1b1b"/>
      <rect x="1455" y="740" width="50" height="60" fill="#fff"/><path d="M1455 740 L1505 800 M1505 740 L1455 800" stroke="#c62828" stroke-width="5"/>
      <rect x="1468" y="704" width="24" height="20" fill="#fff"/>
      <rect x="1550" y="680" width="30" height="120" rx="15" fill="#cfd8dc"/><path d="M1550 690 a15 15 0 0 1 30 0" fill="#90a4ae"/>
      <!-- Cối xay gió -->
      <path d="M1185 750 L1197 640 L1213 640 L1225 750 Z" fill="#f1e3c8" stroke="#bfa57a" stroke-width="3"/>
      <rect x="1196" y="712" width="18" height="26" rx="3" fill="#8a5a3b"/>
      <g transform="translate(1205 642)"><g class="windmill">
        ${[0, 90, 180, 270].map((a) => `<rect x="-6" y="-62" width="12" height="58" rx="3" fill="#fff" stroke="#bfa57a" stroke-width="2" transform="rotate(${a})"/>`).join('')}
        <circle r="7" fill="#8a5a3b"/>
      </g></g>
      ${fence(1240, 1400, 700)}
      ${chicken(1270, 735)}${chicken(1305, 745)}${chicken(1350, 732)}
      </g>`;
  }

  function park() {
    return `<g class="park">
      <ellipse cx="330" cy="800" rx="250" ry="140" fill="#69b86b"/>
      <ellipse cx="300" cy="820" rx="110" ry="55" fill="url(#water)" opacity=".9"/>
      <ellipse cx="270" cy="810" rx="30" ry="8" fill="#e6faff" opacity=".6"/>
      ${tree(120, 700)}${tree(500, 700, 0.9)}${tree(470, 880, 1.1)}${tree(130, 900, 0.9)}
      </g>`;
  }

  function decorations() {
    return [40, 470, 1185, 1575].map((x) => tree(x, 470, 0.95)).join('') +
      tree(1080, 930, 0.9) + tree(620, 980, 0.8) +
      [140, 560, 1060, 1500].map(lamp).join('');
  }

  function labels() {
    const pos = {
      casino: [260, 130], shop: [650, 275], bank: [990, 228], site: [1370, 210], plaza: [1035, 905], farm: [1460, 612],
    };
    return Object.entries(ZONES)
      .map(([id, z]) => {
        const [x, y] = pos[id];
        const w = Math.max(z.name.length, z.sub.length) * 8.6 + 34;
        return `<g class="zone-label${z.soon ? ' soon' : ''}" data-zone="${id}" transform="translate(${x} ${y})">
          <rect x="${-w / 2}" y="-24" width="${w}" height="46" rx="12"/>
          <text y="-4" text-anchor="middle" class="zl-name">${z.name}</text>
          <text y="13" text-anchor="middle" class="zl-sub" id="zl-sub-${id}">${z.sub}</text>
        </g>`;
      })
      .join('');
  }

  // ---------- Nhân vật ----------

  let svgEl = null;
  let me = null; // { node, x, y, dir, look, el, inner, walking }
  let opts = {};
  let anim = null;

  function placeMe() {
    me.el.setAttribute('transform', `translate(${me.x} ${me.y})`);
    me.flip.setAttribute('transform', `scale(${me.dir} 1)`);
  }

  function drawMe() {
    me.inner.innerHTML = C.group(me.look);
    me.inner.setAttribute('class', 'character' + (me.walking ? ' walking' : ''));
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
    // Đang đi giữa hai nút: đi tiếp từ nút gần nhất.
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
      let dist = (SPEED * (now - last)) / 1000;
      last = now;
      while (dist > 0 && path.length) {
        const [tx, ty] = path[0];
        const dx = tx - me.x;
        const dy = ty - me.y;
        const len = Math.hypot(dx, dy);
        if (Math.abs(dx) > 0.5) me.dir = dx < 0 ? -1 : 1;
        if (len <= dist) {
          me.x = tx;
          me.y = ty;
          dist -= len;
          path.shift();
        } else {
          me.x += (dx / len) * dist;
          me.y += (dy / len) * dist;
          dist = 0;
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
    el.innerHTML = `<svg class="city-svg" viewBox="0 0 1600 1000" preserveAspectRatio="xMidYMid slice" role="img" aria-label="Bản đồ thành phố">
      ${ground()}${park()}${roads()}${plaza()}${farm()}${casino()}${shop()}${bank()}${site()}${decorations()}${labels()}
      <g id="me">
        <g id="me-tag" class="me-tag" transform="translate(0 -104)">
          <rect y="-15" height="28" rx="14"/><text class="tag-name" y="5" text-anchor="middle"></text>
          <rect class="tag-lv-bg" y="-11" width="36" height="20" rx="10"/><text class="tag-lv" y="4" text-anchor="middle"></text>
        </g>
        <g class="flip"><g transform="translate(${-32 * SPRITE_SCALE} ${-92 * SPRITE_SCALE}) scale(${SPRITE_SCALE})"><g class="character"></g></g></g>
      </g>
    </svg>`;
    svgEl = el.querySelector('svg');
    const start = ZONES[o.at] ? o.at : 'plaza';
    const [x, y] = NODES[ZONES[start].node];
    me = {
      node: ZONES[start].node, x, y, dir: 1, look: o.look, walking: false,
      el: svgEl.querySelector('#me'), flip: svgEl.querySelector('#me .flip'), inner: svgEl.querySelector('#me .character'),
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

  // Toạ độ x (trên bản đồ 1600 đơn vị) của khu, để đặt bảng thông tin không che khu đó.
  const zoneX = (zone) => NODES[ZONES[zone].node][0];

  window.City = { ZONES, mount, walkTo, setLook, setTag, setSub, zoneX, position: () => me && [me.x, me.y] };
})();
