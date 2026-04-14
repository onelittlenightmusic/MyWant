import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// getColumns() の実際の挙動をシミュレート
const debug = await page.evaluate(() => {
  const grid = document.getElementById('want-grid-container');
  if (!grid) return { error: 'no grid' };
  
  const tracks = window.getComputedStyle(grid).gridTemplateColumns;
  const parts = tracks.split(' ');
  const filtered = parts.filter(Boolean);
  
  return {
    raw: tracks,
    rawLen: tracks.length,
    splitParts: parts,
    filteredParts: filtered,
    filteredLen: filtered.length,
    colCount: Math.max(1, filtered.length),
    // 比較: grid の style 属性
    inlineStyle: grid.style.gridTemplateColumns,
    clientWidth: grid.clientWidth,
  };
});

console.log('=== getColumns() デバッグ ===');
console.log('raw:', JSON.stringify(debug.raw));
console.log('split parts:', JSON.stringify(debug.splitParts));
console.log('filtered parts:', JSON.stringify(debug.filteredParts));
console.log('→ colCount:', debug.colCount);
console.log('inline style:', debug.inlineStyle);
console.log('clientWidth:', debug.clientWidth);

// クリック後も確認
const level1Card = await page.$('[data-want-id="want-ab7dcd5c-f5b9-4fce-91a9-8609362f1b04"]');
await level1Card.click();
await page.waitForTimeout(500);

const debugAfter = await page.evaluate(() => {
  const grid = document.getElementById('want-grid-container');
  const bubble = document.querySelector('.col-span-full');
  const tracks = window.getComputedStyle(grid).gridTemplateColumns;
  const filtered = tracks.split(' ').filter(Boolean);
  
  // React state を React DevTools fiber から取得しようとする
  const gridEl = grid;
  const fiberKey = Object.keys(gridEl).find(k => k.startsWith('__reactFiber'));
  let reactGridCols = 'unknown';
  if (fiberKey) {
    try {
      let fiber = gridEl[fiberKey];
      while (fiber) {
        if (fiber.memoizedState) {
          const state = fiber.memoizedState;
          // gridColumns は WantGrid の最初の state
          if (typeof state.memoizedState === 'number') {
            reactGridCols = state.memoizedState;
            break;
          }
        }
        fiber = fiber.return;
      }
    } catch(e) {}
  }
  
  return {
    tracksAfter: tracks,
    colCountAfter: Math.max(1, filtered.length),
    bubbleExists: !!bubble,
    reactGridCols,
  };
});

console.log('\n=== クリック後 ===');
console.log('tracks:', JSON.stringify(debugAfter.tracksAfter));
console.log('colCount:', debugAfter.colCountAfter);
console.log('bubble exists:', debugAfter.bubbleExists);
console.log('React gridColumns state:', debugAfter.reactGridCols);

await browser.close();
