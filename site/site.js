/*
 * Which guide this is, and the family of guides it links to. The same `guides`
 * list lives in every guide's site.js (MyWant, mywant-gui, mywant-guiex); only `current`,
 * `brand`, `github` and `heroImage` differ. app.js and style.css are shared unchanged
 * between the sites (index.html differs only in its <title> and description),
 * so copy them across when one changes.
 */
/**
 * A screenshot inside a topic's body: shot('menu.jpg', 'caption'), or with
 * { phone: true } for a tall phone capture. Images live in site/img/.
 */
window.shot = (file, caption, opts = {}) =>
  `<figure class="shot${opts.phone ? ' shot-phone' : ''}"><a href="img/${file}" target="_blank" rel="noopener">` +
  `<img src="img/${file}" alt="${caption}" loading="lazy" /></a><figcaption>${caption}</figcaption></figure>`;

window.GUIDE_SITE = {
  current: 'mywant',
  brand: 'MyWant',
  github: 'https://github.com/onelittlenightmusic/mywant',
  heroImage: 'dashboard-detail.jpg',
  guides: [
    {
      id: 'mywant',
      href: 'https://onelittlenightmusic.github.io/MyWant/',
      icon: 'heart',
      color: '#ec4899',
      title: { ja: 'MyWant ガイド', en: 'MyWant guide' },
      sub: { ja: 'インストールと、しくみ', en: 'Installing it, and how it works' },
    },
    {
      id: 'gui',
      href: 'https://onelittlenightmusic.github.io/mywant-gui/',
      icon: 'layout-dashboard',
      color: '#6366f1',
      title: { ja: 'mywant-gui ガイド', en: 'mywant-gui guide' },
      sub: { ja: 'ダッシュボードの使い方', en: 'Using the dashboard' },
    },
    {
      id: 'guiex',
      href: 'https://onelittlenightmusic.github.io/mywant-guiex-guide/',
      icon: 'map',
      color: '#0891b2',
      title: { ja: 'mywant-guiex ガイド', en: 'mywant-guiex guide' },
      sub: { ja: 'キャンバス・ロボット・Web Want', en: 'The canvas, the robot and Web Wants' },
    },
    {
      id: 'dev',
      href: 'https://onelittlenightmusic.github.io/mywant-gui-dev/',
      icon: 'code',
      color: '#64748b',
      title: { ja: '開発者ドキュメント', en: 'Developer docs' },
      sub: { ja: 'GUI の拡張を作る（英語）', en: 'Building GUI extensions' },
      // VitePress, English only: no ?lang= to carry over.
      noLang: true,
    },
  ],
};
