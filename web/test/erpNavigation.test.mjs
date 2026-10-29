import test from 'node:test'
import assert from 'node:assert/strict'
import { erpNavigationPageIcons, erpNavigationGroups, erpNavigationCategory, groupErpSidebar, resolveSidebarPath, erpBreadcrumbs } from '../src/utils/erp-navigation.mjs'

const page = (path, extra = {}) => ({ path, name: `Page-${path}`, component: `erp/${path}/index`, meta: { title: path, icon: 'list', noCache: true }, ...extra })
const erp = children => ({ path: '/erp', name: 'Erp', meta: { title: 'ERP工作台' }, children })
const leaves = (routes, base = '') => routes.flatMap(route => {
  const path = resolveSidebarPath(base, route.path)
  return route.children?.length ? leaves(route.children, path) : [{ ...route, path }]
})

test('all 36 ERP pages retain their URLs, query, names and visibility across nine navigation categories', () => {
  const pages = erpNavigationGroups.flatMap(group => group.pages)
  assert.equal(pages.length, 36); assert.equal(new Set(pages).size, 36)
  const input = [erp(pages.map((path, index) => page(path, { query: JSON.stringify({ source: index }), hidden: path === 'sku' })))]
  const original = JSON.stringify(input), output = groupErpSidebar(input)
  assert.equal(output.length, 9); assert.equal(JSON.stringify(input), original)
  const before = leaves(input).sort((a, b) => a.path.localeCompare(b.path)), after = leaves(output).sort((a, b) => a.path.localeCompare(b.path))
  assert.deepEqual(after.map(({ meta, ...route }) => route), before.map(({ meta, ...route }) => route))
  assert.deepEqual(after.map(route => route.meta.title), before.map(route => route.meta.title))
  assert.equal(new Set(output.map(route => route.path)).size, output.length)
})

test('limited roles only see their supplied pages; empty categories and siblings are never invented', () => {
  const input = [erp([page('purchaseReceive'), page('stock')])], output = groupErpSidebar(input)
  assert.deepEqual(output.map(route => route.meta.title), ['采购管理', '仓储管理'])
  assert.deepEqual(leaves(output).map(route => route.path), ['/erp/purchaseReceive', '/erp/stock'])
  assert.ok(output.every(route => route.alwaysShow))
  assert.deepEqual(groupErpSidebar([erp([])]), [])
})

test('platform/non-ERP navigation is preserved and hidden ERP roots are never promoted', () => {
  const platform = { path: '/platform', children: [page('tenant')] }, system = { path: '/system', children: [page('user')] }
  assert.deepEqual(groupErpSidebar([platform, system]), [platform, system])
  const hidden = { ...erp([page('order')]), hidden: true }
  assert.equal(groupErpSidebar([hidden])[0], hidden)
  assert.equal(groupErpSidebar([erp([page('stock', { hidden: true })])])[0].hidden, true)
})

test('unclassified/custom ERP pages and nested leaves remain reachable without duplicating known pages', () => {
  const custom = page('custom', { children: [page('detail')] }), output = groupErpSidebar([erp([page('order'), custom])])
  assert.equal(output.at(-1).meta.title, '其他ERP功能')
  assert.deepEqual(leaves(output).map(route => route.path), ['/erp/order', '/erp/custom/detail'])
  assert.deepEqual(groupErpSidebar(output), output)
})

test('absolute and relative leaf paths work with synthetic group indexes and external/query links', () => {
  assert.equal(resolveSidebarPath('/__erp-navigation/orders', '/erp/order'), '/erp/order')
  assert.equal(resolveSidebarPath('/erp', 'order'), '/erp/order')
  assert.equal(resolveSidebarPath('/erp/order', ''), '/erp/order')
  assert.equal(resolveSidebarPath('/system', 'user'), '/system/user')
  assert.equal(resolveSidebarPath('/erp', 'https://example.test/path'), 'https://example.test/path')
  assert.equal(resolveSidebarPath('https://example.test/path', ''), 'https://example.test/path')
  assert.equal(resolveSidebarPath('/erp', 'order?source=ui#list'), '/erp/order?source=ui#list')
  assert.equal(resolveSidebarPath('/erp', 'order?return=http://example.test/a//b'), '/erp/order?return=http://example.test/a//b')
  assert.equal(leaves(groupErpSidebar([erp([page('/erp/product')])]))[0].path, '/erp/product')
})

test('grouped breadcrumbs use the original page record and a nonclickable business heading', () => {
  const leaf = page('/erp/picking'), matched = [{ path: '/erp', meta: { title: 'ERP工作台' } }, leaf]
  const result = erpBreadcrumbs('/erp/picking', matched)
  assert.equal(result[1].meta.title, '仓储管理'); assert.equal(result[1].redirect, 'noRedirect'); assert.equal(result[2], leaf)
  assert.equal(erpBreadcrumbs('/erp/overview', [page('/erp/overview')]).length, 2)
  assert.equal(erpBreadcrumbs('/erp/setup', [page('/erp/setup')]).length, 2)
  assert.equal(erpBreadcrumbs('/system/user', [page('/system/user')]), null)
  assert.equal(erpBreadcrumbs('/erp/custom', [page('/erp/custom')]), null)
  assert.equal(erpNavigationCategory('/erp/order?x=1').title, '订单管理')
  assert.equal(erpNavigationCategory('/erp/orderly'), undefined)
})

// The sprite asset must exist for every visible category and page.
test('ERP categories and business leaves have meaningful sprite assets and preserve authorization metadata', async () => {
  const { readFile } = await import('node:fs/promises')
  assert.equal(new Set(erpNavigationGroups.map(g => g.icon)).size, 9)
  const nestedPages = erpNavigationGroups.filter(g => !g.direct).flatMap(g => g.pages)
  assert.deepEqual(Object.keys(erpNavigationPageIcons).sort(), [...nestedPages].sort())
  assert.equal(new Set(Object.values(erpNavigationPageIcons)).size, nestedPages.length)
  for (const group of erpNavigationGroups) {
    const svg=await readFile(new URL('../src/assets/icons/svg/'+group.icon+'.svg',import.meta.url),'utf8')
    assert.match(svg,/viewBox="0 0 24 24"/); assert.match(svg,/stroke="currentColor"/)
    for (const pageKey of group.pages) {
      const icon = group.direct ? group.icon : erpNavigationPageIcons[pageKey]
      assert.ok(icon, `Missing business icon for ${pageKey}`)
      if (!group.direct) assert.notEqual(icon, group.icon)
      const asset = await readFile(new URL('../src/assets/icons/svg/'+icon+'.svg', import.meta.url),'utf8')
      assert.match(asset,/viewBox="0 0 24 24"/); assert.match(asset,/stroke="currentColor"/)
      const supplied = page(pageKey, {meta:{title:'Authorized page',permissions:['only:granted'],noCache:true}})
      const leaf=leaves(groupErpSidebar([erp([supplied])]))[0]
      assert.deepEqual(leaf.meta,{...supplied.meta,icon})
      assert.equal(leaf.path,'/erp/'+pageKey)
    }
  }
})
