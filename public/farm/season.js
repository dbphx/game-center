// Mùa trong nông trại: theme Noel (tuyết rơi, tuyết phủ mái / cây / hàng rào, cây thông, người tuyết, quà, dây đèn) và NPC
// luôn có trong vườn (Ông già Noel trong mùa Noel, Bác nông dân ngoài mùa).
// Chỉ là lớp phủ: không sửa farm.js — theo dõi #objects (vẽ lại cảnh) để đổi ảnh sang bản phủ tuyết và đặt đồ trang trí.
// Ép bật / tắt khi thử: ?season=noel | ?season=off.
(() => {
  const IMG = '/img/toon/farm/noel/';

  // Mùa Noel theo giờ Việt Nam (UTC+7): cả tháng 12, 1–6/1, và riêng năm 2026 bật sớm từ tháng 11.
  function noelActive(date = new Date()) {
    const vn = new Date(date.getTime() + 7 * 3600e3);
    const y = vn.getUTCFullYear(), m = vn.getUTCMonth() + 1, d = vn.getUTCDate();
    return m === 12 || (m === 1 && d <= 6) || (y === 2026 && m === 11);
  }
  const params = new URLSearchParams(location.search);
  const force = params.get('season');
  const NOEL = force === 'noel' ? true : force === 'off' ? false : noelActive();
  const VISIT = params.has('visit');
  window.noelActive = noelActive;
  window.FarmSeason = { noel: NOEL, noelActive };
  if (NOEL) document.body.classList.add('season-noel');

  // Kích thước hiển thị + điểm chân của ảnh dựng 3D (tools/render_noel → noel.json).
  const META = {
    tree_a: { w: 146, h: 194, foot: [73, 173] },
    snowman: { w: 84, h: 135, foot: [42, 121] },
    gift_red: { w: 48, h: 45, foot: [24, 34] },
    gift_green: { w: 38, h: 39, foot: [19, 29] },
    gift_blue: { w: 34, h: 44, foot: [17, 35] },
    wreath: { w: 61, h: 66, foot: [31, 56] },
    npc: { w: 99, h: 134, foot: [55, 118] },
  };

  // Ảnh cảnh có bản phủ tuyết (cùng kích thước ảnh gốc): đường dẫn gốc → tên file trong noel/.
  const SNOWY = {};
  for (const n of ['hut_rice', 'hut_seed', 'pen_shed', 'haystack', 'farm_gate', 'well', 'fence_side', 'fence_side_full']) SNOWY[`/img/toon/farm3d/${n}.png`] = n;
  for (const n of ['tree_big', 'tree_dao', 'tree_mai', 'tree_chuoi', 'tree_da', 'tree_khe', 'tree_dua', 'tree_tre', 'tree_xoai', 'fence_piece']) SNOWY[`/img/toon/farm/${n}.png`] = n;

  const $ = (id) => document.getElementById(id);
  const num = (el, prop) => parseFloat(el.style[prop]) || 0;

  function deco(cls, name, x, y, scale = 1, extra = '') {
    const m = META[name];
    const w = Math.round(m.w * scale), h = Math.round(m.h * scale);
    return `<span class="noel-deco ${cls}" style="left:${Math.round(x)}px;top:${Math.round(y)}px;z-index:${Math.round(y)};width:${w}px;height:${h}px;--fx:${Math.round(m.foot[0] * scale)}px;--fy:${Math.round(m.foot[1] * scale)}px">
      <img src="${IMG}${name}.png" alt="" draggable="false">${extra}</span>`;
  }

  // ---------- Đổi ảnh sang bản phủ tuyết ----------
  function snowify(root) {
    for (const img of root.querySelectorAll('img')) {
      const src = img.getAttribute('src') || '';
      const name = SNOWY[src.split('?')[0]];
      if (name) img.setAttribute('src', `${IMG}${name}.png`);
    }
  }

  // ---------- Đồ trang trí + NPC trong cảnh ----------
  // #season-layer gồm khối đồ trang trí (vẽ lại mỗi lần cảnh đổi) và NPC (giữ nguyên phần tử, chỉ dời chỗ — tháo NPC
  // khỏi trang giữa lúc chạm sẽ làm mất cú bấm).
  let layer = null, decor = null;
  function ensureLayer() {
    if (layer && layer.isConnected) return layer;
    layer = document.createElement('div');
    layer.id = 'season-layer';
    decor = document.createElement('div');
    decor.className = 'season-decor';
    layer.appendChild(decor);
    $('layer').appendChild(layer);
    return layer;
  }

  // NPC giữ nguyên phần tử giữa các lần vẽ lại (agent khác gắn sự kiện / class has-quest vào nó).
  let npc = null;
  function ensureNpc() {
    if (npc) return npc;
    const who = NOEL ? 'santa' : 'farmer';
    const label = NOEL ? 'Ông già Noel' : 'Bác nông dân';
    npc = document.createElement('div');
    npc.id = 'farm-npc';
    npc.className = 'farm-npc has-quest';
    npc.setAttribute('role', 'button');
    npc.setAttribute('tabindex', '0');
    npc.setAttribute('aria-label', label);
    npc.title = label;
    npc.dataset.npc = who;
    const m = META.npc, k = NPC_SCALE;
    npc.style.width = `${Math.round(m.w * k)}px`;
    npc.style.height = `${Math.round(m.h * k)}px`;
    npc.style.setProperty('--fx', `${Math.round(m.foot[0] * k)}px`);
    npc.style.setProperty('--fy', `${Math.round(m.foot[1] * k)}px`);
    npc.innerHTML = `<img class="npc-quest" src="${IMG}quest_mark.png" alt="" draggable="false">
      <img class="npc-img" src="${IMG}npc_${who}_0.png" alt="" draggable="false"><span class="npc-name">${label}</span>`;
    // Nạp sẵn các khung vẫy tay.
    for (const f of [1, 2]) new Image().src = `${IMG}npc_${who}_${f}.png`;
    const open = (e) => {
      e.preventDefault();
      e.stopPropagation();
      npc.classList.remove('bump');
      void npc.offsetWidth;
      npc.classList.add('bump');
      wave();
      document.dispatchEvent(new CustomEvent('farm:npc', { detail: { npc: who } }));
    };
    // Không để cú bấm lọt xuống cảnh (cảnh sẽ cho nhân vật đi tới / kéo cảnh). Cảnh chặn click mặc định khi chạm,
    // nên tự nhận "chạm" bằng pointerdown → pointerup tại chỗ (không kéo); click chỉ còn cho chuột / trình đọc màn hình.
    let down = null, lastOpen = 0;
    const stop = (e) => e.stopPropagation();
    for (const ev of ['mousedown', 'mouseup', 'touchstart', 'touchend']) npc.addEventListener(ev, stop, { passive: true });
    npc.addEventListener('pointerdown', (e) => { stop(e); down = { id: e.pointerId, x: e.clientX, y: e.clientY }; });
    npc.addEventListener('pointerup', (e) => {
      stop(e);
      if (down && down.id === e.pointerId && Math.hypot(e.clientX - down.x, e.clientY - down.y) < 12) { lastOpen = Date.now(); open(e); }
      down = null;
    });
    npc.addEventListener('click', (e) => { stop(e); if (Date.now() - lastOpen > 600) open(e); });
    npc.addEventListener('keydown', (e) => { if (e.key === 'Enter' || e.key === ' ') open(e); });
    // Đứng yên, thỉnh thoảng vẫy tay.
    const img = npc.querySelector('.npc-img');
    let waving = false;
    function wave() {
      if (waving || matchMedia('(prefers-reduced-motion: reduce)').matches) return;
      waving = true;
      const seq = [1, 2, 1, 2, 1, 0];
      seq.forEach((f, i) => setTimeout(() => {
        img.src = `${IMG}npc_${who}_${f}.png`;
        if (i === seq.length - 1) waving = false;
      }, i * 260));
    }
    setInterval(() => { if (!document.hidden) wave(); }, 7000);
    return npc;
  }
  const NPC_SCALE = 0.8;

  function place() {
    const objects = $('objects');
    const gate = objects && objects.querySelector('[data-obj="gate"]');
    if (!gate) return;
    if (NOEL) snowify(objects);
    const L = ensureLayer();
    const gx = num(gate, 'left'), gy = num(gate, 'top');
    const parts = [];
    // Hai cây đào hai bên cổng → cây thông (trái) và người tuyết (phải) trong mùa Noel.
    const front = [...objects.querySelectorAll('[data-tree="dao"]')].filter((t) => num(t, 'top') > gy - 40).sort((a, b) => num(a, 'left') - num(b, 'left'));
    const left = front[0], right = front[front.length - 1];
    if (NOEL) {
      const tx = left ? num(left, 'left') : gx - 142, ty = left ? num(left, 'top') : gy - 4;
      const sx = right && right !== left ? num(right, 'left') : gx + 138, sy = right && right !== left ? num(right, 'top') : gy - 4;
      for (const t of [left, right]) if (t) { t.classList.add('season-hidden'); t.dataset.tree = ''; }
      parts.push(deco('tree', 'tree_a', tx, ty, 0.85, `<img class="glow" src="${IMG}tree_b.png" alt="" draggable="false">`));
      parts.push(deco('gift', 'gift_red', tx - 46, ty + 8, 0.8));
      parts.push(deco('gift', 'gift_blue', tx + 44, ty + 6, 0.8));
      parts.push(deco('gift', 'gift_green', tx - 14, ty + 14, 0.8));
      parts.push(deco('snowman', 'snowman', sx, sy, 0.8));
      parts.push(deco('gift', 'gift_green', sx + 34, sy + 6, 0.7));
      // Vòng hoa trên mái cổng và trước hai kho.
      parts.push(deco('wreath', 'wreath', gx, gy - 94, 0.5).replace(/z-index:\d+/, `z-index:${Math.round(gy) - 17}`));
      for (const k of ['products', 'seeds']) {
        const b = objects.querySelector(`[data-obj="${k}"] img`);
        const o = objects.querySelector(`[data-obj="${k}"]`);
        if (!b || !o) continue;
        const ox = num(o, 'left'), oy = num(o, 'top');
        parts.push(deco('wreath', 'wreath', ox + (k === 'products' ? -6 : -2), oy - b.offsetHeight * 0.46, 0.42).replace(/z-index:\d+/, `z-index:${Math.round(oy) + 1}`));
      }
    }
    const html = parts.join('');
    if (decor.dataset.html !== html) { decor.innerHTML = html; decor.dataset.html = html; }
    // NPC: bên trái cổng, giữa rào chuồng và cây cạnh cổng — chỗ nhân vật hay đi qua, không che ô đất.
    if (!VISIT) {
      const n = ensureNpc();
      const nx = left ? num(left, 'left') - 100 : gx - 242;
      const ny = gy + 4;
      n.style.left = `${Math.round(nx)}px`;
      n.style.top = `${Math.round(ny)}px`;
      n.style.zIndex = Math.round(ny);
      if (n.parentElement !== L) L.appendChild(n);
    }
  }

  // ---------- Tuyết rơi: canvas vẽ ảnh bông tuyết PNG ----------
  const reduce = matchMedia('(prefers-reduced-motion: reduce)');
  const flakes = { dot: new Image(), star: new Image(), starS: new Image() };
  flakes.dot.src = `${IMG}flake_dot.png`;
  flakes.star.src = `${IMG}flake_star.png`;
  flakes.starS.src = `${IMG}flake_star_s.png`;
  let canvas = null, ctx = null, list = [], last = 0, raf = 0, cw = 0, ch = 0;
  function makeFlake(randomY) {
    const r = Math.random();
    // Ba lớp: xa (chấm nhỏ, chậm), giữa (chấm to), gần (tinh thể, nhanh, xoay).
    const layer = r < 0.5 ? 0 : r < 0.82 ? 1 : 2;
    const size = layer === 0 ? 4 + Math.random() * 4 : layer === 1 ? 8 + Math.random() * 5 : 13 + Math.random() * 9;
    return {
      layer, size,
      img: layer < 2 ? flakes.dot : size > 17 ? flakes.star : flakes.starS,
      x: Math.random() * cw, y: randomY ? Math.random() * ch : -20 - Math.random() * 60,
      vy: (layer === 0 ? 18 : layer === 1 ? 32 : 52) * (0.8 + Math.random() * 0.5),
      sway: 10 + Math.random() * 22, freq: 0.4 + Math.random() * 0.8, ph: Math.random() * 6.28,
      rot: Math.random() * 6.28, vr: (Math.random() - 0.5) * 1.2,
      alpha: layer === 0 ? 0.55 : layer === 1 ? 0.8 : 0.95,
    };
  }
  // Kích thước theo chính canvas (CSS phủ kín khung); khung có thể đang ẩn lúc gắn nên kiểm tra lại mỗi khung hình.
  function resize() {
    const dpr = Math.min(1.5, devicePixelRatio || 1);
    cw = canvas.clientWidth; ch = canvas.clientHeight;
    canvas.width = Math.round(cw * dpr); canvas.height = Math.round(ch * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    // Mật độ theo diện tích: điện thoại ít bông hơn.
    const want = cw && ch ? Math.round(Math.min(170, Math.max(36, (cw * ch) / 7500))) : 0;
    while (list.length < want) list.push(makeFlake(true));
    list.length = want;
  }
  function frame(t) {
    raf = requestAnimationFrame(frame);
    const dt = Math.min(0.05, (t - last) / 1000 || 0);
    last = t;
    if (!canvas.isConnected || document.hidden) return;
    if (canvas.clientWidth !== cw || canvas.clientHeight !== ch) resize();
    if (!cw || !ch) return;
    ctx.clearRect(0, 0, cw, ch);
    const time = t / 1000;
    for (const f of list) {
      f.y += f.vy * dt;
      f.rot += f.vr * dt;
      if (f.y > ch + 20) Object.assign(f, makeFlake(false));
      const x = f.x + Math.sin(time * f.freq + f.ph) * f.sway;
      ctx.globalAlpha = f.alpha;
      if (f.layer === 2) {
        ctx.save();
        ctx.translate(x, f.y);
        ctx.rotate(f.rot);
        ctx.drawImage(f.img, -f.size / 2, -f.size / 2, f.size, f.size);
        ctx.restore();
      } else ctx.drawImage(f.img, x - f.size / 2, f.y - f.size / 2, f.size, f.size);
    }
    ctx.globalAlpha = 1;
  }
  // Canvas nằm trong khung nông trại (dưới bảng điều khiển) hoặc trong màn tầng mây khi đang ở trên mây.
  function mountSnow() {
    if (!NOEL || reduce.matches) return;
    if (!canvas) {
      canvas = document.createElement('canvas');
      canvas.className = 'noel-snow';
      canvas.setAttribute('aria-hidden', 'true');
      ctx = canvas.getContext('2d');
    }
    const cloud = document.body.classList.contains('in-cloud') && $('cloud-scene');
    const host = cloud || $('stage');
    if (!host) return;
    if (canvas.parentElement !== host) {
      // Trong khung nông trại: đặt ngay sau cảnh để mọi bảng nổi (HUD, thanh công cụ) nằm trên tuyết.
      if (host.id === 'stage') $('scene').after(canvas); else host.appendChild(canvas);
      resize();
    }
    if (!raf) raf = requestAnimationFrame(frame);
  }

  // ---------- Tầng mây: cây thông nhỏ + quà trên bờ mây đáy ----------
  function cloudDeco() {
    if (!NOEL) return;
    const sky = document.querySelector('#cloud-scene .cloud-sky');
    if (!sky || sky.querySelector('.cloud-noel')) return;
    const box = document.createElement('div');
    box.className = 'cloud-noel';
    box.innerHTML = deco('gift', 'gift_red', 0, 0, 0.9) + deco('tree', 'tree_a', 0, 0, 0.7, `<img class="glow" src="${IMG}tree_b.png" alt="" draggable="false">`) +
      deco('gift', 'gift_blue', 0, 0, 0.9) + deco('snowman', 'snowman', 0, 0, 0.6);
    sky.appendChild(box);
  }

  // ---------- Theo dõi vẽ lại ----------
  let pending = false;
  function refresh() {
    if (pending) return;
    pending = true;
    requestAnimationFrame(() => {
      pending = false;
      place();
      mountSnow();
      cloudDeco();
    });
  }
  function watch() {
    const objects = $('objects');
    if (!objects) return;
    new MutationObserver(refresh).observe(objects, { childList: true });
    // Lên / xuống tầng mây (body.in-cloud) và màn mây vẽ lại.
    new MutationObserver(refresh).observe(document.body, { attributes: true, attributeFilter: ['class'] });
    const cs = () => {
      const el = $('cloud-scene');
      if (el && !el.dataset.seasonWatch) {
        el.dataset.seasonWatch = '1';
        new MutationObserver(refresh).observe(el, { childList: true });
      }
    };
    new MutationObserver(cs).observe(document.body, { childList: true });
    cs();
    refresh();
  }
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', watch);
  else watch();
})();
