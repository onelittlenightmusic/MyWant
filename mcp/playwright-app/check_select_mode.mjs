/**
 * Select-mode keyboard/gamepad navigation test
 * Verifies: Shift+S → select mode, arrow nav (triangle), Enter toggle, Escape exit
 */
import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });

page.on('console', msg => {
  const t = msg.text();
  if (msg.type() === 'error' || t.includes('[CAP]') || t.includes('[DBG]') || t.includes('[GP]') || t.includes('[MOCK]')) console.log('[PAGE]', t);
});

await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

// Helper: check if select mode is active via header button class
const isSelectModeActive = () => page.evaluate(() => {
  const btn = document.querySelector('[data-header-btn-id="select"]');
  if (!btn) return null; // button not found
  return btn.classList.contains('bg-blue-600') || btn.className.includes('bg-blue-600');
});

// Helper: get navId of the currently navigated want card
const getNavId = () => page.evaluate(() => {
  // Find card with data-selected attribute or focus ring
  const selected = document.querySelector('[data-keyboard-nav-id][data-selected="true"]');
  if (selected) return selected.getAttribute('data-keyboard-nav-id');
  // Fallback: check what card has focus
  const active = document.activeElement?.closest('[data-keyboard-nav-id]');
  return active?.getAttribute('data-keyboard-nav-id') ?? null;
});

// Helper: count checked/unchecked squares in visible cards
const getCheckCounts = () => page.evaluate(() => {
  // Look for CheckSquare icon (has specific path data) and Square icon
  // Use the presence of svg inside the absolute top-right div on want cards
  const checked = document.querySelectorAll('[data-keyboard-nav-id] .absolute.top-2.right-2 svg:first-child').length;
  // A simpler approach: find the blue checkbox vs gray checkbox
  const checkSquares = document.querySelectorAll('[data-keyboard-nav-id] .text-blue-600 svg, [data-keyboard-nav-id] svg.lucide-check-square, [data-keyboard-nav-id] [class*="check-square"]');
  const squares = document.querySelectorAll('[data-keyboard-nav-id] .opacity-50 svg, [data-keyboard-nav-id] svg.lucide-square');
  // Most reliable: look for the want cards and see if any have the select overlay
  const cardsWithOverlay = document.querySelectorAll('[data-keyboard-nav-id] .pointer-events-none.absolute.top-2.right-2');
  return {
    cardsWithOverlay: cardsWithOverlay.length,
    checkSquareCount: checkSquares.length,
    squareCount: squares.length,
  };
});

console.log('\n══ Check header button exists ══');
const btnState0 = await isSelectModeActive();
console.log('Select button found:', btnState0 !== null ? 'yes' : 'no');
console.log('Select mode initially active:', btnState0);

console.log('\n══ Enter select mode with Shift+S ══');
await page.keyboard.press('Shift+s');
await page.waitForTimeout(400);

const btnState1 = await isSelectModeActive();
console.log('Select button active after Shift+S:', btnState1);
console.log(btnState1 ? '✅ Select mode active (button highlighted)' : '❌ Select mode NOT active');

// Also try clicking the button directly if Shift+S didn't work
if (!btnState1) {
  console.log('[DEBUG] Shift+S failed, trying button click...');
  await page.click('[data-header-btn-id="select"]');
  await page.waitForTimeout(400);
  const btnState1b = await isSelectModeActive();
  console.log('After button click:', btnState1b);
}

const counts1 = await getCheckCounts();
console.log('Card overlay counts:', JSON.stringify(counts1));

console.log('\n══ Arrow key navigation ══');
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);
const navId1 = await getNavId();
console.log('After ArrowRight, navId:', navId1);
console.log(navId1 ? '✅ Card focused (triangle shows)' : '❌ No card focused');

console.log('\n══ Enter to toggle checkbox ON ══');
await page.keyboard.press('Enter');
await page.waitForTimeout(300);
const counts2 = await getCheckCounts();
const navId2 = await getNavId();
console.log('After Enter, counts:', JSON.stringify(counts2), 'navId:', navId2);

// Check via GUI state
const guiState1 = await page.evaluate(async () => {
  try {
    const r = await fetch('/api/v1/gui/state');
    const j = await r.json();
    return j?.state?.current?.form_situation;
  } catch (e) { return 'error: ' + e.message; }
});
console.log('GUI state form_situation:', guiState1);

console.log('\n══ Enter again to toggle OFF ══');
await page.keyboard.press('Enter');
await page.waitForTimeout(300);
const counts3 = await getCheckCounts();
console.log('After 2nd Enter, counts:', JSON.stringify(counts3));

console.log('\n══ Escape to exit select mode ══');
await page.keyboard.press('Escape');
await page.waitForTimeout(400);
const btnState2 = await isSelectModeActive();
console.log('Select mode after Escape:', btnState2);
console.log(!btnState2 ? '✅ Exited select mode' : '❌ Still in select mode');

// GUI state after exit
const guiState2 = await page.evaluate(async () => {
  try {
    const r = await fetch('/api/v1/gui/state');
    const j = await r.json();
    return j?.state?.current?.form_situation;
  } catch (e) { return 'error: ' + e.message; }
});
console.log('GUI state after Escape:', guiState2);

// Take screenshot for visual confirmation
await page.keyboard.press('Shift+s');
await page.waitForTimeout(300);
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(200);
await page.screenshot({ path: '/tmp/select_mode_nav.png' });
console.log('Screenshot: /tmp/select_mode_nav.png');
await page.keyboard.press('Enter');
await page.waitForTimeout(200);
await page.screenshot({ path: '/tmp/select_mode_checked.png' });
console.log('Screenshot: /tmp/select_mode_checked.png');

await page.waitForTimeout(1000);
await browser.close();
