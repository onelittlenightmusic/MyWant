import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(3000);

// Step 1: Open the Add Want sidebar
console.log('Step 1: Clicking "Add Want" button...');
await page.click('button:has-text("Add Want")');
await page.waitForTimeout(2000);

// Step 2: Click the first want type card in the inventory picker
console.log('Step 2: Clicking first want type card...');
await page.mouse.click(971, 114);
await page.waitForTimeout(2000);

// Gather info about tabs - look for icons and classes
const tabInfo = await page.evaluate(() => {
  // Find all buttons in the sidebar area
  const allButtons = Array.from(document.querySelectorAll('button'));
  const tabButtons = allButtons.filter(b => {
    const r = b.getBoundingClientRect();
    if (r.height <= 0 || r.height >= 60 || r.x < 800) return false;
    const text = b.textContent?.trim() || '';
    // Match tab labels that may include icons
    return ['Name', 'Params', 'Labels', 'Schedule', 'Deps'].some(label => text.includes(label));
  });

  return tabButtons.map(b => {
    const r = b.getBoundingClientRect();
    const svgs = b.querySelectorAll('svg');
    const computedStyle = window.getComputedStyle(b);
    return {
      text: b.textContent?.trim(),
      innerHTML: b.innerHTML.slice(0, 300),
      cls: (typeof b.className === 'string' ? b.className : String(b.className)),
      hasSvg: svgs.length > 0,
      svgCount: svgs.length,
      rect: { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) },
      borderBottom: computedStyle.borderBottom,
      backgroundColor: computedStyle.backgroundColor,
      color: computedStyle.color,
    };
  });
});

console.log('Tab details:');
tabInfo.forEach((t, i) => {
  console.log(`\nTab ${i}: "${t.text}"`);
  console.log(`  rect: x=${t.rect.x} y=${t.rect.y} w=${t.rect.w} h=${t.rect.h}`);
  console.log(`  hasSvg: ${t.hasSvg} (svgCount: ${t.svgCount})`);
  console.log(`  cls: ${t.cls.slice(0, 150)}`);
  console.log(`  borderBottom: ${t.borderBottom}`);
  console.log(`  bg: ${t.backgroundColor}`);
  console.log(`  color: ${t.color}`);
  console.log(`  innerHTML: ${t.innerHTML.slice(0, 200)}`);
});

// Find tab bar Y position
const tabY = tabInfo.length > 0 ? tabInfo[0].rect.y : 70;
const sidebarX = tabInfo.length > 0 ? Math.max(0, tabInfo[0].rect.x - 10) : 900;
console.log(`\nTab bar Y: ${tabY}, Sidebar X: ${sidebarX}`);

// Screenshot: full sidebar
await page.screenshot({
  path: '/tmp/formtab_new.png',
  clip: { x: sidebarX, y: 0, width: 1400 - sidebarX, height: 900 },
});
console.log('Screenshot saved to /tmp/formtab_new.png');

// Also check WantDetailsSidebar tabs for comparison
// Click on an existing want card if any
const existingCards = await page.evaluate(() => {
  const cards = Array.from(document.querySelectorAll('[data-want-id], .want-card, [class*="WantCard"], [class*="want-card"]'));
  return cards.slice(0, 3).map(c => {
    const r = c.getBoundingClientRect();
    return { tag: c.tagName, cls: c.className.toString().slice(0, 80), rect: { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) } };
  });
});
console.log('\nExisting want cards:', JSON.stringify(existingCards, null, 2));

await browser.close();
console.log('\nDone.');
