export namespace config {
	
	export class Config {
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

export namespace main {
	
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
	    tableName: string;
	    rowCount: number;
	    fileCount: number;
	    committedAt: string;
	    overwrote: boolean;
	
	    static createFrom(source: any = {}) {
	        return new CommitResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.tableName = source["tableName"];
	        this.rowCount = source["rowCount"];
	        this.fileCount = source["fileCount"];
	        this.committedAt = source["committedAt"];
	        this.overwrote = source["overwrote"];
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
	export class ExportResult {
	    destPath: string;
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
	
	    static createFrom(source: any = {}) {
	        return new ExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.destPath = source["destPath"];
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
	    }
	}
	export class GridHeader {
	    source: string;
	    weekCode: string;
	    weekStart: string;
	    indexNames: string[];
	    weeks: mps.Week[];
	    total: number;
	    hasStaging: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GridHeader(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.indexNames = source["indexNames"];
	        this.weeks = this.convertValues(source["weeks"], mps.Week);
	        this.total = source["total"];
	        this.hasStaging = source["hasStaging"];
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
	    source: string;
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
	        this.source = source["source"];
	        this.page = source["page"];
	        this.pageSize = source["pageSize"];
	        this.search = source["search"];
	        this.sortField = source["sortField"];
	        this.sortDesc = source["sortDesc"];
	        this.filters = source["filters"];
	    }
	}
	export class Status {
	    hasStaging: boolean;
	    weekCode: string;
	    weekStart: string;
	    stagedRows: number;
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
	        this.hasStaging = source["hasStaging"];
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.stagedRows = source["stagedRows"];
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
	
	export class ArchiveEntry {
	    weekCode: string;
	    weekStart: string;
	    year: number;
	    weekNo: number;
	    weekCodes: string[];
	    indexNames: string[];
	    rowCount: number;
	    fileCount: number;
	    batchId: number;
	    committedAt: string;
	    tableName: string;
	
	    static createFrom(source: any = {}) {
	        return new ArchiveEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.weekCode = source["weekCode"];
	        this.weekStart = source["weekStart"];
	        this.year = source["year"];
	        this.weekNo = source["weekNo"];
	        this.weekCodes = source["weekCodes"];
	        this.indexNames = source["indexNames"];
	        this.rowCount = source["rowCount"];
	        this.fileCount = source["fileCount"];
	        this.batchId = source["batchId"];
	        this.committedAt = source["committedAt"];
	        this.tableName = source["tableName"];
	    }
	}
	export class GridRow {
	    seq: number;
	    index: string[];
	    weeks: string[];
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
	        this.fileName = source["fileName"];
	        this.srcRow = source["srcRow"];
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

