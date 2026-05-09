import { chromium } from 'playwright';

const browser = await chromium.launch({ headless: false });
const page = await browser.newPage();
await page.setViewportSize({ width: 1400, height: 900 });
await page.goto('http://localhost:8080');
await page.waitForTimeout(2000);

// Canvas モードへ切り替え
const isAlreadyCanvas = await page.evaluate(() => !!document.querySelector('[data-want-canvas="true"]'));
if (!isAlreadyCanvas) {
  await page.click('[data-header-btn-id="list"]');
  await page.waitForTimeout(1000);
}

// choice want を取得してタイルクリック
const wantsResp = await page.evaluate(async () => (await fetch('/api/v1/wants')).json());
const wants = Array.isArray(wantsResp) ? wantsResp : (wantsResp.wants || []);
const choiceWant = wants.find(w => (w.metadata?.type || '').toLowerCase() === 'choice');
const targetId = choiceWant.metadata?.id || choiceWant.id;

const tile = await page.$(`[data-want-id="${targetId}"]`);
const tileBox = await tile.boundingBox();
await page.mouse.click(tileBox.x + tileBox.width / 2, tileBox.y + tileBox.height / 2);
await page.waitForSelector('[data-keyboard-nav-selected="true"]', { timeout: 5000 });
await page.waitForTimeout(300);

// ── ドロップダウン閉じた状態の詳細計測 ──
const closed = await page.evaluate(() => {
  const card = document.querySelector('[data-keyboard-nav-selected="true"]');
  const cardR = card.getBoundingClientRect();

  // カード内の全子要素を再帰的に走査してコンテンツ領域を把握
  const contentArea = card.querySelector('.relative.z-10.transition-all.duration-150.flex-1');
  const contentR = contentArea?.getBoundingClientRect();

  // WantCardContent 内の各セクション
  // ヘッダー部分（カード上部のタイプ名・ステータスなど）
  const typeLabel = card.querySelector('[class*="text-xs"][class*="font-medium"]');
  const typeLabelR = typeLabel?.getBoundingClientRect();

  // Choice セクションの外側ラッパー (mt-2 or mt-4 space-y-1)
  const choiceWrapper = card.querySelector('.mt-4.space-y-1, .mt-2.space-y-1');
  const choiceWrapperR = choiceWrapper?.getBoundingClientRect();

  // ラベル行 ("selected_slot" テキスト)
  const labelRow = choiceWrapper?.querySelector('.flex.items-center.justify-between');
  const labelRowR = labelRow?.getBoundingClientRect();

  // ドロップダウンボタン
  const btn = card.querySelector('button[type="button"].w-full');
  const btnR = btn?.getBoundingClientRect();

  return {
    card:         { top: Math.round(cardR.top), bottom: Math.round(cardR.bottom), h: Math.round(cardR.height) },
    contentArea:  contentR ? { top: Math.round(contentR.top), h: Math.round(contentR.height) } : null,
    typeLabel:    typeLabelR ? { top: Math.round(typeLabelR.top), h: Math.round(typeLabelR.height) } : null,
    choiceWrapper:choiceWrapperR ? { top: Math.round(choiceWrapperR.top), bottom: Math.round(choiceWrapperR.bottom), h: Math.round(choiceWrapperR.height) } : null,
    labelRow:     labelRowR ? { h: Math.round(labelRowR.height) } : null,
    btn:          btnR ? { top: Math.round(btnR.top), bottom: Math.round(btnR.bottom), h: Math.round(btnR.height) } : null,
    spaceUnderBtn: btnR ? Math.round(cardR.bottom - btnR.bottom) : null,
    headerH: (choiceWrapperR && cardR) ? Math.round(choiceWrapperR.top - cardR.top) : null,
  };
});

