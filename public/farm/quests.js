// Bảng nhiệm vụ trong nông trại: nhấn NPC trong vườn (#farm-npc, sự kiện "farm:npc" — public/farm/season.js) để mở.
// Tab Ngày / Tuần / Mảnh xe: nhận thưởng, đổi nhiệm vụ, mở rương, ghép xe từ mảnh, đổi mảnh thừa lên bậc trên.
// Mọi luật do server quyết định (/api/quests, games/quests); trang này chỉ hiển thị và gửi thao tác.
// Dấu "!" trên đầu NPC (class has-quest) bật khi có thưởng chờ nhận (`pending`).
(function () {
  'use strict';
  const A = window.Account;
  if (!A) return;
  const I = window.Icons;
  const esc = A.escapeHtml;
  const xu = A.xu || ((n) => Number(n).toLocaleString('vi-VN'));
  const IMG = '/img/quests/';
  const FARM = '/img/toon/farm/';
  const VISIT = new URLSearchParams(location.search).has('visit');
  const icon = (n) => (I && I.svg ? I.svg(n) : '');

  const TIERS = {
    common: { name: 'Thường', next: 'rare' },
    rare: { name: 'Hiếm', next: 'epic' },
    epic: { name: 'Sử thi', next: 'exclusive' },
    exclusive: { name: 'Độc quyền' },
  };
  const TIER_ORDER = ['common', 'rare', 'epic', 'exclusive'];
  // Hình minh hoạ từng loại nhiệm vụ (ảnh sẵn có của nông trại / xe đua).
  const KIND_IMG = {
    farm_harvest: FARM + 'crop_corn_ripe.png',
    farm_plant: FARM + 'seed_carrot.png',
    farm_product: FARM + 'item_egg.png',
    farm_animal: FARM + 'animal_pig.png',
    farm_zombie: FARM + 'zombie.png',
    farm_cloud: FARM + 'bean_2.png',
    farm_visit: FARM + 'pet3d_dog.png',
    fish_catch: FARM + 'fish_koi.png',
    race_finish: IMG + 'cars/comet.png',
    race_win: IMG + 'cars/nova.png',
  };
  const thumb = (item) => `${IMG}cars/${item}.png`;
  const GREET = {
    santa: 'Hô hô hô! Chăm vườn giỏi thì ông tặng mảnh ghép xe đua. Gom đủ mảnh là có xe mới!',
    farmer: 'Chào cháu! Làm giúp bác mấy việc trong vườn, bác thưởng mảnh ghép xe đua nhé.',
  };

  let state = null;
  let tab = 'daily';
  let busy = false;
  let dlg = null;
  let tick = null;

  // ---------- Gọi API ----------
  async function load() {
    if (!A.token()) return null;
    try {
      state = await A.api('/api/quests');
    } catch (e) {
      if (e.status !== 401) console.warn('nhiệm vụ:', e.message);
      return null;
    }
    markNpc();
    return state;
  }
  async function act(path, body) {
    if (busy) return null;
    busy = true;
    try {
      const r = await A.api('/api/quests/' + path, { method: 'POST', body });
      if (r.quests) state = r.quests;
      markNpc();
      render();
      return r;
    } catch (e) {
      A.toast(e.status === 402 ? 'Không đủ xu' : e.message);
      return null;
    } finally {
      busy = false;
    }
  }
  // Số dư trên thanh đầu trang (farm.js giữ bản riêng; đổi nhiệm vụ trừ xu thì cập nhật chữ ngay).
  async function syncBalance() {
    try {
      const u = await A.me();
      const el = document.getElementById('balance');
      if (u && el) el.textContent = xu(u.balance);
    } catch (e) { /* bỏ qua */ }
  }

  // ---------- Dấu "!" trên NPC ----------
  function markNpc() {
    const npc = document.getElementById('farm-npc');
    if (!npc || !state) return;
    npc.classList.toggle('has-quest', state.pending > 0);
    npc.title = state.pending > 0 ? `${npcName()} — có ${state.pending} phần thưởng chờ nhận` : npcName();
  }
  const npcName = () => (state && state.npc === 'farmer' ? 'Bác nông dân' : 'Ông già Noel');

  // ---------- Hộp thoại ----------
  function ensureDlg() {
    if (dlg) return dlg;
    dlg = document.createElement('dialog');
    dlg.className = 'modal shop-dlg quest-dlg';
    dlg.id = 'dlg-quests';
    dlg.innerHTML = `<div class="shop-wrap quest-wrap">
      <div class="quest-top">
        <img class="quest-npc" alt="" draggable="false">
        <div class="quest-say"><h2 class="quest-title"></h2><p class="quest-greet"></p></div>
        <button class="link icon-only quest-x" data-qclose title="Đóng">${icon('x')}</button>
      </div>
      <div class="quest-tabs" role="tablist">
        <button data-qtab="daily" role="tab">Ngày</button>
        <button data-qtab="weekly" role="tab">Tuần</button>
        <button data-qtab="shards" role="tab">Mảnh xe</button>
      </div>
      <div class="quest-body"></div>
    </div>
    <div class="quest-reveal hidden" aria-live="polite"></div>`;
    document.body.appendChild(dlg);
    dlg.addEventListener('click', onClick);
    dlg.addEventListener('close', () => clearInterval(tick));
    // Bấm ra ngoài khung thì đóng.
    dlg.addEventListener('mousedown', (e) => { if (e.target === dlg) dlg.close(); });
    return dlg;
  }

  async function open() {
    if (VISIT) return A.toast('Về vườn nhà mình để nhận nhiệm vụ');
    if (!A.token()) return A.requireLogin();
    ensureDlg();
    if (!state) await load();
    if (!state) return A.toast('Chưa tải được nhiệm vụ, thử lại sau');
    // Mặc định mở tab có thưởng chờ nhận.
    const ready = (list) => list.some((q) => q.done && !q.claimed);
    if (ready(state.daily) || (state.chest.daily.ready && !state.chest.daily.claimed)) tab = 'daily';
    else if (ready(state.weekly) || (state.chest.weekly.ready && !state.chest.weekly.claimed)) tab = 'weekly';
    render();
    if (!dlg.open) dlg.showModal();
    clearInterval(tick);
    tick = setInterval(renderClock, 1000);
    load().then(() => dlg.open && render());
  }

  function render() {
    if (!dlg || !state) return;
    const who = state.npc === 'farmer' ? 'farmer' : 'santa';
    const npcImg = dlg.querySelector('.quest-npc');
    npcImg.src = `/img/toon/farm/noel/npc_${who}_0.png`;
    dlg.querySelector('.quest-title').textContent = npcName();
    dlg.querySelector('.quest-greet').textContent = GREET[who];
    const counts = {
      daily: state.daily.filter((q) => q.done && !q.claimed).length + (state.chest.daily.ready && !state.chest.daily.claimed ? 1 : 0),
      weekly: state.weekly.filter((q) => q.done && !q.claimed).length + (state.chest.weekly.ready && !state.chest.weekly.claimed ? 1 : 0),
      shards: state.shards.filter((s) => !s.owned && s.qty >= s.need).length,
    };
    for (const b of dlg.querySelectorAll('[data-qtab]')) {
      const k = b.dataset.qtab;
      b.classList.toggle('on', k === tab);
      b.setAttribute('aria-selected', k === tab);
      b.innerHTML = `${b.textContent.replace(/\d+$/, '').trim()}${counts[k] ? `<span class="q-dot">${counts[k]}</span>` : ''}`;
    }
    const body = dlg.querySelector('.quest-body');
    body.innerHTML = tab === 'shards' ? shardsHtml() : listHtml(tab);
    renderClock();
  }

  // Đếm ngược tới lúc làm mới (resetAt theo giây Unix).
  function renderClock() {
    if (!dlg || !state) return;
    const el = dlg.querySelector('[data-clock]');
    if (!el) return;
    const at = state.resetAt[el.dataset.clock];
    let s = Math.max(0, Math.floor(at - Date.now() / 1000));
    if (s === 0) { load().then(render); return; }
    const d = Math.floor(s / 86400);
    s -= d * 86400;
    const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), ss = s % 60;
    const pad = (n) => String(n).padStart(2, '0');
    el.textContent = d ? `${d} ngày ${pad(h)}:${pad(m)}` : `${pad(h)}:${pad(m)}:${pad(ss)}`;
  }

  function listHtml(kind) {
    const list = state[kind];
    const chest = state.chest[kind];
    const daily = kind === 'daily';
    const done = list.filter((q) => q.done).length;
    const rerollTxt = state.rerollsLeft > 0 ? (state.rerollCost ? `${xu(state.rerollCost)} xu` : 'miễn phí') : '';
    const rows = list.map((q) => {
      const pct = Math.round((Math.min(q.progress, q.target) / Math.max(1, q.target)) * 100);
      let btn;
      if (q.claimed) btn = `<span class="q-claimed">${icon('check')}Đã nhận</span>`;
      else if (q.done) btn = `<button class="primary q-claim" data-claim="${esc(q.id)}">Nhận</button>`;
      else if (daily && state.rerollsLeft > 0) btn = `<button class="q-reroll" data-reroll="${esc(q.id)}" title="Đổi lấy nhiệm vụ khác (tiến độ về 0)">Đổi<small>${rerollTxt}</small></button>`;
      else btn = '';
      return `<div class="q-row ${q.done ? 'done' : ''} ${q.claimed ? 'claimed' : ''}">
        <img class="q-ic" src="${KIND_IMG[q.kind] || IMG + 'shard_common.png'}" alt="" draggable="false">
        <div class="q-main"><b>${esc(q.name)}</b>
          <div class="q-bar"><span style="width:${pct}%"></span><em>${Math.min(q.progress, q.target)}/${q.target}</em></div>
          <small class="q-reward"><img src="${IMG}shard_common.png" alt="">${q.reward.shards} mảnh · +${q.reward.xp} XP</small>
        </div>
        <div class="q-act">${btn}</div>
      </div>`;
    }).join('');
    const chestImg = `${IMG}chest_${kind}${chest.claimed ? '_open' : ''}.png`;
    const chestBtn = chest.claimed ? '<span class="q-claimed">Đã mở</span>'
      : chest.ready ? `<button class="primary q-claim" data-chest="${kind}">Mở rương</button>`
      : `<span class="muted">${done}/${list.length} nhiệm vụ</span>`;
    return `<div class="q-head"><span>${daily ? 'Làm mới sau' : 'Tuần mới sau'} <b data-clock="${daily ? 'day' : 'week'}">--:--</b></span>
        ${daily ? `<span class="muted">Đổi nhiệm vụ: còn ${state.rerollsLeft} lượt${state.rerollsLeft ? ` · lần tới ${rerollTxt}` : ''}</span>` : ''}</div>
      <div class="q-list">${rows}</div>
      <div class="q-chest ${chest.ready && !chest.claimed ? 'ready' : ''}">
        <img src="${chestImg}" alt="" draggable="false">
        <div><b>${daily ? 'Rương ngày' : 'Rương tuần'}</b><small>${daily ? 'Xong cả 5 nhiệm vụ ngày: thêm 1 mảnh' : 'Xong cả 3 nhiệm vụ tuần: 1 mảnh Độc quyền'}</small></div>
        ${chestBtn}
      </div>`;
  }

  function shardsHtml() {
    const by = {};
    for (const s of state.shards) (by[s.tier] = by[s.tier] || []).push(s);
    const rates = state.rates || {};
    const sections = TIER_ORDER.filter((t) => by[t]).map((t) => {
      const spare = by[t].reduce((n, s) => n + (s.owned ? s.qty : Math.max(0, s.qty - s.need)), 0);
      const next = TIERS[t].next;
      const ex = next ? `<button class="q-ex" data-exchange="${t}" ${spare >= 3 ? '' : 'disabled'} title="3 mảnh thừa bậc ${TIERS[t].name} → 1 mảnh ngẫu nhiên bậc ${TIERS[next].name}">Đổi 3 mảnh thừa → ${TIERS[next].name} <small>(thừa ${spare})</small></button>` : '';
      const cards = by[t].map((s) => {
        const full = s.qty >= s.need;
        const pct = Math.round((Math.min(s.qty, s.need) / s.need) * 100);
        const state2 = s.owned ? `<span class="q-owned">${icon('check')}Đã có</span>`
          : full ? `<button class="primary q-assemble" data-assemble="${esc(s.item)}">Ghép</button>`
          : `<span class="muted">Còn thiếu ${s.need - s.qty}</span>`;
        return `<div class="q-card tier-${t} ${s.owned ? 'owned' : ''} ${full && !s.owned ? 'full' : ''}" title="${esc(s.name)}">
          <img class="q-thumb" src="${thumb(s.item)}" alt="" draggable="false" onerror="this.src='${IMG}shard_${t}.png'">
          <b>${esc(s.name)}</b><small>${s.kind === 'car' ? 'Xe' : 'Đồ trang trí'}</small>
          <div class="q-bar sm"><span style="width:${pct}%"></span><em>${s.qty}/${s.need}</em></div>
          ${state2}
        </div>`;
      }).join('');
      return `<section class="q-tier tier-${t}"><h3><img src="${IMG}shard_${t}.png" alt="">${TIERS[t].name}<small>rơi ${(rates[t] || 0) / 10}%</small>${ex}</h3>
        <div class="q-grid">${cards}</div></section>`;
    }).join('');
    return `<p class="muted q-note">Mỗi phần thưởng là mảnh ngẫu nhiên. Đủ mảnh thì ghép ra xe (không cần đủ cấp); xe ghép xong có trong garage Trường đua.</p>${sections}`;
  }

  // ---------- Thao tác ----------
  async function onClick(e) {
    const t = e.target;
    if (t.closest('[data-qclose]')) return dlg.close();
    const tb = t.closest('[data-qtab]');
    if (tb) { tab = tb.dataset.qtab; return render(); }
    if (t.closest('.quest-reveal')) return hideReveal();
    const c = t.closest('[data-claim]');
    if (c) {
      const r = await act('claim', { id: c.dataset.claim });
      if (r) reveal(r.gained, 'Nhận thưởng nhiệm vụ');
      return;
    }
    const ch = t.closest('[data-chest]');
    if (ch) {
      const r = await act('chest', { kind: ch.dataset.chest });
      if (r) reveal(r.gained, ch.dataset.chest === 'daily' ? 'Rương ngày' : 'Rương tuần', ch.dataset.chest);
      return;
    }
    const rr = t.closest('[data-reroll]');
    if (rr) {
      const cost = state.rerollCost;
      if (cost) {
        const ok = await A.confirm(`Đổi nhiệm vụ này lấy nhiệm vụ khác với giá ${xu(cost)} xu?`, { title: 'Đổi nhiệm vụ', ok: 'Đổi' });
        if (!ok) return;
      }
      const r = await act('reroll', { id: rr.dataset.reroll });
      if (r) {
        A.toast(r.cost ? `Đã đổi nhiệm vụ (−${xu(r.cost)} xu)` : 'Đã đổi nhiệm vụ');
        if (r.cost) syncBalance();
      }
      return;
    }
    const as = t.closest('[data-assemble]');
    if (as) {
      const s = state.shards.find((x) => x.item === as.dataset.assemble);
      const ok = await A.confirm(`Dùng ${s.need} mảnh để ghép ${s.name}?`, { title: 'Ghép mảnh', ok: 'Ghép' });
      if (!ok) return;
      const r = await act('assemble', { item: s.item });
      if (r) revealGrant(s);
      return;
    }
    const ex = t.closest('[data-exchange]');
    if (ex) {
      const tier = ex.dataset.exchange;
      const ok = await A.confirm(`Đổi 3 mảnh thừa bậc ${TIERS[tier].name} lấy 1 mảnh ngẫu nhiên bậc ${TIERS[TIERS[tier].next].name}?`, { title: 'Đổi mảnh thừa', ok: 'Đổi' });
      if (!ok) return;
      const r = await act('exchange', { tier });
      if (r) reveal(r.gained, 'Đổi mảnh');
    }
  }

  // ---------- Hoạt ảnh mở mảnh ----------
  function reveal(gained, title, chest) {
    if (!gained || !gained.length) return;
    const box = dlg.querySelector('.quest-reveal');
    const total = gained.reduce((n, g) => n + g.qty, 0);
    box.innerHTML = `<div class="qr-card">
      ${chest ? `<img class="qr-chest" src="${IMG}chest_${chest}_open.png" alt="">` : ''}
      <h3>${esc(title)}</h3>
      <div class="qr-list">${gained.map((g, i) => `<div class="qr-item tier-${g.tier}" style="--i:${i}">
        <span class="qr-glow"></span>
        <img class="qr-thumb" src="${thumb(g.item)}" alt="" onerror="this.src='${IMG}shard_${g.tier}.png'">
        <img class="qr-shard" src="${IMG}shard_${g.tier}.png" alt="">
        <b>${esc(g.name)}</b><small>${TIERS[g.tier] ? TIERS[g.tier].name : ''} · +${g.qty} mảnh</small>
      </div>`).join('')}</div>
      <p class="muted">Nhận ${total} mảnh · bấm để đóng</p>
    </div>`;
    box.classList.remove('hidden');
    void box.offsetWidth;
    box.classList.add('show');
  }
  function revealGrant(s) {
    const box = dlg.querySelector('.quest-reveal');
    box.innerHTML = `<div class="qr-card grant tier-${s.tier}">
      <h3>Ghép thành công!</h3>
      <div class="qr-item big tier-${s.tier}" style="--i:0"><span class="qr-glow"></span>
        <img class="qr-thumb" src="${thumb(s.item)}" alt="" onerror="this.src='${IMG}shard_${s.tier}.png'">
        <b>${esc(s.name)}</b><small>${s.kind === 'car' ? 'Xe mới đã vào garage Trường đua' : 'Đồ trang trí đã vào garage Trường đua'}</small></div>
      <p class="muted">Bấm để đóng</p></div>`;
    box.classList.remove('hidden');
    void box.offsetWidth;
    box.classList.add('show');
  }
  function hideReveal() {
    const box = dlg.querySelector('.quest-reveal');
    box.classList.remove('show');
    box.classList.add('hidden');
  }

  // ---------- Gắn kết ----------
  document.addEventListener('farm:npc', open);
  // Thao tác nông trại xong (POST /api/farm/..., /api/friends/...) → tải lại tiến độ (gộp nhiều thao tác liền nhau).
  let pendingLoad = null;
  const origFetch = window.fetch.bind(window);
  window.fetch = function (input, init) {
    const p = origFetch(input, init);
    try {
      const url = typeof input === 'string' ? input : input.url;
      const method = (init && init.method) || (input && input.method) || 'GET';
      if (method !== 'GET' && /\/api\/(farm|friends|fishing)\//.test(url)) {
        p.then(() => {
          clearTimeout(pendingLoad);
          pendingLoad = setTimeout(() => load().then(() => dlg && dlg.open && render()), 1200);
        }).catch(() => {});
      }
    } catch (e) { /* bỏ qua */ }
    return p;
  };
  // NPC do season.js dựng sau khi cảnh vẽ: dò vài lần để gắn dấu "!" đúng; sau đó làm mới định kỳ.
  let tries = 0;
  const watch = setInterval(() => {
    if (document.getElementById('farm-npc') && state) { markNpc(); if (++tries > 3) clearInterval(watch); }
  }, 1500);
  if (!VISIT) {
    load();
    setInterval(() => { if (!document.hidden) load(); }, 60000);
  }
  window.FarmQuests = { open, reload: load };
})();
