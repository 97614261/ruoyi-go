export default {
  /**
   * 网页标题
   */
  title: import.meta.env.VITE_APP_TITLE,

  /**
   * 侧边栏主题 深色主题theme-dark，浅色主题theme-light
   */
  sideTheme: 'theme-dark',

  /**
   * 是否系统布局配置
   */
  showSettings: true,

  /**
   * 菜单导航模式 1、纯左侧 2、混合（左侧+顶部） 3、纯顶部
   */
  navType: 1,

  /**
   * 是否显示 tagsView
   */
  tagsView: true,

  /**
   * 持久化标签页
   */
  tagsViewPersist: false,

  /**
   * 显示页签图标
   */
  tagsIcon: false,

  /**
   * 标签页样式：card 卡片（默认）、chrome 谷歌浏览器风格
   */
  tagsViewStyle: 'card',

  /**
   * 是否固定头部
   */
  fixedHeader: true,

  /**
   * 是否显示logo
   */
  sidebarLogo: true,

  /**
   * 是否显示动态标题
   */
  dynamicTitle: false,

  /**
   * 是否显示底部版权
   */
  footerVisible: false,

  /**
   * 底部版权文本内容
   */
  footerContent: 'Copyright © 2018-2026 RuoYi. All Rights Reserved.',

  /**
   * 背景图。空字符串 = 不启用，整套样式一条都不生效。
   * 取值是 utils/background.js 里的预设 key，或一个图片 URL。
   */
  background: '',

  /**
   * 以下四项是**调参开关**，不是给最终用户的配置。
   *
   * 换预设时会被该预设的推荐值整体覆盖 —— 每张图合适的值不一样，
   * 深色图要更实的面板才压得住深色文字。调出满意的组合后，应该把数字
   * 写回 utils/background.js 的预设表，再把抽屉里对应的滑杆撤掉。
   */
  bgAlpha: 0.78,   // 面板不透明度 0.4~1
  bgVeil: 0.3,     // 背景图上的白纱浓度 0~0.8，兜文字可读性
  bgBlur: 14,      // 面板背景模糊半径 px
  bgChrome: true   // 侧栏和顶栏是否也做成玻璃
}

