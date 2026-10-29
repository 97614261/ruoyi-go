<template>
  <el-config-provider :size="appStore.size">
    <router-view />
    <DialogWindowDock />
  </el-config-provider>
</template>

<script setup>
import useAppStore from '@/store/modules/app'
import useSettingsStore from '@/store/modules/settings'
import { handleThemeStyle } from '@/utils/theme'

const appStore = useAppStore()

onMounted(() => {
  nextTick(() => {
    // 初始化主题样式
    handleThemeStyle(useSettingsStore().theme)
    // 初始化背景图
    useSettingsStore().applyBackground()
  })
})
</script>
