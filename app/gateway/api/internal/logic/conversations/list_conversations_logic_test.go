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

// mockListConversationsService implements conversationservice.ConversationService for testing list conversations.
type mockListConversationsService struct {
	CreateConversationFunc     func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error)
	GetConversationHistoryFunc func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error)
	GetConversationMembersFunc func(ctx context.Context, in *conversationservice.GetConversationMembersReq) (*conversationservice.GetConversationMembersResp, error)
	GetUserConversationsFunc   func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error)
}

func (m *mockListConversationsService) CreateConversation(ctx context.Context, in *conversationservice.CreateConversationReq, opts ...grpc.CallOption) (*conversationservice.CreateConversationResp, error) {
	if m.CreateConversationFunc != nil {
		return m.CreateConversationFunc(ctx, in)
	}
	return nil, errors.New("CreateConversation not implemented")
}

func (m *mockListConversationsService) GetConversationHistory(ctx context.Context, in *conversationservice.GetConversationHistoryReq, opts ...grpc.CallOption) (*conversationservice.GetConversationHistoryResp, error) {
	if m.GetConversationHistoryFunc != nil {
		return m.GetConversationHistoryFunc(ctx, in)
	}
	return nil, errors.New("GetConversationHistory not implemented")
}

func (m *mockListConversationsService) GetConversationMembers(ctx context.Context, in *conversationservice.GetConversationMembersReq, opts ...grpc.CallOption) (*conversationservice.GetConversationMembersResp, error) {
	if m.GetConversationMembersFunc != nil {
		return m.GetConversationMembersFunc(ctx, in)
	}
	return nil, errors.New("GetConversationMembers not implemented")
}

func (m *mockListConversationsService) GetUserConversations(ctx context.Context, in *conversationservice.GetUserConversationsReq, opts ...grpc.CallOption) (*conversationservice.GetUserConversationsResp, error) {
	if m.GetUserConversationsFunc != nil {
		return m.GetUserConversationsFunc(ctx, in)
	}
	return nil, errors.New("GetUserConversations not implemented")
}

func TestListConversations(t *testing.T) {
	is := assert.New(t)

	tests := []struct {
		name      string
		mockSetup func(*mockListConversationsService)
		wantResp  *types.ListConversationsResponse
		wantErr   *errorx.CodeError
	}{
		{
			name: "unauthorized - no identity in context",
			mockSetup: func(*mockListConversationsService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeAuth, "unauthorized"),
		},
		{
			name: "nil client",
			mockSetup: func(ms *mockListConversationsService) {
				*ms = mockListConversationsService{}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "rpc error - gRPC application error",
			mockSetup: func(ms *mockListConversationsService) {
				ms.GetUserConversationsFunc = func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error) {
					st := status.New(codes.NotFound, "user not found")
					return nil, st.Err()
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeNotFound, "user not found"),
		},
		{
			name: "rpc error - gRPC infrastructure error",
			mockSetup: func(ms *mockListConversationsService) {
				ms.GetUserConversationsFunc = func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error) {
					st := status.New(codes.Unavailable, "service unavailable")
					return nil, st.Err()
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "rpc error - non-gRPC error",
			mockSetup: func(ms *mockListConversationsService) {
				ms.GetUserConversationsFunc = func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error) {
					return nil, errors.New("some unexpected error")
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "success - empty conversations",
			mockSetup: func(ms *mockListConversationsService) {
				ms.GetUserConversationsFunc = func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error) {
					is.Equal(int64(42), in.UserId)
					return &conversationservice.GetUserConversationsResp{
						Conversations: []*pb.ConversationResponse{},
					}, nil
				}
			},
			wantResp: &types.ListConversationsResponse{
				Conversations: []types.ConversationItem{},
			},
			wantErr: nil,
		},
		{
			name: "success - with conversations",
			mockSetup: func(ms *mockListConversationsService) {
				ms.GetUserConversationsFunc = func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error) {
					return &conversationservice.GetUserConversationsResp{
						Conversations: []*pb.ConversationResponse{
							{
								Id:               1,
								ConversationType: "direct",
								IsActive:         true,
								CreatedAt:        1700000000,
								MemberIds:        []int64{42, 100},
							},
							{
								Id:               2,
								ConversationType: "group",
								IsActive:         false,
								CreatedAt:        1700000100,
								MemberIds:        []int64{42, 100, 200},
							},
						},
					}, nil
				}
			},
			wantResp: &types.ListConversationsResponse{
				Conversations: []types.ConversationItem{
					{
						ConversationId:   1,
						ConversationType: "direct",
						IsActive:         true,
						CreatedAt:        1700000000,
						MemberIds:        []int64{42, 100},
					},
					{
						ConversationId:   2,
						ConversationType: "group",
						IsActive:         false,
						CreatedAt:        1700000100,
						MemberIds:        []int64{42, 100, 200},
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "success - nil conversations treated as empty",
			mockSetup: func(ms *mockListConversationsService) {
				ms.GetUserConversationsFunc = func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error) {
					return &conversationservice.GetUserConversationsResp{
						Conversations: nil,
					}, nil
				}
			},
			wantResp: &types.ListConversationsResponse{
				Conversations: []types.ConversationItem{},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockListConversationsService{}
			tt.mockSetup(mockClient)

			svcCtx := &svc.ServiceContext{
				LogicConversationClient: mockClient,
			}

			var ctx context.Context
			if tt.wantErr != nil && tt.wantErr.Code == errorx.CodeAuth {
				ctx = context.Background()
			} else {
				ctx = ws.WithIdentity(context.Background(), ws.Identity{UserID: 42, DeviceID: "test-device"})
			}
			logic := NewListConversationsLogic(ctx, svcCtx)

			resp, err := logic.ListConversations()

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
