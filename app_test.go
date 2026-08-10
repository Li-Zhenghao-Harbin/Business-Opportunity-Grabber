package main

import "testing"

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
