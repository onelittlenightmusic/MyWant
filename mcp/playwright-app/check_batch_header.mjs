import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

await page.click('[data-header-btn-id="select"]');
await page.waitForTimeout(300);
await page.keyboard.press('ArrowRight'); await page.waitForTimeout(150);
await page.keyboard.press('Enter');      await page.waitForTimeout(150);
await page.keyboard.press('ArrowRight'); await page.waitForTimeout(150);
await page.keyboard.press('Enter');      await page.waitForTimeout(150);
await page.mouse.move(10, 450);

// Header is at y=825, h=75 — clip it
const clip = { x: 0, y: 820, width: 800, height: 80 };

await page.screenshot({ path: '/tmp/hdr_select_mode.png', clip });

await page.keyboard.press('Shift+Space');
await page.waitForTimeout(300);
await page.screenshot({ path: '/tmp/hdr_batch_start.png', clip });

await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);
await page.screenshot({ path: '/tmp/hdr_batch_stop.png', clip });

await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);
await page.screenshot({ path: '/tmp/hdr_batch_delete.png', clip });

await page.keyboard.press('Escape');
await page.waitForTimeout(500);
await page.screenshot({ path: '/tmp/hdr_back_select.png', clip });

await browser.close();
