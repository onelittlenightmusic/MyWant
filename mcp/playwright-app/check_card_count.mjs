import { chromium } from 'playwright';
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);
await page.keyboard.press('Escape');
await page.waitForTimeout(300);

const cards = await page.evaluate(() => {
  return Array.from(document.querySelectorAll('[data-keyboard-nav-id]')).map(c => c.getAttribute('data-keyboard-nav-id')?.slice(-8));
});
console.log('Total cards:', cards.length, cards);
await browser.close();
