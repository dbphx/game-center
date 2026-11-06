// Màn mây: khu vườn trên mây phủ toàn màn hình trên cảnh nông trại. Đổi tầng bằng tab, bấm vào đảo mây để mua chậu, trồng, thu hoạch.
// farm.js gọi FarmCloud.init một lần rồi FarmCloud.open / update; mọi lời gọi API đi qua hàm call của farm.js.
(() => {
  const IMG = '/img/toon/farm/';
  let cfg = null;
  let cat = null;
  let farm = null;
  let root = null;
  let floor = 1;
  let busy = false; // đang hỏi hoặc đang gọi API, chống bấm trùng
  let timer = 0;
  let sig = '';

  const I = () => window.Icons;
  const isOpen = () => !!root && !root.classList.contains('hidden');
  const floors = () => (cat && cat.cloudFloors) || [];
  const info = (n) => floors()[n - 1];
  const potOf = (n, slot) => farm.cloud.pots.find((p) => p.floor === n && p.slot === slot);
  const potCount = (n) => farm.cloud.pots.filter((p) => p.floor === n).length;
  const pad = (n) => String(n).padStart(2, '0');

  function clock(sec) {
    const s = Math.max(0, Math.ceil(sec));
    return `${pad(Math.floor(s / 3600))}:${pad(Math.floor((s % 3600) / 60))}:${pad(s % 60)}`;
  }

  // Trạng thái một chỗ đặt chậu: buy, locked (phải mua chậu trước đó), empty, grow, ripe.
  function slotState(n, slot) {
    const p = potOf(n, slot);
    if (!p) return slot === potCount(n) ? 'buy' : 'locked';
    if (!p.plantedAt) return 'empty';
    return cfg.now() >= p.readyAt ? 'ripe' : 'grow';
  }

  function plantPic(id, p) {
    const pr = (cfg.now() - p.plantedAt) / Math.max(1, p.readyAt - p.plantedAt);
    const stage = pr >= 1 ? 'ripe' : pr < 0.5 ? '1' : '2';
    return `<img class="cloud-crop" src="${IMG}crop_${id}_${stage}.png" alt="" draggable="false">`;
  }

  // Mỗi chỗ đặt chậu là một đảo mây: ảnh mây (đổi mẫu, lật ngang cho đỡ giống nhau), chậu và cây đứng trên mặt mây,
  // nhãn trạng thái ở dưới. Đảo nhấp nhô lệch nhịp nhau (CSS), bấm vào đâu trên đảo cũng được.
  const ISLE_CLOUD = [1, 3, 2, 2, 1, 3];
  const ISLE_FLIP = [false, true, false, true, true, false];
  function isle(st, act, slot, art, label) {
    const k = slot % ISLE_CLOUD.length;
    const cloud = `<img class="isle-cloud${ISLE_FLIP[k] ? ' flip' : ''}" src="${IMG}cloud_${ISLE_CLOUD[k]}.png" alt="" draggable="false">`;
    return `<button class="cloud-isle ${st}" data-act="${act}" data-slot="${slot}" style="--i:${slot}" ${st === 'locked' ? 'disabled' : ''}>
      <span class="isle-bob"><span class="cloud-art">${art}</span>${cloud}</span>
      <span class="isle-label">${label}</span></button>`;
  }

  function slotHTML(n, slot) {
    const f = info(n);
    const st = slotState(n, slot);
    const p = potOf(n, slot);
    const pot = `<img class="cloud-pot" src="${IMG}pot_${n}.png" alt="Chậu" draggable="false">`;
    const name = f.crop.name;
    const coins = (v) => `<em>${I().svg('coins')}${cfg.xu(v)} xu</em>`;
    if (st === 'buy') return isle(st, 'buy', slot, pot, `<b>Mua chậu</b>${coins(f.potPrice)}`);
    if (st === 'locked') return isle(st, 'buy', slot, pot, `<b>Chưa có chậu</b>${coins(f.potPrice)}`);
    if (st === 'empty') return isle(st, 'plant', slot, pot, `<b>Trồng ${name}</b>${coins(f.crop.seedPrice)}`);
    if (st === 'grow') {
      return isle(st, 'wait', slot, plantPic(f.crop.id, p) + pot, `<b>${name}</b><em data-ready="${p.readyAt}">${clock(p.readyAt - cfg.now())}</em>`);
    }
    return isle(st, 'harvest', slot, plantPic(f.crop.id, p) + pot, `<b>Thu hoạch</b><em>${name}</em>`);
  }

  // Hiện báo zombie ở dưới đất để người chơi đang trên mây không bỏ lỡ.
  const zombieDown = () => !!farm.zombieUntil && farm.zombieUntil > cfg.now();

  function render() {
    const c = farm.cloud;
    const f = info(floor);
    const u = cfg.user();
    const tabs = floors().map((x) => {
      const open = x.floor <= c.floors;
      return `<button class="cloud-tab${x.floor === floor ? ' on' : ''}${open ? '' : ' shut'}" data-floor="${x.floor}">${open ? '' : I().svg('lock')}Tầng ${x.floor}</button>`;
    }).join('');
    const hours = Math.round(f.crop.growSeconds / 3600);
    const facts = `Chậu ${cfg.xu(f.potPrice)} xu · ${f.crop.name}: hạt ${cfg.xu(f.crop.seedPrice)} xu, bán ${cfg.xu(f.crop.sellPrice)} xu, chín sau ${hours} giờ, ${f.crop.xp} XP`;
    let body;
    if (floor > c.floors) {
      const waiting = c.beanPlantedAt && floor === c.floors + 1;
      body = `<div class="cloud-shut"><span class="cloud-art big"><img class="cloud-pot" src="${IMG}pot_${floor}.png" alt="" draggable="false"></span>
        <h3>Tầng ${floor} chưa mở</h3>
        <p>${waiting ? `Đậu thần đang vươn tới tầng này, còn <b data-ready="${c.beanReadyAt}">${clock(c.beanReadyAt - cfg.now())}</b>.` : floor === c.floors + 1 ? `Xuống ô 49 trồng hạt đậu thần giá ${cfg.xu(f.beanPrice)} xu để mở tầng này.` : `Mở tầng ${floor - 1} trước đã.`}</p></div>`;
    } else {
      body = `<div class="cloud-garden">${Array.from({ length: f.slots }, (_, i) => slotHTML(floor, i)).join('')}</div>`;
    }
    root.innerHTML = `
      <img class="cloud-puff a" src="${IMG}cloud_1.png" alt="" draggable="false">
      <img class="cloud-puff b" src="${IMG}cloud_2.png" alt="" draggable="false">
      <img class="cloud-puff c" src="${IMG}cloud_3.png" alt="" draggable="false">
      <div class="cloud-head">
        <button class="cloud-down" data-act="down">${I().svg('undo')}Xuống đất</button>
        <h2>Tầng mây</h2>
        <span class="cloud-coins">${I().svg('coins')}${cfg.xu(u ? u.balance : 0)}</span>
      </div>
      ${zombieDown() ? '<p class="cloud-zombie">Zombie đang tấn công nông trại dưới đất!</p>' : ''}
      <div class="cloud-tabs">${tabs}</div>
      <p class="cloud-facts">${facts}</p>
      ${body}`;
  }

  // Dấu trạng thái: chỉ vẽ lại khi có gì đổi, để lần tải định kỳ không làm mất focus của nút.
  function signature() {
    const pots = farm.cloud.pots.map((p) => `${p.floor}.${p.slot}.${p.plantedAt}.${slotState(p.floor, p.slot)}`).join();
    const stages = farm.cloud.pots.filter((p) => p.plantedAt).map((p) => (cfg.now() - p.plantedAt) / Math.max(1, p.readyAt - p.plantedAt) < 0.5).join();
    const u = cfg.user();
    return [floor, farm.cloud.floors, farm.cloud.beanReadyAt, pots, stages, u && u.balance, zombieDown()].join('|');
  }
  function draw(force) {
    if (!isOpen()) return;
    const s = signature();
    if (!force && s === sig) return;
    sig = s;
    const scroll = root.scrollTop;
    render();
    root.scrollTop = scroll;
  }

  function tick() {
    if (!isOpen()) return;
    root.querySelectorAll('[data-ready]').forEach((el) => {
      el.textContent = clock(+el.dataset.ready - cfg.now());
    });
    draw();
  }

  async function act(fn) {
    if (busy) return;
    busy = true;
    root.classList.add('busy');
    try {
      await fn();
    } finally {
      busy = false;
      if (root) root.classList.remove('busy');
    }
  }

  // Đủ xu mới hỏi xác nhận; hết xu thì báo ngay.
  const enough = (price) => {
    const u = cfg.user();
    if (u && u.balance >= price) return true;
    cfg.toast('không đủ xu');
    return false;
  };

  async function onClick(e) {
    const t = e.target.closest('button');
    if (!t || t.disabled) return;
    if (t.dataset.floor) {
      floor = +t.dataset.floor;
      return draw(true);
    }
    const a = t.dataset.act;
    const slot = +t.dataset.slot;
    const f = info(floor);
    if (a === 'down') return close(); // luôn xuống được, kể cả đang chờ API
    if (a === 'wait') {
      const p = potOf(floor, slot);
      return cfg.toast(`${f.crop.name} còn ${clock(p.readyAt - cfg.now())} nữa mới chín`);
    }
    if (a === 'buy') {
      return act(async () => {
        if (!enough(f.potPrice)) return;
        if (!(await cfg.ask(`Mua chậu tầng ${floor} với giá ${cfg.xu(f.potPrice)} xu? Chậu không bán lại được.`, { title: 'Mua chậu', ok: 'Mua chậu' }))) return;
        const res = await cfg.api('/api/farm/pot/buy', { floor });
        if (res) cfg.toast(`Đã mua chậu tầng ${floor}`);
      });
    }
    if (a === 'plant') {
      return act(async () => {
        if (!enough(f.crop.seedPrice)) return;
        const res = await cfg.api('/api/farm/pot/plant', { floor, slot });
        if (res) cfg.toast(`Đã trồng ${f.crop.name}, trừ ${cfg.xu(f.crop.seedPrice)} xu`);
      });
    }
    if (a === 'harvest') {
      return act(async () => {
        const res = await cfg.api('/api/farm/pot/harvest', { floor, slot });
        if (res) cfg.toast(`+1 ${f.crop.name} vào kho sản phẩm · +${f.crop.xp} XP`);
      });
    }
  }

  function open(f, catalog) {
    farm = f;
    cat = catalog;
    if (!root) {
      root = document.createElement('div');
      root.id = 'cloud-scene';
      root.className = 'hidden';
      root.addEventListener('click', onClick);
      document.body.appendChild(root);
    }
    floor = Math.min(floors().length, Math.max(1, f.cloud.floors));
    root.classList.remove('hidden');
    document.body.classList.add('in-cloud');
    draw(true);
    clearInterval(timer);
    timer = setInterval(tick, 1000);
  }

  function close() {
    if (!root) return;
    root.classList.add('hidden');
    document.body.classList.remove('in-cloud');
    clearInterval(timer);
    timer = 0;
  }

  function update(f) {
    farm = f;
    draw();
  }

  window.FarmCloud = { init: (c) => (cfg = c), open, close, update, isOpen };
})();
