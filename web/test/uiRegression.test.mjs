import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import { parse, compileTemplate, compileStyle } from '@vue/compiler-sfc'

const source = path => readFile(new URL(path, import.meta.url), 'utf8')

test('home describes this Go project, not the upstream Java release or donation page', async () => {
  const content = await source('../src/views/index.vue')
  const { descriptor, errors } = parse(content)
  assert.deepEqual(errors, [])
  assert.match(descriptor.template.content, /Go 多租户开发底座/)
  assert.match(descriptor.template.content, /Go \/ Gin \/ GORM/)
  assert.match(descriptor.scriptSetup.content, /identitySummary\(userStore.identity, userStore.name\)/)
  assert.doesNotMatch(content, /SpringBoot|Spring Security|MyBatis|Fastjson|3\.9\.2|捐赠支持|pay\.png|window\.open/)
  assert.deepEqual(compileTemplate({ source: descriptor.template.content, filename: 'index.vue', id: 'home' }).errors, [])
})

test('both document dialogs declare viewport-limited width and independent scrollable bodies', async () => {
  const content = await source('../src/views/erp/components/DocumentWorkspace.vue')
  const { descriptor, errors } = parse(content)
  assert.deepEqual(errors, [])
  const dialogs = [...descriptor.template.content.matchAll(/<el-dialog\b[^>]*>/g)].map(match => match[0])
  assert.equal(dialogs.length, 2)
  for (const dialog of dialogs) {
    assert.match(dialog, /width="min\((850|900)px, calc\(100vw - 32px\)\)"/)
    const classes = (dialog.match(/\bclass="([^"]*)"/)?.[1] || '').split(/\s+/)
    assert.ok(classes.includes('erp-document-dialog'), 'Document-specific scrolling namespace must remain')
    assert.match(dialog, /\balign-center\b/)
  }
  const style = descriptor.styles.find(item => !item.scoped).content
  assert.match(style, /\.erp-document-dialog \.el-dialog__body\s*\{[^}]*min-height:\s*0;[^}]*overflow:\s*auto;/)
  assert.match(style, /max-height:\s*calc\(100dvh - 32px\)/)
  assert.deepEqual(compileStyle({ source: style, filename: 'DocumentWorkspace.vue', id: 'erp-dialog' }).errors, [])
  assert.deepEqual(compileTemplate({ source: descriptor.template.content, filename: 'DocumentWorkspace.vue', id: 'erp-dialog' }).errors, [])
})
