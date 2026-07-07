// Web Want Inspector overlay — the in-page logic injected by both the
// Chrome/CDP path (server.ts's injectWebInspector, via Playwright's
// page.evaluate) and the standalone bootstrap served to non-CDP browsers
// (Safari, iPhone Chrome — see build-standalone-overlay.mjs). Kept in its own
// module, rather than inline in server.ts, specifically so both callers run
// the exact same logic instead of two copies drifting apart.
//
// Self-contained on purpose: only ever touches `window`/`document`/standard
// browser globals plus the params object passed in — no outer closure
// captures — because Playwright serializes this function via .toString() to
// run it in-page, and the standalone bootstrap does the same thing after
// fetching params from GET /api/v1/web-wants/active-inspection.
export function webInspectorOverlayCore({ webhookUrl, suggestNameUrl, myCharacterId, myColor, myAvatar, existingMarks, navElements }: {
  webhookUrl: string;
  suggestNameUrl: string;
  myCharacterId: string;
  myColor: string;
  myAvatar: string;
  existingMarks: Array<{role:string;name:string;selector:string;characterId:string;color:string}>;
  navElements: Array<{role:string;name:string;selector:string}>;
}): void {
  if ((window as any).__mywantInspectorLoaded) return;
  (window as any).__mywantInspectorLoaded = true;

  // My character's aura/cursor color — used both for the shared CursorMan
  // SVG (see cursorColor below) and to tint the "Launch保存済み要素" section
  // of the sidebar, so the saved-elements area visually matches whichever
  // character (aaa/hero/etc, or the cyan default when none is set as
  // mycursor) is doing the inspecting.
  const navColor = myColor || '#f59e0b';

  // Same aura color/fallback as the shared CursorMan (cursorColor below) —
  // used to give already-saved elements (outlined via .mwi-selected, see
  // renderExistingMarks) a soft glow at inspector launch, instead of a flat
  // solid outline, so they visually read as "marked with my aura" the same
  // way the CursorMan and aura ground-painting do elsewhere in the app.
  const auraColor = myColor || '#00e5ff';

  const style = document.createElement('style');
  style.textContent = `
    .mwi-highlight { outline: 2px solid #6366f1 !important; outline-offset: 2px !important; }
    .mwi-selected  { outline-width: 2px !important; outline-style: solid !important; filter: drop-shadow(0 0 6px ${auraColor}99) drop-shadow(0 0 12px ${auraColor}55); }
    .mwi-focused   { outline: 3px solid #f59e0b !important; box-shadow: 0 0 0 4px rgba(245,158,11,0.25) !important; }
    .mwi-mark-dot  { position:fixed; width:10px; height:10px; border-radius:50%; border:1.5px solid rgba(255,255,255,0.8); pointer-events:none; z-index:2147483645; box-shadow:0 1px 3px rgba(0,0,0,.5); }
    .mwi-cursor { position:fixed; pointer-events:none; z-index:2147483647; transition:left .15s cubic-bezier(0.34,1.56,0.64,1),top .15s cubic-bezier(0.34,1.56,0.64,1); }
    .mwi-nav-member { outline: 2px dashed ${navColor} !important; outline-offset: 3px !important; }
    .mwi-nav-name { font-weight:700; color:${navColor}; font-size:12px; margin-bottom:4px; }
    .mwi-nav-hint { font-size:10px; color:#64748b; margin:4px 0 8px; }
    .mwi-nav-list { flex:none; max-height:140px; overflow-y:auto; margin-bottom:4px; border-left:2px solid ${navColor}55; padding-left:8px; }
    .mwi-nav-item-focused { outline:1px solid ${navColor} !important; }
    .mwi-panel {
      position:fixed; top:0; right:0; bottom:0; background:rgba(15,23,42,0.97);
      border-left:1px solid #334155; padding:14px;
      z-index:2147483646; color:#f1f5f9; font:13px/1.4 system-ui,sans-serif;
      width:270px; box-shadow:-6px 0 32px rgba(0,0,0,.5);
      display:flex; flex-direction:column;
      transition: width .18s ease, padding .18s ease;
      overflow:hidden;
    }
    .mwi-panel.mwi-collapsed { width:44px; padding:10px 6px; }
    .mwi-panel-header { display:flex; align-items:center; justify-content:space-between; gap:6px; flex:none; }
    .mwi-panel-body { display:flex; flex-direction:column; flex:1; min-height:0; margin-top:8px; }
    .mwi-panel.mwi-collapsed .mwi-panel-body { display:none; }
    .mwi-collapse-btn {
      position:relative; cursor:pointer; flex:none; width:26px; height:26px;
      display:flex; align-items:center; justify-content:center;
      background:#1e293b; border:1px solid #334155; border-radius:6px;
      font-size:13px; color:#a5b4fc;
    }
    .mwi-panel-badge {
      position:absolute; top:-6px; right:-6px; min-width:14px; text-align:center;
      background:#22c55e; color:#052e13; font-size:9px; font-weight:800; line-height:1;
      border-radius:8px; padding:2px 4px;
    }
    .mwi-panel-hint { color:#64748b; font-size:11px; margin-bottom:10px; padding-bottom:10px; border-bottom:1px solid #334155; }
    .mwi-panel-list { flex:1; overflow-y:auto; min-height:0; }
    .mwi-panel-footer { margin-top:10px; padding-top:10px; border-top:1px solid #334155; display:flex; flex-direction:column; align-items:flex-end; gap:6px; }
    .mwi-item { display:flex; align-items:center; gap:6px; padding:4px 6px; border-radius:6px; background:#1e293b; margin:3px 0; font-size:12px; }
    .mwi-badge { padding:1px 6px; border-radius:4px; font-size:10px; font-weight:700; }
    .mwi-badge-input { background:#3b82f6; color:#fff; }
    .mwi-badge-button { background:#8b5cf6; color:#fff; }
    .mwi-btn { padding:6px 12px; border-radius:6px; border:none; cursor:pointer; font-size:12px; font-weight:600; margin:3px 2px; }
    .mwi-btn-done { background:#22c55e; color:#fff; }
    .mwi-btn-cancel { background:#475569; color:#cbd5e1; }
    .mwi-dialog {
      position:fixed; top:50%; left:50%; transform:translate(-50%,-50%);
      background:rgba(15,23,42,0.99); border:1px solid #475569; border-radius:10px;
      padding:18px; z-index:2147483647; color:#f1f5f9; font:13px/1.4 system-ui,sans-serif;
      min-width:260px; box-shadow:0 20px 60px rgba(0,0,0,.7);
    }
    .mwi-input { width:100%; padding:7px 10px; border-radius:6px; border:1px solid #475569; background:#1e293b; color:#f1f5f9; font-size:13px; margin:6px 0; box-sizing:border-box; }
    .mwi-spinner {
      display:inline-block; width:11px; height:11px; margin-left:6px; vertical-align:middle;
      border:2px solid #475569; border-top-color:#f59e0b; border-radius:50%;
      animation: mwi-spin 0.7s linear infinite;
    }
    @keyframes mwi-spin { to { transform: rotate(360deg); } }
  `;
  document.head.appendChild(style);

  const getInteractive = () =>
    Array.from(document.querySelectorAll<Element>(
      'input:not([type=hidden]):not([disabled]),textarea:not([disabled]),select:not([disabled]),button:not([disabled]),a[href],[role=button],[role=textbox],[role=combobox],[role=searchbox]'
    )).filter(el => {
      const r = el.getBoundingClientRect();
      return r.width > 0 && r.height > 0 && getComputedStyle(el).visibility !== 'hidden' &&
        !el.closest('.mwi-panel,.mwi-dialog,.mwi-cursor');
    });

  let elements: Element[] = [];
  let focusIdx = 0;
  const selected: Array<{role:string;name:string;selector:string;html_name?:string;characterId:string;color:string}> = [];
  let dialogOpen = false;
  // Single-slot state for the currently-open dialog's keyboard handler and
  // in-flight speculative name-suggestion fetch — only one dialog can be
  // open at a time, so these must be torn down in __mwiCancel/__mwiAdd to
  // avoid leaking into the next dialog instance.
  let dialogKeyHandler: ((e: KeyboardEvent) => void) | null = null;
  let suggestController: AbortController | null = null;

  // Free cursor state (left stick) and page zoom (right stick)
  let cursorX = window.innerWidth  / 2;
  let cursorY = window.innerHeight / 2;
  let pageZoom = 1.0;

  // Single CursorMan for the whole overlay — matches mywant-gui's
  // CursorManIcon.tsx exactly: when a character is bound, show its emoji
  // avatar in a colored circle; otherwise fall back to the default
  // stick-figure SVG in cursorColor (cyan when no character is bound at
  // all). Used both for plain-arrow spatial nav over all elements and for
  // the Cmd+Arrow jump to the saved (Launch) element subset — one cursor,
  // two granularities, matching Dashboard.tsx's moveCursorMan /
  // canvasWantFocusNav split.
  const cursorColor = myColor || '#00e5ff';
  const cursor = document.createElement('div');
  cursor.className = 'mwi-cursor';
  if (myAvatar) {
    const circleSize = 30;
    const fontSize = Math.round(circleSize * 0.58);
    cursor.innerHTML = `<div style="width:${circleSize}px;height:${circleSize}px;border-radius:50%;background-color:${cursorColor}33;border:2px solid ${cursorColor};display:flex;align-items:center;justify-content:center;font-size:${fontSize}px;line-height:1;filter:drop-shadow(0 0 6px ${cursorColor}99)">${myAvatar}</div>`;
  } else {
    cursor.innerHTML = `<svg width="28" height="34" viewBox="0 0 24 29" fill="none" xmlns="http://www.w3.org/2000/svg" style="filter:drop-shadow(0 0 6px ${cursorColor}99) drop-shadow(0 0 14px ${cursorColor}55)"><circle cx="12" cy="5" r="3.5" fill="${cursorColor}"/><rect x="8.5" y="9.5" width="7" height="7" rx="1" fill="${cursorColor}"/><line x1="8.5" y1="11" x2="5" y2="15" stroke="${cursorColor}" stroke-width="2.5" stroke-linecap="round"/><line x1="15.5" y1="11" x2="19" y2="15" stroke="${cursorColor}" stroke-width="2.5" stroke-linecap="round"/><line x1="10" y1="16.5" x2="8" y2="22" stroke="${cursorColor}" stroke-width="2.5" stroke-linecap="round"/><line x1="14" y1="16.5" x2="16" y2="22" stroke="${cursorColor}" stroke-width="2.5" stroke-linecap="round"/><ellipse cx="12" cy="23.5" rx="4" ry="1.5" fill="rgba(0,0,0,0.25)"/></svg>`;
  }
  document.body.appendChild(cursor);

  const panel = document.createElement('div');
  panel.className = 'mwi-panel';
  document.body.appendChild(panel);

  // ── Saved-elements list (same elements a want type's Launch action
  // navigates) — merged into the main sidebar panel (rendered inside
  // renderPanel() below) rather than a separate floating box. The shared
  // cursor jumps to them via Cmd+Arrow / gamepad L1+D-pad (see jumpToNav
  // below), which updates navFocusIdx to highlight the current one. All
  // members also get a persistent dashed outline (mwi-nav-member, applied
  // in refresh()) so the saved subset stays visible regardless of where
  // the cursor currently is.
  let navFocusIdx = -1;

  // Auto-collapse the sidebar on narrow viewports (iPhone portrait etc) —
  // at 270px wide it eats most of a ~390px phone screen, leaving barely
  // enough room to see/tap the page underneath. Below COLLAPSE_WIDTH it
  // starts collapsed to a 44px tab instead; the user can still tap the tab
  // to expand it. Once the user manually toggles, we stop overriding their
  // choice on resize/rotate (panelCollapseManual).
  const COLLAPSE_WIDTH = 480;
  let panelCollapsed = window.innerWidth < COLLAPSE_WIDTH;
  let panelCollapseManual = false;

  (window as any).__mwiToggleCollapse = () => {
    panelCollapsed = !panelCollapsed;
    panelCollapseManual = true;
    renderPanel();
  };

  window.addEventListener('resize', () => {
    if (panelCollapseManual) return;
    const shouldCollapse = window.innerWidth < COLLAPSE_WIDTH;
    if (shouldCollapse !== panelCollapsed) {
      panelCollapsed = shouldCollapse;
      renderPanel();
    }
  });

  // Reapply the persistent "part of the saved set" outline to every
  // navElement currently present in the DOM (called from refresh()).
  const renderNavMembers = () => {
    document.querySelectorAll('.mwi-nav-member').forEach(e => e.classList.remove('mwi-nav-member'));
    navElements.forEach(e => { try { document.querySelector(e.selector)?.classList.add('mwi-nav-member'); } catch {} });
  };

  const getRole = (el: Element) =>
    el.getAttribute('role') ||
    (['INPUT','TEXTAREA'].includes(el.tagName) ? 'textbox' :
     el.tagName === 'SELECT' ? 'combobox' : 'button');

  const getSelector = (el: Element) => {
    if (el.id) return '#' + CSS.escape(el.id);
    const name = el.getAttribute('name');
    if (name) return `[name="${name}"]`;
    const aria = el.getAttribute('aria-label');
    if (aria) return `[aria-label="${aria}"]`;
    return el.tagName.toLowerCase();
  };

  const getAutoName = (el: Element) =>
    el.getAttribute('aria-label') || el.getAttribute('placeholder') ||
    el.getAttribute('name') || (el as HTMLElement).innerText?.trim().slice(0,30) ||
    el.id || `element ${selected.length + 1}`;

  // Once __mwiDone has replaced the panel with its completion/error banner,
  // renderPanel must stop repainting — the 4s refresh interval (and any focus/
  // selection re-render) would otherwise clobber the banner within seconds.
  let sessionDone = false;

  const renderPanel = () => {
    if (sessionDone) return;
    panel.classList.toggle('mwi-collapsed', panelCollapsed);
    const navSection = navElements.length > 0 ? `
      <div class="mwi-nav-name">🤖 Launch保存済み要素 (${navElements.length})</div>
      <div class="mwi-nav-list">${navElements.map((e,i)=>`<div class="mwi-item${i===navFocusIdx ? ' mwi-nav-item-focused' : ''}"><span class="mwi-badge mwi-badge-${e.role==='textbox'?'input':'button'}">${e.role}</span><span style="flex:1">${e.name}</span></div>`).join('')}</div>
      <div class="mwi-nav-hint">⌘+↑↓←→ / 🎮 L1+D-pad:ジャンプ</div>
    ` : '';
    const badge = selected.length > 0 ? `<span class="mwi-panel-badge">${selected.length}</span>` : '';
    const header = panelCollapsed
      ? `<div class="mwi-panel-header" style="justify-content:center">
          <div class="mwi-collapse-btn" onclick="window.__mwiToggleCollapse()" title="サイドバーを開く">◀${badge}</div>
        </div>`
      : `<div class="mwi-panel-header">
          <div style="font-weight:700;color:#a5b4fc;font-size:13px">🔍 Web Want Inspector</div>
          <div class="mwi-collapse-btn" onclick="window.__mwiToggleCollapse()" title="サイドバーを折りたたむ">▶</div>
        </div>`;
    panel.innerHTML = `
      ${header}
      <div class="mwi-panel-body">
        <div class="mwi-panel-hint">↑↓←→:近傍移動 · X/タップ:選択・命名 · Esc:キャンセル · Enter/🎮A:完了</div>
        ${navSection}
        <div id="mwi-list" class="mwi-panel-list">${selected.map((s,i)=>`<div class="mwi-item"><span class="mwi-badge mwi-badge-${s.role==='textbox'?'input':'button'}">${s.role}</span><span style="flex:1">${s.name}</span><span onclick="window.__mwiRemove(${i})" style="cursor:pointer;color:#ef4444;font-size:14px">✕</span></div>`).join('')}</div>
        <div class="mwi-panel-footer">
          <div style="color:#475569;font-size:11px">${elements.length} 個のinteractive要素</div>
          <button class="mwi-btn mwi-btn-done" onclick="window.__mwiDone()">✓ 完了 (${selected.length}個)</button>
        </div>
      </div>
    `;
  };

  const setFocus = (idx: number) => {
    elements.forEach(e => e.classList.remove('mwi-focused'));
    const el = elements[idx];
    if (!el) return;
    el.classList.add('mwi-focused');
    el.scrollIntoView({ block: 'nearest', behavior: 'instant' });
    requestAnimationFrame(() => {
      const r = el.getBoundingClientRect();
      cursorX = Math.max(4, Math.min(r.left + r.width / 2 - 14, window.innerWidth - 36));
      cursorY = Math.max(4, Math.min(r.top - 38, window.innerHeight - 40));
      cursor.style.left = cursorX + 'px';
      cursor.style.top  = cursorY + 'px';
    });
  };

  // Dot badges for other characters' marks — pinned near each matched
  // element's top-left corner, one per distinct other-character color.
  let markDots: HTMLDivElement[] = [];
  const renderExistingMarks = () => {
    markDots.forEach(d => d.remove());
    markDots = [];
    const byColorPerElement = new Map<Element, Set<string>>();
    let seededAny = false;
    for (const mark of existingMarks) {
      let el: Element | null;
      try { el = document.querySelector(mark.selector); } catch { el = null; }
      if (!el) continue;
      if (mark.characterId === myCharacterId) {
        // My own prior mark — persistent solid outline in my color, same
        // treatment as a fresh in-session selection. Also seed it into
        // `selected` (once, deduped by selector) so it shows up in the
        // #mwi-list panel and is re-included in the payload when "完了" is
        // pressed — otherwise pressing Done without touching it would drop
        // it from the saved elements.json for this hostname.
        (el as HTMLElement).style.setProperty('outline-color', myColor, 'important');
        el.classList.add('mwi-selected');
        if (!selected.some(s => s.selector === mark.selector)) {
          selected.push({ role: mark.role, name: mark.name, selector: mark.selector, characterId: myCharacterId, color: myColor });
          seededAny = true;
        }
        continue;
      }
      if (!byColorPerElement.has(el)) byColorPerElement.set(el, new Set());
      byColorPerElement.get(el)!.add(mark.color);
    }
    byColorPerElement.forEach((colors, el) => {
      const r = el.getBoundingClientRect();
      Array.from(colors).forEach((color, i) => {
        const dot = document.createElement('div');
        dot.className = 'mwi-mark-dot';
        dot.style.background = color;
        dot.style.left = (r.left - 4 + i * 12) + 'px';
        dot.style.top = (r.top - 4) + 'px';
        document.body.appendChild(dot);
        markDots.push(dot);
      });
    });
    if (seededAny) renderPanel();
  };

  const refresh = () => {
    elements.forEach(e => e.classList.remove('mwi-highlight','mwi-focused'));
    elements = getInteractive();
    elements.forEach(e => e.classList.add('mwi-highlight'));
    focusIdx = Math.min(focusIdx, Math.max(0, elements.length - 1));
    setFocus(focusIdx);
    renderPanel();
    renderExistingMarks();
    renderNavMembers();
  };

  (window as any).__mwiRemove = (idx: number) => { selected.splice(idx,1); renderPanel(); };

  let detectedUrlTemplate: string | null = null;

  const detectGetUrlTemplate = (el: Element): string | null => {
    const form = el.closest('form');
    if (!form || (form as HTMLFormElement).method.toLowerCase() !== 'get') return null;
    const action = (form as HTMLFormElement).action || window.location.href;
    const parts: string[] = [];
    Array.from((form as HTMLFormElement).elements).forEach((fe: Element) => {
      const f = fe as HTMLInputElement;
      if (f.name && f.type !== 'hidden' && f.type !== 'submit' && f.type !== 'button') {
        parts.push(`${encodeURIComponent(f.name)}={{plan.${f.name}}}`);
      }
    });
    return parts.length > 0 ? `${action}?${parts.join('&')}` : null;
  };

  (window as any).__mwiDone = () => {
    const hostname = window.location.hostname || window.location.href;
    const payload: Record<string, any> = { [hostname]: selected };
    if (detectedUrlTemplate) payload.__url_template = detectedUrlTemplate;
    if (myCharacterId) { payload.characterId = myCharacterId; payload.color = myColor; }
    // Page context for the always-on capture endpoint (/web-wants/capture),
    // which derives the type's url/title/name from these since there is no
    // GUI form anymore. Harmless on the want-bound webhook path: its handler
    // skips non-array payload keys.
    payload.__page_url = window.location.href;
    payload.__page_title = document.title;
    fetch(webhookUrl, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    }).then(r => {
      if (!r.ok) {
        return r.text().then((t: string) => { throw new Error(t || ('HTTP ' + r.status)); });
      }
      return r.json().catch(() => ({}));
    }).then((d: any) => {
      // The capture endpoint returns {name: "<created type>"}; the review
      // webhook returns {"status":"ok"} — pick the banner accordingly.
      sessionDone = true;
      panel.innerHTML = d && d.name
        ? `<div style="color:#22c55e;font-weight:700;padding:8px">✓ 作成完了: ${d.name}</div>`
        : '<div style="color:#22c55e;font-weight:700;padding:8px">✓ 保存完了！タブを閉じます...</div>';
      (window as any).__mwiDoneSignal = true;
    }).catch((e: Error) => {
      // Error banner is NOT sticky: the user can fix the selection (e.g. "no
      // elements selected") and press 完了 again, so let renders resume.
      panel.innerHTML = `<div style="color:#ef4444;padding:8px">エラー: ${e.message}</div>`;
    });
  };

  const showDialog = (el: Element) => {
    if (dialogOpen) return;
    dialogOpen = true;
    const role = getRole(el);
    const autoName = getAutoName(el);
    const sel = getSelector(el);

    const d = document.createElement('div');
    d.className = 'mwi-dialog';
    d.innerHTML = `
      <div style="font-weight:700;margin-bottom:4px;color:#a5b4fc">要素を追加</div>
      <div style="color:#64748b;font-size:11px;margin-bottom:10px">role: ${role}</div>
      <label style="font-size:11px;color:#94a3b8">名前<span id="mwi-spinner" class="mwi-spinner" style="display:none"></span></label>
      <input class="mwi-input" id="mwi-name" value="${autoName.replace(/"/g,'&quot;')}" />
      <div style="margin-top:10px;display:flex;justify-content:flex-end">
        <button class="mwi-btn mwi-btn-cancel" onclick="window.__mwiCancel()">キャンセル</button>
        <button class="mwi-btn mwi-btn-done" onclick="window.__mwiAdd('${role}','${sel.replace(/'/g,"\\'")}')">追加</button>
      </div>
    `;
    document.body.appendChild(d);
    const inp = document.getElementById('mwi-name') as HTMLInputElement;
    const spinner = document.getElementById('mwi-spinner') as HTMLElement | null;

    // No auto-focus here — this is the crux of the speculative-naming
    // feature below: as long as focus hasn't been given yet, we treat the
    // user as "not editing" and let the AI suggestion land in the field.
    dialogKeyHandler = (e: KeyboardEvent) => {
      if (e.key === 'Enter')  { e.preventDefault(); e.stopPropagation(); (window as any).__mwiAdd(role, sel); }
      if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); (window as any).__mwiCancel(); }
    };
    document.addEventListener('keydown', dialogKeyHandler, true);

    if (suggestNameUrl) {
      const context = (el.closest('[class],[id]') as HTMLElement | null) ?? (el as HTMLElement);
      const html = (context.outerHTML || '').slice(0, 3000);
      suggestController = new AbortController();
      if (spinner) spinner.style.display = 'inline-block';
      fetch(suggestNameUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ html }),
        signal: suggestController.signal,
      })
        .then(r => r.json())
        .then((data: { name?: string }) => {
          if (spinner) spinner.style.display = 'none';
          if (data?.name && inp && document.activeElement !== inp) {
            inp.value = data.name;
          }
        })
        .catch(() => { if (spinner) spinner.style.display = 'none'; });

      inp?.addEventListener('focus', () => {
        suggestController?.abort();
        suggestController = null;
        if (spinner) spinner.style.display = 'none';
      });
    }

    (window as any).__mwiDialog = d;
  };

  (window as any).__mwiCancel = () => {
    if (suggestController) { suggestController.abort(); suggestController = null; }
    if (dialogKeyHandler) { document.removeEventListener('keydown', dialogKeyHandler, true); dialogKeyHandler = null; }
    (window as any).__mwiDialog?.remove();
    (window as any).__mwiDialog = null;
    dialogOpen = false;
  };

  (window as any).__mwiAdd = (role: string, sel: string) => {
    const name = (document.getElementById('mwi-name') as HTMLInputElement)?.value || 'element';
    const el = elements[focusIdx];
    const htmlName = (el as HTMLInputElement)?.name || '';
    selected.push({ role, name, selector: sel, characterId: myCharacterId, color: myColor, ...(htmlName ? { html_name: htmlName } : {}) });
    if (el) (el as HTMLElement).style.setProperty('outline-color', myColor, 'important');
    el?.classList.add('mwi-selected');
    if (!detectedUrlTemplate && el) {
      detectedUrlTemplate = detectGetUrlTemplate(el);
    }
    (window as any).__mwiCancel();
    renderPanel();
  };

  const blurActive = () => {
    const a = document.activeElement as HTMLElement | null;
    if (a && a !== document.body && a !== document.documentElement) a.blur();
  };

  document.addEventListener('keydown', (e: KeyboardEvent) => {
    if (dialogOpen) return; // ダイアログ中はキーを通す（Enter/Escapeはdialog側のキーハンドラが処理）

    // Enter (no dialog open): finish the whole inspection — same as
    // gamepad A and the "✓ 完了" button. Selecting/naming an element is
    // x/X only (see below), not Enter/A.
    if (e.key === 'Enter') {
      e.preventDefault();
      e.stopPropagation();
      (window as any).__mwiDone();
      return;
    }

    const isX = e.key.toLowerCase() === 'x';
    if (!['ArrowDown','ArrowRight','ArrowUp','ArrowLeft'].includes(e.key) && !isX) return;

    // Cmd+Arrow jumps the shared cursor to the nearest saved (Launch)
    // element in that direction — same cursor as plain-arrow spatial nav,
    // just a different target subset (mirrors the want-canvas's
    // Cmd+Arrow-vs-plain-arrow split). Cmd+x is excluded the same way, so
    // it falls through to the metaKey no-op below.
    if (e.metaKey && navElements.length > 0 && !isX) {
      e.preventDefault();
      e.stopPropagation();
      if      (e.key === 'ArrowRight') jumpToNav('right');
      else if (e.key === 'ArrowLeft')  jumpToNav('left');
      else if (e.key === 'ArrowDown')  jumpToNav('down');
      else if (e.key === 'ArrowUp')    jumpToNav('up');
      return;
    }
    if (e.metaKey) return;

    e.preventDefault();
    e.stopPropagation();
    blurActive();
    if      (e.key === 'ArrowRight') { spatialNav('right'); }
    else if (e.key === 'ArrowLeft')  { spatialNav('left');  }
    else if (e.key === 'ArrowDown')  { spatialNav('down');  }
    else if (e.key === 'ArrowUp')    { spatialNav('up');    }
    else if (isX) {
      if (elements[focusIdx]) showDialog(elements[focusIdx]);
    }
  }, true);

  // Touch/tap-to-select: for platforms without a keyboard or gamepad (e.g.
  // iPhone via the standalone bookmarklet/Shortcut loader — see
  // build-standalone-overlay.mjs), tapping a highlighted element directly
  // opens its naming dialog — equivalent to arrow-navigating there and
  // pressing x. 'click' fires uniformly for touch taps and mouse clicks, so
  // this also gives desktop/CDP users click-to-select instead of only
  // keyboard/gamepad. Captured on document so it runs before the target's
  // own click handler (e.g. a real link navigating away) and can suppress it.
  document.addEventListener('click', (e: MouseEvent) => {
    const target = e.target as Element;
    if (target.closest('.mwi-panel, .mwi-dialog, .mwi-cursor')) return; // let our own UI's clicks behave normally
    if (dialogOpen) return;
    const hit = elements.find(el => el === target || el.contains(target));
    if (!hit) return;
    e.preventDefault();
    e.stopPropagation();
    focusIdx = elements.indexOf(hit);
    setFocus(focusIdx);
    showDialog(hit);
  }, true);

  // Initialize cursor at center
  cursor.style.left = cursorX + 'px';
  cursor.style.top  = cursorY + 'px';

  // Closest interactive element to a viewport point
  const closestElement = (x: number, y: number): Element | null => {
    let best: Element | null = null;
    let bestDist = Infinity;
    for (const el of elements) {
      const r = el.getBoundingClientRect();
      const cx = r.left + r.width  / 2;
      const cy = r.top  + r.height / 2;
      const d  = Math.hypot(cx - x, cy - y);
      if (d < bestDist) { bestDist = d; best = el; }
    }
    return best;
  };

  // Spatial navigation: move to the nearest element in the pressed direction.
  // Uses a weighted score: primary-axis distance + 1.5 * perpendicular distance,
  // so elements that are more aligned in the target direction score better.
  const spatialNav = (dir: 'up'|'down'|'left'|'right') => {
    const cur = elements[focusIdx];
    if (!cur) { if (elements.length) { focusIdx = 0; setFocus(0); } return; }
    const cr  = cur.getBoundingClientRect();
    const cx  = cr.left + cr.width  / 2;
    const cy  = cr.top  + cr.height / 2;

    let bestIdx   = -1;
    let bestScore = Infinity;

    elements.forEach((el, i) => {
      if (i === focusIdx) return;
      const r  = el.getBoundingClientRect();
      const ex = r.left + r.width  / 2;
      const ey = r.top  + r.height / 2;
      const dx = ex - cx;
      const dy = ey - cy;

      let primary: number, perp: number, forward: boolean;
      switch (dir) {
        case 'right': forward = dx > 0; primary =  dx; perp = Math.abs(dy); break;
        case 'left':  forward = dx < 0; primary = -dx; perp = Math.abs(dy); break;
        case 'down':  forward = dy > 0; primary =  dy; perp = Math.abs(dx); break;
        case 'up':    forward = dy < 0; primary = -dy; perp = Math.abs(dx); break;
      }
      if (!forward!) return;
      const score = primary! + perp! * 1.5;
      if (score < bestScore) { bestScore = score; bestIdx = i; }
    });

    if (bestIdx >= 0) { focusIdx = bestIdx; setFocus(focusIdx); }
  };

  // Cmd+Arrow / gamepad L1+D-pad: jump the same cursor directly to the
  // nearest saved (Launch) element in the pressed direction — mirrors the
  // want-canvas's Cmd+Arrow behavior (Dashboard.tsx's canvasWantFocusNav +
  // WantCanvas.findNearest: half-plane filter, primary + 2×perpendicular
  // score) rather than plain-arrow's element-by-element spatial walk.
  const jumpToNav = (dir: 'up'|'down'|'left'|'right') => {
    if (navElements.length === 0) return;
    const cur = elements[focusIdx];
    const cr = cur ? cur.getBoundingClientRect() : { left: cursorX, top: cursorY, width: 0, height: 0 };
    const cx = cr.left + cr.width  / 2;
    const cy = cr.top  + cr.height / 2;

    let bestEl: Element | null = null;
    let bestIdxInNav = -1;
    let bestScore = Infinity;

    navElements.forEach((def, i) => {
      let el: Element | null;
      try { el = document.querySelector(def.selector); } catch { el = null; }
      if (!el || el === cur) return;
      const r = el.getBoundingClientRect();
      const ex = r.left + r.width  / 2;
      const ey = r.top  + r.height / 2;
      const dx = ex - cx;
      const dy = ey - cy;

      let primary: number, perp: number, forward: boolean;
      switch (dir) {
        case 'right': forward = dx > 0; primary =  dx; perp = Math.abs(dy); break;
        case 'left':  forward = dx < 0; primary = -dx; perp = Math.abs(dy); break;
        case 'down':  forward = dy > 0; primary =  dy; perp = Math.abs(dx); break;
        case 'up':    forward = dy < 0; primary = -dy; perp = Math.abs(dx); break;
      }
      if (!forward!) return;
      const score = primary! + perp! * 2;
      if (score < bestScore) { bestScore = score; bestEl = el; bestIdxInNav = i; }
    });

    if (!bestEl || bestIdxInNav < 0) return;
    const matchIdx = elements.indexOf(bestEl);
    if (matchIdx >= 0) {
      focusIdx = matchIdx;
      setFocus(focusIdx);
    } else {
      // Saved element isn't part of the current interactive scan (rare) —
      // move the cursor there directly without touching focusIdx/elements.
      elements.forEach(e => e.classList.remove('mwi-focused'));
      (bestEl as Element).classList.add('mwi-focused');
      (bestEl as Element).scrollIntoView({ block: 'nearest', behavior: 'instant' });
      requestAnimationFrame(() => {
        const r = (bestEl as Element).getBoundingClientRect();
        cursorX = Math.max(4, Math.min(r.left + r.width / 2 - 14, window.innerWidth - 36));
        cursorY = Math.max(4, Math.min(r.top - 38, window.innerHeight - 40));
        cursor.style.left = cursorX + 'px';
        cursor.style.top  = cursorY + 'px';
      });
    }
    navFocusIdx = bestIdxInNav;
    renderPanel();
  };

  let prevBtns: boolean[] = [];
  let gpollActive = true;
  const DEADZONE    = 0.15;
  const CURSOR_SPD  = 10;   // px per frame at full deflection
  const ZOOM_RATE   = 0.02; // scale per frame at full deflection

  const gpoll = () => {
    if (!gpollActive) return;
    for (const gp of navigator.getGamepads?.() ?? []) {
      if (!gp) continue;
      const cur = gp.buttons.map((b: GamepadButton) => b.pressed);

      // ── Left stick: free cursor movement (no element snap) ──────────────
      const lx = Math.abs(gp.axes[0] ?? 0) > DEADZONE ? (gp.axes[0] ?? 0) : 0;
      const ly = Math.abs(gp.axes[1] ?? 0) > DEADZONE ? (gp.axes[1] ?? 0) : 0;
      if (lx !== 0 || ly !== 0) {
        cursorX = Math.max(0, Math.min(window.innerWidth  - 28, cursorX + lx * CURSOR_SPD));
        cursorY = Math.max(0, Math.min(window.innerHeight - 34, cursorY + ly * CURSOR_SPD));
        cursor.style.transition = 'none';
        cursor.style.left = cursorX + 'px';
        cursor.style.top  = cursorY + 'px';
      } else {
        cursor.style.transition = 'left .15s cubic-bezier(0.34,1.56,0.64,1),top .15s cubic-bezier(0.34,1.56,0.64,1)';
      }

      // ── Right stick Y: zoom ─────────────────────────────────────────────
      const rx = gp.axes.length > 2 ? (gp.axes[2] ?? 0) : 0;
      const ry = gp.axes.length > 3 ? (gp.axes[3] ?? 0) : 0;
      if (Math.abs(ry) > DEADZONE && Math.abs(ry) > Math.abs(rx)) {
        pageZoom = Math.max(0.25, Math.min(4.0, pageZoom - ry * ZOOM_RATE));
        (document.documentElement as HTMLElement).style.zoom = String(pageZoom);
      }

      // ── D-pad: spatial element navigation, or — while L1 (button 4) is
      // held and saved elements exist — jump the same cursor to the nearest
      // saved element in that direction instead, mirroring the Cmd+Arrow
      // keyboard binding above. ────────────────────────────────────────────
      const navHeld = !!cur[4] && navElements.length > 0;
      if (navHeld) {
        if (cur[12] && !prevBtns[12]) jumpToNav('up');
        if (cur[13] && !prevBtns[13]) jumpToNav('down');
        if (cur[14] && !prevBtns[14]) jumpToNav('left');
        if (cur[15] && !prevBtns[15]) jumpToNav('right');
      } else {
        if (cur[12] && !prevBtns[12]) { blurActive(); spatialNav('up');    }
        if (cur[13] && !prevBtns[13]) { blurActive(); spatialNav('down');  }
        if (cur[14] && !prevBtns[14]) { blurActive(); spatialNav('left');  }
        if (cur[15] && !prevBtns[15]) { blurActive(); spatialNav('right'); }
      }

      // ── X button: select element nearest to cursor (or focusIdx if D-pad was used) ──
      if (cur[2] && !prevBtns[2]) {
        if (!dialogOpen) {
          const target = (lx !== 0 || ly !== 0)
            ? closestElement(cursorX, cursorY)
            : (elements[focusIdx] ?? closestElement(cursorX, cursorY));
          if (target) showDialog(target);
        }
      }

      // ── A button: done — same as Enter and the "✓ 完了" button ──────────
      if (cur[0] && !prevBtns[0] && !dialogOpen) (window as any).__mwiDone();

      prevBtns = cur;
    }
    requestAnimationFrame(gpoll);
  };
  requestAnimationFrame(gpoll);

  // BFCache restore: re-run refresh and restart gpoll if it stopped
  window.addEventListener('pageshow', (ev: PageTransitionEvent) => {
    if (ev.persisted) {
      gpollActive = true;
      requestAnimationFrame(gpoll);
      setTimeout(refresh, 200);
    }
  });

  refresh();
  setInterval(refresh, 4000);
}
