<template>
  <el-drawer v-model="showSettings" :withHeader="false" :lock-scroll="false" direction="rtl" size="300px">
    <div class="setting-drawer-title">
      <h3 class="drawer-title">菜单导航设置</h3>
    </div>
    <div class="nav-wrap">
      <el-tooltip content="左侧菜单" placement="bottom">
        <div class="item left" @click="handleNavType(1)" :class="{ activeItem: navType == 1 }">
          <b></b><b></b>
        </div>
      </el-tooltip>

      <el-tooltip content="混合菜单" placement="bottom">
        <div class="item mix" @click="handleNavType(2)" :class="{ activeItem: navType == 2 }">
          <b></b><b></b>
        </div>
      </el-tooltip>
      <el-tooltip content="顶部菜单" placement="bottom">
        <div class="item top" @click="handleNavType(3)" :class="{ activeItem: navType == 3 }">
          <b></b><b></b>
        </div>
      </el-tooltip>
    </div>
    <div class="setting-drawer-title">
      <h3 class="drawer-title">主题风格设置</h3>
    </div>
    <div class="setting-drawer-block-checbox">
      <div class="setting-drawer-block-checbox-item" @click="handleTheme('theme-dark')">
        <img src="@/assets/images/dark.svg" alt="dark" />
        <div v-if="sideTheme === 'theme-dark'" class="setting-drawer-block-checbox-selectIcon" style="display: block;">
          <i aria-label="图标: check" class="anticon anticon-check">
            <svg viewBox="64 64 896 896" data-icon="check" width="1em" height="1em" :fill="theme" aria-hidden="true" focusable="false" class>
              <path d="M912 190h-69.9c-9.8 0-19.1 4.5-25.1 12.2L404.7 724.5 207 474a32 32 0 0 0-25.1-12.2H112c-6.7 0-10.4 7.7-6.3 12.9l273.9 347c12.8 16.2 37.4 16.2 50.3 0l488.4-618.9c4.1-5.1.4-12.8-6.3-12.8z" />
            </svg>
          </i>
        </div>
      </div>
      <div class="setting-drawer-block-checbox-item" @click="handleTheme('theme-light')">
        <img src="@/assets/images/light.svg" alt="light" />
        <div v-if="sideTheme === 'theme-light'" class="setting-drawer-block-checbox-selectIcon" style="display: block;">
          <i aria-label="图标: check" class="anticon anticon-check">
            <svg viewBox="64 64 896 896" data-icon="check" width="1em" height="1em" :fill="theme" aria-hidden="true" focusable="false" class>
              <path d="M912 190h-69.9c-9.8 0-19.1 4.5-25.1 12.2L404.7 724.5 207 474a32 32 0 0 0-25.1-12.2H112c-6.7 0-10.4 7.7-6.3 12.9l273.9 347c12.8 16.2 37.4 16.2 50.3 0l488.4-618.9c4.1-5.1.4-12.8-6.3-12.8z" />
            </svg>
          </i>
        </div>
      </div>
    </div>
    <div class="setting-drawer-block-checbox bg-checbox">
      <div v-for="bg in backgrounds" :key="bg.key" class="setting-drawer-block-checbox-item"
           @click="handleBackgroundChange(bg.key)">
        <div class="bg-thumb" :style="bg.css ? { background: bg.css } : null">
          <span v-if="!bg.css">无</span>
        </div>
        <div v-if="settingsStore.background === bg.key" class="setting-drawer-block-checbox-selectIcon" style="display: block;">
          <i aria-label="图标: check" class="anticon anticon-check">
            <svg viewBox="64 64 896 896" data-icon="check" width="1em" height="1em" :fill="theme" aria-hidden="true" focusable="false" class>
              <path d="M912 190h-69.9c-9.8 0-19.1 4.5-25.1 12.2L404.7 724.5 207 474a32 32 0 0 0-25.1-12.2H112c-6.7 0-10.4 7.7-6.3 12.9l273.9 347c12.8 16.2 37.4 16.2 50.3 0l488.4-618.9c4.1-5.1.4-12.8-6.3-12.8z" />
            </svg>
          </i>
        </div>
      </div>
    </div>

    <div class="drawer-item">
      <span>主题颜色</span>
      <span class="comp-style">
        <el-color-picker v-model="theme" :predefine="predefineColors" @change="themeChange"/>
      </span>
    </div>

    <div class="drawer-item">
      <span>自定义背景</span>
      <span class="comp-style">
        <el-input v-model="customBg" size="small" style="width: 132px" placeholder="图片 URL"
                  clearable @change="handleBackgroundChange(customBg)" />
      </span>
    </div>

    <!-- 调参区：调出满意的值后应把数字写回 utils/background.js 的预设表，再撤掉这一段 -->
    <template v-if="settingsStore.background">
      <div class="drawer-item">
        <span>侧栏顶栏玻璃</span>
        <span class="comp-style">
          <el-switch v-model="settingsStore.bgChrome" @change="bgTune" class="drawer-switch" />
        </span>
      </div>

      <div class="drawer-item bg-slider">
        <span>面板不透明度</span>
        <em>{{ Math.round(settingsStore.bgAlpha * 100) }}%</em>
      </div>
      <el-slider v-model="settingsStore.bgAlpha" :min="0.4" :max="1" :step="0.02"
                 :show-tooltip="false" size="small" @input="bgTune" />

      <div class="drawer-item bg-slider">
        <span>纱层浓度</span>
        <em>{{ Math.round(settingsStore.bgVeil * 100) }}%</em>
      </div>
      <el-slider v-model="settingsStore.bgVeil" :min="0" :max="0.8" :step="0.02"
                 :show-tooltip="false" size="small" @input="bgTune" />

      <div class="drawer-item bg-slider">
        <span>背景模糊</span>
        <em>{{ settingsStore.bgBlur }}px</em>
      </div>
      <el-slider v-model="settingsStore.bgBlur" :min="0" :max="40" :step="1"
                 :show-tooltip="false" size="small" @input="bgTune" />

      <div class="bg-tip">
        文字发飘就调高「纱层浓度」—— 它是压在背景图上的一层，专门兜可读性。
        换预设会把三个值重置成该图的推荐值。暗黑模式下整套自动停用。
      </div>
    </template>
    <el-divider />

    <h3 class="drawer-title">系统布局配置</h3>

    <div class="drawer-item">
      <span>开启页签</span>
      <span class="comp-style">
        <el-switch v-model="settingsStore.tagsView" class="drawer-switch" />
      </span>
    </div>

    <div class="drawer-item">
      <span>持久化标签页</span>
      <span class="comp-style">
        <el-switch v-model="settingsStore.tagsViewPersist" :disabled="!settingsStore.tagsView" @change="tagsViewPersistChange" class="drawer-switch" />
      </span>
    </div>

    <div class="drawer-item">
      <span>显示页签图标</span>
      <span class="comp-style">
        <el-switch v-model="settingsStore.tagsIcon" :disabled="!settingsStore.tagsView" class="drawer-switch" />
      </span>
    </div>

    <div class="drawer-item">
      <span>标签页样式</span>
      <span class="comp-style">
        <el-radio-group v-model="settingsStore.tagsViewStyle" :disabled="!settingsStore.tagsView" size="small">
          <el-radio-button label="card">卡片</el-radio-button>
          <el-radio-button label="chrome">谷歌</el-radio-button>
        </el-radio-group>
      </span>
    </div>

    <div class="drawer-item">
      <span>固定 Header</span>
      <span class="comp-style">
        <el-switch v-model="settingsStore.fixedHeader" class="drawer-switch" />
      </span>
    </div>

    <div class="drawer-item">
      <span>显示 Logo</span>
      <span class="comp-style">
        <el-switch v-model="settingsStore.sidebarLogo" class="drawer-switch" />
      </span>
    </div>

    <div class="drawer-item">
      <span>动态标题</span>
      <span class="comp-style">
        <el-switch v-model="settingsStore.dynamicTitle" @change="dynamicTitleChange" class="drawer-switch" />
      </span>
    </div>

    <div class="drawer-item">
      <span>底部版权</span>
      <span class="comp-style">
        <el-switch v-model="settingsStore.footerVisible" class="drawer-switch" />
      </span>
    </div>

    <el-divider />

    <el-button type="primary" plain icon="DocumentAdd" @click="saveSetting">保存配置</el-button>
    <el-button plain icon="Refresh" @click="resetSetting">重置配置</el-button>
  </el-drawer>

