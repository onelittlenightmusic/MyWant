import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();

// Console ログを記録
const logs = [];
page.on('console', msg => logs.push(`[${msg.type()}] ${msg.text()}`));

await page.setViewportSize({ width: 1800, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// 2列目カードをクリック
const cards = await page.$$('[data-want-id]');
await cards[1].click();
await page.waitForTimeout(1000);

// 詳細な状態確認
const result = await page.evaluate(() => {
  const grid = document.getElementById('want-grid-container');
  const tracks = window.getComputedStyle(grid).gridTemplateColumns;
  const actualCols = Math.max(1, tracks.split(' ').filter(Boolean).length);
  
  // グリッド内の子要素の位置
  const children = Array.from(grid.children).map((el, i) => {
    const r = el.getBoundingClientRect();
    const isBubble = el.className?.includes('col-span-full');
    return { i, isBubble, id: el.getAttribute('data-want-id')?.slice(0,8), x: Math.round(r.x), y: Math.round(r.y) };
  });
  
  // バブルが何番目の後ろにある？
  const bubbleChildIdx = children.findIndex(c => c.isBubble);
  const cardBeforeBubble = bubbleChildIdx > 0 ? children[bubbleChildIdx - 1] : null;
  
  return { actualCols, tracks, children, bubbleChildIdx, cardBeforeBubble };
});

console.log(`Grid tracks: "${result.tracks}" → ${result.actualCols}列`);
console.log(`Bubble child index in grid DOM: ${result.bubbleChildIdx}`);
console.log(`Card before bubble: ${JSON.stringify(result.cardBeforeBubble)}`);
console.log('\nAll children:');
result.children.forEach(c => console.log(`  [${c.i}] ${c.isBubble ? 'BUBBLE' : `card(${c.id})`}: x=${c.x} y=${c.y}`));

// filteredWants の index 1 は何？
const allCards = await page.$$('[data-want-id]');
console.log(`\nTotal card elements found: ${allCards.length}`);
for (let i = 0; i < Math.min(3, allCards.length); i++) {
  const id = await allCards[i].getAttribute('data-want-id');
  const box = await allCards[i].boundingBox();
  console.log(`  allCards[${i}]: ${id?.slice(0,8)} x=${Math.round(box.x)} y=${Math.round(box.y)}`);
}

console.log('\nConsole logs from page:');
logs.forEach(l => console.log(' ', l));

await browser.close();
