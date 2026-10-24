// Nhân vật DCity bằng ảnh pixel art (Kenney RPG Urban Pack, CC0 — tô lại màu theo trang phục), không vẽ SVG.
// Ảnh ở /img/characters/ (xem manifest.json): sheet 64x48 gồm 4 hướng × 3 khung (đứng, bước 1, bước 2),
// ảnh tĩnh đã cắt sát từng hướng, và ảnh đầu-vai cho avatar.
//   Character.sprite(look, { dir, walking, scale, pose, hurt, cls }) — sprite động (dùng sheet);
//     pose 'lie' = nằm (xoay khung đứng 90°, ví dụ nằm viện), hurt = nháy đỏ khi bị mất HP
//   Character.url(look, dir)                            — URL ảnh tĩnh
//   Character.svg(look, opts)                           — ảnh tĩnh nhìn chính diện (tên cũ, giữ tương thích)
//   Character.head(look, opts)                          — avatar tròn đầu-vai
(function () {
  'use strict';

  const BASE = '/img/characters/';
  // Tên hiển thị và màu chủ đạo (nền avatar) của từng trang phục; id khớp internal/wallet/character.go.
  const OUTFITS = {
    casual: { name: 'Thường ngày', top: '#4f8ef7' },
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

  function url(look, dir) {
    return `${BASE}${key(look)}_${dirOf(dir)}.png`;
  }

  // Sprite động: một khung 16x16 trong sheet, phóng to số nguyên lần; walking luân phiên 2 khung bước.
  // pose 'lie' cho nhân vật nằm (không bước đi), hurt cho hiệu ứng nháy đỏ.
  function sprite(look = {}, opts = {}) {
    const scale = Math.max(1, Math.round(opts.scale || 4));
    const lying = opts.pose === 'lie';
    const cls = 'char-sprite' + (opts.walking && !lying ? ' walking' : '') + (lying ? ' lying' : '') +
      (opts.hurt ? ' hurt' : '') + (opts.cls ? ' ' + opts.cls : '');
    return `<span class="${cls}" data-dir="${dirOf(opts.dir)}" role="img" aria-label="Nhân vật"
      style="--sheet:url('${BASE}${key(look)}.png');--s:${scale}"></span>`;
  }

  // Ảnh tĩnh nhìn chính diện, chiều cao theo CSS (class "character") hoặc opts.size.
  function svg(look = {}, opts = {}) {
    const cls = 'character char-img' + (opts.hurt ? ' hurt' : '') + (opts.cls ? ' ' + opts.cls : '');
    const size = opts.size ? ` style="height:${opts.size}px"` : '';
    return `<img class="${cls}" src="${url(look, opts.dir || 'down')}" alt="" draggable="false"${size}>`;
  }

  function head(look = {}, opts = {}) {
    // Áo dài nam màu xanh, nữ màu hồng.
    const color = look.outfit === 'aodai' && look.gender !== 'female' ? '#2042a0' : (OUTFITS[look.outfit] || OUTFITS.casual).top;
    const cls = 'avatar char-avatar' + (opts.cls ? ' ' + opts.cls : '');
    return `<span class="${cls}" style="--avatar:${color}"><img src="${BASE}${key(look)}_head.png" alt="" draggable="false"></span>`;
  }

  // Trên bản đồ (bên trong <svg>): thẻ <image> trỏ tới ảnh PNG, tâm ở chân nhân vật.
  function topdown(look = {}, dir = 'down') {
    return `<image href="${url(look, dir)}" x="-12" y="-28" width="24" height="30" style="image-rendering:pixelated"/>`;
  }
  const group = topdown;

  window.Character = { sprite, url, svg, head, topdown, group, OUTFITS, GENDERS, DIRS };
})();
