package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCSGHTMLForOpportunities(t *testing.T) {
	site := defaultCSGSite()
	raw := `
		<div class="nav"><a href="/zbgg/index.jhtml">招标公告</a></div>
		<ul>
			<li>
				<span>2026-08-09</span>
				<span>南方电网供应链集团有限公司 |</span>
				<a href="/zbgg/1200438672.jhtml" title="南方电网公司2026年配网设备第一批框架招标项目招标公告">
					南方电网公司2026年配网设备第一批框架招标项目招...
				</a>
			</li>
			<li>
				<span>2026-08-01</span>
				<span>广东电网有限责任公司 |</span>
				<a href="/zbhxrgs/1200438000.jhtml">广东电网有限责任公司2026年采购项目成交公告</a>
			</li>
		</ul>`

	items := parseCSGHTMLForOpportunities(raw, site, CrawlRequest{Days: 30})
	if len(items) != 2 {
		t.Fatalf("expected 2 notices, got %d: %#v", len(items), items)
	}
	if items[0].Title != "南方电网公司2026年配网设备第一批框架招标项目招标公告" {
		t.Fatalf("unexpected title: %q", items[0].Title)
	}
	if items[0].SourceURL != "https://www.bidding.csg.cn/zbgg/1200438672.jhtml" {
		t.Fatalf("unexpected source url: %q", items[0].SourceURL)
	}
	if items[0].PublishTime != "2026-08-09" {
		t.Fatalf("unexpected publish time: %q", items[0].PublishTime)
	}
	if items[0].Buyer == "" {
		t.Fatal("expected buyer to be parsed")
	}
}

func TestDefaultSitesIncludesCSG(t *testing.T) {
	sites := defaultSites()
	if len(sites) != 2 {
		t.Fatalf("expected 2 default sites, got %d", len(sites))
	}
	if sites[1].ID != "csg-zbcg" || sites[1].SiteType != "csg" || sites[1].BaseURL != defaultCSGURL {
		t.Fatalf("unexpected CSG site config: %#v", sites[1])
	}
}

func TestDefaultSGCCSiteIncludesP0Categories(t *testing.T) {
	site := defaultSGCCSite()
	if len(site.Categories) != 7 {
		t.Fatalf("expected 7 SGCC categories, got %d", len(site.Categories))
	}
	if site.Categories[3].ID != "sgcc-bid" || site.Categories[3].MenuID != sgccListSpeMenuID {
		t.Fatalf("unexpected bid category: %#v", site.Categories[3])
	}
}

func TestNextRunAtDaily(t *testing.T) {
	now := time.Date(2026, time.August, 11, 10, 30, 0, 0, time.Local)
	value := nextRunAt(now, ScheduleConfig{Mode: "daily", DailyTime: "09:00"})
	next, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	if next.Day() != 12 || next.Hour() != 9 || next.Minute() != 0 {
		t.Fatalf("unexpected next daily run: %s", value)
	}
}

func TestArchiveOpportunityWritesSnapshotAndAttachment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/notice":
			_, _ = writer.Write([]byte(`<html><body><h1>测试采购公告</h1><p>` + strings.Repeat("公告正文", 80) + `</p><a href="/files/list.xlsx">需求一览表.xlsx</a></body></html>`))
		case "/files/list.xlsx":
			_, _ = writer.Write([]byte("test attachment"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	app := NewApp()
	app.state.Archive = ArchiveConfig{RootPath: root}
	item := Opportunity{Title: "测试采购公告", PublishTime: "2026-08-11", SourceURL: server.URL + "/notice", Content: "列表摘要"}
	app.archiveOpportunity(&item, NoticeCategory{DownloadAttachments: true})

	if item.ProcessStatus != "已归档" {
		t.Fatalf("expected archived status, got %q: %s", item.ProcessStatus, item.ArchiveError)
	}
	if _, err := os.Stat(filepath.Join(item.ArchivePath, "notice.html")); err != nil {
		t.Fatalf("expected notice snapshot: %v", err)
	}
	if len(item.Attachments) != 1 || item.Attachments[0].Status != "已下载" {
		t.Fatalf("unexpected attachments: %#v", item.Attachments)
	}
}

func TestDedupeKeySeparatesCategories(t *testing.T) {
	base := Opportunity{SiteID: "sgcc", TenderNo: "same-project"}
	first := base
	first.CategoryID = "sgcc-bid"
	second := base
	second.CategoryID = "sgcc-winners"
	if dedupeKey(first) == dedupeKey(second) {
		t.Fatal("expected different dedupe keys for different categories")
	}
}
