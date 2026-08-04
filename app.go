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
)

const defaultSGCCURL = "https://ecp.sgcc.com.cn/ecp2.0/portal/#/list/list-spe"
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
	storePath string
	state     AppState
	client    *http.Client
}

type AppState struct {
	Sites         []SiteConfig  `json:"sites"`
	Opportunities []Opportunity `json:"opportunities"`
	Tasks         []CrawlTask   `json:"tasks"`
}

type SiteConfig struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	SiteType      string   `json:"siteType"`
	BaseURL       string   `json:"baseUrl"`
	Enabled       bool     `json:"enabled"`
	RenderMode    string   `json:"renderMode"`
	Keywords      []string `json:"keywords"`
	Regions       []string `json:"regions"`
	DateRangeDays int      `json:"dateRangeDays"`
	MinIntervalMS int      `json:"minIntervalMs"`
	MaxRetries    int      `json:"maxRetries"`
	CreatedAt     string   `json:"createdAt"`
	UpdatedAt     string   `json:"updatedAt"`
}

type Opportunity struct {
	ID              string   `json:"id"`
	SiteID          string   `json:"siteId"`
	SourceSite      string   `json:"sourceSite"`
	Title           string   `json:"title"`
	NoticeType      string   `json:"noticeType"`
	PublishTime     string   `json:"publishTime"`
	Region          string   `json:"region"`
	TenderNo        string   `json:"tenderNo"`
	Buyer           string   `json:"buyer"`
	Deadline        string   `json:"deadline"`
	SourceURL       string   `json:"sourceUrl"`
	Content         string   `json:"content"`
	MatchedKeywords []string `json:"matchedKeywords"`
	Remark          string   `json:"remark"`
	ContentHash     string   `json:"contentHash"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

type CrawlTask struct {
	ID             string `json:"id"`
	SiteID         string `json:"siteId"`
	SiteName       string `json:"siteName"`
	Status         string `json:"status"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	TotalCount     int    `json:"totalCount"`
	NewCount       int    `json:"newCount"`
	DuplicateCount int    `json:"duplicateCount"`
	FailedCount    int    `json:"failedCount"`
	ErrorMessage   string `json:"errorMessage"`
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
	Search        string `json:"search"`
	SiteID        string `json:"siteId"`
	OnlyWithMatch bool   `json:"onlyWithMatch"`
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
}

func (a *App) initStore() error {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	appDir := filepath.Join(configDir, "Business Opportunity Grabber")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return err
	}

	a.storePath = filepath.Join(appDir, "bog-data.json")
	if _, err := os.Stat(a.storePath); errors.Is(err, os.ErrNotExist) {
		a.state = AppState{Sites: []SiteConfig{defaultSite()}}
		return a.saveLocked()
	}

	raw, err := os.ReadFile(a.storePath)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		a.state = AppState{Sites: []SiteConfig{defaultSite()}}
		return a.saveLocked()
	}
	if err := json.Unmarshal(raw, &a.state); err != nil {
		return err
	}
	if len(a.state.Sites) == 0 {
		a.state.Sites = []SiteConfig{defaultSite()}
		return a.saveLocked()
	}
	return nil
}

func defaultSite() SiteConfig {
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
		CreatedAt:     now,
		UpdatedAt:     now,
	}
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
	a.mu.Lock()
	targets := a.resolveTargetsLocked(req.SiteIDs)
	a.mu.Unlock()

	if len(targets) == 0 {
		return nil, errors.New("没有可抓取的启用站点")
	}

	var tasks []CrawlTask
	for _, site := range targets {
		task := a.crawlSite(site, req)
		tasks = append(tasks, task)
	}
	return tasks, nil
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
		if query.OnlyWithMatch && len(item.MatchedKeywords) == 0 {
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

func (a *App) SaveRemark(id string, remark string) (Opportunity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for i := range a.state.Opportunities {
		if a.state.Opportunities[i].ID == id {
			a.state.Opportunities[i].Remark = strings.TrimSpace(remark)
			a.state.Opportunities[i].UpdatedAt = nowString()
			return a.state.Opportunities[i], a.saveLocked()
		}
	}
	return Opportunity{}, errors.New("未找到公告")
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

func (a *App) fetchOpportunities(site SiteConfig, req CrawlRequest) ([]Opportunity, error) {
	if site.RenderMode == "browser" {
		return nil, errors.New("该站点配置为浏览器渲染模式，但当前版本尚未实现浏览器渲染抓取，请先改为 HTTP 静态抓取或配置真实列表接口")
	}
	if site.SiteType == "sgcc" || strings.Contains(site.BaseURL, "ecp.sgcc.com.cn") {
		return a.fetchSGCCOpportunities(site, req)
	}

	ctx, cancel := context.WithTimeout(context.Background(), crawlRequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, site.BaseURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "BOG/0.1 (+https://local.app)")
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

func (a *App) fetchSGCCOpportunities(site SiteConfig, req CrawlRequest) ([]Opportunity, error) {
	pageSize := 20
	if req.Days > 30 {
		pageSize = 50
	}
	payload := sgccNoteListRequest{
		Index:           1,
		Size:            pageSize,
		FirstPageMenuID: sgccListSpeMenuID,
		Key:             strings.TrimSpace(req.Keyword),
	}

	var data sgccNoteListResponse
	if err := a.postJSON(sgccNoticeListURL, payload, &data); err != nil {
		return nil, err
	}
	if !data.Successful {
		if data.ResultHint != "" {
			return nil, fmt.Errorf("国家电网公告接口返回失败：%s", data.ResultHint)
		}
		return nil, errors.New("国家电网公告接口返回失败")
	}

	cutoff := time.Time{}
	if req.Days > 0 {
		cutoff = time.Now().AddDate(0, 0, -req.Days)
	}

	items := make([]Opportunity, 0, len(data.ResultValue.NoteList))
	for _, notice := range data.ResultValue.NoteList {
		item := sgccNoticeToOpportunity(notice, site, req)
		if item.Title == "" {
			continue
		}
		if !cutoff.IsZero() && item.PublishTime != "" {
			publishedAt, err := time.Parse("2006-01-02", item.PublishTime)
			if err == nil && publishedAt.Before(cutoff) {
				continue
			}
		}
		items = append(items, item)
	}
	return items, nil
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
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 BOG/0.1")
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
			item.Remark = existing.Remark
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
	if item.TenderNo != "" {
		return item.SiteID + "|no|" + item.TenderNo
	}
	if item.SourceURL != "" {
		return item.SiteID + "|url|" + item.SourceURL
	}
	return item.SiteID + "|hash|" + item.ContentHash
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
