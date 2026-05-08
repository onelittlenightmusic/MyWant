/**
 * Gamepad capture bug reproduction test
 *
 * Scenario:
 *   1. Open "Add Want" sidebar (form sidebar)
 *   2. Select a want type from the inventory
 *   3. Tab to the Add button (submit button)
 *   4. Press gamepad B → sidebar should close
 *   5. Press gamepad Left / Right → want card focus should move
 *
 * Bug: After step 4, _captureListener still points to the disabled WantForm
 * handler and swallows all gamepad input.
 */
import { chromium } from 'playwright';

const PASS = '✅ PASS';
const FAIL = '❌ FAIL';

const browser = await chromium.launch({ headless: false, slowMo: 0 });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

page.on('console', msg => {
  const txt = msg.text();
  if (txt.startsWith('[MOCK]') || txt.startsWith('[GP]') || msg.type() === 'error') {
    console.log(`[PAGE ${msg.type().toUpperCase()}]`, txt);
  }
});

// ── Mock gamepad ──────────────────────────────────────────────────────────────
async function installMockGamepad() {
  await page.evaluate(() => {
    const buttons = Array(17).fill(null).map(() => ({ pressed: false, touched: false, value: 0 }));
    window.__mockGP = {
      axes: [0, 0, 0, 0], buttons, connected: true,
      id: 'Mock Standard Gamepad', index: 0, mapping: 'standard',
      timestamp: performance.now(),
    };
    navigator.getGamepads = () => [window.__mockGP, null, null, null];
    console.log('[MOCK] Gamepad installed');
  });
}

const BTN = { CONFIRM: 0, CANCEL: 1, LEFT: 14, RIGHT: 15 };

async function pressBtn(btnIdx, holdMs = 80) {
  await page.evaluate((idx) => {
    window.__mockGP.buttons[idx] = { pressed: true, touched: true, value: 1 };
    window.__mockGP.timestamp = performance.now();
    console.log('[GP] press btn:', idx);
  }, btnIdx);
  await page.waitForTimeout(holdMs);
  await page.evaluate((idx) => {
    window.__mockGP.buttons[idx] = { pressed: false, touched: false, value: 0 };
    window.__mockGP.timestamp = performance.now();
    console.log('[GP] release btn:', idx);
  }, btnIdx);
  await page.waitForTimeout(120);
}

async function getFocusInfo() {
  return await page.evaluate(() => {
    const el = document.activeElement;
    if (!el || el === document.body) return { tag: 'BODY', text: '', inSidebar: false, navId: null };
    return {
      tag: el.tagName,
      text: ((el.textContent || el.getAttribute('placeholder') || '')).trim().slice(0, 40),
      inSidebar: !!el.closest('[data-sidebar="true"]'),
      navId: el.getAttribute('data-keyboard-nav-id'),
    };
  });
}

// Find the form sidebar (the one containing form#want-form)
async function getFormSidebarOpen() {
  return await page.evaluate(() => {
    const sidebars = Array.from(document.querySelectorAll('[data-sidebar="true"]'));
    const formSidebar = sidebars.find(s => s.querySelector('#want-form'));
    if (!formSidebar) return null;  // null = form sidebar not in DOM (unexpected)
    const t = window.getComputedStyle(formSidebar).transform;
    // open = translateX(0) = 'none' or identity matrix
    return t === 'none' || t === 'matrix(1, 0, 0, 1, 0, 0)';
  });
}

// ── Setup ─────────────────────────────────────────────────────────────────────
console.log('\n=== Gamepad capture bug repro ===\n');

await page.goto('http://localhost:8080');
await page.waitForTimeout(2500);

await installMockGamepad();
await page.waitForTimeout(400);

// Close any open sidebars first
await page.keyboard.press('Escape');
await page.waitForTimeout(300);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

const wantCards = await page.$$('[data-keyboard-nav-id]');
console.log(`Want cards found: ${wantCards.length}`);
if (wantCards.length === 0) {
  console.log('No want cards — cannot test card navigation. Exiting.');
  await browser.close();
  process.exit(1);
}

// ── Step 2: open Add Want sidebar ─────────────────────────────────────────────
console.log('\n[Step 2] Opening Add Want sidebar...');
// The header button has data-header-btn-id="add-want" or contains "WANT"/"Want" text
const opened = await page.evaluate(() => {
  // Try data-header-btn-id first
  let btn = document.querySelector('[data-header-btn-id="add-want"]');
  if (!btn) {
    // Find header button containing "WANT" or "Want"
    btn = Array.from(document.querySelectorAll('header button')).find(b =>
      b.textContent?.includes('WANT') || b.textContent?.includes('Want') ||
      b.getAttribute('title')?.includes('Want')
    );
  }
  if (btn) { btn.click(); return btn.textContent?.trim().slice(0, 30); }
  return null;
});
console.log(`  Clicked: ${opened}`);
await page.waitForTimeout(800);

