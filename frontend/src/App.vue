<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Dashboard,
  DeleteSite,
  ListOpportunities,
  ListSites,
  ListTasks,
  RunCrawl,
  SaveRemark,
  SaveSite,
  ToggleFavorite
} from '../wailsjs/go/main/App'
import type { main } from '../wailsjs/go/models'

type Tab = 'opportunities' | 'sites' | 'tasks'

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
  favoriteCount: 0,
  lastTaskCount: 0
})
const sites = ref<main.SiteConfig[]>([])
const tasks = ref<main.CrawlTask[]>([])
const opportunities = ref<main.Opportunity[]>([])
const selectedOpportunity = ref<main.Opportunity | null>(null)

const query = reactive({
  search: '',
  siteId: '',
  onlyFavorite: false,
  onlyWithMatch: false
})

const crawlForm = reactive({
  siteIds: [] as string[],
  keyword: '',
  days: 7
})

const siteForm = reactive<main.SiteConfig>({
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
  createdAt: '',
  updatedAt: ''
})

const keywordInput = ref('')
const regionInput = ref('')
const remarkDraft = ref('')

const visibleOpportunities = computed(() => opportunities.value)

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
    const [dash, siteList, taskList] = await Promise.all([
      Dashboard(),
      ListSites(),
      ListTasks()
    ])
    dashboard.value = dash
    sites.value = siteList
    tasks.value = taskList
    await refreshOpportunities()
    if (crawlForm.siteIds.length === 0) {
      crawlForm.siteIds = siteList.filter((site) => site.enabled).map((site) => site.id)
    }
  } finally {
    loading.value = false
  }
}

async function refreshOpportunities() {
  opportunities.value = await ListOpportunities({
    search: query.search,
    siteId: query.siteId,
    onlyFavorite: query.onlyFavorite,
    onlyWithMatch: query.onlyWithMatch
  })
  if (selectedOpportunity.value) {
    selectedOpportunity.value =
      opportunities.value.find((item) => item.id === selectedOpportunity.value?.id) || null
  }
}

function statusLabel(status: string) {
  const labels: Record<string, string> = {
    success: '成功',
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
        siteIds: crawlForm.siteIds,
        keyword: crawlForm.keyword,
        days: Number(crawlForm.days) || 7
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
  remarkDraft.value = item.remark || ''
}

async function toggleFavorite(item: main.Opportunity) {
  const updated = await ToggleFavorite(item.id)
  Object.assign(item, updated)
  if (selectedOpportunity.value?.id === item.id) selectedOpportunity.value = updated
  await refreshAll()
}

async function saveRemark() {
  if (!selectedOpportunity.value) return
  const updated = await SaveRemark(selectedOpportunity.value.id, remarkDraft.value)
  selectedOpportunity.value = updated
  await refreshAll()
  message.value = '备注已保存'
}

function openSource(url: string) {
  if (!url) return
  window.open(url, '_blank')
}

onMounted(refreshAll)
</script>

<template>
  <main class="shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="mark">BOG</div>
        <div>
          <h1>商机提取器</h1>
          <p>Business Opportunity Grabber</p>
        </div>
      </div>

      <nav>
        <button :class="{ active: activeTab === 'opportunities' }" @click="activeTab = 'opportunities'">公告库</button>
        <button :class="{ active: activeTab === 'sites' }" @click="activeTab = 'sites'">站点配置</button>
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
        <div>
          <strong>{{ dashboard.favoriteCount }}</strong>
          <span>收藏</span>
        </div>
      </section>
    </aside>

    <section class="content">
      <header class="topbar">
        <div>
          <h2 v-if="activeTab === 'opportunities'">公告库</h2>
          <h2 v-else-if="activeTab === 'sites'">站点配置</h2>
          <h2 v-else>任务日志</h2>
          <p>{{ message || '配置目标站点，手动抓取并归档招标采购商机。' }}</p>
        </div>
        <button class="primary" :disabled="loading" @click="runCrawl">
          {{ loading ? '处理中...' : '开始抓取' }}
        </button>
      </header>

      <section class="crawl-panel">
        <label>
          抓取站点
          <select v-model="crawlForm.siteIds" multiple>
            <option v-for="site in sites" :key="site.id" :value="site.id">{{ site.name }}</option>
          </select>
        </label>
        <label>
          临时关键词
          <input v-model="crawlForm.keyword" placeholder="如：变电站、信息化、施工" />
        </label>
        <label>
          最近天数
          <input v-model.number="crawlForm.days" min="1" type="number" />
        </label>
      </section>

      <section v-if="activeTab === 'opportunities'" class="workspace two-column">
        <div class="panel">
          <div class="toolbar">
            <input v-model="query.search" placeholder="搜索标题、正文、来源..." @input="refreshOpportunities" />
            <select v-model="query.siteId" @change="refreshOpportunities">
              <option value="">全部站点</option>
              <option v-for="site in sites" :key="site.id" :value="site.id">{{ site.name }}</option>
            </select>
            <label class="check">
              <input v-model="query.onlyFavorite" type="checkbox" @change="refreshOpportunities" />
              收藏
            </label>
            <label class="check">
              <input v-model="query.onlyWithMatch" type="checkbox" @change="refreshOpportunities" />
              命中关键词
            </label>
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
              <button class="icon" title="收藏" @click.stop="toggleFavorite(item)">{{ item.isFavorite ? '★' : '☆' }}</button>
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
              <button @click="openSource(selectedOpportunity.sourceUrl)">打开原文</button>
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
            </dl>
            <p class="content-text">{{ selectedOpportunity.content }}</p>
            <label>
              备注
              <textarea v-model="remarkDraft" rows="5" placeholder="记录跟进人、判断依据或下一步动作"></textarea>
            </label>
            <button class="primary" @click="saveRemark">保存备注</button>
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
                <option value="browser">浏览器渲染预留</option>
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

        <div class="panel">
          <h3>已配置站点</h3>
          <article v-for="site in sites" :key="site.id" class="site-row">
            <div>
              <h4>{{ site.name }}</h4>
              <p>{{ site.baseUrl }}</p>
              <span>{{ site.siteType }} · {{ site.renderMode }} · {{ site.enabled ? '启用' : '停用' }}</span>
            </div>
            <div class="row-actions">
              <button @click="editSite(site)">编辑</button>
              <button @click="removeSite(site.id)">删除</button>
            </div>
          </article>
        </div>
      </section>

      <section v-else class="workspace">
        <div class="panel">
          <table>
            <thead>
              <tr>
                <th>站点</th>
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
