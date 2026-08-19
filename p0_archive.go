package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var invalidFileNameRE = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)
var attachmentLinkRE = regexp.MustCompile(`(?is)<a\b[^>]*href=["']?([^"'\s>]+)["']?[^>]*>(.*?)</a>`)

type sgccArchiveDetail struct {
	Body        string
	Attachments []sgccAttachmentSource
}

type sgccAttachmentSource struct {
	Name        string
	URL         string
	AutoExtract bool
}

type attachmentDownload struct {
	Content  []byte
	FileName string
}

func (a *App) archiveOpportunity(item *Opportunity, category NoticeCategory) {
	a.archiveOpportunityWithProgress(item, category, nil)
}

func (a *App) archiveOpportunityWithProgress(item *Opportunity, category NoticeCategory, report func(substep string, completed bool, message string)) {
	archiveRoot := a.GetArchiveConfig().RootPath
	if archiveRoot == "" {
		item.ProcessStatus = "归档失败"
		item.ArchiveError = "未配置归档目录"
		return
	}

	archivePath := filepath.Join(archiveRoot, archiveFolderName(*item))
	if report != nil {
		report("创建文件夹", false, "正在创建公告归档目录")
	}
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		item.ProcessStatus = "归档失败"
		item.ArchiveError = fmt.Sprintf("无法创建归档目录：%v", err)
		return
	}
	item.ArchivePath = archivePath
	if report != nil {
		report("创建文件夹", true, "公告归档目录已创建")
	}

	detail, err := a.fetchArchiveDetail(*item, category)
	detailAvailable := err == nil && strings.TrimSpace(detail.Body) != ""
	body := item.Content
	if detailAvailable {
		body = detail.Body
		item.DetailFetchedAt = nowString()
	}
	if report != nil {
		report("创建 Word 文档", false, "正在写入公告 Word 文档")
	}
	if _, writeErr := writeNoticeDocx(archivePath, *item, body); writeErr != nil {
		item.ProcessStatus = "归档失败"
		item.ArchiveError = fmt.Sprintf("无法写入 Word 文档：%v", writeErr)
		return
	}
	if report != nil {
		report("创建 Word 文档", true, "公告 Word 文档已保存")
	}
	if !detailAvailable {
		if category.ID == "sgcc-bid" {
			if report != nil {
				report("下载附件", false, "详情暂不可用，正在下载公告文件")
			}
			attachments := a.downloadAttachments(sgccBidAnnouncementSources(*item), archivePath)
			item.Attachments = attachments
			if report != nil {
				report("下载附件", true, "公告文件下载完成")
			}
			if report != nil {
				report("生成招标及结果 Excel", false, "详情暂不可用，正在生成待复核招标及结果 Excel")
			}
			if writeErr := writeBidResultWorkbook(archivePath, *item, body, nil); writeErr != nil {
				item.ProcessStatus = "归档失败"
				item.ArchiveError = fmt.Sprintf("详情不可用且无法生成招标及结果 Excel：%v", writeErr)
				return
			}
			if report != nil {
				report("生成招标及结果 Excel", true, "已生成待复核招标及结果 Excel")
			}
		}
		item.ProcessStatus = "待归档"
		if err != nil {
			item.ArchiveError = fmt.Sprintf("详情页未归档：%v", err)
		} else {
			item.ArchiveError = "详情页未返回可识别的公开正文，等待国网详情适配"
		}
		return
	}
	attachments := []Attachment{}
	if category.DownloadAttachments {
		if report != nil {
			report("下载附件", false, "正在下载公告附件")
		}
		attachments = a.downloadAttachments(detail.Attachments, archivePath)
		if report != nil {
			report("下载附件", true, "公告附件下载完成")
		}
	} else if report != nil {
		report("下载附件", true, "该栏目无需下载附件")
	}
	item.Attachments = attachments
	for _, attachment := range attachments {
		if attachment.Status != "已下载" {
			item.ProcessStatus = "部分归档失败"
			item.ArchiveError = "存在附件下载失败，请在详情中重试"
			return
		}
	}
	if category.ID == "sgcc-bid" {
		if report != nil {
			report("生成招标及结果 Excel", false, "正在解压公告文件并汇总标包需求")
		}
		if err := writeBidResultWorkbook(archivePath, *item, body, attachments); err != nil {
			item.ProcessStatus = "部分归档失败"
			item.ArchiveError = fmt.Sprintf("无法生成招标及结果 Excel：%v", err)
			return
		}
		if report != nil {
			report("生成招标及结果 Excel", true, "招标及结果 Excel 已生成")
		}
	} else if report != nil {
		report("生成招标及结果 Excel", true, "该栏目无需生成招标及结果 Excel")
	}
	item.ProcessStatus = "已归档"
	item.ArchiveError = ""
}

