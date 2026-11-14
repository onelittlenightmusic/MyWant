import { defineConfig } from 'vitepress'
import reference from './reference.json' with { type: 'json' }

// The command reference of `mywant`, in the developer docs' own look
// (mywant-gui-dev): VitePress as it comes, its badges in the docs' colours.
// The reference pages are generated from the CLI (gen.mjs); the guide pages
// are written by hand.
export default defineConfig({
  title: 'mywant CLI',
  description: 'The command reference of mywant, the MyWant CLI',
  // Served beside the beginner's guide: https://onelittlenightmusic.github.io/MyWant/cli/
  base: '/MyWant/cli/',
  cleanUrls: true,
  // Help text names local servers (http://localhost:8080); they are not pages.
  ignoreDeadLinks: 'localhostLinks',
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'Command index', link: '/reference/' },
      {
        text: 'Guides',
        items: [
          { text: 'MyWant guide', link: 'https://onelittlenightmusic.github.io/MyWant/?lang=en' },
          { text: 'mywant-gui guide', link: 'https://onelittlenightmusic.github.io/mywant-gui/?lang=en' },
          { text: 'mywant-guiex guide', link: 'https://onelittlenightmusic.github.io/mywant-guiex-guide/?lang=en' },
          { text: 'Developer docs', link: 'https://onelittlenightmusic.github.io/mywant-gui-dev/' },
        ],
      },
    ],
    sidebar: [
      {
        text: 'Guide',
        items: [
          { text: 'Getting started', link: '/guide/getting-started' },
          { text: 'Servers and contexts', link: '/guide/contexts' },
          { text: 'Shell completion', link: '/guide/completion' },
          { text: 'Magic sentences', link: '/guide/magic' },
        ],
      },
      {
        text: 'Commands',
        items: [{ text: 'Command index', link: '/reference/' }, ...reference],
      },
    ],
    socialLinks: [{ icon: 'github', link: 'https://github.com/onelittlenightmusic/MyWant' }],
    search: { provider: 'local' },
    outline: [2, 3],
  },
})
