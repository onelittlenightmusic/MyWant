/**
 * Debug: explicitly focus search INPUT then test arrow key routing
 */
import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

page.on('console', msg => {
  const t = msg.text();
  if (msg.type() === 'error' || t.includes('[CAP]') || t.includes('[DBG]')) console.log('[PAGE]', t);
});

await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

// Open form
await page.keyboard.press('a');
await page.waitForTimeout(300);

// Wait for inventory buttons to appear
await page.waitForFunction(() => {
  const sb = Array.from(document.querySelectorAll('[data-sidebar="true"]'))
    .find(s => s.hasAttribute('data-sidebar-open') && s.querySelector('#want-form'));
  if (!sb) return false;
  return Array.from(sb.querySelectorAll('button[draggable]')).filter(b => {
    const r = b.getBoundingClientRect(); return r.width > 0 && r.height > 0;
  }).length > 0;
}, { timeout: 8000 });

// Wait for autofocus (80ms timer in WantInventoryPicker)
await page.waitForTimeout(200);

const focus1 = await page.evaluate(() => {
  const el = document.activeElement;
  return { tag: el.tagName, placeholder: el.getAttribute('placeholder') };
});
console.log('Focus after autofocus timer:', JSON.stringify(focus1));

// Force focus onto the search input if it isn't already
if (focus1.tag !== 'INPUT') {
  await page.evaluate(() => {
    const inp = document.querySelector('#want-form input[placeholder="Search..."]') ||
                document.querySelector('[data-sidebar-open] input[type="text"]');
    if (inp) inp.focus();
  });
  await page.waitForTimeout(50);
}

const focus2 = await page.evaluate(() => ({
  tag: document.activeElement.tagName,
  placeholder: document.activeElement.getAttribute('placeholder'),
}));
console.log('Focus after forcing INPUT:', JSON.stringify(focus2));

// Read formSituation and isTypeSelectionPhase from React internals via data attr or console
const formState = await page.evaluate(() => {
  // Check if sidebar is marked as open
  const sb = Array.from(document.querySelectorAll('[data-sidebar="true"]'))
    .find(s => s.hasAttribute('data-sidebar-open'));
  const slotCount = sb ? Array.from(sb.querySelectorAll('button[draggable]')).length : 0;
  const searchFocused = document.activeElement?.tagName === 'INPUT';
  return { sidebarOpen: !!sb, slotCount, searchFocused };
});
console.log('Form state:', JSON.stringify(formState));

// Now press ArrowDown and observe
await page.keyboard.press('ArrowDown');
await page.waitForTimeout(300);

const focus3 = await page.evaluate(() => {
  const el = document.activeElement;
  return {
    tag: el.tagName,
    placeholder: el.getAttribute('placeholder'),
    inSlot: !!el.closest('[data-sidebar="true"]') && el.tagName === 'BUTTON' && el.getAttribute('draggable') === 'true',
    inSidebar: !!el.closest('[data-sidebar="true"][data-sidebar-open="true"]'),
  };
});
console.log('Focus after ArrowDown from INPUT:', JSON.stringify(focus3));
console.log(focus3.inSlot ? '✅ PASS: slot focused from INPUT' : '❌ FAIL: slot NOT focused from INPUT');

// Also test ArrowRight
await page.evaluate(() => {
  const inp = document.querySelector('#want-form input[placeholder="Search..."]') ||
              document.querySelector('[data-sidebar-open] input[type="text"]');
  if (inp) inp.focus();
});
await page.waitForTimeout(50);
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(300);

const focus4 = await page.evaluate(() => {
  const el = document.activeElement;
  return {
    tag: el.tagName,
    inSlot: !!el.closest('[data-sidebar="true"]') && el.tagName === 'BUTTON' && el.getAttribute('draggable') === 'true',
  };
});
console.log('Focus after ArrowRight from INPUT:', JSON.stringify(focus4));
console.log(focus4.inSlot ? '✅ PASS: slot focused from INPUT' : '❌ FAIL: slot NOT focused from INPUT');

await page.waitForTimeout(1000);
await browser.close();
process.exit((focus3.inSlot && focus4.inSlot) ? 0 : 1);
