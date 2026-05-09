import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// WANT ボタンをクリック
const wantBtn = await page.$('[data-header-btn-id="add-want"]')
  || await page.$('button[title*="WANT"]')
  || await page.$('button span:text("WANT")');
if (wantBtn) await wantBtn.click();
else await page.click('text=WANT');
await page.waitForTimeout(1000);
await page.screenshot({ path: '/tmp/step1_form.png' });

// WantInventoryPicker で最初のタイプをクリック
const typeBtn = await page.$('[data-testid="want-type-item"], .want-inventory-item, [class*="inventory"] button, [class*="WantType"] button');
if (typeBtn) {
  await typeBtn.click();
  console.log('タイプ選択: クリック成功');
} else {
  // フォーム内の最初のクリッカブルなアイテムを探す
  const items = await page.$$('[role="button"], button[class*="aspect"]');
  if (items.length > 0) {
    await items[0].click();
    console.log(`タイプ選択: ${items.length}件中1件をクリック`);
  }
}
await page.waitForTimeout(800);
await page.screenshot({ path: '/tmp/step2_type_selected.png' });

// focusable-section-header の確認
const sections = await page.evaluate(() =>
  Array.from(document.querySelectorAll('.focusable-section-header')).map(el => ({
    text: el.textContent?.trim().slice(0, 30),
    visible: el.getBoundingClientRect().width > 0,
  }))
);
console.log('section headers:', JSON.stringify(sections));

if (sections.length === 0) {
  console.log('セクションヘッダーが見つかりません。スクリーンショットを確認してください。');
  await browser.close();
  process.exit(0);
}

// Params セクションに手動フォーカス
const paramsHeader = await page.$('.focusable-section-header');
if (paramsHeader) await paramsHeader.focus();
await page.waitForTimeout(200);

console.log('\n=== Tab キー 6 回巡回（共通ハンドラ経由のセクションレベル移動確認）===');
for (let i = 0; i < 6; i++) {
  await page.keyboard.press('Tab');
  await page.waitForTimeout(150);
  const focused = await page.evaluate(() => {
    const el = document.activeElement;
    if (!el) return 'none';
    const tag = el.tagName;
    const cls = el.className.slice(0, 60);
    const text = el.textContent?.trim().slice(0, 25) || el.getAttribute('placeholder') || '';
    const isSectionHeader = el.classList.contains('focusable-section-header');
    return `${tag}${isSectionHeader ? '[SECTION]' : ''} "${text}"  cls="${cls}"`;
  });
  console.log(`  Tab[${i+1}]: ${focused}`);
}

await page.screenshot({ path: '/tmp/step3_tab_nav.png' });

// Shift+Tab で逆順
console.log('\n=== Shift+Tab キー 3 回逆巡回 ===');
for (let i = 0; i < 3; i++) {
  await page.keyboard.press('Shift+Tab');
  await page.waitForTimeout(150);
  const focused = await page.evaluate(() => {
    const el = document.activeElement;
    if (!el) return 'none';
    const isSectionHeader = el.classList.contains('focusable-section-header');
    return `${el.tagName}${isSectionHeader ? '[SECTION]' : ''} "${el.textContent?.trim().slice(0, 25) || el.getAttribute('placeholder') || ''}"`;
  });
  console.log(`  Shift+Tab[${i+1}]: ${focused}`);
}

await page.waitForTimeout(2000);
await browser.close();
