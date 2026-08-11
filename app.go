package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const defaultSGCCURL = "https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe"
const defaultCSGURL = "https://www.bidding.csg.cn/zbcg/index.jhtml"
const crawlRequestTimeout = 30 * time.Second
const sgccNoticeListURL = "https://ecp.sgcc.com.cn/ecp2.0/ecpwcmcore/index/noteList"
const sgccListSpeMenuID = "2018032700291334"

var (
	scriptTagRE = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	styleTagRE  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	htmlTagRE   = regexp.MustCompile(`(?is)<[^>]+>`)
	spaceRE     = regexp.MustCompile(`\s+`)
)

type App struct {
	ctx       context.Context
	mu        sync.Mutex
	crawlMu   sync.Mutex
	storePath string
	state     AppState
	client    *http.Client
}

// SiteAdapter isolates site-specific list and detail workflows from shared storage and archiving.
type SiteAdapter interface {
	CrawlTasks(app *App, site SiteConfig, req CrawlRequest) []CrawlTask
}

type sgccAdapter struct{}
type genericSiteAdapter struct{}

type AppState struct {
	Sites         []SiteConfig   `json:"sites"`
	Opportunities []Opportunity  `json:"opportunities"`
	Tasks         []CrawlTask    `json:"tasks"`
	Schedule      ScheduleConfig `json:"schedule"`
	Archive       ArchiveConfig  `json:"archive"`
}

type SiteConfig struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	SiteType      string           `json:"siteType"`
	BaseURL       string           `json:"baseUrl"`
	Enabled       bool             `json:"enabled"`
	RenderMode    string           `json:"renderMode"`
	Keywords      []string         `json:"keywords"`
	Regions       []string         `json:"regions"`
	DateRangeDays int              `json:"dateRangeDays"`
	MinIntervalMS int              `json:"minIntervalMs"`
	MaxRetries    int              `json:"maxRetries"`
	Categories    []NoticeCategory `json:"categories"`
	Watermarks    []CrawlWatermark `json:"watermarks"`
	CreatedAt     string           `json:"createdAt"`
	UpdatedAt     string           `json:"updatedAt"`
}

type NoticeCategory struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	MenuID              string `json:"menuId"`
	NoticeType          string `json:"noticeType"`
	Enabled             bool   `json:"enabled"`
	DownloadAttachments bool   `json:"downloadAttachments"`
	ArchiveProject      bool   `json:"archiveProject"`
}

type CrawlWatermark struct {
	CategoryID     string `json:"categoryId"`
	LastSuccessAt  string `json:"lastSuccessAt"`
	LastNoticeTime string `json:"lastNoticeTime"`
	LastNoticeID   string `json:"lastNoticeId"`
}

type ArchiveConfig struct {
	RootPath string `json:"rootPath"`
}

type Attachment struct {
	Name        string `json:"name"`
	SourceURL   string `json:"sourceUrl"`
	LocalPath   string `json:"localPath"`
	Size        int64  `json:"size"`
	Hash        string `json:"hash"`
	Status      string `json:"status"`
	ErrorReason string `json:"errorReason"`
}

type Opportunity struct {
	ID              string       `json:"id"`
	SiteID          string       `json:"siteId"`
	SourceSite      string       `json:"sourceSite"`
	Title           string       `json:"title"`
	NoticeType      string       `json:"noticeType"`
	PublishTime     string       `json:"publishTime"`
	Region          string       `json:"region"`
	TenderNo        string       `json:"tenderNo"`
	Buyer           string       `json:"buyer"`
	Deadline        string       `json:"deadline"`
	SourceURL       string       `json:"sourceUrl"`
	Content         string       `json:"content"`
	MatchedKeywords []string     `json:"matchedKeywords"`
	ContentHash     string       `json:"contentHash"`
	CategoryID      string       `json:"categoryId"`
	CategoryName    string       `json:"categoryName"`
	NoticeID        string       `json:"noticeId"`
	ProcessStatus   string       `json:"processStatus"`
	ArchivePath     string       `json:"archivePath"`
	DetailFetchedAt string       `json:"detailFetchedAt"`
	ArchiveError    string       `json:"archiveError"`
	Attachments     []Attachment `json:"attachments"`
	CreatedAt       string       `json:"createdAt"`
	UpdatedAt       string       `json:"updatedAt"`
}

