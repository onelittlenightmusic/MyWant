import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();

// Use 1400px viewport - confirmed working from previous tests
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(3000);

// Step 1: Click the "Add Want" button in the grid
console.log('Step 1: Clicking "Add Want" button...');
await page.click('button:has-text("Add Want")');
await page.waitForTimeout(2000);

// Step 2: Click the first want type card in the inventory picker
// From 1400px test: cards are at x=936, y=79 with 70x70 size - center = 971, 114
console.log('Step 2: Clicking first want type card (Description/Approval)...');
await page.mouse.click(971, 114);
await page.waitForTimeout(2000);

// Verify tabs are present
const tabs = await page.evaluate(() => {
  const tabButtons = Array.from(document.querySelectorAll('button')).filter(b => {
    const r = b.getBoundingClientRect();
    const text = b.textContent?.trim();
    return r.height > 0 && r.height < 40 && text &&
      ['Name', 'Params', 'Labels', 'Schedule', 'Deps'].includes(text);
  });
  return tabButtons.map(b => ({
    text: b.textContent?.trim(),
    cls: (typeof b.className === 'string' ? b.className.slice(0, 100) : ''),
    rect: { x: Math.round(b.getBoundingClientRect().x), y: Math.round(b.getBoundingClientRect().y), w: Math.round(b.getBoundingClientRect().width), h: Math.round(b.getBoundingClientRect().height) },
  }));
});
console.log('Tabs found:', JSON.stringify(tabs, null, 2));

// The tabs are at y=70 based on previous exploration
// Take clipped screenshot of just the sidebar from x=900
await page.screenshot({
  path: '/tmp/add_want_tabs.png',
  clip: { x: 900, y: 0, width: 500, height: 900 },
});
console.log('Tab screenshot saved: /tmp/add_want_tabs.png');

// Now click Params tab - it was at x=1019-1112, y=70
console.log('\nStep 3: Clicking Params tab by coordinate...');
await page.mouse.click(1065, 82); // center of Params tab
await page.waitForTimeout(800);

// Verify active tab
const activeTab = await page.evaluate(() => {
  const tabButtons = Array.from(document.querySelectorAll('button')).filter(b => {
    const r = b.getBoundingClientRect();
    const text = b.textContent?.trim();
    return r.height > 0 && r.height < 40 && text &&
      ['Name', 'Params', 'Labels', 'Schedule', 'Deps'].includes(text);
  });
  return tabButtons.map(b => ({
    text: b.textContent?.trim(),
    cls: (typeof b.className === 'string' ? b.className.slice(0, 120) : ''),
  }));
});
console.log('Active tab state:', JSON.stringify(activeTab, null, 2));

await page.screenshot({
  path: '/tmp/add_want_params_grid.png',
  clip: { x: 900, y: 0, width: 500, height: 900 },
});
console.log('Params grid screenshot saved: /tmp/add_want_params_grid.png');

// Also take a full-page screenshot for reference
await page.screenshot({ path: '/tmp/add_want_full.png', fullPage: false });
console.log('Full page screenshot saved: /tmp/add_want_full.png');

await browser.close();
console.log('\nAll done.');
