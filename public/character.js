// Vẽ nhân vật chibi bằng SVG theo giới tính và trang phục (id khớp với internal/wallet/character.go).
// Character.svg(look) cho cả người, Character.head(look) cho avatar tròn.
(function () {
  'use strict';

  const OUTFITS = {
    casual: { name: 'Thường ngày', top: '#4f8ef7', trim: '#3b6fd1', pants: '#2f3e56', shoes: '#f4f4f4' },
    suit: { name: 'Vest lịch lãm', top: '#263445', trim: '#1a2531', pants: '#263445', shoes: '#17110d' },
    sport: { name: 'Thể thao', top: '#e5484d', trim: '#ffffff', pants: '#e5484d', shoes: '#ffffff' },
    aodai: { name: 'Áo dài', top: '#e85d9b', trim: '#f6c744', pants: '#fbfbfb', shoes: '#c98b2c' },
  };
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

  function hairFront(g) {
    return g === 'female'
      ? `<path d="M15 26 Q15 9 32 8 Q49 9 49 26 Q43 15 35 17 Q27 13 15 26 Z" fill="${HAIR.female}"/>
         <circle cx="47" cy="15" r="3.4" fill="#ff7aa8"/>`
      : `<path d="M15 26 Q13 8 32 7 Q51 8 49 26 Q47 18 41 16 Q37 20 31 16 Q25 20 20 17 Q16 20 15 26 Z" fill="${HAIR.male}"/>`;
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
    const skirt = female && look.outfit === 'suit';
    const legColor = skirt ? SKIN : p.pants;

    const legs = `
      <g class="leg leg-l"><rect x="24" y="64" width="7" height="22" rx="3" fill="${legColor}"/><rect x="22.5" y="84" width="9.5" height="5" rx="2.5" fill="${p.shoes}"/></g>
      <g class="leg leg-r"><rect x="33" y="64" width="7" height="22" rx="3" fill="${legColor}"/><rect x="32" y="84" width="9.5" height="5" rx="2.5" fill="${p.shoes}"/></g>`;

    const short = look.outfit === 'casual';
    const sleeve = short
      ? (x) => `<rect x="${x}" y="46" width="7" height="8" rx="3" fill="${p.top}"/><rect x="${x + 0.5}" y="52" width="6" height="12" rx="3" fill="${SKIN}"/>`
      : (x) => `<rect x="${x}" y="46" width="7" height="18" rx="3.5" fill="${p.top}"/>`;
    const arms = `
      <g class="arm arm-l">${sleeve(11)}<circle cx="14.5" cy="65" r="3.2" fill="${SKIN}"/></g>
      <g class="arm arm-r">${sleeve(46)}<circle cx="49.5" cy="65" r="3.2" fill="${SKIN}"/></g>`;

    let torso = `<rect x="17" y="43" width="30" height="25" rx="9" fill="${p.top}"/>`;
    let detail = '';
    if (longTunic) {
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
      ${hairBack(g)}${body(look)}
      <g class="head">${face()}${hairFront(g)}</g>`;
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

  // Avatar tròn: chỉ lấy phần đầu và vai.
  function head(look = {}, opts = {}) {
    const g = look.gender === 'female' ? 'female' : 'male';
    const p = palette(look);
    const cls = 'avatar char-avatar' + (opts.cls ? ' ' + opts.cls : '');
    return `<span class="${cls}" style="--avatar:${p.top}"><svg viewBox="8 2 48 48" aria-hidden="true">
      ${hairBack(g)}<rect x="15" y="42" width="34" height="20" rx="10" fill="${p.top}"/>${face()}${hairFront(g)}
    </svg></span>`;
  }

  window.Character = { svg, group, head, OUTFITS, GENDERS };
})();