type CrawlTask struct {
	ID             string `json:"id"`
	SiteID         string `json:"siteId"`
	SiteName       string `json:"siteName"`
	CategoryID     string `json:"categoryId"`
	CategoryName   string `json:"categoryName"`
	Status         string `json:"status"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	TotalCount     int    `json:"totalCount"`
	NewCount       int    `json:"newCount"`
	DuplicateCount int    `json:"duplicateCount"`
	FailedCount    int    `json:"failedCount"`
	ErrorMessage   string `json:"errorMessage"`
}

type ScheduleConfig struct {
	Enabled         bool   `json:"enabled"`
	Mode            string `json:"mode"`
	IntervalMinutes int    `json:"intervalMinutes"`
	DailyTime       string `json:"dailyTime"`
	LastRunAt       string `json:"lastRunAt"`
	NextRunAt       string `json:"nextRunAt"`
}

type Dashboard struct {
	SiteCount        int `json:"siteCount"`
	EnabledSiteCount int `json:"enabledSiteCount"`
	OpportunityCount int `json:"opportunityCount"`
	LastTaskCount    int `json:"lastTaskCount"`
}

type CrawlRequest struct {
	SiteIDs []string `json:"siteIds"`
	Keyword string   `json:"keyword"`
	Days    int      `json:"days"`
}

type OpportunityQuery struct {
	Search string `json:"search"`
	SiteID string `json:"siteId"`
}

type sgccNoteListRequest struct {
	Index           int    `json:"index"`
	Size            int    `json:"size"`
	FirstPageMenuID string `json:"firstPageMenuId"`
	PurOrgStatus    string `json:"purOrgStatus"`
	PurOrgCode      string `json:"purOrgCode"`
	PurType         string `json:"purType"`
	NoticeType      string `json:"noticeType"`
	OrgID           string `json:"orgId"`
	Key             string `json:"key"`
}

type sgccNoteListResponse struct {
	Successful  bool `json:"successful"`
	ResultValue struct {
		NoteList []sgccNotice `json:"noteList"`
		Count    int          `json:"count"`
	} `json:"resultValue"`
	ResultHint string `json:"resultHint"`
	Type       string `json:"type"`
}

type sgccNotice struct {
	PublishOrgName    string          `json:"publishOrgName"`
	Code              string          `json:"code"`
	PrjStatus         json.RawMessage `json:"prjStatus"`
	FirstPageDocID    json.RawMessage `json:"firstPageDocId"`
	NoticeType        json.RawMessage `json:"noticeType"`
	Title             string          `json:"title"`
	NoticePublishTime string          `json:"noticePublishTime"`
	NoticeID          json.RawMessage `json:"noticeId"`
	TopEndTime        string          `json:"topEndTime"`
	DocType           string          `json:"doctype"`
	FirstPageMenuID   json.RawMessage `json:"firstPageMenuId"`
	ID                json.RawMessage `json:"id"`
}

func NewApp() *App {
	return &App{
		client: &http.Client{Timeout: 25 * time.Second},
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if err := a.initStore(); err != nil {
		fmt.Println("init store:", err)
	}
	go a.schedulerLoop(ctx)
}

func (a *App) initStore() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	appDir := filepath.Join(configDir, "商机提取器")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return err
	}

	a.storePath = filepath.Join(appDir, "opportunity-data.json")
	legacyStorePath := filepath.Join(configDir, strings.Join([]string{"Business", "Opportunity", "Grabber"}, " "), "bo"+"g-data.json")
	if _, err := os.Stat(a.storePath); errors.Is(err, os.ErrNotExist) {
		if raw, readErr := os.ReadFile(legacyStorePath); readErr == nil && len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &a.state); err != nil {
				return err
			}
			a.ensureBuiltInSitesLocked()
			a.normalizeArchiveLocked()
			a.normalizeScheduleLocked()
			return a.saveLocked()
		}
		a.state = AppState{Sites: defaultSites(), Archive: defaultArchiveConfig()}
		a.normalizeArchiveLocked()
		a.normalizeScheduleLocked()
		return a.saveLocked()
	}

	raw, err := os.ReadFile(a.storePath)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		a.state = AppState{Sites: defaultSites(), Archive: defaultArchiveConfig()}
		a.normalizeArchiveLocked()
		a.normalizeScheduleLocked()
		return a.saveLocked()
	}
	if err := json.Unmarshal(raw, &a.state); err != nil {
		return err
	}
	a.ensureBuiltInSitesLocked()
	a.normalizeArchiveLocked()
	a.normalizeScheduleLocked()
	return a.saveLocked()
}

func defaultSites() []SiteConfig {
	return []SiteConfig{defaultSGCCSite(), defaultCSGSite()}
}

func (a *App) ensureBuiltInSitesLocked() {
	if len(a.state.Sites) == 0 {
		a.state.Sites = defaultSites()
		return
	}

	for _, builtIn := range defaultSites() {
		exists := false
		for _, site := range a.state.Sites {
			if site.ID == builtIn.ID || strings.EqualFold(site.BaseURL, builtIn.BaseURL) {
				exists = true
				break
			}
		}
		if !exists {
			a.state.Sites = append(a.state.Sites, builtIn)
		}
	}
	for i := range a.state.Sites {
		if a.state.Sites[i].SiteType == "sgcc" && len(a.state.Sites[i].Categories) == 0 {
			a.state.Sites[i].Categories = defaultSGCCCategories()
		}
	}
}

func defaultSGCCSite() SiteConfig {
	now := nowString()
	return SiteConfig{
		ID:            "sgcc-list-spe",
		Name:          "国家电网 - 招标采购公告",
		SiteType:      "sgcc",
		BaseURL:       defaultSGCCURL,
		Enabled:       true,
		RenderMode:    "http",
		Keywords:      []string{"招标", "采购", "项目", "电网"},
		Regions:       []string{},
		DateRangeDays: 7,
		MinIntervalMS: 1500,
		MaxRetries:    3,
		Categories:    defaultSGCCCategories(),
		Watermarks:    []CrawlWatermark{},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func defaultSGCCCategories() []NoticeCategory {
	return []NoticeCategory{
		{ID: "sgcc-single-source", Name: "单一来源采购事前公示", MenuID: "2019102186483919", NoticeType: "单一来源公示", Enabled: true, DownloadAttachments: true, ArchiveProject: true},
		{ID: "sgcc-annual-plan", Name: "年度采购计划预安排", MenuID: "2020052000175277", NoticeType: "年度采购计划", Enabled: true, DownloadAttachments: false, ArchiveProject: true},
		{ID: "sgcc-prequalification", Name: "资格预审公告", MenuID: "2018032700290425", NoticeType: "资格预审", Enabled: true, DownloadAttachments: true, ArchiveProject: true},
		{ID: "sgcc-bid", Name: "招标公告及投标邀请书", MenuID: "2018032700291334", NoticeType: "招标公告", Enabled: true, DownloadAttachments: true, ArchiveProject: true},
		{ID: "sgcc-procurement", Name: "采购公告", MenuID: "2018032900295987", NoticeType: "采购公告", Enabled: true, DownloadAttachments: true, ArchiveProject: true},
		{ID: "sgcc-winners", Name: "推荐中标候选人公示", MenuID: "2018060501171107", NoticeType: "推荐中标候选人公示", Enabled: true, DownloadAttachments: true, ArchiveProject: true},
		{ID: "sgcc-qualification", Name: "资质能力核实", MenuID: "2019071434441442", NoticeType: "资质能力核实", Enabled: true, DownloadAttachments: true, ArchiveProject: true},
	}
}

func defaultArchiveConfig() ArchiveConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return ArchiveConfig{}
	}
	return ArchiveConfig{RootPath: filepath.Join(home, "Documents", "商机提取器归档")}
}

func defaultCSGSite() SiteConfig {
	now := nowString()
	return SiteConfig{
		ID:            "csg-zbcg",
		Name:          "南方电网 - 采购公告",
		SiteType:      "csg",
		BaseURL:       defaultCSGURL,
		Enabled:       true,
		RenderMode:    "http",
		Keywords:      []string{"招标", "采购", "公告", "南方电网"},
		Regions:       []string{},
		DateRangeDays: 7,
		MinIntervalMS: 1500,
		MaxRetries:    3,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (a *App) GetArchiveConfig() ArchiveConfig {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.normalizeArchiveLocked()
	return a.state.Archive
}

func (a *App) SaveArchiveConfig(config ArchiveConfig) (ArchiveConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	config.RootPath = strings.TrimSpace(config.RootPath)
	if config.RootPath == "" {
		return ArchiveConfig{}, errors.New("归档目录不能为空")
	}
	absPath, err := filepath.Abs(config.RootPath)
	if err != nil {
		return ArchiveConfig{}, fmt.Errorf("归档目录无效：%w", err)
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return ArchiveConfig{}, fmt.Errorf("无法创建归档目录：%w", err)
	}
	a.state.Archive = ArchiveConfig{RootPath: absPath}
	if err := a.saveLocked(); err != nil {
		return ArchiveConfig{}, err
	}
	return a.state.Archive, nil
}

func (a *App) SelectArchiveDirectory() (string, error) {
	defaultDirectory := a.GetArchiveConfig().RootPath
	if defaultDirectory != "" {
		if _, err := os.Stat(defaultDirectory); err != nil {
			defaultDirectory = ""
		}
	}
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "选择公告归档目录",
		DefaultDirectory:     defaultDirectory,
		CanCreateDirectories: true,
	})
}

func (a *App) Dashboard() Dashboard {
	a.mu.Lock()
	defer a.mu.Unlock()

	var enabled int
	for _, site := range a.state.Sites {
		if site.Enabled {
			enabled++
		}
	}
	return Dashboard{
		SiteCount:        len(a.state.Sites),
		EnabledSiteCount: enabled,
		OpportunityCount: len(a.state.Opportunities),
		LastTaskCount:    len(a.state.Tasks),
	}
}

func (a *App) ListSites() []SiteConfig {
	a.mu.Lock()
	defer a.mu.Unlock()

	sites := append([]SiteConfig(nil), a.state.Sites...)
	sort.Slice(sites, func(i, j int) bool {
		return sites[i].CreatedAt < sites[j].CreatedAt
	})
	return sites
}

func (a *App) SaveSite(site SiteConfig) (SiteConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	site.Name = strings.TrimSpace(site.Name)
	site.BaseURL = strings.TrimSpace(site.BaseURL)
	if site.Name == "" {
		return SiteConfig{}, errors.New("站点名称不能为空")
	}
	if _, err := url.ParseRequestURI(site.BaseURL); err != nil {
		return SiteConfig{}, errors.New("入口 URL 格式不正确")
	}
	if site.SiteType == "" {
		site.SiteType = "custom"
	}
	if site.RenderMode == "" {
		site.RenderMode = "http"
	}
	if site.DateRangeDays <= 0 {
		site.DateRangeDays = 7
	}
	if site.MinIntervalMS <= 0 {
		site.MinIntervalMS = 1500
	}
	if site.MaxRetries <= 0 {
		site.MaxRetries = 3
	}
	site.Keywords = cleanList(site.Keywords)
	site.Regions = cleanList(site.Regions)
	site.Categories = normalizeCategories(site.Categories)

	now := nowString()
	if site.ID == "" {
		site.ID = makeID(site.Name + site.BaseURL + now)
		site.CreatedAt = now
	}
	site.UpdatedAt = now

	for i := range a.state.Sites {
		if a.state.Sites[i].ID == site.ID {
			if site.CreatedAt == "" {
				site.CreatedAt = a.state.Sites[i].CreatedAt
			}
			a.state.Sites[i] = site
			return site, a.saveLocked()
		}
	}

	a.state.Sites = append(a.state.Sites, site)
	return site, a.saveLocked()
}

func (a *App) DeleteSite(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i := range a.state.Sites {
		if a.state.Sites[i].ID == id {
			a.state.Sites = append(a.state.Sites[:i], a.state.Sites[i+1:]...)
			return a.saveLocked()
		}
	}
	return errors.New("未找到站点")
}

func (a *App) RunCrawl(req CrawlRequest) ([]CrawlTask, error) {
	a.crawlMu.Lock()
	defer a.crawlMu.Unlock()

	return a.runCrawl(req)
}

func (a *App) runCrawl(req CrawlRequest) ([]CrawlTask, error) {
	a.mu.Lock()
	targets := a.resolveTargetsLocked(req.SiteIDs)
	a.mu.Unlock()

	if len(targets) == 0 {
		return nil, errors.New("没有可抓取的启用站点")
	}

	var tasks []CrawlTask
	for _, site := range targets {
		tasks = append(tasks, a.crawlSiteTasks(site, req)...)
	}
	return tasks, nil
}

func (a *App) GetSchedule() ScheduleConfig {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.normalizeScheduleLocked()
	return a.state.Schedule
}

func (a *App) SaveSchedule(schedule ScheduleConfig) (ScheduleConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if schedule.Mode == "" {
		schedule.Mode = "interval"
	}
	if schedule.Mode != "interval" && schedule.Mode != "daily" {
		return ScheduleConfig{}, errors.New("定时模式必须为 interval 或 daily")
	}
	if schedule.Mode == "daily" && !validDailyTime(schedule.DailyTime) {
		return ScheduleConfig{}, errors.New("每日执行时间格式应为 HH:MM")
	}
	if schedule.IntervalMinutes <= 0 {
		schedule.IntervalMinutes = 60
	}
	if schedule.IntervalMinutes < 5 {
		return ScheduleConfig{}, errors.New("定时间隔不能小于 5 分钟")
	}
	if schedule.IntervalMinutes > 1440 {
		return ScheduleConfig{}, errors.New("定时间隔不能大于 1440 分钟")
	}

	existing := a.state.Schedule
	schedule.LastRunAt = existing.LastRunAt
	if schedule.Enabled {
		changed := !existing.Enabled || existing.IntervalMinutes != schedule.IntervalMinutes || existing.Mode != schedule.Mode || existing.DailyTime != schedule.DailyTime || strings.TrimSpace(schedule.NextRunAt) == ""
		if changed {
			schedule.NextRunAt = nextRunAt(time.Now(), schedule)
		} else {
			schedule.NextRunAt = existing.NextRunAt
		}
	} else {
		schedule.NextRunAt = ""
	}

	a.state.Schedule = schedule
	a.normalizeScheduleLocked()
	if err := a.saveLocked(); err != nil {
		return ScheduleConfig{}, err
	}
	return a.state.Schedule, nil
}

func (a *App) ListTasks() []CrawlTask {
	a.mu.Lock()
	defer a.mu.Unlock()

	tasks := append([]CrawlTask(nil), a.state.Tasks...)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].StartedAt > tasks[j].StartedAt
	})
	return tasks
}

func (a *App) ListOpportunities(query OpportunityQuery) []Opportunity {
	a.mu.Lock()
	defer a.mu.Unlock()

	search := strings.ToLower(strings.TrimSpace(query.Search))
	items := make([]Opportunity, 0, len(a.state.Opportunities))
	for _, item := range a.state.Opportunities {
		if query.SiteID != "" && item.SiteID != query.SiteID {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(item.Title + " " + item.Content + " " + item.SourceSite + " " + item.NoticeType)
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i].PublishTime
		if left == "" {
			left = items[i].CreatedAt
		}
		right := items[j].PublishTime
		if right == "" {
			right = items[j].CreatedAt
		}
		return left > right
	})
	return items
}

func (a *App) RetryArchive(opportunityID string) (Opportunity, error) {
	a.mu.Lock()
	var item Opportunity
	var site SiteConfig
	found := false
	for _, candidate := range a.state.Opportunities {
		if candidate.ID == opportunityID {
			item = candidate
			found = true
			break
		}
	}
	if !found {
		a.mu.Unlock()
		return Opportunity{}, errors.New("未找到公告")
	}
	for _, candidate := range a.state.Sites {
		if candidate.ID == item.SiteID {
			site = candidate
			break
		}
	}
	a.mu.Unlock()

	category := NoticeCategory{ID: item.CategoryID, Name: item.CategoryName, ArchiveProject: true, DownloadAttachments: true}
	for _, candidate := range site.Categories {
		if candidate.ID == item.CategoryID {
			category = candidate
			break
		}
	}
	if !category.ArchiveProject {
		return Opportunity{}, errors.New("该公告栏目未启用归档")
	}
	a.archiveOpportunity(&item, category)
	item.UpdatedAt = nowString()

	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Opportunities {
		if a.state.Opportunities[i].ID == item.ID {
			item.CreatedAt = a.state.Opportunities[i].CreatedAt
			a.state.Opportunities[i] = item
			if err := a.saveLocked(); err != nil {
				return Opportunity{}, err
			}
			return item, nil
		}
	}
	return Opportunity{}, errors.New("公告已不存在")
}

func (a *App) resolveTargetsLocked(ids []string) []SiteConfig {
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}

	var targets []SiteConfig
	for _, site := range a.state.Sites {
		if !site.Enabled {
			continue
		}
		if len(idSet) > 0 && !idSet[site.ID] {
			continue
		}
		targets = append(targets, site)
	}
	return targets
}

func (a *App) schedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runScheduledCrawlIfDue()
		}
	}
}

func (a *App) runScheduledCrawlIfDue() {
	now := time.Now()

	a.mu.Lock()
	a.normalizeScheduleLocked()
	schedule := a.state.Schedule
	if !schedule.Enabled || schedule.NextRunAt == "" {
		a.mu.Unlock()
		return
	}
	scheduledFor, err := time.Parse(time.RFC3339, schedule.NextRunAt)
	if err != nil {
		a.state.Schedule.NextRunAt = now.Add(time.Duration(schedule.IntervalMinutes) * time.Minute).Format(time.RFC3339)
		_ = a.saveLocked()
		a.mu.Unlock()
		return
	}
	if now.Before(scheduledFor) {
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()

	if !a.crawlMu.TryLock() {
		return
	}
	defer a.crawlMu.Unlock()

	days := 7
	if schedule.Mode == "daily" {
		days = 1
	}
	_, _ = a.runCrawl(CrawlRequest{Days: days})

	a.mu.Lock()
	defer a.mu.Unlock()
	a.normalizeScheduleLocked()
	if a.state.Schedule.Enabled {
		a.state.Schedule.LastRunAt = nowString()
		a.state.Schedule.NextRunAt = nextRunAt(time.Now(), a.state.Schedule)
		_ = a.saveLocked()
	}
}

func (a *App) normalizeScheduleLocked() {
	if a.state.Schedule.Mode == "" {
		a.state.Schedule.Mode = "interval"
	}
	if a.state.Schedule.Mode != "daily" {
		a.state.Schedule.Mode = "interval"
	}
	if !validDailyTime(a.state.Schedule.DailyTime) {
		a.state.Schedule.DailyTime = "09:00"
	}
	if a.state.Schedule.IntervalMinutes <= 0 {
		a.state.Schedule.IntervalMinutes = 60
	}
	if a.state.Schedule.IntervalMinutes < 5 {
		a.state.Schedule.IntervalMinutes = 5
	}
	if a.state.Schedule.IntervalMinutes > 1440 {
		a.state.Schedule.IntervalMinutes = 1440
	}
	if !a.state.Schedule.Enabled {
		a.state.Schedule.NextRunAt = ""
	}
}

func (a *App) normalizeArchiveLocked() {
	if strings.TrimSpace(a.state.Archive.RootPath) == "" {
		a.state.Archive = defaultArchiveConfig()
	}
}

func normalizeCategories(categories []NoticeCategory) []NoticeCategory {
	cleaned := make([]NoticeCategory, 0, len(categories))
	seen := map[string]bool{}
	for _, category := range categories {
		category.ID = strings.TrimSpace(category.ID)
		category.Name = strings.TrimSpace(category.Name)
		category.MenuID = strings.TrimSpace(category.MenuID)
		if category.ID == "" || category.Name == "" || category.MenuID == "" || seen[category.ID] {
			continue
		}
		seen[category.ID] = true
		cleaned = append(cleaned, category)
	}
	return cleaned
}

func validDailyTime(value string) bool {
	_, err := time.Parse("15:04", value)
	return err == nil
}

func nextRunAt(now time.Time, schedule ScheduleConfig) string {
	if schedule.Mode != "daily" {
		return now.Add(time.Duration(schedule.IntervalMinutes) * time.Minute).Format(time.RFC3339)
	}
	dailyTime, _ := time.Parse("15:04", schedule.DailyTime)
	next := time.Date(now.Year(), now.Month(), now.Day(), dailyTime.Hour(), dailyTime.Minute(), 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Format(time.RFC3339)
}

func (a *App) crawlSite(site SiteConfig, req CrawlRequest) (task CrawlTask) {
	startedAt := nowString()
	task = CrawlTask{
		ID:        makeID(fmt.Sprintf("%s-%d", site.ID, time.Now().UnixNano())),
		SiteID:    site.ID,
		SiteName:  site.Name,
		Status:    "running",
		StartedAt: startedAt,
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			task.Status = "failed"
			task.FailedCount = 1
			task.ErrorMessage = fmt.Sprintf("抓取任务异常：%v", recovered)
			task.FinishedAt = nowString()
			a.recordTask(task)
		}
	}()

	items, err := a.fetchOpportunities(site, req)
	task.TotalCount = len(items)
	if err != nil {
		task.Status = "failed"
		task.FailedCount = 1
		task.ErrorMessage = err.Error()
		task.FinishedAt = nowString()
		a.recordTask(task)
		return task
	}

	newCount, duplicateCount := a.upsertOpportunities(items)
	task.NewCount = newCount
	task.DuplicateCount = duplicateCount
	task.Status = "success"
	task.FinishedAt = nowString()
	a.recordTask(task)
	return task
}

func (a *App) crawlSiteTasks(site SiteConfig, req CrawlRequest) []CrawlTask {
	if site.SiteType == "sgcc" {
		return sgccAdapter{}.CrawlTasks(a, site, req)
	}
	return genericSiteAdapter{}.CrawlTasks(a, site, req)
}

func (sgccAdapter) CrawlTasks(app *App, site SiteConfig, req CrawlRequest) []CrawlTask {
	categories := normalizeCategories(site.Categories)
	if len(categories) == 0 {
		categories = defaultSGCCCategories()
	}
	tasks := make([]CrawlTask, 0, len(categories))
	for _, category := range categories {
		if category.Enabled {
			tasks = append(tasks, app.crawlSGCCCategory(site, category, req))
		}
	}
	return tasks
}

func (genericSiteAdapter) CrawlTasks(app *App, site SiteConfig, req CrawlRequest) []CrawlTask {
	return []CrawlTask{app.crawlSite(site, req)}
}

func (a *App) crawlSGCCCategory(site SiteConfig, category NoticeCategory, req CrawlRequest) (task CrawlTask) {
	task = CrawlTask{
		ID:           makeID(fmt.Sprintf("%s-%s-%d", site.ID, category.ID, time.Now().UnixNano())),
		SiteID:       site.ID,
		SiteName:     site.Name,
		CategoryID:   category.ID,
		CategoryName: category.Name,
		Status:       "running",
		StartedAt:    nowString(),
	}

	watermark := watermarkFor(site, category.ID)
	items, nextWatermark, err := a.fetchSGCCCategory(site, category, req, watermark)
	task.TotalCount = len(items)
	if err != nil {
		task.Status = "failed"
		task.FailedCount = 1
		task.ErrorMessage = err.Error()
		task.FinishedAt = nowString()
		a.recordTask(task)
		return task
	}
	if len(items) == 0 {
		task.Status = "no_updates"
		task.FinishedAt = nowString()
		a.updateWatermark(site.ID, nextWatermark)
		a.recordTask(task)
		return task
	}

	for i := range items {
		if category.ArchiveProject {
			a.archiveOpportunity(&items[i], category)
		}
	}
	newCount, duplicateCount := a.upsertOpportunities(items)
	task.NewCount = newCount
	task.DuplicateCount = duplicateCount
	task.Status = "success"
	task.FinishedAt = nowString()
	a.updateWatermark(site.ID, nextWatermark)
	a.recordTask(task)
	return task
}

func watermarkFor(site SiteConfig, categoryID string) CrawlWatermark {
	for _, watermark := range site.Watermarks {
		if watermark.CategoryID == categoryID {
			return watermark
		}
	}
	return CrawlWatermark{CategoryID: categoryID}
}

func (a *App) updateWatermark(siteID string, watermark CrawlWatermark) {
	watermark.LastSuccessAt = nowString()
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Sites {
		if a.state.Sites[i].ID != siteID {
			continue
		}
		for j := range a.state.Sites[i].Watermarks {
			if a.state.Sites[i].Watermarks[j].CategoryID == watermark.CategoryID {
				a.state.Sites[i].Watermarks[j] = watermark
				_ = a.saveLocked()
				return
			}
		}
		a.state.Sites[i].Watermarks = append(a.state.Sites[i].Watermarks, watermark)
		_ = a.saveLocked()
		return
	}
}

func (a *App) fetchOpportunities(site SiteConfig, req CrawlRequest) ([]Opportunity, error) {
	if site.RenderMode == "browser" {
		return nil, errors.New("该站点配置为浏览器渲染模式，但当前版本尚未实现浏览器渲染抓取，请先改为 HTTP 静态抓取或配置真实列表接口")
	}
	if site.SiteType == "sgcc" || strings.Contains(site.BaseURL, "ecp.sgcc.com.cn") {
		return a.fetchSGCCOpportunities(site, req)
	}
	if site.SiteType == "csg" || strings.Contains(site.BaseURL, "bidding.csg.cn") {
		return a.fetchCSGOpportunities(site, req)
	}

	ctx, cancel := context.WithTimeout(context.Background(), crawlRequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, site.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "OpportunityCrawler/0.1 (+https://local.app)")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("访问失败：HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}

	items := parseHTMLForOpportunities(string(body), site, req)
	if len(items) == 0 {
		item := Opportunity{
			ID:         makeID(site.ID + site.BaseURL),
			SiteID:     site.ID,
			SourceSite: site.Name,
			Title:      "已访问站点，但未从静态 HTML 中识别出公告列表",
			NoticeType: "抓取提示",
			SourceURL:  site.BaseURL,
			Content:    "该站点可能通过前端接口异步加载公告数据。下一步可在浏览器开发者工具中确认列表接口后，将该接口配置为站点入口，或增加浏览器渲染适配。",
			CreatedAt:  nowString(),
			UpdatedAt:  nowString(),
		}
		item.ContentHash = makeID(item.SourceURL + item.Title + item.Content)
		item.MatchedKeywords = matchKeywords(item.Title+" "+item.Content, site, req)
		items = append(items, item)
	}
	return items, nil
}

func (a *App) fetchCSGOpportunities(site SiteConfig, req CrawlRequest) ([]Opportunity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), crawlRequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, site.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 OpportunityCrawler/0.1")
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	httpReq.Header.Set("Referer", "https://www.bidding.csg.cn/")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("访问失败：HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}

	items := parseCSGHTMLForOpportunities(string(body), site, req)
	if len(items) == 0 {
		return nil, errors.New("南方电网页面已访问，但未识别出采购公告列表")
	}
	return items, nil
}

func (a *App) fetchSGCCOpportunities(site SiteConfig, req CrawlRequest) ([]Opportunity, error) {
	category := NoticeCategory{ID: "sgcc-bid", Name: "招标公告及投标邀请书", MenuID: sgccListSpeMenuID, NoticeType: "招标公告", Enabled: true, DownloadAttachments: true, ArchiveProject: true}
	items, _, err := a.fetchSGCCCategory(site, category, req, CrawlWatermark{CategoryID: category.ID})
	return items, err
}

func (a *App) fetchSGCCCategory(site SiteConfig, category NoticeCategory, req CrawlRequest, watermark CrawlWatermark) ([]Opportunity, CrawlWatermark, error) {
	const pageSize = 50
	cutoff := time.Time{}
	if req.Days > 0 {
		cutoff = time.Now().AddDate(0, 0, -req.Days)
	}
	if watermark.LastNoticeTime != "" {
		if watermarkTime, err := time.Parse("2006-01-02", watermark.LastNoticeTime); err == nil && (cutoff.IsZero() || watermarkTime.After(cutoff)) {
			cutoff = watermarkTime
		}
	}

	items := []Opportunity{}
	nextWatermark := watermark
	for page := 1; page <= 100; page++ {
		payload := sgccNoteListRequest{
			Index:           page,
			Size:            pageSize,
			FirstPageMenuID: category.MenuID,
			Key:             strings.TrimSpace(req.Keyword),
		}
		var data sgccNoteListResponse
		if err := a.postJSON(sgccNoticeListURL, payload, &data); err != nil {
			return nil, watermark, err
		}
		if !data.Successful {
			if data.ResultHint != "" {
				return nil, watermark, fmt.Errorf("国家电网公告接口返回失败：%s", data.ResultHint)
			}
			return nil, watermark, errors.New("国家电网公告接口返回失败")
		}

		pastBoundary := false
		for _, notice := range data.ResultValue.NoteList {
			item := sgccNoticeToOpportunity(notice, site, req)
			if item.Title == "" {
				continue
			}
			item.CategoryID = category.ID
			item.CategoryName = category.Name
			item.NoticeType = category.NoticeType
			item.NoticeID = jsonValueString(notice.NoticeID)
			if item.NoticeID == "" {
				item.NoticeID = jsonValueString(notice.ID)
			}
			if item.PublishTime != "" {
				publishedAt, err := time.Parse("2006-01-02", item.PublishTime)
				if err == nil && !cutoff.IsZero() && publishedAt.Before(cutoff) {
					pastBoundary = true
					continue
				}
			}
			if a.isKnownOpportunity(item) {
				continue
			}
			items = append(items, item)
			if nextWatermark.LastNoticeTime == "" || item.PublishTime > nextWatermark.LastNoticeTime || (item.PublishTime == nextWatermark.LastNoticeTime && item.NoticeID > nextWatermark.LastNoticeID) {
				nextWatermark.LastNoticeTime = item.PublishTime
				nextWatermark.LastNoticeID = item.NoticeID
			}
		}
		if pastBoundary || len(data.ResultValue.NoteList) < pageSize || page*pageSize >= data.ResultValue.Count {
			break
		}
	}
	return items, nextWatermark, nil
}

func (a *App) postJSON(endpoint string, payload any, target any) error {
	ctx, cancel := context.WithTimeout(context.Background(), crawlRequestTimeout)
	defer cancel()

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 OpportunityCrawler/0.1")
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Origin", "https://ecp.sgcc.com.cn")
	httpReq.Header.Set("Referer", defaultSGCCURL)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("访问失败：HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4*1024*1024)).Decode(target)
}

func sgccNoticeToOpportunity(notice sgccNotice, site SiteConfig, req CrawlRequest) Opportunity {
	noticeID := jsonValueString(notice.NoticeID)
	if noticeID == "" {
		noticeID = jsonValueString(notice.ID)
	}
	docID := jsonValueString(notice.FirstPageDocID)
	routeDocID := noticeID
	if routeDocID == "" {
		routeDocID = docID
	}
	menuID := jsonValueString(notice.FirstPageMenuID)
	if menuID == "" {
		menuID = sgccListSpeMenuID
	}
	status := sgccProjectStatus(jsonValueString(notice.PrjStatus))
	sourceURL := sgccContentURL(routeDocID, menuID, notice.DocType, notice.Title)
	content := strings.Join(cleanList([]string{
		"项目名称：" + notice.Title,
		"项目编号：" + notice.Code,
		"项目状态：" + status,
		"发布单位：" + notice.PublishOrgName,
		"创建时间：" + notice.NoticePublishTime,
		"截止时间：" + notice.TopEndTime,
	}), "\n")

	item := Opportunity{
		ID:              makeID(site.ID + noticeID + docID + notice.Title),
		SiteID:          site.ID,
		SourceSite:      site.Name,
		Title:           notice.Title,
		NoticeType:      inferSGCCNoticeType(notice),
		PublishTime:     normalizeSGCCDate(notice.NoticePublishTime),
		Region:          status,
		TenderNo:        notice.Code,
		Buyer:           notice.PublishOrgName,
		Deadline:        notice.TopEndTime,
		SourceURL:       sourceURL,
		Content:         content,
		MatchedKeywords: matchKeywords(notice.Title+" "+notice.Code+" "+notice.PublishOrgName+" "+content, site, req),
		CreatedAt:       nowString(),
		UpdatedAt:       nowString(),
	}
	item.ContentHash = makeID(item.SourceURL + item.Title + item.Content)
	return item
}

func sgccContentURL(docID string, menuID string, docType string, title string) string {
	if docID == "" {
		return defaultSGCCURL
	}
	docType = strings.TrimSpace(docType)
	if docType == "" {
		docType = inferSGCCDocType(title)
	}
	return fmt.Sprintf("https://ecp.sgcc.com.cn/ecp2.0/portal/#/doc/%s/%s_%s", docType, docID, menuID)
}

func inferSGCCNoticeType(notice sgccNotice) string {
	if notice.DocType != "" {
		switch notice.DocType {
		case "doci-change":
			return "变更公告"
		case "doci-bid":
			return "招标公告"
		}
	}
	return inferNoticeType(notice.Title)
}

func inferSGCCDocType(title string) string {
	switch {
	case strings.Contains(title, "变更") || strings.Contains(title, "澄清"):
		return "doci-change"
	default:
		return "doci-bid"
	}
}

func sgccProjectStatus(value string) string {
	switch value {
	case "1":
		return "正在招标"
	case "2":
		return "已截标"
	case "3":
		return "已结束"
	default:
		if value != "" {
			return value
		}
		return "未知"
	}
}

func normalizeSGCCDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		return value[:10]
	}
	return value
}

func jsonValueString(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return strings.Trim(value, `"`)
}

