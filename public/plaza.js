// Quảng trường / gặp gỡ: thấy người chơi khác đi lại trên bản đồ, trò chuyện, gửi biểu cảm, tặng xu, mời chơi.
// Kết nối WebSocket /ws/plaza khi đã vào thành phố; tự nối lại khi rớt mạng.
// Vẽ người khác qua City.actors (nếu bản đồ có), không thì tự vẽ vào lớp thế giới của bản đồ (cạnh #me).
(function () {
  const A = window.Account;
  const C = window.Character;
  const I = window.Icons;
  const $ = (id) => document.getElementById(id);
  const esc = (s) => A.escapeHtml(s);
  const ic = (name) => {
    try { return (I && I.svg(name)) || ''; } catch (e) { return ''; }
  };

  const EMOTES = [
    { kind: 'wave', name: 'Vẫy tay' },
    { kind: 'heart', name: 'Thả tim' },
    { kind: 'laugh', name: 'Cười' },
    { kind: 'clap', name: 'Vỗ tay' },
    { kind: 'dance', name: 'Nhảy' },
  ];
  const emoteImg = (kind) => `/img/toon/city/emote_${kind}.png`;
  const GIFT_STEPS = [10, 50, 100, 500];
  const SEND_EVERY = 150; // ms giữa hai lần gửi vị trí
  const SPRITE_SIZE = 96;

  let ws = null;
  let you = null;
  let retry = 0;
  let retryTimer = null;
  const players = new Map(); // id → { ...player, tx, ty } (vị trí đích để đi mượt)
  let lastSent = '';
  let lastPos = null;
  let lastMoveAt = 0;
  let myDir = 'down';
  let unread = 0;
  let connected = false; // đã kết nối và xác thực
  let inside = false; // server xác nhận đang đứng trong Quảng trường
  const BUBBLE_MS = 4000; // bong bóng chat trên đầu hiện 4 giây

  // ---------- Vẽ người khác ----------

  // Bộ vẽ dự phòng khi bản đồ chưa có City.actors: thêm phần tử cùng lớp với #me (toạ độ bản đồ).
  const fallback = {
    els: new Map(),
    layer() {
      const me = document.querySelector('#map #me');
      return me && me.parentElement;
    },
    upsert(id, a) {
      const layer = this.layer();
      if (!layer) return;
      let el = this.els.get(id);
      if (!el || !el.isConnected) {
        el = document.createElement('div');
        el.className = 'me plaza-actor';
        el.dataset.actor = id;
        el.innerHTML = `<div class="me-tag"><span class="tag-name"></span><span class="tag-lv"></span></div>
          <div class="me-body"></div><div class="plaza-bubble hidden"></div><div class="plaza-fx"></div>`;
        // Bấm vào người khác: mở thẻ hồ sơ, không để bản đồ hiểu là bấm để đi.
        const stop = (e) => e.stopPropagation();
        el.addEventListener('pointerdown', stop);
        el.addEventListener('mousedown', stop);
        el.addEventListener('click', (e) => {
          e.stopPropagation();
          openProfile(id);
        });
        layer.appendChild(el);
        this.els.set(id, el);
        el._key = '';
      }
      el.querySelector('.tag-name').textContent = a.name || '';
      el.querySelector('.tag-lv').textContent = 'Lv ' + (a.level || 1);
      el.style.transform = `translate(${a.x}px, ${a.y}px)`;
      el.style.zIndex = String(100 + Math.round(a.y));
      el.classList.toggle('sitting', !!a.sitting);
      const key = JSON.stringify([a.look, a.dir, a.walking, a.hospital]);
      if (el._key !== key) {
        el._key = key;
        const body = el.querySelector('.me-body');
        if (C && typeof C.sprite === 'function') {
          body.innerHTML = C.sprite(a.look || {}, { dir: a.dir || 'down', walking: !!a.walking && !a.hospital, size: SPRITE_SIZE, pose: a.hospital ? 'lie' : undefined });
        }
        body.className = 'me-body dir-' + (a.dir || 'down') + (a.walking ? ' walking' : '');
      }
    },
    remove(id) {
      const el = this.els.get(id);
      if (el) el.remove();
      this.els.delete(id);
    },
    bubble(id, text, ms) {
      showBubble(this.els.get(id), text, ms);
    },
    emote(id, kind) {
      showEmote(this.els.get(id), kind);
    },
  };

  const actors = () => (window.City && window.City.actors) || fallback;

  function showBubble(el, text, ms = BUBBLE_MS) {
    if (!el) return;
    const b = el.querySelector('.plaza-bubble');
    if (!b) return;
    b.textContent = text;
    b.classList.remove('hidden');
    clearTimeout(b._t);
    b._t = setTimeout(() => b.classList.add('hidden'), ms);
  }

  function showEmote(el, kind) {
    if (!el) return;
    let fx = el.querySelector('.plaza-fx');
    if (!fx) {
      fx = document.createElement('div');
      fx.className = 'plaza-fx';
      el.appendChild(fx);
    }
    const img = document.createElement('img');
    img.className = 'plaza-emote emote-' + kind;
    img.src = emoteImg(kind);
    img.alt = '';
    img.draggable = false;
    fx.appendChild(img);
    setTimeout(() => img.remove(), 2400);
    if (kind === 'dance') {
      el.classList.add('dancing');
      clearTimeout(el._dance);
      el._dance = setTimeout(() => el.classList.remove('dancing'), 2400);
    }
  }

  function floatText(el, text, cls) {
    if (!el) return;
    const fx = el.querySelector('.plaza-fx') || el;
    const s = document.createElement('span');
    s.className = 'plaza-float ' + (cls || '');
    s.textContent = text;
    fx.appendChild(s);
    setTimeout(() => s.remove(), 2200);
  }

  // Phần tử của mình (để hiện bong bóng / biểu cảm trên đầu mình).
  const meEl = () => document.querySelector('#map #me');

  function bubbleMe(text) {
    if (window.City && typeof window.City.bubbleMe === 'function') return window.City.bubbleMe(text, BUBBLE_MS);
    const el = meEl();
    if (el && !el.querySelector('.plaza-bubble')) el.insertAdjacentHTML('beforeend', '<div class="plaza-bubble hidden"></div><div class="plaza-fx"></div>');
    showBubble(el, text);
  }

  function emoteMe(kind) {
    if (window.City && typeof window.City.emoteMe === 'function') return window.City.emoteMe(kind);
    const el = meEl();
    if (el && !el.querySelector('.plaza-fx')) el.insertAdjacentHTML('beforeend', '<div class="plaza-bubble hidden"></div><div class="plaza-fx"></div>');
    showEmote(el, kind);
  }

  const actorEl = (id) => (id === you ? meEl() : fallback.els.get(id) || document.querySelector(`[data-actor="${CSS.escape(id)}"], [data-actor-id="${CSS.escape(id)}"]`));

  function draw(p) {
    actors().upsert(p.id, {
      look: p.look, name: p.name, level: p.level, x: p.x, y: p.y, dir: p.dir, walking: p.walking, sitting: p.sitting, hospital: p.hospital,
    });
  }

  // Đi mượt giữa các gói vị trí (gói tới mỗi ~100 ms).
  function animate() {
    for (const p of players.values()) {
      if (p.tx === undefined) continue;
      const dx = p.tx - p.x;
      const dy = p.ty - p.y;
      if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5) {
        if (p.x !== p.tx || p.y !== p.ty) {
          p.x = p.tx;
          p.y = p.ty;
          draw(p);
        }
        continue;
      }
      const far = Math.hypot(dx, dy) > 400; // dịch chuyển xa (đi nhanh / vào khu): nhảy luôn
      p.x = far ? p.tx : p.x + dx * 0.35;
      p.y = far ? p.ty : p.y + dy * 0.35;
      draw(p);
    }
    requestAnimationFrame(animate);
  }
  requestAnimationFrame(animate);

  function addPlayer(p) {
    players.set(p.id, { ...p, tx: p.x, ty: p.y });
    draw(players.get(p.id));
    updateOnline();
  }

  function removePlayer(id) {
    players.delete(id);
    actors().remove(id);
    updateOnline();
  }

  function clearPlayers() {
    for (const id of players.keys()) actors().remove(id);
    players.clear();
    updateOnline();
  }

  // ---------- Kết nối ----------

  function cityVisible() {
    const c = $('city');
    return !!(c && !c.classList.contains('hidden') && window.City && window.City.position && window.City.position());
  }

  function send(msg) {
    if (ws && ws.readyState === 1) ws.send(JSON.stringify(msg));
  }

  function connect() {
    if (ws || !A.token()) return;
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    const sock = new WebSocket(`${proto}://${location.host}/ws/plaza`);
    ws = sock;
    sock.onopen = () => sock.send(JSON.stringify({ t: 'hello', token: A.token() }));
    sock.onmessage = (e) => {
      let m;
      try { m = JSON.parse(e.data); } catch (err) { return; }
      onMessage(m);
    };
    sock.onclose = () => {
      if (ws !== sock) return;
      ws = null;
      you = null;
      lastSent = '';
      clearPlayers();
      setStatus(false);
      clearTimeout(retryTimer);
      retry = Math.min(retry + 1, 6);
      retryTimer = setTimeout(tick, 1000 * Math.min(15, 2 ** retry));
    };
  }

  function disconnect() {
    if (!ws) return;
    const sock = ws;
    ws = null;
    sock.close();
    clearPlayers();
    setStatus(false);
  }

  function onMessage(m) {
    switch (m.t) {
      case 'welcome':
        // Chưa thấy ai: chỉ khi bước vào Quảng trường server mới gửi "enter" kèm người đang ở đó.
        you = m.you;
        retry = 0;
        clearPlayers();
        setStatus(true);
        setInside(false);
        lastSent = '';
        break;
      case 'enter':
        clearPlayers();
        (m.players || []).forEach(addPlayer);
        $('plaza-log').innerHTML = '';
        (m.chat || []).forEach((c) => logLine(c.name, c.text, c.id === you));
        setInside(true);
        break;
      case 'exit':
        clearPlayers();
        setInside(false);
        break;
      case 'join':
        addPlayer(m.player);
        logSystem(`${m.player.name} vừa vào Quảng trường`);
        break;
      case 'leave':
        removePlayer(m.id);
        break;
      case 'moves':
        for (const v of m.list || []) {
          const p = players.get(v.id);
          if (!p) continue;
          p.tx = v.x;
          p.ty = v.y;
          if (p.dir !== v.dir || p.walking !== v.walking || p.sitting !== v.sitting) {
            Object.assign(p, { dir: v.dir, walking: v.walking, sitting: v.sitting });
            draw(p);
          }
        }
        break;
      case 'look': {
        const p = players.get(m.id);
        if (p) {
          Object.assign(p, { name: m.name, level: m.level, look: m.look, hospital: m.hospital });
          draw(p);
        }
        break;
      }
      case 'chat': {
        const mine = m.id === you;
        logLine(m.name, m.text, mine);
        if (mine) bubbleMe(m.text);
        else {
          const a = actors();
          if (typeof a.bubble === 'function') a.bubble(m.id, m.text, BUBBLE_MS);
          if (!chatOpen()) setUnread(unread + 1);
        }
        break;
      }
      case 'emote':
        if (m.id === you) emoteMe(m.kind);
        else if (typeof actors().emote === 'function') actors().emote(m.id, m.kind);
        break;
      case 'gift': {
        floatText(actorEl(m.to), `+${A.xu(m.amount)} xu`, 'gift');
        if (m.to === you) A.toast(`${m.fromName} tặng bạn ${A.xu(m.amount)} xu`);
        else if (m.from === you) A.toast(`Đã tặng ${m.toName} ${A.xu(m.amount)} xu`);
        logSystem(`${m.fromName} tặng ${m.toName} ${A.xu(m.amount)} xu`);
        break;
      }
      case 'balance':
        if ($('balance')) $('balance').textContent = A.xu(m.balance);
        window.dispatchEvent(new CustomEvent('dcity:balance', { detail: m.balance }));
        break;
      case 'invite':
        showInvite(m);
        break;
      case 'error':
        A.toast(m.msg);
        break;
      case 'auth':
        disconnect();
        break;
    }
  }

  // Gửi vị trí của mình khi đổi (tối đa ~7 lần/giây).
  function sendPosition() {
    if (!ws || !you || !window.City) return;
    let s = typeof window.City.state === 'function' ? window.City.state() : null;
    if (!s) {
      const pos = window.City.position && window.City.position();
      if (!pos) return;
      const now = performance.now();
      if (lastPos && (Math.abs(pos[0] - lastPos[0]) > 0.5 || Math.abs(pos[1] - lastPos[1]) > 0.5)) {
        const dx = pos[0] - lastPos[0];
        const dy = pos[1] - lastPos[1];
        myDir = Math.abs(dx) > Math.abs(dy) ? (dx < 0 ? 'left' : 'right') : dy < 0 ? 'up' : 'down';
        lastMoveAt = now;
      }
      lastPos = pos;
      s = { x: pos[0], y: pos[1], dir: myDir, walking: now - lastMoveAt < 250, sitting: false };
    }
    const msg = { t: 'move', x: Math.round(s.x * 10) / 10, y: Math.round(s.y * 10) / 10, dir: s.dir || 'down', walking: !!s.walking, sitting: !!s.sitting };
    const key = JSON.stringify(msg);
    if (key === lastSent) return;
    lastSent = key;
    send(msg);
    updateTitle(s);
  }

  // Vòng kiểm tra: vào thành phố thì nối, rời (đăng xuất) thì ngắt.
  function tick() {
    if (cityVisible() && A.token()) connect();
    else if (!A.token()) disconnect();
  }
  setInterval(tick, 1000);
  setInterval(sendPosition, SEND_EVERY);

  // Đổi đồ / lên level / nằm viện: báo server đọc lại hồ sơ (ảnh avatar trên thanh trên cùng đổi là có thay đổi).
  let lookTimer = null;
  function lookChanged() {
    clearTimeout(lookTimer);
    lookTimer = setTimeout(() => send({ t: 'look' }), 400);
  }
  function watchLook() {
    const targets = ['menu-avatar', 'level-badge', 'hp-badge'].map($).filter(Boolean);
    if (!targets.length) return setTimeout(watchLook, 1000);
    const mo = new MutationObserver(lookChanged);
    targets.forEach((t) => mo.observe(t, { childList: true, subtree: true, characterData: true }));
  }
  watchLook();
  setInterval(() => send({ t: 'look' }), 30000);

  // ---------- Khung chat, số người online, tiêu đề quảng trường ----------

  function buildUI() {
    const city = $('city');
    if (!city || $('plaza-chat')) return;
    city.insertAdjacentHTML('beforeend', `
      <div class="plaza-online hidden" id="plaza-online"><span class="dot"></span><span id="plaza-count">0</span></div>
      <div class="plaza-title hidden" id="plaza-title"><b>Quảng trường</b><span>gặp gỡ · trò chuyện</span></div>
      <div class="plaza-hint hidden" id="plaza-hint"><span>Vào <b>Quảng trường</b> để gặp mọi người</span>
        <button type="button" class="primary" id="plaza-go">Đi tới</button></div>
      <section class="plaza-chat collapsed" id="plaza-chat" aria-label="Trò chuyện">
        <button class="plaza-chat-head" id="plaza-chat-toggle" type="button">
          ${ic('users')}<span>Trò chuyện</span><kbd>T</kbd><span class="plaza-unread hidden" id="plaza-unread"></span>
        </button>
        <div class="plaza-chat-body">
          <div class="plaza-log" id="plaza-log" aria-live="polite"></div>
          <div class="plaza-emotes" id="plaza-emotes">
            ${EMOTES.map((e, i) => `<button type="button" data-emote="${e.kind}" title="${e.name} (Shift+${i + 1})"><img src="${emoteImg(e.kind)}" alt="${e.name}" draggable="false"></button>`).join('')}
          </div>
          <form class="plaza-form" id="plaza-form" autocomplete="off">
            <input id="plaza-input" maxlength="120" placeholder="Nói gì đó… (Enter gửi, Esc đóng)">
            <button class="primary" type="submit" title="Gửi">Gửi</button>
          </form>
        </div>
      </section>
      <div class="plaza-invite hidden" id="plaza-invite"></div>`);
    $('plaza-chat-toggle').onclick = () => (chatOpen() ? closeChat() : openChat());
    $('plaza-go').onclick = () => {
      if (window.City && typeof window.City.walkTo === 'function') window.City.walkTo('plaza');
    };
    setInside(inside);
    $('plaza-form').onsubmit = (e) => {
      e.preventDefault();
      const input = $('plaza-input');
      const text = input.value.trim();
      if (!text) return closeChat();
      if (!ws) return A.toast('Chưa kết nối được, thử lại sau');
      send({ t: 'chat', text });
      input.value = '';
    };
    $('plaza-input').addEventListener('keydown', (e) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeChat();
      }
    });
    $('plaza-emotes').onclick = (e) => {
      const b = e.target.closest('[data-emote]');
      if (b) send({ t: 'emote', kind: b.dataset.emote });
    };
  }

  function setStatus(on) {
    connected = on;
    if (!on) inside = false;
    setInside(inside);
  }

  // Trong Quảng trường: hiện số người, tiêu đề, khung chat. Ngoài: chỉ hiện lời mời "Vào Quảng trường để gặp mọi người".
  function setInside(on) {
    inside = on;
    const show = (id, v) => $(id) && $(id).classList.toggle('hidden', !v);
    show('plaza-online', connected && on);
    show('plaza-title', connected && on);
    show('plaza-chat', connected && on);
    show('plaza-hint', connected && !on);
    if (!on && $('plaza-chat')) {
      $('plaza-chat').classList.add('collapsed');
      if (document.activeElement === $('plaza-input')) $('plaza-input').blur();
      const inv = $('plaza-invite');
      if (inv) inv.classList.add('hidden');
      const dlg = $('plaza-profile');
      if (dlg && dlg.open) dlg.close();
    }
    updateOnline();
  }

  function updateOnline() {
    const el = $('plaza-count');
    if (el) el.textContent = `${players.size + (you ? 1 : 0)} người ở Quảng trường`;
  }

  function updateTitle() {}

  const chatOpen = () => $('plaza-chat') && !$('plaza-chat').classList.contains('collapsed');

  function openChat() {
    buildUI();
    $('plaza-chat').classList.remove('collapsed');
    setUnread(0);
    $('plaza-input').focus();
    const log = $('plaza-log');
    log.scrollTop = log.scrollHeight;
  }

  function closeChat() {
    $('plaza-chat').classList.add('collapsed');
    $('plaza-input').blur();
  }

  function setUnread(n) {
    unread = n;
    const el = $('plaza-unread');
    if (!el) return;
    el.textContent = n > 9 ? '9+' : String(n);
    el.classList.toggle('hidden', !n);
  }

  function logLine(name, text, mine) {
    const log = $('plaza-log');
    if (!log) return;
    log.insertAdjacentHTML('beforeend', `<p class="${mine ? 'mine' : ''}"><b>${esc(name)}</b> ${esc(text)}</p>`);
    while (log.children.length > 60) log.firstElementChild.remove();
    log.scrollTop = log.scrollHeight;
  }

  function logSystem(text) {
    const log = $('plaza-log');
    if (!log) return;
    log.insertAdjacentHTML('beforeend', `<p class="sys">${esc(text)}</p>`);
    log.scrollTop = log.scrollHeight;
  }

  // ---------- Thẻ hồ sơ người khác ----------

  function profileDialog() {
    let dlg = $('plaza-profile');
    if (!dlg) {
      dlg = document.createElement('dialog');
      dlg.id = 'plaza-profile';
      dlg.className = 'modal plaza-profile';
      document.body.appendChild(dlg);
      dlg.addEventListener('click', (e) => {
        if (e.target === dlg) dlg.close();
      });
    }
    return dlg;
  }

  function openProfile(id) {
    const p = players.get(id);
    if (!p) return;
    const dlg = profileDialog();
    const full = C && typeof C.full === 'function' ? C.full(p.look || {}, { size: 170 }) : '';
    dlg.innerHTML = `<form method="dialog">
        <div class="pp-head">
          <div class="pp-art">${full}</div>
          <div class="pp-info">
            <h2>${esc(p.name)}</h2>
            <span class="pp-lv">Lv ${p.level || 1}</span>
            ${p.hospital ? '<span class="pp-tag">Đang nằm viện</span>' : p.sitting ? '<span class="pp-tag">Đang ngồi nghỉ</span>' : ''}
          </div>
        </div>
        <div class="pp-actions">
          <button type="button" data-act="wave"><img src="${emoteImg('wave')}" alt="">Vẫy tay</button>
          <button type="button" data-act="gift"><img src="${emoteImg('heart')}" alt="">Tặng xu</button>
          <button type="button" data-act="baucua">${ic('layers')}Mời Bầu cua</button>
          <button type="button" data-act="caro">${ic('play')}Mời Cờ Caro</button>
        </div>
        <div class="pp-gift hidden">
          <p class="muted">Tặng bao nhiêu xu? (tối đa 5.000 xu mỗi ngày)</p>
          <div class="pp-steps">${GIFT_STEPS.map((v) => `<button type="button" data-gift="${v}">${A.xu(v)} xu</button>`).join('')}</div>
        </div>
        <div class="actions"><button value="close">Đóng</button></div>
      </form>`;
    dlg.querySelector('.pp-actions').onclick = async (e) => {
      const b = e.target.closest('[data-act]');
      if (!b) return;
      const act = b.dataset.act;
      if (act === 'wave') {
        send({ t: 'emote', kind: 'wave' });
        send({ t: 'chat', text: `Chào ${p.name}!` });
        dlg.close();
      } else if (act === 'gift') {
        dlg.querySelector('.pp-gift').classList.toggle('hidden');
        const first = dlg.querySelector('[data-gift]');
        if (first) first.focus();
      } else if (act === 'baucua') {
        try {
          const r = await A.api('/api/baucua/rooms', { method: 'POST', body: {} });
          send({ t: 'invite', to: id, game: 'baucua', link: `/baucua/?room=${encodeURIComponent(r.code)}` });
          A.toast(`Đã mời ${p.name} — bàn ${r.code}`);
          dlg.close();
          location.href = `/baucua/?room=${encodeURIComponent(r.code)}`;
        } catch (err) {
          A.toast(err.message);
        }
      } else if (act === 'caro') {
        send({ t: 'invite', to: id, game: 'caro', link: '/caro/' });
        A.toast(`Đã mời ${p.name} chơi Cờ Caro`);
        dlg.close();
      }
    };
    dlg.querySelector('.pp-steps').onclick = async (e) => {
      const b = e.target.closest('[data-gift]');
      if (!b) return;
      const amount = Number(b.dataset.gift);
      dlg.close();
      if (await A.confirm(`Tặng ${p.name} ${A.xu(amount)} xu?`, { title: 'Tặng xu', ok: 'Tặng' })) {
        send({ t: 'gift', to: id, amount });
      }
    };
    dlg.showModal();
    const first = dlg.querySelector('[data-act]');
    if (first) first.focus();
  }

  // Bản đồ có API riêng cho người khác: đăng ký nhận sự kiện bấm (nếu có).
  function hookCity() {
    if (!window.City) return setTimeout(hookCity, 500);
    if (typeof window.City.on === 'function') window.City.on('actorClick', openProfile);
  }
  hookCity();
  // Dự phòng khi bản đồ vẽ người khác bằng phần tử có data-actor-id.
  document.addEventListener('click', (e) => {
    const el = e.target.closest && e.target.closest('[data-actor-id]');
    if (el && !(window.City && typeof window.City.on === 'function')) openProfile(el.dataset.actorId);
  });

  // ---------- Lời mời ----------

  const GAME_NAME = { caro: 'Cờ Caro', baucua: 'Bầu cua', tienlen: 'Tiến Lên' };
  function showInvite(m) {
    buildUI();
    const box = $('plaza-invite');
    box.innerHTML = `<span><b>${esc(m.name)}</b> mời bạn chơi ${esc(GAME_NAME[m.game] || m.game)}</span>
      <a class="btn primary" href="${esc(m.link)}">Vào</a><button type="button" class="link" data-close>Bỏ qua</button>`;
    box.classList.remove('hidden');
    box.querySelector('[data-close]').onclick = () => box.classList.add('hidden');
    clearTimeout(box._t);
    box._t = setTimeout(() => box.classList.add('hidden'), 20000);
    logSystem(`${m.name} mời bạn chơi ${GAME_NAME[m.game] || m.game}`);
  }

  // ---------- Phím tắt ----------

  window.addEventListener('keydown', (e) => {
    if (!cityVisible() || e.ctrlKey || e.metaKey || e.altKey) return;
    const t = e.target;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
    if (document.querySelector('dialog[open]')) return;
    if ((e.key === 't' || e.key === 'T') && !e.shiftKey) {
      e.preventDefault();
      if (inside) openChat();
      return;
    }
    // Shift+1..5: biểu cảm.
    const m = /^Digit([1-5])$/.exec(e.code || '');
    if (m && e.shiftKey && inside) {
      e.preventDefault();
      send({ t: 'emote', kind: EMOTES[Number(m[1]) - 1].kind });
    }
  });

  // Dựng khung chat khi trang sẵn sàng.
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', buildUI);
  else buildUI();

  // Cho test / trang khác dùng.
  window.Plaza = { players, send, openProfile, get you() { return you; } };
})();
