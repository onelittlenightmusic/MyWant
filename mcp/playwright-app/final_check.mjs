import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

const test = async (cardIndex, label) => {
  // 初期位置取得
  const cards = await page.$$('[data-want-id]');
  const cardRect = await cards[cardIndex].boundingBox();
  const parentCenterX = Math.round(cardRect.x + cardRect.width / 2);

  await cards[cardIndex].click();
  await page.waitForTimeout(800);

  const result = await page.evaluate(() => {
    const grid = document.getElementById('want-grid-container');
    const bubble = document.querySelector('.col-span-full');
    const caret = bubble?.querySelector('.absolute.overflow-hidden');
    const caretRect = caret?.getBoundingClientRect();
    const gridCols = window.getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean).length;
    const children = Array.from(grid.children).map((el, i) => {
      const r = el.getBoundingClientRect();
      return { i, isBubble: el.className?.includes('col-span-full'), x: Math.round(r.x), y: Math.round(r.y), id: el.getAttribute('data-want-id')?.slice(0,8) };
    });
    return {
      gridCols,
      caretCenterX: caretRect ? Math.round(caretRect.x + caretRect.width/2) : null,
      caretLeft: caret?.style?.left,
      bubbleExists: !!bubble,
      children,
    };
  });

  console.log(`\n=== ${label} ===`);
  console.log(`  親カード中心X: ${parentCenterX}  キャレット中心X: ${result.caretCenterX}  ズレ: ${result.caretCenterX - parentCenterX}px`);
  console.log(`  caretLeft style: "${result.caretLeft}"  grid=${result.gridCols}列`);
  console.log('  子要素並び:');
  result.children.forEach(c => {
    console.log(`    [${c.i}] ${c.isBubble ? 'BUBBLE' : `card(${c.id})`}: x=${c.x} y=${c.y}`);
  });

  // Escape で閉じる
  await page.keyboard.press('Escape');
  await page.waitForTimeout(500);
};

await test(0, '1列目クリック (index=0)');
await test(1, '2列目クリック (index=1, level-1-approval)');

await browser.close();
