import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 3200, height: 1000 });

await page.goto('http://localhost:8080');
await page.waitForLoadState('domcontentloaded');
await page.waitForTimeout(2000);

await page.locator('text=evidence').last().click();
await page.waitForTimeout(1500);

await page.locator('button').filter({ hasText: /^settings$/i }).first().click();
await page.waitForTimeout(500);

// Click PARAMS sub-tab (between name x=2720 and labels x=2912, y=701+23=724)
await page.mouse.click(2816, 724);
await page.waitForTimeout(800);

await page.screenshot({ path: '/tmp/settings_params_grid.png', clip: { x: 2600, y: 0, width: 600, height: 1000 } });
console.log('PARAMS grid screenshot saved');

await browser.close();
