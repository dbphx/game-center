// Nhân vật DCity kiểu hoạt hình chibi (Kenney Toon Characters, CC0 — tô lại màu theo trang phục), không vẽ SVG.
// Ảnh ở /img/characters/ (xem manifest.json):
//   <look>.png       sprite sheet 4 hàng (down, left, right, up) × 5 cột (đứng + 4 khung bước), mỗi khung 96×148,
//                    chân nhân vật ở giữa đáy khung (x 48, y 147)
//   <look>_<dir>.png ảnh đứng từng hướng (cùng cỡ khung)
//   <look>_full.png  ảnh toàn thân chính diện cắt sát (xem trước ở đăng ký / shop / quảng trường)
//   <look>_head.png  chân dung đầu-vai 128×128 (avatar)
//   Character.sprite(look, { dir, walking, scale, size, pose, hurt, cls }) — sprite động
//     size = chiều cao hiển thị (px); không có size thì 24 × scale (tương thích bản pixel cũ: scale 3 → 72px)
//     pose 'lie' = nằm (xoay khung đứng), hurt = nháy đỏ khi mất HP
//   Character.url(look, dir)      — URL ảnh đứng theo hướng
//   Character.svg(look, opts)     — ảnh toàn thân hoạt hình (tên cũ, giữ tương thích); opts.size = chiều cao px
//   Character.head(look, opts)    — avatar tròn chân dung
// window.Portrait (cùng file): full(look, {size}), head(look, {size}), url(look, 'full'|'head').
(function () {
  'use strict';

  const BASE = '/img/characters/';
  const FRAME_W = 96;
  const FRAME_H = 148;
  // Tên hiển thị và màu chủ đạo (nền avatar) của từng trang phục; id khớp internal/wallet/character.go.
  const OUTFITS = {
    casual: { name: 'Thường ngày', top: '#3d7ff0' },
    sport: { name: 'Thể thao', top: '#e5484d' },
    hoodie: { name: 'Áo hoodie', top: '#7e57c2' },
    suit: { name: 'Vest lịch lãm', top: '#2c3a50' },
    aodai: { name: 'Áo dài', top: '#ec609e' },
    royal: { name: 'Hoàng gia', top: '#b71c1c' },
  };
  const GENDERS = { male: 'Nam', female: 'Nữ' };
  const DIRS = ['down', 'left', 'right', 'up'];

  const key = (look = {}) => {
    const g = look.gender === 'female' ? 'female' : 'male';
    const o = OUTFITS[look.outfit] ? look.outfit : 'casual';
    return `${g}_${o}`;
  };
  const dirOf = (d) => (DIRS.includes(d) ? d : 'down');
  // Áo dài nam màu xanh, nữ màu hồng.
  const tint = (look = {}) =>
    look.outfit === 'aodai' && look.gender !== 'female' ? '#2d62c8' : (OUTFITS[look.outfit] || OUTFITS.casual).top;

  function url(look, dir) {
    return `${BASE}${key(look)}_${dirOf(dir)}.png`;
  }

  function sprite(look = {}, opts = {}) {
    const h = Math.round(opts.size || 24 * (opts.scale || 3));
    const w = Math.round((h * FRAME_W) / FRAME_H);
    const lying = opts.pose === 'lie';
    const cls = 'char-sprite' + (opts.walking && !lying ? ' walking' : '') + (lying ? ' lying' : '') +
      (opts.hurt ? ' hurt' : '') + (opts.cls ? ' ' + opts.cls : '');
    return `<span class="${cls}" data-dir="${dirOf(opts.dir)}" role="img" aria-label="Nhân vật"
      style="--sheet:url('${BASE}${key(look)}.png');--w:${w}px;--h:${h}px"></span>`;
  }

  // Ảnh toàn thân hoạt hình, chiều cao theo CSS (class "character") hoặc opts.size.
  function full(look = {}, opts = {}) {
    const cls = 'character char-img portrait-full' + (opts.hurt ? ' hurt' : '') + (opts.cls ? ' ' + opts.cls : '');
    const size = opts.size ? ` style="height:${opts.size}px"` : '';
    return `<img class="${cls}" src="${BASE}${key(look)}_full.png" alt="" draggable="false"${size}>`;
  }

  function head(look = {}, opts = {}) {
    const cls = 'avatar char-avatar portrait-avatar' + (opts.cls ? ' ' + opts.cls : '');
    const size = opts.size ? `;width:${opts.size}px;height:${opts.size}px` : '';
    return `<span class="${cls}" style="--avatar:${tint(look)}${size}"><img src="${BASE}${key(look)}_head.png" alt="" draggable="false"></span>`;
  }

  // Bên trong một <svg> cũ: thẻ <image> ảnh đứng, chân nhân vật ở (0, 0).
  function topdown(look = {}, dir = 'down') {
    return `<image href="${url(look, dir)}" x="-24" y="-74" width="48" height="74"/>`;
  }

  window.Character = { sprite, url, svg: full, head, topdown, group: topdown, OUTFITS, GENDERS, DIRS, FRAME_W, FRAME_H };
  window.Portrait = {
    full: (look, opts = {}) => full(look, { size: 140, ...opts }),
    head,
    url: (look, kind) => `${BASE}${key(look)}_${kind === 'head' ? 'head' : 'full'}.png`,
  };
})();