func parseHTMLForOpportunities(raw string, site SiteConfig, req CrawlRequest) []Opportunity {
	cleaned := stripScripts(raw)
	linkRE := regexp.MustCompile(`(?is)<a\b[^>]*href=["']?([^"'\s>]+)["']?[^>]*>(.*?)</a>`)
	dateRE := regexp.MustCompile(`\d{4}[-/.年]\d{1,2}[-/.月]\d{1,2}`)
	tenderRE := regexp.MustCompile(`(?i)(?:招标编号|采购编号|项目编号|编号)[:：\s]*([A-Z0-9_\-./]+)`)

	base, _ := url.Parse(site.BaseURL)
	seen := map[string]bool{}
	var items []Opportunity

	matches := linkRE.FindAllStringSubmatch(cleaned, -1)
	for _, match := range matches {
		href := html.UnescapeString(strings.TrimSpace(match[1]))
		title := normalizeText(stripTags(match[2]))
		if !looksLikeNotice(title) {
			continue
		}

		sourceURL := resolveURL(base, href)
		key := sourceURL + "|" + title
		if seen[key] {
			continue
		}
		seen[key] = true

		contextText := contextAround(cleaned, match[0])
		publishTime := ""
		if date := dateRE.FindString(contextText); date != "" {
			publishTime = normalizeDate(date)
		}
		tenderNo := ""
		if tender := tenderRE.FindStringSubmatch(contextText); len(tender) > 1 {
			tenderNo = tender[1]
		}

		content := normalizeText(stripTags(contextText))
		item := Opportunity{
			ID:              makeID(site.ID + sourceURL + title),
			SiteID:          site.ID,
			SourceSite:      site.Name,
			Title:           title,
			NoticeType:      inferNoticeType(title + " " + content),
			PublishTime:     publishTime,
			TenderNo:        tenderNo,
			SourceURL:       sourceURL,
			Content:         content,
			MatchedKeywords: matchKeywords(title+" "+content, site, req),
			ContentHash:     makeID(sourceURL + title + content),
			CreatedAt:       nowString(),
			UpdatedAt:       nowString(),
		}
		items = append(items, item)
		if len(items) >= 100 {
			break
		}
	}
	return items
}

