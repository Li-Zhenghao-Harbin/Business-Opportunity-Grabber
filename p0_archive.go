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
	archiveRoot := a.GetArchiveConfig().RootPath
	if archiveRoot == "" {
		item.ProcessStatus = "归档失败"
		item.ArchiveError = "未配置归档目录"
		return
	}

	archivePath := filepath.Join(archiveRoot, archiveFolderName(*item))
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		item.ProcessStatus = "归档失败"
		item.ArchiveError = fmt.Sprintf("无法创建归档目录：%v", err)
		return
	}
	item.ArchivePath = archivePath
	if err := os.WriteFile(filepath.Join(archivePath, "notice.html"), []byte(offlineNoticeHTML(*item)), 0644); err != nil {
		item.ProcessStatus = "归档失败"
		item.ArchiveError = fmt.Sprintf("无法写入公告快照：%v", err)
		return
	}

	rawDetail, err := a.fetchPublicDocument(item.SourceURL)
	if err != nil || !isUsefulNoticeHTML(string(rawDetail), *item) {
		item.ProcessStatus = "待归档"
		if err != nil {
			item.ArchiveError = fmt.Sprintf("详情页未归档：%v", err)
		} else {
			item.ArchiveError = "详情页未返回可识别的公开正文，等待国网详情适配"
		}
		return
	}

	if err := os.WriteFile(filepath.Join(archivePath, "source.html"), rawDetail, 0644); err != nil {
		item.ProcessStatus = "归档失败"
		item.ArchiveError = fmt.Sprintf("无法写入详情快照：%v", err)
		return
	}
	item.DetailFetchedAt = nowString()
	attachments := []Attachment{}
	if category.DownloadAttachments {
		attachments = a.archiveAttachments(rawDetail, item.SourceURL, archivePath)
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

func offlineNoticeHTML(item Opportunity) string {
	return "<!doctype html><html lang=\"zh-CN\"><meta charset=\"utf-8\"><title>" + html.EscapeString(item.Title) + "</title><body><h1>" + html.EscapeString(item.Title) + "</h1><dl><dt>来源</dt><dd>" + html.EscapeString(item.SourceSite) + "</dd><dt>栏目</dt><dd>" + html.EscapeString(item.CategoryName) + "</dd><dt>发布时间</dt><dd>" + html.EscapeString(item.PublishTime) + "</dd><dt>编号</dt><dd>" + html.EscapeString(item.TenderNo) + "</dd><dt>原文</dt><dd><a href=\"" + html.EscapeString(item.SourceURL) + "\">" + html.EscapeString(item.SourceURL) + "</a></dd></dl><pre>" + html.EscapeString(item.Content) + "</pre></body></html>"
}
