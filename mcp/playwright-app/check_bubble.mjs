import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false, slowMo: 500 });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// グリッドを確認
const grid = await page.$('#want-grid-container');
if (!grid) {
  console.log('Grid not found');
  await browser.close();
  process.exit(1);
}

// 全 want card の初期位置を取得
const cards = await page.$$('[data-want-id]');
console.log(`=== 初期状態: ${cards.length} cards ===`);
const initialPositions = [];
for (const card of cards) {
  const box = await card.boundingBox();
  const id = await card.getAttribute('data-want-id');
  if (box) {
    initialPositions.push({ id: id?.slice(0, 8), x: Math.round(box.x), y: Math.round(box.y), w: Math.round(box.width), h: Math.round(box.height) });
    console.log(`  card ${id?.slice(0, 8)}: x=${Math.round(box.x)} y=${Math.round(box.y)} w=${Math.round(box.width)}`);
  }
}

// gridのactual column count をCSSから取得
const gridCols = await grid.evaluate(el => {
  return window.getComputedStyle(el).gridTemplateColumns;
});
console.log(`\n=== Grid computed columns: ${gridCols} ===`);
const colCount = gridCols.split(' ').filter(Boolean).length;
console.log(`Column count: ${colCount}`);

// level-1-approval-example を探してクリック
const target = initialPositions[0]; // 最初のカードをクリック
const firstCard = cards[0];
console.log('\n=== 1枚目カードをクリック ===');
await firstCard.click();
await page.waitForTimeout(1500);

// bubble が出たか確認
const bubble = await page.$('.col-span-full');
console.log(`Bubble appeared: ${!!bubble}`);
if (bubble) {
  const bubbleBox = await bubble.boundingBox();
  console.log(`Bubble: x=${Math.round(bubbleBox.x)} y=${Math.round(bubbleBox.y)} w=${Math.round(bubbleBox.width)}`);

  // キャレット位置
  const caret = await bubble.$('.absolute.overflow-hidden.z-10');
  if (caret) {
    const caretBox = await caret.boundingBox();
    console.log(`Caret: x=${Math.round(caretBox.x)} y=${Math.round(caretBox.y)}`);
    console.log(`Caret center X: ${Math.round(caretBox.x + caretBox.width / 2)}`);
    console.log(`Expected parent card center X: ${Math.round(initialPositions[0].x + initialPositions[0].w / 2)}`);
  }
}

// バブル出現後の全カード位置
console.log('\n=== バブル出現後のカード位置 ===');
const cardsAfter = await page.$$('[data-want-id]');
for (let i = 0; i < cardsAfter.length; i++) {
  const box = await cardsAfter[i].boundingBox();
  const id = await cardsAfter[i].getAttribute('data-want-id');
  if (box) {
    const prev = initialPositions[i];
    const moved = prev ? (Math.abs(box.x - prev.x) > 2 || Math.abs(box.y - prev.y) > 2) : false;
    console.log(`  card[${i}] ${id?.slice(0,8)}: x=${Math.round(box.x)} y=${Math.round(box.y)}${moved ? ' *** MOVED ***' : ''}`);
  }
}

await page.screenshot({ path: '/tmp/bubble_check.png', fullPage: false });
console.log('\nScreenshot saved: /tmp/bubble_check.png');

await page.waitForTimeout(3000);
await browser.close();
