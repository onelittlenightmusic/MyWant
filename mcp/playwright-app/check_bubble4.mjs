import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false, slowMo: 200 });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

const getLayout = async (label) => {
  const data = await page.evaluate(() => {
    const grid = document.getElementById('want-grid-container');
    if (!grid) return null;
    const cols = window.getComputedStyle(grid).gridTemplateColumns;
    const children = Array.from(grid.children).map((el, i) => {
      const rect = el.getBoundingClientRect();
      const isBubble = el.className?.includes('col-span-full');
      const wantId = el.getAttribute('data-want-id');
      return { i, type: isBubble ? 'BUBBLE' : `card(${wantId?.slice(0,8)})`, x: Math.round(rect.x), y: Math.round(rect.y), w: Math.round(rect.width) };
    });
    // カレット位置
    const caret = document.querySelector('.col-span-full .absolute.overflow-hidden');
    const caretRect = caret?.getBoundingClientRect();
    const caretCenterX = caretRect ? Math.round(caretRect.x + caretRect.width/2) : null;
    return { colCount: cols.split(' ').filter(Boolean).length, gridCols: cols, children, caretCenterX };
  });
  console.log(`\n=== [${label}] cols=${data.colCount} (${data.gridCols}) ===`);
  data.children.forEach(c => console.log(`  [${c.i}] ${c.type}: x=${c.x} y=${c.y}`));
  if (data.caretCenterX !== null) console.log(`  caret center X: ${data.caretCenterX}`);
  return data;
};

// --- シナリオ A: 1列目 (index=0) をクリック ---
console.log('\n╔══════════════════════════════╗');
console.log('║ シナリオA: 1列目カードをクリック ║');
console.log('╚══════════════════════════════╝');
await getLayout('クリック前');

const cards0 = await page.$$('[data-want-id]');
const card0rect = await cards0[0].boundingBox();
console.log(`\n→ 1列目カードをクリック (x=${Math.round(card0rect.x)}, y=${Math.round(card0rect.y)})`);
await cards0[0].click();
await page.waitForTimeout(1500);
const afterA = await getLayout('1列目クリック後');
const bubbleA = afterA.children.find(c => c.type === 'BUBBLE');
const parent0 = { x: Math.round(card0rect.x), cx: Math.round(card0rect.x + card0rect.width / 2) };
if (bubbleA) {
  console.log(`\n  → 親カード中心X: ${parent0.cx}`);
  console.log(`  → キャレット中心X: ${afterA.caretCenterX}`);
  console.log(`  → ズレ: ${afterA.caretCenterX - parent0.cx}px`);
  // バブル挿入がどのカードの後か確認
  const bubbleIdx = afterA.children.findIndex(c => c.type === 'BUBBLE');
  console.log(`  → バブル挿入位置: grid child index ${bubbleIdx} (直前: ${afterA.children[bubbleIdx-1]?.type})`);
} else {
  console.log('  → バブルが出なかった（子がない）');
}

// 閉じる
await page.keyboard.press('Escape');
await page.waitForTimeout(1000);

// --- シナリオ B: 2列目 (level-1-approval, index=1) をクリック ---
console.log('\n╔══════════════════════════════╗');
console.log('║ シナリオB: 2列目カードをクリック ║');
console.log('╚══════════════════════════════╝');
await getLayout('クリック前');

const cards1 = await page.$$('[data-want-id]');
const card1 = cards1[1];
const card1rect = await card1.boundingBox();
console.log(`\n→ 2列目カードをクリック (x=${Math.round(card1rect.x)}, y=${Math.round(card1rect.y)})`);
await card1.click();
await page.waitForTimeout(1500);
const afterB = await getLayout('2列目クリック後');
const bubbleB = afterB.children.find(c => c.type === 'BUBBLE');
const parent1cx = Math.round(card1rect.x + card1rect.width / 2);
if (bubbleB) {
  console.log(`\n  → 親カード中心X: ${parent1cx}`);
  console.log(`  → キャレット中心X: ${afterB.caretCenterX}`);
  console.log(`  → ズレ: ${afterB.caretCenterX - parent1cx}px`);
  const bubbleIdx = afterB.children.findIndex(c => c.type === 'BUBBLE');
  console.log(`  → バブル挿入位置: grid child index ${bubbleIdx} (直前: ${afterB.children[bubbleIdx-1]?.type})`);
} else {
  console.log('  → バブルが出なかった（子がない）');
}

await page.screenshot({ path: '/tmp/bubble_check4.png' });
console.log('\nScreenshot: /tmp/bubble_check4.png');
await page.waitForTimeout(3000);
await browser.close();
