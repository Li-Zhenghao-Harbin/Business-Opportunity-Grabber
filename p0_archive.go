package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var attachmentLinkRE = regexp.MustCompile(`(?is)<a\b[^>]*href=["']?([^"'\s>]+)["']?[^>]*>(.*?)</a>`)
var invalidFileNameRE = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

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

	rawDetail, err := a.fetchPublicDocument(item.SourceURL)
	detailAvailable := err == nil && isUsefulNoticeHTML(string(rawDetail), *item)
	body := item.Content
	if detailAvailable {
		body = documentTextFromHTML(string(rawDetail))
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
		attachments = a.archiveAttachments(rawDetail, item.SourceURL, archivePath)
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
	item.ProcessStatus = "已归档"
	item.ArchiveError = ""
}

func (a *App) restoreMissingArchives(site SiteConfig, category NoticeCategory, days int) int {
	cutoffDate := cutoffDateForDays(days)
	a.mu.Lock()
	candidates := make([]Opportunity, 0)
	for _, item := range a.state.Opportunities {
		if item.SiteID != site.ID || item.CategoryID != category.ID || !isWithinArchiveWindow(item, cutoffDate) || !archiveIsMissing(item) {
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

func archiveIsMissing(item Opportunity) bool {
	if item.ArchivePath == "" {
		return true
	}
	info, err := os.Stat(item.ArchivePath)
	if err != nil || !info.IsDir() {
		return true
	}
	_, err = os.Stat(noticeDocxPath(item.ArchivePath, item))
	return err != nil
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

func (a *App) archiveAttachments(raw []byte, sourceURL string, archivePath string) []Attachment {
	base, err := url.Parse(sourceURL)
	if err != nil {
		return nil
	}
	attachmentsPath := filepath.Join(archivePath, "attachments")
	seen := map[string]bool{}
	attachments := []Attachment{}
	for _, match := range attachmentLinkRE.FindAllStringSubmatch(string(raw), -1) {
		href := html.UnescapeString(strings.TrimSpace(match[1]))
		if !looksLikeAttachment(href) || seen[href] {
			continue
		}
		seen[href] = true
		resolved := resolveURL(base, href)
		name := attachmentName(match[2], resolved)
		attachment := Attachment{Name: name, SourceURL: resolved, Status: "待下载"}
		content, err := a.fetchPublicDocument(resolved)
		if err != nil {
			attachment.Status = "下载失败"
			attachment.ErrorReason = err.Error()
			attachments = append(attachments, attachment)
			continue
		}
		if err := os.MkdirAll(attachmentsPath, 0755); err != nil {
			attachment.Status = "下载失败"
			attachment.ErrorReason = err.Error()
			attachments = append(attachments, attachment)
			continue
		}
		localPath := filepath.Join(attachmentsPath, name)
		if err := os.WriteFile(localPath, content, 0644); err != nil {
			attachment.Status = "下载失败"
			attachment.ErrorReason = err.Error()
			attachments = append(attachments, attachment)
			continue
		}
		sum := sha1.Sum(content)
		attachment.LocalPath = localPath
		attachment.Size = int64(len(content))
		attachment.Hash = hex.EncodeToString(sum[:])
		attachment.Status = "已下载"
		attachments = append(attachments, attachment)
	}
	return attachments
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

func archiveFolderName(item Opportunity) string {
	date := item.PublishTime
	if date == "" {
		date = strings.ReplaceAll(item.CreatedAt[:min(10, len(item.CreatedAt))], "/", "-")
	}
	name := sanitizeFileName(item.Title)
	if name == "" {
		name = "未命名公告"
	}
	return trimFileName(date+"_"+name, 140)
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
