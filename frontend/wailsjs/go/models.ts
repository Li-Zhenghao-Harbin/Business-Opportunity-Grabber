export namespace main {
	
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
	export class Dashboard {
	    siteCount: number;
	    enabledSiteCount: number;
	    opportunityCount: number;
	    favoriteCount: number;
	    lastTaskCount: number;
	
	    static createFrom(source: any = {}) {
	        return new Dashboard(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.siteCount = source["siteCount"];
	        this.enabledSiteCount = source["enabledSiteCount"];
	        this.opportunityCount = source["opportunityCount"];
	        this.favoriteCount = source["favoriteCount"];
	        this.lastTaskCount = source["lastTaskCount"];
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
	    isFavorite: boolean;
	    remark: string;
	    contentHash: string;
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
	        this.isFavorite = source["isFavorite"];
	        this.remark = source["remark"];
	        this.contentHash = source["contentHash"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class OpportunityQuery {
	    search: string;
	    siteId: string;
	    onlyFavorite: boolean;
	    onlyWithMatch: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OpportunityQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.search = source["search"];
	        this.siteId = source["siteId"];
	        this.onlyFavorite = source["onlyFavorite"];
	        this.onlyWithMatch = source["onlyWithMatch"];
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
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}

}

