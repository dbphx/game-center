// Chạy tools/render_quests/index.html trong Chrome, lưu ảnh thô từng món vào thư mục ra (kèm meta.json: viền).
// Cần: python3 tools/render_quests/serve.py 8307 và playwright-core (NODE_PATH).
// Dùng: node tools/render_quests/render.mjs [http://127.0.0.1:8307] [thư mục ra] [tiền tố id]
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';

const require = createRequire(import.meta.url);
const { chromium } = require('playwright-core');
const base = process.argv[2] || 'http://127.0.0.1:8307';
const out = process.argv[3] || 'tools/render_quests/out';
fs.mkdirSync(out, { recursive: true });
const browser = await chromium.launch({
  executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  args: ['--use-angle=metal', '--enable-gpu', '--ignore-gpu-blocklist'],
});
const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });
page.on('console', (m) => m.type() === 'error' && console.error('console:', m.text()));
page.on('pageerror', (e) => console.error('pageerror:', e.message));
await page.goto(`${base}/tools/render_quests/index.html`);
await page.waitForFunction(() => window.questsReady === true, null, { timeout: 30000 });
// Tham số thứ 4 (tuỳ chọn): chỉ dựng món có id bắt đầu bằng tiền tố này, vd part_
const only = process.argv[4] || '';
const ids = (await page.evaluate(() => window.ITEM_IDS)).filter((id) => id.startsWith(only));
const meta = {};
for (const id of ids) {
  const r = await page.evaluate((id) => window.renderItem(id), id);
  fs.writeFileSync(path.join(out, `${id}.png`), Buffer.from(r.png.split(',')[1], 'base64'));
  meta[id] = { outline: r.outline, car: !!r.car };
}
fs.writeFileSync(path.join(out, 'meta.json'), JSON.stringify(meta, null, 1));
await browser.close();
console.log('xong', ids.length, 'món →', out);
