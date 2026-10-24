// Trang chủ Game Hub.
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const $ = (id) => document.getElementById(id);
  const ART = { tienlen: I.cardFan };
  const GAME_ICON = { tienlen: 'layers' };

  let user = null;
  let config = { startBalance: 1000, dailyBonus: 500 }; // ghi đè bằng /api/config

  // Sau khi đăng nhập thì quay lại trang đang mở dở (ví dụ link mời vào phòng).
  function nextPage() {
    const next = new URLSearchParams(location.search).get('next');
    return next && next.startsWith('/') && !next.startsWith('//') ? next : null;
  }

  function show(id) {
    $('auth').classList.toggle('hidden', id !== 'auth');
    $('hub').classList.toggle('hidden', id !== 'hub');
    $('topbar').classList.toggle('hidden', id !== 'hub');
  }

  // ---------- Đăng nhập / đăng ký ----------

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
      user = await action(Object.fromEntries(new FormData(form)));
      form.reset();
      await enterHub();
    } catch (e) {
      A.toast(e.message);
    } finally {
      btn.disabled = false;
    }
  }

  // ---------- Hub ----------

  function renderUser() {
    const balance = A.xu(user.balance);
    $('balance').textContent = balance;
    $('welcome-balance').textContent = balance;
    $('welcome-name').textContent = user.name;
    $('welcome-avatar').innerHTML = I.avatar(user.name).replace('class="avatar"', 'class="avatar lg"');
    $('menu-avatar').innerHTML = I.avatar(user.name);
    $('menu-name').textContent = user.name;
    $('menu-display').textContent = user.name;
    $('menu-username').textContent = user.username ? '@' + user.username : 'Tài khoản khách';
    $('btn-password').classList.toggle('hidden', !user.username);
    $('legacy').classList.toggle('hidden', !!user.username);

    const daily = $('btn-daily');
    daily.disabled = !user.canDaily;
    daily.innerHTML = user.canDaily
      ? `${I.svg('gift')}Nhận ${A.xu(config.dailyBonus)} xu hôm nay`
      : `${I.svg('check')}Đã nhận thưởng hôm nay`;
  }

  async function renderGames() {
    const games = await A.api('/api/games');
    $('games').innerHTML = games
      .map((g) => {
        const n = g.online && g.online.players;
        const live = n ? `<span class="tag live">${n} người đang chơi</span>` : '';
        const art = ART[g.id] ? ART[g.id]() : I.svg('gamepad', 'art');
        return `<a class="game-card" href="${A.escapeHtml(g.path)}">
          <div class="game-cover">${art}${live}</div>
          <div class="game-body">
            <h3>${A.escapeHtml(g.name)}</h3>
            <p>${A.escapeHtml(g.description)}</p>
            <div class="game-meta">
              <span>${I.svg('users')}${A.escapeHtml(g.players)}</span>
              <span class="btn primary">${I.svg('play')}Chơi ngay</span>
            </div>
          </div>
        </a>`;
      })
      .join('');
  }

  function entryIcon(e) {
    if (e.game === 'hub') return e.note.includes('ngày') ? 'gift' : 'sparkles';
    return GAME_ICON[e.game] || (e.delta > 0 ? 'trendingUp' : 'trendingDown');
  }

  async function renderHistory() {
    const entries = await A.api('/api/history');
    if (!entries.length) {
      $('history').innerHTML = '<li class="empty">Chưa có giao dịch nào</li>';
      return;
    }
    $('history').innerHTML = entries
      .map((e) => {
        const when = new Date(e.at).toLocaleString('vi-VN', { dateStyle: 'short', timeStyle: 'short' });
        const cls = e.delta > 0 ? 'plus' : 'minus';
        return `<li>
          <span class="kind ${cls}">${I.svg(entryIcon(e))}</span>
          <span class="note">${A.escapeHtml(e.note)}<span class="when">${when}</span></span>
          <span class="delta ${cls}">${e.delta > 0 ? '+' : ''}${A.xu(e.delta)}</span>
          <span class="after">${A.xu(e.balanceAfter)} xu</span>
        </li>`;
      })
      .join('');
  }

  async function enterHub() {
    const next = nextPage();
    if (next) {
      location.href = next;
      return;
    }
    show('hub');
    renderUser();
    await Promise.all([renderGames(), renderHistory()]);
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
    submit(e.target, (f) => A.login(f.username, f.password));
  });

  $('form-register').addEventListener('submit', (e) => {
    e.preventDefault();
    const f = Object.fromEntries(new FormData(e.target));
    if (f.password !== f.confirm) return A.toast('Mật khẩu nhập lại không khớp');
    submit(e.target, () => A.register(f.username, f.password, f.name));
  });

  $('btn-daily').addEventListener('click', async () => {
    try {
      user = await A.api('/api/daily', { method: 'POST' });
      renderUser();
      renderHistory();
      A.toast(`Đã nhận ${A.xu(config.dailyBonus)} xu`);
    } catch (e) {
      A.toast(e.message);
    }
  });

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
        renderUser();
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

  const openLegacy = () =>
    openDialog('dlg-legacy', async (f) => {
      user = await A.api('/api/credentials', { method: 'POST', body: f });
      renderUser();
      A.toast('Đã đặt tên đăng nhập, từ giờ đăng nhập bằng @' + user.username);
    });
  $('btn-legacy').addEventListener('click', openLegacy);

  $('btn-logout').addEventListener('click', async () => {
    await A.logout().catch(() => {});
    user = null;
    show('auth');
    switchTab('login');
  });

  // Quay lại trang (ví dụ từ ván bài) thì cập nhật số dư và số người online.
  window.addEventListener('pageshow', (e) => e.persisted && user && A.me().then((u) => u && ((user = u), enterHub())));

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(32);
  $('auth-logo').innerHTML = I.logo(36);
  $('hero-art').innerHTML = I.cardFan();

  const showConfig = () => {
    $('start-coins').textContent = A.xu(config.startBalance);
    $('daily-coins').textContent = A.xu(config.dailyBonus);
  };
  showConfig();
  A.api('/api/config')
    .then((c) => {
      config = c;
      showConfig();
      if (user) renderUser();
    })
    .catch(() => {});

  (async () => {
    try {
      user = await A.me();
    } catch (e) {
      A.toast(e.message);
    }
    if (user) enterHub();
    else {
      show('auth');
      switchTab(nextPage() ? 'register' : 'login');
    }
  })();
})();
