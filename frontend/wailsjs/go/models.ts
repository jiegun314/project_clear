export namespace config {
	
	export class Config {
	    configVersion: number;
	    readColumns: number;
	    locFilter: string;
	    pageSize: number;
	    headerDisplay: string;
	    exportMode: string;
	    exportDir: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configVersion = source["configVersion"];
	        this.readColumns = source["readColumns"];
	        this.locFilter = source["locFilter"];
	        this.pageSize = source["pageSize"];
	        this.headerDisplay = source["headerDisplay"];
	        this.exportMode = source["exportMode"];
	        this.exportDir = source["exportDir"];
	    }
	}

}

export namespace logging {
	
	export class Entry {
	    seq: number;
	    time: string;
	    level: string;
	    source: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.time = source["time"];
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
	    }
	}

}

export namespace mps {
	
	export class Week {
	    code: string;
	    year: number;
	    weekNo: number;
	    // Go type: time
	    start: any;
	
	    static createFrom(source: any = {}) {
	        return new Week(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.year = source["year"];
	        this.weekNo = source["weekNo"];
	        this.start = this.convertValues(source["start"], null);
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

export namespace service {
	
	export class ArchiveEntry {
	    weekCode: string;
	    weekStart: string;
	    year: number;
	    weekNo: number;
	    dataset: string;
	    datasetId: number;
	    version: number;
	    rowCount: number;
	    fileCount: number;
	    committedAt: string;
	    expired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.year = source["year"];
	        this.weekNo = source["weekNo"];
	        this.dataset = source["dataset"];
	        this.datasetId = source["datasetId"];
	        this.version = source["version"];
	        this.rowCount = source["rowCount"];
	        this.fileCount = source["fileCount"];
	        this.committedAt = source["committedAt"];
	        this.expired = source["expired"];
	    }
	}
	export class FileResult {
	    name: string;
	    path: string;
	    size: number;
	    status: string;
	    rowsTotal: number;
	    rowsKept: number;
	    weekCode: string;
	    err?: string;
	
	    static createFrom(source: any = {}) {
	        return new FileResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.status = source["status"];
	        this.rowsTotal = source["rowsTotal"];
	        this.rowsKept = source["rowsKept"];
	        this.weekCode = source["weekCode"];
	        this.err = source["err"];
	    }
	}
	export class FileWithDataset {
	    name: string;
	    path: string;
	    size: number;
	    status: string;
	    rowsTotal: number;
	    rowsKept: number;
	    weekCode: string;
	    err?: string;
	    dataset: string;
	    version: number;
	
	    static createFrom(source: any = {}) {
	        return new FileWithDataset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.status = source["status"];
	        this.rowsTotal = source["rowsTotal"];
	        this.rowsKept = source["rowsKept"];
	        this.weekCode = source["weekCode"];
	        this.err = source["err"];
	        this.dataset = source["dataset"];
	        this.version = source["version"];
	    }
	}
	export class FinishResult {
	    datasetId: number;
	    weekCode: string;
	    weekStart: string;
	    version: number;
	    rowCount: number;
	    fileCount: number;
	    failedCount: number;
	    weekCodes: mps.Week[];
	    indexNames: string[];
	    importedAt: string;
	    expired: boolean;
	    integrationAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new FinishResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.datasetId = source["datasetId"];
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.version = source["version"];
	        this.rowCount = source["rowCount"];
	        this.fileCount = source["fileCount"];
	        this.failedCount = source["failedCount"];
	        this.weekCodes = this.convertValues(source["weekCodes"], mps.Week);
	        this.indexNames = source["indexNames"];
	        this.importedAt = source["importedAt"];
	        this.expired = source["expired"];
	        this.integrationAt = source["integrationAt"];
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
	export class ImportResult {
	    action: string;
	    weekCode: string;
	    weekStart: string;
	    weekCodes: mps.Week[];
	    indexNames: string[];
	    total: number;
	    ok: number;
	    failed: number;
	    rowsKept: number;
	    files: FileResult[];
	    warnings: string[];
	    durationMs: number;
	    replaced?: string[];
	    draft: boolean;
	    datasetId: number;
	    localRows: number;
	    version: number;
	    expired: boolean;
	    rowCount: number;
	    fileCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action = source["action"];
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.weekCodes = this.convertValues(source["weekCodes"], mps.Week);
	        this.indexNames = source["indexNames"];
	        this.total = source["total"];
	        this.ok = source["ok"];
	        this.failed = source["failed"];
	        this.rowsKept = source["rowsKept"];
	        this.files = this.convertValues(source["files"], FileResult);
	        this.warnings = source["warnings"];
	        this.durationMs = source["durationMs"];
	        this.replaced = source["replaced"];
	        this.draft = source["draft"];
	        this.datasetId = source["datasetId"];
	        this.localRows = source["localRows"];
	        this.version = source["version"];
	        this.expired = source["expired"];
	        this.rowCount = source["rowCount"];
	        this.fileCount = source["fileCount"];
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

export namespace store {
	
	export class CellMeta {
	    color?: string;
	    comment?: string;
	    author?: string;
	
	    static createFrom(source: any = {}) {
	        return new CellMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.color = source["color"];
	        this.comment = source["comment"];
	        this.author = source["author"];
	    }
	}
	export class GridRow {
	    seq: number;
	    index: string[];
	    weeks: string[];
	    weekMeta: CellMeta[];
	    fileName: string;
	    srcRow: number;
	
	    static createFrom(source: any = {}) {
	        return new GridRow(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.index = source["index"];
	        this.weeks = source["weeks"];
	        this.weekMeta = this.convertValues(source["weekMeta"], CellMeta);
	        this.fileName = source["fileName"];
	        this.srcRow = source["srcRow"];
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
	export class GridResult {
	    rows: GridRow[];
	    total: number;
	    page: number;
	    pageSize: number;
	
	    static createFrom(source: any = {}) {
	        return new GridResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rows = this.convertValues(source["rows"], GridRow);
	        this.total = source["total"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
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

export namespace view {
	
	export class AppInfo {
	    name: string;
	    fullName: string;
	    version: string;
	    dataDir: string;
	    configPath: string;
	    database: string;
	    goVersion: string;
	    platform: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.fullName = source["fullName"];
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	        this.configPath = source["configPath"];
	        this.database = source["database"];
	        this.goVersion = source["goVersion"];
	        this.platform = source["platform"];
	    }
	}
	export class CommitResult {
	    weekCode: string;
	    weekStart: string;
	    mergedId: number;
	    globalId: number;
	    localId: number;
	    mergedRows: number;
	    globalRows: number;
	    localRows: number;
	    globalVersion: number;
	    localVersion: number;
	    mergedVersion: number;
	    fileCount: number;
	    committedAt: string;
	    overwrote: boolean;
	    wasExpired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CommitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.mergedId = source["mergedId"];
	        this.globalId = source["globalId"];
	        this.localId = source["localId"];
	        this.mergedRows = source["mergedRows"];
	        this.globalRows = source["globalRows"];
	        this.localRows = source["localRows"];
	        this.globalVersion = source["globalVersion"];
	        this.localVersion = source["localVersion"];
	        this.mergedVersion = source["mergedVersion"];
	        this.fileCount = source["fileCount"];
	        this.committedAt = source["committedAt"];
	        this.overwrote = source["overwrote"];
	        this.wasExpired = source["wasExpired"];
	    }
	}
	export class ConfigView {
	    config: config.Config;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.config = this.convertValues(source["config"], config.Config);
	        this.path = source["path"];
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
	export class DiscardWeekResult {
	    weekCode: string;
	    datasets: number;
	
	    static createFrom(source: any = {}) {
	        return new DiscardWeekResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekCode = source["weekCode"];
	        this.datasets = source["datasets"];
	    }
	}
	export class ExportResult {
	    destPath: string;
	    datasetId: number;
	    weekCode: string;
	    mode: string;
	    rows: number;
	    cols: number;
	    comments: number;
	    cfRows: number;
	    stylesUsed: number;
	    dropped: number;
	    sizeBytes: number;
	    preservedVba: boolean;
	    durationMs: number;
	    sheets: number;
	    expired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.destPath = source["destPath"];
	        this.datasetId = source["datasetId"];
	        this.weekCode = source["weekCode"];
	        this.mode = source["mode"];
	        this.rows = source["rows"];
	        this.cols = source["cols"];
	        this.comments = source["comments"];
	        this.cfRows = source["cfRows"];
	        this.stylesUsed = source["stylesUsed"];
	        this.dropped = source["dropped"];
	        this.sizeBytes = source["sizeBytes"];
	        this.preservedVba = source["preservedVba"];
	        this.durationMs = source["durationMs"];
	        this.sheets = source["sheets"];
	        this.expired = source["expired"];
	    }
	}
	export class FilesView {
	    files: service.FileWithDataset[];
	    hasDraft: boolean;
	    fileCount: number;
	    rowCount: number;
	    failedCount: number;
	    note?: string;
	
	    static createFrom(source: any = {}) {
	        return new FilesView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], service.FileWithDataset);
	        this.hasDraft = source["hasDraft"];
	        this.fileCount = source["fileCount"];
	        this.rowCount = source["rowCount"];
	        this.failedCount = source["failedCount"];
	        this.note = source["note"];
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
	export class GridHeader {
	    datasetId: number;
	    kind: string;
	    version: number;
	    weekCode: string;
	    weekStart: string;
	    indexNames: string[];
	    weeks: mps.Week[];
	    total: number;
	    expired: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GridHeader(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.datasetId = source["datasetId"];
	        this.kind = source["kind"];
	        this.version = source["version"];
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.indexNames = source["indexNames"];
	        this.weeks = this.convertValues(source["weeks"], mps.Week);
	        this.total = source["total"];
	        this.expired = source["expired"];
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
	export class GridQuery {
	    datasetId: number;
	    page: number;
	    pageSize: number;
	    search: string;
	    sortField: string;
	    sortDesc: boolean;
	    filters: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new GridQuery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.datasetId = source["datasetId"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.search = source["search"];
	        this.sortField = source["sortField"];
	        this.sortDesc = source["sortDesc"];
	        this.filters = source["filters"];
	    }
	}
	export class Status {
	    hasDraft: boolean;
	    draftId: number;
	    draftWeek: string;
	    draftRows: number;
	    draftFiles: number;
	    draftFailed: number;
	    hasGlobal: boolean;
	    globalId: number;
	    globalWeek: string;
	    globalRows: number;
	    globalFiles: number;
	    globalVersion: number;
	    globalAt: string;
	    hasLocal: boolean;
	    localId: number;
	    localWeek: string;
	    localRows: number;
	    localFiles: number;
	    localVersion: number;
	    localAt: string;
	    localFileName: string;
	    hasIntegration: boolean;
	    integrationId: number;
	    integrationWeek: string;
	    integrationRows: number;
	    integrationAt: string;
	    integrationExpired: boolean;
	    activeWeek: string;
	    unintegrated: boolean;
	    canDiscardWeek: boolean;
	    archivedWeeks: number;
	    archivedRows: number;
	    readColumns: number;
	    locFilter: string;
	    pageSize: number;
	    headerDisplay: string;
	    exportMode: string;
	    database: string;
	    lastAction: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hasDraft = source["hasDraft"];
	        this.draftId = source["draftId"];
	        this.draftWeek = source["draftWeek"];
	        this.draftRows = source["draftRows"];
	        this.draftFiles = source["draftFiles"];
	        this.draftFailed = source["draftFailed"];
	        this.hasGlobal = source["hasGlobal"];
	        this.globalId = source["globalId"];
	        this.globalWeek = source["globalWeek"];
	        this.globalRows = source["globalRows"];
	        this.globalFiles = source["globalFiles"];
	        this.globalVersion = source["globalVersion"];
	        this.globalAt = source["globalAt"];
	        this.hasLocal = source["hasLocal"];
	        this.localId = source["localId"];
	        this.localWeek = source["localWeek"];
	        this.localRows = source["localRows"];
	        this.localFiles = source["localFiles"];
	        this.localVersion = source["localVersion"];
	        this.localAt = source["localAt"];
	        this.localFileName = source["localFileName"];
	        this.hasIntegration = source["hasIntegration"];
	        this.integrationId = source["integrationId"];
	        this.integrationWeek = source["integrationWeek"];
	        this.integrationRows = source["integrationRows"];
	        this.integrationAt = source["integrationAt"];
	        this.integrationExpired = source["integrationExpired"];
	        this.activeWeek = source["activeWeek"];
	        this.unintegrated = source["unintegrated"];
	        this.canDiscardWeek = source["canDiscardWeek"];
	        this.archivedWeeks = source["archivedWeeks"];
	        this.archivedRows = source["archivedRows"];
	        this.readColumns = source["readColumns"];
	        this.locFilter = source["locFilter"];
	        this.pageSize = source["pageSize"];
	        this.headerDisplay = source["headerDisplay"];
	        this.exportMode = source["exportMode"];
	        this.database = source["database"];
	        this.lastAction = source["lastAction"];
	    }
	}

}

