<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import {
  ClearHistory,
  DeleteSite,
  GetArchiveConfig,
  ListOpportunities,
  ListSites,
  ListTasks,
  OpenArchiveDirectory,
  RetryArchive,
  RunSGCCAutoPages1To7,
  SaveArchiveConfig,
  SaveSite,
  SelectArchiveDirectory
} from '../wailsjs/go/main/App'
import { main } from '../wailsjs/go/models'
import { BrowserOpenURL, EventsOn } from '../wailsjs/runtime/runtime'

type Tab = 'automatic' | 'opportunities' | 'sites' | 'tasks'
type AutomationProgress = {
  current: number
  total: number
  title: string
  status: string
  message: string
  percent: number
  substep: string
  substepPercent: number
}
type AutoSubstep = {
  name: string
  status: string
  percent: number
  message: string
}
type AutoStep = {
  title: string
  status: string
  message: string
  percent: number
  substeps: AutoSubstep[]
}

const autoSubstepNames = ['抓取数据', '创建文件夹', '创建 Word 文档', '下载附件', '生成招标及结果 Excel', '更新状态']

function createAutoSteps(): AutoStep[] {
  return [
    '1.1 单一来源采购事前公示',
    '1.2 年度采购计划预安排',
    '2.1 资格预审公告',
    '2.2 招标公告及投标邀请书'
  ].map((title) => ({
    title,
    status: 'pending',
    message: '等待执行',
    percent: 0,
    substeps: autoSubstepNames.map((name) => ({ name, status: 'pending', percent: 0, message: '等待执行' }))
  }))
}

const activeTab = ref<Tab>('automatic')
const loading = ref(false)
const message = ref('')
const sites = ref<main.SiteConfig[]>([])
const tasks = ref<main.CrawlTask[]>([])
const opportunities = ref<main.Opportunity[]>([])
const selectedOpportunity = ref<main.Opportunity | null>(null)
const archive = ref<main.ArchiveConfig>({ rootPath: '' })
const autoRunning = ref(false)
const autoSteps = ref<AutoStep[]>(createAutoSteps())

const query = reactive({
  search: '',
  siteId: ''
})

const siteForm = reactive(new main.SiteConfig({
  id: '',
  name: '',
  siteType: 'custom',
  baseUrl: '',
  enabled: true,
  renderMode: 'http',
  dateRangeDays: 7,
  minIntervalMs: 1500,
  maxRetries: 3,
  categories: [],
  watermarks: [],
  createdAt: '',
  updatedAt: ''
}))

const siteEditorOpen = ref(false)

const visibleOpportunities = computed(() => opportunities.value)
const autoOverallPercent = computed(() => Math.round(autoSteps.value.reduce((total, step) => total + step.percent, 0) / autoSteps.value.length))

function resetSiteForm() {
  Object.assign(siteForm, {
    id: '',
    name: '',
    siteType: 'custom',
    baseUrl: '',
    enabled: true,
    renderMode: 'http',
    dateRangeDays: 7,
    minIntervalMs: 1500,
    maxRetries: 3,
    categories: [],
    watermarks: [],
    createdAt: '',
    updatedAt: ''
  })
  siteEditorOpen.value = false
}

function editSite(site: main.SiteConfig) {
  Object.assign(siteForm, JSON.parse(JSON.stringify(site)))
  siteEditorOpen.value = true
  activeTab.value = 'sites'
}

function addSite() {
  resetSiteForm()
  siteEditorOpen.value = true
}

async function refreshAll() {
  loading.value = true
  try {
    const [siteList, taskList, archiveConfig] = await Promise.all([
      ListSites(),
      ListTasks(),
      GetArchiveConfig()
    ])
    sites.value = siteList || []
    tasks.value = taskList || []
    archive.value = archiveConfig
    await refreshOpportunities()
  } finally {
    loading.value = false
  }
}

