import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false, slowMo: 400 });
const page = await browser.newPage();
await page.setViewportSize({ width: 3200, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// Click Add Want node on canvas
await page.locator('text=Add Want').first().click();
await page.waitForTimeout(800);

// Search for cloudflare type
const typeSearch = page.locator('input').filter({ hasText: '' }).last();
// Use the search input in the Add Want sidebar
const inputs = page.locator('input');
const count = await inputs.count();
console.log('Input count:', count);

// Fill in the search box (top right sidebar)
const searchBox = page.locator('input[placeholder*="cloudflare"], input').nth(count - 1);
await page.locator('input').last().fill('cloudflare');
await page.waitForTimeout(500);

// Click the Cloudflare Tunnel type card
const cloudflareCard = page.locator('text=Cloudflare Tu').first();
if (await cloudflareCard.isVisible()) {
  await cloudflareCard.click();
  console.log('Clicked Cloudflare Tunnel type');
  await page.waitForTimeout(800);
} else {
  console.log('Cloudflare card not found');
}
await page.screenshot({ path: '/tmp/e1_type_selected.png' });

// Click PARAMS tab
const paramsTab = page.locator('button').filter({ hasText: /^PARAMS$|^Params$/i }).last();
if (await paramsTab.isVisible()) {
  await paramsTab.click();
  console.log('Clicked PARAMS tab');
  await page.waitForTimeout(500);
} else {
  console.log('PARAMS tab not found');
  // Try finding by text content
  const allButtons = page.locator('button');
  const btnCount = await allButtons.count();
  for (let i = 0; i < btnCount; i++) {
    const text = await allButtons.nth(i).textContent();
    if (text && text.toLowerCase().includes('param')) {
      console.log('Found param button at index', i, ':', text);
      await allButtons.nth(i).click();
      break;
    }
  }
  await page.waitForTimeout(500);
}
await page.screenshot({ path: '/tmp/e2_params_tab.png' });

// Look for parameter cards - they should be in the Add Want sidebar
// Find an input inside the sidebar
const sidebarInputs = page.locator('aside input, [class*="sidebar"] input, .right-sidebar input');
const sidebarInputCount = await sidebarInputs.count();
console.log('Sidebar inputs count:', sidebarInputCount);

if (sidebarInputCount > 0) {
  const firstInput = sidebarInputs.first();
  await firstInput.scrollIntoViewIfNeeded();
  await firstInput.click();
  await page.waitForTimeout(300);
  
  const focused1 = await page.evaluate(() => document.activeElement?.tagName);
  console.log('After clicking input:', focused1);
  await page.screenshot({ path: '/tmp/e3_input_focused.png' });
  
  // Press Escape - Level 1: should blur input but stay on card
  await page.keyboard.press('Escape');
  await page.waitForTimeout(300);
  const focused2 = await page.evaluate(() => document.activeElement?.tagName);
  console.log('After Escape (Level 1):', focused2, '← should NOT be INPUT');
  await page.screenshot({ path: '/tmp/e4_after_escape1.png' });
  
  // Try ArrowRight - should navigate to next card
  await page.keyboard.press('ArrowRight');
  await page.waitForTimeout(400);
  await page.screenshot({ path: '/tmp/e5_after_right.png' });
  console.log('Pressed ArrowRight - should move to next card');
  
  // Press Escape again - Level 2: should exit grid
  await page.keyboard.press('Escape');
  await page.waitForTimeout(300);
  await page.screenshot({ path: '/tmp/e6_after_escape2.png' });
  console.log('Pressed Escape (Level 2) - should exit grid');
} else {
  console.log('No sidebar inputs found - checking what is visible');
  const allText = await page.locator('body').textContent();
  console.log('Looking for param cards in DOM...');
  const paramCards = page.locator('[class*="rounded"][class*="border"]');
  const pcCount = await paramCards.count();
  console.log('Rounded border elements:', pcCount);
}

await browser.close();
console.log('Done');
