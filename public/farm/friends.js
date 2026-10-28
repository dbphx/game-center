// Sang nhà bạn (nông trại ?visit=<id>) và nhật ký trộm của nhà mình. farm.js vẽ cảnh như thường, module này lo phần riêng:
//   FarmVisit.id                — id chủ nhà đang thăm (0: đang ở nhà mình)
//   FarmVisit.load()            — tải nông trại bạn → { owner, dog, stealsLeft, farm } (farm cùng dạng state của mình)
//   FarmVisit.banner(v)         — băng "Đang ở nhà …" trên cảnh
//   FarmVisit.steal(plot)       — trộm một ô chín → stolen (chó đói) / chase (chó canh: phải chạy ra cổng) hoặc null khi lỗi
//   FarmVisit.finish(chase, ok) — kết thúc cuộc chó đuổi (ok = đã ra tới cổng) → stolen / caught
//   FarmVisit.flyToBag(el, id)  — hình nông sản bay từ ô vào túi trên băng
//   FarmVisit.notifyThefts(list)— báo vụ trộm mới kể từ lần trước (nhà mình)
//   FarmVisit.openLog(list)     — hộp thoại nhật ký trộm
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const esc = A.escapeHtml;
  const $ = (id) => document.getElementById(id);
  const IMG = '/img/toon/farm/';
  const SEEN_KEY = 'farm.theftsSeen';

  const id = Math.max(0, parseInt(new URLSearchParams(location.search).get('visit'), 10) || 0);
  const EMPTY_STORE = { capacity: 0, used: 0, maxCapacity: 0, nextExpandPrice: 0, items: {} };
  const look = (u) => ({ gender: u.gender, outfit: u.outfit, hair: u.hair || 'default', wings: u.wings || '', face: u.face || 'default' });
  // Mốc thời gian server: giây (nông trại); phòng khi là mili giây.
  const sec = (t) => (t > 1e12 ? t / 1000 : t || 0);

  let last = null; // lần tải gần nhất

  // Dạng trả về có thể là { owner, plots, dog, stealsLeft } hoặc kèm cả state nông trại ({ farm: {...} }).
  function normalize(r) {
    const f = r.farm || r;
    const plots = (f.plots || r.plots || []).map((p, i) => ({
      index: p.index != null ? p.index : i,
      crop: p.crop || '',
      plantedAt: p.plantedAt || 0,
      readyAt: p.readyAt || 0,
      stolen: !!(p.stolen || p.stolenBy),
      tried: !!p.tried, // mình đã thử trộm ô này vụ này (bị bắt cũng tính) — không thử lại được
    }));
    const now = f.now || r.now || Date.now() / 1000;
    return {
      owner: r.owner || { id, name: 'bạn', level: 1 },
      dog: r.dog || f.dog || null,
      stealsLeft: r.stealsLeft != null ? r.stealsLeft : f.stealsLeft != null ? f.stealsLeft : 0,
      farm: {
        now,
        plots,
        maxPlots: f.maxPlots || plots.length,
        nextPlotPrice: 0, // nhà bạn: không mua đất
        barn: f.barn || null,
        yard: f.yard || null,
        pond: f.pond || null,
        storage: { seeds: EMPTY_STORE, products: EMPTY_STORE },
        dog: r.dog || f.dog || null,
      },
    };
  }

  async function load() {
    try {
      last = normalize(await A.api(`/api/friends/${id}/farm`));
      return last;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      // Không phải bạn bè (403) / không có người này (404): báo rồi đưa về nhà.
      if (!last) {
        await A.alert(e.message || 'Không sang nhà bạn được', { title: 'Không vào được' });
        location.href = '/farm/';
      } else {
        A.toast(e.message);
      }
      return null;
    }
  }

  // Chó canh nhà khi còn no; có fedUntil thì tính theo giờ (đếm ngược không cần tải lại), không thì theo cờ server.
  const dogGuarding = (dog, now) => !!dog && (dog.fedUntil ? sec(dog.fedUntil) > now : !!dog.guarding);

  function banner(v) {
    const el = $('visit-bar');
    if (!el || !v) return;
    const now = v.farm.now;
    const guard = dogGuarding(v.dog, now);
    const ripe = v.farm.plots.filter((p) => p.crop && now >= p.readyAt && !p.stolen && !p.tried).length;
    el.innerHTML = `${C.head(look(v.owner), { cls: 'vb-avatar' })}
      <span class="vb-text"><span>Đang ở nhà <b>${esc(v.owner.name)}</b></span>
        <span class="vb-meta"><span class="vb-steal${v.stealsLeft ? '' : ' none'}" id="visit-bag">${I.svg('hand')}còn ${v.stealsLeft} lượt trộm<span class="vb-long"> hôm nay</span></span>
          <span class="vb-ripe">${I.svg('sprout')}${ripe} ô chín</span>
          <span class="vb-dog ${guard ? 'on' : 'off'}">${I.svg('shield')}Chó: ${guard ? 'đang canh' : 'đang đói'}</span></span></span>
      <a class="btn primary vb-home" href="/farm/">${I.svg('door')}Về nhà</a>`;
    el.classList.remove('hidden');
  }

  async function steal(plot) {
    try {
      const r = await A.api(`/api/friends/${id}/steal`, { method: 'POST', body: { plot } });
      // Server trả kèm nông trại bạn sau khi trộm (cùng dạng GET /api/friends/{id}/farm).
      if (r.farm) {
        last = normalize({ ...r.farm, owner: r.farm.owner || (last && last.owner) });
      } else if (last) {
        last.stealsLeft = Math.max(0, last.stealsLeft - 1);
      }
      return r;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      A.toast(e.message);
      return null;
    }
  }

  // Kết thúc cuộc chó đuổi: escaped = đã ra tới cổng. Server trả stolen (thoát, lấy được đồ) hoặc caught (bị cắn, mất máu).
  async function finish(chase, escaped) {
    try {
      const r = await A.api(`/api/friends/${id}/steal/finish`, { method: 'POST', body: { chase, escaped } });
      if (r.farm) last = normalize({ ...r.farm, owner: r.farm.owner || (last && last.owner) });
      return r;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      A.toast(e.message);
      return null;
    }
  }

  // Hình nông sản (item_<id>.png) bay từ ô vào túi trên băng.
  function flyToBag(from, item) {
    const bag = $('visit-bag');
    if (!from || !bag || !item) return;
    const a = from.getBoundingClientRect();
    const b = bag.getBoundingClientRect();
    const ghost = document.createElement('img');
    ghost.src = `${IMG}item_${item}.png`;
    ghost.className = 'steal-fly';
    ghost.alt = '';
    ghost.style.left = `${a.left + a.width / 2 - 22}px`;
    ghost.style.top = `${a.top + a.height / 2 - 22}px`;
    document.body.appendChild(ghost);
    requestAnimationFrame(() => {
      ghost.style.transform = `translate(${b.left + 14 - (a.left + a.width / 2)}px, ${b.top + b.height / 2 - 22 - (a.top + a.height / 2 - 22)}px) scale(.45)`;
      ghost.style.opacity = '0.2';
    });
    setTimeout(() => {
      ghost.remove();
      bag.classList.remove('pop');
      void bag.offsetWidth;
      bag.classList.add('pop');
    }, 750);
  }

  // ---------- Nhật ký trộm (nhà mình) ----------

  const norm = (list) => (list || []).map((t) => ({ ...t, at: sec(t.at), thief: t.thief || { name: t.thiefName || 'ai đó' } })).sort((x, y) => y.at - x.at);

  function seen() {
    try { return +localStorage.getItem(SEEN_KEY) || 0; } catch (e) { return 0; }
  }
  function markSeen(at) {
    try { localStorage.setItem(SEEN_KEY, String(at)); } catch (e) { /* bỏ qua */ }
  }

  // nameOf(item) → tên nông sản (farm.js truyền vào, lấy theo catalog).
  let nameOf = (item) => item;
  function setNames(fn) { nameOf = fn; }

  function line(t) {
    const what = `${t.qty || 1} ${esc(nameOf(t.item))}`;
    if (!t.caught) return `<b>${esc(t.thief.name)}</b> đã trộm ${what}`;
    const cost = t.hpLost ? ` (−${t.hpLost} máu)` : t.fine ? ` — phạt <b>${A.xu(t.fine)} xu</b> cho bạn` : '';
    return `Chó đuổi cắn <b>${esc(t.thief.name)}</b> khi định trộm ${what}${cost}`;
  }

  function notifyThefts(list) {
    const all = norm(list);
    if (!all.length) return;
    const mark = seen();
    const fresh = all.filter((t) => t.at > mark);
    markSeen(all[0].at);
    if (!fresh.length) return;
    const tmp = document.createElement('span');
    tmp.innerHTML = fresh.length === 1 ? line(fresh[0]) : `${fresh.length} vụ trộm mới — ${line(fresh[0])}`;
    A.toast(tmp.textContent);
    const btn = $('btn-thefts');
    if (btn) btn.classList.add('new');
  }

  function ago(at) {
    const s = Math.max(0, Date.now() / 1000 - at);
    if (s < 60) return 'vừa xong';
    if (s < 3600) return `${Math.floor(s / 60)} phút trước`;
    if (s < 86400) return `${Math.floor(s / 3600)} giờ trước`;
    return `${Math.floor(s / 86400)} ngày trước`;
  }

  function openLog(list) {
    const all = norm(list);
    const body = $('thefts-body');
    body.innerHTML = all.length
      ? `<ul class="theft-list">${all.map((t) => `<li class="${t.caught ? 'caught' : 'stolen'}">
          <img src="${IMG}item_${esc(t.item)}.png" alt="" width="34" height="34">
          <span>${line(t)}<small class="muted">${ago(t.at)}</small></span></li>`).join('')}</ul>`
      : '<p class="muted">Chưa có ai trộm nhà bạn. Cho chó ăn đều để nó canh nhà nhé!</p>';
    const btn = $('btn-thefts');
    if (btn) btn.classList.remove('new');
    if (all.length) markSeen(all[0].at);
    $('dlg-thefts').showModal();
  }

  window.FarmVisit = { id, load, banner, steal, finish, flyToBag, notifyThefts, openLog, setNames, dogGuarding, last: () => last };
})();
