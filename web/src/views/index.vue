<template>
  <div class="app-container home">
    <h1>Go 多租户开发底座</h1>
    <p class="home-intro">以 Go 为后端、Vue 3 为前端，提供企业隔离、账号权限和基础管理能力。当前 ERP 是库存业务底座，不代表完整电商 ERP。</p>

    <el-card shadow="never" class="home-panel">
      <template #header>当前工作空间</template>
      <dl class="workspace-details">
        <div><dt>企业</dt><dd>{{ summary.tenantName }}</dd></div>
        <div><dt>账号</dt><dd>{{ summary.userName }}</dd></div>
        <div><dt>角色</dt><dd>{{ summary.roleLabel }}</dd></div>
      </dl>
      <el-alert v-if="summary.platformAdmin" title="平台账号管理企业生命周期与开通范围，不进入客户的 ERP 业务数据。" type="info" :closable="false" />
      <el-alert v-else title="菜单由企业开通范围与账号权限共同决定；库存操作还需要仓库权限。看不到入口时，请联系企业管理员。" type="info" :closable="false" />
    </el-card>

    <el-row :gutter="16">
      <el-col :xs="24" :md="12">
        <el-card shadow="never" class="home-panel">
          <template #header>技术栈</template>
          <dl class="technology-details">
            <div><dt>后端</dt><dd>Go / Gin / GORM</dd></div>
            <div><dt>前端</dt><dd>Vue 3 / Vite / Pinia / Element Plus</dd></div>
            <div><dt>数据</dt><dd>MySQL / Redis</dd></div>
            <div><dt>认证</dt><dd>JWT / bcrypt</dd></div>
          </dl>
        </el-card>
      </el-col>
      <el-col :xs="24" :md="12">
        <el-card shadow="never" class="home-panel">
          <template #header>使用说明</template>
          <ul class="usage-list">
            <li>企业管理员维护本企业账号、角色和员工仓库范围。</li>
            <li>ERP 先维护商品、仓库，再保存草稿并确认入库或出库。</li>
            <li>已过账单据不能直接修改；通过关联退货、冲销或盘点纠错，保留历史流水。</li>
            <li>当前不包含渠道店铺、物流、金额退款和财务核算。</li>
          </ul>
        </el-card>
      </el-col>
    </el-row>
    <p class="home-credit">界面基于 RuoYi-Vue3 扩展，后端为本项目的 Go 实现；参考项目的版本与更新日志不代表本项目版本。</p>
  </div>
</template>

<script setup name="Index">
import { computed } from 'vue'
import useUserStore from '@/store/modules/user'
import { identitySummary } from '@/utils/sessionIdentity.mjs'

const userStore = useUserStore()
const summary = computed(() => identitySummary(userStore.identity, userStore.name))
</script>

<style scoped>
.home { color: var(--el-text-color-regular); }
.home h1 { margin: 0 0 10px; font-size: 22px; color: var(--el-text-color-primary); }
.home-intro, .home-credit { margin: 0 0 16px; line-height: 1.7; overflow-wrap: anywhere; }
.home-credit { color: var(--el-text-color-secondary); font-size: 12px; }
.home-panel { margin-bottom: 16px; }
.workspace-details { display: flex; flex-wrap: wrap; gap: 12px 32px; margin: 0 0 16px; }
.workspace-details > div, .technology-details > div { display: flex; gap: 12px; min-width: 0; }
.workspace-details dt, .technology-details dt { flex-shrink: 0; color: var(--el-text-color-secondary); }
.workspace-details dd, .technology-details dd { margin: 0; overflow-wrap: anywhere; }
.technology-details { margin: 0; }
.technology-details > div + div { margin-top: 12px; }
.usage-list { margin: 0; padding-left: 18px; line-height: 1.8; }
.usage-list li + li { margin-top: 6px; }
</style>
