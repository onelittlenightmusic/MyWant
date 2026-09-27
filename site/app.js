/*
 * Draws the guide from window.GUIDE: a card grid per section, a hamburger menu
 * of sections, and the right-hand sidebar a card opens. The open topic lives in
 * the URL hash (#install), so every topic has a link and Back closes it.
 */
(function () {
  const sections = window.GUIDE;
  const topics = sections.flatMap(s => s.topics.map(t => ({ ...t, section: s })));
  const byId = new Map(topics.map(t => [t.id, t]));

  const $ = sel => document.querySelector(sel);
  const el = (tag, attrs = {}, html = '') => {
    const e = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
    if (html) e.innerHTML = html;
    return e;
  };
  const icons = () => window.lucide && window.lucide.createIcons();

  /** An accent as the three custom properties a card and the sidebar read. */
  function accentVars(node, color) {
    node.style.setProperty('--accent', color);
    node.style.setProperty('--accent-ring', color + '99');
    node.style.setProperty('--accent-soft', color + '33');
  }

  // ── Grid ──────────────────────────────────────────────────────────────────
  $('#topic-count').textContent = `${topics.length} topics`;
  const root = $('#sections');
  let delay = 0;
  for (const s of sections) {
    const sec = el('section', { class: 'section', id: 'section-' + s.id });
    sec.append(el('div', { class: 'section-heading' }, `<h2>${s.title}</h2><p>${s.note}</p>`));
    const grid = el('div', { class: 'grid' });
    for (const t of s.topics) {
      const card = el('button', { class: 'card', 'data-id': t.id }, `
        <span class="tile"><i data-lucide="${t.icon}"></i></span>
        <span class="card-text">
          ${t.step ? `<span class="card-step">${t.step}</span>` : ''}
          <span class="card-title">${t.title}</span>
          <span class="card-sub">${t.sub}</span>
        </span>`);
      accentVars(card, t.color);
      card.style.animationDelay = `${Math.min(delay, 12) * 30}ms`;
      delay++;
      card.addEventListener('click', () => go(t.id));
      grid.append(card);
    }
    sec.append(grid);
    root.append(sec);
  }

  // ── Menu ──────────────────────────────────────────────────────────────────
  const menu = $('#menu');
  const menuBtn = $('#menu-btn');
  menu.append(el('p', { class: 'menu-heading' }, 'Sections'));
  const menuGrid = el('div', { class: 'menu-grid' });
  for (const s of sections) {
    const item = el('button', { class: 'menu-item', role: 'menuitem' }, `<i data-lucide="${s.icon}"></i>${s.title}`);
    item.addEventListener('click', () => {
      closeMenu();
      go(null);
      document.getElementById('section-' + s.id).scrollIntoView({ behavior: 'smooth' });
    });
    menuGrid.append(item);
  }
  menu.append(menuGrid);
  menu.append(el('p', { class: 'menu-heading', style: 'margin-top:12px' }, 'Start here'));
  const startGrid = el('div', { class: 'menu-grid' });
  for (const t of sections[0].topics.slice(1, 4)) {
    const item = el('button', { class: 'menu-item', role: 'menuitem' }, `<i data-lucide="${t.icon}"></i>${t.title}`);
    item.querySelector('i').style.color = t.color;
    item.addEventListener('click', () => { closeMenu(); go(t.id); });
    startGrid.append(item);
  }
  menu.append(startGrid);

  function openMenu() { menu.hidden = false; menuBtn.setAttribute('aria-expanded', 'true'); }
  function closeMenu() { menu.hidden = true; menuBtn.setAttribute('aria-expanded', 'false'); }
  menuBtn.addEventListener('click', e => { e.stopPropagation(); menu.hidden ? openMenu() : closeMenu(); });
  document.addEventListener('click', e => { if (!menu.hidden && !menu.contains(e.target)) closeMenu(); });

  // ── Theme ─────────────────────────────────────────────────────────────────
  $('#theme-btn').addEventListener('click', () => {
    const html = document.documentElement;
    const dark = html.dataset.theme
      ? html.dataset.theme === 'dark'
      : window.matchMedia('(prefers-color-scheme: dark)').matches;
    html.dataset.theme = dark ? 'light' : 'dark';
    try { localStorage.setItem('mywant-guide-theme', html.dataset.theme); } catch (e) {}
  });

  // ── Sidebar ───────────────────────────────────────────────────────────────
  const sidebar = $('#sidebar');
  const body = $('#sb-body');
  const scrim = $('#scrim');
  let current = null;

  function decorateCode(scope) {
    scope.querySelectorAll('.code').forEach(block => {
      const pre = block.querySelector('pre');
      const text = pre.textContent;
      // Comments: whole "#" lines, and a trailing "  # …" after a command.
      pre.innerHTML = pre.innerHTML
        .split('\n')
        .map(line => line.replace(/(^|\s{2,})(#\s.*)$/, (_, sp, c) => `${sp}<span class="c">${c}</span>`))
        .join('\n');
      const btn = el('button', { class: 'copy', 'aria-label': 'コピー' }, '<i data-lucide="copy"></i>');
      btn.addEventListener('click', async () => {
        const commands = text
          .split('\n')
          .filter(l => !/^\s*#/.test(l))
          .map(l => l.replace(/\s{2,}#\s.*$/, ''))
          .join('\n')
          .trim();
        try {
          await navigator.clipboard.writeText(commands);
          btn.classList.add('done');
          btn.innerHTML = '<i data-lucide="check"></i>';
          icons();
          setTimeout(() => { btn.classList.remove('done'); btn.innerHTML = '<i data-lucide="copy"></i>'; icons(); }, 1400);
        } catch (e) {}
      });
      block.append(btn);
    });
  }

  function render(t, direction) {
    const i = topics.indexOf(t);
    accentVars(sidebar, t.color);
    $('#sb-title-text').textContent = t.section.title;
    const icon = $('#sb-title svg, #sb-title i');
    icon.replaceWith(el('i', { 'data-lucide': t.section.icon }));

    body.innerHTML = `
      <div class="sb-hero">
        <span class="tile"><i data-lucide="${t.icon}"></i></span>
        <div><h3>${t.title}</h3><p>${t.step ? t.step + ' · ' : ''}${t.sub}</p></div>
      </div>
      <div class="doc">${t.body}</div>`;
    decorateCode(body);
    body.scrollTop = 0;
    body.classList.remove('swap', 'swap-back');
    if (direction) { void body.offsetWidth; body.classList.add(direction < 0 ? 'swap-back' : 'swap'); }

    $('#sb-prev').disabled = i === 0;
    $('#sb-next').disabled = i === topics.length - 1;
    document.querySelectorAll('.card').forEach(c => c.setAttribute('aria-current', String(c.dataset.id === t.id)));
    document.title = `${t.title} · MyWant ガイド`;
    icons();
  }

  function show(id) {
    const t = id && byId.get(id);
    if (!t) {
      current = null;
      sidebar.classList.remove('open');
      sidebar.setAttribute('aria-hidden', 'true');
      document.body.classList.remove('sb-open');
      scrim.hidden = true;
      document.querySelectorAll('.card[aria-current="true"]').forEach(c => c.setAttribute('aria-current', 'false'));
      document.title = 'MyWant ガイド';
      return;
    }
    const direction = current ? Math.sign(topics.indexOf(t) - topics.indexOf(current)) : 0;
    current = t;
    render(t, direction);
    sidebar.classList.add('open');
    sidebar.setAttribute('aria-hidden', 'false');
    document.body.classList.add('sb-open');
    scrim.hidden = false;
    // Keep the chosen card in view beside the sidebar.
    const card = document.querySelector(`.card[data-id="${t.id}"]`);
    if (card && window.innerWidth >= 1024) card.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }

  function go(id) {
    const hash = id ? '#' + id : ' ';
    if (id ? location.hash !== hash : location.hash) {
      history.pushState(null, '', id ? hash : location.pathname + location.search);
    }
    show(id);
  }

  const step = d => {
    if (!current) return;
    const next = topics[topics.indexOf(current) + d];
    if (next) go(next.id);
  };
  $('#sb-prev').addEventListener('click', () => step(-1));
  $('#sb-next').addEventListener('click', () => step(1));
  $('#sb-grid').addEventListener('click', () => go(null));
  $('#sb-close').addEventListener('click', () => go(null));
  $('.sb-grip').addEventListener('click', () => go(null));
  scrim.addEventListener('click', () => go(null));
  $('#home-link').addEventListener('click', e => { e.preventDefault(); go(null); window.scrollTo({ top: 0, behavior: 'smooth' }); });

  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') { if (!menu.hidden) closeMenu(); else if (current) go(null); }
    if (!current || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === 'ArrowRight') step(1);
    if (e.key === 'ArrowLeft') step(-1);
  });

  window.addEventListener('popstate', () => show(location.hash.slice(1)));
  show(location.hash.slice(1));
  icons();
})();
