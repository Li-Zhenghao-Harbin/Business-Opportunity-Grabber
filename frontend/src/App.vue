<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Dashboard,
  DeleteSite,
  GetArchiveConfig,
  GetSchedule,
  ListOpportunities,
  ListSites,
  ListTasks,
  RetryArchive,
  RunCrawl,
  SaveArchiveConfig,
  SaveSchedule,
  SaveSite,
  SelectArchiveDirectory
} from '../wailsjs/go/main/App'
import { main } from '../wailsjs/go/models'
import { BrowserOpenURL } from '../wailsjs/runtime/runtime'

type Tab = 'opportunities' | 'sites' | 'schedule' | 'tasks'

const activeTab = ref<Tab>('opportunities')
const loading = ref(false)
const message = ref('')
const crawlElapsedSeconds = ref(0)
let crawlElapsedTimer: ReturnType<typeof setInterval> | null = null
const crawlTimeoutMs = 60_000
const dashboard = ref<main.Dashboard>({
  siteCount: 0,
  enabledSiteCount: 0,
  opportunityCount: 0,
  lastTaskCount: 0
})
const sites = ref<main.SiteConfig[]>([])
const tasks = ref<main.CrawlTask[]>([])
const opportunities = ref<main.Opportunity[]>([])
const selectedOpportunity = ref<main.Opportunity | null>(null)
const savingSchedule = ref(false)
const schedule = ref<main.ScheduleConfig>({
  enabled: false,
  mode: 'interval',
  intervalMinutes: 60,
  dailyTime: '09:00',
  lastRunAt: '',
  nextRunAt: ''
})
const archive = ref<main.ArchiveConfig>({ rootPath: '' })

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
  keywords: [],
  regions: [],
  dateRangeDays: 7,
  minIntervalMs: 1500,
  maxRetries: 3,
  categories: [],
  watermarks: [],
  createdAt: '',
  updatedAt: ''
}))

const keywordInput = ref('')
const regionInput = ref('')

const visibleOpportunities = computed(() => opportunities.value)
const scheduleStatus = computed(() => {
  if (!schedule.value.enabled) return '定时抓取未开启'
  const next = formatDateTime(schedule.value.nextRunAt)
  return next ? `下次 ${next}` : '等待下次抓取'
})

function resetSiteForm() {
  Object.assign(siteForm, {
    id: '',
    name: '',
    siteType: 'custom',
    baseUrl: '',
    enabled: true,
    renderMode: 'http',
    keywords: [],
    regions: [],
    dateRangeDays: 7,
    minIntervalMs: 1500,
    maxRetries: 3,
    categories: [],
    watermarks: [],
    createdAt: '',
    updatedAt: ''
  })
  keywordInput.value = ''
  regionInput.value = ''
}

function editSite(site: main.SiteConfig) {
  Object.assign(siteForm, JSON.parse(JSON.stringify(site)))
  keywordInput.value = site.keywords?.join('，') || ''
  regionInput.value = site.regions?.join('，') || ''
  activeTab.value = 'sites'
}

function splitList(value: string) {
  return value
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter(Boolean)
}

async function refreshAll() {
  loading.value = true
  try {
    const [dash, siteList, taskList, scheduleConfig, archiveConfig] = await Promise.all([
      Dashboard(),
      ListSites(),
      ListTasks(),
      GetSchedule(),
      GetArchiveConfig()
    ])
    dashboard.value = dash
    sites.value = siteList
    tasks.value = taskList
    schedule.value = scheduleConfig
    archive.value = archiveConfig
    await refreshOpportunities()
  } finally {
    loading.value = false
  }
}

async function refreshOpportunities() {
  opportunities.value = await ListOpportunities({
    search: query.search,
    siteId: query.siteId
  })
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
    running: '运行中'
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

function withTimeout<T>(promise: Promise<T>, ms: number, timeoutMessage: string) {
  let timer: ReturnType<typeof setTimeout> | null = null
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error(timeoutMessage)), ms)
  })
  return Promise.race([
    promise.finally(() => {
      if (timer) clearTimeout(timer)
    }),
    timeout
  ])
}

