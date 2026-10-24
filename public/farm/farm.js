// Nông trại DCity: ruộng ô đất, chuồng gia súc, ao cá, cửa hàng. Mọi luật do server quyết định
// (/api/farm/...), trang này chỉ hiển thị, đếm ngược và gửi thao tác.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const esc = A.escapeHtml;

  const IMG = '/img/toon/farm/';

  // ---------- Hình ----------
  // Ảnh hoạt hình CC0 trong public/img/toon/farm (xem manifest.json, CREDITS.csv) — phong cách game "Avatar".
  let sizes = {}; // tên file (không đuôi) → [w, h], đọc từ manifest.json

  const art = {
    // Ảnh theo chiều cao mong muốn (giữ tỉ lệ gốc). k = hệ số phóng so với kích thước gốc khi không truyền h.
    img(name, { h, k = 1, cls = '' } = {}) {
      const [w0, h0] = sizes[name] || [64, 64];
      const hh = h || Math.round(h0 * k);
      const ww = Math.round((w0 / h0) * hh);
      return `<img class="toon ${cls}" src="${IMG}${name}.png" width="${ww}" height="${hh}" alt="" draggable="false">`;
    },
    // Cây theo giai đoạn: 1 mầm, 2 đang lớn (3 = sắp chín, cùng hình 2 nhưng to hơn), 'ripe' chín.
    crop(id, stage) {
      if (stage === 'ripe') return art.img(`crop_${id}_ripe`, { cls: 'crop-img' });
      return art.img(`crop_${id}_${stage === 1 ? 1 : 2}`, { cls: `crop-img${stage === 3 ? ' big' : ''}` });
    },
    item: (id, h = 44) => art.img(`item_${id}`, { h }),
    animal: (kind, h = 56) => art.img(`animal_${kind}`, { h }),
    fish: (kind, h = 26) => art.img(`fish_${kind}`, { h }),
  };

  // ---------- Trạng thái ----------

  let user = null;
  let cat = null; // catalog
  let farm = null;
  let offset = 0; // giờ server − giờ máy (giây)
  let shopMode = { tab: 'crops', plot: null };
  let pollTimer = null;

  const nowS = () => Date.now() / 1000 + offset;
  const xu = A.xu;
  const find = (list, id) => (cat ? cat[list].find((x) => x.id === id) : null) || { id, name: id, units: 1 };

  function fmtDur(sec) {
    sec = Math.max(0, Math.round(sec));
    if (sec < 60) return `${sec} giây`;
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (!h) return `${m} phút`;
    return m ? `${h} giờ ${m} phút` : `${h} giờ`;
  }

  function countdown(sec) {
    sec = Math.max(0, Math.ceil(sec));
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    const s = sec % 60;
    const pad = (n) => String(n).padStart(2, '0');
    return h ? `${h}:${pad(m)}:${pad(s)}` : `${m}:${pad(s)}`;
  }

  // Tiến độ 0..1 của một thứ đang lớn.
  const progress = (start, ready) => (ready <= start ? 1 : Math.min(1, Math.max(0, (nowS() - start) / (ready - start))));

  async function call(path, body) {
    try {
      const data = await A.api(path, body === undefined ? {} : { method: 'POST', body });
      if (data.user) user = data.user;
      if (data.farm) setFarm(data.farm);
      renderHUD();
      return data;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      // Thiếu máu (403 "cần ít nhất 40 HP") hoặc đang nằm viện (423): chỉ đường về Bệnh viện.
      const hp = e.status === 423 || (e.status === 403 && /HP/.test(e.message));
      A.toast(hp ? `${e.message} — ghé Bệnh viện trên bản đồ để hồi máu` : e.message);
      if (hp) A.me().then((u) => u && ((user = u), renderHUD())).catch(() => {});
      return null;
    }
  }

  function setFarm(f) {
    farm = f;
    offset = f.now - Date.now() / 1000;
    render();
  }

  // ---------- Topbar ----------

  function renderHUD() {
    if (!user) return;
    $('balance').textContent = xu(user.balance);
    $('user-name').textContent = user.name;
    $('user-avatar').innerHTML = C.head({ gender: user.gender, outfit: user.outfit });
    const pct = Math.min(100, Math.round(((user.xp - user.levelXP) / Math.max(1, user.nextXP - user.levelXP)) * 100));
    $('level-badge').innerHTML = `<span class="lv">Lv ${user.level}</span><span class="xp-bar"><span style="width:${pct}%"></span></span>`;
    $('level-badge').title = `${user.xp - user.levelXP} / ${user.nextXP - user.levelXP} XP để lên level ${user.level + 1}`;
    $('farm-name').textContent = `Nông trại của ${user.name}`;
    if (user.maxHp) {
      const hpPct = Math.round((user.hp / user.maxHp) * 100);
      const sick = user.hospitalUntil && user.hospitalUntil > user.now;
      const pill = $('hp-pill');
      pill.className = `hp-pill${hpPct < 40 || sick ? ' low' : ''}`;
      pill.title = sick ? 'Đang nằm viện' : 'Làm nông cần ít nhất 40 HP';
      pill.innerHTML = `${I.svg('heart')}<span class="hp-bar"><span style="width:${hpPct}%"></span></span><b>${user.hp}/${user.maxHp}</b>`;
    }
    // Nhân vật đứng trên đường đất cạnh ruộng, nhãn tên + level như trong game.
    $('me').innerHTML = `<span class="me-tag"><span class="lv">Lv ${user.level}</span>${esc(user.name)}</span>
      ${C.sprite({ gender: user.gender, outfit: user.outfit }, { dir: 'down', size: 104 })}`;
  }

  // ---------- Ruộng ----------

  // Mỗi ô: nền đất (hoặc cỏ khi chưa mở), cây đứng trên ô, nhãn nhỏ phía dưới.
  function plotHTML(p) {
    if (!p.crop) {
      return `<div class="plot empty" data-plot="${p.index}" title="Gieo hạt">
        <span class="soil"></span><span class="tag-pill add">${I.svg('plus')}Gieo hạt</span></div>`;
    }
    const c = find('crops', p.crop);
    const pr = progress(p.plantedAt, p.readyAt);
    if (pr >= 1) {
      return `<div class="plot ripe" data-plot="${p.index}" title="Thu hoạch ${esc(c.name)}">
        <span class="soil"></span><span class="crop">${art.crop(p.crop, 'ripe')}</span>
        <span class="harvest-arrow">${I.svg('chevronDown')}</span>
        <span class="tag-pill go">Thu hoạch</span></div>`;
    }
    const stage = pr < 0.34 ? 1 : pr < 0.67 ? 2 : 3;
    return `<div class="plot growing" data-plot="${p.index}" data-start="${p.plantedAt}" data-ready="${p.readyAt}" data-stage="${stage}"
        title="${esc(c.name)}">
      <span class="soil wet"></span><span class="crop">${art.crop(p.crop, stage)}</span>
      <span class="tag-pill"><span data-left>${countdown(p.readyAt - nowS())}</span>
        <span class="bar"><span style="width:${Math.round(pr * 100)}%"></span></span></span></div>`;
  }

  function renderPlots() {
    const owned = farm.plots.length;
    const max = farm.maxPlots || 48;
    const parts = farm.plots.map(plotHTML);
    if (owned < max && farm.nextPlotPrice) {
      parts.push(`<div class="plot buy" data-buy-plot title="Mở thêm ô đất">
        <span class="soil grass"></span><span class="lock">${I.svg('lock')}</span>
        <span class="tag-pill price">${I.svg('coins')}${xu(farm.nextPlotPrice)}</span></div>`);
    }
    const left = max - owned - 1;
    if (left > 0) {
      parts.push(`<div class="plot locked" title="Mở lần lượt từng ô">
        <span class="soil grass"></span><span class="lock">${I.svg('lock')}</span>
        <span class="tag-pill">Còn ${left} ô</span></div>`);
    }
    $('plots').innerHTML = parts.join('');
    $('plot-count').textContent = `${owned}/${max} ô đất`;
    const ripe = farm.plots.filter((p) => p.crop && nowS() >= p.readyAt).length;
    $('btn-harvest-all').classList.toggle('hidden', ripe < 2);
    $('btn-harvest-all').querySelector('span').textContent = `Thu hoạch tất cả (${ripe})`;
  }

  // ---------- Chuồng và ao ----------

  // Vị trí ngẫu nhiên nhưng cố định theo id, để con vật không nhảy chỗ mỗi lần vẽ lại.
  function spot(id, i) {
    const r = (n) => ((Math.sin(id * 9301 + n * 49297) + 1) / 2);
    return { x: 10 + r(1) * 76, y: 30 + r(2) * 70, dx: (r(3) - 0.5) * 120, dy: (r(4) - 0.5) * 16, dur: 6 + r(5) * 8, delay: -r(6) * 8, i };
  }

  function critterHTML(item, kind, list, swim) {
    const info = find(list, kind);
    const pr = progress(item.boughtAt, item.readyAt);
    const s = spot(item.id, 0);
    const cls = `critter${swim ? ' fish-swim' : ''}${pr < 1 ? ' young' : ''}${s.dx < 0 ? ' flip' : ''}`;
    // Con nhỏ vẽ nhỏ hơn; bò / cừu vốn to nên giới hạn theo chiều cao.
    const h = swim ? (pr < 1 ? 20 : 28) : { cow: 70, sheep: 58, pig: 50, chicken: 42 }[kind] * (pr < 1 ? 0.7 : 1);
    const draw = swim ? art.fish(kind, h) : art.animal(kind, Math.round(h || 50));
    return `<button class="${cls}" data-${swim ? 'fish' : 'animal'}="${item.id}" title="${esc(info.name)}"
        style="left:${s.x}%;top:${s.y}%;--dx:${s.dx}px;--dy:${s.dy}px;--dur:${s.dur}s;--delay:${s.delay}s">
      ${draw}${pr >= 1 ? '<span class="ready">!</span>' : `<span class="mini-bar"><span style="width:${Math.round(pr * 100)}%"></span></span>`}
    </button>`;
  }

  function listHTML(items, list, swim) {
    if (!items.length) return `<ul class="critter-list"><li class="empty">Chưa có ${swim ? 'cá' : 'vật nuôi'} — mua ở cửa hàng</li></ul>`;
    return `<ul class="critter-list">${items
      .map((it) => {
        const info = find(list, it.kind);
        const ready = nowS() >= it.readyAt;
        const draw = swim ? art.fish(it.kind, 18) : art.animal(it.kind, 30);
        const action = ready
          ? `<button class="primary" data-sell-${swim ? 'fish' : 'animal'}="${it.id}">${I.svg('coins')}Bán ${info.sellPrice ? xu(info.sellPrice) : ''}</button>`
          : '';
        return `<li>${draw}<span class="grow"><span>${esc(info.name)}</span>
          <span class="t" data-left-at="${it.readyAt}">${ready ? 'Đã lớn' : countdown(it.readyAt - nowS())}</span></span>${action}</li>`;
      })
      .join('')}</ul>`;
  }

  function penHTML(kind) {
    const isBarn = kind === 'barn';
    const pen = isBarn ? farm.barn : farm.pond;
    const cfg = cat ? cat[kind] : { price: 0, level: 1 };
    const title = isBarn ? 'Chuồng gia súc' : 'Ao cá';
    const head = `<div class="section-head"><h2>${I.svg(isBarn ? 'landmark' : 'sparkles')}${title}</h2>`;
    if (!pen) {
      const locked = user.level < cfg.level;
      return `${head}</div><div class="pen-offer">
        <div class="offer-art">${isBarn ? art.img('barn', { h: 150 }) + art.img('hay', { h: 56 }) : art.img('pond', { h: 120 })}</div>
        <h3>${isBarn ? 'Chưa có chuồng' : 'Chưa có ao'}</h3>
        <p class="muted">${isBarn
          ? `Nuôi gà, heo, cừu, bò. Mỗi con chiếm chỗ theo đơn vị (gà, heo 1 · cừu, bò 2); chuồng chứa ${cfg.capacity || 5} đơn vị, mở rộng được.`
          : `Thả cá rô phi, cá trê, cá chép, cá Koi; mỗi loại chiếm số đơn vị khác nhau, ao chứa ${cfg.capacity || 5} đơn vị, mở rộng được.`}</p>
        <button class="primary" data-buy-pen="${kind}" ${locked ? 'disabled' : ''}>
          ${locked ? `${I.svg('lock')}Cần Lv ${cfg.level}` : `${I.svg('coins')}Mua ${isBarn ? 'chuồng' : 'ao'} ${xu(cfg.price)} xu`}</button></div>`;
    }
    const items = (isBarn ? pen.animals : pen.fish) || []; // server bỏ trường khi chuồng / ao trống
    const list = isBarn ? 'animals' : 'fish';
    const pct = Math.round((pen.used / Math.max(1, pen.capacity)) * 100);
    const expand = pen.nextExpandPrice
      ? `<button data-expand="${kind}" title="Thêm ${cfg.expandUnits || 5} đơn vị">${I.svg('plus')}+${cfg.expandUnits || 5} · ${xu(pen.nextExpandPrice)}</button>`
      : '<span class="tag">Tối đa</span>';
    // Cảnh chuồng: nhà kho phía sau, máng, rơm, hàng rào; cảnh ao: mặt nước ảnh ao.
    const scene = isBarn
      ? `<div class="pen yard">
          <span class="deco" style="left:10px;bottom:62%">${art.img('barn', { h: 150 })}</span>
          <span class="deco" style="right:12px;bottom:58%">${art.img('silo', { h: 130 })}</span>
          <span class="deco" style="right:120px;bottom:60%">${art.img('hay', { h: 52 })}</span>
          <span class="deco" style="left:44%;bottom:8px">${art.img('trough_feed', { h: 34 })}</span>
          <span class="deco" style="left:18%;bottom:8px">${art.img('trough_water', { h: 34 })}</span>
          <span class="fence-top"></span><span class="fence-bottom"></span>
          <div class="roam">${items.map((it) => critterHTML(it, it.kind, list, false)).join('')}</div>
        </div>`
      : `<div class="pen pond-scene">
          <span class="deco" style="left:8px;bottom:6px">${art.img('flowers', { h: 52 })}</span>
          <span class="deco" style="right:10px;bottom:6px">${art.img('bush_3', { h: 44 })}</span>
          <span class="deco" style="right:16px;top:14px">${art.img('tree_med', { h: 110 })}</span>
          <div class="pond-water">${art.img('pond', { cls: 'pond-img' })}
            <div class="swim">${items.map((it) => critterHTML(it, it.kind, list, true)).join('')}</div></div>
        </div>`;
    return `${head}<button data-open-shop="${list}">${I.svg('plus')}Mua ${isBarn ? 'con giống' : 'cá giống'}</button></div>
      <div class="cap"><span>${pen.used}/${pen.capacity} đơn vị</span><span class="cap-bar"><span style="width:${pct}%"></span></span>${expand}</div>
      ${scene}
      ${listHTML(items, list, !isBarn)}`;
  }

  function render() {
    if (!farm || !user) return;
    renderPlots();
    $('barn').innerHTML = penHTML('barn');
    $('pond').innerHTML = penHTML('pond');
    if ($('dlg-shop').open) renderShop();
  }

  // Mỗi giây: cập nhật đếm ngược, thanh tiến độ; vẽ lại khi có thứ vừa chín / lớn.
  function tick() {
    if (!farm) return;
    let changed = false;
    document.querySelectorAll('.plot.growing').forEach((el) => {
      const start = +el.dataset.start;
      const ready = +el.dataset.ready;
      const pr = progress(start, ready);
      const stage = pr < 0.34 ? 1 : pr < 0.67 ? 2 : 3;
      if (pr >= 1 || String(stage) !== el.dataset.stage) changed = true;
      el.querySelector('[data-left]').textContent = countdown(ready - nowS());
      el.querySelector('.bar > span').style.width = `${Math.round(pr * 100)}%`;
    });
    document.querySelectorAll('[data-left-at]').forEach((el) => {
      const left = +el.dataset.leftAt - nowS();
      if (left <= 0 && el.textContent !== 'Đã lớn') changed = true;
      else if (left > 0) el.textContent = countdown(left);
    });
    if (changed) render();
  }

  // ---------- Hiệu ứng ----------

  function floatGain(anchor, gained) {
    if (!gained) return;
    const r = anchor ? anchor.getBoundingClientRect() : { left: innerWidth / 2, top: innerHeight / 2, width: 0 };
    const el = document.createElement('span');
    el.className = 'gain';
    el.innerHTML = `+${xu(gained.coins || 0)} xu${gained.xp ? `<span class="xp">+${gained.xp} XP</span>` : ''}`;
    el.style.left = `${r.left + r.width / 2}px`;
    el.style.top = `${r.top}px`;
    document.body.appendChild(el);
    setTimeout(() => el.remove(), 1700);
  }

  // ---------- Cửa hàng ----------

  function openShop(tab, plot = null) {
    shopMode = { tab, plot };
    renderShop();
    if (!$('dlg-shop').open) $('dlg-shop').showModal();
  }

  function metaLine(parts) {
    return `<span class="meta">${parts.filter(Boolean).map(([icon, text]) => `<span>${I.svg(icon)}${text}</span>`).join('')}</span>`;
  }

  function renderShop() {
    const { tab, plot } = shopMode;
    document.querySelectorAll('#shop-tabs .tab').forEach((b) => b.classList.toggle('active', b.dataset.tab === tab));
    $('shop-tabs').classList.toggle('hidden', plot !== null);
    $('shop-title').textContent = plot !== null ? `Gieo hạt vào ô ${plot + 1}` : 'Cửa hàng';
    if (!cat) {
      $('shop-list').innerHTML = '<p class="muted">Đang tải cửa hàng...</p>';
      return;
    }
    const rows = [];
    if (tab === 'crops') {
      const empty = farm.plots.filter((p) => !p.crop).length;
      $('shop-note').textContent = plot !== null ? 'Chọn hạt giống để gieo.' : `Bạn có ${empty} ô trống. Bấm "Gieo" để gieo vào ô trống đầu tiên.`;
      for (const c of cat.crops) {
        const locked = user.level < c.level;
        const poor = user.balance < c.seedPrice;
        const btn = locked
          ? `<button disabled>${I.svg('lock')}Cần Lv ${c.level}</button>`
          : `<button class="primary" data-plant="${c.id}" ${poor || (plot === null && !empty) ? 'disabled' : ''}>${I.svg('sprout')}Gieo · ${xu(c.seedPrice)}</button>`;
        rows.push(`<div class="shop-row${locked ? ' locked' : ''}"><span class="thumb">${art.item(c.id, 40)}</span>
          <div><b>${esc(c.name)}</b>${metaLine([['timer', fmtDur(c.growSeconds)], ['coins', `bán ${xu(c.sellPrice)}`], ['trophy', `+${c.xp} XP`], ['sparkles', `Lv ${c.level}`]])}</div>${btn}</div>`);
      }
    } else {
      const isBarn = tab === 'animals';
      const kind = isBarn ? 'barn' : 'pond';
      const pen = farm[kind];
      const cfg = cat[kind];
      $('shop-note').textContent = pen
        ? `${isBarn ? 'Chuồng' : 'Ao'} đang dùng ${pen.used}/${pen.capacity} đơn vị.`
        : `Cần mua ${isBarn ? 'chuồng' : 'ao'} trước.`;
      // Dòng đầu: mua chuồng / ao, hoặc mở rộng.
      if (!pen) {
        const locked = user.level < cfg.level;
        rows.push(`<div class="shop-row feature${locked ? ' locked' : ''}"><span class="thumb">${isBarn ? art.img('barn', { h: 46 }) : art.img('pond', { h: 34 })}</span>
          <div><b>${isBarn ? 'Chuồng gia súc' : 'Ao cá'}</b>${metaLine([['users', `chứa ${cfg.capacity} đơn vị`], ['plus', `mở rộng tới ${cfg.maxCapacity}`], ['sparkles', `Lv ${cfg.level}`]])}</div>
          ${locked ? `<button disabled>${I.svg('lock')}Cần Lv ${cfg.level}</button>` : `<button class="primary" data-buy-pen="${kind}">${I.svg('coins')}${xu(cfg.price)}</button>`}</div>`);
      } else if (pen.nextExpandPrice) {
        rows.push(`<div class="shop-row feature"><span class="thumb">${isBarn ? art.img('fence_piece', { h: 34 }) : art.img('pond', { h: 34 })}</span>
          <div><b>Mở rộng ${isBarn ? 'chuồng' : 'ao'} +${cfg.expandUnits}</b>${metaLine([['users', `${pen.capacity} → ${pen.capacity + cfg.expandUnits} đơn vị`], ['layers', `tối đa ${pen.maxCapacity}`]])}</div>
          <button class="primary" data-expand="${kind}">${I.svg('coins')}${xu(pen.nextExpandPrice)}</button></div>`);
      }
      for (const a of cat[isBarn ? 'animals' : 'fish']) {
        const locked = user.level < a.level;
        const full = pen && pen.used + a.units > pen.capacity;
        let btn;
        if (locked) btn = `<button disabled>${I.svg('lock')}Cần Lv ${a.level}</button>`;
        else if (!pen) btn = `<button disabled>Cần ${isBarn ? 'chuồng' : 'ao'}</button>`;
        else if (full) btn = `<button disabled>Hết chỗ</button>`;
        else btn = `<button class="primary" data-buy-${isBarn ? 'animal' : 'fish'}="${a.id}" ${user.balance < a.price ? 'disabled' : ''}>${I.svg('coins')}${xu(a.price)}</button>`;
        rows.push(`<div class="shop-row${locked ? ' locked' : ''}"><span class="thumb${isBarn ? '' : ' water'}">${isBarn ? art.animal(a.id, 40) : art.fish(a.id, 24)}</span>
          <div><b>${esc(a.name)}</b>${metaLine([['users', `${a.units} đơn vị`], ['timer', fmtDur(a.growSeconds)], ['coins', `bán ${xu(a.sellPrice)}`], ['trophy', `+${a.xp} XP`], ['sparkles', `Lv ${a.level}`]])}</div>${btn}</div>`);
      }
    }
    $('shop-list').innerHTML = rows.join('');
  }

  // ---------- Thao tác ----------

  async function plant(cropId) {
    let plot = shopMode.plot;
    if (plot === null) {
      const empty = farm.plots.find((p) => !p.crop);
      if (!empty) return A.toast('Hết ô trống');
      plot = empty.index;
    }
    const res = await call('/api/farm/plant', { plot, crop: cropId });
    if (res && shopMode.plot !== null) $('dlg-shop').close();
  }

  async function onClick(e) {
    const t = e.target.closest('button, .plot');
    if (!t) return;
    const d = t.dataset;
    if (t.classList.contains('plot')) {
      if (t.classList.contains('empty')) return openShop('crops', +d.plot);
      if (t.classList.contains('ripe')) {
        const res = await call('/api/farm/harvest', { plot: +d.plot });
        if (res) floatGain(t, res.gained);
        return;
      }
      if (t.classList.contains('growing')) return A.toast(`Còn ${fmtDur(+d.ready - nowS())} nữa mới chín`);
      if (d.buyPlot !== undefined) {
        if (!confirm(`Mở thêm ô đất với giá ${xu(farm.nextPlotPrice)} xu?`)) return;
        await call('/api/farm/plots/buy', {});
      }
      return;
    }
    if (d.plant) return plant(d.plant);
    if (d.buyPen) {
      const cfg = cat[d.buyPen];
      if (!confirm(`Mua ${d.buyPen === 'barn' ? 'chuồng gia súc' : 'ao cá'} với giá ${xu(cfg.price)} xu?`)) return;
      return call(`/api/farm/${d.buyPen}/buy`, {});
    }
    if (d.expand) {
      const pen = farm[d.expand];
      if (!confirm(`Mở rộng thêm ${cat[d.expand].expandUnits} đơn vị với giá ${xu(pen.nextExpandPrice)} xu?`)) return;
      return call(`/api/farm/${d.expand}/expand`, {});
    }
    if (d.buyAnimal) return call('/api/farm/animals/buy', { kind: d.buyAnimal });
    if (d.buyFish) return call('/api/farm/fish/buy', { kind: d.buyFish });
    if (d.sellAnimal || d.sellFish) {
      const isFish = !!d.sellFish;
      const res = await call(`/api/farm/${isFish ? 'fish' : 'animals'}/sell`, { id: +(d.sellFish || d.sellAnimal) });
      if (res) floatGain(t, res.gained);
      return;
    }
    if (d.animal || d.fish) {
      const isFish = !!d.fish;
      const id = +(d.fish || d.animal);
      const items = (isFish ? farm.pond && farm.pond.fish : farm.barn && farm.barn.animals) || [];
      const it = items.find((x) => x.id === id);
      if (!it) return;
      const info = find(isFish ? 'fish' : 'animals', it.kind);
      if (nowS() < it.readyAt) return A.toast(`${info.name} còn ${fmtDur(it.readyAt - nowS())} nữa mới lớn`);
      const res = await call(`/api/farm/${isFish ? 'fish' : 'animals'}/sell`, { id });
      if (res) floatGain(t, res.gained);
      return;
    }
    if (d.openShop) return openShop(d.openShop);
  }

  async function load() {
    const res = await call('/api/farm');
    if (res) $('farm').classList.remove('hidden');
  }

  // ---------- Sự kiện ----------

  document.addEventListener('click', onClick);
  $('btn-shop').addEventListener('click', () => openShop(shopMode.tab === 'crops' && shopMode.plot !== null ? 'crops' : shopMode.tab));
  $('shop-close').addEventListener('click', () => $('dlg-shop').close());
  $('dlg-shop').addEventListener('close', () => (shopMode.plot = null));
  document.querySelectorAll('#shop-tabs .tab').forEach((b) => b.addEventListener('click', (e) => {
    e.stopPropagation();
    shopMode = { tab: b.dataset.tab, plot: null };
    renderShop();
  }));
  $('btn-harvest-all').addEventListener('click', async (e) => {
    const res = await call('/api/farm/harvest-all', {});
    if (res) floatGain(e.currentTarget, res.gained);
  });
  document.addEventListener('visibilitychange', () => document.visibilityState === 'visible' && load());

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(28);
  A.me().then(
    async (u) => {
      if (!u) return A.requireLogin();
      user = u;
      renderHUD();
      const manifest = await fetch(IMG + 'manifest.json').then((r) => r.json()).catch(() => ({}));
      for (const f of manifest.images || manifest.files || []) sizes[f.file.replace(/\.png$/, '')] = [f.w, f.h];
      cat = await A.api('/api/farm/catalog').catch((e) => (A.toast(e.message), null));
      await load();
      setInterval(tick, 1000);
      pollTimer = setInterval(load, 15000);
    },
    (e) => A.toast(e.message),
  );
})();
