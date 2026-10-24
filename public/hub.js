// Trang chủ DCity: đăng nhập / tạo nhân vật, HUD, bản đồ và bảng thông tin từng khu.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const City = window.City;
  const $ = (id) => document.getElementById(id);

  const ZONE_ICON = { plaza: 'mapPin', casino: 'layers', shop: 'shirt', bank: 'landmark', farm: 'sprout', hospital: 'hospital' };
  const GAME_PATH = { tienlen: '/tienlen/' };

  let user = null;
  // Ghi đè bằng /api/config; catalog là danh mục trang phục (level, giá) của shop.
  let config = {
    startBalance: 1000,
    dailyBonus: 500,
    catalog: [
      { id: 'casual', name: 'Thường ngày', level: 1, price: 0, starter: true },
      { id: 'sport', name: 'Thể thao', level: 1, price: 0, starter: true },
    ],
    // HP tối thiểu để vào từng khu, và quy tắc bệnh viện.
    zoneMinHP: { plaza: 0, bank: 0, hospital: 0, shop: 10, casino: 30, farm: 40 },
    hospital: { dischargePrice: 200, healPerHP: 2, stayMinutes: 5, regenPerMinute: 1, carDamage: 20 },
  };
  const outfitName = (id) => (config.catalog.find((o) => o.id === id) || C.OUTFITS[id] || { name: id }).name;
  let games = [];
  let current = null; // khu đang mở bảng
  let cityMounted = false;

  // Ngoại hình nhân vật: tóc / cánh đọc ở đây (user.hair, user.wings) để các chỗ vẽ dùng chung.
  const look = (u) => ({ gender: u.gender, outfit: u.outfit, hair: u.hair || 'default', wings: u.wings || '' });

  // Ba nhóm của shop thời trang: danh mục (từ /api/config), đồ đã có, đồ đang mặc.
  let shopTab = 'outfit';
  const SHOP_KINDS = {
    outfit: { name: 'Đồ', icon: 'shirt', list: () => config.catalog, owned: () => user.owned || [], worn: () => user.outfit },
    hair: { name: 'Tóc', icon: 'sparkles', list: () => config.hair || [], owned: () => user.ownedHair || ['default'], worn: () => user.hair || 'default' },
    wings: { name: 'Cánh', icon: 'crown', list: () => config.wings || [], owned: () => user.ownedWings || [], worn: () => user.wings || '' },
  };

  // Sau khi đăng nhập thì quay lại trang đang mở dở (ví dụ link mời vào phòng).
  function nextPage() {
    const next = new URLSearchParams(location.search).get('next');
    return next && next.startsWith('/') && !next.startsWith('//') ? next : null;
  }

  function show(id) {
    $('auth').classList.toggle('hidden', id !== 'auth');
    $('city').classList.toggle('hidden', id !== 'city');
    $('topbar').classList.toggle('hidden', id !== 'city');
  }

  // ---------- Tạo nhân vật (lúc đăng ký) ----------

  // Giới tính chỉ chọn một lần ở đây; trang phục chỉ được chọn trong các bộ khởi đầu,
  // các bộ khác mua ở Shop thời trang khi đủ level.
  function creator(el, initial, onChange) {
    const state = { gender: initial.gender || 'male', outfit: initial.outfit || 'casual' };
    const draw = () => {
      const starters = config.catalog.filter((o) => o.starter);
      if (!starters.some((o) => o.id === state.outfit)) state.outfit = starters[0].id;
      el.innerHTML = `
        <div class="creator-preview">${C.svg(state)}</div>
        <div class="creator-controls">
          <span class="creator-label">Giới tính <span class="creator-note">· không đổi được về sau</span></span>
          <div class="segmented">${Object.entries(C.GENDERS)
            .map(([id, name]) => `<button type="button" data-gender="${id}" class="${state.gender === id ? 'active' : ''}">${name}</button>`)
            .join('')}</div>
          <span class="creator-label">Trang phục khởi đầu</span>
          <div class="outfits">${starters
            .map((o) => `<button type="button" class="outfit ${state.outfit === o.id ? 'active' : ''}" data-outfit="${o.id}" title="${o.name}">
              ${C.svg({ gender: state.gender, outfit: o.id })}<span>${o.name}</span></button>`)
            .join('')}</div>
          <span class="hint">Lên level để mua thêm trang phục ở Shop thời trang.</span>
        </div>`;
      el.querySelectorAll('[data-gender]').forEach((b) => (b.onclick = () => { state.gender = b.dataset.gender; draw(); }));
      el.querySelectorAll('[data-outfit]').forEach((b) => (b.onclick = () => { state.outfit = b.dataset.outfit; draw(); }));
      if (onChange) onChange({ ...state });
    };
    draw();
    return { state, redraw: draw };
  }

  // ---------- Đăng nhập / đăng ký ----------

  let registerLook = null;

  function switchTab(tab) {
    document.querySelectorAll('.tab').forEach((b) => b.classList.toggle('active', b.dataset.tab === tab));
    $('form-login').classList.toggle('hidden', tab !== 'login');
    $('form-register').classList.toggle('hidden', tab !== 'register');
    const first = $(tab === 'login' ? 'form-login' : 'form-register').querySelector('input');
    first && first.focus();
  }

  async function submit(form, action) {
    const btn = form.querySelector('button[type=submit]');
    btn.disabled = true;
    try {
      setUser(await action());
      form.reset();
      await enterCity();
    } catch (e) {
      A.toast(e.message);
    } finally {
      btn.disabled = false;
    }
  }

  // ---------- HP ----------

  // Giờ server (giây) ước theo lệch giờ đo lúc nhận user gần nhất.
  let clockSkew = 0;
  let hpBase = { hp: 100, at: 0 }; // HP và thời điểm server lúc nhận user
  let hospitalMode = false;

  const serverNow = () => Date.now() / 1000 + clockSkew;
  const maxHP = () => (user && user.maxHp) || 100;
  const inHospital = () => !!(user && user.hospitalUntil && user.hospitalUntil > serverNow());

  // Gán user mới từ server và ghi lại mốc tính HP tự hồi.
  function setUser(u) {
    user = u;
    if (u && typeof u.now === 'number') clockSkew = u.now - Date.now() / 1000;
    if (u && typeof u.hp === 'number') hpBase = { hp: u.hp, at: typeof u.now === 'number' ? u.now : serverNow() };
    return u;
  }

  // HP hiện tại: tự hồi regenPerMinute mỗi phút kể từ lần nhận user (server cũng tính như vậy).
  function hpNow() {
    if (!user || typeof user.hp !== 'number') return maxHP();
    if (inHospital()) {
      // Nằm viện: máu hồi dần từ 0 tới đầy trong thời gian nằm viện (giống server).
      const stay = ((config.hospital && config.hospital.stayMinutes) || 5) * 60;
      const elapsed = serverNow() - (user.hospitalUntil - stay);
      return Math.min(maxHP() - 1, Math.max(user.hp, Math.floor((elapsed * maxHP()) / stay)));
    }
    const regen = (config.hospital && config.hospital.regenPerMinute) || 0;
    const gained = Math.floor(Math.max(0, serverNow() - hpBase.at) / 60) * regen;
    return Math.min(maxHP(), hpBase.hp + gained);
  }

  const minHP = (zone) => (config.zoneMinHP && config.zoneMinHP[zone]) || 0;
  const zoneName = (zone) => (City.ZONES[zone] ? City.ZONES[zone].name : zone);
  const fmtLeft = (sec) => {
    sec = Math.max(0, Math.ceil(sec));
    return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`;
  };

  function renderHP() {
    const el = $('hp-badge');
    if (!el || !user) return;
    const hp = hpNow();
    const pct = Math.round((hp / maxHP()) * 100);
    const tone = pct > 60 ? 'good' : pct > 30 ? 'warn' : 'bad';
    el.className = `hp-badge ${tone}${inHospital() ? ' sick' : ''}${el.classList.contains('hit') ? ' hit' : ''}`;
    el.innerHTML = `${I.svg('heart')}<span class="hp-bar"><span style="width:${pct}%"></span></span><b>${hp}/${maxHP()}</b>`;
    const regen = (config.hospital && config.hospital.regenPerMinute) || 1;
    el.title = inHospital()
      ? `Đang nằm viện, còn ${fmtLeft(user.hospitalUntil - serverNow())}`
      : `Máu ${hp}/${maxHP()} HP · tự hồi ${regen} HP mỗi phút. Va chạm xe mất máu; hết máu phải nằm viện.`;
  }

  // Chữ bay lên trên HUD, ví dụ "-20 HP".
  function floatText(text, cls) {
    const anchor = $('hp-badge');
    if (!anchor) return;
    const r = anchor.getBoundingClientRect();
    const f = document.createElement('div');
    f.className = 'hp-float ' + (cls || '');
    f.textContent = text;
    f.style.left = `${r.left + r.width / 2}px`;
    f.style.top = `${r.bottom + 4}px`;
    document.body.appendChild(f);
    setTimeout(() => f.remove(), 1300);
  }

  // Xe tông trên bản đồ: trừ máu ở server; hết máu thì vào viện.
  async function onHit(source) {
    if (!user || inHospital()) return;
    try {
      setUser(await A.api('/api/hp/hit', { method: 'POST', body: { source: source || 'car' } }));
    } catch (e) {
      if (e.status !== 423) A.toast(e.message);
      return;
    }
    const dmg = (config.hospital && config.hospital.carDamage) || 20;
    floatText(`-${dmg} HP`, 'dmg');
    const badge = $('hp-badge');
    badge.classList.add('hit');
    setTimeout(() => badge.classList.remove('hit'), 700);
    renderHUD();
    if (inHospital()) {
      A.toast('Bạn bị xe tông, hết máu! Đưa vào bệnh viện.');
      checkHospital();
    } else {
      A.toast(`Cẩn thận xe! -${dmg} HP`);
      refreshPanel();
    }
  }

  // Vào / ra chế độ nằm viện theo hospitalUntil.
  function checkHospital() {
    if (!cityMounted || !user) return;
    if (inHospital() && !hospitalMode) {
      hospitalMode = true;
      if (typeof City.teleport === 'function') City.teleport('hospital');
      if (typeof City.setLocked === 'function') City.setLocked(true);
      openPanel('hospital');
    } else if (!inHospital() && hospitalMode) {
      hospitalMode = false;
      if (typeof City.setLocked === 'function') City.setLocked(false);
      A.toast('Đã xuất viện');
      A.me().then((u) => u && (setUser(u), renderHUD(), refreshPanel())).catch(() => {});
    }
  }

  // Mỗi giây: cập nhật thanh máu, đồng hồ bệnh viện, kiểm tra xuất viện.
  function tick() {
    if (!user) return;
    renderHP();
    checkHospital();
    const left = $('hosp-left');
    if (left && inHospital()) {
      const stay = ((config.hospital && config.hospital.stayMinutes) || 5) * 60;
      const remain = user.hospitalUntil - serverNow();
      left.textContent = fmtLeft(remain);
      const bar = $('hosp-bar');
      if (bar) bar.style.width = `${Math.min(100, Math.max(0, 100 - (remain / stay) * 100))}%`;
    }
  }

  // ---------- HUD ----------

  function levelPct(u) {
    return Math.min(100, Math.round(((u.xp - u.levelXP) / Math.max(1, u.nextXP - u.levelXP)) * 100));
  }

  function renderHUD() {
    renderHP();
    $('balance').textContent = A.xu(user.balance);
    $('level-badge').innerHTML = `<span class="lv">Lv ${user.level}</span><span class="xp-bar"><span style="width:${levelPct(user)}%"></span></span>`;
    $('level-badge').title = `${user.xp - user.levelXP} / ${user.nextXP - user.levelXP} XP để lên level ${user.level + 1}`;
    $('menu-avatar').innerHTML = C.head(look(user));
    $('menu-name').textContent = user.name;
    $('menu-display').textContent = user.name;
    $('menu-username').textContent = user.username ? '@' + user.username : 'Tài khoản khách';
    $('btn-password').classList.toggle('hidden', !user.username);
    $('legacy').classList.toggle('hidden', !!user.username);
    if (cityMounted) City.setTag(user.name, user.level);
  }

  // ---------- Bảng thông tin từng khu ----------

  function panelHead(zone, title, sub) {
    return `<div class="panel-head"><span class="panel-icon">${I.svg(ZONE_ICON[zone])}</span>
      <div><h2>${A.escapeHtml(title)}</h2><div class="muted">${A.escapeHtml(sub)}</div></div></div>`;
  }

  // Nút vào game của một khu; thiếu HP thì khoá kèm lý do.
  function enterButton(zone, href, label) {
    const need = minHP(zone);
    if (hpNow() < need || inHospital()) {
      return `<button class="primary" disabled title="Cần ít nhất ${need} HP">${I.svg('heart')}Cần ≥ ${need} HP để vào</button>`;
    }
    return `<a class="btn primary" href="${href}">${I.svg('play')}${label}</a>`;
  }

  // Bảng hiện thay cho khu khi không đủ HP (hoặc đang nằm viện).
  function gatePanel(zone) {
    const need = minHP(zone);
    const why = inHospital()
      ? 'Bạn đang nằm viện, chưa đi đâu được.'
      : `Cần ít nhất <b>${need} HP</b> để vào ${A.escapeHtml(zoneName(zone))}. Bạn có <b>${hpNow()} HP</b>.`;
    return `${panelHead(zone, zoneName(zone), 'Không đủ máu để vào')}
      <div class="gate-box">${I.svg('heart')}<p>${why}</p>
        <p class="muted">Máu tự hồi ${(config.hospital && config.hospital.regenPerMinute) || 1} HP mỗi phút, hoặc chữa ở Bệnh viện.</p>
        <button class="primary" id="btn-go-hospital">${I.svg('hospital')}Tới Bệnh viện</button>
      </div>`;
  }

  const PANELS = {
    hospital() {
      const h = config.hospital || {};
      const hp = hpNow();
      if (inHospital()) {
        return `${panelHead('hospital', 'Bệnh viện', 'Bạn đang nằm viện')}
          <div class="panel-section hosp-stay">
            <div class="hosp-bed">${I.svg('hospital')}</div>
            <p style="margin:0">Hết máu nên bạn được đưa vào viện. Nằm viện xong sẽ đầy máu và đi tiếp được.</p>
            <div class="stat"><div class="muted">Xuất viện sau</div><b class="hosp-left" id="hosp-left">--:--</b></div>
            <span class="hp-bar big"><span id="hosp-bar" style="width:0%"></span></span>
            <button class="primary" id="btn-discharge" ${user.balance < (h.dischargePrice || 0) ? 'disabled title="Không đủ xu"' : ''}>
              ${I.svg('coins')}Xuất viện ngay — ${A.xu(h.dischargePrice || 0)} xu</button>
          </div>`;
      }
      const missing = maxHP() - hp;
      const cost = missing * (h.healPerHP || 0);
      return `${panelHead('hospital', 'Bệnh viện', 'Chữa máu cho nhân vật')}
        <div class="panel-section">
          <div class="stats">
            <div class="stat"><div class="muted">Máu</div><b>${I.svg('heart')}${hp}/${maxHP()} HP</b></div>
            <div class="stat"><div class="muted">Số dư</div><b class="gold">${I.svg('coins')}${A.xu(user.balance)}</b></div>
          </div>
          <span class="hp-bar big"><span style="width:${Math.round((hp / maxHP()) * 100)}%"></span></span>
          <button class="primary" id="btn-heal" ${missing <= 0 || user.balance < cost ? 'disabled' : ''}>
            ${I.svg('heart')}${missing <= 0 ? 'Đang đầy máu' : `Chữa đầy — ${A.xu(cost)} xu`}</button>
        </div>
        <div class="panel-section"><h3>${I.svg('scroll')}Quy tắc</h3>
          <ul class="rules">
            <li>Va chạm xe trên đường: <b>-${h.carDamage || 20} HP</b>.</li>
            <li>Hết máu phải nằm viện <b>${h.stayMinutes || 5} phút</b>, hoặc trả <b>${A.xu(h.dischargePrice || 0)} xu</b> để xuất viện ngay.</li>
            <li>Máu tự hồi <b>${h.regenPerMinute || 1} HP</b> mỗi phút; chữa ở đây ${h.healPerHP || 2} xu mỗi HP.</li>
            <li>Mỗi khu cần đủ máu mới vào được: ${Object.entries(config.zoneMinHP || {})
              .filter(([, n]) => n > 0)
              .map(([z, n]) => `${A.escapeHtml(zoneName(z))} ≥ ${n}`)
              .join(', ')}.</li>
          </ul>
        </div>`;
    },

    plaza() {
      return `${panelHead('plaza', 'Quảng trường', 'Trung tâm thành phố')}
        <div class="panel-section">
          <div class="profile">${C.svg(look(user))}
            <div>
              <div class="profile-name">${A.escapeHtml(user.name)}</div>
              <div class="muted">${user.username ? '@' + A.escapeHtml(user.username) : 'Tài khoản khách'}</div>
              <div class="xp-block">
                <span class="level-badge"><span class="lv">Lv ${user.level}</span>${user.xp} XP</span>
                <span class="xp-bar"><span style="width:${levelPct(user)}%"></span></span>
                <span class="muted">Còn ${user.nextXP - user.xp} XP để lên level ${user.level + 1}</span>
              </div>
            </div>
          </div>
          <div class="stats">
            <div class="stat"><div class="muted">Số dư</div><b class="gold">${I.svg('coins')}${A.xu(user.balance)}</b></div>
            <div class="stat"><div class="muted">Trang phục</div><b>${I.svg('shirt')}${outfitName(user.outfit)}</b></div>
          </div>
        </div>
        <div class="panel-section"><h3>${I.svg('mapPin')}Đi đâu tiếp?</h3>
          <p class="muted">Bấm vào một khu trên bản đồ, nhân vật sẽ đi bộ tới đó. Chơi game để kiếm xu và lên level.</p>
        </div>`;
    },

    casino() {
      const g = games.find((x) => x.id === 'tienlen');
      const online = g && g.online ? `${g.online.players} người đang chơi · ${g.online.rooms} phòng` : '';
      return `${panelHead('casino', 'Casino', online || 'Sòng bài thành phố')}
        <div class="panel-section">
          <div class="panel-art">${C.svg({ gender: 'male', outfit: 'suit' }, { cls: 'art' })}${C.svg({ gender: 'female', outfit: 'royal' }, { cls: 'art' })}</div>
          <h2 style="margin:0">Tiến Lên Miền Nam</h2>
          <p class="muted" style="margin:0">${g ? A.escapeHtml(g.description) : ''}</p>
          <div class="stats">
            <div class="stat"><div class="muted">Số người</div><b>${I.svg('users')}${g ? A.escapeHtml(g.players) : '2–4'}</b></div>
            <div class="stat"><div class="muted">Thưởng mỗi ván</div><b>${I.svg('trophy')}10–30 XP</b></div>
          </div>
          ${enterButton('casino', GAME_PATH.tienlen, 'Vào sòng Tiến Lên')}
        </div>
        <div class="panel-section">
          <div class="panel-art">${['bau', 'cua', 'tom', 'ca', 'ga', 'nai'].map((f) => `<img src="/img/toon/baucua/face_${f}.png" alt="" width="40" height="40" draggable="false">`).join('')}</div>
          <h2 style="margin:0">Bầu Cua Tôm Cá</h2>
          <p class="muted" style="margin:0">Đặt chip vào bầu, cua, tôm, cá, gà, nai rồi chờ nhà cái mở bát: mặt ra mấy lần ăn gấp mấy lần.</p>
          <div class="stats">
            <div class="stat"><div class="muted">Mỗi vòng</div><b>${I.svg('timer')}30 giây</b></div>
            <div class="stat"><div class="muted">Thưởng</div><b>${I.svg('trophy')}XP mỗi vòng</b></div>
          </div>
          ${enterButton('casino', '/baucua/', 'Chơi Bầu cua')}
        </div>`;
    },

    bank() {
      return `${panelHead('bank', 'Ngân hàng', 'Ví xu dùng chung mọi khu')}
        <div class="panel-section">
          <div class="muted">Số dư</div>
          <div class="big-coins">${I.svg('coins')}${A.xu(user.balance)}</div>
          <button class="primary daily" id="btn-daily" ${user.canDaily ? '' : 'disabled'}>
            ${user.canDaily ? `${I.svg('gift')}Nhận ${A.xu(config.dailyBonus)} xu hôm nay` : `${I.svg('check')}Đã nhận thưởng hôm nay`}</button>
          <p class="muted" style="margin:0">Xu chỉ để chơi cho vui, không quy đổi ra tiền thật.</p>
        </div>
        <div class="panel-section"><h3>${I.svg('history')}Lịch sử xu</h3><ul class="history" id="history"><li class="empty">Đang tải...</li></ul></div>`;
    },

    shop() {
      const kind = SHOP_KINDS[shopTab] ? shopTab : 'outfit';
      const K = SHOP_KINDS[kind];
      const list = kind === 'wings' ? [{ id: '', name: 'Không cánh', level: 1, price: 0, starter: true }, ...K.list()] : K.list();
      const items = list.map((o) => {
        // Quần áo starter chỉ có bộ đã chọn lúc tạo nhân vật; tóc mặc định / không cánh thì ai cũng có.
        const have = K.owned().includes(o.id) || (kind !== 'outfit' && o.starter);
        const wearing = K.worn() === o.id;
        const locked = !have && user.level < o.level;
        const poor = !have && !locked && user.balance < o.price;
        let action;
        if (wearing) action = `<button class="wear" disabled>${I.svg('check')}${o.id === '' ? 'Đang để trống' : 'Đang mặc'}</button>`;
        else if (have) action = `<button class="wear" data-wear="${o.id}">${I.svg(K.icon)}${kind === 'wings' && o.id === '' ? 'Tháo cánh' : 'Mặc'}</button>`;
        else if (locked) action = `<button class="wear" disabled>${I.svg('lock')}Cần Lv ${o.level}</button>`;
        else action = `<button class="primary wear" data-buy="${o.id}" ${poor ? 'disabled title="Không đủ xu"' : ''}>${I.svg('coins')}${A.xu(o.price)}</button>`;
        const tag = have ? `<span class="tag me">${o.starter ? 'Miễn phí' : 'Đã có'}</span>` : `<span class="tag lv-tag">Lv ${o.level}+</span>`;
        return `<div class="shop-item${locked ? ' locked' : ''}${wearing ? ' wearing' : ''}" data-try="${o.id}">
            <div class="shop-preview">${C.svg({ ...look(user), [kind]: o.id })}${locked ? `<span class="lock">${I.svg('lock')}</span>` : ''}</div>
            <div class="shop-name">${A.escapeHtml(o.name)}</div>${tag}
            ${action}
          </div>`;
      });
      const tabs = Object.entries(SHOP_KINDS)
        .map(([k, t]) => `<button class="tab${k === kind ? ' active' : ''}" data-shop-tab="${k}" role="tab">${I.svg(t.icon)}${t.name}</button>`)
        .join('');
      return `${panelHead('shop', 'Shop thời trang', 'Quần áo, tóc, cánh: đủ level thì mua bằng xu, đồ đã mua mặc lúc nào cũng được')}
        <div class="panel-section">
          <div class="stats">
            <div class="stat"><div class="muted">Level</div><b>${I.svg('trophy')}Lv ${user.level}</b></div>
            <div class="stat"><div class="muted">Số dư</div><b class="gold">${I.svg('coins')}${A.xu(user.balance)}</b></div>
          </div>
          <div class="tabs" role="tablist" style="grid-template-columns:repeat(3,1fr)">${tabs}</div>
          <p class="muted" style="margin:0">Bấm vào một món để mặc thử trên bản đồ.</p>
          <div class="shop-grid">${items.join('')}</div>
        </div>`;
    },


    farm() {
      return `${panelHead('farm', 'Nông trại', 'Trồng trọt, chăn nuôi, nuôi cá')}
        <div class="panel-section">
          <p class="muted" style="margin:0">Gieo hạt chờ cây chín rồi thu hoạch, nuôi gia súc trong chuồng và thả cá trong ao để bán lấy xu và XP.</p>
          <div class="stats">
            <div class="stat"><div class="muted">Ô đất</div><b>${I.svg('sprout')}6 → 48 ô</b></div>
            <div class="stat"><div class="muted">Chuồng · Ao</div><b>${I.svg('landmark')}Mua ở cửa hàng</b></div>
          </div>
          ${enterButton('farm', '/farm/', 'Vào nông trại')}
        </div>`;
    },
  };

  // Có vào được khu không: đủ HP, và không nằm viện (trừ chính bệnh viện).
  const canEnter = (zone) => (inHospital() ? zone === 'hospital' : hpNow() >= minHP(zone));

  function openPanel(zone) {
    if (!PANELS[zone]) return;
    current = zone;
    $('panel').classList.remove('hidden');
    let x = 0;
    try { x = City.zoneX(zone); } catch (e) { /* khu chưa có trên bản đồ */ }
    $('panel').classList.toggle('left', x >= 900);
    const allowed = canEnter(zone);
    $('panel-body').innerHTML = allowed ? PANELS[zone]() : gatePanel(zone);
    document.querySelectorAll('.zone-nav button').forEach((b) => b.classList.toggle('active', b.dataset.zone === zone));
    if (!allowed) {
      const go = $('btn-go-hospital');
      if (go) go.onclick = () => (inHospital() ? openPanel('hospital') : City.walkTo('hospital'));
      return;
    }
    if (zone === 'bank') bindBank();
    if (zone === 'shop') bindShop();
    if (zone === 'hospital') bindHospital();
    tick();
  }

  function bindHospital() {
    const act = async (msg) => {
      try {
        setUser(await A.api('/api/hospital/heal', { method: 'POST' }));
        renderHUD();
        if (!inHospital() && hospitalMode) checkHospital();
        else {
          openPanel('hospital');
          A.toast(msg);
        }
      } catch (e) {
        A.toast(e.message);
      }
    };
    const heal = $('btn-heal');
    if (heal) heal.onclick = () => act('Đã chữa đầy máu');
    const out = $('btn-discharge');
    if (out) out.onclick = () => act('Đã xuất viện');
  }

  function refreshPanel() {
    if (current && !$('panel').classList.contains('hidden')) openPanel(current);
  }

  function entryIcon(e) {
    if (e.game === 'hub') return e.note.includes('ngày') ? 'gift' : 'sparkles';
    if (e.game === 'shop') return 'shirt';
    if (e.game === 'hospital') return 'heart';
    return e.game === 'tienlen' ? 'layers' : e.delta > 0 ? 'trendingUp' : 'trendingDown';
  }

  async function bindBank() {
    const daily = $('btn-daily');
    daily.onclick = async () => {
      try {
        setUser(await A.api('/api/daily', { method: 'POST' }));
        renderHUD();
        openPanel('bank');
        A.toast(`Đã nhận ${A.xu(config.dailyBonus)} xu`);
      } catch (e) {
        A.toast(e.message);
      }
    };
    const entries = await A.api('/api/history').catch(() => []);
    const list = $('history');
    if (!list) return;
    list.innerHTML = entries.length
      ? entries
          .map((e) => {
            const when = new Date(e.at).toLocaleString('vi-VN', { dateStyle: 'short', timeStyle: 'short' });
            const cls = e.delta > 0 ? 'plus' : 'minus';
            return `<li><span class="kind ${cls}">${I.svg(entryIcon(e))}</span>
              <span class="note">${A.escapeHtml(e.note)}<span class="when">${when} · còn ${A.xu(e.balanceAfter)}</span></span>
              <span class="delta ${cls}">${e.delta > 0 ? '+' : ''}${A.xu(e.delta)}</span></li>`;
          })
          .join('')
      : '<li class="empty">Chưa có giao dịch nào</li>';
  }

  function bindShop() {
    const body = $('panel-body');
    const kind = SHOP_KINDS[shopTab] ? shopTab : 'outfit';
    body.querySelectorAll('[data-shop-tab]').forEach((b) => (b.onclick = () => {
      shopTab = b.dataset.shopTab;
      City.setLook(look(user));
      openPanel('shop');
    }));
    // Mặc thử trên bản đồ (chưa lưu); đóng bảng hoặc đi chỗ khác thì trở lại đồ đang mặc.
    body.querySelectorAll('[data-try]').forEach((el) =>
      el.addEventListener('click', (e) => {
        if (e.target.closest('button')) return;
        City.setLook({ ...look(user), [kind]: el.dataset.try });
        body.querySelectorAll('.shop-item').forEach((x) => x.classList.toggle('trying', x === el));
      }),
    );
    const act = async (path, payload, done) => {
      try {
        setUser(await A.api(path, { method: 'POST', body: payload }));
        renderHUD();
        City.setLook(look(user));
        openPanel('shop');
        A.toast(done);
      } catch (e) {
        A.toast(e.message);
      }
    };
    const item = (id) => SHOP_KINDS[kind].list().find((x) => x.id === id);
    body.querySelectorAll('[data-wear]').forEach((b) => (b.onclick = () => {
      const id = b.dataset.wear;
      if (kind === 'outfit') return act('/api/character', { outfit: id }, 'Đã thay trang phục');
      const done = kind === 'wings' ? (id ? 'Đã đeo cánh' : 'Đã tháo cánh') : 'Đã đổi kiểu tóc';
      act('/api/me/items/wear', { kind, id }, done);
    }));
    body.querySelectorAll('[data-buy]').forEach((b) => (b.onclick = () => {
      const o = item(b.dataset.buy);
      if (!o || !confirm(`Mua "${o.name}" với giá ${A.xu(o.price)} xu?`)) return;
      if (kind === 'outfit') act('/api/outfits/buy', { outfit: o.id }, `Đã mua ${o.name}`);
      else act('/api/me/items/buy', { kind, id: o.id }, `Đã mua và mặc ${o.name}`);
    }));
  }

  function closePanel() {
    $('panel').classList.add('hidden');
    if (current === 'shop') City.setLook(look(user)); // bỏ thử đồ chưa lưu
    current = null;
    document.querySelectorAll('.zone-nav button').forEach((b) => b.classList.remove('active'));
  }

  // ---------- Thành phố ----------

  async function loadGames() {
    games = await A.api('/api/games').catch(() => games);
    const g = games.find((x) => x.id === 'tienlen');
    if (g && g.online) City.setSub('casino', g.online.players ? `Tiến Lên · ${g.online.players} người đang chơi` : 'Tiến Lên Miền Nam');
  }

  // Màn hình hẹp: bản đồ cuộn ngang, giữ nhân vật trong tầm nhìn.
  function follow(x) {
    const map = $('map');
    if (map.scrollWidth <= map.clientWidth) return;
    const px = (x / 1600) * map.scrollWidth;
    map.scrollLeft = px - map.clientWidth / 2;
  }

  async function enterCity() {
    const next = nextPage();
    if (next) {
      location.href = next;
      return;
    }
    show('city');
    if (!cityMounted) {
      let at = 'plaza';
      try { at = localStorage.getItem('city.at') || 'plaza'; } catch (e) { /* bỏ qua */ }
      const start = City.mount($('map'), {
        look: look(user), name: user.name, level: user.level, at,
        onArrive: openPanel, onMove: follow, onDepart: closePanel, onHit,
      });
      cityMounted = true;
      $('zone-nav').innerHTML = Object.entries(City.ZONES)
        .map(([id, z]) => {
          const need = minHP(id);
          return `<button data-zone="${id}">${I.svg(ZONE_ICON[id] || 'mapPin')}<span>${z.name}</span>${need ? `<em class="hp-need">≥${need}</em>` : ''}</button>`;
        })
        .join('');
      $('zone-nav').querySelectorAll('button').forEach((b) => (b.onclick = () => {
        if (hospitalMode) return A.toast('Bạn đang nằm viện, chưa đi được');
        City.walkTo(b.dataset.zone);
      }));
      requestAnimationFrame(() => follow(City.position()[0]));
      document.querySelectorAll('.zone-nav button').forEach((b) => b.classList.toggle('active', b.dataset.zone === start));
      setInterval(tick, 1000);
      // Đồng bộ máu / xu với server định kỳ (máu tự hồi, có thể thay đổi ở trang khác).
      setInterval(() => A.me().then((u) => u && (setUser(u), renderHUD())).catch(() => {}), 30000);
      await loadGames();
      setInterval(loadGames, 15000);
    } else {
      City.setLook(look(user));
    }
    renderHUD();
    checkHospital();
  }

  // Mở hộp thoại; onOk trả về promise, lỗi thì giữ hộp thoại mở để sửa.
  function openDialog(id, onOk, init) {
    const dlg = $(id);
    const form = dlg.querySelector('form');
    form.reset();
    if (init) init(form);
    dlg.showModal();
    form.onsubmit = async (e) => {
      if (e.submitter && e.submitter.value === 'cancel') return;
      e.preventDefault();
      try {
        await onOk(Object.fromEntries(new FormData(form)));
        dlg.close();
      } catch (err) {
        A.toast(err.message);
      }
    };
  }

  // ---------- Sự kiện ----------

  document.querySelectorAll('.tab').forEach((b) => b.addEventListener('click', () => switchTab(b.dataset.tab)));

  $('form-login').addEventListener('submit', (e) => {
    e.preventDefault();
    const f = Object.fromEntries(new FormData(e.target));
    submit(e.target, () => A.login(f.username, f.password));
  });

  $('form-register').addEventListener('submit', (e) => {
    e.preventDefault();
    const f = Object.fromEntries(new FormData(e.target));
    if (f.password !== f.confirm) return A.toast('Mật khẩu nhập lại không khớp');
    submit(e.target, () => A.register(f.username, f.password, f.name, registerLook));
  });

  $('panel-close').addEventListener('click', closePanel);

  $('btn-menu').addEventListener('click', (e) => {
    e.stopPropagation();
    $('menu').classList.toggle('hidden');
  });
  document.addEventListener('click', () => $('menu').classList.add('hidden'));

  $('btn-rename').addEventListener('click', () =>
    openDialog(
      'dlg-rename',
      async (f) => {
        user = await A.api('/api/me', { method: 'POST', body: { name: f.name } });
        renderHUD();
        refreshPanel();
      },
      (form) => (form.elements.name.value = user.name),
    ),
  );

  $('btn-password').addEventListener('click', () =>
    openDialog('dlg-password', async (f) => {
      await A.api('/api/password', { method: 'POST', body: { old: f.old, new: f.new } });
      A.toast('Đã đổi mật khẩu');
    }),
  );

  $('btn-legacy').addEventListener('click', () =>
    openDialog('dlg-legacy', async (f) => {
      user = await A.api('/api/credentials', { method: 'POST', body: f });
      renderHUD();
      A.toast('Đã đặt tên đăng nhập, từ giờ đăng nhập bằng @' + user.username);
    }),
  );

  $('btn-logout').addEventListener('click', async () => {
    await A.logout().catch(() => {});
    location.href = '/';
  });

  // Quay lại từ ván bài: cập nhật số dư, level.
  window.addEventListener('pageshow', (e) => {
    if (e.persisted && user) A.me().then((u) => u && (setUser(u), renderHUD(), refreshPanel()));
  });

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(32);
  $('auth-logo').innerHTML = I.logo(36);
  $('hero-art').innerHTML = C.svg({ gender: 'male', outfit: 'suit' }) + C.svg({ gender: 'female', outfit: 'aodai' });
  const registerCreator = creator($('register-creator'), { gender: 'male', outfit: 'casual' }, (l) => (registerLook = l));

  const showConfig = () => {
    $('start-coins').textContent = A.xu(config.startBalance);
    $('daily-coins').textContent = A.xu(config.dailyBonus);
  };
  showConfig();
  A.api('/api/config')
    .then((c) => {
      config = { ...config, ...c };
      showConfig();
      registerCreator.redraw();
      refreshPanel();
    })
    .catch(() => {});

  (async () => {
    try {
      setUser(await A.me());
    } catch (e) {
      A.toast(e.message);
    }
    if (user) enterCity();
    else {
      show('auth');
      switchTab(nextPage() ? 'register' : 'login');
    }
  })();
})();