function startCrawlTimer() {
  crawlElapsedSeconds.value = 0
  if (crawlElapsedTimer) clearInterval(crawlElapsedTimer)
  crawlElapsedTimer = setInterval(() => {
    crawlElapsedSeconds.value += 1
    message.value = `正在抓取，请稍候...已耗时 ${crawlElapsedSeconds.value} 秒`
  }, 1000)
}

function stopCrawlTimer() {
  if (crawlElapsedTimer) {
    clearInterval(crawlElapsedTimer)
    crawlElapsedTimer = null
  }
}

function formatDateTime(value: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

async function saveSchedule() {
  savingSchedule.value = true
  try {
    schedule.value = await SaveSchedule({
      enabled: schedule.value.enabled,
      mode: schedule.value.mode,
      intervalMinutes: Number(schedule.value.intervalMinutes) || 60,
      dailyTime: schedule.value.dailyTime,
      lastRunAt: schedule.value.lastRunAt,
      nextRunAt: schedule.value.nextRunAt
    })
    message.value = schedule.value.enabled
      ? `定时抓取已开启：每 ${schedule.value.intervalMinutes} 分钟一次`
      : '定时抓取已关闭'
  } catch (error) {
    message.value = error instanceof Error ? error.message : String(error)
  } finally {
    savingSchedule.value = false
  }
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

async function saveSite() {
  loading.value = true
  message.value = ''
  try {
    siteForm.keywords = splitList(keywordInput.value)
    siteForm.regions = splitList(regionInput.value)
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

async function runCrawl() {
  loading.value = true
  message.value = '正在抓取，请稍候...'
  startCrawlTimer()
  try {
    const result = await withTimeout(
      RunCrawl({
        siteIds: sites.value.filter((site) => site.enabled).map((site) => site.id),
        keyword: '',
        days: 7
      }),
      crawlTimeoutMs,
      '抓取超过 60 秒仍未返回，请到任务日志查看是否有长时间运行或失败的站点。'
    )
    message.value = summarizeCrawlTasks(result)
    await refreshAll()
  } catch (error) {
    message.value = error instanceof Error ? error.message : String(error)
  } finally {
    stopCrawlTimer()
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

onMounted(refreshAll)
</script>

<template>
  <main class="shell">
    <aside class="sidebar">
      <div class="brand">
        <div>
          <h1>商机提取器</h1>
        </div>
      </div>

      <nav>
        <button :class="{ active: activeTab === 'opportunities' }" @click="activeTab = 'opportunities'">公告库</button>
        <button :class="{ active: activeTab === 'sites' }" @click="activeTab = 'sites'">站点配置</button>
        <button :class="{ active: activeTab === 'schedule' }" @click="activeTab = 'schedule'">定时抓取</button>
        <button :class="{ active: activeTab === 'tasks' }" @click="activeTab = 'tasks'">任务日志</button>
      </nav>

      <section class="stats">
        <div>
          <strong>{{ dashboard.opportunityCount }}</strong>
          <span>公告记录</span>
        </div>
        <div>
          <strong>{{ dashboard.enabledSiteCount }}/{{ dashboard.siteCount }}</strong>
          <span>启用站点</span>
        </div>
      </section>
    </aside>

    <section class="content">
      <header class="topbar">
        <div>
          <h2 v-if="activeTab === 'opportunities'">公告库</h2>
          <h2 v-else-if="activeTab === 'sites'">站点配置</h2>
          <h2 v-else-if="activeTab === 'schedule'">定时抓取</h2>
          <h2 v-else>任务日志</h2>
          <p>{{ message || '配置目标站点，手动抓取并归档招标采购商机。' }}</p>
        </div>
        <div class="topbar-actions">
          <button class="primary" :disabled="loading" @click="runCrawl">
            {{ loading ? '处理中...' : '开始抓取' }}
          </button>
        </div>
      </header>

      <section v-if="activeTab === 'opportunities'" class="workspace two-column">
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
            <div v-if="!visibleOpportunities.length" class="empty">暂无公告。先确认站点配置，然后点击开始抓取。</div>
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

      <section v-else-if="activeTab === 'sites'" class="workspace two-column">
        <form class="panel form" @submit.prevent="saveSite">
          <h3>{{ siteForm.id ? '编辑站点' : '新增站点' }}</h3>
          <label>
            站点名称
            <input v-model="siteForm.name" required placeholder="国家电网 - 招标采购公告" />
          </label>
          <label>
            入口 URL
            <input v-model="siteForm.baseUrl" required placeholder="https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe" />
          </label>
          <div class="grid-2">
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
          <div class="grid-3">
            <label>
              最近天数
              <input v-model.number="siteForm.dateRangeDays" min="1" type="number" />
            </label>
            <label>
              间隔毫秒
              <input v-model.number="siteForm.minIntervalMs" min="300" type="number" />
            </label>
            <label>
              重试次数
              <input v-model.number="siteForm.maxRetries" min="1" type="number" />
            </label>
          </div>
          <label>
            关键词
            <textarea v-model="keywordInput" rows="3" placeholder="招标，采购，变电站"></textarea>
          </label>
          <label>
            地区
            <textarea v-model="regionInput" rows="2" placeholder="广东，江苏，上海"></textarea>
          </label>
          <label class="check">
            <input v-model="siteForm.enabled" type="checkbox" />
            启用站点
          </label>
          <div class="actions">
            <button class="primary" type="submit">保存站点</button>
            <button type="button" @click="resetSiteForm">清空</button>
          </div>
        </form>

        <div class="panel form">
          <form class="form" @submit.prevent="saveArchiveConfig">
            <h3>公告归档目录</h3>
            <label>
              下载与快照保存位置
              <input v-model="archive.rootPath" required placeholder="D:\\商机提取器归档" />
            </label>
            <div class="actions">
              <button type="button" :disabled="loading" @click="chooseArchiveDirectory">选择目录</button>
              <button class="primary" type="submit" :disabled="loading">保存目录</button>
            </div>
          </form>

          <h3>已配置站点</h3>
          <article v-for="site in sites" :key="site.id" class="site-row">
            <div>
              <h4>{{ site.name }}</h4>
              <p>{{ site.baseUrl }}</p>
              <span>{{ site.siteType }} · {{ site.renderMode }} · {{ site.enabled ? '启用' : '停用' }}</span>
              <p v-if="site.siteType === 'sgcc'">已启用栏目：{{ site.categories?.filter((item) => item.enabled).length || 0 }}</p>
            </div>
            <div class="row-actions">
              <button @click="editSite(site)">编辑</button>
              <button @click="removeSite(site.id)">删除</button>
            </div>
          </article>
        </div>
      </section>

      <section v-if="activeTab === 'sites' && siteForm.siteType === 'sgcc'" class="workspace">
        <div class="panel form">
          <h3>国家电网公告栏目</h3>
          <label v-for="category in siteForm.categories" :key="category.id" class="check">
            <input v-model="category.enabled" type="checkbox" />
            {{ category.name }}
          </label>
        </div>
      </section>

      <section v-else-if="activeTab === 'schedule'" class="workspace">
        <div class="panel schedule-page">
          <div class="schedule-row">
            <label class="check">
              <input v-model="schedule.enabled" type="checkbox" :disabled="savingSchedule" @change="saveSchedule" />
              启用定时抓取
            </label>
            <label>
              执行方式
              <select v-model="schedule.mode" :disabled="savingSchedule || !schedule.enabled" @change="saveSchedule">
                <option value="interval">固定间隔</option>
                <option value="daily">每日固定时间</option>
              </select>
            </label>
            <label>
              间隔分钟
              <input
                v-model.number="schedule.intervalMinutes"
                min="5"
                max="1440"
                step="5"
                type="number"
                :disabled="savingSchedule || !schedule.enabled || schedule.mode === 'daily'"
                @change="saveSchedule"
              />
            </label>
            <label v-if="schedule.mode === 'daily'">
              每日执行时间
              <input v-model="schedule.dailyTime" type="time" :disabled="savingSchedule || !schedule.enabled" @change="saveSchedule" />
            </label>
          </div>
          <dl>
            <dt>状态</dt>
            <dd>{{ scheduleStatus }}</dd>
            <dt>上次运行</dt>
            <dd>{{ formatDateTime(schedule.lastRunAt) || '暂无' }}</dd>
            <dt>抓取范围</dt>
            <dd>所有启用站点，最近 7 天</dd>
          </dl>
        </div>
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