func (a *App) fetchArchiveDetail(item Opportunity, category NoticeCategory) (sgccArchiveDetail, error) {
	if item.SiteID == "sgcc-list-spe" {
		switch category.ID {
		case "sgcc-single-source":
			return a.fetchSGCCSingleSourceDetail(item)
		case "sgcc-annual-plan":
			return a.fetchSGCCAnnualPlanDetail(item)
		case "sgcc-prequalification":
			return a.fetchSGCCBidDetail(item)
		case "sgcc-bid":
			return a.fetchSGCCBidDetail(item)
		}
	}
	raw, err := a.fetchPublicDocument(item.SourceURL)
	if err != nil {
		return sgccArchiveDetail{}, err
	}
	if !isUsefulNoticeHTML(string(raw), item) {
		return sgccArchiveDetail{}, fmt.Errorf("详情页未返回可识别的公开正文")
	}
	return sgccArchiveDetail{Body: documentTextFromHTML(string(raw)), Attachments: htmlAttachmentSources(raw, item.SourceURL)}, nil
}

func (a *App) fetchSGCCSingleSourceDetail(item Opportunity) (sgccArchiveDetail, error) {
	ids := cleanList([]string{item.DetailID, item.NoticeID})
	if len(ids) == 0 {
		return sgccArchiveDetail{}, fmt.Errorf("单一来源公告缺少详情标识")
	}
	var response struct {
		Successful  bool   `json:"successful"`
		ResultHint  string `json:"resultHint"`
		ResultValue struct {
			PurSingleList []struct {
				PlanName       string `json:"planname"`
				ProjectName    string `json:"projectname"`
				PurOrgName     string `json:"purorgname"`
				PublishOrgName string `json:"publishorgname"`
				PlanCode       string `json:"plancode"`
				MinCatName     string `json:"mincatname"`
				SupplierName   string `json:"suppliername"`
				Note           string `json:"note"`
			} `json:"purSingleList"`
			FileList []struct {
				Path string `json:"publicityFilePath"`
				Name string `json:"publicityFileName"`
			} `json:"fileList"`
		} `json:"resultValue"`
	}
	var lastErr error
	for _, id := range ids {
		response = struct {
			Successful  bool   `json:"successful"`
			ResultHint  string `json:"resultHint"`
			ResultValue struct {
				PurSingleList []struct {
					PlanName       string `json:"planname"`
					ProjectName    string `json:"projectname"`
					PurOrgName     string `json:"purorgname"`
					PublishOrgName string `json:"publishorgname"`
					PlanCode       string `json:"plancode"`
					MinCatName     string `json:"mincatname"`
					SupplierName   string `json:"suppliername"`
					Note           string `json:"note"`
				} `json:"purSingleList"`
				FileList []struct {
					Path string `json:"publicityFilePath"`
					Name string `json:"publicityFileName"`
				} `json:"fileList"`
			} `json:"resultValue"`
		}{}
		if err := a.postJSON(sgccCoreURL+"purNotice/getPurSingle", map[string]any{"index": 1, "size": 100, "singleSourceId": id}, &response); err != nil {
			lastErr = err
			continue
		}
		if response.Successful && len(response.ResultValue.PurSingleList) > 0 {
			break
		}
		lastErr = fmt.Errorf("单一来源详情接口未返回正文：%s", response.ResultHint)
	}
	if !response.Successful || len(response.ResultValue.PurSingleList) == 0 {
		return sgccArchiveDetail{}, lastErr
	}
	lines := make([]string, 0, len(response.ResultValue.PurSingleList)*6)
	for index, record := range response.ResultValue.PurSingleList {
		if index == 0 {
			lines = append(lines, cleanList([]string{"公示名称：" + record.PlanName, "发布单位：" + record.PublishOrgName, "采购单位：" + record.PurOrgName, "采购编号：" + record.PlanCode, "公示说明：" + record.Note})...)
		}
		lines = append(lines, cleanList([]string{fmt.Sprintf("项目 %d：%s", index+1, record.ProjectName), "采购内容：" + record.MinCatName, "供应商：" + record.SupplierName})...)
	}
	return sgccArchiveDetail{Body: strings.Join(lines, "\n"), Attachments: singleSourceAttachments(response.ResultValue.FileList)}, nil
}

