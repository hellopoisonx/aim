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
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockConversationService implements conversationservice.ConversationService for testing.
type mockConversationService struct {
	CreateConversationFunc     func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error)
	GetConversationHistoryFunc func(ctx context.Context, in *conversationservice.GetConversationHistoryReq) (*conversationservice.GetConversationHistoryResp, error)
	GetConversationMembersFunc func(ctx context.Context, in *conversationservice.GetConversationMembersReq) (*conversationservice.GetConversationMembersResp, error)
	GetUserConversationsFunc   func(ctx context.Context, in *conversationservice.GetUserConversationsReq) (*conversationservice.GetUserConversationsResp, error)
}

func (m *mockConversationService) CreateConversation(ctx context.Context, in *conversationservice.CreateConversationReq, opts ...grpc.CallOption) (*conversationservice.CreateConversationResp, error) {
	if m.CreateConversationFunc != nil {
		return m.CreateConversationFunc(ctx, in)
	}
	return nil, errors.New("CreateConversation not implemented")
}

func (m *mockConversationService) GetConversationHistory(ctx context.Context, in *conversationservice.GetConversationHistoryReq, opts ...grpc.CallOption) (*conversationservice.GetConversationHistoryResp, error) {
	if m.GetConversationHistoryFunc != nil {
		return m.GetConversationHistoryFunc(ctx, in)
	}
	return nil, errors.New("GetConversationHistory not implemented")
}

func (m *mockConversationService) GetConversationMembers(ctx context.Context, in *conversationservice.GetConversationMembersReq, opts ...grpc.CallOption) (*conversationservice.GetConversationMembersResp, error) {
	if m.GetConversationMembersFunc != nil {
		return m.GetConversationMembersFunc(ctx, in)
	}
	return nil, errors.New("GetConversationMembers not implemented")
}

func (m *mockConversationService) GetUserConversations(ctx context.Context, in *conversationservice.GetUserConversationsReq, opts ...grpc.CallOption) (*conversationservice.GetUserConversationsResp, error) {
	if m.GetUserConversationsFunc != nil {
		return m.GetUserConversationsFunc(ctx, in)
	}
	return nil, errors.New("GetUserConversations not implemented")
}

func (m *mockConversationService) GrantGroupAdmin(ctx context.Context, in *conversationservice.GrantGroupAdminReq, opts ...grpc.CallOption) (*conversationservice.GrantGroupAdminResp, error) {
	return nil, errors.New("GrantGroupAdmin not implemented")
}

func (m *mockConversationService) RevokeGroupAdmin(ctx context.Context, in *conversationservice.RevokeGroupAdminReq, opts ...grpc.CallOption) (*conversationservice.RevokeGroupAdminResp, error) {
	return nil, errors.New("RevokeGroupAdmin not implemented")
}

func (m *mockConversationService) TransferGroupOwner(ctx context.Context, in *conversationservice.TransferGroupOwnerReq, opts ...grpc.CallOption) (*conversationservice.TransferGroupOwnerResp, error) {
	return nil, errors.New("TransferGroupOwner not implemented")
}