func parseCSGHTMLForOpportunities(raw string, site SiteConfig, req CrawlRequest) []Opportunity {
	cleaned := stripScripts(raw)
	linkRE := regexp.MustCompile(`(?is)<a\b[^>]*href=["']?([^"'\s>]+)["']?[^>]*>(.*?)</a>`)
	dateRE := regexp.MustCompile(`\d{4}[-/.年]\d{1,2}[-/.月]\d{1,2}`)
	tenderRE := regexp.MustCompile(`(?i)(?:项目编号|招标编号|采购编号|编号)[:：\s]*([A-Z0-9_\-./]+)`)
	orgRE := regexp.MustCompile(`(?s)([^|<>]{2,80}(?:公司|有限责任公司|有限公司|供应链集团|供应链科技|电网公司|电网有限责任公司))\s*(?:&nbsp;|\xc2\xa0|\s)*\|`)

	base, _ := url.Parse(site.BaseURL)
	cutoff := time.Time{}
	if req.Days > 0 {
		cutoff = time.Now().AddDate(0, 0, -req.Days)
	}

	seen := map[string]bool{}
	var items []Opportunity
	matches := linkRE.FindAllStringSubmatch(cleaned, -1)
	for _, match := range matches {
		href := html.UnescapeString(strings.TrimSpace(match[1]))
		sourceURL := resolveURL(base, href)
		if !isCSGNoticeURL(sourceURL) {
			continue
		}

		title := normalizeCSGTitle(linkTitle(match[0], stripTags(match[2])))
		if !looksLikeNotice(title) {
			continue
		}

		key := sourceURL + "|" + title
		if seen[key] {
			continue
		}
		seen[key] = true

		contextText := contextAround(cleaned, match[0])
		content := normalizeText(stripTags(contextText))
		publishTime := ""
		if date := dateRE.FindString(contextText); date != "" {
			publishTime = normalizeDate(date)
		}
		if !cutoff.IsZero() && publishTime != "" {
			publishedAt, err := time.Parse("2006-01-02", publishTime)
			if err == nil && publishedAt.Before(cutoff) {
				continue
			}
		}

		buyer := ""
		if buyerMatch := orgRE.FindStringSubmatch(content); len(buyerMatch) > 1 {
			buyer = normalizeText(stripTags(buyerMatch[1]))
		}
		tenderNo := ""
		if tender := tenderRE.FindStringSubmatch(content); len(tender) > 1 {
			tenderNo = tender[1]
		} else if bracketNo := extractBracketTenderNo(title); bracketNo != "" {
			tenderNo = bracketNo
		}

		item := Opportunity{
			ID:              makeID(site.ID + sourceURL + title),
			SiteID:          site.ID,
			SourceSite:      site.Name,
			Title:           title,
			NoticeType:      inferNoticeType(title + " " + content),
			PublishTime:     publishTime,
			TenderNo:        tenderNo,
			Buyer:           buyer,
			SourceURL:       sourceURL,
			Content:         content,
			MatchedKeywords: matchKeywords(title+" "+content, site, req),
			ContentHash:     makeID(sourceURL + title + content),
			CreatedAt:       nowString(),
			UpdatedAt:       nowString(),
		}
		items = append(items, item)
		if len(items) >= 100 {
			break
		}
	}
	return items
}

