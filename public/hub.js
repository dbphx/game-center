// Trang chủ DCity: đăng nhập / tạo nhân vật, HUD, bản đồ và bảng thông tin từng khu.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const City = window.City;
  const $ = (id) => document.getElementById(id);

  const ZONE_ICON = { plaza: 'mapPin', casino: 'layers', shop: 'shirt', bank: 'landmark', site: 'hammer', farm: 'sprout' };
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
  };
  const outfitName = (id) => (config.catalog.find((o) => o.id === id) || C.OUTFITS[id] || { name: id }).name;
  let games = [];
  let current = null; // khu đang mở bảng
  let cityMounted = false;

  const look = (u) => ({ gender: u.gender, outfit: u.outfit });

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
      user = await action();
      form.reset();
      await enterCity();
    } catch (e) {
      A.toast(e.message);
    } finally {
      btn.disabled = false;
    }
  }

  // ---------- HUD ----------

  function levelPct(u) {
    return Math.min(100, Math.round(((u.xp - u.levelXP) / Math.max(1, u.nextXP - u.levelXP)) * 100));
  }

  function renderHUD() {
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

  const PANELS = {
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
          <div class="panel-art">${I.cardFan()}</div>
          <h2 style="margin:0">Tiến Lên Miền Nam</h2>
          <p class="muted" style="margin:0">${g ? A.escapeHtml(g.description) : ''}</p>
          <div class="stats">
            <div class="stat"><div class="muted">Số người</div><b>${I.svg('users')}${g ? A.escapeHtml(g.players) : '2–4'}</b></div>
            <div class="stat"><div class="muted">Thưởng mỗi ván</div><b>${I.svg('trophy')}10–30 XP</b></div>
          </div>
          <a class="btn primary" href="${GAME_PATH.tienlen}">${I.svg('play')}Vào sòng Tiến Lên</a>
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
      const items = config.catalog.map((o) => {
        const have = (user.owned || []).includes(o.id);
        const wearing = user.outfit === o.id;
        const locked = !have && user.level < o.level;
        const poor = !have && !locked && user.balance < o.price;
        let action;
        if (wearing) action = `<button class="wear" disabled>${I.svg('check')}Đang mặc</button>`;
        else if (have) action = `<button class="wear" data-wear="${o.id}">${I.svg('shirt')}Mặc</button>`;
        else if (locked) action = `<button class="wear" disabled>${I.svg('lock')}Cần Lv ${o.level}</button>`;
        else action = `<button class="primary wear" data-buy="${o.id}" ${poor ? 'disabled title="Không đủ xu"' : ''}>${I.svg('coins')}${A.xu(o.price)}</button>`;
        const tag = have ? '<span class="tag me">Đã có</span>' : `<span class="tag lv-tag">Lv ${o.level}+</span>`;
        return `<div class="shop-item${locked ? ' locked' : ''}${wearing ? ' wearing' : ''}" data-try="${o.id}">
            <div class="shop-preview">${C.svg({ gender: user.gender, outfit: o.id })}${locked ? `<span class="lock">${I.svg('lock')}</span>` : ''}</div>
            <div class="shop-name">${A.escapeHtml(o.name)}</div>${tag}
            ${action}
          </div>`;
      });
      return `${panelHead('shop', 'Shop thời trang', 'Đủ level thì mua bằng xu, đồ đã mua mặc lúc nào cũng được')}
        <div class="panel-section">
          <div class="stats">
            <div class="stat"><div class="muted">Level</div><b>${I.svg('trophy')}Lv ${user.level}</b></div>
            <div class="stat"><div class="muted">Số dư</div><b class="gold">${I.svg('coins')}${A.xu(user.balance)}</b></div>
          </div>
          <p class="muted" style="margin:0">Bấm vào một bộ để mặc thử trên bản đồ.</p>
          <div class="shop-grid">${items.join('')}</div>
        </div>`;
    },

    site() {
      return `${panelHead('site', 'Công trường', 'Đang xây dựng')}
        <div class="soon-box">${I.svg('hammer')}<b>Game mới đang được xây</b><span>Khu này sẽ mở một game mới trong thời gian tới.</span></div>`;
    },

    farm() {
      return `${panelHead('farm', 'Nông trại', 'Trồng trọt, chăn nuôi')}
        <div class="soon-box">${I.svg('sprout')}<b>Nông trại sắp mở</b><span>Gieo hạt, chăm cây, thu hoạch đổi xu và XP. Đang chuẩn bị đất!</span></div>`;
    },
  };

  function openPanel(zone) {
    current = zone;
    $('panel').classList.remove('hidden');
    $('panel').classList.toggle('left', City.zoneX(zone) >= 900);
    $('panel-body').innerHTML = PANELS[zone]();
    document.querySelectorAll('.zone-nav button').forEach((b) => b.classList.toggle('active', b.dataset.zone === zone));
    if (zone === 'bank') bindBank();
    if (zone === 'shop') bindShop();
  }

  function refreshPanel() {
    if (current && !$('panel').classList.contains('hidden')) openPanel(current);
  }

  function entryIcon(e) {
    if (e.game === 'hub') return e.note.includes('ngày') ? 'gift' : 'sparkles';
    if (e.game === 'shop') return 'shirt';
    return e.game === 'tienlen' ? 'layers' : e.delta > 0 ? 'trendingUp' : 'trendingDown';
  }

  async function bindBank() {
    const daily = $('btn-daily');
    daily.onclick = async () => {
      try {
        user = await A.api('/api/daily', { method: 'POST' });
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
    // Mặc thử trên bản đồ (chưa lưu); đóng bảng hoặc đi chỗ khác thì trở lại đồ đang mặc.
    body.querySelectorAll('[data-try]').forEach((el) =>
      el.addEventListener('click', (e) => {
        if (e.target.closest('button')) return;
        City.setLook({ gender: user.gender, outfit: el.dataset.try });
        body.querySelectorAll('.shop-item').forEach((x) => x.classList.toggle('trying', x === el));
      }),
    );
    const act = async (path, outfit, done) => {
      try {
        user = await A.api(path, { method: 'POST', body: { outfit } });
        renderHUD();
        City.setLook(look(user));
        openPanel('shop');
        A.toast(done);
      } catch (e) {
        A.toast(e.message);
      }
    };
    body.querySelectorAll('[data-wear]').forEach((b) => (b.onclick = () => act('/api/character', b.dataset.wear, 'Đã thay trang phục')));
    body.querySelectorAll('[data-buy]').forEach((b) => (b.onclick = () => {
      const o = config.catalog.find((x) => x.id === b.dataset.buy);
      if (!confirm(`Mua "${o.name}" với giá ${A.xu(o.price)} xu?`)) return;
      act('/api/outfits/buy', o.id, `Đã mua ${o.name}`);
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
        onArrive: openPanel, onMove: follow, onDepart: closePanel,
      });
      cityMounted = true;
      $('zone-nav').innerHTML = Object.entries(City.ZONES)
        .map(([id, z]) => `<button data-zone="${id}">${I.svg(ZONE_ICON[id])}<span>${z.name}</span></button>`)
        .join('');
      $('zone-nav').querySelectorAll('button').forEach((b) => (b.onclick = () => City.walkTo(b.dataset.zone)));
      requestAnimationFrame(() => follow(City.position()[0]));
      document.querySelectorAll('.zone-nav button').forEach((b) => b.classList.toggle('active', b.dataset.zone === start));
      await loadGames();
      setInterval(loadGames, 15000);
    } else {
      City.setLook(look(user));
    }
    renderHUD();
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
    if (e.persisted && user) A.me().then((u) => u && ((user = u), renderHUD(), refreshPanel()));
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
      user = await A.me();
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
