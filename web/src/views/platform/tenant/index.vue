<template>
  <div class="app-container">
    <div class="tenant-heading"><div><h2>租户管理</h2><p>开通和管理企业工作空间，企业之间的数据与权限保持隔离。</p></div><el-tag effect="plain">平台专属</el-tag></div>
    <el-alert title="平台租户管理：停用、修改到期时间或交接所有者，会让该企业原有会话失效。停用不会删除业务数据。" type="info" :closable="false" class="mb12" />
    <el-form :inline="true" @submit.prevent="search">
      <el-form-item label="企业名称"><el-input v-model="query.tenantName" clearable @keyup.enter="search" /></el-form-item>
      <el-form-item><el-button @click="search">查询</el-button><el-button v-hasPermi="['platform:tenant:add']" type="primary" @click="openCreate">开通企业</el-button></el-form-item>
    </el-form>
    <el-table :data="rows" v-loading="loading">
      <el-table-column prop="tenantId" label="ID" width="100" />
      <el-table-column prop="tenantCode" label="企业编码" />
      <el-table-column prop="tenantName" label="企业名称" />
      <el-table-column prop="ownerUserId" label="所有者ID" width="110" />
      <el-table-column label="状态" width="100"><template #default="{row}"><el-tag :type="row.status === '0' ? 'success' : row.status === '3' ? 'warning' : 'info'">{{ tenantStatus(row.status) }}</el-tag></template></el-table-column>
      <el-table-column prop="expireTime" label="到期时间"><template #default="{row}">{{ row.expireTime || '长期有效' }}</template></el-table-column>
      <el-table-column label="操作" width="285" fixed="right"><template #default="{row}">
        <el-button v-hasPermi="['platform:tenant:edit']" :disabled="!['0','1'].includes(row.status)" link type="primary" @click="edit(row)">管理</el-button>
        <el-button v-hasPermi="['platform:tenant:grant']" :disabled="!['0','1'].includes(row.status)" link type="primary" @click="openMenus(row)">功能授权</el-button>
        <el-button v-hasPermi="['platform:tenant:quota']" link type="primary" @click="openQuota(row)">配额</el-button>
        <el-dropdown v-if="auth.hasPermiOr(['platform:tenant:resetPwd','platform:tenant:lifecycle'])" trigger="click" @command="command => handleMore(command,row)">
          <el-button link type="primary">更多</el-button>
          <template #dropdown><el-dropdown-menu>
            <el-dropdown-item v-if="auth.hasPermi('platform:tenant:resetPwd') && ['0','1'].includes(row.status)" command="resetPassword">重置管理员密码</el-dropdown-item>
            <template v-if="auth.hasPermi('platform:tenant:lifecycle')"><el-dropdown-item v-for="action in lifecycleActions(row.status)" :key="action" :command="action">{{ lifecycleLabels[action] }}</el-dropdown-item></template>
          </el-dropdown-menu></template>
        </el-dropdown>
      </template></el-table-column>
    </el-table>
    <pagination v-show="total > 0" :total="total" v-model:page="query.pageNum" v-model:limit="query.pageSize" @pagination="load" />
    <tenant-quota-dialog v-model="quotaVisible" :tenant-id="operationTenantId" @saved="load" />
    <tenant-lifecycle-dialog v-model="lifecycleVisible" :tenant-id="operationTenantId" :action="lifecycleAction" @saved="load" />
    <el-dialog v-model="visible" :title="form.tenantId ? '管理企业' : '开通企业'" width="520px" destroy-on-close>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="企业编码" prop="tenantCode"><el-input v-model="form.tenantCode" :disabled="!!form.tenantId" placeholder="3到32位小写字母、数字或连字符" /></el-form-item>
        <el-form-item label="企业名称" prop="tenantName"><el-input v-model="form.tenantName" maxlength="100" /></el-form-item>
        <template v-if="!form.tenantId">
          <el-form-item label="管理员账号" prop="userName"><el-input v-model="form.userName" maxlength="20" /></el-form-item>
          <el-form-item label="初始密码" prop="password"><el-input v-model="form.password" type="password" show-password autocomplete="new-password" placeholder="8到20位，请单独安全交付" /></el-form-item>
        </template>
        <el-form-item v-else label="所有者ID"><el-input-number v-model="form.ownerUserId" :min="1" :precision="0" /><div>只能交接给本企业已启用员工。</div></el-form-item>
        <el-form-item label="状态"><el-radio-group v-model="form.status"><el-radio value="0">正常</el-radio><el-radio value="1">停用</el-radio></el-radio-group></el-form-item>
        <el-form-item label="到期时间"><el-date-picker v-model="form.expireTime" type="datetime" value-format="YYYY-MM-DD HH:mm:ss" placeholder="留空长期有效" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="visible = false">取消</el-button><el-button type="primary" :loading="saving" @click="submit">保存</el-button></template>
    </el-dialog>
    <el-dialog v-model="menusVisible" title="企业功能授权" width="680px" destroy-on-close :close-on-click-modal="false">
      <div v-loading="menusLoading">
        <p class="target-name">{{ menuSettings.tenantName }} <span>({{ menuSettings.tenantCode }})</span></p>
        <el-alert title="保存后该企业全部旧会话失效。撤回的普通角色权限会被删除，重新开通功能也不会自动恢复。" type="warning" :closable="false" class="mb12" />
        <div class="tree-toolbar"><el-button size="small" @click="selectMenus(true)">全选</el-button><el-button size="small" @click="selectMenus(false)">清空</el-button><span>可单独勾选菜单与按钮，保存时自动补齐上级目录。</span></div>
        <el-tree ref="menuTreeRef" class="menu-tree" :data="menuSettings.menus" node-key="id" show-checkbox check-strictly :props="{ label: 'label', children: 'children', disabled: 'disabled' }" :default-checked-keys="menuSettings.checkedKeys" :expand-on-click-node="false" />
        <el-form ref="menusFormRef" :model="menusForm" :rules="menusRules" label-width="110px" class="action-form">
          <el-form-item label="确认企业编码" prop="confirmTenantCode"><el-input v-model="menusForm.confirmTenantCode" :placeholder="`请输入 ${menuSettings.tenantCode || '目标企业编码'}`" autocomplete="off" /></el-form-item>
          <el-form-item label="操作原因" prop="reason"><el-input v-model="menusForm.reason" type="textarea" maxlength="200" show-word-limit /></el-form-item>
        </el-form>
      </div>
      <template #footer><el-button @click="menusVisible = false">取消</el-button><el-button type="primary" :loading="menusSaving" :disabled="menusLoading || !menuSettings.tenantId" @click="submitMenus">保存授权</el-button></template>
    </el-dialog>
    <el-dialog v-model="passwordVisible" title="重置企业管理员密码" width="540px" destroy-on-close :close-on-click-modal="false" @closed="clearPassword">
      <p class="target-name">{{ passwordTarget.tenantName }} <span>({{ passwordTarget.tenantCode }})</span></p>
      <el-alert :title="`仅重置当前所有者（用户ID ${passwordTarget.ownerUserId}），不会修改员工密码；该企业全部旧会话将失效。`" type="warning" :closable="false" class="mb12" />
      <el-form ref="passwordFormRef" :model="passwordForm" :rules="passwordRules" label-width="130px" autocomplete="off">
        <el-form-item label="确认企业编码" prop="confirmTenantCode"><el-input v-model="passwordForm.confirmTenantCode" :placeholder="`请输入 ${passwordTarget.tenantCode || '目标企业编码'}`" /></el-form-item>
        <el-form-item label="你的平台密码" prop="operatorPassword"><el-input v-model="passwordForm.operatorPassword" type="password" show-password autocomplete="current-password" placeholder="再次验证平台管理员身份" /></el-form-item>
        <el-form-item label="管理员新密码" prop="newPassword"><el-input v-model="passwordForm.newPassword" type="password" show-password autocomplete="new-password" placeholder="8到20个字符，请单独安全交付" /></el-form-item>
        <el-form-item label="确认新密码" prop="confirmPassword"><el-input v-model="passwordForm.confirmPassword" type="password" show-password autocomplete="new-password" /></el-form-item>
        <el-form-item label="操作原因" prop="reason"><el-input v-model="passwordForm.reason" type="textarea" maxlength="200" show-word-limit /></el-form-item>
      </el-form>
      <template #footer><el-button @click="passwordVisible = false">取消</el-button><el-button type="danger" :loading="passwordSaving" @click="submitPassword">确认重置</el-button></template>
    </el-dialog>
  </div>
