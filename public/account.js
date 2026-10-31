// Tài khoản và ví dùng chung cho mọi trang trong DCity.
// Token phiên đăng nhập lưu trong localStorage nên mọi tab cùng trình duyệt là cùng một người.
(function () {
  'use strict';
  const KEY = 'hub.token';

  const load = () => { try { return localStorage.getItem(KEY); } catch (e) { return null; } };
  const save = (v) => { try { v ? localStorage.setItem(KEY, v) : localStorage.removeItem(KEY); } catch (e) { /* bỏ qua */ } };

  // Mỗi tài khoản chỉ một phiên: đăng nhập ở nơi khác thì phiên này bị đá ra (API trả 401 code "elsewhere",
  // WebSocket gửi {t:"auth", reason:"elsewhere"}). Báo người chơi rồi mới về trang đăng nhập; cờ trong
  // sessionStorage để nếu trang tự chuyển trước khi kịp bấm thì trang chủ vẫn hiện thông báo.
  const KICKED = 'hub.kicked';
  const RELOGIN = 'hub.relogin'; // vừa bị đá ra: trang chủ mở sẵn tab Đăng nhập
  const KICK_MSG = 'Tài khoản của bạn vừa đăng nhập ở thiết bị/trình duyệt khác, phiên này đã thoát.';
  let kicking = null;
  function kicked(token) {
    if (kicking) return kicking;
    // Tab khác cùng trình duyệt vừa đăng nhập lại: token mới đã có, chỉ cần tải lại trang.
    if (token && load() && load() !== token) {
      kicking = new Promise(() => location.reload());
      return kicking;
    }
    save(null);
    try {
      sessionStorage.setItem(KICKED, '1');
      localStorage.setItem(KICKED, String(Date.now())); // báo các tab khác cùng trình duyệt (token đã bị xoá)
    } catch (e) { /* bỏ qua */ }
    kicking = alertBox(KICK_MSG, { title: 'Phiên đã thoát' }).then(() => {
      try {
        sessionStorage.removeItem(KICKED);
        sessionStorage.setItem(RELOGIN, '1');
      } catch (e) { /* bỏ qua */ }
      if (location.pathname !== '/') location.href = '/?next=' + encodeURIComponent(location.pathname + location.search);
      else location.reload(); // trang chủ tải lại để hiện màn đăng nhập
    });
    return kicking;
  }

  async function api(path, { method = 'GET', body } = {}) {
    const headers = { 'Content-Type': 'application/json' };
    const token = load();
    if (token) headers.Authorization = 'Bearer ' + token;
    const res = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : undefined });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      const err = new Error(data.error || 'Lỗi kết nối, thử lại sau');
      err.status = res.status;
      err.code = data.code;
      if (res.status === 401 && data.code === 'elsewhere') kicked(token);
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

  // ---------- Đăng nhập bằng Google (Google Identity Services) ----------
  // Script của Google chỉ nạp khi cần hiện nút; bị chặn / lỗi mạng thì nút ẩn đi, form tên đăng nhập + mật khẩu vẫn dùng được.
  // GIS chỉ có một callback chung: nút nào đang hiện thì gán gHandler của nút đó.
  const GSI = 'https://accounts.google.com/gsi/client';
  let gsiLoad = null;
  let gsiClient = '';
  let gHandler = null;
  function loadGoogle() {
    if (gsiLoad) return gsiLoad;
    gsiLoad = new Promise((resolve, reject) => {
      if (window.google && window.google.accounts && window.google.accounts.id) return resolve(window.google);
      const s = document.createElement('script');
      s.src = GSI;
      s.async = true;
      const timer = setTimeout(() => reject(new Error('Không tải được Google')), 10000);
      s.onload = () => {
        clearTimeout(timer);
        if (window.google && window.google.accounts && window.google.accounts.id) resolve(window.google);
        else reject(new Error('Không tải được Google'));
      };
      s.onerror = () => { clearTimeout(timer); reject(new Error('Không tải được Google')); };
      document.head.appendChild(s);
    });
    gsiLoad.catch(() => { gsiLoad = null; }); // lần sau thử lại
    return gsiLoad;
  }

  // googleButton vẽ nút Google vào el (nút do Google vẽ); bấm xong gọi onCredential(credential).
  // Trả về true nếu hiện được nút; clientId rỗng hoặc script lỗi thì false (người gọi ẩn khung chứa nút).
  async function googleButton(el, clientId, onCredential, { text = 'signin_with' } = {}) {
    if (!el || !clientId) return false;
    try {
      const g = await loadGoogle();
      if (gsiClient !== clientId) {
        g.accounts.id.initialize({
          client_id: clientId,
          callback: (r) => gHandler && r && r.credential && gHandler(r.credential),
          ux_mode: 'popup',
          auto_select: false,
          cancel_on_tap_outside: true,
        });
        gsiClient = clientId;
      }
      gHandler = onCredential;
      el.innerHTML = '';
      // Nút Google rộng tối đa 400px, vừa với khung chứa (điện thoại 390px không tràn ngang).
      const width = Math.max(200, Math.min(400, Math.floor(el.clientWidth || (el.parentElement && el.parentElement.clientWidth) || 300)));
      g.accounts.id.renderButton(el, { type: 'standard', theme: 'outline', size: 'large', shape: 'pill', text, width, locale: 'vi' });
      return true;
    } catch (e) {
      return false;
    }
  }

  // Google đã liên kết → đăng nhập luôn ({token, user}); chưa có tài khoản → {needSignup, email, name, signup}.
  async function googleLogin(credential) {
    const data = await api('/api/login/google', { method: 'POST', body: { credential } });
    if (data.token) save(data.token);
    return data;
  }

  async function googleRegister(signup, name, look = {}) {
    const data = await api('/api/register/google', { method: 'POST', body: { signup, name, ...look } });
    save(data.token);
    return data.user;
  }

  async function logout() {
    try {
      if (window.google && window.google.accounts && window.google.accounts.id) window.google.accounts.id.disableAutoSelect();
    } catch (e) { /* bỏ qua */ }
    try {
      await api('/api/logout', { method: 'POST' });
    } finally {
      save(null);
    }
  }

  // Chuyển về hub để đăng nhập rồi quay lại đúng trang này. Nếu lý do là tài khoản vừa đăng nhập ở nơi khác
  // (vd WebSocket báo {t:"auth"}) thì hỏi lại server để hiện thông báo trước khi chuyển.
  async function requireLogin() {
    if (!kicking && load()) await api('/api/me').catch(() => {});
    if (kicking) return kicking;
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

  // Service worker giữ ảnh trong bộ đệm của trình duyệt (js / css hỏi lại bằng ETag), lần sau mở trang không phải tải lại
  // (server đổi phiên bản mỗi lần cập nhật nên bộ đệm cũ tự bị xoá).
  if ('serviceWorker' in navigator && location.protocol !== 'file:') {
    window.addEventListener('load', () => navigator.serviceWorker.register('/sw.js').catch(() => {}));
  }

  // Quay lại tab / cửa sổ thì hỏi lại phiên ngay (không đợi lần cập nhật định kỳ), để biết sớm nếu đã bị
  // đăng nhập nơi khác đá ra.
  let lastCheck = 0;
  const recheck = () => {
    if (document.hidden || !load() || kicking || Date.now() - lastCheck < 5000) return;
    lastCheck = Date.now();
    api('/api/me').catch(() => {});
  };
  window.addEventListener('focus', recheck);
  window.addEventListener('storage', (e) => { if (e.key === KICKED && e.newValue) kicked(); });
  document.addEventListener('visibilitychange', recheck);

  // Trang trước bị chuyển đi khi chưa kịp đọc thông báo bị đá ra thì hiện lại ở đây.
  try {
    if (sessionStorage.getItem(KICKED)) {
      sessionStorage.removeItem(KICKED);
      const show = () => alertBox(KICK_MSG, { title: 'Phiên đã thoát' });
      if (document.body) setTimeout(show, 0);
      else window.addEventListener('DOMContentLoaded', show);
    }
  } catch (e) { /* bỏ qua */ }

  // relogin: trang này mở ngay sau khi bị đá ra (để trang chủ chọn tab Đăng nhập thay vì Đăng ký).
  let relogin = false;
  try {
    relogin = sessionStorage.getItem(RELOGIN) === '1';
    sessionStorage.removeItem(RELOGIN);
  } catch (e) { /* bỏ qua */ }

  window.Account = { token: load, api, me, register, login, logout, requireLogin, xu, escapeHtml, toast,
    ask, confirm: confirmBox, alert: alertBox, relogin, googleButton, googleLogin, googleRegister };
})();
