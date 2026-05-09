/**
 * Fresh load → open Add Want sidebar → press arrow keys → verify focus ring
 */
import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

page.on('console', msg => {
  const t = msg.text();
  if (msg.type() === 'error' || t.includes('[CAP]') || t.includes('[DBG]') || t.includes('[GP]')) console.log('[PAGE]', t);
});

// Fresh load
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// Dismiss any overlay
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

// Open Add Want sidebar
await page.keyboard.press('a');
await page.waitForTimeout(100);

// Wait for inventory slots to render
await page.waitForFunction(() => {
  return Array.from(document.querySelectorAll('[data-sidebar="true"][data-sidebar-open] button[draggable]'))
    .some(b => b.getBoundingClientRect().width > 0);
}, { timeout: 6000 });
await page.waitForTimeout(500); // wait for 80ms autofocus + settle

// Check initial focus
const focus0 = await page.evaluate(() => {
  const el = document.activeElement;
  return { tag: el.tagName, placeholder: el.getAttribute('placeholder') };
});
console.log('Focus before arrow:', JSON.stringify(focus0));

// Move mouse far away so hover doesn't interfere
await page.mouse.move(10, 450);
await page.waitForTimeout(100);

// Screenshot before
await page.screenshot({ path: '/tmp/before_arrow.png' });

// Press ArrowRight
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);

// Check DOM activeElement and computed styles
const result = await page.evaluate(() => {
  const el = document.activeElement;
  if (!el) return { error: 'no activeElement' };
  const styles = window.getComputedStyle(el);
  return {
    tag: el.tagName,
    isSlot: el.tagName === 'BUTTON' && el.getAttribute('draggable') === 'true',
    hasFocusRing: el.classList.contains('sidebar-focus-ring'),
    outline: styles.outline,
    outlineColor: styles.outlineColor,
    outlineWidth: styles.outlineWidth,
    outlineStyle: styles.outlineStyle,
    outlineOffset: styles.outlineOffset,
    inSidebar: !!el.closest('[data-sidebar-open]'),
  };
});
console.log('After ArrowRight:', JSON.stringify(result, null, 2));

// Screenshot after
await page.screenshot({ path: '/tmp/after_arrow.png' });
console.log('Screenshots: /tmp/before_arrow.png  /tmp/after_arrow.png');

// Also try pressing ArrowDown and screenshot
await page.keyboard.press('ArrowDown');
await page.waitForTimeout(200);
await page.screenshot({ path: '/tmp/after_arrowdown.png' });

const result2 = await page.evaluate(() => {
  const el = document.activeElement;
  const styles = window.getComputedStyle(el);
  return {
    tag: el.tagName,
    isSlot: el.tagName === 'BUTTON' && el.getAttribute('draggable') === 'true',
    outline: styles.outline,
    outlineColor: styles.outlineColor,
    outlineWidth: styles.outlineWidth,
  };
});
console.log('After ArrowDown:', JSON.stringify(result2, null, 2));

await page.waitForTimeout(2000);
await browser.close();
// already ran above — add zoomed clip
