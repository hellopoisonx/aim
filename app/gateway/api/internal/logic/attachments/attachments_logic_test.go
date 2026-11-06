package attachments

import (
	"context"
	"errors"
	"testing"

	attachmentpb "github.com/hellopoisonx/aim/app/attachment/rpc/pb"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeAttachmentClient embeds the generated client interface so only the RPCs
// these handlers actually call need an implementation.
type fakeAttachmentClient struct {
	attachmentpb.AttachmentServiceClient

	initReq     *attachmentpb.InitUploadReq
	completeReq *attachmentpb.CompleteUploadReq
	getFileReq  *attachmentpb.GetFileReq
	authReq     *attachmentpb.AuthorizeDownloadReq

	initResp     *attachmentpb.InitUploadResp
	completeResp *attachmentpb.FileInfo
	getFileResp  *attachmentpb.FileInfo
	authResp     *attachmentpb.AuthorizeDownloadResp

	initErr     error
	completeErr error
	getFileErr  error
	authErr     error
}

func (f *fakeAttachmentClient) InitUpload(_ context.Context, in *attachmentpb.InitUploadReq, _ ...grpc.CallOption) (*attachmentpb.InitUploadResp, error) {
	f.initReq = in

	return f.initResp, f.initErr
}

func (f *fakeAttachmentClient) CompleteUpload(_ context.Context, in *attachmentpb.CompleteUploadReq, _ ...grpc.CallOption) (*attachmentpb.FileInfo, error) {
	f.completeReq = in

	return f.completeResp, f.completeErr
}

func (f *fakeAttachmentClient) GetFile(_ context.Context, in *attachmentpb.GetFileReq, _ ...grpc.CallOption) (*attachmentpb.FileInfo, error) {
	f.getFileReq = in

	return f.getFileResp, f.getFileErr
}

func (f *fakeAttachmentClient) AuthorizeDownload(_ context.Context, in *attachmentpb.AuthorizeDownloadReq, _ ...grpc.CallOption) (*attachmentpb.AuthorizeDownloadResp, error) {
	f.authReq = in

	return f.authResp, f.authErr
}

// fakeMembershipClient answers the membership lookup DownloadAttachment makes
// before signing a URL.
type fakeMembershipClient struct {
	conversationservice.ConversationService

	got  *conversationservice.GetConversationMembersReq
	resp *conversationservice.GetConversationMembersResp
	err  error
}

func (f *fakeMembershipClient) GetConversationMembers(_ context.Context, in *conversationservice.GetConversationMembersReq, _ ...grpc.CallOption) (*conversationservice.GetConversationMembersResp, error) {
	f.got = in

	return f.resp, f.err
}

var (
	_ attachmentpb.AttachmentServiceClient    = (*fakeAttachmentClient)(nil)
	_ conversationservice.ConversationService = (*fakeMembershipClient)(nil)
)

func userContext(userID int64) context.Context {
	return authctx.WithIdentity(context.Background(), authctx.Identity{UserID: userID})
}

// membersOf builds a membership client reporting exactly these conversation members.
func membersOf(memberIDs ...int64) *fakeMembershipClient {
	return &fakeMembershipClient{resp: &conversationservice.GetConversationMembersResp{MemberIds: memberIDs}}
}

func requireCodeError(t *testing.T, err error, wantCode int, wantMsg string) {
	t.Helper()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, wantCode, codeErr.Code)
	require.Equal(t, wantMsg, codeErr.Message)
}

func TestInitAttachmentUploadRequiresUser(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{}
	_, err := NewInitAttachmentUploadLogic(context.Background(), &svc.ServiceContext{AttachmentClient: client}).
		InitAttachmentUpload(&types.InitAttachmentUploadRequest{})

	requireCodeError(t, err, errorx.CodeAuth, "unauthorized")
	require.Nil(t, client.initReq, "upstream must not be called without an authenticated caller")
}

