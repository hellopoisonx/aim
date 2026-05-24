export namespace main {
	
	export class ConfigDTO {
	    gateway_url: string;
	    ws_url: string;
	
	    static createFrom(source: any = {}) {
	        return new ConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway_url = source["gateway_url"];
	        this.ws_url = source["ws_url"];
	    }
	}
	export class ConversationView {
	    conversation_id: string;
	    conversation_type: string;
	    is_active: boolean;
	    created_at: number;
	    member_ids: string[];
	    name: string;
	    avatar: string;
	    creator_id: string;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new ConversationView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.conversation_type = source["conversation_type"];
	        this.is_active = source["is_active"];
	        this.created_at = source["created_at"];
	        this.member_ids = source["member_ids"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	        this.creator_id = source["creator_id"];
	        this.display_name = source["display_name"];
	    }
	}
	export class CreateConversationRequest {
	    conversation_type: string;
	    member_ids: string[];
	    name: string;
	    avatar?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateConversationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_type = source["conversation_type"];
	        this.member_ids = source["member_ids"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	    }
	}
	export class CreateGroupRequest {
	    member_ids: string[];
	    name: string;
	    avatar?: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateGroupRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.member_ids = source["member_ids"];
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	    }
	}
	export class FriendView {
	    user_id: string;
	    friend_id: string;
	    status: string;
	    created_at: number;
	    updated_at: number;
	    display_name: string;
	    email: string;
	    avatar: string;
	
	    static createFrom(source: any = {}) {
	        return new FriendView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.friend_id = source["friend_id"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	        this.display_name = source["display_name"];
	        this.email = source["email"];
	        this.avatar = source["avatar"];
	    }
	}
	export class ReadStateView {
	    user_id: string;
	    last_read_message_id: string;
	    updated_at: number;
	    email: string;
	    avatar: string;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new ReadStateView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.last_read_message_id = source["last_read_message_id"];
	        this.updated_at = source["updated_at"];
	        this.email = source["email"];
	        this.avatar = source["avatar"];
	        this.display_name = source["display_name"];
	    }
	}
	export class MessageReadDetailView {
	    user_id: string;
	    is_read: boolean;
	    last_read_message_id: string;
	    updated_at: number;
	    email: string;
	    avatar: string;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new MessageReadDetailView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.is_read = source["is_read"];
	        this.last_read_message_id = source["last_read_message_id"];
	        this.updated_at = source["updated_at"];
	        this.email = source["email"];
	        this.avatar = source["avatar"];
	        this.display_name = source["display_name"];
	    }
	}
	export class SenderInfoView {
	    name: string;
	    email: string;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new SenderInfoView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.email = source["email"];
	        this.display_name = source["display_name"];
	    }
	}
	export class MessageView {
	    message_id: string;
	    conversation_id: string;
	    sender_id: string;
	    sender_info: SenderInfoView;
	    message_type: string;
	    content: string;
	    client_msg_id: string;
	    created_at: number;
	    is_system?: boolean;
	    mentions?: string[];
	    read_details: MessageReadDetailView[];
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new MessageView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message_id = source["message_id"];
	        this.conversation_id = source["conversation_id"];
	        this.sender_id = source["sender_id"];
	        this.sender_info = this.convertValues(source["sender_info"], SenderInfoView);
	        this.message_type = source["message_type"];
	        this.content = source["content"];
	        this.client_msg_id = source["client_msg_id"];
	        this.created_at = source["created_at"];
	        this.is_system = source["is_system"];
	        this.mentions = source["mentions"];
	        this.read_details = this.convertValues(source["read_details"], MessageReadDetailView);
	        this.status = source["status"];
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
	export class HistoryResponse {
	    messages: MessageView[];
	    next_cursor_created_at: number;
	    next_cursor_id: string;
	    has_more: boolean;
	    read_states: ReadStateView[];
	
	    static createFrom(source: any = {}) {
	        return new HistoryResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], MessageView);
	        this.next_cursor_created_at = source["next_cursor_created_at"];
	        this.next_cursor_id = source["next_cursor_id"];
	        this.has_more = source["has_more"];
	        this.read_states = this.convertValues(source["read_states"], ReadStateView);
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
	export class LoginInput {
	    email: string;
	    password: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.password = source["password"];
	    }
	}
	export class MemberView {
	    user_id: string;
	    email: string;
	    avatar: string;
	    role: string;
	    joined_at: number;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new MemberView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.email = source["email"];
	        this.avatar = source["avatar"];
	        this.role = source["role"];
	        this.joined_at = source["joined_at"];
	        this.display_name = source["display_name"];
	    }
	}
	
	
	export class PresenceView {
	    user_id: string;
	    status: string;
	    updated_at: number;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new PresenceView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.status = source["status"];
	        this.updated_at = source["updated_at"];
	        this.display_name = source["display_name"];
	    }
	}
	
	export class RegisterInput {
	    email: string;
	    password: string;
	    username: string;
	    avatar: string;
	
	    static createFrom(source: any = {}) {
	        return new RegisterInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.password = source["password"];
	        this.username = source["username"];
	        this.avatar = source["avatar"];
	    }
	}
	export class RegisterResponse {
	    user_id: string;
	
	    static createFrom(source: any = {}) {
	        return new RegisterResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	    }
	}
	
	export class SessionInfo {
	    user_id: string;
	    email: string;
	    nickname: string;
	    avatar: string;
	    access_token: string;
	    refresh_token: string;
	    expires_at: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.email = source["email"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.access_token = source["access_token"];
	        this.refresh_token = source["refresh_token"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class UpdateGroupInfoRequest {
	    name?: string;
	    avatar?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateGroupInfoRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.avatar = source["avatar"];
	    }
	}
	export class UserView {
	    id: string;
	    nickname: string;
	    email: string;
	    avatar: string;
	    display_name: string;
	
	    static createFrom(source: any = {}) {
	        return new UserView(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.nickname = source["nickname"];
	        this.email = source["email"];
	        this.avatar = source["avatar"];
	        this.display_name = source["display_name"];
	    }
	}

}

