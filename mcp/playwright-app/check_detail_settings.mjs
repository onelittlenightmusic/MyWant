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
if (await settingsTab.count() > 0) {
  await settingsTab.first().click();
  await page.waitForTimeout(500);
}

// Find sub-tab buttons
const allBtns = await page.locator('button').all();
const subTabs = {};
for (const btn of allBtns) {
  const text = (await btn.textContent() || '').trim().toLowerCase();
  const bb = await btn.boundingBox();
  if (bb && bb.x > 2600 && bb.y > 50 && bb.y < 900 && bb.height < 60) {
    if (['name','labels','schedule','deps'].includes(text)) {
      subTabs[text] = bb;
      console.log(`  "${text}" x=${bb.x.toFixed(0)} y=${bb.y.toFixed(0)}`);
    }
    // params button text includes SVG content, find by position between name and labels
  }
}

// Screenshot current state (NAME tab)
await page.screenshot({ path: '/tmp/settings_name.png', clip: { x: 2600, y: 0, width: 600, height: 1000 } });
console.log('NAME tab screenshot saved');

// Click PARAMS (between Name x and Labels x)
if (subTabs['name'] && subTabs['labels']) {
  const paramsX = (subTabs['name'].x + subTabs['labels'].x) / 2;
  const paramsY = subTabs['name'].y + subTabs['name'].height / 2;
  console.log(`Clicking PARAMS at x=${paramsX.toFixed(0)} y=${paramsY.toFixed(0)}`);
  await page.mouse.click(paramsX, paramsY);
  await page.waitForTimeout(400);
  await page.screenshot({ path: '/tmp/settings_params.png', clip: { x: 2600, y: 0, width: 600, height: 1000 } });
  console.log('PARAMS tab screenshot saved');
}

await browser.close();