func singleSourceAttachments(files []struct {
	Path string `json:"publicityFilePath"`
	Name string `json:"publicityFileName"`
}) []sgccAttachmentSource {
	attachments := make([]sgccAttachmentSource, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		attachments = append(attachments, sgccAttachmentSource{Name: file.Name, URL: sgccCoreURL + "purStd/downPurSingleFiles?path=" + url.QueryEscape(file.Path) + "&name=" + url.QueryEscape(file.Name)})
	}
	return attachments
}

func (a *App) fetchSGCCAnnualPlanDetail(item Opportunity) (sgccArchiveDetail, error) {
	id := firstNonEmpty(item.DetailID, item.NoticeID)
	if id == "" {
		return sgccArchiveDetail{}, fmt.Errorf("年度采购计划缺少详情标识")
	}
	var response struct {
		Successful  bool   `json:"successful"`
		ResultHint  string `json:"resultHint"`
		ResultValue struct {
			PurplanList []map[string]any `json:"purplanList"`
			PurplanYear map[string]any   `json:"purplanYear"`
		} `json:"resultValue"`
	}
	if err := a.postJSON(sgccCoreURL+"purNotice/getPurplan", map[string]any{"index": 1, "size": 100, "prePurplanYearId": id, "projectType": "", "month": ""}, &response); err != nil {
		return sgccArchiveDetail{}, err
	}
	if !response.Successful {
		return sgccArchiveDetail{}, fmt.Errorf("年度采购计划详情接口未返回正文：%s", response.ResultHint)
	}
	lines := flattenRecord("年度采购计划", response.ResultValue.PurplanYear)
	for index, record := range response.ResultValue.PurplanList {
		lines = append(lines, flattenRecord(fmt.Sprintf("计划项目 %d", index+1), record)...)
	}
	if len(lines) == 0 {
		return sgccArchiveDetail{}, fmt.Errorf("年度采购计划详情接口未返回可用内容")
	}
	return sgccArchiveDetail{Body: strings.Join(lines, "\n")}, nil
}

