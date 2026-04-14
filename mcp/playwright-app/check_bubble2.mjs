import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false, slowMo: 300 });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

const grid = await page.$('#want-grid-container');

// Grid の実際の列数
const gridCols = await grid.evaluate(el =>
  window.getComputedStyle(el).gridTemplateColumns
);
const colCount = gridCols.split(' ').filter(Boolean).length;
console.log(`Grid columns: ${colCount}  (tracks: ${gridCols})`);

// 全カードの初期位置
const cards = await page.$$('[data-want-id]');
console.log(`\n=== 初期: ${cards.length} cards ===`);
const before = [];
for (const c of cards) {
  const box = await c.boundingBox();
  const id = await c.getAttribute('data-want-id');
  if (box) {
    before.push({ id, shortId: id.slice(0,8), x: Math.round(box.x), y: Math.round(box.y), w: Math.round(box.width), h: Math.round(box.height) });
    console.log(`  [${before.length-1}] ${id.slice(0,8)}: x=${Math.round(box.x)} y=${Math.round(box.y)} h=${Math.round(box.height)}`);
  }
}

// level-1-approval-example (want-ab7dcd5c) を探す
const level1Id = 'want-ab7dcd5c-f5b9-4fce-91a9-8609362f1b04';
const level1Card = await page.$(`[data-want-id="${level1Id}"]`);
if (!level1Card) {
  console.log('level-1-approval card not found! Trying partial match...');
  // try clicking any card that has children
  // Click 2nd card
  await cards[1].click();
} else {
  const idx = before.findIndex(b => b.id === level1Id);
  console.log(`\n=== level-1-approval-example: index=${idx} ===`);
  await level1Card.click();
}

await page.waitForTimeout(2000);

// bubble チェック
const bubbles = await page.$$('.col-span-full');
console.log(`\nBubbles found: ${bubbles.length}`);
for (const b of bubbles) {
  const box = await b.boundingBox();
  if (box) console.log(`  bubble: x=${Math.round(box.x)} y=${Math.round(box.y)} w=${Math.round(box.width)} h=${Math.round(box.height)}`);
  // キャレット
  const caret = await b.$('.absolute.overflow-hidden.z-10');
  if (caret) {
    const cb = await caret.boundingBox();
    console.log(`  caret left edge: x=${Math.round(cb.x)}  center: ${Math.round(cb.x + cb.width/2)}`);
  }
}

// バブル後のカード位置
console.log('\n=== バブル後のカード位置 ===');
const cardsAfter = await page.$$('[data-want-id]');
for (let i = 0; i < cardsAfter.length; i++) {
  const box = await cardsAfter[i].boundingBox();
  const id = await cardsAfter[i].getAttribute('data-want-id');
  if (!box) continue;
  const prev = before[i];
  const movedX = prev ? Math.abs(box.x - prev.x) > 2 : false;
  const movedY = prev ? Math.abs(box.y - prev.y) > 5 : false;
  const tag = movedX || movedY ? ` *** MOVED (dx=${prev ? Math.round(box.x-prev.x) : '?'} dy=${prev ? Math.round(box.y-prev.y) : '?'}) ***` : '';
  console.log(`  [${i}] ${id.slice(0,8)}: x=${Math.round(box.x)} y=${Math.round(box.y)}${tag}`);
}

// 期待するバブル挿入位置
console.log('\n=== 診断 ===');
const level1Idx = before.findIndex(b => b.id === level1Id);
if (level1Idx >= 0) {
  const parentRow = Math.floor(level1Idx / colCount);
  const expectedRowEnd = Math.min(parentRow * colCount + colCount - 1, before.length - 1);
  console.log(`parent index: ${level1Idx}, colCount: ${colCount}`);
  console.log(`expected bubbleRowEndIndex: ${expectedRowEnd} (= card "${before[expectedRowEnd]?.shortId}")`);
}

await page.screenshot({ path: '/tmp/bubble_check2.png', fullPage: false });
console.log('\nScreenshot: /tmp/bubble_check2.png');
await page.waitForTimeout(4000);
await browser.close();
