export namespace browsersession {

	export class Bounds {
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	    viewport_width: number;
	    viewport_height: number;

	    static createFrom(source: any = {}) {
	        return new Bounds(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.viewport_width = source["viewport_width"];
	        this.viewport_height = source["viewport_height"];
	    }
	}
	export class OriginStatus {
	    origin: string;
	    phase: string;

	    static createFrom(source: any = {}) {
	        return new OriginStatus(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.origin = source["origin"];
	        this.phase = source["phase"];
	    }
	}
	export class Status {
	    store_id: string;
	    available: boolean;
	    enabled: boolean;
	    phase: string;
	    message: string;
	    revision: number;
	    origins: OriginStatus[];

	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.store_id = source["store_id"];
	        this.available = source["available"];
	        this.enabled = source["enabled"];
	        this.phase = source["phase"];
	        this.message = source["message"];
	        this.revision = source["revision"];
	        this.origins = this.convertValues(source["origins"], OriginStatus);
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
