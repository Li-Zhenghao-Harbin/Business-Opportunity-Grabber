package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
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
	if len(site.Categories) != 4 {
		t.Fatalf("expected 4 SGCC auto categories, got %d", len(site.Categories))
	}
	if site.Categories[0].PagePath == "" || site.Categories[1].PagePath == "" || site.Categories[2].PagePath == "" || site.Categories[3].PagePath == "" {
		t.Fatal("expected each SGCC auto category to include its PDF page path")
	}
}

func TestMergeBidPackageRows(t *testing.T) {
	rows := mergeBidPackageRows([]bidPackageRow{
		{SectionName: "变压器", SectionNo: "SB-01", PackageNo: "1", Amount: 120, Quantity: 2},
		{SectionName: "变压器", SectionNo: "SB-01", PackageNo: "1", Amount: 30, Quantity: 1},
		{SectionName: "开关", SectionNo: "SB-02", PackageNo: "2", Amount: 80, Quantity: 4},
	})
	if len(rows) != 2 {
		t.Fatalf("expected 2 merged package rows, got %d", len(rows))
	}
	for _, row := range rows {
		if row.SectionNo == "SB-01" && (row.Amount != 150 || row.Quantity != 3) {
			t.Fatalf("expected duplicate package values to merge, got %#v", row)
		}
	}
}

func TestExtractBidRowsSummarizesUniquePackageInSingleSheet(t *testing.T) {
	rows := extractBidRows([]string{
		"国网甘肃电力2026年采购（综合服务标段）需求一览表",
		"分包编号\t包名称\t含税最高限价(万元)\t数量",
		"272635-9003001-9998-包01\t项目甲\t125\t2",
		"272635-9003001-9998-包01\t项目乙\t2.5\t1",
	})
	rows = mergeBidPackageRows(rows)
	if len(rows) != 1 {
		t.Fatalf("expected one unique package, got %#v", rows)
	}
	row := rows[0]
	if row.SectionName != "综合服务标段" || row.SectionNo != "9003001" || row.PackageNo != "包01" || row.Amount != 127.5 || row.Quantity != 3 {
		t.Fatalf("unexpected summarized package: %#v", row)
	}
}