console.log('=== ドロップダウン閉じた状態 ===');
console.log(`カード全体:       h=${closed.card.h}px  (top=${closed.card.top} bottom=${closed.card.bottom})`);
console.log(`コンテンツエリア: h=${closed.contentArea?.h}px`);
console.log(`ヘッダー部分:     h=${closed.headerH}px  (カードtopからChoiceWrapper上端まで)`);
console.log(`Choiceラッパー:   h=${closed.choiceWrapper?.h}px`);
console.log(`  ラベル行:       h=${closed.labelRow?.h}px`);
console.log(`  ボタン:         h=${closed.btn?.h}px`);
console.log(`ボタン下の余白:   ${closed.spaceUnderBtn}px  ← ここが未使用領域`);

// ── ドロップダウンを開く ──
const dropBtn = await page.$('[data-keyboard-nav-selected="true"] button[type="button"].w-full');
await dropBtn.click();
await page.waitForTimeout(400);

// ── ドロップダウン開いた状態の詳細計測 ──
const opened = await page.evaluate(() => {
  const card = document.querySelector('[data-keyboard-nav-selected="true"]');
  const cardR = card.getBoundingClientRect();

  const btn = card.querySelector('button[type="button"].w-full');
  const btnR = btn?.getBoundingClientRect();

  // overflow-hidden の影響確認
  const styles = window.getComputedStyle(card);

  // ドロップダウンリスト本体
  const dropList = card.querySelector('.max-h-40.overflow-y-auto');
  const dropListR = dropList?.getBoundingClientRect();

  // ドロップダウンリストの可視領域（カード内に収まっている部分）
  const visibleTop    = dropListR ? Math.max(dropListR.top, cardR.top) : null;
  const visibleBottom = dropListR ? Math.min(dropListR.bottom, cardR.bottom) : null;
  const visibleH      = (visibleTop !== null && visibleBottom !== null) ? Math.max(0, visibleBottom - visibleTop) : null;

  // 理想のカード高さ: ヘッダー + ラベル + ボタン + ドロップダウンmax-h-40
  const maxH40px = 160; // max-h-40 = 10rem = 160px
  const headerH = btnR ? Math.round(btnR.top - cardR.top) : 0;
  const idealH = headerH + (btnR ? Math.round(btnR.height) : 0) + maxH40px + 8; // +8 for margin

  return {
    card: { h: Math.round(cardR.height), overflow: styles.overflow },
    btn:  btnR ? { bottom: Math.round(btnR.bottom), h: Math.round(btnR.height) } : null,
    dropList: dropListR ? {
      top:    Math.round(dropListR.top),
      bottom: Math.round(dropListR.bottom),
      h:      Math.round(dropListR.height),
    } : null,
    visibleH,
    clippedH: dropListR ? Math.round(dropListR.height - (visibleH ?? 0)) : null,
    idealCardH: idealH,
    headerH,
  };
});

console.log('\n=== ドロップダウン開いた状態 ===');
console.log(`カード overflow:          ${opened.card.overflow}`);
console.log(`ドロップリスト実高さ:     ${opened.dropList?.h}px (max-h-40 = 160px)`);
console.log(`ドロップリスト top:        ${opened.dropList?.top}  (カードtop=${closed.card.top})`);
console.log(`ドロップリスト bottom:     ${opened.dropList?.bottom}  (カードbottom=${closed.card.bottom})`);
console.log(`カード内で見えている高さ: ${opened.visibleH}px`);
console.log(`クリップされている高さ:   ${opened.clippedH}px`);
console.log('');
console.log('=== 必要な高さの見積もり ===');
console.log(`ヘッダー（ボタントップまで）: ${opened.headerH}px`);
console.log(`ボタン:                       ${opened.btn?.h}px`);
console.log(`ドロップリスト (max-h-40):   160px`);
console.log(`─────────────────────────────`);
console.log(`合計必要高さ:                 ${opened.idealCardH}px`);
console.log(`現在のカード高さ:             ${opened.card.h}px`);
console.log(`不足:                         ${opened.idealCardH - opened.card.h}px`);

await page.screenshot({ path: '/tmp/choice_canvas_height.png' });
await page.waitForTimeout(3000);
await browser.close();
