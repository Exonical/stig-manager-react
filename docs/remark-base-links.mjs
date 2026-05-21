// remark plugin: prefix root-relative internal markdown links with the
// configured `base`. Astro rewrites HTML/CSS asset paths and the
// links Starlight generates from `slug:` entries, but it does NOT
// touch links that authors write as `[text](/path)` in markdown. When
// the site is served under a sub-path (e.g. GitHub Pages at
// /stig-manager-react/), those links break. This plugin runs at build
// time and rewrites them to `${base}/path`.
//
// Behaviour:
//   - `/path` -> `${base}/path`
//   - external (`https://...`), anchor (`#...`), and relative
//     (`./x`, `../x`) links are left alone.
//   - protocol-relative (`//example.com/x`) links are left alone.
//   - already-prefixed links (`/stig-manager-react/...`) are left
//     alone so this plugin is idempotent.

import { visit } from 'unist-util-visit'

export default function remarkBaseLinks({ base = '/' } = {}) {
  const trimmed = base === '/' ? '' : base.replace(/\/+$/, '')
  if (trimmed === '') {
    // No sub-path configured; nothing to rewrite.
    return () => {}
  }
  const needsPrefix = (v) =>
    typeof v === 'string' &&
    v.startsWith('/') &&
    !v.startsWith('//') &&
    v !== trimmed &&
    !v.startsWith(trimmed + '/')
  const prefix = (v) => trimmed + v

  return (tree, file) => {
    visit(tree, ['link', 'definition'], (node) => {
      if (!needsPrefix(node.url)) return
      node.url = prefix(node.url)
    })

    // MDX JSX expressions: rewrite `href="/..."` on raw JSX
    // attributes (e.g. <LinkCard href="/installation/quickstart" />).
    visit(tree, (node) => {
      if (
        node.type !== 'mdxJsxFlowElement' &&
        node.type !== 'mdxJsxTextElement'
      ) {
        return
      }
      if (!Array.isArray(node.attributes)) return
      for (const attr of node.attributes) {
        if (attr.type !== 'mdxJsxAttribute') continue
        if (attr.name !== 'href') continue
        if (!needsPrefix(attr.value)) continue
        attr.value = prefix(attr.value)
      }
    })

    // NOTE: Starlight's `splash` template renders `hero.actions[].link`
    // straight from the parsed frontmatter object, which Astro's
    // content-collections layer builds BEFORE remark runs. So
    // mutating `file.data.astro.frontmatter` here does NOT propagate
    // to the rendered page. The matching rewrite lives in the
    // custom Hero override at `src/components/Hero.astro`.
  }
}
