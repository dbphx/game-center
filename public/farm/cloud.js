// Màn mây: khu vườn trên mây phủ toàn màn hình trên cảnh nông trại. Các tầng chồng nhau cuộn dọc (tầng 4 trên cùng),
// thân đậu thần chạy dọc mép trái; mỗi tầng là một dải mây có 6 chậu đứng trên mặt mây. Bấm chậu để mua, trồng, thu hoạch.
// farm.js gọi FarmCloud.init một lần rồi FarmCloud.open / update; mọi lời gọi API đi qua hàm call của farm.js.
(() => {
  const IMG = '/img/toon/farm/';
  let cfg = null;
  let cat = null;
  let farm = null;
  let root = null;
  let busy = false; // đang hỏi hoặc đang gọi API, chống bấm trùng
  let timer = 0;
  let sig = '';
  let picking = null; // {floor, slot} khi đang mở bảng chọn kiểu chậu

  const I = () => window.Icons;
  const isOpen = () => !!root && !root.classList.contains('hidden');
  const floors = () => (cat && cat.cloudFloors) || [];
  const info = (n) => floors()[n - 1];
  const potOf = (n, slot) => farm.cloud.pots.find((p) => p.floor === n && p.slot === slot);
  const potCount = (n) => farm.cloud.pots.filter((p) => p.floor === n).length;
  const pad = (n) => String(n).padStart(2, '0');
  // Kiểu dáng chậu: "default" là chậu riêng của tầng (pot_1..4), còn lại là pot_skin_<id>.png. Chỉ để đẹp.
  const skins = () => (cat && cat.potSkins) || [];
  const owned = (id) => {
    const s = skins().find((x) => x.id === id);
    return !!s && (s.price === 0 || ((farm.cloud && farm.cloud.skins) || []).includes(id));
  };
  const skinSrc = (n, id) => (!id || id === 'default' ? `${IMG}pot_${n}.png` : `${IMG}pot_skin_${id}.png`);

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

  // Mỗi chỗ đặt chậu: chậu (và cây) đứng trên mặt dải mây, nhãn trạng thái nằm trên thân mây ngay dưới chậu.
  // Chậu đã mua có thêm nút "Đổi kiểu" ngay dưới nhãn (nút riêng, không lồng trong nút chậu).
  function pot(st, act, n, slot, art, label, skinBtn) {
    const btn = skinBtn && skins().length
      ? `<button class="skin-btn" data-act="skin" data-floor="${n}" data-slot="${slot}" title="Đổi kiểu chậu">${I().svg('sparkles') || ''}Đổi kiểu</button>`
      : '';
    return `<div class="cloud-cell"><button class="cloud-slot ${st}" data-act="${act}" data-floor="${n}" data-slot="${slot}" ${st === 'locked' ? 'disabled' : ''}>
      <span class="cloud-art">${art}</span><span class="slot-label">${label}</span></button>${btn}</div>`;
  }

  function slotHTML(n, slot) {
    const f = info(n);
    const st = slotState(n, slot);
    const p = potOf(n, slot);
    const potImg = `<img class="cloud-pot" src="${skinSrc(n, p && p.skin)}" alt="Chậu" draggable="false">`;
    const name = f.crop.name;
    const coins = (v) => `<em>${I().svg('coins')}${cfg.xu(v)} xu</em>`;
    if (st === 'buy') return pot(st, 'buy', n, slot, potImg, `<b>Mua chậu</b>${coins(f.potPrice)}`);
    if (st === 'locked') return pot(st, 'buy', n, slot, potImg, `<b>Chưa có chậu</b>${coins(f.potPrice)}`);
    if (st === 'empty') return pot(st, 'plant', n, slot, potImg, `<b>Trồng</b>${coins(f.crop.seedPrice)}`, true);
    if (st === 'grow') {
      return pot(st, 'wait', n, slot, plantPic(f.crop.id, p) + potImg, `<b>${name}</b><em data-ready="${p.readyAt}">${clock(p.readyAt - cfg.now())}</em>`, true);
    }
    return pot(st, 'harvest', n, slot, plantPic(f.crop.id, p) + potImg, `<b>Thu hoạch</b><em>${name}</em>`, true);
  }

  // Chậu chia thành hai dải mây, mỗi dải 3 chậu: máy tính đặt hai dải cạnh nhau, điện thoại xếp chồng.
  function bands(items, inner) {
    const half = Math.ceil(items.length / 2);
    const html = [items.slice(0, half), items.slice(half)].map((g) => `<div class="cloud-band">${g.join('')}</div>`).join('');
    return inner ? html : `<div class="cloud-bands">${html}</div>`;
  }

  // Tầng chưa mở: 6 chậu mờ trên dải mây và một biển báo ở giữa.
  function shutHTML(n) {
    const c = farm.cloud;
    const f = info(n);
    const waiting = c.beanPlantedAt && n === c.floors + 1;
    const msg = waiting
      ? `Đậu thần đang vươn tới, còn <b data-ready="${c.beanReadyAt}">${clock(c.beanReadyAt - cfg.now())}</b>`
      : n === c.floors + 1
        ? (cfg.user().level < f.level
          ? `Cần Lv ${f.level} để trồng đậu thần mở tầng này`
          : `Trồng hạt đậu thần ở ô 49 (${cfg.xu(f.beanPrice)} xu) để mở`)
        : `Mở tầng ${n - 1} trước đã`;
    const pots = Array.from({ length: f.slots }, () =>
      `<div class="cloud-cell"><span class="cloud-slot locked"><span class="cloud-art"><img class="cloud-pot" src="${IMG}pot_${n}.png" alt="" draggable="false"></span><span class="slot-label"><b>Chưa mở</b></span></span></div>`);
    return `<div class="cloud-bands shut">${bands(pots, true)}<p class="shut-sign">${I().svg('lock')}<span>${msg}</span></p></div>`;
  }

  function floorHTML(n) {
    const f = info(n);
    const open = n <= farm.cloud.floors;
    const hours = Math.round(f.crop.growSeconds / 3600);
    const facts = `chậu ${cfg.xu(f.potPrice)} xu · hạt ${cfg.xu(f.crop.seedPrice)} · bán ${cfg.xu(f.crop.sellPrice)} · ${hours} giờ · ${f.crop.xp} XP`;
    const band = open ? bands(Array.from({ length: f.slots }, (_, i) => slotHTML(n, i))) : shutHTML(n);
    return `<section class="cloud-floor${open ? '' : ' shut'}" data-floor-sec="${n}">
      <h3 class="floor-sign"><span class="floor-no">Tầng ${n}</span><span class="floor-crop">${f.crop.name}</span><span class="floor-facts">${facts}</span></h3>
      ${band}</section>`;
  }

  // Bảng chọn kiểu chậu cho chậu đang chọn: kiểu đã có thì lắp ngay, chưa có thì hỏi rồi mua.
  function pickHTML() {
    if (!picking) return '';
    const { floor, slot } = picking;
    const p = potOf(floor, slot);
    if (!p) return '';
    const cur = p.skin || 'default';
    const cards = skins().map((s) => {
      const mine = owned(s.id);
      const using = s.id === cur;
      const tag = using ? '<em class="using">Đang dùng</em>' : mine ? '<em class="owned">Đã có</em>'
        : `<em>${I().svg('coins')}${cfg.xu(s.price)} xu</em>`;
      return `<button class="skin-card${using ? ' using' : ''}${mine ? '' : ' locked'}" data-act="pick" data-skin="${s.id}" ${using ? 'disabled' : ''}>
        <img src="${skinSrc(floor, s.id)}" alt="" draggable="false"><b>${s.name}</b>${tag}</button>`;
    }).join('');
    return `<div class="skin-pick" data-act="pick-close"><div class="skin-panel" role="dialog" aria-label="Chọn kiểu chậu">
      <div class="skin-head"><h3>Kiểu chậu · tầng ${floor}, chậu ${slot + 1}</h3><button class="skin-x" data-act="pick-close" aria-label="Đóng">${I().svg('x')}</button></div>
      <p class="skin-note">Chỉ để đẹp, không đổi cây, giá hay thời gian chín. Mua một lần dùng cho mọi chậu.</p>
      <div class="skin-grid">${cards}</div></div></div>`;
  }

  // Hiện báo zombie ở dưới đất để người chơi đang trên mây không bỏ lỡ.
  const zombieDown = () => !!farm.zombieUntil && farm.zombieUntil > cfg.now();

  function render() {
    const u = cfg.user();
    const list = floors().map((x) => x.floor).sort((a, b) => b - a);
    root.innerHTML = `
      <div class="cloud-head">
        <button class="cloud-down" data-act="down">${I().svg('undo')}Xuống đất</button>
        <h2>Tầng mây</h2>
        <span class="cloud-coins">${I().svg('coins')}${cfg.xu(u ? u.balance : 0)}</span>
      </div>
      ${zombieDown() ? '<p class="cloud-zombie">Zombie đang tấn công nông trại dưới đất!</p>' : ''}
      <div class="cloud-sky"><div class="cloud-stalk" aria-hidden="true"></div>
        <div class="cloud-floors">${list.map(floorHTML).join('')}</div>
        <div class="cloud-stalk front" aria-hidden="true"></div>
        <img class="stalk-crown" src="/img/toon/farm/beanstalk_crown.png" alt="" aria-hidden="true">
        <img class="stalk-root" src="/img/toon/farm/beanstalk_root.png" alt="" aria-hidden="true"></div>
      ${pickHTML()}`;
  }

  // Tầng nên cuộn tới khi mở màn: tầng thấp nhất có cây chín, rồi có chậu trống, rồi còn chỗ mua chậu.
  function focusFloor() {
    const open = floors().map((x) => x.floor).filter((n) => n <= farm.cloud.floors).sort((a, b) => a - b);
    for (const want of ['ripe', 'empty', 'buy']) {
      const n = open.find((k) => Array.from({ length: info(k).slots }, (_, i) => slotState(k, i)).includes(want));
      if (n) return n;
    }
    return open[0] || 1;
  }

  // Dấu trạng thái: chỉ vẽ lại khi có gì đổi, để lần tải định kỳ không làm mất focus của nút.
  function signature() {
    const pots = farm.cloud.pots.map((p) => `${p.floor}.${p.slot}.${p.plantedAt}.${p.skin}.${slotState(p.floor, p.slot)}`).join();
    const stages = farm.cloud.pots.filter((p) => p.plantedAt).map((p) => (cfg.now() - p.plantedAt) / Math.max(1, p.readyAt - p.plantedAt) < 0.5).join();
    const u = cfg.user();
    const pick = picking ? `${picking.floor}.${picking.slot}` : '';
    return [farm.cloud.floors, farm.cloud.beanReadyAt, pots, stages, u && u.balance, zombieDown(), (farm.cloud.skins || []).join(), pick].join('|');
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
    // Bấm nền tối quanh bảng chọn kiểu thì đóng bảng.
    if (e.target.classList && e.target.classList.contains('skin-pick')) {
      picking = null;
      return draw(true);
    }
    const t = e.target.closest('button');
    if (!t || t.disabled) return;
    const a = t.dataset.act;
    if (a === 'pick-close') {
      picking = null;
      return draw(true);
    }
    if (a === 'skin') {
      picking = { floor: +t.dataset.floor, slot: +t.dataset.slot };
      return draw(true);
    }
    if (a === 'pick') return pickSkin(t.dataset.skin);
    const floor = +t.dataset.floor;
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

  // Lắp kiểu đã có; kiểu chưa có thì hỏi xác nhận, mua rồi lắp luôn cho chậu đang chọn.
  function pickSkin(id) {
    if (!picking) return;
    const { floor, slot } = picking;
    const s = skins().find((x) => x.id === id);
    if (!s) return;
    return act(async () => {
      if (!owned(id)) {
        if (!enough(s.price)) return;
        if (!(await cfg.ask(`Mua kiểu chậu "${s.name}" với giá ${cfg.xu(s.price)} xu? Mua một lần dùng cho mọi chậu.`, { title: 'Mua kiểu chậu', ok: 'Mua' }))) return;
        const bought = await cfg.api('/api/farm/pot/skin/buy', { skin: id });
        if (!bought) return;
      }
      const res = await cfg.api('/api/farm/pot/skin/set', { floor, slot, skin: id });
      if (res) {
        picking = null;
        cfg.toast(`Đã đổi sang chậu ${s.name}`);
        draw(true);
      }
    });
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
    root.classList.remove('hidden');
    document.body.classList.add('in-cloud');
    draw(true);
    const sec = root.querySelector(`[data-floor-sec="${focusFloor()}"]`);
    if (sec) sec.scrollIntoView({ block: 'center' });
    clearInterval(timer);
    timer = setInterval(tick, 1000);
  }

  function close() {
    if (!root) return;
    picking = null;
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
