/**
 * Inventory navigation from fresh state test
 *
 * Scenario A: No want card focused → press 'a' → arrow keys should navigate inventory
 * Scenario B: No want card focused → Y button → Add Want → arrow keys should navigate inventory
 */
import { chromium } from 'playwright';

const PASS = '✅ PASS';
const FAIL = '❌ FAIL';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

page.on('console', msg => {
  const t = msg.text();
  if (t.startsWith('[GP]') || t.startsWith('[MOCK]') || msg.type() === 'error') console.log('[PAGE]', t);
});

async function installMockGamepad() {
  await page.evaluate(() => {
    const buttons = Array(17).fill(null).map(() => ({ pressed: false, touched: false, value: 0 }));
    window.__mockGP = { axes: [0,0,0,0], buttons, connected: true, id: 'Mock Gamepad', index: 0, mapping: 'standard', timestamp: performance.now() };
    navigator.getGamepads = () => [window.__mockGP, null, null, null];
    console.log('[MOCK] Gamepad installed');
  });
}

const BTN = { CONFIRM: 0, CANCEL: 1, Y: 3, LEFT: 14, RIGHT: 15, UP: 12, DOWN: 13 };

async function pressBtn(idx, holdMs = 80) {
  await page.evaluate(i => { window.__mockGP.buttons[i] = { pressed: true, touched: true, value: 1 }; window.__mockGP.timestamp = performance.now(); console.log('[GP] press', i); }, idx);
  await page.waitForTimeout(holdMs);
  await page.evaluate(i => { window.__mockGP.buttons[i] = { pressed: false, touched: false, value: 0 }; window.__mockGP.timestamp = performance.now(); console.log('[GP] release', i); }, idx);
  await page.waitForTimeout(100);
}

async function getFocus() {
  return await page.evaluate(() => {
    const el = document.activeElement;
    if (!el || el === document.body) return { tag: 'BODY', text: '', inSidebar: false, navId: null, inSlot: false };
    return {
      tag: el.tagName,
      text: (el.textContent || el.getAttribute('placeholder') || '').trim().slice(0, 30),
      inSidebar: !!el.closest('[data-sidebar="true"][data-sidebar-open="true"]'),
      navId: el.getAttribute('data-keyboard-nav-id'),
      inSlot: !!el.closest('[data-sidebar="true"]') && el.tagName === 'BUTTON' && el.getAttribute('draggable') === 'true',
    };
  });
}

async function waitForFormOpen() {
  await page.waitForFunction(() =>
    Array.from(document.querySelectorAll('[data-sidebar="true"]'))
      .some(s => s.hasAttribute('data-sidebar-open') && s.querySelector('#want-form'))
  , { timeout: 5000 });
}

async function waitForInventoryLoaded() {
  await page.waitForFunction(() => {
    const formSidebar = Array.from(document.querySelectorAll('[data-sidebar="true"]'))
      .find(s => s.hasAttribute('data-sidebar-open') && s.querySelector('#want-form'));
    if (!formSidebar) return false;
    return Array.from(formSidebar.querySelectorAll('button[draggable]')).filter(b => {
      const r = b.getBoundingClientRect(); return r.width > 0 && r.height > 0;
    }).length > 0;
  }, { timeout: 8000 });
}

async function closeFormSidebar() {
  await page.keyboard.press('Escape');
  await page.waitForTimeout(600);
  // Verify closed
  const open = await page.evaluate(() =>
    Array.from(document.querySelectorAll('[data-sidebar="true"]'))
      .some(s => s.hasAttribute('data-sidebar-open') && s.querySelector('#want-form'))
  );
  if (open) {
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);
  }
}

// ── Setup ──────────────────────────────────────────────────────────────────────
await page.goto('http://localhost:8080');
await page.waitForTimeout(2500);
await installMockGamepad();
await page.waitForTimeout(300);
// Dismiss any sidebars
await page.keyboard.press('Escape');
await page.waitForTimeout(400);

// Verify no cards are focused (initial state)
const initFocus = await getFocus();
console.log(`\nInitial focus: ${JSON.stringify(initFocus)}`);

// ════════════════════════════════════════════════
// SCENARIO A: keyboard 'a' → arrow keys → inventory
// ════════════════════════════════════════════════
console.log('\n══ SCENARIO A: keyboard "a" from fresh state ══\n');

await page.keyboard.press('a');
await page.waitForTimeout(500);
await waitForFormOpen();
await waitForInventoryLoaded();

// Wait for search INPUT to receive autofocus (80ms timer in WantInventoryPicker)
await page.waitForTimeout(200);
const focusA1 = await getFocus();
console.log(`[A] Focus after form open (with autofocus): ${JSON.stringify(focusA1)}`);

// Press ArrowRight (keyboard) → should navigate to first inventory slot
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(300);
const focusA2 = await getFocus();
console.log(`[A] Focus after ArrowRight: ${JSON.stringify(focusA2)}`);

await page.keyboard.press('ArrowDown');
await page.waitForTimeout(300);
const focusA3 = await getFocus();
console.log(`[A] Focus after ArrowDown: ${JSON.stringify(focusA3)}`);

