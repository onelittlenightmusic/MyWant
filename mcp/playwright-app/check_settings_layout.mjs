import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 3200, height: 1000 });

await page.goto('http://localhost:8080');
await page.waitForLoadState('domcontentloaded');
await page.waitForTimeout(2000);

await page.locator('text=evidence').last().click();
await page.waitForTimeout(1500);

const settingsTab = page.locator('button').filter({ hasText: /^settings$/i });
await settingsTab.first().click();
await page.waitForTimeout(500);

// Find the tab bar buttons to measure their position
const allBtns = await page.locator('button').all();
for (const btn of allBtns) {
  const text = (await btn.textContent() || '').trim().toLowerCase();
  const bb = await btn.boundingBox();
  if (bb && bb.x > 2600 && ['name','labels','schedule','deps','settings','results'].includes(text)) {
    console.log(`"${text}" x=${bb.x.toFixed(0)} y=${bb.y.toFixed(0)} h=${bb.height.toFixed(0)}`);
  }
}

// Full sidebar screenshot
await page.screenshot({ path: '/tmp/settings_full.png', clip: { x: 2600, y: 0, width: 600, height: 1000 } });
console.log('Full sidebar screenshot saved');

await browser.close();
