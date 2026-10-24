// Tài khoản và ví dùng chung cho mọi trang trong DCity.
// Token phiên đăng nhập lưu trong localStorage nên mọi tab cùng trình duyệt là cùng một người.
(function () {
  'use strict';
  const KEY = 'hub.token';

  const load = () => { try { return localStorage.getItem(KEY); } catch (e) { return null; } };
  const save = (v) => { try { v ? localStorage.setItem(KEY, v) : localStorage.removeItem(KEY); } catch (e) { /* bỏ qua */ } };

  async function api(path, { method = 'GET', body } = {}) {
    const headers = { 'Content-Type': 'application/json' };
    const token = load();
    if (token) headers.Authorization = 'Bearer ' + token;
    const res = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : undefined });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      const err = new Error(data.error || 'Lỗi kết nối, thử lại sau');
      err.status = res.status;
      throw err;
    }
    return data;
  }

  // Trả về người dùng hiện tại, hoặc null nếu chưa có tài khoản / token hết hạn.
  async function me() {
    if (!load()) return null;
    try {
      return await api('/api/me');
    } catch (e) {
      if (e.status === 401) {
        save(null);
        return null;
      }
      throw e;
    }
  }

  async function register(username, password, name, look = {}) {
    const data = await api('/api/register', { method: 'POST', body: { username, password, name, ...look } });
    save(data.token);
    return data.user;
  }

  async function login(username, password) {
    const data = await api('/api/login', { method: 'POST', body: { username, password } });
    save(data.token);
    return data.user;
  }

  async function logout() {
    try {
      await api('/api/logout', { method: 'POST' });
    } finally {
      save(null);
    }
  }

  // Chuyển về hub để đăng nhập rồi quay lại đúng trang này.
  function requireLogin() {
    location.href = '/?next=' + encodeURIComponent(location.pathname + location.search);
  }

  const xu = (n) => Number(n).toLocaleString('vi-VN');
  const capitalize = (s) => (s ? s[0].toUpperCase() + s.slice(1) : s);
  const escapeHtml = (s) => String(s).replace(/[&<>"']/g, (ch) => `&#${ch.charCodeAt(0)};`);

  let toastTimer = null;
  function toast(msg) {
    let el = document.getElementById('toast');
    if (!el) {
      el = document.createElement('div');
      el.id = 'toast';
      el.className = 'toast hidden';
      document.body.appendChild(el);
    }
    el.textContent = capitalize(msg);
    el.classList.remove('hidden');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => el.classList.add('hidden'), 3000);
  }

  // Hộp xác nhận / thông báo kiểu game thay cho confirm() / alert() / prompt() của trình duyệt.
  // ask({ title, message, ok, cancel, danger, input }) → Promise: true/false (hoặc chuỗi khi có input, null khi huỷ).
  // Enter = đồng ý, Esc = huỷ. Chỉ một hộp mở tại một thời điểm; mở hộp mới thì hộp cũ coi như huỷ.
  let pending = null;
  function ask(opts) {
    if (pending) pending(false);
    let dlg = document.getElementById('ask-dialog');
    if (!dlg) {
      dlg = document.createElement('dialog');
      dlg.id = 'ask-dialog';
      dlg.className = 'modal ask-dialog';
      document.body.appendChild(dlg);
    }
    const o = { title: 'Xác nhận', ok: 'Đồng ý', cancel: 'Huỷ', ...opts };
    dlg.classList.toggle('danger', !!o.danger);
    dlg.innerHTML = `<form method="dialog">
        <h2>${escapeHtml(o.title)}</h2>
        ${o.message ? `<p class="ask-msg">${escapeHtml(capitalize(o.message))}</p>` : ''}
        ${o.input !== undefined ? `<input class="ask-input" value="${escapeHtml(o.input)}" readonly>` : ''}
        <div class="actions">
          ${o.cancel ? `<button type="button" value="cancel" data-ask="no">${escapeHtml(o.cancel)}</button>` : ''}
          <button type="submit" value="ok" class="primary${o.danger ? ' danger-fill' : ''}" data-ask="yes">${escapeHtml(o.ok)}</button>
        </div>
      </form>`;
    return new Promise((resolve) => {
      const done = (yes) => {
        if (pending !== finish) return;
        pending = null;
        if (dlg.open) dlg.close();
        const input = dlg.querySelector('.ask-input');
        resolve(input ? (yes ? input.value : null) : yes);
      };
      const finish = done;
      pending = finish;
      dlg.querySelector('form').onsubmit = (e) => { e.preventDefault(); done(true); };
      const no = dlg.querySelector('[data-ask=no]');
      if (no) no.onclick = () => done(false);
      dlg.oncancel = (e) => { e.preventDefault(); done(false); };
      dlg.onclick = (e) => { if (e.target === dlg && o.cancel) done(false); }; // bấm ra ngoài hộp = huỷ
      dlg.showModal();
      const input = dlg.querySelector('.ask-input');
      if (input) input.select();
      else dlg.querySelector('[data-ask=yes]').focus();
    });
  }
  const confirmBox = (message, opts = {}) => ask({ ...opts, message });
  const alertBox = (message, opts = {}) => ask({ title: 'Thông báo', ok: 'Đã hiểu', cancel: '', ...opts, message });

  window.Account = { token: load, api, me, register, login, logout, requireLogin, xu, escapeHtml, toast,
    ask, confirm: confirmBox, alert: alertBox };
})();