</template>

<script setup>
import useAppStore from '@/store/modules/app'
import useSettingsStore from '@/store/modules/settings'
import usePermissionStore from '@/store/modules/permission'
import { handleThemeStyle } from '@/utils/theme'
import { backgrounds } from '@/utils/background'

const { proxy } = getCurrentInstance()
const appStore = useAppStore()
const settingsStore = useSettingsStore()
const permissionStore = usePermissionStore()
const showSettings = ref(false)
const navType = ref(settingsStore.navType)
const theme = ref(settingsStore.theme)
const sideTheme = ref(settingsStore.sideTheme)
const tagsViewPersist = ref(settingsStore.tagsViewPersist)
const storeSettings = computed(() => settingsStore)
const predefineColors = ref(["#409EFF", "#ff4500", "#ff8c00", "#ffd700", "#90ee90", "#00ced1", "#1e90ff", "#c71585"])
// 当前背景不是内置预设时，说明是自定义 URL，回填到输入框
const customBg = ref(backgrounds.some(b => b.key === settingsStore.background) ? '' : settingsStore.background)

/** 是否需要dynamicTitle */
function dynamicTitleChange() {
  useSettingsStore().setTitle(useSettingsStore().title)
}

function tagsViewPersistChange(val) {
  settingsStore.tagsViewPersist = val
  tagsViewPersist.value = val
}

