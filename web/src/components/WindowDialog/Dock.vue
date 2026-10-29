<template>
  <Teleport to="body">
    <nav v-if="entries.length" class="dialog-window-dock" aria-label="最小化的弹窗" :style="{ zIndex }">
      <div v-for="entry in entries" :key="entry.id" class="dialog-window-dock__item">
        <el-button text class="dialog-window-dock__restore" :title="'恢复：'+entry.title()" @click="entry.restore">
          <el-icon><CopyDocument /></el-icon><span>{{ entry.title() }}</span>
        </el-button>
        <el-button v-if="entry.closable()" text circle :icon="Close" :aria-label="'关闭：'+entry.title()" @click="entry.close" />
      </div>
    </nav>
  </Teleport>
</template>
<script setup>
import { inject, computed } from 'vue'
import { CopyDocument, Close } from '@element-plus/icons-vue'
import { dialogWindowsKey } from '@/utils/dialog-windows.mjs'
const manager = inject(dialogWindowsKey)
const entries = computed(() => Array.from(manager.windows.values()))
const zIndex = computed(() => Math.max(0, ...entries.value.map(entry => entry.zIndex)))
</script>
