// Bầu Cua Tôm Cá của DCity: một sòng chung cho cả thành phố, mỗi vòng 30 giây (đặt cược 20 s → lắc 4 s → mở bát 6 s).
// Server quyết định mọi thứ (/api/baucua/...): trang này chỉ hiển thị, đếm ngược theo giờ server và gửi lệnh đặt / huỷ cược.
// Hình hoạt hình CC0 ở /img/toon/baucua (xem manifest.json, CREDITS.csv).
(function () {
  'use strict';
  const A = window.Account;
  const I = window.Icons;
  const C = window.Character;
  const $ = (id) => document.getElementById(id);
  const esc = A.escapeHtml;
  const xu = A.xu;

  const IMG = '/img/toon/baucua/';
  const FACES = [
    { id: 'bau', name: 'Bầu' }, { id: 'cua', name: 'Cua' }, { id: 'tom', name: 'Tôm' },
    { id: 'ca', name: 'Cá' }, { id: 'ga', name: 'Gà' }, { id: 'nai', name: 'Nai' },
  ];
  const XP_PLAY = 2; // khớp games/baucua: XP mỗi vòng có đặt cược
  const XP_WIN = 3; // thêm khi vòng đó có lời
  const PHASES = { betting: 'Đặt cược', rolling: 'Nhà cái lắc', result: 'Mở bát' };
  const faceImg = (id, size, cls = '') => `<img class="face ${cls}" src="${IMG}face_${id}.png" width="${size}" height="${size}" alt="" draggable="false">`;
  const chipImg = (v, size = 34, cls = '') => `<img class="chip-img ${cls}" src="${IMG}chip_${v}.png" width="${size}" height="${Math.round(size * 1.05)}" alt="" draggable="false">`;

  let user = null;
  let st = null; // trạng thái sòng mới nhất
  let offset = 0; // giờ server − giờ máy (ms)
  let chip = 10; // mức chip đang chọn
  let seen = { round: null, phase: null }; // để chỉ chạy hiệu ứng một lần mỗi pha
  let busy = false;
  let pollTimer = null;

  const serverNow = () => Date.now() + offset;

  // Ghi HTML chỉ khi nội dung đổi — poll mỗi giây không làm hiệu ứng (xúc xắc rơi, ô sáng) chạy lại.
  function setHTML(id, html) {
    const el = $(id);
    if (el._html !== html) {
      el._html = html;
      el.innerHTML = html;
    }
  }

  // Chip gần nhất không lớn hơn số xu (để chồng chip trên ô cược).
  function chipStack(amount) {
    const vals = ((st && st.chips) || [10, 50, 100, 500, 1000]).slice().sort((a, b) => b - a);
    const out = [];
    for (const v of vals) {
      while (amount >= v && out.length < 6) {
        out.push(v);
        amount -= v;
      }
    }
    return out.reverse();
  }

  // ---------- Topbar ----------

  function renderHUD() {
    if (!user) return;
    $('balance').textContent = xu(user.balance);
    $('user-name').textContent = user.name;
    $('user-avatar').innerHTML = C.head(user);
    const pct = Math.min(100, Math.round(((user.xp - user.levelXP) / Math.max(1, user.nextXP - user.levelXP)) * 100));
    $('level-badge').innerHTML = `<span class="lv">Lv ${user.level}</span><span class="xp-bar"><span style="width:${pct}%"></span></span>`;
    $('level-badge').title = `${user.xp - user.levelXP} / ${user.nextXP - user.levelXP} XP để lên level ${user.level + 1}`;
    if (user.maxHp) {
      const hpPct = Math.round((user.hp / user.maxHp) * 100);
      const sick = user.hospitalUntil && user.hospitalUntil > user.now;
      const pill = $('hp-pill');
      pill.className = `hp-pill${hpPct < 30 || sick ? ' low' : ''}`;
      pill.title = sick ? 'Đang nằm viện' : 'Vào Casino cần ít nhất 30 HP';
      pill.innerHTML = `${I.svg('heart')}<span class="hp-bar"><span style="width:${hpPct}%"></span></span><b>${user.hp}/${user.maxHp}</b>`;
    }
  }

  // ---------- Vẽ sòng ----------

  function renderBoard() {
    const totals = st.totals || {};
    const bets = (st.mine && st.mine.bets) || {};
    const faces = st.faces && st.faces.length ? st.faces : FACES;
    const open = st.phase === 'betting';
    const counts = {};
    for (const d of st.dice || []) counts[d] = (counts[d] || 0) + 1;
    setHTML('board', faces
      .map((f) => {
        const mine = bets[f.id] || 0;
        const hit = st.phase === 'result' && counts[f.id];
        const stack = chipStack(mine)
          .map((v, i) => `<span class="stack-chip" style="--i:${i}">${chipImg(v, 30)}</span>`)
          .join('');
        return `<button class="cell${hit ? ' win' : ''}${mine ? ' mine' : ''}" data-face="${f.id}" ${open ? '' : 'aria-disabled="true"'}>
          ${hit ? `<span class="hit-badge">×${counts[f.id]}</span>` : ''}
          ${faceImg(f.id, 104)}
          <span class="cell-name">${esc(f.name)}</span>
          <span class="pot" title="Tổng cược cả sòng">${I.svg('users')}${xu(totals[f.id] || 0)}</span>
          <span class="my-bet${mine ? '' : ' empty'}"><span class="stack">${stack}</span><b>${mine ? xu(mine) : ''}</b></span>
        </button>`;
      })
      .join(''));
  }

  function renderChips() {
    const chips = (st && st.chips) || [10, 50, 100, 500, 1000];
    if (!chips.includes(chip)) chip = chips[0];
    setHTML('chips', chips
      .map((v) => `<button class="chip-btn${v === chip ? ' active' : ''}" data-chip="${v}" role="radio" aria-checked="${v === chip}" title="Chip ${xu(v)} xu">
          ${chipImg(v, 52)}<span class="chip-val">${v >= 1000 ? v / 1000 + 'K' : v}</span></button>`)
      .join(''));
  }

  function renderHistory() {
    const list = st.history || [];
    setHTML('history', list.length
      ? list.map((r, i) => `<span class="h-round${i === 0 ? ' last' : ''}">${r.map((f) => faceImg(f, 22)).join('')}</span>`).join('')
      : '<span class="muted">Chưa có vòng nào</span>');
  }

  // Người chơi trong sòng: chia hai bên bàn (máy tính) hoặc một hàng cuộn ngang (điện thoại).
  function renderCrowd() {
    const players = (st.players || []).slice(0, 10);
    const tag = (p) => `<span class="p-tag"><span class="lv">Lv ${p.level || 1}</span>${esc(p.name)}</span>
      <span class="p-bet">${p.total ? `${I.svg('coins')}${xu(p.total)}` : 'Đang xem'}</span>`;
    const person = (p, dir) => `<div class="person${user && p.name === user.name ? ' me' : ''}">
        ${C.sprite(p, { size: 110, dir })}<span class="shadow"></span>${tag(p)}</div>`;
    const left = players.filter((_, i) => i % 2 === 0);
    const right = players.filter((_, i) => i % 2 === 1);
    setHTML('crowd-left', left.map((p) => person(p, 'right')).join(''));
    setHTML('crowd-right', right.map((p) => person(p, 'left')).join(''));
    setHTML('crowd-mobile', players.length
      ? players.map((p) => person(p, 'down')).join('')
      : '<p class="muted">Chưa ai đặt cược vòng này — bạn mở hàng nhé!</p>');
  }

  function renderDice() {
    const show = st.phase === 'result' && st.dice;
    setHTML('dice', show
      ? st.dice.map((f, i) => `<img class="die" style="--i:${i}" src="${IMG}dice_${f}.png" width="62" height="62" alt="${f}" draggable="false">`).join('')
      : '');
    const dealer = $('dealer');
    dealer.classList.toggle('rolling', st.phase === 'rolling');
    dealer.classList.toggle('opened', !!show);
  }

  function renderDock() {
    const mine = (st.mine && st.mine.total) || 0;
    $('my-total').textContent = xu(mine);
    $('max-total').textContent = st.maxTotal ? ` / tối đa ${xu(st.maxTotal)}` : '';
    $('btn-clear').disabled = st.phase !== 'betting' || !mine;
    document.body.classList.toggle('betting', st.phase === 'betting');
  }

  function renderClock() {
    if (!st) return;
    const left = Math.max(0, Math.ceil((st.endsAt - serverNow()) / 1000));
    $('clock').textContent = `${left}s`;
    $('phase').className = `phase ${st.phase}${st.phase === 'betting' && left <= 5 ? ' hurry' : ''}`;
  }

  function render() {
    $('round').textContent = `Vòng #${st.round % 10000}`;
    $('phase-text').textContent = PHASES[st.phase] || '';
    renderClock();
    renderBoard();
    renderChips();
    renderHistory();
    renderCrowd();
    renderDice();
    renderDock();
  }

  // ---------- Hiệu ứng ----------

  // Chữ bay lên (vd "+120 xu") tại một phần tử.
  function float(el, text, cls) {
    if (!el) return;
    const r = el.getBoundingClientRect();
    const f = document.createElement('div');
    f.className = `float-text ${cls}`;
    f.textContent = text;
    f.style.left = `${r.left + r.width / 2}px`;
    // Phần tử trên topbar thì cho chữ hiện bên dưới rồi bay lên, không bị khuất mép màn hình.
    f.style.top = `${r.top < 80 ? r.bottom + 40 : r.top + r.height / 3}px`;
    document.body.appendChild(f);
    setTimeout(() => f.remove(), 1600);
  }

  // Chip bay từ phần tử này sang phần tử kia.
  function flyChips(from, to, n, delay = 0, v = 100) {
    if (!from || !to) return;
    const a = from.getBoundingClientRect();
    const b = to.getBoundingClientRect();
    for (let i = 0; i < n; i++) {
      const c = document.createElement('div');
      c.className = 'fly-chip';
      c.innerHTML = chipImg(v, 30);
      c.style.left = `${a.left + a.width / 2 - 15 + (i - n / 2) * 6}px`;
      c.style.top = `${a.top + a.height / 2 - 15}px`;
      document.body.appendChild(c);
      const dx = b.left + b.width / 2 - (a.left + a.width / 2);
      const dy = b.top + b.height / 2 - (a.top + a.height / 2);
      c.animate(
        [{ transform: 'translate(0,0) scale(1)', opacity: 1 }, { transform: `translate(${dx}px, ${dy}px) scale(.6)`, opacity: 0.2 }],
        { duration: 750, delay: delay + i * 70, easing: 'cubic-bezier(.4,.1,.3,1)', fill: 'forwards' },
      ).onfinish = () => c.remove();
    }
  }

  function banner(text, cls) {
    const el = $('banner');
    el.className = `banner show ${cls || ''}`;
    el.textContent = text;
    clearTimeout(banner.t);
    banner.t = setTimeout(() => (el.className = 'banner'), 2600);
  }

  // Chạy khi sang pha mới: lắc bát, mở bát, trả thưởng.
  function onPhase(prev) {
    if (st.phase === 'betting' && prev) banner('Mời đặt cược!', 'go');
    if (st.phase === 'rolling') banner('Nhà cái lắc bát...', 'roll');
    if (st.phase !== 'result' || !st.dice) return;
    const mine = st.mine || {};
    const bets = mine.bets || {};
    const counts = {};
    for (const d of st.dice) counts[d] = (counts[d] || 0) + 1;
    const names = st.dice.map((d) => (FACES.find((f) => f.id === d) || { name: d }).name).join(' · ');
    if (!mine.total) {
      banner(names);
      return;
    }
    const net = mine.net !== undefined ? mine.net : (mine.won || 0) - mine.total;
    setTimeout(() => {
      for (const [face, amount] of Object.entries(bets)) {
        if (!amount) continue;
        const cell = document.querySelector(`.cell[data-face="${face}"]`);
        if (counts[face]) flyChips(cell, $('wallet'), 3, 200, chipStack(amount).pop() || 100);
        else flyChips(cell, $('dealer'), 2, 0, chipStack(amount).pop() || 100);
      }
      banner(net > 0 ? `${names} — Bạn thắng ${xu(net)} xu!` : net < 0 ? `${names} — Thua ${xu(-net)} xu` : `${names} — Hoà vốn`, net > 0 ? 'win' : net < 0 ? 'lose' : '');
      float($('wallet'), `${net >= 0 ? '+' : '−'}${xu(Math.abs(net))} xu`, net >= 0 ? 'plus' : 'minus');
      // Mỗi vòng có cược được XP_PLAY, có lời thêm XP_WIN (server cộng; ở đây chỉ hiện chữ bay).
      setTimeout(() => float($('level-badge'), `+${XP_PLAY + (net > 0 ? XP_WIN : 0)} XP`, 'xp'), 350);
    }, 500);
    // Cập nhật số dư / XP sau khi server chốt vòng.
    setTimeout(refreshUser, 900);
  }

  async function refreshUser() {
    try {
      const u = await A.me();
      if (u) {
        user = u;
        renderHUD();
      }
    } catch (e) { /* bỏ qua */ }
  }

  // ---------- Đồng bộ với server ----------

  function apply(s) {
    if (!s || s.round === undefined) return;
    if (typeof s.now === 'number') offset = s.now - Date.now();
    const prev = st;
    st = s;
    if (typeof s.balance === 'number' && user) {
      user.balance = s.balance;
      renderHUD();
    }
    render();
    if (seen.round !== s.round || seen.phase !== s.phase) {
      const had = seen.phase !== null;
      seen = { round: s.round, phase: s.phase };
      if (had) onPhase(prev);
    }
    schedule();
  }

  // Hỏi lại server mỗi giây, và ngay lúc pha hiện tại kết thúc.
  function schedule() {
    clearTimeout(pollTimer);
    const toEnd = st ? st.endsAt - serverNow() + 80 : 1000;
    pollTimer = setTimeout(poll, Math.max(150, Math.min(1000, toEnd)));
  }

  async function poll() {
    try {
      apply(await A.api('/api/baucua/state'));
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      schedule();
    }
  }

  async function send(path, body) {
    if (busy) return;
    busy = true;
    try {
      apply(await A.api(path, { method: 'POST', body: body || {} }));
      return true;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      const hp = e.status === 423 || (e.status === 403 && /HP/.test(e.message));
      A.toast(hp ? `${e.message} — ghé Bệnh viện trên bản đồ để hồi máu` : e.message);
      return false;
    } finally {
      busy = false;
    }
  }

  // ---------- Thao tác ----------

  $('board').addEventListener('click', async (e) => {
    const cell = e.target.closest('.cell');
    if (!cell || !st) return;
    if (st.phase !== 'betting') return A.toast('Đợi vòng sau để đặt cược');
    const face = cell.dataset.face;
    // Chip bay từ thanh chip vào ô ngay khi bấm cho cảm giác nhanh.
    flyChips(document.querySelector(`.chip-btn[data-chip="${chip}"]`), cell, 1, 0, chip);
    if (await send('/api/baucua/bet', { face, amount: chip })) {
      const el = document.querySelector(`.cell[data-face="${face}"]`);
      el && el.classList.add('bump');
      setTimeout(() => el && el.classList.remove('bump'), 300);
    }
  });

  $('chips').addEventListener('click', (e) => {
    const b = e.target.closest('.chip-btn');
    if (!b) return;
    chip = Number(b.dataset.chip);
    renderChips();
  });

  $('btn-clear').addEventListener('click', () => send('/api/baucua/clear'));

  setInterval(renderClock, 250);
  document.addEventListener('visibilitychange', () => document.visibilityState === 'visible' && poll());

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(28);
  A.me().then(
    (u) => {
      if (!u) return A.requireLogin();
      user = u;
      renderHUD();
      $('room').classList.remove('hidden');
      I.render();
      poll();
    },
    (e) => A.toast(e.message),
  );
})();
