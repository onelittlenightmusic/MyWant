import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);
await page.click('[data-header-btn-id="list"]');
await page.waitForTimeout(600);

const getFocal = () => page.evaluate(() => {
  const cv = document.querySelector('[data-want-canvas]');
  if (!cv) return null;
  let el = cv.parentElement;
  while (el && el !== document.body) {
    const s = window.getComputedStyle(el);
    if (s.overflow === 'auto' || s.overflowX === 'auto' || s.overflowY === 'auto') break;
    el = el.parentElement;
  }
  if (!el || el === document.body) return null;
  const m = cv.style.transform?.match(/translate3d\(([^,]+)px,\s*([^,]+)px[^)]*\)\s*scale\(([^)]+)\)/);
  const osx = m ? parseFloat(m[1]) : 0, osy = m ? parseFloat(m[2]) : 0, s = m ? parseFloat(m[3]) : 1;
  const fpx = el.clientWidth / 2, fpy = el.clientHeight / 2;
  return { cx: Math.round((el.scrollLeft - osx + fpx) / s), cy: Math.round((el.scrollTop - osy + fpy) / s), scale: Math.round(s * 100) / 100 };
});

const getBtn = async (txt) => {
  for (const h of await page.$$('button')) { if ((await h.innerText().catch(() => '')).trim() === txt) return h; }
  return null;
};
const zoomIn = await getBtn('+'), zoomOut = await getBtn('−');  // note: − is U+2212
if (!zoomIn || !zoomOut) { console.log('❌ buttons not found'); await browser.close(); process.exit(1); }

// Zoom in 5× to get to a good starting position
for (let i=0; i<5; i++) { await zoomIn.click(); await page.waitForTimeout(220); }
const f0 = await getFocal();
console.log('Reference focal after zoom-in:', f0);

let maxDrift = 0;
for (let i=1; i<=5; i++) {
  await zoomOut.click();
  await page.waitForTimeout(250);
  const fi = await getFocal();
  const d = fi && f0 ? Math.round(Math.hypot(fi.cx - f0.cx, fi.cy - f0.cy)) : -1;
  maxDrift = Math.max(maxDrift, d);
  console.log(`ZoomOut ${i}: scale=${fi?.scale}  focal=(${fi?.cx},${fi?.cy})  drift=${d}px`);
}
console.log(`\nMax drift: ${maxDrift}px  ${maxDrift <= 5 ? '✅ PASS' : '❌ FAIL'}`);
await browser.close();
process.exit(maxDrift <= 5 ? 0 : 1);
