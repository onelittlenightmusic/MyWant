/**
 * Keyboard capture consistency test
 *
 * Verifies that pressing 'a' to open Add Want sidebar and then pressing
 * arrow keys navigates the inventory picker (not want cards), consistent
 * with the gamepad behavior.
 */
import { chromium } from 'playwright';

const PASS = '✅ PASS';
const FAIL = '❌ FAIL';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

async function getFocusInfo() {
  return await page.evaluate(() => {
    const el = document.activeElement;
    if (!el || el === document.body) return { tag: 'BODY', text: '', inSidebar: false, navId: null };
    return {
      tag: el.tagName,
      text: ((el.textContent || el.getAttribute('placeholder') || '')).trim().slice(0, 40),
      inSidebar: !!el.closest('[data-sidebar="true"][data-sidebar-open="true"]'),
      navId: el.getAttribute('data-keyboard-nav-id'),
    };
  });
}

console.log('\n=== Keyboard + Sidebar consistency test ===\n');

await page.goto('http://localhost:8080');
await page.waitForTimeout(2500);

// Dismiss any open sidebars
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

// ── Step 1: focus a want card first ──────────────────────────────────────────
console.log('[Step 1] Focus a want card via Right arrow...');
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(300);
const focusOnCard = await getFocusInfo();
console.log(`  Focus: ${JSON.stringify(focusOnCard)}`);
const cardFocusedInitially = focusOnCard.navId !== null;
console.log(`  Card focused: ${cardFocusedInitially ? PASS : FAIL}`);

// ── Step 2: press 'a' to open Add Want sidebar ───────────────────────────────
console.log('\n[Step 2] Press "a" to open Add Want sidebar...');
await page.keyboard.press('a');
await page.waitForTimeout(800);

// Wait for form sidebar to open
try {
  await page.waitForFunction(() => {
    const sidebars = Array.from(document.querySelectorAll('[data-sidebar="true"]'));
    return sidebars.some(s => s.hasAttribute('data-sidebar-open') && s.querySelector('#want-form'));
  }, { timeout: 5000 });
  console.log('  Form sidebar opened');
} catch {
  console.log('  WARNING: form sidebar not detected');
}
await page.screenshot({ path: '/tmp/kb_step2_form_open.png' });

// ── Step 3: press arrow keys — should navigate inventory, NOT cards ───────────
console.log('\n[Step 3] Arrow keys inside sidebar should navigate inventory...');
const focusBeforeArrow = await getFocusInfo();
console.log(`  Focus before arrows: ${JSON.stringify(focusBeforeArrow)}`);

await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);
const focusAfterRight = await getFocusInfo();
console.log(`  After ArrowRight: ${JSON.stringify(focusAfterRight)}`);

await page.keyboard.press('ArrowDown');
await page.waitForTimeout(200);
const focusAfterDown = await getFocusInfo();
console.log(`  After ArrowDown: ${JSON.stringify(focusAfterDown)}`);

// Card nav would change the want card selection (navId changes outside sidebar)
// Inventory nav would either change focus inside sidebar or do nothing visible
const arrowWentToCards = focusAfterRight.navId !== null && !focusAfterRight.inSidebar;
const arrowStayedInSidebar = focusAfterRight.inSidebar || focusAfterRight.tag === 'BODY';

console.log(`  Arrow stayed in sidebar context (not card nav): ${!arrowWentToCards ? PASS : FAIL}`);

// ── Step 4: press Escape to close the sidebar ─────────────────────────────────
console.log('\n[Step 4] Press Escape to close sidebar...');
await page.keyboard.press('Escape');
await page.waitForTimeout(500);

try {
  await page.waitForFunction(() => {
    return !Array.from(document.querySelectorAll('[data-sidebar="true"]'))
      .some(s => s.hasAttribute('data-sidebar-open') && s.querySelector('#want-form'));
  }, { timeout: 3000 });
  console.log('  Form sidebar closed');
} catch {
  console.log('  WARNING: form sidebar still open?');
}

// ── Step 5: arrow keys should now navigate cards ──────────────────────────────
console.log('\n[Step 5] Arrow keys after close should navigate cards...');
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(300);
const focusAfterClose = await getFocusInfo();
console.log(`  After ArrowRight (post-close): ${JSON.stringify(focusAfterClose)}`);

const cardNavWorksAfterClose = focusAfterClose.navId !== null;
console.log(`  Card navigation restored: ${cardNavWorksAfterClose ? PASS : FAIL}`);

await page.screenshot({ path: '/tmp/kb_step5_after_close.png' });

// ── Verdict ───────────────────────────────────────────────────────────────────
console.log('\n══════════════════════════════════════');
console.log('RESULTS:');
console.log(`  Initial card focus via Right: ${cardFocusedInitially ? PASS : FAIL}`);
console.log(`  Arrows don't nav cards while sidebar open: ${!arrowWentToCards ? PASS : FAIL}`);
console.log(`  Card nav restored after close: ${cardNavWorksAfterClose ? PASS : FAIL}`);
console.log('══════════════════════════════════════\n');

const allPassed = cardFocusedInitially && !arrowWentToCards && cardNavWorksAfterClose;
console.log(allPassed ? `${PASS} Keyboard/gamepad parity confirmed` : `${FAIL} Inconsistency remains`);

await page.waitForTimeout(2000);
await browser.close();
process.exit(allPassed ? 0 : 1);