func isCSGNoticeURL(sourceURL string) bool {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return false
	}
	if !strings.Contains(parsed.Host, "bidding.csg.cn") {
		return false
	}
	path := parsed.EscapedPath()
	if !strings.HasSuffix(path, ".jhtml") || strings.Contains(path, "index") {
		return false
	}
	return strings.HasPrefix(path, "/zbgg/") ||
		strings.HasPrefix(path, "/fzbgg/") ||
		strings.HasPrefix(path, "/zbhxrgs/") ||
		strings.HasPrefix(path, "/fbgg/") ||
		strings.HasPrefix(path, "/gsgg/") ||
		strings.HasPrefix(path, "/xygg/")
}

func linkTitle(anchorHTML string, fallback string) string {
	titleAttrRE := regexp.MustCompile(`(?is)\btitle=["']([^"']+)["']`)
	if match := titleAttrRE.FindStringSubmatch(anchorHTML); len(match) > 1 {
		return html.UnescapeString(match[1])
	}
	return fallback
}

func normalizeCSGTitle(title string) string {
	return normalizeText(title)
}

func extractBracketTenderNo(title string) string {
	re := regexp.MustCompile(`\[[A-Z0-9_\-./]+\]`)
	match := re.FindString(title)
	return strings.Trim(match, "[]")
}