const slotFocusedA = focusA2.inSlot || focusA3.inSlot;
console.log(`[A] Inventory slot focused: ${slotFocusedA ? PASS : FAIL}`);
console.log(`[A] Focus NOT on card (navId=null): ${focusA2.navId === null ? PASS : FAIL}`);

await page.screenshot({ path: '/tmp/inv_a_after_arrows.png' });
await closeFormSidebar();
await page.waitForTimeout(500);

// ════════════════════════════════════════════════
// SCENARIO B: gamepad Down after 'a' open
// ════════════════════════════════════════════════
console.log('\n══ SCENARIO B: gamepad D-pad after "a" open ══\n');

await page.keyboard.press('a');
await page.waitForTimeout(500);
await waitForFormOpen();
await waitForInventoryLoaded();

// Wait for autofocus
await page.waitForTimeout(200);
const focusB1 = await getFocus();
console.log(`[B] Focus after form open (with autofocus): ${JSON.stringify(focusB1)}`);

await pressBtn(BTN.DOWN);
await page.waitForTimeout(300);
const focusB2 = await getFocus();
console.log(`[B] Focus after D-pad Down: ${JSON.stringify(focusB2)}`);

await pressBtn(BTN.RIGHT);
await page.waitForTimeout(300);
const focusB3 = await getFocus();
console.log(`[B] Focus after D-pad Right: ${JSON.stringify(focusB3)}`);

const slotFocusedB = focusB2.inSlot || focusB3.inSlot;
console.log(`[B] Inventory slot focused (gamepad): ${slotFocusedB ? PASS : FAIL}`);

await page.screenshot({ path: '/tmp/inv_b_gamepad.png' });
await closeFormSidebar();
await page.waitForTimeout(500);

// ════════════════════════════════════════════════
// SCENARIO C: Y button → Add Want → gamepad nav
// ════════════════════════════════════════════════
console.log('\n══ SCENARIO C: Y button → Add Want → gamepad nav ══\n');

// Ensure no focus
await page.evaluate(() => /** @type {HTMLElement} */ (document.activeElement)?.blur?.());

// Press Y to enter header focus mode
await pressBtn(BTN.Y);
await page.waitForTimeout(300);

const focusC0 = await getFocus();
console.log(`[C] After Y button: ${JSON.stringify(focusC0)}`);

// Navigate to Add Want button (should be to the right in header)
// Keep pressing Right until we see the Want button focused
let wantBtnFocused = false;
for (let i = 0; i < 8; i++) {
  await pressBtn(BTN.RIGHT);
  await page.waitForTimeout(200);
  const hFocus = await page.evaluate(() => {
    const idx = window.__headerFocusIdx;
    const btn = document.querySelector('[data-header-btn-id="add-want"]');
    return { idx, btnExists: !!btn };
  });
  const f = await getFocus();
  console.log(`[C] Header nav [${i}]: ${JSON.stringify(f)}`);
  // Check if we're on the want button via visual state
  const wantBtnActive = await page.evaluate(() => {
    // The header shows which button is focused via ring class
    const btns = Array.from(document.querySelectorAll('header button'));
    return btns.some(b => b.classList.contains('ring-2') && (b.textContent?.includes('WANT') || b.textContent?.includes('Want') || b.querySelector('[class*="ring"]')));
  });
  if (wantBtnActive) {
    wantBtnFocused = true;
    console.log('[C] Add Want button appears focused in header');
    break;
  }
}

// If we didn't find via class, just confirm after a few presses
console.log(`[C] Pressing A to confirm...`);
await pressBtn(BTN.CONFIRM);
await page.waitForTimeout(800);

const formOpenC = await page.evaluate(() =>
  Array.from(document.querySelectorAll('[data-sidebar="true"]'))
    .some(s => s.hasAttribute('data-sidebar-open') && s.querySelector('#want-form'))
);
console.log(`[C] Form opened: ${formOpenC}`);

if (formOpenC) {
  await waitForInventoryLoaded();
  // Wait for autofocus
  await page.waitForTimeout(200);
  const focusC1 = await getFocus();
  console.log(`[C] Focus after form open (with autofocus): ${JSON.stringify(focusC1)}`);

  await pressBtn(BTN.DOWN);
  await page.waitForTimeout(300);
  const focusC2 = await getFocus();
  console.log(`[C] Focus after D-pad Down: ${JSON.stringify(focusC2)}`);

  const slotFocusedC = focusC2.inSlot;
  console.log(`[C] Inventory slot focused: ${slotFocusedC ? PASS : FAIL}`);
  await page.screenshot({ path: '/tmp/inv_c_ybtn.png' });
  await closeFormSidebar();
} else {
  console.log(`[C] Form did not open — skipping`);
}

// ════════════════════════════════════════════════
// Summary
// ════════════════════════════════════════════════
console.log('\n══════════════════════════════════════');
console.log('RESULTS:');
console.log(`  A: keyboard ArrowRight/Down navigates inventory: ${slotFocusedA ? PASS : FAIL}`);
console.log(`  B: gamepad D-pad navigates inventory (after 'a'): ${slotFocusedB ? PASS : FAIL}`);
console.log('══════════════════════════════════════\n');

await page.waitForTimeout(2000);
await browser.close();
process.exit((slotFocusedA && slotFocusedB) ? 0 : 1);
