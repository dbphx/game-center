// Trang chủ Game Hub.
(function () {
  'use strict';
  const A = window.Account;
  const $ = (id) => document.getElementById(id);
  const ICONS = { tienlen: '🂡' };

  let user = null;
  let config = { startBalance: 1000, dailyBonus: 500 }; // ghi đè bằng /api/config

  // Sau khi tạo tài khoản thì quay lại trang đang chơi dở (ví dụ link mời vào phòng).
  function nextPage() {
    const next = new URLSearchParams(location.search).get('next');
    return next && next.startsWith('/') && !next.startsWith('//') ? next : null;
  }

  function show(id) {
    $('welcome').classList.toggle('hidden', id !== 'welcome');
    $('hub').classList.toggle('hidden', id !== 'hub');
    $('topbar').classList.toggle('hidden', id !== 'hub');
  }

  function renderUser() {
    $('user-name').textContent = user.name;
    $('balance').textContent = '💰 ' + A.xu(user.balance) + ' xu';
    $('btn-daily').disabled = !user.canDaily;
    $('btn-daily').textContent = user.canDaily ? `🎁 Nhận ${A.xu(config.dailyBonus)} xu` : '✓ Đã nhận hôm nay';
  }

  async function renderGames() {
    const games = await A.api('/api/games');
    $('games').innerHTML = games
      .map((g) => {
        const online = g.online && g.online.players ? `${g.online.players} người đang chơi` : 'Chưa có ai, mở phòng đi!';
        return `<a class="game-card" href="${A.escapeHtml(g.path)}">
          <span class="icon">${ICONS[g.id] || '🎮'}</span>
          <h3>${A.escapeHtml(g.name)}</h3>
          <p>${A.escapeHtml(g.description)}</p>
          <div class="meta"><span>${A.escapeHtml(g.players)} · ${online}</span><span class="play">Chơi</span></div>
        </a>`;
      })
      .join('');
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
          <span>${A.escapeHtml(e.note)}<br><span class="when">${when}</span></span>
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

  A.api('/api/config')
    .then((c) => (config = c))
    .catch(() => {})
    .finally(() => ($('start-coins').textContent = A.xu(config.startBalance)));

  $('btn-start').addEventListener('click', async () => {
    const name = $('name').value.trim();
    if (!name) {
      $('name').focus();
      return A.toast('Nhập tên trước đã');
    }
    $('btn-start').disabled = true;
    try {
      user = await A.createGuest(name);
      await enterHub();
    } catch (e) {
      A.toast(e.message);
    } finally {
      $('btn-start').disabled = false;
    }
  });
  $('name').addEventListener('keydown', (e) => e.key === 'Enter' && $('btn-start').click());

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

  $('user-name').addEventListener('click', async () => {
    const name = prompt('Tên mới của bạn', user.name);
    if (!name || !name.trim()) return;
    try {
      user = await A.api('/api/me', { method: 'POST', body: { name } });
      renderUser();
    } catch (e) {
      A.toast(e.message);
    }
  });

  // Quay lại trang (ví dụ từ ván bài) thì cập nhật số dư và số người online.
  window.addEventListener('pageshow', (e) => e.persisted && user && A.me().then((u) => u && ((user = u), enterHub())));

  (async () => {
    try {
      user = await A.me();
    } catch (e) {
      A.toast(e.message);
    }
    if (user) enterHub();
    else {
      show('welcome');
      $('name').focus();
    }
  })();
})();
