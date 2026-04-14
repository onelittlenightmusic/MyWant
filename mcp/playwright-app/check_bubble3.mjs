import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false, slowMo: 200 });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// 初期状態の計測
const getGridInfo = async (label) => {
  const info = await page.evaluate(() => {
    const grid = document.getElementById('want-grid-container');
    if (!grid) return null;
    const rect = grid.getBoundingClientRect();
    const cols = window.getComputedStyle(grid).gridTemplateColumns;
    // 親コンテナの幅も取得
    const parent = grid.parentElement;
    const parentRect = parent?.getBoundingClientRect();
    // サイドバーやパネルのサイズも確認
    const allPanels = Array.from(document.querySelectorAll('[class*="sidebar"], [class*="panel"], [class*="drawer"]'))
      .map(el => ({ tag: el.tagName, cls: el.className.slice(0,60), w: el.getBoundingClientRect().width }))
      .filter(p => p.w > 100);
    return {
      gridX: Math.round(rect.x), gridW: Math.round(rect.width),
      gridCols: cols,
      colCount: cols.split(' ').filter(Boolean).length,
      parentW: Math.round(parentRect?.width || 0),
      panels: allPanels
    };
  });
  console.log(`\n=== [${label}] ===`);
  console.log(`  grid: x=${info.gridX} width=${info.gridW} parent=${info.parentW}`);
  console.log(`  cols(${info.colCount}): ${info.gridCols}`);
  if (info.panels.length) console.log(`  panels: ${JSON.stringify(info.panels)}`);
  return info;
};

await getGridInfo('初期');

// level-1-approval をクリック
const level1Id = 'want-ab7dcd5c-f5b9-4fce-91a9-8609362f1b04';
const level1Card = await page.$(`[data-want-id="${level1Id}"]`);
if (!level1Card) {
  // 2番目のカードをクリック
  const cards = await page.$$('[data-want-id]');
  await cards[1].click();
} else {
  await level1Card.click();
}

// クリック直後
await page.waitForTimeout(100);
await getGridInfo('クリック直後(100ms)');

// 少し待ってから
await page.waitForTimeout(400);
await getGridInfo('バブル出現後(500ms)');

await page.waitForTimeout(1000);
await getGridInfo('安定後(1500ms)');

// バブルの挿入位置確認
const bubbleCheck = await page.evaluate(() => {
  const grid = document.getElementById('want-grid-container');
  if (!grid) return null;
  const children = Array.from(grid.children);
  return children.map((el, i) => {
    const isBubble = el.classList.contains('col-span-full') || el.getAttribute('class')?.includes('col-span-full');
    const wantId = el.getAttribute('data-want-id');
    const rect = el.getBoundingClientRect();
    return { i, isBubble, wantId: wantId?.slice(0,8), x: Math.round(rect.x), y: Math.round(rect.y), w: Math.round(rect.width) };
  });
});
console.log('\n=== グリッド子要素の並び ===');
bubbleCheck.forEach(c => {
  const label = c.isBubble ? '[BUBBLE]' : `[card ${c.wantId}]`;
  console.log(`  [${c.i}] ${label}: x=${c.x} y=${c.y} w=${c.w}`);
});

await page.screenshot({ path: '/tmp/bubble_check3.png' });
await page.waitForTimeout(3000);
await browser.close();
