package conversations

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/ws"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockGetHistoryConversationService implements conversationservice.ConversationService for testing get history.
type mockGetHistoryConversationService struct {
	CreateConversationFunc     func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error)
	GetConversationHistoryFunc func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error)
	GetConversationMembersFunc func(ctx context.Context, in *conversationservice.GetConversationMembersReq) (*conversationservice.GetConversationMembersResp, error)
	GetUserConversationsFunc   func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error)
}

func (m *mockGetHistoryConversationService) CreateConversation(ctx context.Context, in *conversationservice.CreateConversationReq, opts ...grpc.CallOption) (*conversationservice.CreateConversationResp, error) {
	if m.CreateConversationFunc != nil {
		return m.CreateConversationFunc(ctx, in)
	}
	return nil, errors.New("CreateConversation not implemented")
}

func (m *mockGetHistoryConversationService) GetConversationHistory(ctx context.Context, in *conversationservice.GetConversationHistoryReq, opts ...grpc.CallOption) (*conversationservice.GetConversationHistoryResp, error) {
	if m.GetConversationHistoryFunc != nil {
		return m.GetConversationHistoryFunc(ctx, in)
	}
	return nil, errors.New("GetConversationHistory not implemented")
}

func (m *mockGetHistoryConversationService) GetConversationMembers(ctx context.Context, in *conversationservice.GetConversationMembersReq, opts ...grpc.CallOption) (*conversationservice.GetConversationMembersResp, error) {
	if m.GetConversationMembersFunc != nil {
		return m.GetConversationMembersFunc(ctx, in)
	}
	return nil, errors.New("GetConversationMembers not implemented")
}

func (m *mockGetHistoryConversationService) GetUserConversations(ctx context.Context, in *conversationservice.GetUserConversationsReq, opts ...grpc.CallOption) (*conversationservice.GetUserConversationsResp, error) {
	if m.GetUserConversationsFunc != nil {
		return m.GetUserConversationsFunc(ctx, in)
	}
	return nil, errors.New("GetUserConversations not implemented")
}

func (m *mockGetHistoryConversationService) GrantGroupAdmin(ctx context.Context, in *conversationservice.GrantGroupAdminReq, opts ...grpc.CallOption) (*conversationservice.GrantGroupAdminResp, error) {
	return nil, errors.New("GrantGroupAdmin not implemented")
}

func (m *mockGetHistoryConversationService) RevokeGroupAdmin(ctx context.Context, in *conversationservice.RevokeGroupAdminReq, opts ...grpc.CallOption) (*conversationservice.RevokeGroupAdminResp, error) {
	return nil, errors.New("RevokeGroupAdmin not implemented")
}

func (m *mockGetHistoryConversationService) TransferGroupOwner(ctx context.Context, in *conversationservice.TransferGroupOwnerReq, opts ...grpc.CallOption) (*conversationservice.TransferGroupOwnerResp, error) {
	return nil, errors.New("TransferGroupOwner not implemented")
}

