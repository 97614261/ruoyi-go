/**
 * 背景图主题 —— 把选中的背景翻译成 <html> 上的行内 CSS 变量。
 *
 * 和 utils/theme.js 的 handleThemeStyle 同一套路数：样式表里的规则读这些变量，
 * 行内优先级高于样式表，所以切换即时生效；关闭时把变量清干净，一条规则都不留。
 *
 * 【为什么只有一个配置项】
 *
 * 不透明度 / 模糊 / 纱层浓度看着像三个旋钮，实际描述的是同一个感受轴 ——
 * 背景显不显。用户说得出"背景淡一点"，说不出"alpha 0.7 blur 16 veil 0.34"。
 * 而且每张图的合适值不一样（深色图要更实的面板才压得住文字），
 * 所以它们是**跟着预设走的常量**，不是独立开关。
 *
 * 【为什么不需要"文字颜色"和"深浅色调"】
 *
 * 面板一律保持浅色且足够实，文字就永远是深色压浅底，不存在白底白字的问题。
 * 深色背景图只要把 alpha 调高即可，不需要连带切换一整套文字色。
 */

/**
 * 内置预设。背景一律用纯 CSS 渐变 —— 不引入图片资源，离线可用，
 * 抽屉里的缩略图直接渲染同一段 css，不需要额外的缩略图文件。
 *
 * alpha 面板不透明度，深色背景要更高才压得住深色文字
 * veil  压在背景上的白纱浓度，兜可读性
 * blur  面板的背景模糊半径
 */
export const backgrounds = [
  { key: '', label: '无', css: '' },
  {
    key: 'dawn',
    label: '晨曦',
    css: 'linear-gradient(135deg, #ffd9a0 0%, #ffb3ba 45%, #a5b4fc 100%)',
    alpha: 0.62, veil: 0.22, blur: 14
  },
  {
    key: 'mint',
    label: '薄荷',
    css: 'linear-gradient(135deg, #a8edea 0%, #d5f0c0 50%, #fed6e3 100%)',
    alpha: 0.6, veil: 0.2, blur: 14
  },
  {
    key: 'sakura',
    label: '浅樱',
    css: 'linear-gradient(135deg, #fff1eb 0%, #ace0f9 100%)',
    alpha: 0.58, veil: 0.18, blur: 14
  },
  {
    key: 'ocean',
    label: '深海',
    css: 'linear-gradient(135deg, #0f2027 0%, #203a43 50%, #2c5364 100%)',
    alpha: 0.74, veil: 0.06, blur: 16
  }
]

/** 自定义 URL 用的中间档参数 */
const CUSTOM = { alpha: 0.64, veil: 0.24, blur: 14 }

/** 开启背景时写上去的变量，关闭时按这张表清干净 */
const OWNED = ['--bg-image', '--bg-veil', '--bg-alpha', '--bg-blur', '--bg-side-tint']

/**
 * 侧栏玻璃的底色 RGB。
 *
 * 侧栏必须**保持自己的色调**，只是变半透明：深色主题是深底浅字，
 * 直接换成白色半透明会让浅灰的菜单文字压在浅色背景图上，完全看不见。
 * 取值对齐 variables.module.scss 的 $menuBg / $menuLightBg。
 */
const SIDE_TINT = {
  'theme-dark': '26, 31, 46',
  'theme-light': '255, 255, 255'
}

/**
 * 把用户填的地址包成 CSS url()。
 *
 * 只接受 http(s) 和站内绝对路径；引号、括号、换行一律去掉，
 * 避免值里带着分号闯进 background 简写把整条声明改掉。
 * 地址本身来自用户自己的设置，这里只做基本收口，不当安全边界。
 */
function toCssUrl(raw) {
  const url = String(raw).trim().replace(/["'()\s]/g, '')
  if (!/^(https?:\/\/|\/)/i.test(url)) return ''
  return `url("${url}") center / cover no-repeat fixed`
}

/**
 * 应用背景。value 为空字符串时彻底关闭。
 *
 * @param {string} value      预设 key，或图片 URL
 * @param {string} sideTheme  侧栏主题，决定侧栏玻璃的底色，见 SIDE_TINT
 * @param {object} [tune]     调参覆盖值 { alpha, veil, blur, chrome }。
 *                            传了就用传的，没传就用预设自带的推荐值。
 *                            调参阶段结束后这个参数应该整个去掉。
 */
export function handleBackground(value, sideTheme = 'theme-dark', tune = {}) {
  const el = document.documentElement
  const preset = backgrounds.find(b => b.key === value && b.key)
  const image = preset ? preset.css : toCssUrl(value || '')

  // 认不出来的值当作关闭，不要留下一个开着但没有图的空壳
  if (!image) {
    el.classList.remove('has-bg', 'has-bg-chrome')
    OWNED.forEach(name => el.style.removeProperty(name))
    return
  }

  const base = preset || CUSTOM
  const alpha = tune.alpha ?? base.alpha
  const veil = tune.veil ?? base.veil
  const blur = tune.blur ?? base.blur
  el.style.setProperty('--bg-image', image)
  el.style.setProperty('--bg-veil', String(veil))
  // 只写这一个不透明度源头；--bg-alpha-strong / --bg-alpha-chrome 由 CSS
  // 用 calc 派生。JS 多写一个变量就多一处能和 CSS 不同步，而不同步的表现
  // （无效 rgba 导致整条声明被丢弃）看着像特异性问题，极难排查
  el.style.setProperty('--bg-alpha', String(alpha))
  el.style.setProperty('--bg-blur', `${blur}px`)
  el.style.setProperty('--bg-side-tint', SIDE_TINT[sideTheme] || SIDE_TINT['theme-dark'])
  el.classList.add('has-bg')
  // 侧栏和顶栏是否一起做玻璃，单独一个 class，方便直接对比两种效果
  el.classList.toggle('has-bg-chrome', tune.chrome !== false)
}

export default handleBackground