func TestInitAttachmentUploadForwardsRequestAndMapsResponse(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{initResp: &attachmentpb.InitUploadResp{
		FileId:       "file-1",
		Bucket:       "attachments",
		ObjectKey:    "2026/11/file-1",
		UploadUrl:    "https://s3.example.com/put",
		UploadMethod: "PUT",
		Headers:      map[string]string{"x-signature": "sig"},
		ExpiresAt:    1700000000000,
	}}
	logic := NewInitAttachmentUploadLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: membersOf(42),
	})

	resp, err := logic.InitAttachmentUpload(&types.InitAttachmentUploadRequest{
		ConversationId: 7,
		Kind:           "image",
		OriginalName:   "photo.png",
		Mime:           "image/png",
		Size:           1234,
		Sha256:         "abc123",
	})
	require.NoError(t, err)

	require.Equal(t, int64(42), client.initReq.GetOwnerId(), "the caller owns the upload, not a body field")
	require.Equal(t, int64(7), client.initReq.GetConversationId())
	require.Equal(t, "image", client.initReq.GetKind())
	require.Equal(t, "photo.png", client.initReq.GetOriginalName())
	require.Equal(t, "image/png", client.initReq.GetMime())
	require.Equal(t, int64(1234), client.initReq.GetSize())
	require.Equal(t, "abc123", client.initReq.GetSha256())

	require.Equal(t, &types.InitAttachmentUploadResponse{
		FileId:       "file-1",
		Bucket:       "attachments",
		ObjectKey:    "2026/11/file-1",
		UploadUrl:    "https://s3.example.com/put",
		UploadMethod: "PUT",
		Headers:      map[string]string{"x-signature": "sig"},
		ExpiresAt:    1700000000000,
	}, resp)
}

func TestInitAttachmentUploadRequiresMembership(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{}
	members := membersOf(1, 2)
	_, err := NewInitAttachmentUploadLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: members,
	}).InitAttachmentUpload(&types.InitAttachmentUploadRequest{ConversationId: 7, Kind: "file"})

	requireCodeError(t, err, errorx.CodeForbidden, "user is not a member of this conversation")
	require.Equal(t, int64(7), members.got.GetConversationId())
	require.Nil(t, client.initReq, "non-members must not start an upload in the conversation")
}

func TestInitAttachmentUploadPropagatesUpstreamError(t *testing.T) {
	t.Parallel()

	upstream := status.Error(codes.ResourceExhausted, "quota exceeded")
	client := &fakeAttachmentClient{initErr: upstream}

	_, err := NewInitAttachmentUploadLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: membersOf(42),
	}).InitAttachmentUpload(&types.InitAttachmentUploadRequest{ConversationId: 7, Kind: "file"})

	require.Equal(t, upstream, err, "InitUpload errors reach the caller untouched")
}

func TestCompleteAttachmentUploadRequiresUser(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{}
	_, err := NewCompleteAttachmentUploadLogic(context.Background(), &svc.ServiceContext{AttachmentClient: client}).
		CompleteAttachmentUpload(&types.CompleteAttachmentUploadRequest{Id: "file-1"})

	requireCodeError(t, err, errorx.CodeAuth, "unauthorized")
	require.Nil(t, client.completeReq)
}

func TestCompleteAttachmentUploadForwardsRequestAndMapsFileInfo(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{completeResp: &attachmentpb.FileInfo{
		FileId:             "file-1",
		OwnerId:            42,
		ConversationId:     7,
		Kind:               "image",
		OriginalName:       "photo.png",
		Mime:               "image/png",
		Size:               1234,
		Sha256:             "abc123",
		Status:             "ready",
		ParseStatus:        "parsed",
		Bucket:             "attachments",
		ObjectKey:          "2026/11/file-1",
		ThumbnailObjectKey: "2026/11/file-1-thumb",
		DurationMs:         900,
		Width:              640,
		Height:             480,
		MetadataJson:       `{"width":640,"source":"camera"}`,
	}}
	logic := NewCompleteAttachmentUploadLogic(userContext(42), &svc.ServiceContext{AttachmentClient: client})

	resp, err := logic.CompleteAttachmentUpload(&types.CompleteAttachmentUploadRequest{Id: "file-1", Sha256: "abc123"})
	require.NoError(t, err)

	require.Equal(t, int64(42), client.completeReq.GetUserId())
	require.Equal(t, "file-1", client.completeReq.GetFileId())
	require.Equal(t, "abc123", client.completeReq.GetSha256())

	require.Equal(t, &types.AttachmentFileInfo{
		FileId:             "file-1",
		OwnerId:            42,
		ConversationId:     7,
		Kind:               "image",
		OriginalName:       "photo.png",
		Mime:               "image/png",
		Size:               1234,
		Sha256:             "abc123",
		Status:             "ready",
		ParseStatus:        "parsed",
		Bucket:             "attachments",
		ObjectKey:          "2026/11/file-1",
		ThumbnailObjectKey: "2026/11/file-1-thumb",
		DurationMs:         900,
		Width:              640,
		Height:             480,
		Metadata:           map[string]any{"width": float64(640), "source": "camera"},
	}, resp)
}

func TestCompleteAttachmentUploadMetadataJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata string
		want     map[string]any
	}{
		{name: "empty json becomes an empty map", metadata: "", want: map[string]any{}},
		{name: "whitespace only becomes an empty map", metadata: "  \n", want: map[string]any{}},
		{name: "malformed json is dropped", metadata: "{not json", want: map[string]any{}},
		{name: "json array is not an object", metadata: `[1,2]`, want: map[string]any{}},
		{name: "json object is decoded", metadata: `{"a":1}`, want: map[string]any{"a": float64(1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeAttachmentClient{completeResp: &attachmentpb.FileInfo{FileId: "file-1", MetadataJson: tt.metadata}}

			resp, err := NewCompleteAttachmentUploadLogic(userContext(42), &svc.ServiceContext{AttachmentClient: client}).
				CompleteAttachmentUpload(&types.CompleteAttachmentUploadRequest{Id: "file-1"})
			require.NoError(t, err)
			require.Equal(t, tt.want, resp.Metadata)
		})
	}
}

func TestCompleteAttachmentUploadPropagatesUpstreamError(t *testing.T) {
	t.Parallel()

	upstream := status.Error(codes.FailedPrecondition, "sha mismatch")
	client := &fakeAttachmentClient{completeErr: upstream}

	_, err := NewCompleteAttachmentUploadLogic(userContext(42), &svc.ServiceContext{AttachmentClient: client}).
		CompleteAttachmentUpload(&types.CompleteAttachmentUploadRequest{Id: "file-1"})

	require.Equal(t, upstream, err, "CompleteUpload errors reach the caller untouched")
}

func TestGetAttachmentMapsFileInfo(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{getFileResp: &attachmentpb.FileInfo{FileId: "file-1", OwnerId: 42, ConversationId: 7}}

	resp, err := NewGetAttachmentLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: membersOf(7, 42),
	}).GetAttachment(&types.GetAttachmentRequest{Id: "file-1"})
	require.NoError(t, err)
	require.Equal(t, int64(42), client.getFileReq.GetUserId())
	require.Equal(t, "file-1", client.getFileReq.GetFileId())
	require.Equal(t, "file-1", resp.FileId)
	require.Empty(t, resp.Metadata, "absent metadata_json still yields an empty map")
}

func TestDownloadAttachmentRequiresUser(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{}
	_, err := NewDownloadAttachmentLogic(context.Background(), &svc.ServiceContext{AttachmentClient: client}).
		DownloadAttachment(&types.DownloadAttachmentRequest{Id: "file-1"})

	requireCodeError(t, err, errorx.CodeAuth, "unauthorized")
	require.Nil(t, client.getFileReq)
	require.Nil(t, client.authReq)
}

