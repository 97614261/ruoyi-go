import test from 'node:test'
import assert from 'node:assert/strict'
import { readdir, readFile } from 'node:fs/promises'

async function vueFiles(root) {
  const files = []
  for (const entry of await readdir(root, { withFileTypes: true })) {
    const path = new URL(entry.name + (entry.isDirectory() ? '/' : ''), root)
    if (entry.isDirectory()) files.push(...await vueFiles(path))
    else if (entry.name.endsWith('.vue')) files.push(path)
  }
  return files
}

// These are source-wiring regressions, NOT viewport rendering acceptance.
// Actual dialog geometry and reachable controls are checked in the browser.
test('every ERP dialog opts into the teleported responsive namespace and bounded width', async () => {
  let count = 0
  for (const file of await vueFiles(new URL('../src/views/erp/', import.meta.url))) {
    const source = await readFile(file, 'utf8')
    for (const match of source.matchAll(/<el-dialog\b[^>]*>/g)) {
      count++
      const classes = (match[0].match(/\bclass="([^"]*)"/)?.[1] || '').split(/\s+/)
      assert.ok(classes.includes('erp-dialog'), `${file.pathname}: missing exact namespace`)
      assert.match(match[0], /\bwidth="min\([^"\n]*100vw/, `${file.pathname}: unbounded width`)
    }
  }
  assert.ok(count >= 14, 'ERP dialog inventory must not silently disappear')
})

test('ERP-only global rules bound both axes and keep dialog header/footer outside body scrolling', async () => {
  const index = await readFile(new URL('../src/assets/styles/index.scss', import.meta.url), 'utf8')
  const css = await readFile(new URL('../src/assets/styles/erp-dialog.scss', import.meta.url), 'utf8')
  assert.match(index, /@use ['"]\.\/erp-dialog\.scss['"];/)
  assert.match(css, /\.el-dialog\.erp-dialog\s*\{/)
  assert.match(css, /max-width:\s*calc\(100vw - 24px\)/)
  assert.match(css, /max-height:\s*calc\(100dvh - 32px\)/)
  assert.match(css, /\.el-dialog__header,\s*\.el-dialog__footer\s*\{\s*flex:\s*0 0 auto/)
  assert.match(css, /\.el-dialog__body\s*\{[^}]*min-height:\s*0;[^}]*overflow:\s*auto;/)
  assert.doesNotMatch(css, /(?:^|\n)\.el-dialog\s*\{/, 'Do not affect non-ERP dialogs')
})

test('narrow ERP dialogs neutralize inline grid gutters and wrap footer actions without widening the page', async () => {
  const css = await readFile(new URL('../src/assets/styles/erp-dialog.scss', import.meta.url), 'utf8')
  assert.match(css, /@media\s*\(max-width:\s*640px\)/)
  assert.match(css, /\.el-row\s*\{[^}]*margin-left:\s*0 !important;[^}]*margin-right:\s*0 !important;/)
  assert.match(css, /\.el-col\s*\{[^}]*flex:\s*0 0 100%;[^}]*padding-left:\s*0 !important;[^}]*padding-right:\s*0 !important;/)
  assert.match(css, /\.el-dialog__footer\s*\{[^}]*flex-wrap:\s*wrap;/)
  assert.match(css, /input\[type='file'\]\s*\{\s*width:\s*100%;/)
})

test('ERP viewport height budget overrides the inherited important top margin and desktop row overflow', async () => {
  const css = await readFile(new URL('../src/assets/styles/erp-dialog.scss', import.meta.url), 'utf8')
  assert.match(css, /\.el-dialog\.erp-dialog:not\(\.is-fullscreen\)\s*\{[^}]*margin-top:\s*16px !important;/)
  assert.match(css.split('@media')[0], /\.el-row\s*\{[^}]*margin-left:\s*0 !important;[^}]*margin-right:\s*0 !important;/)
})