func looksLikeNotice(title string) bool {
	if len([]rune(title)) < 6 {
		return false
	}
	words := []string{"招标", "采购", "公告", "公示", "项目", "中标", "成交", "竞争性", "询价", "需求"}
	for _, word := range words {
		if strings.Contains(title, word) {
			return true
		}
	}
	return false
}

func inferNoticeType(text string) string {
	switch {
	case strings.Contains(text, "中标") || strings.Contains(text, "成交"):
		return "中标/成交"
	case strings.Contains(text, "变更") || strings.Contains(text, "澄清"):
		return "变更/澄清"
	case strings.Contains(text, "询价"):
		return "询价"
	case strings.Contains(text, "采购"):
		return "采购"
	case strings.Contains(text, "招标"):
		return "招标"
	default:
		return "公告"
	}
}

func matchKeywords(text string, site SiteConfig, req CrawlRequest) []string {
	candidates := append([]string{}, site.Keywords...)
	if strings.TrimSpace(req.Keyword) != "" {
		candidates = append(candidates, req.Keyword)
	}

	var matched []string
	for _, keyword := range cleanList(candidates) {
		if strings.Contains(strings.ToLower(text), strings.ToLower(keyword)) {
			matched = append(matched, keyword)
		}
	}
	return matched
}

func (a *App) upsertOpportunities(items []Opportunity) (int, int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	index := map[string]int{}
	for i, item := range a.state.Opportunities {
		key := dedupeKey(item)
		index[key] = i
	}

	var newCount, duplicateCount int
	for _, item := range items {
		key := dedupeKey(item)
		if existingIndex, ok := index[key]; ok {
			existing := a.state.Opportunities[existingIndex]
			item.CreatedAt = existing.CreatedAt
			item.UpdatedAt = nowString()
			a.state.Opportunities[existingIndex] = item
			duplicateCount++
			continue
		}
		a.state.Opportunities = append(a.state.Opportunities, item)
		index[key] = len(a.state.Opportunities) - 1
		newCount++
	}
	_ = a.saveLocked()
	return newCount, duplicateCount
}

