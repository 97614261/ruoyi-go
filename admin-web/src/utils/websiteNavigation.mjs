export function menuPath(base, path) {
  if (/^(https?:|mailto:|tel:)/.test(path) || path.startsWith('/')) return path
  return (base + '/' + path).replace(/\/+/g, '/').replace(/\/$/, '') || '/'
}
function absolute(routes, base = '') { return routes.map(r => { const path=menuPath(base,r.path||'');return {...r,path,meta:r.meta?{...r.meta}:undefined,...(r.children?{children:absolute(r.children,path)}:{})} }) }
function group(path,title,icon,children) { return {path,alwaysShow:true,meta:{title,icon},children} }
// Presentation only: router records and API/menu permissions remain untouched.
export function websiteNavigation(routes) {
  const result=absolute(routes), cms=result.find(r=>r.path==='/cms')
  if(cms?.children){
    const children=cms.children, used=new Set(), take=paths=>paths.map(p=>children.find(c=>c.path==='/cms/'+p)).filter(c=>c&&!c.hidden).map(c=>{used.add(c);return c})
    const content=take(['channel','article','page','product','tag','download']),assets=take(['asset']),site=take(['site']),presets=take(['presets']),operations=take(['inquiry','statistics']),advanced=take(['redirect','workflow','review','notification','data','search-index'])
    if(site.length){site[0].meta={...site[0].meta,title:'模板与外观',icon:'theme'}}
    cms.meta={...cms.meta,title:'网站管理'}
    cms.children=[...(content.length?[group('/__website/content','内容管理','documentation',content)]:[]),...assets,...site,...(!site.length?presets:[]),...(operations.length?[group('/__website/operations','留言与统计','chart',operations)]:[]),...(advanced.length?[group('/__website/advanced','高级设置','tool',advanced)]:[]),...children.filter(c=>!used.has(c))]
  }
  const sys=result.find(r=>r.path==='/system')
  if(sys?.children){const hiddenInAdvanced=new Set(['/system/dept','/system/post','/system/dict','/system/config','/system/notice']);const advanced=sys.children.filter(c=>hiddenInAdvanced.has(c.path)&&!c.hidden);if(advanced.length)sys.children=[...sys.children.filter(c=>!advanced.includes(c)),group('/__system/advanced','高级配置','tool',advanced)];sys.meta={...sys.meta,title:'系统设置'}}
  const tools=result.find(r=>r.path==='/tool');if(tools)tools.meta={...tools.meta,title:'开发工具'}
  return result
}

// Rename existing route titles for breadcrumbs/tags; never register virtual sidebar groups.
export function websiteRouteTitles(routes, base='') {
 const titles={'/cms':'网站管理','/cms/site':'模板与外观','/cms/presets':'初始方案','/system':'系统设置','/tool':'开发工具'}
 return routes.map(r=>{const path=menuPath(base,r.path||'');return {...r,...(r.meta?{meta:{...r.meta,...(titles[path]?{title:titles[path]}:{})}}:{}),...(r.children?{children:websiteRouteTitles(r.children,path)}:{})}})
}
