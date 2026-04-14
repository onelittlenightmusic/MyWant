import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
// 3列になる幅に設定 (384*3 + gap*2 + padding = ~1200px+)
await page.setViewportSize({ width: 1800, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

const getState = async (label) => {
  const data = await page.evaluate(() => {
    const grid = document.getElementById('want-grid-container');
    const cols = window.getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean);
    const children = Array.from(grid.children).map((el, i) => {
      const r = el.getBoundingClientRect();
      const isBubble = el.className?.includes('col-span-full');
      return { i, isBubble, id: el.getAttribute('data-want-id')?.slice(0,8), x: Math.round(r.x), y: Math.round(r.y) };
    });
    const caret = document.querySelector('.col-span-full .absolute.overflow-hidden');
    const caretR = caret?.getBoundingClientRect();
    return {
      colCount: cols.length,
      tracks: cols.join(', '),
      children,
      caretCenterX: caretR ? Math.round(caretR.x + caretR.width/2) : null,
      caretLeft: caret?.style?.left,
    };
  });
  console.log(`\n=== [${label}] ${data.colCount}列 (${data.tracks}) ===`);
  data.children.forEach(c =>
    console.log(`  [${c.i}] ${c.isBubble ? 'BUBBLE' : `card(${c.id})`}: x=${c.x} y=${c.y}`)
  );
  if (data.caretCenterX !== null) {
    console.log(`  caret center X: ${data.caretCenterX}  (left style: "${data.caretLeft}")`);
  }
  return data;
};

await getState('初期 3列');

// 各カード位置を記録
const cards = await page.$$('[data-want-id]');
const rects = [];
for (const c of cards) {
  rects.push(await c.boundingBox());
}

// 各列のカードをテスト
for (const [cardIdx, colLabel] of [[0,'1列目'], [1,'2列目'], [2,'3列目']]) {
  if (cardIdx >= cards.length) break;
  const r = rects[cardIdx];
  const parentCX = Math.round(r.x + r.width / 2);
  
  const freshCards = await page.$$('[data-want-id]');
  await freshCards[cardIdx].click();
  await page.waitForTimeout(800);
  
  const after = await getState(`${colLabel}クリック後`);
  const bubbleIdx = after.children.findIndex(c => c.isBubble);
  
  if (after.caretCenterX !== null) {
    console.log(`  → 親カード中心X=${parentCX}  キャレット中心X=${after.caretCenterX}  ズレ=${after.caretCenterX - parentCX}px`);
    console.log(`  → バブル挿入位置: [${bubbleIdx}] (直前: ${after.children[bubbleIdx-1]?.id})`);
  } else {
    console.log(`  → バブルなし（子がない）`);
  }
  
  await page.keyboard.press('Escape');
  await page.waitForTimeout(500);
}

await browser.close();
