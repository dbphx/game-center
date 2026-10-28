// Bạn bè DCity: danh sách bạn, lời mời kết bạn (theo tên đăng nhập), sang nhà bạn (nông trại) để trộm cây chín.
// Dùng chung: trang chủ (bảng "Bạn bè") và các trang khác cần gửi lời mời (vd bấm người chơi ở quảng trường).
//   Friends.mount(el)      — vẽ bảng bạn bè vào el (tự tải, tự vẽ lại sau mỗi thao tác)
//   Friends.request(body)  — gửi lời mời: { username } hoặc { id }
//   Friends.load()         — tải danh sách { friends, incoming, outgoing }
//   Friends.onChange(fn)   — gọi fn(data) mỗi lần danh sách đổi (vd số lời mời đến trên nút ở thanh trên cùng)
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const esc = A.escapeHtml;

  let data = null; // { friends, incoming, outgoing }
  let err = '';
  let busy = false;
  let box = null; // phần tử đang vẽ bảng
  const listeners = [];

  const look = (u) => ({ gender: u.gender, outfit: u.outfit, hair: u.hair || 'default', wings: u.wings || '', face: u.face || 'default' });
  const norm = (d) => ({ friends: (d && d.friends) || [], incoming: (d && d.incoming) || [], outgoing: (d && d.outgoing) || [] });

  function set(d) {
    data = norm(d);
    err = '';
    listeners.forEach((fn) => fn(data));
    draw();
  }

  async function load() {
    try {
      set(await A.api('/api/friends'));
    } catch (e) {
      if (e.status === 401) return null;
      err = e.message || 'Không tải được danh sách bạn bè';
      draw();
    }
    return data;
  }

  // Gửi một thao tác; server trả lại cả danh sách mới.
  async function act(path, body, okMsg) {
    if (busy) return null;
    busy = true;
    draw();
    try {
      const d = await A.api(path, { method: 'POST', body });
      set(d);
      if (okMsg) A.toast(okMsg);
      return d;
    } catch (e) {
      A.toast(e.message);
      return null;
    } finally {
      busy = false;
      draw();
    }
  }
  const request = (body) => act('/api/friends/request', body, 'Đã gửi lời mời kết bạn');

  function who(u) {
    return `${C.head(look(u), { cls: 'fr-avatar' })}
      <span class="fr-who"><span class="fr-name"><span>${esc(u.name)}</span><i class="fr-lv">Lv ${u.level || 1}</i></span>`;
  }

  function friendRow(u) {
    const ripe = u.ripe || 0;
    const dog = u.dogGuarding
      ? `<span class="fr-dog on" title="Chó no — đang canh nhà">${I.svg('shield')}Chó đang canh</span>`
      : `<span class="fr-dog off" title="Chó đói — không canh nhà">${I.svg('shield')}Chó đang đói</span>`;
    return `<li class="fr-row">${who(u)}
        <span class="fr-meta muted"><span class="fr-ripe${ripe ? ' has' : ''}">${I.svg('sprout')}${ripe ? `${ripe} ô chín` : 'chưa có ô chín'}</span>${dog}</span></span>
      <span class="fr-btns">
        <a class="btn primary fr-visit" href="/farm/?visit=${encodeURIComponent(u.id)}" title="Sang nông trại của ${esc(u.name)}">${I.svg('door')}Sang nhà</a>
        <button class="link icon-only" data-fr-remove="${u.id}" data-fr-name="${esc(u.name)}" title="Huỷ kết bạn">${I.svg('x')}</button>
      </span></li>`;
  }

  function inRow(u) {
    return `<li class="fr-row invite">${who(u)}<span class="fr-meta muted">muốn kết bạn với bạn</span></span>
      <span class="fr-btns">
        <button class="primary" data-fr-accept="${u.id}" ${busy ? 'disabled' : ''}>${I.svg('check')}Đồng ý</button>
        <button data-fr-decline="${u.id}" ${busy ? 'disabled' : ''}>Từ chối</button>
      </span></li>`;
  }

  function outRow(u) {
    return `<li class="fr-row sent">${who(u)}<span class="fr-meta muted">đang chờ đồng ý</span></span>
      <span class="fr-btns"><button data-fr-cancel="${u.id}" ${busy ? 'disabled' : ''}>Huỷ lời mời</button></span></li>`;
  }

  function html() {
    const form = `<form class="fr-add" id="fr-add" autocomplete="off">
        <span class="input-icon"><i data-icon="user"></i><input name="username" placeholder="Tên đăng nhập của bạn bè" autocapitalize="none" maxlength="20" required></span>
        <button class="primary" type="submit" ${busy ? 'disabled' : ''}>${I.svg('userPlus')}Kết bạn</button>
      </form>`;
    if (!data) return `${form}<p class="fr-empty ${err ? '' : 'muted'}">${esc(err || 'Đang tải danh sách bạn bè…')}</p>`;
    const { friends, incoming, outgoing } = data;
    return `${form}
      ${incoming.length ? `<h3>${I.svg('gift')}Lời mời kết bạn <span class="fr-count">${incoming.length}</span></h3><ul class="fr-list">${incoming.map(inRow).join('')}</ul>` : ''}
      <h3>${I.svg('users')}Bạn bè <span class="fr-count">${friends.length}</span></h3>
      ${friends.length
        ? `<ul class="fr-list">${friends.map(friendRow).join('')}</ul>`
        : '<p class="fr-empty muted">Chưa có bạn nào — nhập tên đăng nhập ở trên để gửi lời mời.</p>'}
      ${outgoing.length ? `<h3>${I.svg('timer')}Đã gửi lời mời</h3><ul class="fr-list">${outgoing.map(outRow).join('')}</ul>` : ''}
      <p class="fr-note muted">Sang nhà bạn để trộm cây đã chín (mỗi ô một lần, tối đa 5 lần mỗi ngày một nhà).
        Chó nhà bạn mà no thì canh nhà và đuổi kẻ trộm: chạy ra cổng là thoát, bị đuổi kịp thì mất 50 máu.
        Nhớ cho chó nhà mình ăn — một bữa no 4 giờ, đói là chó bỏ canh.</p>
      ${err ? `<p class="fr-empty">${esc(err)}</p>` : ''}`;
  }

  function draw() {
    if (!box || !box.isConnected) return;
    // Giữ chữ đang gõ dở trong ô tên khi vẽ lại.
    const input = box.querySelector('#fr-add input');
    const typed = input ? input.value : '';
    const focused = input && document.activeElement === input;
    box.innerHTML = html();
    I.render(box);
    const now = box.querySelector('#fr-add input');
    if (now) {
      now.value = typed;
      if (focused) now.focus();
    }
  }

  function bind(el) {
    el.addEventListener('submit', (e) => {
      if (!e.target.matches('#fr-add')) return;
      e.preventDefault();
      const name = new FormData(e.target).get('username').trim();
      if (!name) return;
      request({ username: name }).then((d) => {
        const input = el.querySelector('#fr-add input');
        if (d && input) input.value = '';
      });
    });
    el.addEventListener('click', async (e) => {
      const b = e.target.closest('button');
      if (!b) return;
      const d = b.dataset;
      if (d.frAccept) return act('/api/friends/accept', { id: +d.frAccept }, 'Đã kết bạn');
      if (d.frDecline) return act('/api/friends/decline', { id: +d.frDecline });
      if (d.frCancel) return act('/api/friends/remove', { id: +d.frCancel }, 'Đã huỷ lời mời');
      if (d.frRemove) {
        const ok = await A.confirm(`Huỷ kết bạn với ${d.frName}? Hai người sẽ không sang nhà nhau được nữa.`, { title: 'Huỷ kết bạn', ok: 'Huỷ kết bạn', cancel: 'Giữ lại', danger: true });
        if (ok) act('/api/friends/remove', { id: +d.frRemove }, 'Đã huỷ kết bạn');
      }
    });
  }

  function mount(el) {
    if (el.dataset.frBound !== '1') {
      bind(el);
      el.dataset.frBound = '1';
    }
    box = el;
    draw();
    load();
  }

  window.Friends = {
    mount,
    load,
    request,
    data: () => data,
    onChange: (fn) => listeners.push(fn),
  };
})();
