import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 3200, height: 1000 });

await page.goto('http://localhost:8080');
await page.waitForLoadState('domcontentloaded');
await page.waitForTimeout(2000);

// Click the Add Want canvas card to open the form
await page.locator('text=Add Want').first().click();
await page.waitForTimeout(1000);

// Search and select Evidence type
const searchInput = page.locator('input[placeholder="Search..."]').last();
await searchInput.fill('evidence');
await page.waitForTimeout(800);

const evidenceType = page.locator('span').filter({ hasText: /^Evidence$/ }).first();
const bb = await evidenceType.boundingBox();
if (bb) {
  await page.mouse.click(bb.x + bb.width/2, bb.y - 30);
  await page.waitForTimeout(1200);
}

// Click LABELS tab
const labelsTab = page.locator('button').filter({ hasText: /^labels$/i });
const labelsCount = await labelsTab.count();
console.log('Labels tabs found:', labelsCount);
if (labelsCount > 0) {
  await labelsTab.first().click();
  await page.waitForTimeout(500);
}

await page.screenshot({ path: '/tmp/labels_tab.png', clip: { x: 2600, y: 0, width: 600, height: 1000 } });
console.log('Labels tab screenshot saved');

// Click SCHEDULE tab  
const scheduleTab = page.locator('button').filter({ hasText: /^schedule$/i });
if (await scheduleTab.count() > 0) {
  await scheduleTab.first().click();
  await page.waitForTimeout(500);
}
await page.screenshot({ path: '/tmp/schedule_tab.png', clip: { x: 2600, y: 0, width: 600, height: 1000 } });
console.log('Schedule tab screenshot saved');

// Click DEPS tab
const depsTab = page.locator('button').filter({ hasText: /^deps$/i });
if (await depsTab.count() > 0) {
  await depsTab.first().click();
  await page.waitForTimeout(500);
}
await page.screenshot({ path: '/tmp/deps_tab.png', clip: { x: 2600, y: 0, width: 600, height: 1000 } });
console.log('Deps tab screenshot saved');

// Check Example button click -> dropdown
const exampleBtn = page.locator('button').filter({ hasText: /example/i }).first();
if (await exampleBtn.count() > 0) {
  await exampleBtn.click();
  await page.waitForTimeout(300);
  await page.screenshot({ path: '/tmp/example_dropdown.png', clip: { x: 2600, y: 700, width: 600, height: 300 } });
  console.log('Example dropdown screenshot saved');
}

await browser.close();
