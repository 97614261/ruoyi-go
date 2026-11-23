<script setup lang="ts">
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { PhArrowsClockwise, PhEye, PhEyeSlash, PhList, PhSignOut, PhWarningCircle, PhX } from '@phosphor-icons/vue'
import CreationLaunchpad from './CreationLaunchpad.vue'
import Projects from './Projects.vue'
import Templates from './Templates.vue'
import Assets from './Assets.vue'
import Runs from './Runs.vue'
import ProjectEditor from './ProjectEditor.vue'
import Eligibility from './Eligibility.vue'
import GettingStarted from './GettingStarted.vue'
import ThemeSwitch from './ThemeSwitch.vue'
import StudioBrand from './StudioBrand.vue'
import StudioNavigation from './StudioNavigation.vue'
import StudioPoints from './StudioPoints.vue'
import { productLocation, studioLocation, type Section } from './navigation'
import { APIError, request } from './api'
import type { Bootstrap, Captcha, CreatorEligibility } from './contracts'

const tokenKey = 'pod-studio.session'
const ProductStudio = defineAsyncComponent(() => import('./ProductStudio.vue'))
const token = ref(sessionStorage.getItem(tokenKey) || '')
const bootstrap = ref<Bootstrap | null>(null)
const captcha = ref<Captcha | null>(null)
const userName = ref('')
const password = ref('')
const showPassword = ref(false)
const code = ref('')
const pending = ref(false)
const error = ref('')
const loginError = ref(''), captchaLoading = ref(false)
const loginDialog = ref<HTMLDialogElement>()
const createOnEntry = ref(false)
let loginIntent: Section | 'create' | undefined
let captchaRequest = 0
const section = ref<Section>('home'), initialRunId = ref('')
const mobileNavigation = ref<HTMLDialogElement>()
const helpExpanded = ref(false)
const activeProject = ref('')
const productProject = ref('')
let returnProduct: string | null = null
const eligibility = ref<CreatorEligibility | null>(null)
const displayName = computed(() => bootstrap.value?.nickName || bootstrap.value?.userName || '')

async function loadCaptcha(keepError = false) {
  const sequence = ++captchaRequest
  captcha.value = null
  code.value = ''
  captchaLoading.value = true
  if (!keepError) loginError.value = ''
  try {
    const result = await request<Captcha>('/captchaImage')
    if (sequence === captchaRequest) captcha.value = result
  } catch (e) {
    if (sequence === captchaRequest && !loginError.value) loginError.value = (e as Error).message
  } finally {
    if (sequence === captchaRequest) captchaLoading.value = false
  }
}
function showLogin(intent?: Section | 'create') {
  if (!loginDialog.value || loginDialog.value.open) return
  mobileNavigation.value?.close()
  loginIntent = intent
  loginDialog.value.showModal()
  void loadCaptcha()
}
function closeLogin() {
  loginDialog.value?.close()
  loginIntent = undefined
  captchaRequest++
  captchaLoading.value = false
  captcha.value = null
  loginError.value = ''
  password.value = ''
  showPassword.value = false
  code.value = ''
}
function dismissLogin() {
  if (!pending.value) closeLogin()
}
async function enterStudio() {
  pending.value = true
  error.value = ''
  bootstrap.value = null
  eligibility.value = null
  try {
    eligibility.value = (await request<{data: CreatorEligibility}>('/pod/eligibility', {}, token.value)).data
    bootstrap.value = (await request<{ data: Bootstrap }>('/pod/bootstrap', {}, token.value)).data
    if (eligibility.value.creator.expired) bootstrap.value.capabilities.projectEditing = false
    restoreLocation()
  } catch (e) {
    error.value = (e as Error).message
    if (e instanceof APIError && e.code === 401) expired()
  } finally { pending.value = false }
}
function openProject(id: string) {
  returnProduct = section.value === 'product' ? productProject.value : null
  activeProject.value = id
  history.replaceState(null, '', `${location.pathname}${location.search}#project/${id}`)
}
function restoreLocation() {
  const hash = token.value ? location.hash : ''
  const target = studioLocation(hash)
  section.value = target.section; activeProject.value = target.projectId; initialRunId.value = target.runId
  productProject.value = productLocation(hash)
}
function navigate(target: Section) {
  if (!token.value && target !== 'home') { showLogin(target); return }
  mobileNavigation.value?.close()
  helpExpanded.value = false
  returnProduct = null
  section.value = target; activeProject.value = ''; initialRunId.value = ''; productProject.value = ''
  history.replaceState(null, '', `${location.pathname}${location.search}#${target}`)
}
function rememberRun(run: { runId: string }) {
  initialRunId.value = run.runId
  history.replaceState(null, '', `${location.pathname}${location.search}#runs/${run.runId}`)
}
function forgetRun(id: string) { if (id === initialRunId.value) navigate('runs') }
function closeProject() {
  if (returnProduct !== null) openProduct(returnProduct)
  else navigate('projects')
}
function openProduct(id = '') {
  navigate('product')
  productProject.value = id
  if (id) history.replaceState(null, '', `${location.pathname}${location.search}#product/${id}`)
}
function expired() {
  token.value = ''; bootstrap.value = null; eligibility.value = null
  createOnEntry.value = false; helpExpanded.value = false; returnProduct = null
  sessionStorage.removeItem(tokenKey); restoreLocation()
}
async function login() {
  if (pending.value || !loginDialog.value?.open || !captcha.value) return
  loginError.value = ''; pending.value = true
  try {
    const result = await request<{ token: string }>('/login', {
      method: 'POST', body: JSON.stringify({ username: userName.value.trim(), password: password.value, code: code.value, uuid: captcha.value.uuid || '' }),
    })
    if (typeof result.token !== 'string' || !result.token) throw new APIError('登录响应不完整，请重试', 500)
    const intent = loginIntent
    token.value = result.token; sessionStorage.setItem(tokenKey, result.token)
    closeLogin()
    if (intent === 'create') { navigate('home'); createOnEntry.value = true }
    else if (intent) navigate(intent)
    await enterStudio()
  } catch (e) {
    loginError.value = (e as Error).message
    await loadCaptcha(true)
  }
  finally { pending.value = false }
}
async function logout() {
  mobileNavigation.value?.close()
  pending.value = true; error.value = ''
  try {
    await request('/logout', { method: 'POST' }, token.value)
    expired(); navigate('home')
  } catch (e) { error.value = (e as Error).message }
  finally { pending.value = false }
}
onMounted(() => {
  window.addEventListener('hashchange', restoreLocation)
  if (token.value) void enterStudio()
})
onBeforeUnmount(() => { captchaRequest++; window.removeEventListener('hashchange', restoreLocation) })
</script>

