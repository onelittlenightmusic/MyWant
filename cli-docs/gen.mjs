// Writes the command reference (docs/reference/**) from the CLI's own
// description of itself — `mywant commands --json --core-only --groups` —
// so every page says what the binary says: its help, examples and flags.
// The sidebar's index comes out beside it (docs/.vitepress/reference.json).
//
//   node gen.mjs                 # runs ../bin/mywant (make release first)
//   MYWANT_BIN=mywant node gen.mjs
//   node gen.mjs commands.json   # from a saved copy
import { execFileSync } from 'node:child_process'
import { mkdirSync, readFileSync, rmSync, writeFileSync, existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const out = join(here, 'docs', 'reference')

function load() {
  const file = process.argv[2]
  if (file) return JSON.parse(readFileSync(file, 'utf8'))
  const bin = process.env.MYWANT_BIN || join(here, '..', 'bin', 'mywant')
  if (!process.env.MYWANT_BIN && !existsSync(bin)) {
    throw new Error(`no ${bin}: run \`make release\` in the repository first, or set MYWANT_BIN`)
  }
  // A home of its own: what the reader's config says must not leak into the
  // defaults the reference prints.
  const json = execFileSync(bin, ['commands', '--json', '--core-only', '--groups'],
    { encoding: 'utf8', env: { ...process.env, HOME: join(here, '.gen-home') } })
  return JSON.parse(json)
}

const all = load()
const root = all.find(c => c.path === '' && c.group)
const cmds = all.filter(c => c.path !== '')
const byPath = new Map(cmds.map(c => [c.path, c]))

// ── Text ────────────────────────────────────────────────────────────────────
// Prose goes into Markdown, where < starts a tag (and {{ an expression: the
// pages sit in ::: v-pre). Code goes into fences as it is.
const esc = s => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/\{\{/g, '&#123;&#123;')
const cell = s => esc(s || '').replace(/\|/g, '\\|').replace(/\n/g, ' ')

/** Help text as the CLI writes it: paragraphs, and indented blocks kept as code. */
function prose(text) {
  if (!text) return ''
  const blocks = text.replace(/\r/g, '').split(/\n\s*\n/)
  return blocks.map(b => {
    const lines = b.split('\n')
    if (lines.every(l => /^(\s{2,}|\t)/.test(l) || !l.trim())) {
      const indent = Math.min(...lines.filter(l => l.trim()).map(l => l.match(/^\s*/)[0].length))
      return '```text\n' + lines.map(l => l.slice(indent)).join('\n') + '\n```'
    }
    // A list the help wrote with - or • stays a list.
    if (lines.every(l => /^\s*([-•*]|\d+[.)])\s/.test(l) || /^\s{2,}\S/.test(l))) {
      return lines.map(l => (/^\s*[-•*]\s/.test(l) ? '- ' + esc(l.replace(/^\s*[-•*]\s/, '')) : /^\s*\d+[.)]\s/.test(l) ? esc(l.trim()) : '  ' + esc(l.trim()))).join('\n')
    }
    return esc(lines.map(l => l.trim()).join(' '))
  }).join('\n\n')
}

const pageOf = path => {
  const parts = path.split(' ')
  const hasKids = cmds.some(c => c.path.startsWith(path + ' '))
  return '/reference/' + parts.join('/') + (hasKids ? '/' : '')
}
const fileOf = path => {
  const parts = path.split(' ')
  const hasKids = cmds.some(c => c.path.startsWith(path + ' '))
  return join(out, ...parts) + (hasKids ? '/index.md' : '.md')
}
const children = path => cmds.filter(c => c.path.startsWith(path + ' ') && c.path.split(' ').length === path.split(' ').length + 1)

const RISK = { read: ['tip', 'reads'], change: ['warning', 'changes'], destroy: ['danger', 'destroys'] }
function badges(c) {
  if (c.group) return ''
  const [type, text] = RISK[c.risk] || RISK.read
  return ` <Badge type="${type}" text="${text}" />` + (c.canvas ? ' <Badge type="info" text="canvas" />' : '')
}

function flagsTable(flags) {
  if (!flags || !flags.length) return ''
  const rows = flags.map(f => {
    const name = (f.shorthand ? `\`-${f.shorthand}\`, ` : '') + `\`--${f.name}\``
    const def = f.default && f.default !== '[]' && f.default !== 'false' ? '`' + f.default.replace(/\|/g, '\\|') + '`' : ''
    return `| ${name} | ${f.type || ''} | ${def} | ${cell(f.usage)} |`
  })
  return ['| Flag | Type | Default | Description |', '| --- | --- | --- | --- |', ...rows].join('\n')
}