func (a *App) isKnownOpportunity(item Opportunity) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := dedupeKey(item)
	for _, existing := range a.state.Opportunities {
		if dedupeKey(existing) == key && existing.ContentHash == item.ContentHash {
			return true
		}
	}
	return false
}

func (a *App) recordTask(task CrawlTask) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.Tasks = append(a.state.Tasks, task)
	if len(a.state.Tasks) > 200 {
		a.state.Tasks = a.state.Tasks[len(a.state.Tasks)-200:]
	}
	_ = a.saveLocked()
}

func dedupeKey(item Opportunity) string {
	scope := item.SiteID + "|" + item.CategoryID
	if item.TenderNo != "" {
		return scope + "|no|" + item.TenderNo
	}
	if item.SourceURL != "" {
		return scope + "|url|" + item.SourceURL
	}
	return scope + "|hash|" + item.ContentHash
}

func stripScripts(raw string) string {
	withoutScripts := scriptTagRE.ReplaceAllString(raw, " ")
	return styleTagRE.ReplaceAllString(withoutScripts, " ")
}

func stripTags(raw string) string {
	return html.UnescapeString(htmlTagRE.ReplaceAllString(raw, " "))
}

func normalizeText(text string) string {
	return strings.TrimSpace(spaceRE.ReplaceAllString(text, " "))
}

func normalizeDate(date string) string {
	date = strings.ReplaceAll(date, "年", "-")
	date = strings.ReplaceAll(date, "月", "-")
	date = strings.ReplaceAll(date, "日", "")
	date = strings.ReplaceAll(date, "/", "-")
	date = strings.ReplaceAll(date, ".", "-")
	return date
}

func contextAround(raw string, needle string) string {
	idx := strings.Index(raw, needle)
	if idx < 0 {
		return needle
	}
	start := idx - 400
	if start < 0 {
		start = 0
	}
	end := idx + len(needle) + 400
	if end > len(raw) {
		end = len(raw)
	}
	return raw[start:end]
}

func resolveURL(base *url.URL, href string) string {
	if href == "" {
		return ""
	}
	parsed, err := url.Parse(href)
	if err != nil {
		return href
	}
	if base == nil {
		return parsed.String()
	}
	return base.ResolveReference(parsed).String()
}

func cleanList(values []string) []string {
	seen := map[string]bool{}
	var cleaned []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func makeID(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func nowString() string {
	return time.Now().Format(time.RFC3339)
}

func (a *App) saveLocked() error {
	raw, err := json.MarshalIndent(a.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(a.storePath, raw, 0644)
}
