import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: false, slowMo: 150 });
const page = await browser.newPage();
await page.setViewportSize({ width: 1800, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(1500);

const getLayout = async (label) => {
  const data = await page.evaluate(() => {
    const grid = document.getElementById('want-grid-container');
    const tracks = window.getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean);
    const children = Array.from(grid.children).map((el, i) => {
      const r = el.getBoundingClientRect();
      const isBubble = el.className?.includes('col-span-full');
      const wantId = el.getAttribute('data-want-id');
      const isBtn = el.tagName === 'BUTTON';
      const type = isBubble ? 'BUBBLE' : isBtn ? 'ADD_WANT' : wantId ? `card(${wantId.slice(0,8)})` : `other`;
      return { i, type, x: Math.round(r.x), y: Math.round(r.y) };
    });
    return { cols: tracks.length, children };
  });
  console.log(`\n=== [${label}] ${data.cols}列 ===`);
  // グループ化して表示
  const byRow = {};
  data.children.forEach(c => {
    if (!byRow[c.y]) byRow[c.y] = [];
    byRow[c.y].push(c);
  });
  Object.entries(byRow).sort(([a],[b]) => +a - +b).forEach(([y, items]) => {
    console.log(`  y=${y}: ${items.map(c => c.type).join(', ')}`);
  });
  return data;
};

const initial = await getLayout('3列・バブル前');
const filteredCount = initial.children.filter(c => c.type.startsWith('card')).length;
console.log(`\n→ filteredWants数: ${filteredCount}`);

// 最後の行のカード（＝ADD_WANTと同じ行にある可能性のあるカード）をクリック
// 3列で4 wantsなら: row1=[fw0,fw1,fw2], row2=[fw3,ADD_WANT] → fw3をクリック
const cards = await page.$$('[data-want-id]');
const lastCardIdx = cards.length - 1;
console.log(`\n→ 最後のwant card (index=${lastCardIdx}) をクリック`);

const lastRect = await cards[lastCardIdx].boundingBox();
const parentCX = Math.round(lastRect.x + lastRect.width / 2);
await cards[lastCardIdx].click();
await page.waitForTimeout(800);
await getLayout(`最後のカード(col=${Math.round(lastRect.x / 430) + 1})クリック後`);

await page.screenshot({ path: '/tmp/addwant2.png' });
await page.waitForTimeout(4000);
await browser.close();
