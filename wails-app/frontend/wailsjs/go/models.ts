export namespace main {
	
	export class ActionInfo {
	    id: string;
	    title: string;
	    type: string;
	    state: string;
	    platform: string;
	    keywords: string;
	    content_message: string;
	    reached_index: number;
	    exec_count: number;
	    target_count: number;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new ActionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.type = source["type"];
	        this.state = source["state"];
	        this.platform = source["platform"];
	        this.keywords = source["keywords"];
	        this.content_message = source["content_message"];
	        this.reached_index = source["reached_index"];
	        this.exec_count = source["exec_count"];
	        this.target_count = source["target_count"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class CreateActionRequest {
	    title: string;
	    type: string;
	    platform: string;
	    keywords: string;
	    content_message: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateActionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.type = source["type"];
	        this.platform = source["platform"];
	        this.keywords = source["keywords"];
	        this.content_message = source["content_message"];
	    }
	}
	export class SessionSummary {
	    platform: string;
	    username: string;
	    expiry: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platform = source["platform"];
	        this.username = source["username"];
	        this.expiry = source["expiry"];
	        this.active = source["active"];
	    }
	}
	export class DashboardStats {
	    active_sessions: number;
	    total_actions: number;
	    actions_by_state: Record<string, number>;
	    total_people: number;
	    total_lists: number;
	    sessions: SessionSummary[];
	    recent_actions: ActionInfo[];
	    db_path: string;
	
	    static createFrom(source: any = {}) {
	        return new DashboardStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active_sessions = source["active_sessions"];
	        this.total_actions = source["total_actions"];
	        this.actions_by_state = source["actions_by_state"];
	        this.total_people = source["total_people"];
	        this.total_lists = source["total_lists"];
	        this.sessions = this.convertValues(source["sessions"], SessionSummary);
	        this.recent_actions = this.convertValues(source["recent_actions"], ActionInfo);
	        this.db_path = source["db_path"];
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
	export class LogEntry {
	    time: string;
	    source: string;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.source = source["source"];
	        this.level = source["level"];
	        this.message = source["message"];
	    }
	}
	export class PersonDetailInfo {
	    id: string;
	    username: string;
	    platform: string;
	    full_name: string;
	    image_url: string;
	    profile_url: string;
	    follower_count: string;
	    following_count: number;
	    content_count: number;
	    is_verified: boolean;
	    job_title: string;
	    category: string;
	    introduction: string;
	    website: string;
	    contact_details: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new PersonDetailInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.platform = source["platform"];
	        this.full_name = source["full_name"];
	        this.image_url = source["image_url"];
	        this.profile_url = source["profile_url"];
	        this.follower_count = source["follower_count"];
	        this.following_count = source["following_count"];
	        this.content_count = source["content_count"];
	        this.is_verified = source["is_verified"];
	        this.job_title = source["job_title"];
	        this.category = source["category"];
	        this.introduction = source["introduction"];
	        this.website = source["website"];
	        this.contact_details = source["contact_details"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class PersonInfo {
	    id: string;
	    username: string;
	    platform: string;
	    full_name: string;
	    image_url: string;
	    profile_url: string;
	    follower_count: string;
	    following_count: number;
	    is_verified: boolean;
	    job_title: string;
	    category: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new PersonInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.platform = source["platform"];
	        this.full_name = source["full_name"];
	        this.image_url = source["image_url"];
	        this.profile_url = source["profile_url"];
	        this.follower_count = source["follower_count"];
	        this.following_count = source["following_count"];
	        this.is_verified = source["is_verified"];
	        this.job_title = source["job_title"];
	        this.category = source["category"];
	        this.created_at = source["created_at"];
	    }
	}
	export class PersonInteraction {
	    action_id: string;
	    action_title: string;
	    action_type: string;
	    platform: string;
	    link: string;
	    status: string;
	    comment_text: string;
	    source_type: string;
	    last_interacted_at: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new PersonInteraction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.action_id = source["action_id"];
	        this.action_title = source["action_title"];
	        this.action_type = source["action_type"];
	        this.platform = source["platform"];
	        this.link = source["link"];
	        this.status = source["status"];
	        this.comment_text = source["comment_text"];
	        this.source_type = source["source_type"];
	        this.last_interacted_at = source["last_interacted_at"];
	        this.created_at = source["created_at"];
	    }
	}
	export class SessionInfo {
	    id: number;
	    username: string;
	    platform: string;
	    expiry: string;
	    added_at: string;
	    active: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.username = source["username"];
	        this.platform = source["platform"];
	        this.expiry = source["expiry"];
	        this.added_at = source["added_at"];
	        this.active = source["active"];
	    }
	}
	
	export class SocialListInfo {
	    id: string;
	    name: string;
	    list_type: string;
	    item_count: number;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new SocialListInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.list_type = source["list_type"];
	        this.item_count = source["item_count"];
	        this.created_at = source["created_at"];
	    }
	}
	export class TagInfo {
	    id: string;
	    name: string;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new TagInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	    }
	}
	export class TargetInfo {
	    id: string;
	    action_id: string;
	    platform: string;
	    link: string;
	    status: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new TargetInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.action_id = source["action_id"];
	        this.platform = source["platform"];
	        this.link = source["link"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	    }
	}
	export class TemplateInfo {
	    id: number;
	    name: string;
	    subject: string;
	    body: string;
	
	    static createFrom(source: any = {}) {
	        return new TemplateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.subject = source["subject"];
	        this.body = source["body"];
	    }
	}

}

