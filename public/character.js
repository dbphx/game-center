// Nhân vật DCity kiểu hoạt hình chibi (Kenney Toon Characters, CC0 — tô lại màu theo trang phục), không vẽ SVG.
// Ảnh ở /img/characters/ (xem manifest.json):
//   <look>.png       sprite sheet 4 hàng (down, left, right, up) × 5 cột (đứng + 4 khung bước), mỗi khung 96×148,
//                    chân nhân vật ở giữa đáy khung (x 48, y 147)
//   <look>_<dir>.png ảnh đứng từng hướng (cùng cỡ khung)
//   <look>_full.png  ảnh toàn thân chính diện cắt sát (xem trước ở đăng ký / shop / quảng trường)
//   <look>_head.png  chân dung đầu-vai 128×128 (avatar)
//   face/, hair/, wings/, crown/ — lớp mặt, tóc, cánh, vương miện chồng lên nhân vật (cùng bố cục khung, xem manifest "layers")
//   Thứ tự lớp: cánh sau → thân → mặt → tóc → vương miện → cánh trước (nhìn sau lưng).
// look = { gender, outfit, hair, wings, face } — thiếu hair / wings / face thì vẽ như cũ;
// truyền thẳng object user cũng được.
//   Character.sprite(look, { dir, walking, scale, size, pose, hurt, cls }) — sprite động
//     size = chiều cao hiển thị (px) khi đứng; không có size thì 24 × scale (tương thích bản pixel cũ: scale 3 → 72px)
//     pose 'sit' = ngồi (<look>_sit.png, 1 khung × 4 hướng), 'lie' = nằm ngửa nhắm mắt (<look>_lie.png, khung 148×96,
//     đầu trái / phải theo dir), hurt = nháy đỏ khi mất HP
//     Hiệu ứng (CSS transform / opacity / filter trên các lớp PNG): cánh vỗ (đứng chậm, đi nhanh), cánh thiên thần / tiên
//     phát sáng + hạt lấp lánh rơi (fx/sparkle_*.png), dơi ánh tím, rồng ánh lửa; mặt nháy mắt ngẫu nhiên 3–6 s
//     (face/<mặt>_<giới>_blink.png), mắt sao lấp lánh, má ửng, kính loé sáng (face/<mặt>_<giới>_fx.png).
//   Character.url(look, dir)      — URL ảnh đứng theo hướng (chỉ thân, không có lớp tóc / cánh)
//   Character.svg(look, opts)     — ảnh toàn thân hoạt hình (tên cũ, giữ tương thích); opts.size = chiều cao px
//   Character.head(look, opts)    — avatar tròn chân dung
//   Character.HAIR / WINGS / FACES — tên hiển thị các kiểu tóc / cánh / gương mặt (id khớp internal/wallet)
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
    school: { name: 'Đồng phục học sinh', top: '#1f3a68' },
    aobaba: { name: 'Áo bà ba', top: '#7a4b2a' },
    tet: { name: 'Áo Tết', top: '#d32f2f' },
    pirate: { name: 'Hải tặc', top: '#3e2723' },
    wedding: { name: 'Đồ cưới', top: '#e9a8c0' },
    astronaut: { name: 'Phi hành gia', top: '#546e7a' },
  };
  // Kiểu tóc ("default" là tóc gốc của nhân vật, không có lớp) và cánh ("" là không cánh).
  const HAIR = {
    default: { name: 'Tóc mặc định' },
    short_spiky: { name: 'Tóc dựng' },
    ponytail: { name: 'Đuôi ngựa' },
    long_wave: { name: 'Tóc dài xoăn' },
    bun: { name: 'Tóc búi' },
    mohawk: { name: 'Mohican' },
    pink_twin: { name: 'Hai bím hồng' },
  };
  const WINGS = {
    angel: { name: 'Cánh thiên thần' },
    fairy: { name: 'Cánh tiên' },
    butterfly: { name: 'Cánh bướm' },
    bat: { name: 'Cánh dơi' },
    dragon: { name: 'Cánh rồng' },
  };
  // Gương mặt ("default" là mặt gốc, không có lớp).
  const FACES = {
    default: { name: 'Mặt mặc định' },
    smile: { name: 'Cười tươi' },
    wink: { name: 'Nháy mắt' },
    blush: { name: 'Má hồng' },
    cool: { name: 'Kính râm' },
    cat: { name: 'Mèo con' },
    star: { name: 'Mắt lấp lánh' },
  };
  // Mặt có lớp mắt nhắm (nháy mắt) và lớp hiệu ứng riêng; "cool" đeo kính, "smile" mắt đã híp nên không nháy.
  const BLINK = ['default', 'wink', 'blush', 'cat', 'star'];
  const FX = ['star', 'blush', 'cool'];
  // Cánh có hạt lấp lánh rơi.
  const SPARK = { angel: 'gold', fairy: 'pastel' };
  const GENDERS = { male: 'Nam', female: 'Nữ' };
  const DIRS = ['down', 'left', 'right', 'up'];
  // Ảnh toàn thân của mỗi giới tính (px) và lề trên của lớp tóc / cánh toàn thân; đồ Hoàng gia nữ cao hơn vì tiara.
  const FULL = { male: { h: 243, pad: 73 }, female: { h: 217, pad: 65 } };
  const FULL_H = { female_royal: 229 };

  const genderOf = (look = {}) => (look.gender === 'female' ? 'female' : 'male');
  const key = (look = {}) => {
    const o = OUTFITS[look.outfit] ? look.outfit : 'casual';
    return `${genderOf(look)}_${o}`;
  };
  // Lớp chồng của một look: mặt, tóc (bỏ qua mặc định), cánh, vương miện (đồ Hoàng gia đội trên tóc mới).
  function layers(look = {}) {
    const g = genderOf(look);
    const hair = look.hair && look.hair !== 'default' && HAIR[look.hair] ? look.hair : '';
    const wings = look.wings && WINGS[look.wings] ? look.wings : '';
    const face = look.face && look.face !== 'default' && FACES[look.face] ? look.face : '';
    const fid = face || 'default';
    return {
      g,
      wid: wings,
      face: face && `${BASE}face/${face}_${g}`,
      blink: BLINK.includes(fid) ? `${BASE}face/${fid}_${g}_blink` : '',
      fx: FX.includes(fid) ? `${BASE}face/${fid}_${g}_fx` : '',
      fxKind: FX.includes(fid) ? fid : '',
      hair: hair && `${BASE}hair/${hair}_${g}`,
      crown: hair && look.outfit === 'royal' ? `${BASE}crown/${g}` : '',
      wings: wings && `${BASE}wings/${wings}_${g}`,
    };
  }
  const dirOf = (d) => (DIRS.includes(d) ? d : 'down');
  // Áo dài nam màu xanh, nữ màu hồng.
  const tint = (look = {}) =>
    look.outfit === 'aodai' && look.gender !== 'female' ? '#2d62c8' : (OUTFITS[look.outfit] || OUTFITS.casual).top;

  function url(look, dir) {
    return `${BASE}${key(look)}_${dirOf(dir)}.png`;
  }

  // Nhịp nháy mắt ngẫu nhiên cho mỗi lần vẽ (3–6 s, lệch pha) để các nhân vật không nháy cùng lúc.
  const blinkStyle = () => `--blink:${(3 + Math.random() * 3).toFixed(2)}s;animation-delay:-${(Math.random() * 6).toFixed(2)}s`;

  function sprite(look = {}, opts = {}) {
    const h = Math.round(opts.size || 24 * (opts.scale || 3));
    const pose = opts.pose === 'sit' || opts.pose === 'lie' ? opts.pose : '';
    const lie = pose === 'lie';
    // Nằm: khung 148×96, chiều dài người nằm = chiều cao khi đứng.
    const w = lie ? h : Math.round((h * FRAME_W) / FRAME_H);
    const hh = lie ? Math.round((h * FRAME_W) / FRAME_H) : h;
    const dir = dirOf(opts.dir);
    const L = layers(look);
    const sfx = pose ? '_' + pose : '';
    const grid = pose === 'sit' ? '--cols:1;--rows:4' : lie ? '--cols:1;--rows:2' : '--cols:5;--rows:4';
    const walking = opts.walking && !pose;
    const wings = L.wings && !lie;
    const cls = 'char-sprite' + (pose ? ' pose-' + pose : '') + (walking ? ' walking' : '') + (opts.hurt ? ' hurt' : '') +
      (wings ? ' has-wings' : '') + (opts.cls ? ' ' + opts.cls : '');
    const row = lie ? (dir === 'right' ? 1 : 0) : null;
    const lay = (c, u, extra = '') => `<i class="${c}" style="--img:url('${u}${sfx}.png')${extra}"></i>`;
    const spark = wings && SPARK[L.wid] ? `<i class="cs-spark" style="--img:url('${BASE}fx/sparkle_${SPARK[L.wid]}_1.png')"></i><i
      class="cs-spark s2" style="--img:url('${BASE}fx/sparkle_${SPARK[L.wid]}_2.png')"></i>` : '';
    return `<span class="${cls}" data-dir="${dir}"${wings ? ` data-wing="${L.wid}"` : ''} role="img" aria-label="Nhân vật"
      style="--w:${w}px;--h:${hh}px;${grid}${row !== null ? `;--row:${row}` : ''}">${
      wings ? lay('cs-wing', `${L.wings}_back`) : ''}${lay('cs-body', `${BASE}${key(look)}`)}${
      L.face ? lay('cs-face', L.face) : ''}${
      L.blink ? lay('cs-blink' + (lie ? ' shut' : ''), L.blink, lie ? '' : ';' + blinkStyle()) : ''}${
      L.fx && !lie ? lay(`cs-fx fx-${L.fxKind}`, L.fx) : ''}${
      L.hair ? lay('cs-hair', L.hair) : ''}${L.crown ? lay('cs-crown', L.crown) : ''}${
      wings ? lay('cs-wing front', `${L.wings}_front`) + spark : ''}</span>`;
  }

  // Ảnh toàn thân hoạt hình, chiều cao theo CSS (class "character") hoặc opts.size.
  function full(look = {}, opts = {}) {
    const cls = 'character char-img portrait-full' + (opts.hurt ? ' hurt' : '') + (opts.cls ? ' ' + opts.cls : '');
    const size = opts.size ? ` style="height:${opts.size}px"` : '';
    const src = `${BASE}${key(look)}_full.png`;
    const L = layers(look);
    if (!L.hair && !L.wings && !L.face && !L.blink) return `<img class="${cls}" src="${src}" alt="" draggable="false"${size}>`;
    // Lớp tóc / cánh có lề trên và căn đáy theo ảnh thân của giới tính; tính theo % để hợp mọi cỡ hiển thị.
    const f = FULL[L.g];
    const baseH = FULL_H[key(look)] || f.h;
    const top = (((baseH - f.h - f.pad) / baseH) * 100).toFixed(2);
    const hgt = (((f.h + f.pad) / baseH) * 100).toFixed(2);
    // Lớp mặt cùng cỡ ảnh thân của giới tính (không có lề trên).
    const ftop = (((baseH - f.h) / baseH) * 100).toFixed(2);
    const fhgt = ((f.h / baseH) * 100).toFixed(2);
    const lay = (u, extra = '') => `<img class="char-layer${extra}" src="${u}" alt="" draggable="false" style="top:${top}%;height:${hgt}%">`;
    const flay = (u, c) => `<img class="char-layer ${c}" src="${u}_full.png" alt="" draggable="false" style="top:${ftop}%;height:${fhgt}%">`;
    return `<span class="${cls} char-stack"${L.wid ? ` data-wing="${L.wid}"` : ''}${size}><span class="char-stack-in">${
      L.wings ? lay(`${L.wings}_full.png`, ' wing') : ''}<img class="char-base" src="${src}" alt="" draggable="false">${
      L.face ? flay(L.face, 'face') : ''}${
      L.blink ? flay(L.blink, 'blink').replace('style="', `style="${blinkStyle()};`) : ''}${
      L.fx ? flay(L.fx, 'fx fx-' + L.fxKind) : ''}${
      L.hair ? lay(`${L.hair}_full.png`) : ''}${
      L.crown ? `<img class="char-layer crown" src="${L.crown}_full.png" alt="" draggable="false">` : ''}</span></span>`;
  }

  function head(look = {}, opts = {}) {
    const cls = 'avatar char-avatar portrait-avatar' + (opts.cls ? ' ' + opts.cls : '');
    const size = opts.size ? `;width:${opts.size}px;height:${opts.size}px` : '';
    const L = layers(look);
    const extra = [L.face, L.hair, L.crown].filter(Boolean)
      .map((u) => `<img class="char-layer" src="${u}_head.png" alt="" draggable="false">`).join('');
    return `<span class="${cls}" style="--avatar:${tint(look)}${size}"><img src="${BASE}${key(look)}_head.png" alt="" draggable="false">${extra}</span>`;
  }

  // Bên trong một <svg> cũ: thẻ <image> ảnh đứng, chân nhân vật ở (0, 0).
  function topdown(look = {}, dir = 'down') {
    return `<image href="${url(look, dir)}" x="-24" y="-74" width="48" height="74"/>`;
  }

  window.Character = { sprite, url, svg: full, head, topdown, group: topdown, OUTFITS, HAIR, WINGS, FACES, BLINK, FX, GENDERS, DIRS, FRAME_W, FRAME_H };
  window.Portrait = {
    full: (look, opts = {}) => full(look, { size: 140, ...opts }),
    head,
    url: (look, kind) => `${BASE}${key(look)}_${kind === 'head' ? 'head' : 'full'}.png`,
  };
})();
