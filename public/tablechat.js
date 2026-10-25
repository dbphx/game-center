// Chat theo bàn dùng chung cho các game casino (Tiến Lên, Cờ Caro, Bầu Cua).
// TableChat.mount({ send(text), anchor(line) → phần tử để hiện bong bóng | null, isMe(line) → bool })
// trả về { add(line) — tin mới (hiện bong bóng 4 giây), sync(lines) — lịch sử từ state (chỉ tin có seq mới hiện bong bóng) }.
// Tin biểu cảm là chuỗi "::wave::" (wave | heart | laugh | clap | dance), hiện bằng ảnh.
(function () {
  const A = window.Account;
  const BUBBLE_MS = 4000;
  const EMOTES = [
    { kind: 'wave', name: 'Vẫy tay' },
    { kind: 'heart', name: 'Thả tim' },
    { kind: 'laugh', name: 'Cười' },
    { kind: 'clap', name: 'Vỗ tay' },
    { kind: 'dance', name: 'Nhảy' },
  ];
  const QUICK = ['Nhanh lên!', 'Hay quá!', 'Chơi tiếp không?', 'Cảm ơn!'];
  const emoteOf = (text) => {
    const m = /^::(wave|heart|laugh|clap|dance)::$/.exec(text || '');
    return m ? m[1] : null;
  };
  const emoteImg = (kind) => `<img class="tc-emote" src="/img/toon/city/emote_${kind}.png" alt="${kind}" draggable="false">`;
  const esc = (s) => A.escapeHtml(s);
  const body = (text) => (emoteOf(text) ? emoteImg(emoteOf(text)) : esc(text));

  function mount(opts) {
    const root = document.createElement('section');
    root.className = 'tablechat collapsed';
    root.setAttribute('aria-label', 'Trò chuyện trong bàn');
    root.innerHTML = `
      <button type="button" class="tc-head" title="Trò chuyện (phím T)">
        <img src="/img/toon/city/emote_wave.png" alt="" draggable="false"><span>Trò chuyện</span><kbd>T</kbd>
        <span class="tc-unread hidden"></span>
      </button>
      <div class="tc-body">
        <div class="tc-log" aria-live="polite"></div>
        <div class="tc-quick">
          ${QUICK.map((q) => `<button type="button" data-quick="${esc(q)}">${esc(q)}</button>`).join('')}
          ${EMOTES.map((e) => `<button type="button" class="tc-emo" data-emote="${e.kind}" title="${e.name}">${emoteImg(e.kind)}</button>`).join('')}
        </div>
        <form class="tc-form" autocomplete="off">
          <input class="tc-input" maxlength="120" placeholder="Nhắn gì đó… (Enter gửi, Esc đóng)">
          <button type="submit" class="primary">Gửi</button>
        </form>
      </div>`;
    document.body.appendChild(root);
    const $ = (s) => root.querySelector(s);
    const log = $('.tc-log');
    const input = $('.tc-input');
    const unread = $('.tc-unread');
    let lastSeq = null; // seq lớn nhất đã thấy; null = chưa đồng bộ lần nào
    let unreadN = 0;

    const isOpen = () => !root.classList.contains('collapsed');
    const open = () => {
      root.classList.remove('collapsed');
      unreadN = 0;
      unread.classList.add('hidden');
      log.scrollTop = log.scrollHeight;
      setTimeout(() => input.focus(), 0);
    };
    const close = () => {
      root.classList.add('collapsed');
      input.blur();
    };
    $('.tc-head').onclick = () => (isOpen() ? close() : open());

    const send = (text) => {
      text = String(text || '').trim();
      if (!text) return;
      Promise.resolve(opts.send(text)).catch((e) => A.toast(e && e.message ? e.message : 'Không gửi được'));
    };
    $('.tc-form').onsubmit = (e) => {
      e.preventDefault();
      send(input.value);
      input.value = '';
    };
    root.addEventListener('click', (e) => {
      const q = e.target.closest('[data-quick]');
      if (q) return send(q.dataset.quick);
      const em = e.target.closest('[data-emote]');
      if (em) send(`::${em.dataset.emote}::`);
    });

    // Phím: T mở chat (khi không gõ chữ, không mở hộp thoại); khi đang gõ trong ô chat thì chặn phím tắt của game.
    window.addEventListener('keydown', (e) => {
      if (e.target === input) {
        if (e.key === 'Escape') {
          e.preventDefault();
          close();
        }
        e.stopImmediatePropagation();
        return;
      }
      if ((e.key === 't' || e.key === 'T') && !e.ctrlKey && !e.metaKey && !e.altKey) {
        const t = e.target;
        if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
        if (document.querySelector('dialog[open]')) return;
        e.preventDefault();
        e.stopImmediatePropagation();
        open();
      }
    }, true);
    ['keyup', 'keypress'].forEach((type) => window.addEventListener(type, (e) => {
      if (e.target === input) e.stopImmediatePropagation();
    }, true));

    function appendLog(line) {
      const me = opts.isMe && opts.isMe(line);
      const row = document.createElement('div');
      row.className = 'tc-line' + (me ? ' me' : '');
      row.innerHTML = `<b>${esc(me ? 'Bạn' : line.name)}</b><span>${body(line.text)}</span>`;
      log.appendChild(row);
      while (log.children.length > 20) log.firstChild.remove();
      log.scrollTop = log.scrollHeight;
    }

    // Bong bóng nổi trên phần tử neo (theo toạ độ màn hình lúc tin tới), tự mất sau 4 giây.
    function bubble(line) {
      const el = opts.anchor && opts.anchor(line);
      if (!el || !el.getClientRects().length) return;
      const r = el.getBoundingClientRect();
      const b = document.createElement('div');
      b.className = 'tc-bubble' + (emoteOf(line.text) ? ' emo' : '');
      b.innerHTML = body(line.text);
      b.style.left = `${Math.round(r.left + r.width / 2)}px`;
      b.style.top = `${Math.round(r.top)}px`;
      document.body.appendChild(b);
      setTimeout(() => b.classList.add('out'), BUBBLE_MS - 300);
      setTimeout(() => b.remove(), BUBBLE_MS);
    }

    function add(line) {
      if (line.seq != null) {
        if (lastSeq != null && line.seq <= lastSeq) return; // đã có
        lastSeq = line.seq;
      }
      appendLog(line);
      bubble(line);
      if (!isOpen() && !(opts.isMe && opts.isMe(line))) {
        unreadN++;
        unread.textContent = unreadN > 9 ? '9+' : String(unreadN);
        unread.classList.remove('hidden');
      }
    }

    function sync(lines) {
      lines = lines || [];
      if (lastSeq == null) {
        // Lần đầu: chỉ nạp lịch sử, không bong bóng.
        log.innerHTML = '';
        lines.forEach(appendLog);
        lastSeq = lines.length ? lines[lines.length - 1].seq : 0;
        return;
      }
      if (lines.length && lines[lines.length - 1].seq < lastSeq) {
        // Bàn khác / bàn mới: nạp lại từ đầu.
        lastSeq = null;
        return sync(lines);
      }
      lines.filter((l) => l.seq > lastSeq).forEach(add);
    }

    function reset() {
      lastSeq = null;
      log.innerHTML = '';
    }

    return { add, sync, reset, open, close, el: root };
  }

  window.TableChat = { mount, BUBBLE_MS };
})();