func (a *App) fetchSGCCBidDetail(item Opportunity) (sgccArchiveDetail, error) {
	id := firstNonEmpty(item.NoticeID, item.DetailID)
	if id == "" {
		return sgccArchiveDetail{}, fmt.Errorf("资格预审公告缺少详情标识")
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		var response struct {
			Successful  bool            `json:"successful"`
			ResultHint  string          `json:"resultHint"`
			ResultValue json.RawMessage `json:"resultValue"`
		}
		if err := a.postJSON(sgccCoreURL+"index/getNoticeBid", id, &response); err != nil {
			lastErr = err
			continue
		}
		var result struct {
			Notice   map[string]any `json:"notice"`
			FileFlag any            `json:"fileFlag"`
		}
		resultValue := bytes.TrimSpace(response.ResultValue)
		if len(resultValue) > 0 && resultValue[0] == '"' {
			var encoded string
			if err := json.Unmarshal(resultValue, &encoded); err != nil {
				lastErr = fmt.Errorf("招标公告详情数据解析失败：%w", err)
				continue
			}
			resultValue = []byte(encoded)
		}
		if err := json.Unmarshal(resultValue, &result); err != nil {
			lastErr = fmt.Errorf("招标公告详情数据解析失败：%w", err)
			continue
		}
		if !response.Successful || len(result.Notice) == 0 {
			lastErr = fmt.Errorf("资格预审详情接口未返回正文：%s", response.ResultHint)
			continue
		}
		lines := flattenRecord("招标公告", result.Notice)
		body := strings.Join(lines, "\n")
		if content, ok := mapString(result.Notice, "CONT", "CONTENT", "NOTICE_CONTENT"); ok {
			body = documentTextFromHTML(content)
			if body == "" {
				body = strings.Join(lines, "\n")
			}
		}
		attachments := []sgccAttachmentSource{}
		if sgccFileAvailable(result.FileFlag) {
			attachments = sgccBidAnnouncementSources(item)
		}
		return sgccArchiveDetail{Body: body, Attachments: attachments}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("资格预审详情接口未返回正文")
	}
	return sgccArchiveDetail{}, lastErr
}

func sgccBidAnnouncementSources(item Opportunity) []sgccAttachmentSource {
	noticeID := strings.TrimSpace(item.NoticeID)
	if noticeID == "" {
		return nil
	}
	return []sgccAttachmentSource{{Name: "公告文件.zip", URL: sgccBidAttachmentURL(noticeID), AutoExtract: true}}
}

// The public page's "下载公告文件" button passes noticeDetId as an empty value.
// The list's detail ID must never be substituted here, or the server returns an empty ZIP.
func sgccBidAttachmentURL(noticeID string) string {
	return sgccCoreURL + "/index/downLoadBid?noticeId=" + url.QueryEscape(strings.TrimSpace(noticeID)) + "&noticeDetId="
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func mapString(record map[string]any, names ...string) (string, bool) {
	for _, name := range names {
		if value, ok := record[name]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text, true
			}
		}
	}
	return "", false
}

func flattenRecord(title string, record map[string]any) []string {
	if len(record) == 0 {
		return nil
	}
	lines := []string{title}
	for _, key := range []string{"PLAN_NAME", "PLANNAME", "PROJECT_NAME", "PROJECTNAME", "PUR_ORG_NAME", "PURORGNAME", "PUBLISH_ORG_NAME", "PUBLISHORGNAME", "PLAN_CODE", "PLANCODE", "NOTICE_TYPE_NAME", "NOTE", "CONTENT", "CONT"} {
		if value, ok := mapString(record, key); ok {
			value = documentTextFromHTML(value)
			if value != "" {
				lines = append(lines, key+"："+value)
			}
		}
	}
	return cleanList(lines)
}

func sgccFileAvailable(value any) bool {
	text := strings.TrimSpace(fmt.Sprint(value))
	return text == "1" || strings.EqualFold(text, "true")
}

func htmlAttachmentSources(raw []byte, sourceURL string) []sgccAttachmentSource {
	base, err := url.Parse(sourceURL)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	attachments := []sgccAttachmentSource{}
	for _, match := range attachmentLinkRE.FindAllStringSubmatch(string(raw), -1) {
		href := html.UnescapeString(strings.TrimSpace(match[1]))
		if !looksLikeAttachment(href) || seen[href] {
			continue
		}
		seen[href] = true
		resolved := resolveURL(base, href)
		attachments = append(attachments, sgccAttachmentSource{Name: attachmentName(match[2], resolved), URL: resolved})
	}
	return attachments
}

