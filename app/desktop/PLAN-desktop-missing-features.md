# Desktop Missing Features Plan

## Context

Desktop client currently needs support for these missing IM features:

1. Accept/reject friend requests
2. Group member management: remove member, grant admin, revoke admin, transfer owner
3. Per-conversation-member read receipts
4. Per-conversation-member typing status
5. User online/offline status display
6. Click user to show user details

Initial repo scan: this is a Wails v2 + Vue 3 + Arco Design desktop app. Backend-facing logic is split between Go bindings (`app.go`, `views.go`, `internal/api`, `internal/ws`, `internal/store`) and a compact Vue UI in `frontend/src/App.vue`.

## Approach

Implement the feature set across the desktop client and the gateway/logic backend where group role APIs are missing.

Current findings:

- Friend request accept/reject is already exposed in Go/Wails (`ListFriendApplications`, `AcceptFriend`, `RejectFriend`) but the Vue friends tab only shows the accepted/pending friends list and never renders application actions.
- Group remove member is already wired end-to-end, but current backend service only allows the owner. Requirements: owner can grant/revoke admin and transfer owner; admins can add members, edit group profile, and remove ordinary members; old owner becomes admin after transfer.
- Backend already stores member roles (`owner`, `admin`, `member`) in `conversation_members.role`, exposes roles in member detail, and has `CodeNotAdmin`/`ErrNotAdmin`, but no role update/owner transfer SQL/service/rpc/gateway methods yet.
- Per-member read receipt data partly exists: message DTOs include `MessageReadDetailView`, history response includes `ReadStates`, WS emits `ws:read-receipt`, and `SendReadReceipt` exists. Frontend currently ignores all read state/read receipt events and never sends read receipt when viewing messages.
- Per-member typing data partly exists: `SendTyping` exists and WS emits `ws:typing`. Frontend sends typing, but does not listen/render who is typing.
- Presence data partly exists: REST `GetFriendsPresence`, SQLite `presence`, and WS `ws:presence` exist. Frontend does not load/listen/render presence.
- User detail is partly supported through search/list/member/friend DTO fields. There is an internal `resolveUserInfo` helper, but no explicit exported `GetUserByID` Wails method; clicking users currently has no detail drawer/modal.

Recommended approach:

1. Add backend group role-management APIs in the same style as existing conversation APIs: SQLC query → `ConversationService` method → logic RPC method/proto → gateway handler/logic/route → desktop API client/Wails binding.
2. Adjust existing group service permission checks so owner or admin can add members/update group info/remove ordinary members, while only owner can grant/revoke admin, transfer owner, dismiss group, and remove admins if desired.
3. Keep frontend-only features concentrated in `frontend/src/App.vue` where possible, using existing bindings and Wails events.
4. Add minimal exported desktop Go bindings for user detail and new group role-management operations.
5. Regenerate generated code/bindings during implementation after changing proto/goctl/Wails definitions.
6. Use local reactive maps for live UI state (presence, typing timers, read receipts) and preserve existing SQLite cache behavior for conversations/messages/friends/members.

## Files to modify

Backend:

- `../../app/logic/rpc/model/queries/conversation.sql` — add SQLC queries for updating member role and conversation owner.
- `../../app/logic/rpc/model/*.sql.go`, `../../app/logic/rpc/model/querier.go` — regenerated SQLC output.
- `../../app/logic/rpc/internal/service/conversation_service.go` — permission helpers and new `GrantGroupAdmin`, `RevokeGroupAdmin`, `TransferGroupOwner` service methods; update add/remove/update permissions.
- `../../app/logic/rpc/internal/logic/conversationservice/*` — new RPC logic files for grant/revoke/transfer.
- `../../app/logic/rpc/internal/server/conversationservice/conversation_service_server.go` — wire new RPC methods.
- `../../app/logic/rpc/pb/logic.proto` or service definition source, plus generated `pb/*.go` and `conversationservice/*.go` — add RPC messages/methods.
- `../../app/gateway/api/internal/types/types.go` — request/response DTOs for role operations.
- `../../app/gateway/api/internal/handler/routes.go` — add authenticated routes.
- `../../app/gateway/api/internal/handler/conversations/*` and `../../app/gateway/api/internal/logic/conversations/*` — handlers/logic for role operations.

Desktop:

- `frontend/src/App.vue` — main UI changes: friend applications, member actions, read/typing/presence displays, user detail drawer/modal, event handlers.
- `app.go` — exported Wails methods for user detail and group role-management; optionally persist read receipt updates to cache if adding store helper.
- `views.go` — user/detail view additions if needed.
- `internal/api/client.go` — REST methods for group admin/owner operations.
- `internal/api/types.go` — request/response DTOs for role-management endpoints if needed.
- `internal/store/sqlite.go` — optional helpers for cached read states/presence/member updates if UI needs cached reads beyond current schema.
- `frontend/wailsjs/go/main/App.d.ts`, `frontend/wailsjs/go/main/App.js`, `frontend/wailsjs/go/models.ts` — regenerated, not hand edited.

## Reuse

