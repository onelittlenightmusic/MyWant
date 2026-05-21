import { chromium } from 'playwright';

const BASE_URL = 'http://localhost:8081';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

console.log('Navigating...');
await page.goto(BASE_URL);
await page.waitForTimeout(2000);

// canvas モードに切り替える
const canvasBtn = await page.$('[data-header-btn-id="list"]');
if (!canvasBtn) {
  console.log('❌ canvas toggle button not found');
  await page.screenshot({ path: '/tmp/rpg_full_debug.png' });
  await browser.close();
  process.exit(1);
}

console.log('Switching to Canvas mode...');
await canvasBtn.click();
await page.waitForTimeout(300);

// ① 切り替え直後（API完了前）
await page.screenshot({ path: '/tmp/rpg_canvas_initial.png' });
console.log('initial.png saved');

// ② 2秒後（fetchWantTypes完了後）
await page.waitForTimeout(2000);
await page.screenshot({ path: '/tmp/rpg_canvas_loaded.png' });
console.log('loaded.png saved');

await browser.close();
