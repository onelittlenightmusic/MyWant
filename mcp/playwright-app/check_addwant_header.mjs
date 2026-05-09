import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 3200, height: 1000 });

await page.goto('http://localhost:8080');
await page.waitForLoadState('domcontentloaded');
await page.waitForTimeout(2000);

// Open Add Want, select type by coordinates
await page.locator('text=Add Want').first().click();
await page.waitForTimeout(500);

const searchInput = page.locator('input[placeholder="Search..."]').last();
await searchInput.fill('rpg_observe');
await page.waitForTimeout(800);
await page.mouse.click(2771, 114);
await page.waitForTimeout(1200);

// Screenshot after type selection
await page.screenshot({ path: '/tmp/check_filter.png', clip: { x: 2600, y: 840, width: 600, height: 160 } });

// Verify Filter is gone
const filterBtn = page.locator('button[title="Toggle search/filter"]');
const filterSpan = page.locator('span').filter({ hasText: /^Filter$/i });
console.log('Filter button count:', await filterBtn.count());
console.log('Filter span count:', await filterSpan.count());

// Check what buttons ARE in the header
const allBtns = await page.locator('button').all();
console.log('\nHeader buttons (x>2600, y>840):');
for (const btn of allBtns) {
  const text = await btn.textContent();
  const bb = await btn.boundingBox();
  if (bb && bb.x > 2600 && bb.y > 840) {
    console.log(`  "${text?.trim().substring(0,30)}" x=${bb.x.toFixed(0)} y=${bb.y.toFixed(0)}`);
  }
}

await browser.close();