- Friend request bindings: `App.ListFriendApplications`, `App.AcceptFriend`, `App.RejectFriend` in `app.go`; generated declarations already exist in `frontend/wailsjs/go/main/App.d.ts`.
- Existing friend REST client methods: `ListFriendApplications`, `AcceptFriend`, `RejectFriend` in `internal/api/client.go`.
- Existing group member loading/removal: `GetConversationMembers`, `AddGroupMembers`, `RemoveGroupMember`, `LeaveGroup`, `DismissGroup`, `UpdateGroupInfo` in `app.go` and `internal/api/client.go`.
- Existing read receipt plumbing: `SendReadReceipt` in `app.go`, `ReadReceipt` in `internal/ws/client.go`, `ReadReceiptView` and `MessageReadDetailView` in `views.go`, `ws:read-receipt` emission in `app.go`.
- Existing typing plumbing: `SendTyping` in `app.go`, `Typing` in `internal/ws/client.go`, `TypingView` in `views.go`, `ws:typing` emission in `app.go`.
- Existing presence plumbing: `GetFriendsPresence` in `app.go`/`internal/api/client.go`, `UpsertPresence` cache in `internal/store/sqlite.go`, `ws:presence` emission in `app.go`.
- Existing user lookup internals: `api.GetUserByID` in `internal/api/client.go`, `resolveUserInfo` and `userViewFromAPI` in `app.go`/`views.go`.
- Existing UI patterns: Arco tabs/lists/drawer/modal and member drawer patterns in `frontend/src/App.vue`.

## Steps

- [ ] Add SQLC queries for `UpdateConversationMemberRole` and `UpdateConversationCreator`, then regenerate SQLC output.
- [ ] Add backend service permission helpers: `isOwner`, `isAdminOrOwner`, target role lookup; allow admins to add members, edit group profile, and remove only ordinary members; keep owner-only for grant/revoke/transfer/dismiss.
- [ ] Implement backend role methods:
  - grant admin: owner-only, target must be group member and currently ordinary member.
  - revoke admin: owner-only, target must be admin; becomes ordinary member.
  - transfer owner: owner-only, target must be member/admin; old owner role becomes `admin`, target role becomes `owner`, conversation `creator_id` changes to target.
- [ ] Expose role methods through logic RPC/proto/server and gateway REST routes. Proposed REST contract:
  - `POST /api/conversations/:id/members/:uid/admin` — grant admin.
  - `DELETE /api/conversations/:id/members/:uid/admin` — revoke admin.
  - `POST /api/conversations/:id/owner` with `{ "user_id": "<uid>" }` or numeric `user_id` — transfer owner.
- [ ] Add/update backend tests for owner/admin permission matrix and transfer-owner role changes.
- [ ] Add desktop REST client methods and Wails methods: `GrantGroupAdmin`, `RevokeGroupAdmin`, `TransferGroupOwner`, plus `GetUserByID` returning `UserView`.
- [ ] Add a friend applications section in the friends tab, loaded by `refreshFriends`, with accept/reject buttons and `ws:friend-application` updates.
- [ ] Implement a reusable user detail drawer/modal opened from friend, search result, member, sender, read receipt, typing, and presence UI.
- [ ] Extend member drawer actions based on current user's role: owner can transfer owner/grant/revoke/remove; admin can add/edit/remove ordinary members; hide invalid self/owner/admin actions.
- [ ] Track presence in frontend with a `presenceByUserId` map, load via `GetFriendsPresence`, update from `ws:presence`, and render online/offline badges in friends, members, user detail, conversation header/direct chats, and sender/user chips where practical.
- [ ] Track typing per conversation member with a TTL map updated by `ws:typing`; render “X 正在输入…” in the active conversation composer/header, excluding current user.
- [ ] Track read states per conversation user from `HistoryResponse.ReadStates`, message `read_details`, and `ws:read-receipt`; render per-message “已读 N / 未读 N” summary with click-to-expand read/unread member details.
- [ ] Send `SendReadReceipt` when selecting a conversation and after loading/receiving messages, using the latest non-empty message id in that conversation.
- [ ] Regenerate generated code and bindings after API changes: sqlc/goctl/protoc as applicable, then Wails bindings.

## Verification

- Backend:
  - Regenerate SQLC/goctl/proto outputs as required by project tooling.
  - Run backend unit tests for logic/gateway packages touched by group role APIs.
  - Add/verify tests for admin permissions: owner-only grant/revoke/transfer, admin add/edit/remove ordinary member, admin cannot remove owner/admin, old owner becomes admin after transfer.
- Desktop:
  - Run `go test ./...` from desktop root.
  - Run `wails generate module` after exported Go method changes.
  - Run frontend type/build check (`cd frontend && pnpm build`).
- Manual two/three-account checks:
  - Account A sends friend request to B; B sees application, accepts/rejects; both lists update.
  - In a group, owner grants/revokes admin, admin adds/edits/removes ordinary members, owner transfers ownership; member drawer reflects role changes and permissions.
  - In a group/direct chat, each message shows “已读 N / 未读 N”; clicking expands per-member read/unread details.
  - Typing indicator appears for the correct user and conversation and expires automatically.
  - Online/offline status updates after login/logout/WS presence push.
  - Clicking users from search/friends/members/messages/read-details opens detail with profile and presence.
