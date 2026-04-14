import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// 2列目 (level-1-approval) をクリック
const level1Id = 'want-ab7dcd5c-f5b9-4fce-91a9-8609362f1b04';
const card = await page.$(`[data-want-id="${level1Id}"]`);
await card.click();
await page.waitForTimeout(1500);

// バブルの詳細を調べる
const info = await page.evaluate(() => {
  const bubble = document.querySelector('.col-span-full');
  if (!bubble) return { error: 'no bubble' };

  const bubbleRect = bubble.getBoundingClientRect();
  const bubbleStyle = window.getComputedStyle(bubble);

  // キャレットdiv (absolute overflow-hidden)
  const caret = bubble.querySelector('.absolute.overflow-hidden');
  const caretRect = caret?.getBoundingClientRect();
  const caretStyle = caret ? window.getComputedStyle(caret) : null;

  // バブルの relative div (outermost)
  const outerDiv = bubble; // ref=containerRef
  const outerRect = outerDiv.getBoundingClientRect();

  // caretのleftスタイル (inline style)
  const caretInlineLeft = caret?.style?.left;

  // グリッドのcomputed columns
  const grid = document.getElementById('want-grid-container');
  const gridCols = grid ? window.getComputedStyle(grid).gridTemplateColumns : 'N/A';

  return {
    bubbleX: Math.round(bubbleRect.x), bubbleW: Math.round(bubbleRect.width),
    caretX: caretRect ? Math.round(caretRect.x) : null,
    caretW: caretRect ? Math.round(caretRect.width) : null,
    caretInlineLeft,
    caretComputedLeft: caretStyle?.left,
    gridCols,
    gridColCount: gridCols.split(' ').filter(Boolean).length,
  };
});

console.log('=== キャレット診断 ===');
console.log(`Grid computed columns: "${info.gridCols}" (${info.gridColCount}列)`);
console.log(`Bubble: x=${info.bubbleX} w=${info.bubbleW}`);
console.log(`Caret inline style left: "${info.caretInlineLeft}"`);
console.log(`Caret computed left: "${info.caretComputedLeft}"`);
console.log(`Caret screen x: ${info.caretX} w=${info.caretW}`);
if (info.caretX && info.bubbleX) {
  console.log(`Caret left from bubble: ${info.caretX - info.bubbleX}px`);
  console.log(`Caret center from bubble: ${info.caretX - info.bubbleX + info.caretW/2}px (${((info.caretX - info.bubbleX + info.caretW/2)/info.bubbleW*100).toFixed(1)}% of bubble)`);
}

// 期待値
const expectedCol = 1 % info.gridColCount;
const expectedPct = (expectedCol / info.gridColCount) * 100 + (1 / info.gridColCount / 2) * 100;
console.log(`\n期待値 (col=1, gridColumns=${info.gridColCount}):`);
console.log(`  caretLeftPct = ${expectedPct.toFixed(1)}%`);
console.log(`  caretLeft = calc(${expectedPct.toFixed(1)}% - 14px)`);
console.log(`  = ${(expectedPct/100 * info.bubbleW - 14).toFixed(1)}px from bubble left`);

await browser.close();
