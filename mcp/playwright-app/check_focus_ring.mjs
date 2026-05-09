import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

// Open Add Want sidebar
await page.keyboard.press('a');
await page.waitForTimeout(500);

// Press arrow right to navigate to first slot
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(300);

// Check if focused button has sidebar-focus-ring class
const result = await page.evaluate(() => {
  const el = document.activeElement;
  return {
    tag: el.tagName,
    hasSidebarFocusRing: el.classList.contains('sidebar-focus-ring'),
    classes: el.className,
    isSlot: el.tagName === 'BUTTON' && el.getAttribute('draggable') === 'true',
  };
});
console.log('Focus state:', JSON.stringify(result, null, 2));

// Take screenshot
await page.screenshot({ path: '/tmp/focus_ring_check.png' });
console.log('Screenshot saved to /tmp/focus_ring_check.png');

await page.waitForTimeout(1000);
await browser.close();