func (a *App) restoreMissingArchives(site SiteConfig, category NoticeCategory, days int) int {
	cutoffDate := cutoffDateForDays(days)
	a.mu.Lock()
	candidates := make([]Opportunity, 0)
	for _, item := range a.state.Opportunities {
		if item.SiteID != site.ID || item.CategoryID != category.ID || !isWithinArchiveWindow(item, cutoffDate) || !archiveNeedsRefresh(item, category) {
			continue
		}
		candidates = append(candidates, item)
	}
	a.mu.Unlock()

	restored := 0
	for _, item := range candidates {
		a.archiveOpportunity(&item, category)
		item.UpdatedAt = nowString()
		a.replaceArchivedOpportunity(item)
		restored++
	}
	return restored
}

func isWithinArchiveWindow(item Opportunity, cutoffDate string) bool {
	value := item.PublishTime
	if value == "" {
		value = item.CreatedAt
	}
	if len(value) < 10 {
		return false
	}
	return isOnOrAfterCutoffDate(value, cutoffDate)
}

func archiveIsMissing(item Opportunity, category NoticeCategory) bool {
	if item.ArchivePath == "" {
		return true
	}
	info, err := os.Stat(item.ArchivePath)
	if err != nil || !info.IsDir() {
		return true
	}
	_, err = os.Stat(noticeDocxPath(item.ArchivePath, item))
	if err != nil {
		return true
	}
	if category.ID == "sgcc-bid" {
		workbookPath := filepath.Join(item.ArchivePath, "招标及结果.xlsx")
		if _, err = os.Stat(workbookPath); err != nil {
			return true
		}
		return bidResultWorkbookNeedsRefresh(workbookPath, item.Attachments)
	}
	return false
}

func archiveNeedsRefresh(item Opportunity, category NoticeCategory) bool {
	return item.DetailFetchedAt == "" || archiveIsMissing(item, category)
}

func (a *App) replaceArchivedOpportunity(updated Opportunity) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.state.Opportunities {
		if a.state.Opportunities[i].ID == updated.ID {
			updated.CreatedAt = a.state.Opportunities[i].CreatedAt
			a.state.Opportunities[i] = updated
			_ = a.saveLocked()
			return
		}
	}
}

func (a *App) downloadAttachments(sources []sgccAttachmentSource, archivePath string) []Attachment {
	attachments := make([]Attachment, 0, len(sources))
	for index, source := range sources {
		attachment := Attachment{Name: attachmentName(source.Name, source.URL), SourceURL: source.URL, Status: "待下载"}
		download, err := a.fetchAttachmentDocument(source.URL)
		if err != nil {
			attachment.Status = "下载失败"
			attachment.ErrorReason = err.Error()
			attachments = append(attachments, attachment)
			continue
		}
		name := firstNonEmpty(download.FileName, source.Name, attachment.Name)
		if source.AutoExtract && isZipContent(download.Content) && !strings.EqualFold(filepath.Ext(name), ".zip") {
			name += ".zip"
		}
		name = uniqueAttachmentName(archivePath, attachmentName(name, source.URL), index)
		attachment.Name = name
		localPath := filepath.Join(archivePath, name)
		if err := os.WriteFile(localPath, download.Content, 0644); err != nil {
			attachment.Status = "下载失败"
			attachment.ErrorReason = err.Error()
			attachments = append(attachments, attachment)
			continue
		}
		sum := sha1.Sum(download.Content)
		attachment.LocalPath = localPath
		attachment.Size = int64(len(download.Content))
		attachment.Hash = hex.EncodeToString(sum[:])
		if source.AutoExtract && isZipContent(download.Content) {
			if _, err := unzipBidAttachment(localPath, archivePath); err != nil {
				attachment.Status = "解压失败"
				attachment.ErrorReason = fmt.Sprintf("公告文件已下载，但解压失败：%v", err)
				attachments = append(attachments, attachment)
				continue
			}
		}
		attachment.Status = "已下载"
		attachments = append(attachments, attachment)
	}
	return attachments
}