func TestDownloadAttachmentSanitizesGetFileError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		upstream error
		wantCode int
		wantMsg  string
	}{
		{
			name:     "infrastructure error is hidden",
			upstream: status.Error(codes.Unavailable, "postgres password=leaked"),
			wantCode: errorx.CodeInternal,
			wantMsg:  "internal error",
		},
		{
			name:     "client-visible grpc error keeps its message",
			upstream: status.Error(codes.NotFound, "attachment not found"),
			wantCode: errorx.CodeNotFound,
			wantMsg:  "attachment not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeAttachmentClient{getFileErr: tt.upstream}
			members := &fakeMembershipClient{}
			logic := NewDownloadAttachmentLogic(userContext(42), &svc.ServiceContext{
				AttachmentClient:        client,
				LogicConversationClient: members,
			})

			_, err := logic.DownloadAttachment(&types.DownloadAttachmentRequest{Id: "file-1"})

			requireCodeError(t, err, tt.wantCode, tt.wantMsg)
			require.Nil(t, members.got, "no membership lookup when the file cannot be read")
			require.Nil(t, client.authReq, "no signed url when the file cannot be read")
		})
	}
}

func TestDownloadAttachmentRequiresMembership(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{getFileResp: &attachmentpb.FileInfo{FileId: "file-1", ConversationId: 7}}
	members := &fakeMembershipClient{resp: &conversationservice.GetConversationMembersResp{MemberIds: []int64{1, 2}}}
	logic := NewDownloadAttachmentLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: members,
	})

	_, err := logic.DownloadAttachment(&types.DownloadAttachmentRequest{Id: "file-1"})

	requireCodeError(t, err, errorx.CodeForbidden, "user is not a member of this conversation")
	require.Equal(t, int64(7), members.got.GetConversationId(), "membership is checked against the file's conversation")
	require.Nil(t, client.authReq, "non-members must not receive a signed url")
}

func TestDownloadAttachmentReturnsSignedURL(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{
		getFileResp: &attachmentpb.FileInfo{FileId: "file-1", ConversationId: 7},
		authResp: &attachmentpb.AuthorizeDownloadResp{
			Url:       "https://s3.example.com/get",
			Headers:   map[string]string{"x-signature": "sig"},
			ExpiresAt: 1700000000000,
		},
	}
	members := &fakeMembershipClient{resp: &conversationservice.GetConversationMembersResp{MemberIds: []int64{7, 42}}}
	logic := NewDownloadAttachmentLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: members,
	})

	resp, err := logic.DownloadAttachment(&types.DownloadAttachmentRequest{Id: "file-1"})
	require.NoError(t, err)

	require.Equal(t, int64(42), client.getFileReq.GetUserId())
	require.Equal(t, int64(42), client.authReq.GetUserId())
	require.Equal(t, "file-1", client.authReq.GetFileId())
	require.Equal(t, &types.DownloadAttachmentResponse{
		Url:       "https://s3.example.com/get",
		Headers:   map[string]string{"x-signature": "sig"},
		ExpiresAt: 1700000000000,
	}, resp)
}

func TestDownloadAttachmentPropagatesAuthorizeDownloadError(t *testing.T) {
	t.Parallel()

	upstream := errors.New("signer unavailable")
	client := &fakeAttachmentClient{
		getFileResp: &attachmentpb.FileInfo{FileId: "file-1", ConversationId: 7},
		authErr:     upstream,
	}
	members := &fakeMembershipClient{resp: &conversationservice.GetConversationMembersResp{MemberIds: []int64{42}}}
	logic := NewDownloadAttachmentLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: members,
	})

	_, err := logic.DownloadAttachment(&types.DownloadAttachmentRequest{Id: "file-1"})

	require.Equal(t, upstream, err, "AuthorizeDownload errors reach the caller untouched")
}

func TestGetAttachmentRequiresMembership(t *testing.T) {
	t.Parallel()

	client := &fakeAttachmentClient{getFileResp: &attachmentpb.FileInfo{FileId: "file-1", OwnerId: 99, ConversationId: 7}}
	members := membersOf(1, 2)
	_, err := NewGetAttachmentLogic(userContext(42), &svc.ServiceContext{
		AttachmentClient:        client,
		LogicConversationClient: members,
	}).GetAttachment(&types.GetAttachmentRequest{Id: "file-1"})

	requireCodeError(t, err, errorx.CodeForbidden, "user is not a member of this conversation")
	require.Equal(t, int64(7), members.got.GetConversationId())
}