function page(c) {
  const parent = c.path.split(' ').slice(0, -1).join(' ')
  const usage = 'mywant ' + (parent ? parent + ' ' : '') + (c.use || c.path.split(' ').pop())
  const kids = children(c.path)
  const lines = [
    '---', `title: "mywant ${c.path}"`, `description: ${JSON.stringify(c.short || '')}`, 'outline: [2, 3]', '---', '',
    `# mywant ${c.path}${badges(c)}`, '', '::: v-pre', '',
    esc(c.short || ''), '',
  ]
  if (!c.group || c.use) lines.push('```sh', usage, '```', '')
  if (c.aliases && c.aliases.length) {
    lines.push(`**Aliases:** ${c.aliases.map(a => '`' + a + '`').join(', ')}`, '')
  }
  if (c.long) lines.push(prose(c.long), '')
  if (c.example) lines.push('## Examples', '', '```sh', c.example.split('\n').map(l => l.replace(/^ {2}/, '')).join('\n'), '```', '')
  if (c.flags && c.flags.length) lines.push('## Flags', '', flagsTable(c.flags), '')
  lines.push(':::', '')
  if (kids.length) {
    lines.push('## Commands', '', '| Command | What it does |', '| --- | --- |')
    for (const k of kids) lines.push(`| [${k.path}](${pageOf(k.path)}) | ${cell(k.short)} |`)
    lines.push('')
  }
  lines.push('## See also', '')
  if (parent) lines.push(`- [mywant ${parent}](${pageOf(parent)})`)
  lines.push('- [Global flags](/reference/#global-flags) — `--context`, `--server`, `--config` work with every command', '')
  return lines.join('\n')
}

// ── Write ───────────────────────────────────────────────────────────────────
rmSync(out, { recursive: true, force: true })
const tops = [...new Set(cmds.map(c => c.path.split(' ')[0]))].sort()
for (const top of tops) {
  // A group the CLI lists only through its commands still gets a page.
  if (!byPath.has(top)) cmds.push({ path: top, group: true, short: '' })
}
for (const c of cmds) {
  const f = fileOf(c.path)
  mkdirSync(dirname(f), { recursive: true })
  writeFileSync(f, page(c))
}

// The index: every command, by group, with what running it costs.
const idx = [
  '---', 'title: Command index', 'outline: [2, 2]', '---', '',
  '# Command index', '',
  `Every command of \`mywant\`, by group — ${cmds.filter(c => !c.group).length} commands, generated from the CLI itself. `
  + 'The badge says what running it does: <Badge type="tip" text="reads" /> only looks, '
  + '<Badge type="warning" text="changes" /> changes something, <Badge type="danger" text="destroys" /> removes something.', '',
]
for (const top of tops) {
  const t = byPath.get(top)
  idx.push(`## ${top}`, '', `::: v-pre`, '', esc((t && t.short) || ''), '', ':::', '', '| Command | What it does | |', '| --- | --- | --- |')
  for (const c of cmds.filter(c => (c.path === top && !c.group) || c.path.startsWith(top + ' ')).sort((a, b) => a.path.localeCompare(b.path))) {
    if (c.group && c.path !== top) { idx.push(`| [**${c.path}**](${pageOf(c.path)}) | ${cell(c.short)} | |`); continue }
    idx.push(`| [${c.path}](${pageOf(c.path)}) | ${cell(c.short)} | ${badges(c).trim()} |`)
  }
  idx.push('')
}
idx.push('## Global flags', '', 'Every command takes these.', '', flagsTable(root ? root.flags : []), '')
writeFileSync(join(out, 'index.md'), idx.join('\n'))

// The sidebar: one group per top-level command, its commands under it.
const sidebar = tops.map(top => {
  const items = cmds.filter(c => c.path.startsWith(top + ' ') && !c.group)
    .sort((a, b) => a.path.localeCompare(b.path))
    .map(c => ({ text: c.path.slice(top.length + 1), link: pageOf(c.path) }))
  return items.length
    ? { text: top, collapsed: true, link: pageOf(top), items }
    : { text: top, link: pageOf(top) }
})
writeFileSync(join(here, 'docs', '.vitepress', 'reference.json'), JSON.stringify(sidebar, null, 2))
console.log(`[gen] ${cmds.length} pages, ${tops.length} groups → docs/reference`)
