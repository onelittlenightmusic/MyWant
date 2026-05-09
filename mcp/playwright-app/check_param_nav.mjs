import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false, slowMo: 300 });
const page = await browser.newPage();
await page.setViewportSize({ width: 3200, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// Click Add Want node on canvas
const addWantNode = page.locator('text=Add Want').first();
await addWantNode.click();
await page.waitForTimeout(1000);

// Find and click a want type in the search
const searchInput = page.locator('input[placeholder*="Search"]').first();
if (await searchInput.isVisible()) {
  await searchInput.fill('cloudflare');
  await page.waitForTimeout(500);
  const firstResult = page.locator('[data-testid="type-option"]').first();
  if (await firstResult.isVisible()) {
    await firstResult.click();
    await page.waitForTimeout(500);
  }
}

// Navigate to PARAMS tab using keyboard (R bumper / right arrow)
await page.keyboard.press('ArrowRight'); // name -> params
await page.waitForTimeout(500);
await page.screenshot({ path: '/tmp/params_tab.png' });
console.log('Navigated to PARAMS tab');

// Press ArrowRight again to move to first card
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(500);
await page.screenshot({ path: '/tmp/first_card.png' });
console.log('Moved to first card or next tab');

// Press Enter to focus input
await page.keyboard.press('Enter');
await page.waitForTimeout(500);
await page.screenshot({ path: '/tmp/card_focused.png' });
console.log('Pressed Enter');

// Check if an input is now focused
const focused = await page.evaluate(() => document.activeElement?.tagName);
console.log('Active element:', focused);

// Press Escape - should blur input (level 1)
await page.keyboard.press('Escape');
await page.waitForTimeout(500);
const focused2 = await page.evaluate(() => document.activeElement?.tagName);
console.log('After first Escape, active element:', focused2);
await page.screenshot({ path: '/tmp/after_escape1.png' });

// Try L/R navigation (should work again)
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(500);
await page.screenshot({ path: '/tmp/after_right.png' });
console.log('Pressed ArrowRight after first Escape');

// Press Escape again - should exit grid (level 2)
await page.keyboard.press('Escape');
await page.waitForTimeout(500);
const focused3 = await page.evaluate(() => document.activeElement?.tagName);
console.log('After second Escape, active element:', focused3);
await page.screenshot({ path: '/tmp/after_escape2.png' });

await browser.close();