// Wait for form sidebar to appear in DOM and open
try {
  await page.waitForFunction(() => {
    const sidebars = Array.from(document.querySelectorAll('[data-sidebar="true"]'));
    const formSidebar = sidebars.find(s => s.querySelector('#want-form'));
    if (!formSidebar) return false;
    const t = window.getComputedStyle(formSidebar).transform;
    return t === 'none' || t === 'matrix(1, 0, 0, 1, 0, 0)';
  }, { timeout: 5000 });
  console.log('  Form sidebar opened');
} catch {
  console.log('  WARNING: form sidebar not detected as open');
}
await page.screenshot({ path: '/tmp/gp_step2_sidebar.png' });

// Debug: show what sidebars exist
const sidebarInfo = await page.evaluate(() => {
  return Array.from(document.querySelectorAll('[data-sidebar="true"]')).map((s, i) => {
    const t = window.getComputedStyle(s).transform;
    const hasForm = !!s.querySelector('#want-form');
    const visButtons = Array.from(s.querySelectorAll('button')).filter(b => {
      const r = b.getBoundingClientRect();
      return r.width > 0 && r.height > 0;
    }).length;
    return { i, hasForm, transform: t.slice(0, 40), visButtons };
  });
});
console.log('  Sidebars:', JSON.stringify(sidebarInfo));

// ── Step 3: select first want type ───────────────────────────────────────────
console.log('\n[Step 3] Selecting first want type from inventory...');

// Wait for inventory items to be rendered (draggable aspect-square buttons)
try {
  await page.waitForFunction(() => {
    const sidebars = Array.from(document.querySelectorAll('[data-sidebar="true"]'));
    const formSidebar = sidebars.find(s => s.querySelector('#want-form'));
    if (!formSidebar) return false;
    // Inventory buttons have draggable=true and aspect-square class
    const invBtns = Array.from(formSidebar.querySelectorAll('button[draggable]')).filter(b => {
      const r = b.getBoundingClientRect();
      return r.width > 0 && r.height > 0;
    });
    return invBtns.length > 0;
  }, { timeout: 8000 });
  console.log('  Inventory items loaded');
} catch {
  console.log('  WARNING: inventory items may not have loaded (check screenshot)');
}

// Click first inventory button inside the form sidebar
const typeClicked = await page.evaluate(() => {
  const sidebars = Array.from(document.querySelectorAll('[data-sidebar="true"]'));
  const formSidebar = sidebars.find(s => s.querySelector('#want-form'));
  if (!formSidebar) return 'no form sidebar';

  // Draggable buttons = inventory type items
  const btns = Array.from(formSidebar.querySelectorAll('button[draggable]')).filter(b => {
    const r = b.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && !b.disabled;
  });
  if (btns.length > 0) {
    btns[0].click();
    return `inventory btn (${btns.length} found): ${btns[0].textContent?.trim().slice(0,20)}`;
  }

  // Fallback: TypeRecipeSelector list items (px-2 py-1 list buttons)
  const listBtns = Array.from(formSidebar.querySelectorAll('button.text-left, button[class*="rounded-lg"]')).filter(b => {
    const r = b.getBoundingClientRect();
    return r.width > 0 && r.height > 0 && !b.disabled;
  });
  if (listBtns.length > 0) {
    listBtns[0].click();
    return `list btn: ${listBtns[0].textContent?.trim().slice(0, 20)}`;
  }

  return 'no clickable type button found';
});
console.log(`  Type click: ${typeClicked}`);
await page.waitForTimeout(1000);
await page.screenshot({ path: '/tmp/gp_step3_type_selected.png' });
console.log('  Screenshot: /tmp/gp_step3_type_selected.png');

// Check that form fields appeared (type was selected)
const hasFormFields = await page.evaluate(() => {
  const sidebars = Array.from(document.querySelectorAll('[data-sidebar="true"]'));
  const formSidebar = sidebars.find(s => s.querySelector('#want-form'));
  if (!formSidebar) return false;
  // After type selection, the Name input and section headers appear
  const nameInput = formSidebar.querySelector('input[placeholder*="Name"], input[name="name"], #want-form input');
  const submitBtn = formSidebar.querySelector('button[form="want-form"]');
  return !!nameInput || !!submitBtn;
});
console.log(`  Form fields visible (type selected): ${hasFormFields}`);
if (!hasFormFields) {
  console.log('  Type was NOT selected — check /tmp/gp_step3_type_selected.png');
  await browser.close();
  process.exit(1);
}

// ── Step 4: Tab to the Add button ────────────────────────────────────────────
console.log('\n[Step 4] Focusing Add button in form sidebar...');

// Focus something inside the form sidebar to start Tab navigation
await page.evaluate(() => {
  const sidebars = Array.from(document.querySelectorAll('[data-sidebar="true"]'));
  const formSidebar = sidebars.find(s => s.querySelector('#want-form'));
  if (!formSidebar) return;
  const first = formSidebar.querySelector('input, .focusable-section-header, button:not([tabindex="-1"])');
  if (first) first.focus();
});
await page.waitForTimeout(200);