function themeChange(val) {
  settingsStore.theme = val
  handleThemeStyle(val)
}

function handleTheme(val) {
  settingsStore.sideTheme = val
  sideTheme.value = val
  // 侧栏玻璃的底色跟着侧栏主题走，换主题必须重算，否则深色主题的浅字会压在白玻璃上
  settingsStore.applyBackground()
}

function handleNavType(val) {
  settingsStore.navType = val
  navType.value = val
}

/** 选预设缩略图或填自定义 URL 都走这里，实时写 <html> 上的 CSS 变量 */
function handleBackgroundChange(val) {
  settingsStore.background = val
  // 选中内置预设时清掉输入框，避免两个入口显示的不是同一个值
  if (backgrounds.some(b => b.key === val)) customBg.value = ''
  // 换图必须把三个参数重置成该图的推荐值：深色图要更实的面板才压得住文字，
  // 沿用上一张图的数值会直接看不清
  const preset = backgrounds.find(b => b.key === val && b.key)
  if (preset) {
    settingsStore.bgAlpha = preset.alpha
    settingsStore.bgVeil = preset.veil
    settingsStore.bgBlur = preset.blur
  }
  settingsStore.applyBackground()
}

/** 调参滑杆和开关的统一回调 */
function bgTune() {
  settingsStore.applyBackground()
}

/** 菜单导航设置 */
watch(() => navType, val => {
  if (val.value == 1) {
    appStore.sidebar.opened = true
    appStore.toggleSideBarHide(false)
  }
  if (val.value == 2) {
    appStore.sidebar.opened = true
  }
  if (val.value == 3) {
    appStore.sidebar.opened = false
    appStore.toggleSideBarHide(true)
  }
  if ([1, 3].includes(val.value)) {
      permissionStore.setSidebarRouters(permissionStore.defaultRoutes)
  }
  }, { immediate: true, deep: true }
)

function saveSetting() {
  proxy.$modal.loading("正在保存到本地，请稍候...")
  if (!tagsViewPersist.value) {
    proxy.$cache.local.remove('tags-view-visited')
  }
  let layoutSetting = {
    "navType": storeSettings.value.navType,
    "tagsView": storeSettings.value.tagsView,
    "tagsIcon": storeSettings.value.tagsIcon,
    "tagsViewStyle": storeSettings.value.tagsViewStyle,
    "tagsViewPersist": storeSettings.value.tagsViewPersist,
    "fixedHeader": storeSettings.value.fixedHeader,
    "sidebarLogo": storeSettings.value.sidebarLogo,
    "dynamicTitle": storeSettings.value.dynamicTitle,
    "footerVisible": storeSettings.value.footerVisible,
    "sideTheme": storeSettings.value.sideTheme,
    "theme": storeSettings.value.theme,
    "background": storeSettings.value.background,
    "bgAlpha": storeSettings.value.bgAlpha,
    "bgVeil": storeSettings.value.bgVeil,
    "bgBlur": storeSettings.value.bgBlur,
    "bgChrome": storeSettings.value.bgChrome
  }
  localStorage.setItem("layout-setting", JSON.stringify(layoutSetting))
  setTimeout(proxy.$modal.closeLoading(), 1000)
}

