// Bầu Cua Tôm Cá của DCity: sảnh gồm sòng chung (máy làm cái) và các bàn riêng mời bạn bè, trong bàn riêng ai muốn thì
// làm cái bằng quỹ của mình. Mỗi vòng 30 giây (đặt cược 20 s → lắc 4 s → mở bát 6 s).
// Server quyết định mọi thứ (/api/baucua/...): trang này chỉ hiển thị, đếm ngược theo giờ server và gửi lệnh.
// /baucua/ là sảnh, /baucua/?room=MÃ vào thẳng một bàn (link mời).
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

  const BANK_MIN = 3000; // quỹ cái tối thiểu (khớp games/baucua MinBankroll)
  const LOBBY = 'chung'; // mã sòng chung (máy làm cái)
  const BANK_PRESETS = [3000, 10000, 50000];

  // Bàn đang ngồi: null = đang ở sảnh; '' = sòng chung khi server không đặt mã cho nó.
  const roomCode = new URLSearchParams(location.search).get('room');
  const roomQuery = () => (roomCode ? `?room=${encodeURIComponent(roomCode)}` : '');
  const goRoom = (code) => (location.href = `/baucua/?room=${encodeURIComponent(code || '')}`);

  let user = null;
  let st = null; // trạng thái sòng mới nhất
  let offset = 0; // giờ server − giờ máy (ms)
  let chip = 10; // mức chip đang chọn
  let seen = { round: null, phase: null }; // để chỉ chạy hiệu ứng một lần mỗi pha
  let busy = false;
  let pollTimer = null;
  let fundAtRoll = null; // quỹ cái lúc bắt đầu lắc, để tính lời / lỗ của người làm cái

  const serverNow = () => Date.now() + offset;
  const isMe = (p) => !!(p && user && p.name === user.name);
  const room = () => (st && st.room) || null;
  const banker = () => (room() && room().banker) || null;
  // Server báo mine.banker; máy chủ cũ không có thì so tên.
  const amBanker = () => (st && st.mine && typeof st.mine.banker === 'boolean' ? st.mine.banker : isMe(banker()));
  // Bàn riêng có chủ bàn; sòng chung thì không (máy luôn làm cái).
  const isPrivate = () => !!(room() && room().host);

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
    const all = (room() && room().members) || st.players || [];
    const b = banker();
    const players = all.filter((p) => !(b && p.name === b.name)).slice(0, 10);
    const tag = (p) => `<span class="p-tag"><span class="lv">Lv ${p.level || 1}</span>${esc(p.name)}</span>
      <span class="p-bet">${p.total ? `${I.svg('coins')}${xu(p.total)}` : 'Đang xem'}</span>`;
    const person = (p, dir) => `<div class="person${isMe(p) ? ' me' : ''}">
        ${C.sprite(p, { size: 110, dir })}<span class="shadow"></span>${tag(p)}</div>`;
    const left = players.filter((_, i) => i % 2 === 0);
    const right = players.filter((_, i) => i % 2 === 1);
    setHTML('crowd-left', left.map((p) => person(p, 'right')).join(''));
    setHTML('crowd-right', right.map((p) => person(p, 'left')).join(''));
    setHTML('crowd-mobile', players.length
      ? players.map((p) => person(p, 'down')).join('')
      : `<p class="muted">${isPrivate() ? 'Bàn còn trống — bấm "Mời bạn" để gửi link cho bạn bè.' : 'Chưa ai đặt cược vòng này — bạn mở hàng nhé!'}</p>`);
  }

  // Lãi / lỗ của quỹ cái trong vòng vừa mở bát (server chỉ gửi ở pha mở bát).
  const netTag = (b) => (st.phase === 'result' && typeof b.net === 'number' && b.net
    ? `<b class="net ${b.net > 0 ? 'plus' : 'minus'}">${b.net > 0 ? '+' : '−'}${xu(Math.abs(b.net))}</b>` : '');

  // Nhà cái: máy, hoặc nhân vật người làm cái đứng sau bàn kèm quỹ cái.
  function renderBanker() {
    const b = banker();
    setHTML('banker', b ? C.sprite(b, { size: 104, dir: 'down' }) : '');
    setHTML('banker-tag', b
      ? `<span class="who">${I.svg('crown')}Cái: ${esc(b.name)}${amBanker() ? ' (bạn)' : ''}</span><span class="fund">${I.svg('coins')}Quỹ ${xu(b.bankroll || 0)}${netTag(b)}</span>${b.leaving ? '<span class="leaving">Sắp thôi làm cái</span>' : ''}`
      : `<span class="who">${I.svg('bot')}Máy làm cái</span>`);
    $('dealer').classList.toggle('has-banker', !!b);
  }

  // Thông tin bàn: tên, mã, mời bạn; nút làm cái / thôi làm cái; giới hạn cược theo quỹ cái.
  function renderTable() {
    const r = room();
    const priv = isPrivate();
    $('room-title').textContent = priv ? `Bàn của ${r.host.name}` : 'Sòng chung';
    $('room-code').classList.toggle('hidden', !priv);
    $('btn-invite').classList.toggle('hidden', !priv);
    if (priv) $('room-code').textContent = `Mã ${r.code}`;
    const b = banker();
    const me = amBanker();
    $('dock').classList.toggle('hidden', me);
    $('bank-dock').classList.toggle('hidden', !me);
    $('btn-bank').classList.toggle('hidden', !priv || !!b);
    const max = (r && r.maxRoundTotal) || 0;
    const total = (r && r.roundTotal) || 0;
    if (me) {
      $('bank-fund').textContent = xu(b.bankroll || 0);
      $('bank-total').textContent = xu(total);
      $('bank-max').textContent = max ? ` / tối đa ${xu(max)}` : '';
      $('bank-meter').style.width = `${max ? Math.min(100, Math.round((total / max) * 100)) : 0}%`;
      $('btn-unbank').disabled = !!b.leaving;
      $('btn-unbank').lastChild.textContent = b.leaving ? 'Thôi sau vòng này' : 'Thôi làm cái';
    }
    const left = $('table-left');
    left.classList.toggle('hidden', !b || me);
    if (b && !me) {
      const remain = Math.max(0, max - total);
      left.innerHTML = `${I.svg('crown')}<span>Cái <b>${esc(b.name)}</b> nhận tối đa <b>${xu(max)}</b> xu mỗi vòng — còn nhận được <b class="${remain ? '' : 'full'}">${xu(remain)}</b> xu</span>`;
    }
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
    renderBanker();
    renderTable();
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
    if (st.phase === 'rolling') {
      banner(banker() ? `Cái ${banker().name} lắc bát...` : 'Nhà cái lắc bát...', 'roll');
      fundAtRoll = amBanker() ? banker().bankroll || 0 : null;
    }
    if (st.phase !== 'result' || !st.dice) return;
    if (amBanker() && fundAtRoll !== null) {
      const names = st.dice.map((d) => (FACES.find((f) => f.id === d) || { name: d }).name).join(' · ');
      const diff = typeof banker().net === 'number' ? banker().net : (banker().bankroll || 0) - fundAtRoll;
      setTimeout(() => {
        banner(diff > 0 ? `${names} — Cái ăn ${xu(diff)} xu!` : diff < 0 ? `${names} — Cái chung ${xu(-diff)} xu` : names, diff > 0 ? 'win' : diff < 0 ? 'lose' : '');
        if (diff) float($('bank-fund'), `${diff > 0 ? '+' : '−'}${xu(Math.abs(diff))} xu`, diff > 0 ? 'plus' : 'minus');
      }, 500);
      setTimeout(refreshUser, 900);
      return;
    }
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
      apply(await A.api('/api/baucua/state' + roomQuery()));
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      if (e.status === 404) return gone(e.message);
      schedule();
    }
  }

  async function send(path, body) {
    if (busy) return;
    busy = true;
    try {
      const data = await A.api(path, { method: 'POST', body: { ...(body || {}), ...(roomCode ? { room: roomCode } : {}) } });
      if (data && data.round !== undefined) apply(data);
      else poll();
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

  // ---------- Bàn riêng: mời bạn, làm cái ----------

  // Bàn không còn (chủ bàn đóng / mã sai): báo rồi về sảnh.
  function gone(msg) {
    clearTimeout(pollTimer);
    A.toast(msg || 'Bàn này không còn nữa');
    setTimeout(() => (location.href = '/baucua/'), 1500);
  }

  $('btn-lobby').addEventListener('click', () => (location.href = '/baucua/'));

  $('btn-invite').addEventListener('click', async () => {
    const link = location.origin + ((st && st.invite) || `/baucua/?room=${encodeURIComponent(roomCode || '')}`);
    try {
      await navigator.clipboard.writeText(link);
      A.toast('Đã sao chép link mời — gửi cho bạn bè để vào bàn');
    } catch (e) {
      window.prompt('Sao chép link mời:', link);
    }
  });

  $('bank-presets').innerHTML = BANK_PRESETS.map((v) => `<button type="button" data-fund="${v}">${I.svg('coins')}${xu(v)}</button>`).join('');
  $('bank-presets').addEventListener('click', (e) => {
    const b = e.target.closest('[data-fund]');
    if (b) $('bank-input').value = b.dataset.fund;
  });
  $('btn-bank').addEventListener('click', () => {
    $('bank-input').value = Math.min(Math.max(BANK_MIN, 10000), Math.max(BANK_MIN, user.balance));
    $('bank-hint').textContent = `Tối thiểu ${xu(BANK_MIN)} xu · ví bạn đang có ${xu(user.balance)} xu. Làm cái thì không đặt cược.`;
    $('dlg-bank').showModal();
  });
  $('bank-form').addEventListener('submit', async (e) => {
    if (e.submitter && e.submitter.value === 'cancel') return;
    const bankroll = Number($('bank-input').value);
    if (!(bankroll >= BANK_MIN)) {
      e.preventDefault();
      return A.toast(`Quỹ cái tối thiểu ${xu(BANK_MIN)} xu`);
    }
    if (await send('/api/baucua/bank', { bankroll })) {
      A.toast(`Bạn đang làm cái với quỹ ${xu(bankroll)} xu`);
      refreshUser();
    }
  });
  $('btn-unbank').addEventListener('click', async () => {
    if (await send('/api/baucua/unbank')) {
      A.toast(banker() && banker().leaving ? 'Bạn sẽ thôi làm cái sau khi chốt vòng này — quỹ còn lại về ví' : 'Đã thôi làm cái — quỹ còn lại về ví');
      refreshUser();
    }
  });

  // Vào bàn thì báo server (để hiện trong danh sách thành viên); rời trang thì báo rời, kể cả khi đóng tab.
  async function join() {
    try {
      await A.api('/api/baucua/join', { method: 'POST', body: roomCode ? { room: roomCode } : {} });
      return true;
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      if (e.status === 404 || e.status === 400) {
        gone(e.message);
        return false;
      }
      return true; // lỗi khác (vd máy chủ cũ chưa có bàn riêng): vẫn xem sòng được
    }
  }
  function leave() {
    const token = A.token();
    if (!token) return;
    fetch('/api/baucua/leave', {
      method: 'POST', keepalive: true,
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token },
      body: JSON.stringify(roomCode ? { room: roomCode } : {}),
    }).catch(() => {});
  }

  // ---------- Sảnh ----------

  let rooms = null;
  let lobbyTimer = null;

  function tableCard(r) {
    const common = !r.host;
    const b = r.banker;
    const phase = PHASES[r.phase] || '';
    const who = common
      ? `<div class="t-art">${['cua', 'ga', 'nai'].map((f) => faceImg(f, 54)).join('')}</div>`
      : `<div class="t-art">${C.sprite(r.host, { size: 88, dir: 'down' })}</div>`;
    const bankLine = b
      ? `<span class="t-bank">${C.head(b, { size: 26 })}Cái: <b>${esc(b.name)}</b> · quỹ ${xu(b.bankroll || 0)}</span>`
      : `<span class="t-bank">${I.svg('bot')}Máy làm cái</span>`;
    return `<div class="t-card${common ? ' common' : ''}">
        ${who}
        <div class="t-body">
          <h3>${common ? 'Sòng chung' : `Bàn của ${esc(r.host.name)}`}</h3>
          ${common ? '<span class="muted">Cả thành phố chơi cùng, luôn mở</span>' : `<span class="t-code">Mã ${esc(r.code)}</span>`}
          ${bankLine}
          <span class="t-meta">${I.svg('users')}${r.members || 0} người${phase ? ` · ${phase}` : ''}</span>
        </div>
        <button class="primary" data-room="${esc(r.code || '')}">${I.svg('play')}Vào</button>
      </div>`;
  }

  function renderLobby() {
    if (rooms === null) return;
    const list = rooms.slice().sort((a, b) => (!a.host ? -1 : !b.host ? 1 : (b.members || 0) - (a.members || 0)));
    if (!list.some((r) => !r.host)) list.unshift({ code: LOBBY, host: null, banker: null, members: 0, phase: '' });
    const empty = list.length === 1 ? '<p class="muted t-empty">Chưa có bàn riêng nào — bấm "Tạo bàn" để mở bàn và mời bạn bè.</p>' : '';
    setHTML('tables', list.map(tableCard).join('') + empty);
  }

  async function loadRooms() {
    try {
      const data = await A.api('/api/baucua/rooms');
      rooms = Array.isArray(data) ? data : data.rooms || [];
    } catch (e) {
      if (e.status === 401) return A.requireLogin();
      if (rooms === null) rooms = [];
    }
    renderLobby();
    clearTimeout(lobbyTimer);
    lobbyTimer = setTimeout(loadRooms, 3000);
  }

  $('tables').addEventListener('click', (e) => {
    const b = e.target.closest('[data-room]');
    if (b) goRoom(b.dataset.room);
  });
  $('code-form').addEventListener('submit', (e) => {
    e.preventDefault();
    const code = $('code-input').value.trim().toUpperCase();
    if (!code) return A.toast('Nhập mã bàn bạn bè gửi cho bạn');
    goRoom(code);
  });
  $('btn-create').addEventListener('click', async (e) => {
    e.currentTarget.disabled = true;
    try {
      const data = await A.api('/api/baucua/rooms', { method: 'POST', body: {} });
      goRoom(data.code);
    } catch (err) {
      if (err.status === 401) return A.requireLogin();
      const hp = err.status === 423 || (err.status === 403 && /HP/.test(err.message));
      A.toast(hp ? `${err.message} — ghé Bệnh viện trên bản đồ để hồi máu` : err.message);
      e.currentTarget.disabled = false;
    }
  });

  setInterval(renderClock, 250);
  document.addEventListener('visibilitychange', () => document.visibilityState === 'visible' && poll());

  // ---------- Khởi động ----------

  $('logo').innerHTML = I.logo(28);
  A.me().then(
    async (u) => {
      if (!u) return A.requireLogin();
      user = u;
      renderHUD();
      $('lobby-faces').innerHTML = FACES.map((f) => faceImg(f.id, 40)).join('');
      I.render();
      if (roomCode === null) {
        $('lobby').classList.remove('hidden');
        loadRooms();
        return;
      }
      $('room').classList.remove('hidden');
      if (!(await join())) return;
      window.addEventListener('pagehide', leave);
      poll();
    },
    (e) => A.toast(e.message),
  );
})();
