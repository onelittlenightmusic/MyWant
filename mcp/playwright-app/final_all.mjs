import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();

const testLayout = async (width, label) => {
  await page.setViewportSize({ width, height: 900 });
  await page.goto('http://localhost:8080');
  await page.waitForTimeout(1500);

  const initial = await page.evaluate(() => {
    const grid = document.getElementById('want-grid-container');
    const tracks = window.getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean);
    return { cols: tracks.length, cards: document.querySelectorAll('[data-want-id]').length };
  });
  console.log(`\n━━━ ${label} (${initial.cols}列, ${initial.cards}cards) ━━━`);

  const cards = await page.$$('[data-want-id]');
  for (let idx = 0; idx < Math.min(initial.cols, cards.length); idx++) {
    const box = await cards[idx].boundingBox();
    const parentCX = Math.round(box.x + box.width / 2);
    
    const freshCards = await page.$$('[data-want-id]');
    await freshCards[idx].click();
    await page.waitForTimeout(700);

    const result = await page.evaluate(() => {
      const grid = document.getElementById('want-grid-container');
      const children = Array.from(grid.children).map((el, i) => {
        const r = el.getBoundingClientRect();
        return { i, isBubble: !!el.className?.includes('col-span-full'), id: el.getAttribute('data-want-id')?.slice(0,8), x: Math.round(r.x), y: Math.round(r.y) };
      });
      const caret = document.querySelector('.col-span-full .absolute.overflow-hidden');
      const cr = caret?.getBoundingClientRect();
      return { children, caretCX: cr ? Math.round(cr.x + cr.width/2) : null };
    });

    const bubbleIdx = result.children.findIndex(c => c.isBubble);
    if (bubbleIdx === -1) {
      console.log(`  col${idx+1}: バブルなし（子がない）`);
    } else {
      const beforeBubble = result.children.slice(0, bubbleIdx);
      const row1Cards = beforeBubble.filter(c => !c.isBubble);
      const caretOk = result.caretCX === parentCX;
      const layoutOk = row1Cards.every((c, i) => i === 0 || result.children[i].y === result.children[0].y);
      console.log(`  col${idx+1}: バブル前カード数=${row1Cards.length}/${initial.cols} キャレットズレ=${result.caretCX-parentCX}px ${caretOk ? '✓' : '✗'} レイアウト=${layoutOk ? 'OK ✓' : 'NG ✗'}`);
      row1Cards.forEach(c => process.stdout.write(`    [${c.i}]${c.id} x=${c.x} y=${c.y}\n`));
      const afterBubble = result.children.slice(bubbleIdx + 1).filter(c => !c.isBubble);
      if (afterBubble.length) {
        process.stdout.write(`    BUBBLE → `);
        afterBubble.forEach(c => process.stdout.write(`[${c.i}]${c.id}(${c.x},${c.y}) `));
        process.stdout.write('\n');
      }
    }

    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);
  }
};

await testLayout(1400, '2列グリッド');
await testLayout(1800, '3列グリッド');

await browser.close();