async function refreshOpportunities() {
  opportunities.value = (await ListOpportunities({
    search: query.search,
    siteId: query.siteId
  })) || []
  if (selectedOpportunity.value) {
    selectedOpportunity.value =
      opportunities.value.find((item) => item.id === selectedOpportunity.value?.id) || null
  }
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    success: '成功',
    no_updates: '无更新',
    failed: '失败',
    running: '运行中',
    pending: '等待中',
    skipped: '已跳过'
  }
  return labels[status] || status
}

function summarizeCrawlTasks(result: main.CrawlTask[]) {
  const newCount = result.reduce((sum, task) => sum + task.newCount, 0)
  const duplicateCount = result.reduce((sum, task) => sum + task.duplicateCount, 0)
  const failedTasks = result.filter((task) => task.status === 'failed' || task.failedCount > 0)

  if (failedTasks.length) {
    const failureText = failedTasks
      .map((task) => `${task.siteName}：${task.errorMessage || '抓取失败'}`)
      .join('；')
    return `抓取完成，但有 ${failedTasks.length} 个站点失败。新增 ${newCount} 条，重复/更新 ${duplicateCount} 条。${failureText}`
  }

  return `抓取完成：新增 ${newCount} 条，重复/更新 ${duplicateCount} 条`
}


async function chooseArchiveDirectory() {
  const selected = await SelectArchiveDirectory()
  if (selected) archive.value.rootPath = selected
}

async function saveArchiveConfig() {
  loading.value = true
  try {
    archive.value = await SaveArchiveConfig({ rootPath: archive.value.rootPath })
    message.value = '公告归档目录已保存'
  } catch (error) {
    message.value = error instanceof Error ? error.message : String(error)
  } finally {
    loading.value = false
  }
}

async function retryArchive(item: main.Opportunity) {
  loading.value = true
  try {
    selectedOpportunity.value = await RetryArchive(item.id)
    message.value = '已重新执行公告归档'
    await refreshOpportunities()
  } catch (error) {
    message.value = error instanceof Error ? error.message : String(error)
  } finally {
    loading.value = false
  }
}

function updateAutoProgress(progress: AutomationProgress) {
  const index = progress.current - 1
  if (index < 0 || index >= autoSteps.value.length) return
  const step = autoSteps.value[index]
  const substep = step.substeps.find((item) => item.name === progress.substep)
  if (substep) {
    substep.status = progress.status
    substep.percent = progress.substepPercent
    substep.message = progress.message
  }
  step.title = progress.title
  step.status = progress.status
  step.message = progress.message
  step.percent = progress.percent
  const terminalNoWork = progress.status === 'no_updates' || progress.status === 'skipped'
  if (progress.substep === '更新状态' && ['success', 'no_updates', 'failed', 'skipped'].includes(progress.status)) {
    step.substeps.forEach((item) => {
      if (item.status === 'pending') {
        item.status = progress.status === 'success' ? 'success' : progress.status
        item.percent = 100
        item.message = terminalNoWork ? '无新增公告，无需执行' : progress.status === 'success' ? '本步骤已完成或不适用' : '本步骤未执行'
      }
    })
  }
}

async function startAutomaticMode() {
  activeTab.value = 'automatic'
  if (autoRunning.value) return
  autoRunning.value = true
  autoSteps.value = createAutoSteps()
  message.value = '全自动模式正在处理国网站点配置范围内的公告...'
  try {
    const result = await RunSGCCAutoPages1To7()
    message.value = summarizeCrawlTasks(result)
    await refreshAll()
  } catch (error) {
    message.value = error instanceof Error ? error.message : String(error)
  } finally {
    autoRunning.value = false
  }
}

async function clearHistory() {
  if (!confirm('将删除公告记录、任务日志、抓取水位及已归档文件。清理后需点击“执行”才会重新抓取，确认继续？')) return
  autoRunning.value = true
  try {
    const result = await ClearHistory()
    selectedOpportunity.value = null
    autoSteps.value = createAutoSteps()
    message.value = `历史已删除：${result.deletedOpportunities} 条公告、${result.deletedTasks} 条任务、${result.deletedFolders} 个归档目录。请点击“执行”重新抓取。`
    await refreshAll()
  } catch (error) {
    message.value = error instanceof Error ? error.message : String(error)
  } finally {
    autoRunning.value = false
  }
}

