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

  window.Account = { token: load, api, me, register, login, logout, requireLogin, xu, escapeHtml, toast };
})();
