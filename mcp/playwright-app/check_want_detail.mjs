import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 2400, height: 900 });

await page.goto('http://localhost:8080');
await page.waitForLoadState('domcontentloaded');
await page.waitForTimeout(2000);

// From prior run: sidebar at x~1880, Settings btn at x=2040
// Click evidence card
await page.locator('text=evidence').first().click();
await page.waitForTimeout(1500);

// Screenshot results tab sidebar
await page.screenshot({ path: '/tmp/detail_results_sidebar.png', clip: { x: 1880, y: 0, width: 520, height: 900 } });
console.log('Results tab screenshot saved');

// Click SETTINGS by force at known position (~x=2040, y=649)
await page.mouse.click(2040, 649, { force: true });
await page.waitForTimeout(700);
await page.screenshot({ path: '/tmp/detail_settings_sidebar.png', clip: { x: 1880, y: 0, width: 520, height: 900 } });
console.log('Settings tab screenshot saved');

await browser.close();