func TestUnzipBidAttachmentExpandsNestedZIP(t *testing.T) {
	inner := newZIPBytes(t, map[string][]byte{"招标公告272635ZZ/需求一览表.xlsx": []byte("workbook")})
	outer := newZIPBytes(t, map[string][]byte{"招标公告272635ZZ.zip": inner})
	root := t.TempDir()
	zipPath := filepath.Join(root, "招标公告.zip")
	if err := os.WriteFile(zipPath, outer, 0644); err != nil {
		t.Fatal(err)
	}
	paths, err := unzipBidAttachment(zipPath, root)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(root, "招标公告", "招标公告272635ZZ", "招标公告272635ZZ", "需求一览表.xlsx")
	found := false
	for _, path := range paths {
		if path == expected {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected nested workbook path %q, got %#v", expected, paths)
	}
}

func newZIPBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestDateCutoffIncludesCurrentDay(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	if !isOnOrAfterCutoffDate(today, cutoffDateForDays(1)) {
		t.Fatal("current-day notices must remain eligible for a one-day crawl")
	}
	if isOnOrAfterCutoffDate("2020-01-01", cutoffDateForDays(1)) {
		t.Fatal("old notices must not pass the crawl cutoff")
	}
}

func TestLiveSingleSourceArchiveDetail(t *testing.T) {
	if os.Getenv("SGCC_LIVE_DETAIL") != "1" {
		t.Skip("live detail verification is opt-in")
	}
	app := NewApp()
	detail, err := app.fetchSGCCSingleSourceDetail(Opportunity{DetailID: "2607249458694064"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail.Body, "安徽淮南寿县丰庄") {
		t.Fatalf("expected page detail content, got %q", detail.Body)
	}
	if len(detail.Attachments) == 0 || !strings.Contains(detail.Attachments[0].Name, "专业论证意见") {
		t.Fatalf("expected listed attachment sources, got %#v", detail.Attachments)
	}
}

func TestLiveBidAnnouncementDetailAndDownload(t *testing.T) {
	if os.Getenv("SGCC_LIVE_DETAIL") != "1" {
		t.Skip("live detail verification is opt-in")
	}
	app := NewApp()
	item := Opportunity{NoticeID: "2608189739928729", DetailID: "2608189740080817"}
	detail, err := app.fetchSGCCBidDetail(item)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(detail.Body) == "" || len(detail.Attachments) != 1 {
		t.Fatalf("expected bid detail and announcement attachment, got %#v", detail)
	}
	download, err := app.fetchAttachmentDocument(detail.Attachments[0].URL)
	if err != nil {
		t.Fatal(err)
	}
	if !isZipContent(download.Content) || len(download.Content) <= 22 {
		t.Fatalf("expected non-empty announcement ZIP, got %d bytes", len(download.Content))
	}
	root := t.TempDir()
	zipPath := filepath.Join(root, "招标公告.zip")
	if err := os.WriteFile(zipPath, download.Content, 0644); err != nil {
		t.Fatal(err)
	}
	paths, err := unzipBidAttachment(zipPath, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected the real announcement ZIP to contain files")
	}
	for _, path := range paths {
		if !strings.HasPrefix(path, filepath.Join(root, "招标公告")) || strings.Contains(filepath.Base(path), "�") {
			t.Fatalf("unexpected extracted path: %q", path)
		}
	}
	rows, notes := collectBidPackageRows(root, []Attachment{{Name: "招标公告.zip", LocalPath: zipPath, Status: "已下载"}})
	if len(rows) == 0 {
		t.Fatalf("expected package demand rows from the real announcement, notes: %#v", notes)
	}
	found := false
	for _, row := range rows {
		if row.SectionName == "信息系统服务标段" && row.SectionNo == "9007001" && row.PackageNo == "包01" && row.Amount == 9.6142 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected known package from the real announcement, got %#v", rows[:min(10, len(rows))])
	}
}

func TestArchiveNeedsRefreshWhenDetailWasNeverFetched(t *testing.T) {
	root := t.TempDir()
	item := Opportunity{Title: "测试公告", ArchivePath: root}
	if !archiveNeedsRefresh(item, NoticeCategory{}) {
		t.Fatal("archive without fetched detail must be refreshed")
	}
	item.DetailFetchedAt = nowString()
	if !archiveNeedsRefresh(item, NoticeCategory{}) {
		t.Fatal("archive without Word document must be refreshed")
	}
	if _, err := writeNoticeDocx(root, item, "正文"); err != nil {
		t.Fatal(err)
	}
	if archiveNeedsRefresh(item, NoticeCategory{}) {
		t.Fatal("complete archive should not need refresh")
	}
	if !archiveNeedsRefresh(item, NoticeCategory{ID: "sgcc-bid"}) {
		t.Fatal("bid archive without result workbook must be refreshed")
	}
	if err := writeBidResultWorkbook(root, item, "", nil); err != nil {
		t.Fatal(err)
	}
	if archiveNeedsRefresh(item, NoticeCategory{ID: "sgcc-bid"}) {
		t.Fatal("bid archive with result workbook should not need refresh")
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
	if _, err := os.Stat(filepath.Join(item.ArchivePath, "notice.html")); !os.IsNotExist(err) {
		t.Fatalf("notice.html should not be created, got %v", err)
	}
	docxPath := filepath.Join(item.ArchivePath, "测试采购公告.docx")
	docx, err := zip.OpenReader(docxPath)
	if err != nil {
		t.Fatalf("expected readable Word document: %v", err)
	}
	defer docx.Close()
	foundDocumentXML := false
	for _, entry := range docx.File {
		if entry.Name != "word/document.xml" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), "测试采购公告") || !strings.Contains(string(content), "公告正文") {
			t.Fatalf("Word document does not contain expected notice text: %s", content)
		}
		foundDocumentXML = true
		break
	}
	if !foundDocumentXML {
		t.Fatal("Word document is missing word/document.xml")
	}
	if len(item.Attachments) != 1 || item.Attachments[0].Status != "已下载" {
		t.Fatalf("unexpected attachments: %#v", item.Attachments)
	}
}

func TestDownloadBidAnnouncementFileUsesResponseNameAndExtractsZIP(t *testing.T) {
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	entry, err := zipWriter.Create("需求一览表.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("test workbook")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Disposition", "attachment; filename=%E6%8B%9B%E6%A0%87%E5%85%AC%E5%91%8A.zip")
		writer.Header().Set("Content-Type", "application/octet-stream")
		_, _ = writer.Write(archive.Bytes())
	}))
	defer server.Close()

	root := t.TempDir()
	attachments := NewApp().downloadAttachments([]sgccAttachmentSource{{
		Name:        "公告文件",
		URL:         server.URL,
		AutoExtract: true,
	}}, root)
	if len(attachments) != 1 {
		t.Fatalf("expected one attachment, got %#v", attachments)
	}
	if attachments[0].Status != "已下载" || attachments[0].Name != "招标公告.zip" {
		t.Fatalf("unexpected attachment: %#v", attachments[0])
	}
	if _, err := os.Stat(filepath.Join(root, "招标公告.zip")); err != nil {
		t.Fatalf("expected downloaded ZIP: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "招标公告", "需求一览表.xlsx")); err != nil {
		t.Fatalf("expected extracted announcement file: %v", err)
	}
}

