export const setupChecks = [
 {key:'roles',group:0,label:'常用业务角色',permission:'system:role:list',path:'/system/role',help:'新增角色可预览并预填模板；模板不会自动分配给员工。'},
 {key:'employees',group:0,label:'启用员工账号',permission:'system:user:list',path:'/system/user',help:'创建员工并分配角色。账号不自动拥有仓库或店铺范围。'},
 {key:'warehouseScopes',group:0,label:'员工仓库范围',permission:'erp:warehouse:grant',path:'/erp/warehouse',help:'存在有效员工范围记录；请逐人核对实际可操作仓库。'},
 {key:'shopScopes',group:0,label:'员工店铺范围',permission:'erp:shop:grant',path:'/erp/shop',help:'存在有效员工范围记录；请逐人核对实际可操作店铺。'},
 {key:'warehouses',group:1,label:'启用仓库',permission:'erp:warehouse:list',path:'/erp/warehouse',help:'至少一个启用仓库。所有者无需给自己分配范围。'},
 {key:'products',group:1,label:'可用商品SKU',permission:'erp:sku:list',path:'/erp/sku',help:'商品档案维护多SKU，也可沿原SKU入口使用未归组商品。'},
 {key:'shops',group:1,label:'启用店铺',permission:'erp:shop:list',path:'/erp/shop',help:'按真实业务选择渠道、站点和币种；不代表接通平台API。'},
 {key:'suppliers',group:1,label:'启用供应商',permission:'erp:supplier:list',path:'/erp/supplier',help:'用于采购。供应商联系人需要独立详情权限。'},
 {key:'pairings',group:1,label:'有效店铺商品配对',permission:'erp:shopItem:list',path:'/erp/shopItem',help:'店铺商品与内部SKU都启用，且有有效配对。下单审核仍逐行校验。'},
 {key:'logistics',group:1,label:'可用物流方式（可选）',permission:'erp:logistics:list',path:'/erp/logistics',help:'目录公司和方式均启用；也可继续使用原手工物流入口。'},
 {key:'accounts',group:1,label:'内部记账账户',permission:'erp:financeAccount:list',path:'/erp/financeAccount',help:'按实际币种建账；不会接入银行或产生真实转账。'},
 {key:'stock',group:2,label:'有实物库存记录',permission:'erp:stock:list',path:'/erp/stock',help:'期初库存走原采购入库单并确认，或真实采购收货；不要直接覆盖库存余额。'},
 {key:'receipts',group:2,label:'第一笔采购收货',permission:'erp:purchaseReceipt:list',path:'/erp/purchaseReceipt',help:'在采购单中确认并分批收货；查看收货事实。'},
 {key:'shipments',group:2,label:'第一笔本地发货',permission:'erp:shipment:list',path:'/erp/shipment',help:'录入订单→审核预占→保存运单→确认发货，只表示本地出库。'},
 {key:'payables',group:2,label:'登记采购应付',permission:'erp:payable:list',path:'/erp/payable',help:'按原收货批次显式登记，再按币种核销；不会自动补历史账。'},
 {key:'receivables',group:2,label:'登记销售应收',permission:'erp:receivable:list',path:'/erp/receivable',help:'原订单金额明确且已发货才可显式登记；未知金额不能当成0。'}
]
export const setupStates={READY:'已有记录',MISSING:'待准备',NOT_INSTALLED:'模块未安装',NOT_ENABLED:'功能未开通'}
export function setupRows(data){
 if(!data||!Array.isArray(data.checks))throw Error('启用检查响应无效')
 const map=new Map();for(const row of data.checks){if(!row||map.has(row.key)||!Object.hasOwn(setupStates,row.state))throw Error('启用检查响应无效');map.set(row.key,row.state)}
 return setupChecks.map(spec=>{const state=map.get(spec.key);if(!state)throw Error('启用检查项缺失');return{...spec,state}})
}
// Template application is an exact, independent tree selection. Selecting a
// parent in linked mode would silently select every sibling action below it.
export function roleTemplateDraft(template){
 if(!template||!/^erp_[a-z]+$/.test(template.templateKey)||!Array.isArray(template.menuIds)||!template.menuIds.length||!template.menuIds.every(id=>Number.isSafeInteger(id)&&id>0))throw Error('模板无可用权限')
 return{roleName:template.name,roleKey:template.templateKey,menuIds:[...new Set(template.menuIds)],menuCheckStrictly:false}
}
