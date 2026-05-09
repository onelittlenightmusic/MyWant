/**
 * Sample focal point every ~16ms DURING the zoom animation to catch
 * mid-animation drift, not just final position.
 */
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

// Inject a sampler that polls focal point every frame during zoom
await page.evaluate(() => {
  window.__zoomSamples = [];
  window.__sampleFocal = () => {
    const cv = document.querySelector('[data-want-canvas]');
    if (!cv) return null;
    let el = cv.parentElement;
    while (el && el !== document.body) {
      const s = getComputedStyle(el);
      if (s.overflow === 'auto' || s.overflowX === 'auto' || s.overflowY === 'auto') break;
      el = el.parentElement;
    }
    if (!el || el === document.body) return null;
    const m = cv.style.transform?.match(/translate3d\(([^,]+)px,\s*([^,]+)px[^)]*\)\s*scale\(([^)]+)\)/);
    const osx = m ? parseFloat(m[1]) : 0, osy = m ? parseFloat(m[2]) : 0, s2 = m ? parseFloat(m[3]) : 1;
    const fpx = el.clientWidth / 2, fpy = el.clientHeight / 2;
    return {
      t: performance.now(),
      cx: (el.scrollLeft - osx + fpx) / s2,
      cy: (el.scrollTop  - osy + fpy) / s2,
      scale: s2,
      sl: el.scrollLeft,
    };
  };

  // Also track what the CSS computed transform says (separate from inline style)
  window.__sampleComputed = () => {
    const cv = document.querySelector('[data-want-canvas]');
    if (!cv) return null;
    let el = cv.parentElement;
    while (el && el !== document.body) {
      const s = getComputedStyle(el);
      if (s.overflow === 'auto' || s.overflowX === 'auto' || s.overflowY === 'auto') break;
      el = el.parentElement;
    }
    if (!el || el === document.body) return null;
    // Get the computed transform matrix to read ACTUAL painted scale
    const computed = getComputedStyle(cv).transform;
    let scaleComputed = 1;
    if (computed && computed !== 'none') {
      const vals = computed.match(/matrix\(([^)]+)\)/);
      if (vals) { const parts = vals[1].split(','); scaleComputed = parseFloat(parts[0]); }
    }
    const m = cv.style.transform?.match(/translate3d\(([^,]+)px,\s*([^,]+)px[^)]*\)\s*scale\(([^)]+)\)/);
    const osx = m ? parseFloat(m[1]) : 0, osy = m ? parseFloat(m[2]) : 0;
    const fpx = el.clientWidth / 2, fpy = el.clientHeight / 2;
    return {
      t: performance.now(),
      cx_inline: (el.scrollLeft - osx + fpx) / (m ? parseFloat(m[3]) : 1),
      cx_computed: (el.scrollLeft - osx + fpx) / scaleComputed,
      scaleInline: m ? parseFloat(m[3]) : 1,
      scaleComputed,
      sl: el.scrollLeft,
    };
  };
});

// Continuous sampler running during zoom animation
const startSampling = () => page.evaluate(() => {
  window.__zoomSamples = [];
  let handle;
  const loop = () => {
    const s = window.__sampleComputed();
    if (s) window.__zoomSamples.push(s);
    handle = requestAnimationFrame(loop);
  };
  handle = requestAnimationFrame(loop);
  window.__stopSampling = () => cancelAnimationFrame(handle);
});

const stopSampling = () => page.evaluate(() => {
  window.__stopSampling?.();
  return window.__zoomSamples;
});

const getBtn = async (txt) => {
  for (const h of await page.$$('button')) { if ((await h.innerText().catch(() => '')).trim() === txt) return h; }
  return null;
};
const zoomIn = await getBtn('+');
if (!zoomIn) { console.log('❌ + button not found'); await browser.close(); process.exit(1); }

// Get baseline focal point
const baseline = await page.evaluate(() => window.__sampleFocal?.());
console.log('Baseline focal:', baseline ? `(${baseline.cx.toFixed(1)}, ${baseline.cy.toFixed(1)}) scale=${baseline.scale}` : 'null');

// Sample during zoom animation
await startSampling();
await zoomIn.click();
await page.waitForTimeout(350); // wait for animation + a bit more
const samples = await stopSampling();

console.log(`\nSampled ${samples.length} frames during zoom`);

if (samples.length > 0 && baseline) {
  const refCx = baseline.cx, refCy = baseline.cy;
  let maxDrift = 0, maxDriftIdx = 0;
  
  console.log('\nFrame-by-frame (scale, computed_scale, cx_inline, cx_computed, drift_inline, drift_computed):');
  samples.forEach((s, i) => {
    const driftInline   = Math.abs(s.cx_inline   - refCx);
    const driftComputed = Math.abs(s.cx_computed - refCx);
    if (driftComputed > maxDrift) { maxDrift = driftComputed; maxDriftIdx = i; }
    if (i % 2 === 0 || driftComputed > 5) { // print every 2nd frame or large drift
      console.log(`  [${i.toString().padStart(3)}] t=${((s.t - samples[0].t)|0).toString().padStart(4)}ms  inline=${s.scaleInline.toFixed(3)}  computed=${s.scaleComputed.toFixed(3)}  cx_inline=${s.cx_inline.toFixed(1)}  cx_computed=${s.cx_computed.toFixed(1)}  drift=${driftComputed.toFixed(1)}px`);
    }
  });
  
  console.log(`\nMax drift (computed scale): ${maxDrift.toFixed(1)}px at frame ${maxDriftIdx}`);
  const passed = maxDrift <= 5;
  console.log(passed ? '✅ PASS' : `❌ FAIL (${maxDrift.toFixed(1)}px > 5px threshold)`);
}

await browser.close();
