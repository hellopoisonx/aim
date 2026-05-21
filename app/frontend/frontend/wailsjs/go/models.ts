export namespace client {
	
	export class FriendshipItem {
	    user_id: number;
	    friend_id: number;
	    status: string;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new FriendshipItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.friend_id = source["friend_id"];
	        this.status = source["status"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class AcceptFriendResponse {
	    friendship: FriendshipItem;
	
	    static createFrom(source: any = {}) {
	        return new AcceptFriendResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.friendship = this.convertValues(source["friendship"], FriendshipItem);
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
	export class AddFriendResponse {
	    friendship: FriendshipItem;
	
	    static createFrom(source: any = {}) {
	        return new AddFriendResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.friendship = this.convertValues(source["friendship"], FriendshipItem);
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
	export class CreateConversationResponse {
	    conversation_id: number;
	    conversation_type: string;
	    is_active: boolean;
	    created_at: number;
	    member_ids: number[];
	
	    static createFrom(source: any = {}) {
	        return new CreateConversationResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.conversation_type = source["conversation_type"];
	        this.is_active = source["is_active"];
	        this.created_at = source["created_at"];
	        this.member_ids = source["member_ids"];
	    }
	}
	
	export class MessageItem {
	    id: number;
	    conversation_id: number;
	    sender_id: number;
	    message_type: string;
	    content: string;
	    client_msg_id: string;
	    created_at: number;
	
	    static createFrom(source: any = {}) {
	        return new MessageItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.sender_id = source["sender_id"];
	        this.message_type = source["message_type"];
	        this.content = source["content"];
	        this.client_msg_id = source["client_msg_id"];
	        this.created_at = source["created_at"];
	    }
	}
	export class GetConversationHistoryResponse {
	    messages: MessageItem[];
	    next_cursor_created_at: number;
	    next_cursor_id: number;
	    has_more: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GetConversationHistoryResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.messages = this.convertValues(source["messages"], MessageItem);
	        this.next_cursor_created_at = source["next_cursor_created_at"];
	        this.next_cursor_id = source["next_cursor_id"];
	        this.has_more = source["has_more"];
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
	export class UserInfo {
	    id: number;
	    email: string;
	    status: number;
	    nickname: string;
	    avatar: string;
	    created_at: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new UserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.status = source["status"];
	        this.nickname = source["nickname"];
	        this.avatar = source["avatar"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class GetUserByIdResponse {
	    user: UserInfo;
	
	    static createFrom(source: any = {}) {
	        return new GetUserByIdResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user = this.convertValues(source["user"], UserInfo);
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
	export class ListFriendApplicationsResponse {
	    applications: FriendshipItem[];
	
	    static createFrom(source: any = {}) {
	        return new ListFriendApplicationsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.applications = this.convertValues(source["applications"], FriendshipItem);
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
	export class ListFriendsResponse {
	    friends: FriendshipItem[];
	
	    static createFrom(source: any = {}) {
	        return new ListFriendsResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.friends = this.convertValues(source["friends"], FriendshipItem);
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
	export class LoginRequest {
	    email: string;
	    password: string;
	    device_id: string;
	
	    static createFrom(source: any = {}) {
	        return new LoginRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.password = source["password"];
	        this.device_id = source["device_id"];
	    }
	}
	export class LoginResponse {
	    user_id: number;
	    access_token: string;
	    refresh_token: string;
	    expires_at: number;
	
	    static createFrom(source: any = {}) {
	        return new LoginResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	        this.access_token = source["access_token"];
	        this.refresh_token = source["refresh_token"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class LogoutResponse {
	    success: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LogoutResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	    }
	}
	
	export class RefreshRequest {
	    refresh_token: string;
	
	    static createFrom(source: any = {}) {
	        return new RefreshRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.refresh_token = source["refresh_token"];
	    }
	}
	export class RefreshResponse {
	    access_token: string;
	    refresh_token: string;
	    expires_at: number;
	
	    static createFrom(source: any = {}) {
	        return new RefreshResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.access_token = source["access_token"];
	        this.refresh_token = source["refresh_token"];
	        this.expires_at = source["expires_at"];
	    }
	}
	export class RegisterRequest {
	    email: string;
	    password: string;
	    username?: string;
	    avatar?: string;
	    device_id: string;
	
	    static createFrom(source: any = {}) {
	        return new RegisterRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.email = source["email"];
	        this.password = source["password"];
	        this.username = source["username"];
	        this.avatar = source["avatar"];
	        this.device_id = source["device_id"];
	    }
	}
	export class RegisterResponse {
	    user_id: number;
	
	    static createFrom(source: any = {}) {
	        return new RegisterResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.user_id = source["user_id"];
	    }
	}
	export class RejectFriendResponse {
	    friendship: FriendshipItem;
	
	    static createFrom(source: any = {}) {
	        return new RejectFriendResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.friendship = this.convertValues(source["friendship"], FriendshipItem);
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
	
	export class UserListItem {
	    id: number;
	    email: string;
	    avatar: string;
	
	    static createFrom(source: any = {}) {
	        return new UserListItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.email = source["email"];
	        this.avatar = source["avatar"];
	    }
	}

}

export namespace main {
	
	export class AppConfig {
	    gateway_http: string;
	    gateway_ws: string;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway_http = source["gateway_http"];
	        this.gateway_ws = source["gateway_ws"];
	    }
	}
	export class ProtocolFrame {
	    name: string;
	    value: number;
	    direction: string;
	    payload: string;
	
	    static createFrom(source: any = {}) {
	        return new ProtocolFrame(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.direction = source["direction"];
	        this.payload = source["payload"];
	    }
	}
	export class ProtocolCatalog {
	    rest: string[];
	    frames: ProtocolFrame[];
	
	    static createFrom(source: any = {}) {
	        return new ProtocolCatalog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rest = source["rest"];
	        this.frames = this.convertValues(source["frames"], ProtocolFrame);
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
	
	export class SendMessageRequest {
	    conversation_id: number;
	    message_type: string;
	    content: string;
	    client_msg_id: string;
	    mentions: string[];
	
	    static createFrom(source: any = {}) {
	        return new SendMessageRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.message_type = source["message_type"];
	        this.content = source["content"];
	        this.client_msg_id = source["client_msg_id"];
	        this.mentions = source["mentions"];
	    }
	}
	export class SessionState {
	    gateway_http: string;
	    gateway_ws: string;
	    device_id: string;
	    user_id: number;
	    access_token: boolean;
	    refresh_token: boolean;
	    expires_at: number;
	    ws_connected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SessionState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gateway_http = source["gateway_http"];
	        this.gateway_ws = source["gateway_ws"];
	        this.device_id = source["device_id"];
	        this.user_id = source["user_id"];
	        this.access_token = source["access_token"];
	        this.refresh_token = source["refresh_token"];
	        this.expires_at = source["expires_at"];
	        this.ws_connected = source["ws_connected"];
	    }
	}

}

