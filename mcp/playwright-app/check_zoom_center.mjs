/**
 * Measure focal-point drift during zoom (button + R-stick).
 * Records the canvas-space coordinate under the viewport center before and after zoom.
 */
import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

// Switch to canvas mode via the List/Canvas toggle button
await page.click('[data-header-btn-id="list"]');
await page.waitForTimeout(600);

// Verify canvas is present
const cvOk = await page.evaluate(() => !!document.querySelector('[data-want-canvas]'));
if (!cvOk) { console.log('❌ Canvas not found after toggle'); await browser.close(); process.exit(1); }

// Get focal point: canvas-space coordinate currently at viewport center
const getFocal = () => page.evaluate(() => {
  const cv = document.querySelector('[data-want-canvas]');
  if (!cv) return null;
  // Find scroll container: first ancestor with scroll
  let el = cv.parentElement;
  while (el && el !== document.body) {
    const s = window.getComputedStyle(el);
    if (s.overflow === 'auto' || s.overflowX === 'auto' || s.overflowY === 'auto' ||
        s.overflow === 'scroll' || s.overflowX === 'scroll' || s.overflowY === 'scroll') break;
    el = el.parentElement;
  }
  if (!el || el === document.body) return null;

  const m = cv.style.transform?.match(/translate3d\(([^,]+)px,\s*([^,]+)px[^)]*\)\s*scale\(([^)]+)\)/);
  const osx = m ? parseFloat(m[1]) : 0;
  const osy = m ? parseFloat(m[2]) : 0;
  const s   = m ? parseFloat(m[3]) : 1;

  const fpx = el.clientWidth  / 2;
  const fpy = el.clientHeight / 2;
  return {
    cx: Math.round((el.scrollLeft - osx + fpx) / s),
    cy: Math.round((el.scrollTop  - osy + fpy) / s),
    scale: Math.round(s * 100) / 100,
    scrollLeft: Math.round(el.scrollLeft),
    scrollTop:  Math.round(el.scrollTop),
  };
});

// Find zoom-in button (the '+' in the canvas toolbar)
const getZoomInBtn = async () => {
  const handles = await page.$$('button');
  for (const h of handles) {
    const txt = (await h.innerText().catch(() => '')).trim();
    if (txt === '+') return h;
  }
  return null;
};

console.log('\n══ Initial ══');
const f0 = await getFocal();
console.log(f0);

const zoomIn = await getZoomInBtn();
if (!zoomIn) { console.log('❌ Zoom + button not found'); await browser.close(); process.exit(1); }

const results = [];
for (let i = 1; i <= 4; i++) {
  await zoomIn.click();
  await page.waitForTimeout(300);
  const fi = await getFocal();
  const drift = fi && f0 ? Math.round(Math.hypot(fi.cx - f0.cx, fi.cy - f0.cy)) : -1;
  results.push({ step: i, ...fi, drift });
  console.log(`Zoom ${i}: scale=${fi?.scale}  focal=(${fi?.cx}, ${fi?.cy})  drift=${drift}px`);
  await page.screenshot({ path: `/tmp/zoom_step${i}.png` });
}

const maxDrift = Math.max(...results.map(r => r.drift));
console.log(`\nMax drift: ${maxDrift}px`);
console.log(maxDrift <= 5 ? '✅ PASS' : `❌ FAIL (${maxDrift}px > 5px threshold)`);

await browser.close();
process.exit(maxDrift <= 5 ? 0 : 1);

// This runs after the zoom-in test — also test zoom-out