func uniqueAttachmentName(archivePath string, name string, index int) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	extension := filepath.Ext(name)
	if base == "" {
		base = "附件"
	}
	if index > 0 {
		return trimFileName(fmt.Sprintf("%s_%d%s", base, index+1, extension), 120)
	}
	return name
}

func (a *App) fetchPublicDocument(sourceURL string) ([]byte, error) {
	if strings.TrimSpace(sourceURL) == "" {
		return nil, fmt.Errorf("来源地址为空")
	}
	request, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 OpportunityCrawler/0.2")
	request.Header.Set("Accept", "text/html,application/pdf,application/msword,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.ms-excel,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,*/*")
	response, err := a.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("访问失败：HTTP %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 25*1024*1024))
}

func (a *App) fetchAttachmentDocument(sourceURL string) (attachmentDownload, error) {
	if strings.TrimSpace(sourceURL) == "" {
		return attachmentDownload{}, fmt.Errorf("附件地址为空")
	}
	request, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return attachmentDownload{}, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 OpportunityCrawler/0.2")
	request.Header.Set("Accept", "application/octet-stream,*/*")
	request.Header.Set("Referer", defaultSGCCURL)
	client := &http.Client{Timeout: 5 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return attachmentDownload{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return attachmentDownload{}, fmt.Errorf("附件下载失败：HTTP %d", response.StatusCode)
	}
	const maxAttachmentSize = 512 * 1024 * 1024
	content, err := io.ReadAll(io.LimitReader(response.Body, maxAttachmentSize+1))
	if err != nil {
		return attachmentDownload{}, err
	}
	if len(content) > maxAttachmentSize {
		return attachmentDownload{}, fmt.Errorf("附件超过 512 MB 限制")
	}
	return attachmentDownload{Content: content, FileName: contentDispositionFilename(response.Header.Get("Content-Disposition"))}, nil
}

func contentDispositionFilename(value string) string {
	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(params["filename"])
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}
	return sanitizeFileName(name)
}

func isZipContent(content []byte) bool {
	return len(content) >= 4 && bytes.Equal(content[:2], []byte{'P', 'K'}) &&
		(bytes.Equal(content[2:4], []byte{3, 4}) || bytes.Equal(content[2:4], []byte{5, 6}) || bytes.Equal(content[2:4], []byte{7, 8}))
}

func archiveFolderName(item Opportunity) string {
	date := item.PublishTime
	if date == "" {
		date = strings.ReplaceAll(item.CreatedAt[:min(10, len(item.CreatedAt))], "/", "-")
	}
	name := sanitizeFileName(item.Title)
	if name == "" {
		name = "未命名公告"
	}
	return trimFileName(date+name, 140)
}

func attachmentName(anchorText string, sourceURL string) string {
	name := sanitizeFileName(normalizeText(stripTags(anchorText)))
	if name == "" {
		if parsed, err := url.Parse(sourceURL); err == nil {
			name = sanitizeFileName(filepath.Base(parsed.Path))
		}
	}
	if name == "" || name == "." {
		name = "attachment"
	}
	return trimFileName(name, 120)
}

func sanitizeFileName(value string) string {
	value = invalidFileNameRE.ReplaceAllString(strings.TrimSpace(value), "_")
	value = strings.Trim(value, " .")
	return value
}

func trimFileName(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func looksLikeAttachment(href string) bool {
	path := strings.ToLower(href)
	for _, suffix := range []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".zip", ".rar"} {
		if strings.Contains(path, suffix) {
			return true
		}
	}
	return strings.Contains(path, "download") || strings.Contains(path, "downLoad")
}

func isUsefulNoticeHTML(raw string, item Opportunity) bool {
	text := normalizeText(stripTags(raw))
	return len([]rune(text)) > 200 && strings.Contains(text, item.Title)
}
