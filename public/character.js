// Vẽ nhân vật chibi bằng SVG theo giới tính và trang phục (id khớp với internal/wallet/character.go).
// Character.svg(look) cho cả người, Character.head(look) cho avatar tròn.
(function () {
  'use strict';

  const OUTFITS = {
    casual: { name: 'Thường ngày', top: '#4f8ef7', trim: '#3b6fd1', pants: '#2f3e56', shoes: '#f4f4f4' },
    sport: { name: 'Thể thao', top: '#e5484d', trim: '#ffffff', pants: '#e5484d', shoes: '#ffffff' },
    hoodie: { name: 'Áo hoodie', top: '#7e57c2', trim: '#b39ddb', pants: '#2f3e56', shoes: '#f4f4f4', hood: '#5e3fa0' },
    suit: { name: 'Vest lịch lãm', top: '#263445', trim: '#1a2531', pants: '#263445', shoes: '#17110d' },
    aodai: { name: 'Áo dài', top: '#e85d9b', trim: '#f6c744', pants: '#fbfbfb', shoes: '#c98b2c' },
    royal: { name: 'Hoàng gia', top: '#b71c1c', trim: '#f6c744', pants: '#7a1414', shoes: '#c98b2c' },
  };
  const GOLD = '#f6c744';
  const GOLD_DARK = '#b98a00';
  const GENDERS = { male: 'Nam', female: 'Nữ' };
  const SKIN = '#f6d2b5';
  const HAIR = { male: '#3a2a20', female: '#4a2c1d' };

  // Áo dài nam màu xanh cho khác áo dài nữ.
  const palette = (look) =>
    look.outfit === 'aodai' && look.gender === 'male' ? { ...OUTFITS.aodai, top: '#2d7dd2' } : OUTFITS[look.outfit] || OUTFITS.casual;

  function hairBack(g) {
    return g === 'female'
      ? `<path d="M13 27 Q11 7 32 6 Q53 7 51 27 L52 54 Q47 58 43 51 L43 30 L21 30 L21 51 Q17 58 12 54 Z" fill="${HAIR.female}"/>`
      : '';
  }

  // Nơ hồng của tóc nữ; bỏ khi đội tiara hoàng gia.
  function hairFront(g, outfit) {
    return g === 'female'
      ? `<path d="M15 26 Q15 9 32 8 Q49 9 49 26 Q43 15 35 17 Q27 13 15 26 Z" fill="${HAIR.female}"/>` +
        (outfit === 'royal' ? '' : `<circle cx="47" cy="15" r="3.4" fill="#ff7aa8"/>`)
      : `<path d="M15 26 Q13 8 32 7 Q51 8 49 26 Q47 18 41 16 Q37 20 31 16 Q25 20 20 17 Q16 20 15 26 Z" fill="${HAIR.male}"/>`;
  }

  // Lớp sau lưng: mũ trùm của hoodie nằm sau đầu, lộ ra hai bên cổ.
  function backLayer(look) {
    if (look.outfit !== 'hoodie') return '';
    const p = palette(look);
    return `<path d="M13 44 Q11 22 32 19 Q53 22 51 44 L46 48 L18 48 Z" fill="${p.hood}"/>
      <path d="M17 44 Q16 27 32 24 Q48 27 47 44" fill="none" stroke="${p.trim}" stroke-width="1.5" opacity=".6"/>`;
  }

  // Vương miện (nam) / tiara (nữ) vàng gắn ngọc cho trang phục hoàng gia.
  function crown(look) {
    if (look.outfit !== 'royal') return '';
    if (look.gender === 'female') {
      return `<path d="M23 13 L25.5 7.5 L28.5 11 L32 4.5 L35.5 11 L38.5 7.5 L41 13 Z" fill="${GOLD}" stroke="${GOLD_DARK}" stroke-width="1"/>
        <circle cx="32" cy="9.5" r="1.8" fill="#e91e63"/>`;
    }
    return `<path d="M20.5 15 L22 5 L27.5 10 L32 3 L36.5 10 L42 5 L43.5 15 Z" fill="${GOLD}" stroke="${GOLD_DARK}" stroke-width="1"/>
      <rect x="20.5" y="12.5" width="23" height="3.5" rx="1" fill="${GOLD_DARK}"/>
      <circle cx="32" cy="9" r="1.9" fill="#e53935"/><circle cx="25.5" cy="12" r="1.2" fill="#1e88e5"/><circle cx="38.5" cy="12" r="1.2" fill="#1e88e5"/>
      <circle cx="22" cy="5" r="1.1" fill="${GOLD}"/><circle cx="32" cy="3" r="1.1" fill="${GOLD}"/><circle cx="42" cy="5" r="1.1" fill="${GOLD}"/>`;
  }

  function face() {
    return `<circle cx="32" cy="26" r="17" fill="${SKIN}"/>
      <ellipse cx="25.5" cy="28" rx="2.3" ry="2.8" fill="#2b1d16"/>
      <ellipse cx="38.5" cy="28" rx="2.3" ry="2.8" fill="#2b1d16"/>
      <circle cx="26.3" cy="27" r=".8" fill="#fff"/><circle cx="39.3" cy="27" r=".8" fill="#fff"/>
      <ellipse cx="21.5" cy="33" rx="2.8" ry="1.6" fill="#ff9a9a" opacity=".55"/>
      <ellipse cx="42.5" cy="33" rx="2.8" ry="1.6" fill="#ff9a9a" opacity=".55"/>
      <path d="M28.5 35 Q32 38 35.5 35" stroke="#7a3b2b" stroke-width="1.6" fill="none" stroke-linecap="round"/>`;
  }

  // Thân, tay, chân theo trang phục. Tay / chân có class để CSS tạo bước đi.
  function body(look) {
    const p = palette(look);
    const female = look.gender === 'female';
    const longTunic = look.outfit === 'aodai';
    const robe = look.outfit === 'royal';
    const skirt = female && look.outfit === 'suit';
    const legColor = skirt ? SKIN : p.pants;

    const legs = `
      <g class="leg leg-l"><rect x="24" y="64" width="7" height="22" rx="3" fill="${legColor}"/><rect x="22.5" y="84" width="9.5" height="5" rx="2.5" fill="${p.shoes}"/></g>
      <g class="leg leg-r"><rect x="33" y="64" width="7" height="22" rx="3" fill="${legColor}"/><rect x="32" y="84" width="9.5" height="5" rx="2.5" fill="${p.shoes}"/></g>`;

    const short = look.outfit === 'casual';
    const sleeve = short
      ? (x) => `<rect x="${x}" y="46" width="7" height="8" rx="3" fill="${p.top}"/><rect x="${x + 0.5}" y="52" width="6" height="12" rx="3" fill="${SKIN}"/>`
      : (x) => `<rect x="${x}" y="46" width="7" height="18" rx="3.5" fill="${p.top}"/>` +
        (look.outfit === 'hoodie' || robe ? `<rect x="${x}" y="60" width="7" height="4" rx="1.5" fill="${p.trim}"/>` : '');
    const arms = `
      <g class="arm arm-l">${sleeve(11)}<circle cx="14.5" cy="65" r="3.2" fill="${SKIN}"/></g>
      <g class="arm arm-r">${sleeve(46)}<circle cx="49.5" cy="65" r="3.2" fill="${SKIN}"/></g>`;

    let torso = `<rect x="17" y="43" width="30" height="25" rx="9" fill="${p.top}"/>`;
    let detail = '';
    if (robe) {
      torso = `<path d="M17 50 Q17 43 25 43 L39 43 Q47 43 47 50 L51 85 Q32 89 13 85 Z" fill="${p.top}"/>`;
      const dots = [[22, 46], [27, 48.5], [37, 48.5], [42, 46], [32, 50]].map(([x, y]) => `<circle cx="${x}" cy="${y}" r=".9" fill="#222"/>`).join('');
      detail = `<path d="M13 85 Q32 89 51 85" fill="none" stroke="${GOLD}" stroke-width="2.5"/>
        <path d="M32 50 L32 86" stroke="${GOLD}" stroke-width="2.2"/>
        <rect x="18" y="59" width="28" height="4" rx="1" fill="${GOLD}"/><rect x="29.5" y="58" width="5" height="6" rx="1" fill="${GOLD_DARK}"/>
        <path d="M17 46 Q32 55 47 46 Q47 40.5 40 40.5 L24 40.5 Q17 40.5 17 46 Z" fill="#fff" stroke="#e0e0e0" stroke-width=".8"/>${dots}`;
    } else if (look.outfit === 'hoodie') {
      detail = `<path d="M22.5 56 L41.5 56 L43.5 66 L20.5 66 Z" fill="${p.hood}" opacity=".55"/>
        <path d="M22.5 56 L41.5 56" stroke="${p.trim}" stroke-width="1.2" opacity=".8"/>
        <path d="M29 44 L28.5 53 M35 44 L35.5 53" stroke="#fff" stroke-width="1.3" stroke-linecap="round"/>
        <circle cx="28.5" cy="53.5" r="1.1" fill="#fff"/><circle cx="35.5" cy="53.5" r="1.1" fill="#fff"/>
        <rect x="17" y="64" width="30" height="4" rx="2" fill="${p.trim}" opacity=".7"/>`;
    } else if (longTunic) {
      torso = `<path d="M18 50 Q18 43 26 43 L38 43 Q46 43 46 50 L49 82 Q32 86 15 82 Z" fill="${p.top}"/>`;
      detail = `<path d="M27 43 L32 49 L37 43" fill="none" stroke="${p.trim}" stroke-width="2"/>
        <circle cx="26" cy="58" r="1.4" fill="${p.trim}"/><circle cx="38" cy="64" r="1.4" fill="${p.trim}"/><circle cx="30" cy="72" r="1.4" fill="${p.trim}"/>`;
    } else if (look.outfit === 'suit') {
      detail = `<path d="M26 43 L32 55 L38 43 Z" fill="#fff"/>` +
        (female ? `<circle cx="32" cy="50" r="1.6" fill="#f6c744"/>` : `<path d="M31 47 L33 47 L34 58 L32 61 L30 58 Z" fill="#d93a3a"/>`) +
        `<circle cx="32" cy="61" r="1" fill="#93a3b5"/>`;
    } else if (look.outfit === 'sport') {
      detail = `<rect x="31" y="43" width="2" height="25" fill="#fff" opacity=".9"/>
        <path d="M24 46 L24 66 M40 46 L40 66" stroke="#fff" stroke-width="1.5" opacity=".6"/>`;
    } else {
      detail = `<circle cx="32" cy="55" r="5" fill="#fff" opacity=".25"/><path d="M28 43 Q32 47 36 43" fill="none" stroke="${p.trim}" stroke-width="2"/>`;
    }
    const skirtShape = skirt ? `<path d="M19 62 L45 62 L48 74 L16 74 Z" fill="${p.top}"/>` : '';
    return legs + arms + torso + skirtShape + detail;
  }

  function parts(look) {
    const g = look.gender === 'female' ? 'female' : 'male';
    return `<ellipse cx="32" cy="91" rx="15" ry="3.5" fill="rgba(0,0,0,.22)"/>
      ${hairBack(g)}${backLayer(look)}${body(look)}
      <g class="head">${face()}${hairFront(g, look.outfit)}${crown(look)}</g>`;
  }

  // Cả người, viewBox 64×96. opts.walking để bật hiệu ứng bước đi.
  function svg(look = {}, opts = {}) {
    const cls = 'character' + (opts.walking ? ' walking' : '') + (opts.cls ? ' ' + opts.cls : '');
    const size = opts.size ? ` width="${opts.size * (64 / 96)}" height="${opts.size}"` : '';
    return `<svg class="${cls}" viewBox="0 0 64 96"${size} aria-hidden="true">${parts(look)}</svg>`;
  }

  // Phần trong để nhúng vào một SVG lớn hơn (bản đồ).
  function group(look = {}) {
    return parts(look);
  }

  // Cổ áo hiện ở phần vai của avatar.
  function avatarCollar(look) {
    if (look.outfit === 'royal') return `<path d="M16 46 Q32 54 48 46 Q48 41 40 41 L24 41 Q16 41 16 46 Z" fill="#fff"/><circle cx="22" cy="45" r=".9" fill="#222"/><circle cx="42" cy="45" r=".9" fill="#222"/>`;
    if (look.outfit === 'hoodie') return `<path d="M29 44 L29 50 M35 44 L35 50" stroke="#fff" stroke-width="1.3" stroke-linecap="round"/>`;
    return '';
  }

  // Avatar tròn: chỉ lấy phần đầu và vai.
  function head(look = {}, opts = {}) {
    const g = look.gender === 'female' ? 'female' : 'male';
    const p = palette(look);
    const cls = 'avatar char-avatar' + (opts.cls ? ' ' + opts.cls : '');
    return `<span class="${cls}" style="--avatar:${p.top}"><svg viewBox="8 2 48 48" aria-hidden="true">
      ${hairBack(g)}${backLayer(look)}<rect x="15" y="42" width="34" height="20" rx="10" fill="${p.top}"/>${avatarCollar(look)}${face()}${hairFront(g, look.outfit)}${crown(look)}
    </svg></span>`;
  }

  // Nhân vật nhìn từ trên cao xuống, tâm ở (0, 0), mặt hướng lên trên (bắc).
  // Dùng trên bản đồ; xoay cả nhóm theo hướng đi. Chân / tay có class để CSS tạo bước đi.
  function topdown(look = {}) {
    const p = palette(look);
    const female = look.gender === 'female';
    const hair = female ? HAIR.female : HAIR.male;
    const sleeve = look.outfit === 'casual' ? SKIN : p.top;
    let tail = '';
    if (look.outfit === 'aodai') tail = `<path d="M-10 4 Q-8 22 0 24 Q8 22 10 4 Z" fill="${p.top}"/><path d="M-4 8 L4 8" stroke="${p.trim}" stroke-width="1.5"/>`;
    if (look.outfit === 'royal') tail = `<path d="M-14 2 Q-14 25 0 28 Q14 25 14 2 Z" fill="${p.top}" stroke="${GOLD}" stroke-width="1.8"/>`;
    let detail = '';
    if (look.outfit === 'suit') detail = `<path d="M-5 -7 L0 1 L5 -7" fill="#fff"/>` + (female ? '' : `<path d="M0 -4 L0 3" stroke="#d93a3a" stroke-width="2.4" stroke-linecap="round"/>`);
    if (look.outfit === 'sport') detail = `<path d="M-13 -1 L-5 -1 M5 -1 L13 -1" stroke="#fff" stroke-width="2"/>`;
    if (look.outfit === 'aodai') detail = `<path d="M-4 -7 Q0 -3 4 -7" fill="none" stroke="${p.trim}" stroke-width="1.6"/>`;
    if (look.outfit === 'royal') {
      detail = `<ellipse cx="0" cy="1" rx="12" ry="5.5" fill="#fff" stroke="#e0e0e0" stroke-width=".8"/>` +
        [[-8, 1], [-3, 4], [3, 4], [8, 1]].map(([x, y]) => `<circle cx="${x}" cy="${y}" r=".9" fill="#222"/>`).join('');
    }
    if (look.outfit === 'hoodie') detail = `<path d="M-3 -7 L-3 -2 M3 -7 L3 -2" stroke="#fff" stroke-width="1.3" stroke-linecap="round"/>`;
    // Mũ trùm nằm phía sau đầu (hướng nam); vẽ sau tóc đuôi ngựa để đuôi tóc nằm trong mũ.
    const hood = look.outfit === 'hoodie'
      ? `<path d="M-10.5 0 Q-11.5 14 0 15 Q11.5 14 10.5 0 Z" fill="${p.hood}" stroke="rgba(0,0,0,.2)"/><path d="M-7 2 Q0 9 7 2" fill="none" stroke="${p.trim}" stroke-width="1.2" opacity=".7"/>`
      : '';
    // Vương miện nhìn từ trên: vòng vàng có các mũi nhọn và ngọc.
    const tdCrown = look.outfit === 'royal'
      ? (female
        ? `<path d="M-6 -3 A6.5 6.5 0 0 1 6 -3" fill="none" stroke="${GOLD}" stroke-width="2.6" stroke-linecap="round"/><circle cx="0" cy="-6.3" r="1.6" fill="#e91e63"/>`
        : `<circle cx="0" cy="0" r="6.2" fill="none" stroke="${GOLD}" stroke-width="2.8"/>` +
          [0, 72, 144, 216, 288].map((a) => {
            const r = (a * Math.PI) / 180;
            return `<circle cx="${(6.2 * Math.sin(r)).toFixed(2)}" cy="${(-6.2 * Math.cos(r)).toFixed(2)}" r="1.5" fill="${GOLD}" stroke="${GOLD_DARK}" stroke-width=".6"/>`;
          }).join('') + `<circle cx="0" cy="0" r="2" fill="#e53935"/>`)
      : '';
    const ponytail = female ? `<ellipse cx="0" cy="9.5" rx="4.6" ry="7" fill="${hair}"/>` : '';
    const bow = female && look.outfit !== 'royal' ? `<circle cx="7.5" cy="-6" r="2.6" fill="#ff7aa8"/>` : '';
    return `<ellipse cx="3" cy="4" rx="17" ry="15" fill="rgba(0,0,0,.25)"/>
      <g class="td-foot td-foot-l"><ellipse cx="-5.5" cy="0" rx="4" ry="6" fill="${p.shoes}" stroke="rgba(0,0,0,.25)"/></g>
      <g class="td-foot td-foot-r"><ellipse cx="5.5" cy="0" rx="4" ry="6" fill="${p.shoes}" stroke="rgba(0,0,0,.25)"/></g>
      ${tail}
      <g class="td-arm td-arm-l"><circle cx="-14.5" cy="1" r="4.4" fill="${sleeve}" stroke="rgba(0,0,0,.2)"/></g>
      <g class="td-arm td-arm-r"><circle cx="14.5" cy="1" r="4.4" fill="${sleeve}" stroke="rgba(0,0,0,.2)"/></g>
      <ellipse cx="0" cy="1" rx="14" ry="8.5" fill="${p.top}" stroke="rgba(0,0,0,.2)"/>
      ${detail}${ponytail}${hood}
      <circle cx="0" cy="-2" r="9.5" fill="${SKIN}"/>
      <circle cx="0" cy="0" r="9" fill="${hair}"/>
      <path d="M-6 -6.5 Q0 -10.5 6 -6.5" fill="none" stroke="rgba(255,255,255,.25)" stroke-width="1.5"/>
      ${bow}${tdCrown}`;
  }

  window.Character = { svg, group, head, topdown, OUTFITS, GENDERS };
})();
