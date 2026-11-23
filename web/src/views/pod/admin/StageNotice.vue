<template>
  <div class="app-container">
    <el-page-header title="返回概览" :content="title" @back="$router.push('/index')" />
    <el-alert class="stage-alert" :title="`将在第 ${stage} 步开放`" type="info" :closable="false" show-icon>
      <p>{{ description }}</p>
      <p>当前已接通管理入口和功能权限，此页面尚未提供业务操作。</p>
    </el-alert>
    <el-card shadow="never">
      <h3>本模块的交付范围</h3>
      <ul><li v-for="item in items" :key="item">{{ item }}</li></ul>
      <p>创作端已支持正式点数和模型报价；本页功能尚未开放，启用范围以当前说明为准。</p>
    </el-card>
  </div>
</template>
<script setup>
defineProps({ title: String, stage: Number, description: String, items: Array })
</script>
<style scoped>
.stage-alert { margin: 20px 0; }
h3 { margin-top: 0; }
li { margin: 12px 0; line-height: 1.6; }
p { color: var(--el-text-color-regular); line-height: 1.6; }
</style>
