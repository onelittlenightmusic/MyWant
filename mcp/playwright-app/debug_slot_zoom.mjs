import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);
await page.keyboard.press('a');
await page.waitForFunction(() =>
  Array.from(document.querySelectorAll('[data-sidebar="true"][data-sidebar-open] button[draggable]'))
    .some(b => b.getBoundingClientRect().width > 0), { timeout: 6000 });
await page.waitForTimeout(500);
await page.mouse.move(10, 450);
await page.waitForTimeout(100);
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);

const rect = await page.evaluate(() => {
  const el = document.activeElement;
  if (!el) return null;
  const r = el.getBoundingClientRect();
  const styles = window.getComputedStyle(el);
  return {
    x: r.x, y: r.y, width: r.width, height: r.height,
    outline: styles.outline,
    outlineOffset: styles.outlineOffset,
    boxShadow: styles.boxShadow,
    overflow: styles.overflow,
    parentOverflow: window.getComputedStyle(el.parentElement).overflow,
  };
});
console.log('Focused slot:', JSON.stringify(rect, null, 2));

if (rect) {
  const pad = 30;
  await page.screenshot({
    path: '/tmp/slot_zoom.png',
    clip: { x: Math.max(0, rect.x - pad), y: Math.max(0, rect.y - pad), width: rect.width + pad*2, height: rect.height + pad*2 }
  });
  await page.screenshot({ path: '/tmp/sidebar_crop.png', clip: { x: 450, y: 0, width: 250, height: 420 } });
}
await browser.close();