async function openArchiveDirectory() {
  try {
    await OpenArchiveDirectory()
  } catch (error) {
    message.value = error instanceof Error ? error.message : String(error)
  }
}

async function saveSite() {
  loading.value = true
  message.value = ''
  try {
    await SaveSite(siteForm)
    message.value = '站点配置已保存'
    resetSiteForm()
    await refreshAll()
  } catch (error) {
    message.value = String(error)
  } finally {
    loading.value = false
  }
}

async function removeSite(id: string) {
  if (!confirm('确认删除这个站点配置？历史公告不会被删除。')) return
  loading.value = true
  try {
    await DeleteSite(id)
    await refreshAll()
    message.value = '站点已删除'
  } catch (error) {
    message.value = String(error)
  } finally {
    loading.value = false
  }
}

function selectOpportunity(item: main.Opportunity) {
  selectedOpportunity.value = item
}

function openSource(url: string) {
  if (!url) return
  BrowserOpenURL(url)
}

const removeAutoProgressListener = EventsOn('automation:progress', updateAutoProgress)
onMounted(refreshAll)
onBeforeUnmount(removeAutoProgressListener)
</script>

<template>
  <main class="shell">
    <aside class="sidebar">
      <div class="brand">
        <div>
          <h1>商机提取器</h1>
          <small>公告归档与项目整理</small>
        </div>
      </div>

      <nav>
        <p class="side-title">工作区</p>
        <button :class="{ active: activeTab === 'automatic' }" @click="activeTab = 'automatic'"><span>全自动模式</span><em>4</em></button>
        <button :class="{ active: activeTab === 'opportunities' }" @click="activeTab = 'opportunities'"><span>公告库</span><em>{{ opportunities.length }}</em></button>
        <p class="side-title menu-divider">管理</p>
        <button :class="{ active: activeTab === 'sites' }" @click="activeTab = 'sites'"><span>站点配置</span><em>{{ sites.length }}</em></button>
        <button :class="{ active: activeTab === 'tasks' }" @click="activeTab = 'tasks'"><span>执行记录</span><em>{{ tasks.length }}</em></button>
      </nav>

      <div class="sidebar-footer">
        <span>国家电网</span>
        <strong>自动归档已启用</strong>
      </div>
    </aside>

    <section class="content">
      <section v-if="activeTab === 'automatic'" class="workspace">
        <div class="panel automation-panel">
          <div class="automation-toolbar">
            <button class="primary" :disabled="autoRunning" @click="startAutomaticMode">
              {{ autoRunning ? '正在执行...' : '执行' }}
            </button>
            <button :disabled="autoRunning" @click="openArchiveDirectory">打开保存目录</button>
            <button class="danger" :disabled="autoRunning" @click="clearHistory">删除历史</button>
          </div>
          <div class="automation-context">
            <div>
              <p class="eyebrow">国家电网</p>
              <h2>全自动归档</h2>
            </div>
          </div>
          <p v-if="message" class="automation-message">{{ message }}</p>
          <div class="overall-progress">
            <span>总进度</span>
            <strong>{{ autoOverallPercent }}%</strong>
          </div>
          <progress :value="autoOverallPercent" max="100" />
          <ol class="automation-steps">
            <li v-for="step in autoSteps" :key="step.title" :class="step.status">
              <div class="automation-step-head">
                <strong>{{ step.title }}</strong>
                <span>{{ step.percent }}% · {{ step.message }}</span>
              </div>
              <span>{{ statusLabel(step.status) }}</span>
              <progress :value="step.percent" max="100" />
              <div class="automation-substeps">
                <div v-for="substep in step.substeps" :key="substep.name" class="automation-substep">
                  <div>
                    <span>{{ substep.name }}</span>
                    <span>{{ substep.percent }}%</span>
                  </div>
                  <progress :value="substep.percent" max="100" />
                  <small>{{ substep.message }}</small>
                </div>
              </div>
            </li>
          </ol>
        </div>
      </section>

      <section v-else-if="activeTab === 'opportunities'" class="workspace two-column">
        <div class="panel">
          <div class="toolbar">
            <input v-model="query.search" placeholder="搜索标题、正文、来源..." @input="refreshOpportunities" />
            <select v-model="query.siteId" @change="refreshOpportunities">
              <option value="">全部站点</option>
              <option v-for="site in sites" :key="site.id" :value="site.id">{{ site.name }}</option>
            </select>
          </div>

          <div class="list">
            <article
              v-for="item in visibleOpportunities"
              :key="item.id"
              class="opportunity"
              :class="{ selected: selectedOpportunity?.id === item.id }"
              @click="selectOpportunity(item)"
            >
              <div>
                <h3>{{ item.title }}</h3>
                <p>{{ item.sourceSite }} · {{ item.noticeType || '公告' }} · {{ item.publishTime || '未知时间' }}</p>
              </div>
              <div v-if="item.matchedKeywords?.length" class="tags">
                <span v-for="keyword in item.matchedKeywords" :key="keyword">{{ keyword }}</span>
              </div>
            </article>
            <div v-if="!visibleOpportunities.length" class="empty">暂无公告。请在全自动模式中执行抓取。</div>
          </div>
        </div>

        <aside class="panel detail">
          <template v-if="selectedOpportunity">
            <div class="detail-head">
              <h3>{{ selectedOpportunity.title }}</h3>
              <div class="row-actions">
                <button @click="openSource(selectedOpportunity.sourceUrl)">打开原文</button>
                <button :disabled="loading" @click="retryArchive(selectedOpportunity)">重新归档</button>
              </div>
            </div>
            <dl>
              <dt>来源</dt>
              <dd>{{ selectedOpportunity.sourceSite }}</dd>
              <dt>类型</dt>
              <dd>{{ selectedOpportunity.noticeType || '公告' }}</dd>
              <dt>发布时间</dt>
              <dd>{{ selectedOpportunity.publishTime || '未识别' }}</dd>
              <dt>发布单位</dt>
              <dd>{{ selectedOpportunity.buyer || '未识别' }}</dd>
              <dt>项目状态</dt>
              <dd>{{ selectedOpportunity.region || '未识别' }}</dd>
              <dt>编号</dt>
              <dd>{{ selectedOpportunity.tenderNo || '未识别' }}</dd>
              <dt>截止时间</dt>
              <dd>{{ selectedOpportunity.deadline || '未识别' }}</dd>
              <dt>处理状态</dt>
              <dd>{{ selectedOpportunity.processStatus || '待处理' }}</dd>
              <dt>归档目录</dt>
              <dd>{{ selectedOpportunity.archivePath || '尚未归档' }}</dd>
              <dt>附件</dt>
              <dd>{{ selectedOpportunity.attachments?.length || 0 }} 个</dd>
            </dl>
            <p v-if="selectedOpportunity.archiveError" class="content-text">归档说明：{{ selectedOpportunity.archiveError }}</p>
            <p class="content-text">{{ selectedOpportunity.content }}</p>
          </template>
          <div v-else class="empty">选择一条公告查看详情。</div>
        </aside>
      </section>

      <section v-else-if="activeTab === 'sites'" class="workspace site-workspace">
        <div class="panel archive-panel">
          <form class="form" @submit.prevent="saveArchiveConfig">
            <h3>公告归档目录</h3>
            <label>
              下载保存位置
              <input v-model="archive.rootPath" required placeholder="D:\\商机提取器归档" />
            </label>
            <div class="actions">
              <button type="button" :disabled="loading" @click="chooseArchiveDirectory">选择目录</button>
              <button class="primary" type="submit" :disabled="loading">保存目录</button>
            </div>
          </form>
        </div>

        <div class="panel site-list-panel">
          <div class="section-head">
            <h3>已配置站点</h3>
            <button class="primary" @click="addSite">添加站点</button>
          </div>
          <article v-for="site in sites" :key="site.id" class="site-row clickable" @click="editSite(site)">
            <div>
              <h4>{{ site.name }}</h4>
              <template v-if="site.siteType === 'sgcc'">
                <p>国网全自动流程 · 已启用步骤：{{ site.categories?.filter((item) => item.enabled).length || 0 }}</p>
                <ul class="configured-paths">
                  <li v-for="category in site.categories" :key="category.id">
                    <strong>{{ category.name }}</strong>
                    <span>{{ category.pagePath }}</span>
                  </li>
                </ul>
              </template>
              <template v-else>
                <p>{{ site.baseUrl }}</p>
                <span>{{ site.siteType }} · {{ site.renderMode }} · {{ site.enabled ? '启用' : '停用' }}</span>
              </template>
            </div>
            <button class="danger" @click.stop="removeSite(site.id)">删除</button>
          </article>
          <div v-if="!sites.length" class="empty">暂无站点。添加站点后可进行配置。</div>
        </div>

        <form v-if="siteEditorOpen" class="panel form" @submit.prevent="saveSite">
          <div class="section-head">
            <h3>{{ siteForm.id ? '编辑站点' : '添加站点' }}</h3>
            <button type="button" @click="resetSiteForm">关闭</button>
          </div>
          <label>
            站点名称
            <input v-model="siteForm.name" required placeholder="国家电网 - 招标采购公告" />
          </label>
          <label v-if="siteForm.siteType !== 'sgcc'">
            入口 URL
            <input v-model="siteForm.baseUrl" required placeholder="https://example.com/notices" />
          </label>
          <div v-if="siteForm.siteType !== 'sgcc'" class="grid-2">
            <label>
              站点类型
              <select v-model="siteForm.siteType">
                <option value="sgcc">国家电网</option>
                <option value="csg">南方电网</option>
                <option value="custom">自定义</option>
              </select>
            </label>
            <label>
              抓取模式
              <select v-model="siteForm.renderMode">
                <option value="http">HTTP 静态抓取</option>
                <option value="browser">浏览器渲染（未启用）</option>
              </select>
            </label>
          </div>
          <div :class="siteForm.siteType === 'sgcc' ? 'grid-1' : 'grid-3'">
            <label>
              最近天数
              <input v-model.number="siteForm.dateRangeDays" min="1" type="number" />
            </label>
            <label v-if="siteForm.siteType !== 'sgcc'">
              间隔毫秒
              <input v-model.number="siteForm.minIntervalMs" min="300" type="number" />
            </label>
            <label v-if="siteForm.siteType !== 'sgcc'">
              重试次数
              <input v-model.number="siteForm.maxRetries" min="1" type="number" />
            </label>
          </div>
          <label class="check">
            <input v-model="siteForm.enabled" type="checkbox" />
            启用站点
          </label>
          <div class="actions">
            <button class="primary" type="submit">保存站点</button>
            <button type="button" @click="resetSiteForm">取消</button>
          </div>
          <section v-if="siteForm.siteType === 'sgcc'" class="category-settings">
            <h3>国网全自动步骤与指定路径</h3>
            <label v-for="category in siteForm.categories" :key="category.id" class="check">
              <input v-model="category.enabled" type="checkbox" />
              <span>
                <strong>{{ category.name }}</strong>
                <small>{{ category.pagePath }}</small>
              </span>
            </label>
          </section>
        </form>

      </section>

      <section v-else class="workspace">
        <div class="panel">
          <table>
            <thead>
              <tr>
                <th>站点</th>
                <th>栏目</th>
                <th>状态</th>
                <th>新增</th>
                <th>重复/更新</th>
                <th>失败</th>
                <th>开始时间</th>
                <th>错误</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="task in tasks" :key="task.id">
                <td>{{ task.siteName }}</td>
                <td>{{ task.categoryName || '-' }}</td>
                <td>{{ statusLabel(task.status) }}</td>
                <td>{{ task.newCount }}</td>
                <td>{{ task.duplicateCount }}</td>
                <td>{{ task.failedCount }}</td>
                <td>{{ task.startedAt }}</td>
                <td>{{ task.errorMessage }}</td>
              </tr>
            </tbody>
          </table>
          <div v-if="!tasks.length" class="empty">暂无任务日志。</div>
        </div>
      </section>
    </section>
  </main>
</template>