// Tab until we hit the submit button
let addBtnFocused = false;
for (let i = 0; i < 20; i++) {
  await page.keyboard.press('Tab');
  await page.waitForTimeout(80);
  const isSubmit = await page.evaluate(() => {
    const el = document.activeElement;
    if (!el) return false;
    return el.getAttribute('form') === 'want-form' || el.getAttribute('type') === 'submit';
  });
  const f = await getFocusInfo();
  console.log(`  Tab[${i+1}]: ${f.tag} "${f.text}" inSidebar=${f.inSidebar} isSubmit=${isSubmit}`);
  if (isSubmit && f.inSidebar) { addBtnFocused = true; break; }
}

await page.screenshot({ path: '/tmp/gp_step4_add_focused.png' });
console.log(`  Add button focused: ${addBtnFocused}`);

if (!addBtnFocused) {
  // Direct focus fallback
  await page.evaluate(() => {
    const btn = document.querySelector('button[form="want-form"]');
    if (btn) btn.focus();
  });
  await page.waitForTimeout(200);
  const f = await getFocusInfo();
  addBtnFocused = f.inSidebar;
  console.log(`  After direct focus: ${JSON.stringify(f)}`);
}

// ── Step 5: press gamepad B to close ─────────────────────────────────────────
console.log('\n[Step 5] Pressing gamepad B (CANCEL, index=1)...');
const focusBeforeB = await getFocusInfo();
console.log(`  Focus before B: ${JSON.stringify(focusBeforeB)}`);

await pressBtn(BTN.CANCEL);
await page.waitForTimeout(700);  // React commit + layout effects + transition

await page.screenshot({ path: '/tmp/gp_step5_after_b.png' });
const formSidebarOpen = await getFormSidebarOpen();
const focusAfterB = await getFocusInfo();
console.log(`  Form sidebar open after B: ${formSidebarOpen}`);
console.log(`  Focus after B: ${JSON.stringify(focusAfterB)}`);

if (formSidebarOpen === null) {
  console.log('  Form sidebar disappeared from DOM — treating as closed (OK)');
} else if (formSidebarOpen) {
  console.log(`  ${FAIL} Form sidebar did NOT close. Check /tmp/gp_step5_after_b.png`);
  await browser.close();
  process.exit(1);
}
console.log(`  ${PASS} Form sidebar closed`);

// ── Step 6: check gamepad Left ────────────────────────────────────────────────
console.log('\n[Step 6] Pressing gamepad Left after sidebar close...');
const focusBefore = await getFocusInfo();
console.log(`  Focus before Left: ${JSON.stringify(focusBefore)}`);

await pressBtn(BTN.LEFT);
await page.waitForTimeout(400);

const focusAfterLeft = await getFocusInfo();
console.log(`  Focus after Left: ${JSON.stringify(focusAfterLeft)}`);
await page.screenshot({ path: '/tmp/gp_step6_after_left.png' });

// ── Step 7: check gamepad Right ───────────────────────────────────────────────
console.log('\n[Step 7] Pressing gamepad Right...');
await pressBtn(BTN.RIGHT);
await page.waitForTimeout(400);

const focusAfterRight = await getFocusInfo();
console.log(`  Focus after Right: ${JSON.stringify(focusAfterRight)}`);
await page.screenshot({ path: '/tmp/gp_step7_after_right.png' });

// ── Verdict ───────────────────────────────────────────────────────────────────
// "Navigation works" = a want card got focused (navId != null) OR
//   focus moved to a non-BODY, non-sidebar element
const leftNavWorks  = focusAfterLeft.navId  !== null || (!focusAfterLeft.inSidebar  && focusAfterLeft.tag  !== 'BODY');
const rightNavWorks = focusAfterRight.navId !== null || (!focusAfterRight.inSidebar && focusAfterRight.tag !== 'BODY');

console.log('\n══════════════════════════════════════');
console.log('RESULTS:');
console.log(`  Form sidebar closed on B:  ${!(formSidebarOpen) ? PASS : FAIL}`);
console.log(`  Focus left sidebar after B: ${!focusAfterB.inSidebar ? PASS : FAIL}  (${focusAfterB.tag})`);
console.log(`  Left navigates card:       ${leftNavWorks  ? PASS : FAIL}  navId=${focusAfterLeft.navId}`);
console.log(`  Right navigates card:      ${rightNavWorks ? PASS : FAIL}  navId=${focusAfterRight.navId}`);
console.log('══════════════════════════════════════\n');

const allPassed = !formSidebarOpen && (leftNavWorks || rightNavWorks);
console.log(allPassed ? `${PASS} Bug FIXED` : `${FAIL} Bug still present — gamepad navigation stuck after closing form sidebar`);

await page.waitForTimeout(2000);
await browser.close();
process.exit(allPassed ? 0 : 1);