function resetSetting() {
  proxy.$cache.local.remove('tags-view-visited')
  proxy.$modal.loading("正在清除设置缓存并刷新，请稍候...")
  localStorage.removeItem("layout-setting")
  setTimeout("window.location.reload()", 1000)
}

function openSetting() {
  showSettings.value = true
}

defineExpose({
  openSetting
})
</script>

<style lang='scss' scoped>
.setting-drawer-title {
  margin-bottom: 12px;
  color: var(--el-text-color-primary, rgba(0, 0, 0, 0.85));
  line-height: 22px;
  font-weight: bold;

  .drawer-title {
    font-size: 14px;
  }
}

/* 背景缩略图：复用侧栏主题那一行的选中打勾结构，只是把 img 换成渐变块 */
.bg-checbox {
  flex-wrap: wrap;
  gap: 4px 0;
  margin-bottom: 12px;

  .bg-thumb {
    width: 48px;
    height: 48px;
    border-radius: 3px;
    border: 1px solid var(--el-border-color-light);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    color: var(--el-text-color-secondary);
  }
}

/* 调参区：滑杆行左标题右数值，比 drawer-item 紧凑一点 */
.bg-slider {
  padding-bottom: 2px !important;

  em {
    float: right;
    font-style: normal;
    font-size: 12px;
    color: var(--el-text-color-secondary);
    font-variant-numeric: tabular-nums;
  }
}

.bg-tip {
  margin: 10px 0 4px;
  padding: 8px 10px;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--el-text-color-secondary);
  background: var(--el-fill-color-light, rgba(0, 0, 0, 0.04));
}

.setting-drawer-block-checbox {
  display: flex;
  justify-content: flex-start;
  align-items: center;
  margin-top: 10px;
  margin-bottom: 20px;

  .setting-drawer-block-checbox-item {
    position: relative;
    margin-right: 16px;
    border-radius: 2px;
    cursor: pointer;

    img {
      width: 48px;
      height: 48px;
    }

    .setting-drawer-block-checbox-selectIcon {
      position: absolute;
      top: 0;
      right: 0;
      width: 100%;
      height: 100%;
      padding-top: 15px;
      padding-left: 24px;
      color: #1890ff;
      font-weight: 700;
      font-size: 14px;
    }
  }
}

.drawer-item {
  color: var(--el-text-color-regular, rgba(0, 0, 0, 0.65));
  padding: 12px 0;
  font-size: 14px;

  .comp-style {
    float: right;
    margin: -3px 8px 0px 0px;
  }
}

// 导航模式
.nav-wrap {
  display: flex;
  justify-content: flex-start;
  align-items: center;
  margin-top: 10px;
  margin-bottom: 20px;

  .activeItem {
    border: 2px solid var(--el-color-primary) !important;
  }

  .item {
    position: relative;
    margin-right: 16px;
    cursor: pointer;
    width: 56px;
    height: 48px;
    border-radius: 4px;
    background: #f0f2f5;
    border: 2px solid transparent;
  }

  .left {
    b:first-child {
      display: block;
      height: 30%;
      background: #fff;
    }
    b:last-child {
      width: 30%;
      background: #1b2a47;
      position: absolute;
      height: 100%;
      top: 0;
      border-radius: 4px 0 0 4px;
    }
  }
  .mix {
    b:first-child {
      border-radius: 4px 4px 0 0;
      display: block;
      height: 30%;
      background: #1b2a47;
    }
    b:last-child {
      width: 30%;
      background: #1b2a47;
      position: absolute;
      height: 70%;
      border-radius: 0 0 0 4px;
    }
  }
  .top {
    b:first-child {
      display: block;
      height: 30%;
      background: #1b2a47;
      border-radius: 4px 4px 0 0;
    }
  }
}
</style>