func TestGetConversationHistory(t *testing.T) {
	is := assert.New(t)

	tests := []struct {
		name      string
		req       *types.GetConversationHistoryRequest
		mockSetup func(*mockGetHistoryConversationService)
		wantResp  *types.GetConversationHistoryResponse
		wantErr   *errorx.CodeError
	}{
		{
			name: "unauthorized - no identity in context",
			req: &types.GetConversationHistoryRequest{
				Id: 123,
			},
			mockSetup: func(*mockGetHistoryConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeAuth, "unauthorized"),
		},
		{
			name: "invalid id - zero",
			req: &types.GetConversationHistoryRequest{
				Id: 0,
			},
			mockSetup: func(*mockGetHistoryConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive"),
		},
		{
			name: "invalid id - negative",
			req: &types.GetConversationHistoryRequest{
				Id: -5,
			},
			mockSetup: func(*mockGetHistoryConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeBadInput, "id is required and must be positive"),
		},
		{
			name: "nil client",
			req: &types.GetConversationHistoryRequest{
				Id: 123,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				*ms = mockGetHistoryConversationService{}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "rpc error - gRPC application error",
			req: &types.GetConversationHistoryRequest{
				Id: 123,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					st := status.New(codes.NotFound, "conversation not found")
					return nil, st.Err()
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeNotFound, "conversation not found"),
		},
		{
			name: "rpc error - gRPC infrastructure error",
			req: &types.GetConversationHistoryRequest{
				Id: 123,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					st := status.New(codes.Unavailable, "service unavailable")
					return nil, st.Err()
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "rpc error - non-gRPC error",
			req: &types.GetConversationHistoryRequest{
				Id: 123,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					return nil, errors.New("some unexpected error")
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "success - empty messages",
			req: &types.GetConversationHistoryRequest{
				Id: 123,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					return &conversationservice.GetConversationHistoryResp{
						Messages:            []*pb.MessageItem{},
						NextCursorCreatedAt: 0,
						NextCursorId:        0,
						HasMore:             false,
					}, nil
				}
			},
			wantResp: &types.GetConversationHistoryResponse{
				Messages:            []types.MessageItem{},
				NextCursorCreatedAt: 0,
				NextCursorId:        0,
				HasMore:             false,
			},
			wantErr: nil,
		},
		{
			name: "success - with messages",
			req: &types.GetConversationHistoryRequest{
				Id: 123,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					return &conversationservice.GetConversationHistoryResp{
						Messages: []*pb.MessageItem{
							{
								Id:             1,
								ConversationId: 123,
								SenderId:       100,
								SenderInfo:     &pb.SenderInfo{Name: "Alice", Email: "alice@example.com"},
								MessageType:    "text",
								Content:        "hello",
								ClientMsgId:    "client-msg-1",
								CreatedAt:      1700000000,
								Mentions:       []string{"200"},
							},
							{
								Id:             2,
								ConversationId: 123,
								SenderId:       200,
								SenderInfo:     &pb.SenderInfo{Name: "Bob", Email: "bob@example.com"},
								MessageType:    "text",
								Content:        "hi there",
								ClientMsgId:    "client-msg-2",
								CreatedAt:      1700000100,
							},
						},
						NextCursorCreatedAt: 1700000100,
						NextCursorId:        2,
						HasMore:             true,
					}, nil
				}
			},
			wantResp: &types.GetConversationHistoryResponse{
				Messages: []types.MessageItem{
					{
						Id:             1,
						ConversationId: 123,
						SenderId:       100,
						SenderInfo:     types.SenderInfo{Name: "Alice", Email: "alice@example.com"},
						MessageType:    "text",
						Content:        "hello",
						ClientMsgId:    "client-msg-1",
						CreatedAt:      1700000000,
						Mentions:       []string{"200"},
						ReadDetails:    []types.MessageReadDetailItem{},
					},
					{
						Id:             2,
						ConversationId: 123,
						SenderId:       200,
						SenderInfo:     types.SenderInfo{Name: "Bob", Email: "bob@example.com"},
						MessageType:    "text",
						Content:        "hi there",
						ClientMsgId:    "client-msg-2",
						CreatedAt:      1700000100,
						ReadDetails:    []types.MessageReadDetailItem{},
					},
				},
				NextCursorCreatedAt: 1700000100,
				NextCursorId:        2,
				HasMore:             true,
			},
			wantErr: nil,
		},
		{
			name: "success - limit normalized to default 50",
			req: &types.GetConversationHistoryRequest{
				Id:    123,
				Limit: 0,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					is.Equal(int32(50), in.Limit)
					return &conversationservice.GetConversationHistoryResp{
						Messages:            nil,
						NextCursorCreatedAt: 0,
						NextCursorId:        0,
						HasMore:             false,
					}, nil
				}
			},
			wantResp: &types.GetConversationHistoryResponse{
				Messages:            []types.MessageItem{},
				NextCursorCreatedAt: 0,
				NextCursorId:        0,
				HasMore:             false,
			},
			wantErr: nil,
		},
		{
			name: "success - limit normalized from negative to 50",
			req: &types.GetConversationHistoryRequest{
				Id:    123,
				Limit: -10,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					is.Equal(int32(50), in.Limit)
					return &conversationservice.GetConversationHistoryResp{
						Messages:            nil,
						NextCursorCreatedAt: 0,
						NextCursorId:        0,
						HasMore:             false,
					}, nil
				}
			},
			wantResp: &types.GetConversationHistoryResponse{
				Messages:            []types.MessageItem{},
				NextCursorCreatedAt: 0,
				NextCursorId:        0,
				HasMore:             false,
			},
			wantErr: nil,
		},
		{
			name: "success - limit normalized from >100 to 100",
			req: &types.GetConversationHistoryRequest{
				Id:    123,
				Limit: 200,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					is.Equal(int32(100), in.Limit)
					return &conversationservice.GetConversationHistoryResp{
						Messages:            nil,
						NextCursorCreatedAt: 0,
						NextCursorId:        0,
						HasMore:             false,
					}, nil
				}
			},
			wantResp: &types.GetConversationHistoryResponse{
				Messages:            []types.MessageItem{},
				NextCursorCreatedAt: 0,
				NextCursorId:        0,
				HasMore:             false,
			},
			wantErr: nil,
		},
		{
			name: "success - cursor parameters passed correctly",
			req: &types.GetConversationHistoryRequest{
				Id:              123,
				CursorCreatedAt: 1700000000,
				CursorId:        50,
				Limit:           20,
			},
			mockSetup: func(ms *mockGetHistoryConversationService) {
				ms.GetConversationHistoryFunc = func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error) {
					is.Equal(int64(123), in.ConversationId)
					is.Equal(int64(1700000000), in.CursorCreatedAt)
					is.Equal(int64(50), in.CursorId)
					is.Equal(int32(20), in.Limit)
					return &conversationservice.GetConversationHistoryResp{
						Messages:            []*pb.MessageItem{},
						NextCursorCreatedAt: 1700000500,
						NextCursorId:        70,
						HasMore:             false,
					}, nil
				}
			},
			wantResp: &types.GetConversationHistoryResponse{
				Messages:            []types.MessageItem{},
				NextCursorCreatedAt: 1700000500,
				NextCursorId:        70,
				HasMore:             false,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockGetHistoryConversationService{}
			tt.mockSetup(mockClient)

			svcCtx := &svc.ServiceContext{
				LogicConversationClient: mockClient,
			}

			// Use identity context for all tests except the unauthorized test
			var ctx context.Context
			if tt.wantErr != nil && tt.wantErr.Code == errorx.CodeAuth {
				ctx = context.Background() // No identity for unauthorized test
			} else {
				ctx = ws.WithIdentity(context.Background(), ws.Identity{UserID: 42, DeviceID: "test-device"})
			}
			logic := NewGetConversationHistoryLogic(ctx, svcCtx)

			resp, err := logic.GetConversationHistory(tt.req)

			if tt.wantErr != nil {
				is.Error(err)
				codeErr, ok := err.(*errorx.CodeError)
				is.True(ok, "expected *errorx.CodeError, got %T", err)
				is.Equal(tt.wantErr.Code, codeErr.Code)
				is.Equal(tt.wantErr.Message, codeErr.Message)
				is.Nil(resp)
			} else {
				is.NoError(err)
				is.Equal(tt.wantResp, resp)
			}
		})
	}
}