func TestDecodeZIPEntryNameDecodesGBK(t *testing.T) {
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte("需求一览表.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	if name := decodeZIPEntryName(string(encoded), true); name != "需求一览表.xlsx" {
		t.Fatalf("expected GBK file name to decode, got %q", name)
	}
}

func TestSGCCBidAttachmentURLMatchesPublicDownloadButton(t *testing.T) {
	url := sgccBidAttachmentURL("2608189739928729")
	if !strings.Contains(url, "noticeId=2608189739928729") {
		t.Fatalf("expected notice ID in download URL, got %q", url)
	}
	if !strings.HasSuffix(url, "&noticeDetId=") {
		t.Fatalf("download URL must include an empty noticeDetId like the public page button: %q", url)
	}
	if strings.Contains(url, "noticeDetId=2608189740080817") {
		t.Fatalf("download URL must not use the list detail ID: %q", url)
	}

	if sources := sgccBidAnnouncementSources(Opportunity{NoticeID: "2608189739928729"}); len(sources) != 1 || sources[0].URL != url {
		t.Fatalf("expected a public announcement source, got %#v", sources)
	}
}

func TestSGCCBidDetailUsesNoticeIDForPublicEndpoints(t *testing.T) {
	item := Opportunity{NoticeID: "2608189739928729", DetailID: "2608189740080817"}
	if sources := sgccBidAnnouncementSources(item); len(sources) != 1 || !strings.Contains(sources[0].URL, "noticeId=2608189739928729") {
		t.Fatalf("expected download to use notice ID, got %#v", sources)
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

func TestRestoreMissingArchives(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`<html><body><h1>恢复归档公告</h1><p>` + strings.Repeat("归档正文", 80) + `</p></body></html>`))
	}))
	defer server.Close()

	root := t.TempDir()
	site := defaultSGCCSite()
	category := site.Categories[0]
	item := Opportunity{
		ID:           "restore-me",
		SiteID:       site.ID,
		CategoryID:   category.ID,
		CategoryName: category.Name,
		Title:        "恢复归档公告",
		PublishTime:  time.Now().Format("2006-01-02"),
		SourceURL:    server.URL,
		Content:      "列表摘要",
	}
	app := NewApp()
	app.state = AppState{Archive: ArchiveConfig{RootPath: root}, Opportunities: []Opportunity{item}}

	if restored := app.restoreMissingArchives(site, category, 1); restored != 1 {
		t.Fatalf("expected one restored archive, got %d", restored)
	}
	restoredItem := app.state.Opportunities[0]
	if _, err := os.Stat(noticeDocxPath(restoredItem.ArchivePath, restoredItem)); err != nil {
		t.Fatalf("expected restored Word document: %v", err)
	}
}

func TestClearHistoryResetsWatermarksAndArchiveFolders(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "2026-08-11_测试公告")
	if err := os.MkdirAll(archivePath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archivePath, "测试公告.docx"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	storePath := filepath.Join(t.TempDir(), "opportunity-data.json")
	app := NewApp()
	app.storePath = storePath
	app.state = AppState{
		Archive:       ArchiveConfig{RootPath: root},
		Opportunities: []Opportunity{{ID: "notice", ArchivePath: archivePath}},
		Tasks:         []CrawlTask{{ID: "task"}},
		Sites:         []SiteConfig{{ID: "sgcc", Watermarks: []CrawlWatermark{{CategoryID: "sgcc-single-source"}}}},
	}

	result, err := app.ClearHistory()
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedOpportunities != 1 || result.DeletedTasks != 1 || result.DeletedFolders != 1 {
		t.Fatalf("unexpected clear result: %#v", result)
	}
	if len(app.state.Opportunities) != 0 || len(app.state.Tasks) != 0 || len(app.state.Sites[0].Watermarks) != 0 {
		t.Fatalf("history was not reset: %#v", app.state)
	}
	if app.state.Opportunities == nil || app.state.Tasks == nil || app.state.Sites[0].Watermarks == nil {
		t.Fatal("cleared collections must remain empty arrays")
	}
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Fatalf("archive folder should be removed, got %v", err)
	}
}
