// Presentation only: consume the routes already filtered by the backend.
// These navigation containers are never registered with Vue Router or stored
// as menu/permission records, so existing URLs and grants keep their meaning.
const definitions = [
  { key: 'overview', title: '经营概览', icon: 'erp-overview', direct: true, pages: ['overview'] },
  { key: 'catalog', title: '商品管理', icon: 'erp-catalog', pages: ['product', 'productCategory', 'sku'] },
  { key: 'shops', title: '店铺与渠道', icon: 'erp-shops', pages: ['channel', 'shop', 'shopItem'] },
  { key: 'purchasing', title: '采购管理', icon: 'erp-purchasing', pages: ['supplier', 'purchase', 'purchaseReceive', 'purchaseReceipt', 'purchaseReturnOrder'] },
  { key: 'orders', title: '订单管理', icon: 'erp-orders', pages: ['order', 'importJob', 'package', 'salesReturnOrder'] },
  { key: 'warehouse', title: '仓储管理', icon: 'erp-warehouse', pages: ['warehouse', 'stock', 'ledger', 'picking', 'transfer', 'count', 'receipt', 'sales', 'purchaseReturn', 'salesReturn'] },
  { key: 'logistics', title: '物流管理', icon: 'erp-logistics', pages: ['logistics', 'shipment'] },
  { key: 'finance', title: '财务管理', icon: 'erp-finance', pages: ['financeAccount', 'payable', 'payment', 'financeStatement', 'receivable', 'collection', 'shopStatement'] },
  { key: 'setup', title: '企业启用', icon: 'erp-setup', direct: true, pages: ['setup'] }
]
export const erpNavigationGroups = Object.freeze(definitions.map(group => Object.freeze({
  ...group, path: `/__erp-navigation/${group.key}`, pages: Object.freeze(group.pages)
})))
// Known business leaves have their own icons; categories keep their domain icon.
export const erpNavigationPageIcons = Object.freeze({
  "product": "erp-product",
  "productCategory": "erp-product-category",
  "sku": "erp-sku",
  "channel": "erp-channel",
  "shop": "erp-shop",
  "shopItem": "erp-shop-item",
  "supplier": "erp-supplier",
  "purchase": "erp-purchase",
  "purchaseReceive": "erp-purchase-receive",
  "purchaseReceipt": "erp-purchase-receipt",
  "purchaseReturnOrder": "erp-purchase-return-order",
  "order": "erp-order",
  "importJob": "erp-import-job",
  "package": "erp-package",
  "salesReturnOrder": "erp-sales-return-order",
  "warehouse": "erp-warehouse-info",
  "stock": "erp-stock",
  "ledger": "erp-ledger",
  "picking": "erp-picking",
  "transfer": "erp-transfer",
  "count": "erp-count",
  "receipt": "erp-receipt",
  "sales": "erp-sales",
  "purchaseReturn": "erp-purchase-return",
  "salesReturn": "erp-sales-return",
  "logistics": "erp-logistics-settings",
  "shipment": "erp-shipment",
  "financeAccount": "erp-finance-account",
  "payable": "erp-payable",
  "payment": "erp-payment",
  "financeStatement": "erp-finance-statement",
  "receivable": "erp-receivable",
  "collection": "erp-collection",
  "shopStatement": "erp-shop-statement"
})
const byPage = new Map(erpNavigationGroups.flatMap(group => group.pages.map(page => [`/erp/${page}`, group])))
const external = path => /^(https?:|mailto:|tel:)/.test(path)
const normalize = path => {
  const cut = path.search(/[?#]/)
  const pathname = cut < 0 ? path : path.slice(0, cut)
  return (pathname.replace(/\/{2,}/g, '/').replace(/\/$/, '') || '/') + (cut < 0 ? '' : path.slice(cut))
}

// Sidebar containers have their own unique expansion indexes while leaves
// retain absolute real URLs. Relative children and external links still work.
export function resolveSidebarPath(basePath = '', routePath = '') {
  if (external(routePath)) return routePath
  if (external(basePath)) return basePath
  if (routePath.startsWith('/')) return normalize(routePath)
  return normalize(`${basePath}/${routePath}`)
}

export function erpNavigationCategory(path) {
  return byPage.get(normalize(path.split(/[?#]/, 1)[0]))
}

export function groupErpSidebar(routes = []) {
  return routes.flatMap(root => {
    if (root.hidden || root.path !== '/erp' || !Array.isArray(root.children)) return [root]
    const buckets = new Map(), remaining = []
    for (const child of root.children) {
      const fullPath = resolveSidebarPath(root.path, child.path)
      const group = erpNavigationCategory(fullPath)
      if (!group) { remaining.push(child); continue }
      if (!buckets.has(group.key)) buckets.set(group.key, [])
      const page = fullPath.split(/[?#]/, 1)[0].slice('/erp/'.length)
      const icon = erpNavigationPageIcons[page] || child.meta?.icon || group.icon
      buckets.get(group.key).push({ ...child, path: fullPath, meta: { ...child.meta, icon } })
    }
    const navigation = []
    for (const group of erpNavigationGroups) {
      const children = buckets.get(group.key)
      if (!children?.length) continue
      children.sort((a, b) => group.pages.indexOf(a.path.slice('/erp/'.length)) - group.pages.indexOf(b.path.slice('/erp/'.length)))
      if (group.direct) {
        navigation.push(...children.map(child => ({ ...child, meta: { ...child.meta, icon: group.icon } })))
      } else {
        navigation.push({
          path: group.path, name: `ErpNavigation-${group.key}`, alwaysShow: true,
          hidden: children.every(child => child.hidden),
          meta: { title: group.title, icon: group.icon }, children
        })
      }
    }
    // Newly added/custom pages stay reachable until a category is assigned.
    if (remaining.length) navigation.push({ ...root, meta: { ...root.meta, title: '其他ERP功能' }, children: remaining })
    return navigation
  })
}

// Breadcrumb headings are navigation labels, never clickable virtual routes.
export function erpBreadcrumbs(path, matched) {
  const group = erpNavigationCategory(path)
  const leaf = matched.at(-1)
  if (!group || !leaf?.meta?.title) return null
  return [
    { path: '/index', meta: { title: '首页' } },
    ...(group.direct ? [] : [{ path: group.path, redirect: 'noRedirect', meta: { title: group.title } }]),
    leaf
  ]
}