func TestCreateConversation(t *testing.T) {
	is := assert.New(t)

	tests := []struct {
		name      string
		req       *types.CreateConversationRequest
		mockSetup func(*mockConversationService)
		wantResp  *types.CreateConversationResponse
		wantErr   *errorx.CodeError
	}{
		{
			name: "unauthorized - no identity in context",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "Direct Chat",
			},
			mockSetup: func(*mockConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeAuth, "unauthorized"),
		},
		{
			name: "invalid conversation_type - empty",
			req: &types.CreateConversationRequest{
				ConversationType: "",
				MemberIds:        []int64{1, 2},
				Name:             "Invalid Chat",
			},
			mockSetup: func(*mockConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'"),
		},
		{
			name: "invalid conversation_type - unknown value",
			req: &types.CreateConversationRequest{
				ConversationType: "channel",
				MemberIds:        []int64{1, 2},
				Name:             "Invalid Chat",
			},
			mockSetup: func(*mockConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeBadInput, "conversation_type must be 'direct' or 'group'"),
		},
		{
			name: "member_ids empty",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{},
				Name:             "Direct Chat",
			},
			mockSetup: func(*mockConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeBadInput, "direct conversation member_ids must contain exactly one peer user id"),
		},
		{
			name: "nil client",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "Direct Chat",
			},
			mockSetup: func(ms *mockConversationService) {
				*ms = mockConversationService{}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "rpc error - gRPC application error",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "Direct Chat",
			},
			mockSetup: func(ms *mockConversationService) {
				ms.CreateConversationFunc = func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error) {
					st := status.New(codes.InvalidArgument, "missing required field: member_ids")
					return nil, st.Err()
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeBadInput, "missing required field: member_ids"),
		},
		{
			name: "rpc error - gRPC infrastructure error",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "Direct Chat",
			},
			mockSetup: func(ms *mockConversationService) {
				ms.CreateConversationFunc = func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error) {
					st := status.New(codes.Internal, "connection refused")
					return nil, st.Err()
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "rpc error - non-gRPC error",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "Direct Chat",
			},
			mockSetup: func(ms *mockConversationService) {
				ms.CreateConversationFunc = func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error) {
					return nil, errors.New("some unexpected error")
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "nil conversation in response",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "Direct Chat",
			},
			mockSetup: func(ms *mockConversationService) {
				ms.CreateConversationFunc = func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error) {
					return &conversationservice.CreateConversationResp{}, nil
				}
			},
			wantResp: nil,
			wantErr:  errorx.NewCodeError(errorx.CodeInternal, "internal error"),
		},
		{
			name: "success - direct conversation with empty name",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "",
			},
			mockSetup: func(ms *mockConversationService) {
				ms.CreateConversationFunc = func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error) {
					require.Equal(t, int64(42), in.CreatorId, "CreatorId should be set from identity")
					return &conversationservice.CreateConversationResp{
						Conversation: &pb.ConversationResponse{
							Id:               12345,
							ConversationType: "direct",
							IsActive:         true,
							CreatedAt:        1700000000,
							MemberIds:        []int64{1, 2},
						},
					}, nil
				}
			},
			wantResp: &types.CreateConversationResponse{
				ConversationId:   12345,
				ConversationType: "direct",
				IsActive:         true,
				CreatedAt:        1700000000,
				MemberIds:        []int64{1, 2},
			},
			wantErr: nil,
		},
		{
			name: "missing group name",
			req: &types.CreateConversationRequest{
				ConversationType: "group",
				MemberIds:        []int64{1, 2},
				Name:             "   ",
			},
			mockSetup: func(*mockConversationService) {},
			wantResp:  nil,
			wantErr:   errorx.NewCodeError(errorx.CodeBadInput, "name is required"),
		},
		{
			name: "success - group conversation",
			req: &types.CreateConversationRequest{
				ConversationType: "group",
				MemberIds:        []int64{1, 2, 3, 4},
				Name:             "Group Chat",
			},
			mockSetup: func(ms *mockConversationService) {
				ms.CreateConversationFunc = func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error) {
					return &conversationservice.CreateConversationResp{
						Conversation: &pb.ConversationResponse{
							Id:               67890,
							ConversationType: "group",
							IsActive:         true,
							CreatedAt:        1700000100,
							MemberIds:        []int64{1, 2, 3, 4},
						},
					}, nil
				}
			},
			wantResp: &types.CreateConversationResponse{
				ConversationId:   67890,
				ConversationType: "group",
				IsActive:         true,
				CreatedAt:        1700000100,
				MemberIds:        []int64{1, 2, 3, 4},
			},
			wantErr: nil,
		},
		{
			name: "success - nil member_ids in rpc response",
			req: &types.CreateConversationRequest{
				ConversationType: "direct",
				MemberIds:        []int64{1},
				Name:             "Direct Chat",
			},
			mockSetup: func(ms *mockConversationService) {
				ms.CreateConversationFunc = func(ctx context.Context, in *conversationservice.CreateConversationReq) (*conversationservice.CreateConversationResp, error) {
					return &conversationservice.CreateConversationResp{
						Conversation: &pb.ConversationResponse{
							Id:               999,
							ConversationType: "direct",
							IsActive:         true,
							CreatedAt:        1700000200,
							MemberIds:        nil,
						},
					}, nil
				}
			},
			wantResp: &types.CreateConversationResponse{
				ConversationId:   999,
				ConversationType: "direct",
				IsActive:         true,
				CreatedAt:        1700000200,
				MemberIds:        []int64{},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockConversationService{}
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
			logic := NewCreateConversationLogic(ctx, svcCtx)

			resp, err := logic.CreateConversation(tt.req)

			if tt.wantErr != nil {
				is.Error(err)
				is.Equal(tt.wantErr.Code, err.(*errorx.CodeError).Code)
				is.Equal(tt.wantErr.Message, err.(*errorx.CodeError).Message)
				is.Nil(resp)
			} else {
				is.NoError(err)
				is.Equal(tt.wantResp, resp)
			}
		})
	}
}

func (m *mockConversationService) AddGroupMembers(ctx context.Context, in *pb.AddGroupMembersReq, opts ...grpc.CallOption) (*pb.AddGroupMembersResp, error) {
	return nil, errors.New("AddGroupMembers not implemented")
}
func (m *mockConversationService) RemoveGroupMembers(ctx context.Context, in *pb.RemoveGroupMembersReq, opts ...grpc.CallOption) (*pb.RemoveGroupMembersResp, error) {
	return nil, errors.New("RemoveGroupMembers not implemented")
}
func (m *mockConversationService) LeaveGroup(ctx context.Context, in *pb.LeaveGroupReq, opts ...grpc.CallOption) (*pb.LeaveGroupResp, error) {
	return nil, errors.New("LeaveGroup not implemented")
}
func (m *mockConversationService) DismissGroup(ctx context.Context, in *pb.DismissGroupReq, opts ...grpc.CallOption) (*pb.DismissGroupResp, error) {
	return nil, errors.New("DismissGroup not implemented")
}
func (m *mockConversationService) UpdateGroupInfo(ctx context.Context, in *pb.UpdateGroupInfoReq, opts ...grpc.CallOption) (*pb.UpdateGroupInfoResp, error) {
	return nil, errors.New("UpdateGroupInfo not implemented")
}
func (m *mockConversationService) GetConversationMembersDetail(ctx context.Context, in *pb.GetConversationMembersDetailReq, opts ...grpc.CallOption) (*pb.GetConversationMembersDetailResp, error) {
	return nil, errors.New("GetConversationMembersDetail not implemented")
}
func (m *mockConversationService) UpdateReadReceipt(ctx context.Context, in *pb.UpdateReadReceiptReq, opts ...grpc.CallOption) (*pb.UpdateReadReceiptResp, error) {
	return nil, errors.New("UpdateReadReceipt not implemented")
}
func (m *mockConversationService) ListConversationReadStates(ctx context.Context, in *pb.ListConversationReadStatesReq, opts ...grpc.CallOption) (*pb.ListConversationReadStatesResp, error) {
	return nil, errors.New("ListConversationReadStates not implemented")
}