<template>
  <ProjectEditor v-if="token && activeProject && bootstrap" :key="activeProject" :token="token" :bootstrap="bootstrap" :project-id="activeProject" @close="closeProject" @open="openProject" @expired="expired" />
  <div v-else class="studio-layout studio-v2">
    <aside class="studio-sidebar">
      <button type="button" class="studio-home-link" aria-label="返回首页" @click="navigate('home')"><StudioBrand /></button>
      <StudioNavigation :section="section" @navigate="navigate" />
    </aside>
    <header class="studio-header">
      <div class="studio-header-start"><button class="icon-button mobile-navigation-toggle" aria-label="打开导航" @click="mobileNavigation?.showModal()"><PhList :size="21" /></button><button type="button" class="studio-home-link" aria-label="返回首页" @click="navigate('home')"><StudioBrand /></button></div>
      <div class="account">
        <ThemeSwitch />
        <template v-if="token"><StudioPoints v-if="bootstrap" :token="token" :managed="bootstrap.execution?.mode === 'managed'" :refresh-key="section" @open="navigate('eligibility')" @expired="expired" /><button class="account-profile" aria-label="查看我的资格与点数" @click="navigate('eligibility')"><span class="avatar">{{ displayName.slice(0, 1) || 'P' }}</span><span class="account-name">{{ displayName }}</span></button><button class="icon-button signout-button" aria-label="退出登录" :disabled="pending" @click="logout"><PhSignOut :size="19" /></button></template>
        <button v-else class="primary-button studio-login-trigger" type="button" @click="showLogin()">登录</button>
      </div>
    </header>
    <main class="studio-main">
      <section v-if="!token" class="workspace-home" aria-label="创作首页">
        <CreationLaunchpad @create="showLogin('create')" @templates="navigate('templates')" @product="navigate('product')" />
      </section>
      <Eligibility v-else-if="section === 'eligibility'" :token="token" @expired="expired" @change="eligibility=$event" />
      <div v-else-if="pending && !bootstrap" class="loading-state" role="status"><span class="skeleton" /><span class="skeleton short" /><p>正在打开工作空间…</p></div>
      <div v-else-if="!bootstrap" class="connection-error"><PhWarningCircle :size="36" /><h1>暂时无法进入工作空间</h1><p role="alert">{{ error }}</p><p>账号开通、续期和恢复资格请联系管理员。</p><button class="secondary-button" @click="navigate('eligibility')">查看资格与点数</button><button class="primary-button" @click="enterStudio"><PhArrowsClockwise :size="18" />重新检查</button></div>
      <template v-else>
        <p v-if="error" class="error-message" role="alert">{{ error }}</p>
        <p v-if="eligibility?.creator.expired" class="error-message" role="status">创作资格已到期，可查看和下载已有内容；新增、编辑、上传和运行需要管理员续期。</p>
        <template v-if="section === 'home' || section === 'projects'">
          <Projects :key="section" :home="section === 'home'" :token="token" :bootstrap="bootstrap" :start-create="createOnEntry" @create-entry-consumed="createOnEntry = false" @open="openProject" @navigate="navigate" @expired="expired" />
          <details v-if="section === 'home'" class="workspace-help" @toggle="helpExpanded = ($event.target as HTMLDetailsElement).open"><summary>使用帮助</summary><GettingStarted v-if="helpExpanded" :token="token" :bootstrap="bootstrap" :eligibility="eligibility" @navigate="navigate" @expired="expired" /></details>
        </template>
        <ProductStudio v-else-if="section === 'product'" :key="productProject" :project-id="productProject" :token="token" :bootstrap="bootstrap" @open="openProduct" @canvas="openProject" @expired="expired" />
        <Templates v-else-if="section === 'templates'" initial-tab="official" :token="token" :bootstrap="bootstrap" @open="openProject" @projects="navigate('projects')" @expired="expired" />
        <Assets v-else-if="section === 'assets'" :token="token" :limits="bootstrap.assetLimits" @expired="expired" />
        <Runs v-else-if="section === 'runs'" :scope="`${bootstrap.userId}:${bootstrap.workspace.workspaceId}`" :token="token" :execution="bootstrap.execution" :initial-run-id="initialRunId" @selected="rememberRun" @unavailable="forgetRun" @project="openProject" @expired="expired" />
      </template>
    </main>
    <dialog ref="mobileNavigation" class="navigation-dialog" aria-labelledby="navigation-title" @click="event => { if (event.target === mobileNavigation) mobileNavigation?.close() }">
      <div class="navigation-dialog-heading"><h2 id="navigation-title">工作空间</h2><button class="icon-button" aria-label="关闭导航" @click="mobileNavigation?.close()"><PhX :size="21" /></button></div>
      <StudioNavigation :section="section" @navigate="navigate" />
      <button v-if="token" class="secondary-button mobile-signout" :disabled="pending" @click="logout"><PhSignOut :size="18" />退出登录</button>
      <button v-if="token" type="button" class="secondary-button mobile-eligibility" @click="navigate('eligibility')">资格与点数</button>
    </dialog>
  </div>
  <dialog ref="loginDialog" class="studio-dialog signin-dialog" aria-labelledby="signin-heading" @cancel.prevent="dismissLogin">
    <form class="signin-form" :aria-busy="pending || captchaLoading" @submit.prevent="login">
      <header class="signin-dialog-heading"><h2 id="signin-heading">登录</h2><button class="icon-button" type="button" aria-label="关闭登录" :disabled="pending" @click="dismissLogin"><PhX :size="19" /></button></header>
      <div class="field"><label for="username">账号</label><input id="username" v-model="userName" autocomplete="username" autofocus required maxlength="30" :disabled="pending" /></div>
      <div class="field"><label for="password">密码</label><div class="password-field"><input id="password" v-model="password" :type="showPassword ? 'text' : 'password'" autocomplete="current-password" required maxlength="20" :disabled="pending" /><button class="icon-button" type="button" :aria-label="showPassword ? '隐藏密码' : '显示密码'" :aria-pressed="showPassword" :disabled="pending" @click="showPassword = !showPassword"><component :is="showPassword ? PhEyeSlash : PhEye" :size="19" /></button></div></div>
      <div v-if="captcha?.captchaEnabled" class="field"><label for="captcha">验证码</label><div class="captcha-row"><input id="captcha" v-model="code" required autocomplete="off" maxlength="12" :disabled="pending" /><button class="captcha-image" type="button" aria-label="刷新验证码" :disabled="pending" @click="loadCaptcha()"><img :src="`data:image/png;base64,${captcha.img}`" alt="登录验证码，点击刷新" /></button></div></div>
      <p v-if="loginError" class="error-message" role="alert">{{ loginError }}</p>
      <button v-if="!captcha && !captchaLoading && !pending" class="primary-button signin-submit" type="button" @click="loadCaptcha()">重试</button>
      <button v-else class="primary-button signin-submit" type="submit" :disabled="pending || !captcha">{{ pending ? '正在登录…' : '登录' }}</button>
    </form>
  </dialog>
</template>