func (m *mockGetHistoryConversationService) AddGroupMembers(ctx context.Context, in *pb.AddGroupMembersReq, opts ...grpc.CallOption) (*pb.AddGroupMembersResp, error) {
	return nil, errors.New("AddGroupMembers not implemented")
}
func (m *mockGetHistoryConversationService) RemoveGroupMembers(ctx context.Context, in *pb.RemoveGroupMembersReq, opts ...grpc.CallOption) (*pb.RemoveGroupMembersResp, error) {
	return nil, errors.New("RemoveGroupMembers not implemented")
}
func (m *mockGetHistoryConversationService) LeaveGroup(ctx context.Context, in *pb.LeaveGroupReq, opts ...grpc.CallOption) (*pb.LeaveGroupResp, error) {
	return nil, errors.New("LeaveGroup not implemented")
}
func (m *mockGetHistoryConversationService) DismissGroup(ctx context.Context, in *pb.DismissGroupReq, opts ...grpc.CallOption) (*pb.DismissGroupResp, error) {
	return nil, errors.New("DismissGroup not implemented")
}
func (m *mockGetHistoryConversationService) UpdateGroupInfo(ctx context.Context, in *pb.UpdateGroupInfoReq, opts ...grpc.CallOption) (*pb.UpdateGroupInfoResp, error) {
	return nil, errors.New("UpdateGroupInfo not implemented")
}
func (m *mockGetHistoryConversationService) GetConversationMembersDetail(ctx context.Context, in *pb.GetConversationMembersDetailReq, opts ...grpc.CallOption) (*pb.GetConversationMembersDetailResp, error) {
	return nil, errors.New("GetConversationMembersDetail not implemented")
}
func (m *mockGetHistoryConversationService) UpdateReadReceipt(ctx context.Context, in *pb.UpdateReadReceiptReq, opts ...grpc.CallOption) (*pb.UpdateReadReceiptResp, error) {
	return nil, errors.New("UpdateReadReceipt not implemented")
}
func (m *mockGetHistoryConversationService) ListConversationReadStates(ctx context.Context, in *pb.ListConversationReadStatesReq, opts ...grpc.CallOption) (*pb.ListConversationReadStatesResp, error) {
	return nil, errors.New("ListConversationReadStates not implemented")
}
