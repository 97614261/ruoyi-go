<template>
  <el-dropdown class="user-identity" trigger="click" placement="bottom-end" @command="emit('command', $event)">
    <button class="identity-trigger" type="button" :aria-label="`企业：${summary.tenantName}；账号：${summary.userName}；角色：${summary.roleLabel}`">
      <el-avatar :src="avatar" :size="30" shape="square" class="identity-avatar" aria-hidden="true">
        {{ summary.userName.slice(0, 1).toUpperCase() }}
      </el-avatar>
      <span class="identity-lines">
        <span class="identity-tenant" :title="summary.tenantName">{{ summary.tenantName }}</span>
        <span class="identity-account" :title="`${summary.userName} · ${summary.roleLabel}`">
          <span class="identity-username">{{ summary.userName }}</span>
          <span class="identity-divider" aria-hidden="true">/</span>
          <span class="identity-role">{{ summary.roleLabel }}</span>
        </span>
      </span>
      <el-icon class="identity-chevron"><ArrowDown /></el-icon>
    </button>
    <template #dropdown>
      <el-dropdown-menu class="identity-menu">
        <li class="identity-panel" role="presentation">
          <div class="identity-workspace">{{ summary.platformAdmin ? '平台工作空间' : '当前企业' }}</div>
          <div class="identity-company">{{ summary.tenantName }}</div>
          <div v-if="summary.tenantCode" class="identity-code">企业编码：{{ summary.tenantCode }}</div>
          <dl class="identity-details">
            <div><dt>账号</dt><dd>{{ summary.userName }}</dd></div>
            <div><dt>角色</dt><dd>{{ summary.roleLabel }}</dd></div>
          </dl>
        </li>
        <el-dropdown-item v-if="summary.platformAdmin" command="tenantManagement" divided>
          <el-icon><OfficeBuilding /></el-icon>租户管理
        </el-dropdown-item>
        <el-dropdown-item v-if="!summary.platformAdmin" command="workspace" divided>企业信息与配额</el-dropdown-item>
        <el-dropdown-item command="profile" :divided="!summary.platformAdmin">个人中心</el-dropdown-item>
        <el-dropdown-item v-if="showSettings" command="setLayout">布局设置</el-dropdown-item>
        <el-dropdown-item command="lockScreen">锁定屏幕</el-dropdown-item>
        <el-dropdown-item command="logout" divided>退出登录</el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup>
import { computed } from 'vue'
import { ArrowDown, OfficeBuilding } from '@element-plus/icons-vue'
import { identitySummary } from '@/utils/sessionIdentity.mjs'

const props = defineProps({
  identity: { type: Object, default: null },
  avatar: { type: String, default: '' },
  userName: { type: String, default: '' },
  showSettings: { type: Boolean, default: false }
})
const emit = defineEmits(['command'])
const summary = computed(() => identitySummary(props.identity, props.userName))
</script>

<style scoped>
.user-identity { flex-shrink: 0; margin: 0 8px 0 6px; max-width: min(350px, 42vw); }
.identity-trigger {
  display: flex; align-items: center; gap: 9px; width: 100%; min-width: 0;
  height: 42px; padding: 4px 8px; border: 1px solid transparent; border-radius: 6px;
  background: transparent; color: var(--el-text-color-primary); cursor: pointer;
  text-align: left; font-family: inherit; line-height: 1.25;
}
.identity-trigger:hover { background: var(--el-fill-color-light); }
.identity-trigger:focus-visible { outline: 2px solid var(--el-color-primary); outline-offset: -2px; }
.identity-avatar { flex-shrink: 0; border-radius: 6px; background: var(--el-color-primary-light-9); color: var(--el-color-primary); }
.identity-lines { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.identity-tenant { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; font-weight: 600; }
.identity-account { display: flex; gap: 6px; min-width: 0; font-size: 11px; color: var(--el-text-color-secondary); }
.identity-username { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100px; flex-shrink: 0; color: var(--el-text-color-regular); }
.identity-role { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.identity-divider { color: var(--el-border-color-darker); }
.identity-chevron { margin-left: 4px; flex-shrink: 0; font-size: 11px; color: var(--el-text-color-secondary); }
.identity-menu { width: 292px; max-width: calc(100vw - 24px); }
.identity-panel { padding: 10px 16px 8px; line-height: 1.5; color: var(--el-text-color-primary); }
.identity-workspace { font-size: 11px; color: var(--el-text-color-secondary); }
.identity-company { margin-top: 4px; font-size: 15px; font-weight: 600; overflow-wrap: anywhere; }
.identity-code { margin-top: 3px; font-size: 12px; color: var(--el-text-color-secondary); overflow-wrap: anywhere; }
.identity-details { margin: 12px 0 0; font-size: 12px; }
.identity-details > div { display: grid; grid-template-columns: 34px minmax(0, 1fr); gap: 8px; margin-top: 5px; }
.identity-details dt { color: var(--el-text-color-secondary); }
.identity-details dd { margin: 0; overflow-wrap: anywhere; }
@media (max-width: 767px) {
  .user-identity { max-width: min(220px, 58vw); margin-right: 4px; }
  .identity-trigger { gap: 7px; padding-inline: 5px; }
  .identity-chevron { display: none; }
  .identity-username { max-width: 70px; }
}
</style>
