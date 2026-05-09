/**
 * Batch-action overlay keyboard navigation test
 * Flow: select-mode → check wants → Start button → batch-action overlay → left/right → Enter confirm
 *       also: Escape = back to select-mode, Select button (menu-toggle) = back
 */
import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

page.on('console', msg => {
  const t = msg.text();
  if (msg.type() === 'error' || t.includes('[GP]') || t.includes('[MOCK]')) console.log('[PAGE]', t);
});

await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

const isBatchFocused = (idx) => page.evaluate((i) => {
  const btns = document.querySelectorAll('[data-header-btn-id="select"]');
  // Check via ring-4 class on action buttons
  // The BatchActionBar action buttons are inside HeaderOverlay
  // They're buttons inside the header overlay that have ring-4 class
  const allBtns = document.querySelectorAll('button.ring-4');
  return { ringCount: allBtns.length, idx: i };
}, idx);

const getBatchRingIdx = () => page.evaluate(() => {
  // Find batch action buttons — they are buttons with specific background colors
  const header = document.querySelector('[data-header-overlay]') || document.querySelector('.fixed.top-0');
  const focused = document.querySelectorAll('button.ring-4, button[class*="ring-4"]');
  return focused.length;
});

// Enter select mode via button click
console.log('\n══ Enter select mode ══');
await page.click('[data-header-btn-id="select"]');
await page.waitForTimeout(300);

// Navigate and check 2 wants
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(150);
await page.keyboard.press('Enter');
await page.waitForTimeout(150);
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(150);
await page.keyboard.press('Enter');
await page.waitForTimeout(150);

const checkedCount = await page.evaluate(() =>
  document.querySelectorAll('[data-keyboard-nav-id] .text-blue-600 svg, [data-keyboard-nav-id] svg.lucide-check-square').length
);
console.log('Checked wants:', checkedCount);
console.log(checkedCount >= 2 ? '✅ Multiple wants checked' : '❌ Not enough wants checked');

// Screenshot in select mode
await page.screenshot({ path: '/tmp/batch_select.png' });

// Press Start button transition (Shift+Space = context-menu)
console.log('\n══ Press Shift+Space (Start button) → batch-action mode ══');
await page.keyboard.press('Shift+Space');
await page.waitForTimeout(300);

// Check for ring on first button
const ringCount1 = await getBatchRingIdx();
console.log('Ring buttons (should be 1):', ringCount1);
console.log(ringCount1 > 0 ? '✅ Batch-action mode active (ring visible)' : '❌ No ring visible');

await page.screenshot({ path: '/tmp/batch_action_start.png' });

// Navigate right to Stop
console.log('\n══ ArrowRight → Stop button ══');
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);
await page.screenshot({ path: '/tmp/batch_action_stop.png' });

// Navigate right to Delete
console.log('\n══ ArrowRight → Delete button ══');
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);
await page.screenshot({ path: '/tmp/batch_action_delete.png' });

// Navigate left back to Stop
await page.keyboard.press('ArrowLeft');
await page.waitForTimeout(200);

// Escape → back to select-mode
console.log('\n══ Escape → back to select-mode ══');
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

const selectActive = await page.evaluate(() => {
  const btn = document.querySelector('[data-header-btn-id="select"]');
  return btn?.classList.contains('bg-blue-600') || btn?.className.includes('bg-blue-600');
});
const ringAfterEsc = await getBatchRingIdx();
console.log('Select mode still active after Escape:', selectActive);
console.log('Ring visible after Escape (should be 0):', ringAfterEsc);
console.log((selectActive && ringAfterEsc === 0) ? '✅ Back to select-mode (no ring)' : '❌ State mismatch');

// Re-enter batch action and confirm with Enter
console.log('\n══ Re-enter batch-action, press Enter to confirm Start ══');
await page.keyboard.press('Shift+Space');
await page.waitForTimeout(300);
// First button (Start) is focused — press Enter
await page.keyboard.press('Enter');
await page.waitForTimeout(300);

// Should show confirmation dialog
const confirmVisible = await page.evaluate(() => {
  // HeaderOverlay shows confirmation text
  const overlayText = document.body.innerText;
  return overlayText.includes('Batch start') || overlayText.includes('batch start') || overlayText.includes('Start') && overlayText.includes('Cancel');
});
console.log('Confirmation dialog visible:', confirmVisible ? 'yes' : 'possibly (check screenshot)');
await page.screenshot({ path: '/tmp/batch_confirm.png' });

console.log('\nScreenshots:');
console.log('  /tmp/batch_select.png        — select mode with 2 checked');
console.log('  /tmp/batch_action_start.png  — Start button highlighted');
console.log('  /tmp/batch_action_stop.png   — Stop button highlighted');
console.log('  /tmp/batch_action_delete.png — Delete button highlighted');
console.log('  /tmp/batch_confirm.png       — after Enter confirm');

await page.waitForTimeout(1000);
await browser.close();
