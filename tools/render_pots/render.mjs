// Chạy trang index.html trong Chrome headless, lưu ảnh thô từng chậu vào thư mục ra (mặc định ./out).
// Cần: một static server phục vụ thư mục gốc repo (vd `python3 -m http.server 8305`) và playwright-core (NODE_PATH).
// Dùng: node tools/render_pots/render.mjs [http://localhost:8305] [thư mục ra]
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright-core');
const base = process.argv[2] || 'http://localhost:8305';
const out = process.argv[3] || 'tools/render_pots/out';
fs.mkdirSync(out, { recursive: true });

const browser = await chromium.launch({
  executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  args: ['--use-angle=metal', '--enable-gpu', '--ignore-gpu-blocklist'],
});
const page = await browser.newPage();
page.on('console', (m) => m.type() === 'error' && console.error('console:', m.text()));
page.on('pageerror', (e) => console.error('pageerror:', e.message));
await page.goto(`${base}/tools/render_pots/index.html`);
await page.waitForFunction(() => window.potsReady === true, null, { timeout: 30000 });
// Tham số thứ 3 (tuỳ chọn): danh sách id cách nhau dấu phẩy, vd "pot_skin_vip_gift,pot_skin_vip_pine".
const only = process.argv[4] ? process.argv[4].split(',') : null;
const ids = (await page.evaluate(() => window.POT_IDS)).filter((id) => !only || only.includes(id));
const meta = {};
for (const id of ids) {
  const r = await page.evaluate((id) => window.renderPot(id), id);
  fs.writeFileSync(path.join(out, `${id}.png`), Buffer.from(r.png.split(',')[1], 'base64'));
  meta[id] = { outline: r.outline, glow: r.glow, vip: r.vip };
}
fs.writeFileSync(path.join(out, 'meta.json'), JSON.stringify(meta, null, 1));
await browser.close();
console.log('xong', ids.length, 'chậu →', out);
