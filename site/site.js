/*
 * Which guide this is, and the family of guides it links to. The same `guides`
 * list lives in every guide's site.js (MyWant, mywant-gui); only `current`,
 * `brand` and `github` differ. app.js and style.css are shared unchanged
 * between the sites (index.html differs only in its <title> and description),
 * so copy them across when one changes.
 */
window.GUIDE_SITE = {
  current: 'mywant',
  brand: 'MyWant',
  github: 'https://github.com/onelittlenightmusic/mywant',
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
