import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: false, slowMo: 200 });
const page = await browser.newPage();

const getGrid = async (label) => {
  const data = await page.evaluate(() => {
    const grid = document.getElementById('want-grid-container');
    const tracks = window.getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean);
    const children = Array.from(grid.children).map((el, i) => {
      const r = el.getBoundingClientRect();
      const isBubble = el.className?.includes('col-span-full');
      const wantId = el.getAttribute('data-want-id');
      const isAddWant = !isBubble && !wantId && el.tagName === 'BUTTON';
      const isDraft = !isBubble && !wantId && el.tagName === 'DIV';
      const type = isBubble ? 'BUBBLE' : isAddWant ? 'ADD_WANT' : wantId ? `card(${wantId.slice(0,8)})` : `div(${el.className.slice(0,20)})`;
      return { i, type, x: Math.round(r.x), y: Math.round(r.y), h: Math.round(r.height) };
    });
    return { cols: tracks.length, children };
  });
  console.log(`\n=== [${label}] ${data.cols}列 ===`);
  data.children.forEach(c => console.log(`  [${c.i}] ${c.type}: x=${c.x} y=${c.y}`));
  return data;
};

// 2列テスト
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(1500);

const before2 = await getGrid('2列・バブル前');

// 1列目をクリック (index=0)
const cards = await page.$$('[data-want-id]');
await cards[0].click();
await page.waitForTimeout(800);
await getGrid('2列・1列目クリック後');
await page.keyboard.press('Escape');
await page.waitForTimeout(400);

// 2列目をクリック (index=1)
const cards2 = await page.$$('[data-want-id]');
await cards2[1].click();
await page.waitForTimeout(800);
await getGrid('2列・2列目クリック後');

await page.screenshot({ path: '/tmp/addwant_check.png' });
await page.waitForTimeout(3000);
await browser.close();
