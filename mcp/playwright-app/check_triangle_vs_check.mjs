/**
 * Verify: triangle follows navigation cursor, checkbox follows Enter — must be independent
 */
import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

await page.click('[data-header-btn-id="select"]');
await page.waitForTimeout(300);

// Find which want-card contains the FocusTriangle (bg-blue-500 with absolute positioning)
// and which have the CheckSquare (text-blue-600 in select mode)
const snapshot = () => page.evaluate(() => {
  // Triangle: FocusTriangle renders a div with class "absolute left-3 z-20 bg-blue-500"
  // Find all triangles and their parent want cards
  const triangles = Array.from(document.querySelectorAll('.absolute.left-3.z-20.bg-blue-500, .absolute.left-3.z-20.bg-blue-400'));
  const triangleCards = triangles.map(t => {
    const container = t.closest('.relative');
    const cardEl = container?.querySelector('[data-keyboard-nav-id]');
    return cardEl?.getAttribute('data-keyboard-nav-id')?.slice(-8);
  }).filter(Boolean);

  // Checkbox: CheckSquare icon — text-blue-600 inside [data-keyboard-nav-id]
  const checkedCards = Array.from(document.querySelectorAll('[data-keyboard-nav-id]'))
    .filter(c => !!c.querySelector('.text-blue-600'))
    .map(c => c.getAttribute('data-keyboard-nav-id')?.slice(-8));

  const allCards = Array.from(document.querySelectorAll('[data-keyboard-nav-id]'))
    .map(c => c.getAttribute('data-keyboard-nav-id')?.slice(-8));

  return { triangleCards, checkedCards, allCards };
});

// Step 0: navigate left several times to land on a non-last card
console.log('\n══ Step 0: Navigate left to reach a non-boundary card ══');
for (let i = 0; i < 5; i++) {
  await page.keyboard.press('ArrowLeft');
  await page.waitForTimeout(150);
}
const s0 = await snapshot();
console.log('All cards:', s0.allCards, '  Triangles:', s0.triangleCards);

// Step 1: ArrowRight → move to card A
console.log('\n══ Step 1: ArrowRight → card A ══');
await page.keyboard.press('ArrowRight');
await page.waitForTimeout(400);
const s1 = await snapshot();
console.log('Triangles:', s1.triangleCards, '  Checked:', s1.checkedCards);
const cardA = s1.triangleCards[0];
console.log('Card A:', cardA, s1.triangleCards.length > 0 ? '✅' : '❌ no triangle');

await page.screenshot({ path: '/tmp/t1_nav.png', clip: { x: 0, y: 0, width: 460, height: 300 } });

// Step 2: Enter → check A, triangle stays on A
console.log('\n══ Step 2: Enter → check A ══');
await page.keyboard.press('Enter');
await page.waitForTimeout(300);
const s2 = await snapshot();
console.log('Triangles:', s2.triangleCards, '  Checked:', s2.checkedCards);
const triStillOnA = s2.triangleCards.includes(cardA);
const aIsChecked = s2.checkedCards.includes(cardA);
console.log(triStillOnA ? '✅ Triangle still on A' : '❌ Triangle moved away from A');
console.log(aIsChecked ? '✅ A is checked' : '❌ A not checked');

await page.screenshot({ path: '/tmp/t2_checked.png', clip: { x: 0, y: 0, width: 460, height: 300 } });

// Step 3: ArrowLeft → triangle moves to prev card B, A keeps checkbox
// (using ArrowLeft because A might be at the rightmost boundary)
console.log('\n══ Step 3: ArrowLeft → triangle moves to prev card B ══');
await page.keyboard.press('ArrowLeft');
await page.waitForTimeout(400);
const s3 = await snapshot();
console.log('Triangles:', s3.triangleCards, '  Checked:', s3.checkedCards);
const cardB = s3.triangleCards[0];
const triMovedToB = cardB && cardB !== cardA;
const aStillChecked = s3.checkedCards.includes(cardA);
const aTriGone = !s3.triangleCards.includes(cardA);
console.log('Card B (triangle):', cardB);
console.log(triMovedToB ? '✅ Triangle moved to B' : '❌ Triangle did NOT move');
console.log(aStillChecked ? '✅ A checkbox persists' : '❌ A checkbox lost');
console.log(aTriGone ? '✅ A triangle gone' : '❌ A triangle stuck');

await page.screenshot({ path: '/tmp/t3_moved.png', clip: { x: 0, y: 0, width: 460, height: 300 } });

const passed = s1.triangleCards.length > 0 && triStillOnA && aIsChecked && triMovedToB && aStillChecked && aTriGone;
console.log('\n' + (passed ? '✅ PASS: Triangle and checkbox are independent' : '❌ FAIL'));
console.log('Screenshots: /tmp/t1_nav.png  /tmp/t2_checked.png  /tmp/t3_moved.png');

await page.waitForTimeout(500);
await browser.close();
process.exit(passed ? 0 : 1);
