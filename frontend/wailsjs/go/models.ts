export namespace main {

	export class ArchiveConfig {
	    rootPath: string;

	    static createFrom(source: any = {}) {
	        return new ArchiveConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rootPath = source["rootPath"];
	    }
	}
	export class Attachment {
	    name: string;
	    sourceUrl: string;
	    localPath: string;
	    size: number;
	    hash: string;
	    status: string;
	    errorReason: string;

	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.sourceUrl = source["sourceUrl"];
	        this.localPath = source["localPath"];
	        this.size = source["size"];
	        this.hash = source["hash"];
	        this.status = source["status"];
	        this.errorReason = source["errorReason"];
	    }
	}
	export class CrawlRequest {
	    siteIds: string[];
	    keyword: string;
	    days: number;

	    static createFrom(source: any = {}) {
	        return new CrawlRequest(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.siteIds = source["siteIds"];
	        this.keyword = source["keyword"];
	        this.days = source["days"];
	    }
	}
	export class CrawlTask {
	    id: string;
	    siteId: string;
	    siteName: string;
	    categoryId: string;
	    categoryName: string;
	    status: string;
	    startedAt: string;
	    finishedAt: string;
	    totalCount: number;
	    newCount: number;
	    duplicateCount: number;
	    failedCount: number;
	    errorMessage: string;

	    static createFrom(source: any = {}) {
	        return new CrawlTask(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.siteId = source["siteId"];
	        this.siteName = source["siteName"];
	        this.categoryId = source["categoryId"];
	        this.categoryName = source["categoryName"];
	        this.status = source["status"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	        this.totalCount = source["totalCount"];
	        this.newCount = source["newCount"];
	        this.duplicateCount = source["duplicateCount"];
	        this.failedCount = source["failedCount"];
	        this.errorMessage = source["errorMessage"];
	    }
	}
	export class CrawlWatermark {
	    categoryId: string;
	    lastSuccessAt: string;
	    lastNoticeTime: string;
	    lastNoticeId: string;

	    static createFrom(source: any = {}) {
	        return new CrawlWatermark(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.categoryId = source["categoryId"];
	        this.lastSuccessAt = source["lastSuccessAt"];
	        this.lastNoticeTime = source["lastNoticeTime"];
	        this.lastNoticeId = source["lastNoticeId"];
	    }
	}
	export class Dashboard {
	    siteCount: number;
	    enabledSiteCount: number;
	    opportunityCount: number;
	    lastTaskCount: number;

	    static createFrom(source: any = {}) {
	        return new Dashboard(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.siteCount = source["siteCount"];
	        this.enabledSiteCount = source["enabledSiteCount"];
	        this.opportunityCount = source["opportunityCount"];
	        this.lastTaskCount = source["lastTaskCount"];
	    }
	}
	export class HistoryClearResult {
	    deletedOpportunities: number;
	    deletedTasks: number;
	    deletedFolders: number;

	    static createFrom(source: any = {}) {
	        return new HistoryClearResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deletedOpportunities = source["deletedOpportunities"];
	        this.deletedTasks = source["deletedTasks"];
	        this.deletedFolders = source["deletedFolders"];
	    }
	}
	export class NoticeCategory {
	    id: string;
	    name: string;
	    menuId: string;
	    noticeType: string;
	    enabled: boolean;
	    downloadAttachments: boolean;
	    archiveProject: boolean;

	    static createFrom(source: any = {}) {
	        return new NoticeCategory(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.menuId = source["menuId"];
	        this.noticeType = source["noticeType"];
	        this.enabled = source["enabled"];
	        this.downloadAttachments = source["downloadAttachments"];
	        this.archiveProject = source["archiveProject"];
	    }
	}
	export class Opportunity {
	    id: string;
	    siteId: string;
	    sourceSite: string;
	    title: string;
	    noticeType: string;
	    publishTime: string;
	    region: string;
	    tenderNo: string;
	    buyer: string;
	    deadline: string;
	    sourceUrl: string;
	    content: string;
	    matchedKeywords: string[];
	    contentHash: string;
	    categoryId: string;
	    categoryName: string;
	    noticeId: string;
	    detailId: string;
	    processStatus: string;
	    archivePath: string;
	    detailFetchedAt: string;
	    archiveError: string;
	    attachments: Attachment[];
	    createdAt: string;
	    updatedAt: string;

	    static createFrom(source: any = {}) {
	        return new Opportunity(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.siteId = source["siteId"];
	        this.sourceSite = source["sourceSite"];
	        this.title = source["title"];
	        this.noticeType = source["noticeType"];
	        this.publishTime = source["publishTime"];
	        this.region = source["region"];
	        this.tenderNo = source["tenderNo"];
	        this.buyer = source["buyer"];
	        this.deadline = source["deadline"];
	        this.sourceUrl = source["sourceUrl"];
	        this.content = source["content"];
	        this.matchedKeywords = source["matchedKeywords"];
	        this.contentHash = source["contentHash"];
	        this.categoryId = source["categoryId"];
	        this.categoryName = source["categoryName"];
	        this.noticeId = source["noticeId"];
	        this.detailId = source["detailId"];
	        this.processStatus = source["processStatus"];
	        this.archivePath = source["archivePath"];
	        this.detailFetchedAt = source["detailFetchedAt"];
	        this.archiveError = source["archiveError"];
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OpportunityQuery {
	    search: string;
	    siteId: string;

	    static createFrom(source: any = {}) {
	        return new OpportunityQuery(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.search = source["search"];
	        this.siteId = source["siteId"];
	    }
	}
	export class ScheduleConfig {
	    enabled: boolean;
	    mode: string;
	    intervalMinutes: number;
	    dailyTime: string;
	    lastRunAt: string;
	    nextRunAt: string;

	    static createFrom(source: any = {}) {
	        return new ScheduleConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.mode = source["mode"];
	        this.intervalMinutes = source["intervalMinutes"];
	        this.dailyTime = source["dailyTime"];
	        this.lastRunAt = source["lastRunAt"];
	        this.nextRunAt = source["nextRunAt"];
	    }
	}
	export class SiteConfig {
	    id: string;
	    name: string;
	    siteType: string;
	    baseUrl: string;
	    enabled: boolean;
	    renderMode: string;
	    keywords: string[];
	    regions: string[];
	    dateRangeDays: number;
	    minIntervalMs: number;
	    maxRetries: number;
	    categories: NoticeCategory[];
	    watermarks: CrawlWatermark[];
	    createdAt: string;
	    updatedAt: string;

	    static createFrom(source: any = {}) {
	        return new SiteConfig(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.siteType = source["siteType"];
	        this.baseUrl = source["baseUrl"];
	        this.enabled = source["enabled"];
	        this.renderMode = source["renderMode"];
	        this.keywords = source["keywords"];
	        this.regions = source["regions"];
	        this.dateRangeDays = source["dateRangeDays"];
	        this.minIntervalMs = source["minIntervalMs"];
	        this.maxRetries = source["maxRetries"];
	        this.categories = this.convertValues(source["categories"], NoticeCategory);
	        this.watermarks = this.convertValues(source["watermarks"], CrawlWatermark);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

