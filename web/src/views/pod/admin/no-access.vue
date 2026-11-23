<template>
  <main class="access-denied">
    <el-result icon="warning" title="此账号没有平台管理权限" sub-title="创作者请使用创作端；管理账号需由管理员分配 POD 管理权限。">
      <template #extra>
        <el-button type="primary" @click="$router.replace('/login')">切换管理账号</el-button>
        <el-link v-if="studioURL" :href="studioURL" type="primary">打开创作端</el-link>
      </template>
    </el-result>
  </main>
</template>
<script setup>
const studioURL = import.meta.env.VITE_POD_STUDIO_URL || (import.meta.env.DEV ? 'http://127.0.0.1:5177' : '')
</script>
<style scoped>
.access-denied { min-height: 100vh; display: grid; place-items: center; background: var(--el-bg-color-page); }
.el-link { margin-left: 20px; }
</style>