</template>
<script setup>
import { listTenants, saveTenant, getTenantMenus, saveTenantMenus, resetTenantOwnerPassword } from '@/api/platform/tenant'
import { ElMessage, ElMessageBox } from 'element-plus'
import { menuNodeIds, tenantMenusPayload, emptyOwnerPasswordForm } from '@/utils/tenantAdmin.mjs'
import { tenantStatus, lifecycleActions, lifecycleLabels } from '@/utils/tenantOperations.mjs'
import TenantQuotaDialog from './TenantQuotaDialog.vue'
import TenantLifecycleDialog from './TenantLifecycleDialog.vue'
import auth from '@/plugins/auth'
const rows = ref([]), total = ref(0), loading = ref(false), visible = ref(false), saving = ref(false), formRef = ref()
const query = reactive({ tenantName: '', pageNum: 1, pageSize: 10 })
const form = ref({})
const quotaVisible=ref(false),lifecycleVisible=ref(false),operationTenantId=ref(),lifecycleAction=ref('')
function openQuota(row){operationTenantId.value=row.tenantId;quotaVisible.value=true}
function handleMore(command,row){if(command==='resetPassword'){openPassword(row);return}operationTenantId.value=row.tenantId;lifecycleAction.value=command;lifecycleVisible.value=true}
const required = message => [{ required: true, message, trigger: 'blur' }]
const rules = { tenantCode: required('请输入企业编码'), tenantName: required('请输入企业名称'), userName: required('请输入管理员账号'), password: required('请输入初始密码') }
const menusVisible = ref(false), menusLoading = ref(false), menusSaving = ref(false), menuTreeRef = ref(), menusFormRef = ref()
const menuSettings = ref({ menus: [], checkedKeys: [] })
const menusForm = reactive({ confirmTenantCode: '', reason: '' })
const confirmCode = target => (_rule, value, callback) => value === target.value.tenantCode ? callback() : callback(new Error('企业编码不一致'))
const menusRules = { confirmTenantCode: [{ validator: confirmCode(menuSettings), trigger: 'blur' }], reason: required('请填写操作原因') }
const passwordVisible = ref(false), passwordSaving = ref(false), passwordFormRef = ref(), passwordTarget = ref({})
const passwordForm = ref(emptyOwnerPasswordForm())
const passwordRules = {
  confirmTenantCode: [{ validator: confirmCode(passwordTarget), trigger: 'blur' }],
  operatorPassword: required('请输入你的平台管理员密码'), reason: required('请填写操作原因'),
  newPassword: [{ required: true, validator: (_rule, value, cb) => value && [...value].length >= 8 && [...value].length <= 20 ? cb() : cb(new Error('密码需8到20个字符')), trigger: 'blur' }],
  confirmPassword: [{ validator: (_rule, value, cb) => value && value === passwordForm.value.newPassword ? cb() : cb(new Error('两次输入的新密码不一致')), trigger: 'blur' }]
}
let menuRequest = 0
async function openMenus(row) {
  const request = ++menuRequest
  menusLoading.value = true
  menusVisible.value = true
  menuSettings.value = { menus: [], checkedKeys: [] }
  menusForm.confirmTenantCode = ''; menusForm.reason = ''
  try {
    const res = await getTenantMenus(row.tenantId)
    if (request !== menuRequest || !menusVisible.value) return
    menuSettings.value = res.data
    await nextTick()
    menuTreeRef.value?.setCheckedKeys(res.data.checkedKeys)
  } catch (_) { if (request === menuRequest) menusVisible.value = false }
  finally { if (request === menuRequest) menusLoading.value = false }
}
function selectMenus(all) { menuTreeRef.value?.setCheckedKeys(all ? menuNodeIds(menuSettings.value.menus) : []) }
async function submitMenus() {
  await menusFormRef.value.validate()
  const keys = menuTreeRef.value.getCheckedKeys()
  try {
    await ElMessageBox.confirm(keys.length ? '确认更新该企业功能授权，并让其全部旧会话失效？' : '当前未选择任何功能，确认清空该企业全部功能授权？', '确认授权', { type: 'warning' })
  } catch (_) { return }
  menusSaving.value = true
  try {
    await saveTenantMenus(menuSettings.value.tenantId, tenantMenusPayload(menuSettings.value, keys, menusForm.confirmTenantCode, menusForm.reason))
    ElMessage.success('授权已更新，该企业用户需重新登录')
    menusVisible.value = false
    await load()
  } finally { menusSaving.value = false }
}
function openPassword(row) { passwordTarget.value = { ...row }; passwordForm.value = emptyOwnerPasswordForm(row); passwordVisible.value = true }
function clearPassword() { passwordForm.value = emptyOwnerPasswordForm(); passwordTarget.value = {} }
async function submitPassword() {
  await passwordFormRef.value.validate()
  try { await ElMessageBox.confirm('确认重置目标企业管理员密码？该企业全部旧会话会失效，操作将写入审计日志。', '确认重置', { type: 'warning' }) }
  catch (_) { return }
  passwordSaving.value = true
  try {
    const res = await resetTenantOwnerPassword(passwordTarget.value.tenantId, { ...passwordForm.value })
    passwordVisible.value = false
    if (res.data.retryCounterCleared) ElMessage.success('密码已重置，旧会话已失效，请安全交付新密码')
    else ElMessage.warning('密码已重置，但登录失败计数清理未完成，请稍后重试登录（锁定最长10分钟）')
    await load()
  } finally {
    passwordForm.value.newPassword = ''; passwordForm.value.confirmPassword = ''; passwordForm.value.operatorPassword = ''
    passwordSaving.value = false
  }
}
async function load() { loading.value = true; try { const res = await listTenants(query); rows.value = res.rows; total.value = res.total } finally { loading.value = false } }
function search() { query.pageNum = 1; load() }
function openCreate() { form.value = { tenantCode: '', tenantName: '', userName: 'admin', password: '', status: '0', expireTime: null }; visible.value = true }
function edit(row) { form.value = { ...row, password: undefined }; visible.value = true }
async function submit() {
  await formRef.value.validate(); saving.value = true
  try { await saveTenant(form.value); ElMessage.success('保存成功'); visible.value = false; form.value.password = ''; await load() } finally { saving.value = false }
}
load()
</script>
<style scoped>
.mb12 { margin-bottom: 12px; }
.tenant-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.tenant-heading h2 { margin: 0; font-size: 19px; color: var(--el-text-color-primary); }
.tenant-heading p { margin: 6px 0 0; font-size: 13px; color: var(--el-text-color-secondary); }
.target-name { margin: 0 0 12px; font-weight: 600; overflow-wrap: anywhere; }
.target-name span { color: var(--el-text-color-secondary); font-weight: 400; }
.tree-toolbar { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-bottom: 8px; }
.tree-toolbar span { font-size: 12px; color: var(--el-text-color-secondary); }
.menu-tree { max-height: 42vh; overflow-y: auto; padding: 10px; border: 1px solid var(--el-border-color); border-radius: 4px; }
.action-form { margin-top: 18px; }
</style>
