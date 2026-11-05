package mqs

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/rpc"
	"github.com/hellopoisonx/aim/app/core/rpc/internal/svc"
	logicpb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	gwpb "github.com/hellopoisonx/aim/shared/proto/gateway/pb"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeGatewayClient struct {
	pushes            []pushRecord
	typingPushes      []*gwpb.PushTypingReq
	readReceiptPushes []*gwpb.PushReadReceiptReq
	pushErr           error
	pushResp          *gwpb.PushMessageResp
}

type pushRecord struct {
	ctx context.Context
	req *gwpb.PushMessageReq
}

func (f *fakeGatewayClient) PushMessageToNode(ctx context.Context, _ string, req *gwpb.PushMessageReq) (*gwpb.PushMessageResp, error) {
	if f.pushErr != nil {
		return nil, f.pushErr
	}

	f.pushes = append(f.pushes, pushRecord{ctx: ctx, req: req})
	if f.pushResp != nil {
		return f.pushResp, nil
	}

	return &gwpb.PushMessageResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushPresenceToNode(context.Context, string, *gwpb.PushPresenceReq) (*gwpb.PushPresenceResp, error) {
	return &gwpb.PushPresenceResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushTypingToNode(_ context.Context, _ string, req *gwpb.PushTypingReq) (*gwpb.PushTypingResp, error) {
	f.typingPushes = append(f.typingPushes, req)

	return &gwpb.PushTypingResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushReadReceiptToNode(_ context.Context, _ string, req *gwpb.PushReadReceiptReq) (*gwpb.PushReadReceiptResp, error) {
	f.readReceiptPushes = append(f.readReceiptPushes, req)

	return &gwpb.PushReadReceiptResp{Success: true}, nil
}

func (f *fakeGatewayClient) PushFriendApplicationToNode(context.Context, string, *gwpb.PushFriendApplicationReq) (*gwpb.PushFriendApplicationResp, error) {
	return &gwpb.PushFriendApplicationResp{Success: true}, nil
}

func newFakeGatewayClient() *fakeGatewayClient {
	return &fakeGatewayClient{pushResp: &gwpb.PushMessageResp{Success: true}}
}

var _ rpc.GatewayPusher = (*fakeGatewayClient)(nil)

type fakeConversationClient struct {
	memberIDs        []int64
	conversationType string
	err              error
}

func (f *fakeConversationClient) CreateConversation(context.Context, *logicpb.CreateConversationReq, ...grpc.CallOption) (*logicpb.CreateConversationResp, error) {
	return nil, errors.New("CreateConversation not implemented")
}

func (f *fakeConversationClient) GetConversationHistory(context.Context, *logicpb.GetConversationHistoryReq, ...grpc.CallOption) (*logicpb.GetConversationHistoryResp, error) {
	return nil, errors.New("GetConversationHistory not implemented")
}

func (f *fakeConversationClient) GetConversationMembers(context.Context, *logicpb.GetConversationMembersReq, ...grpc.CallOption) (*logicpb.GetConversationMembersResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &logicpb.GetConversationMembersResp{
		MemberIds:        f.memberIDs,
		ConversationType: f.conversationType,
	}, nil
}

func (f *fakeConversationClient) GetUserConversations(context.Context, *logicpb.GetUserConversationsReq, ...grpc.CallOption) (*logicpb.GetUserConversationsResp, error) {
	return nil, errors.New("GetUserConversations not implemented")
}

func (f *fakeConversationClient) AddGroupMembers(context.Context, *logicpb.AddGroupMembersReq, ...grpc.CallOption) (*logicpb.AddGroupMembersResp, error) {
	return nil, errors.New("AddGroupMembers not implemented")
}

func (f *fakeConversationClient) RemoveGroupMembers(context.Context, *logicpb.RemoveGroupMembersReq, ...grpc.CallOption) (*logicpb.RemoveGroupMembersResp, error) {
	return nil, errors.New("RemoveGroupMembers not implemented")
}

func (f *fakeConversationClient) LeaveGroup(context.Context, *logicpb.LeaveGroupReq, ...grpc.CallOption) (*logicpb.LeaveGroupResp, error) {
	return nil, errors.New("LeaveGroup not implemented")
}

func (f *fakeConversationClient) DismissGroup(context.Context, *logicpb.DismissGroupReq, ...grpc.CallOption) (*logicpb.DismissGroupResp, error) {
	return nil, errors.New("DismissGroup not implemented")
}

func (f *fakeConversationClient) UpdateGroupInfo(context.Context, *logicpb.UpdateGroupInfoReq, ...grpc.CallOption) (*logicpb.UpdateGroupInfoResp, error) {
	return nil, errors.New("UpdateGroupInfo not implemented")
}

func (f *fakeConversationClient) GrantGroupAdmin(context.Context, *logicpb.GrantGroupAdminReq, ...grpc.CallOption) (*logicpb.GrantGroupAdminResp, error) {
	return nil, errors.New("GrantGroupAdmin not implemented")
}

func (f *fakeConversationClient) RevokeGroupAdmin(context.Context, *logicpb.RevokeGroupAdminReq, ...grpc.CallOption) (*logicpb.RevokeGroupAdminResp, error) {
	return nil, errors.New("RevokeGroupAdmin not implemented")
}

func (f *fakeConversationClient) TransferGroupOwner(context.Context, *logicpb.TransferGroupOwnerReq, ...grpc.CallOption) (*logicpb.TransferGroupOwnerResp, error) {
	return nil, errors.New("TransferGroupOwner not implemented")
}

func (f *fakeConversationClient) GetConversationMembersDetail(context.Context, *logicpb.GetConversationMembersDetailReq, ...grpc.CallOption) (*logicpb.GetConversationMembersDetailResp, error) {
	return nil, errors.New("GetConversationMembersDetail not implemented")
}

func (f *fakeConversationClient) UpdateReadReceipt(context.Context, *logicpb.UpdateReadReceiptReq, ...grpc.CallOption) (*logicpb.UpdateReadReceiptResp, error) {
	return nil, errors.New("UpdateReadReceipt not implemented")
}

func (f *fakeConversationClient) ListConversationReadStates(context.Context, *logicpb.ListConversationReadStatesReq, ...grpc.CallOption) (*logicpb.ListConversationReadStatesResp, error) {
	return nil, errors.New("ListConversationReadStates not implemented")
}

var _ logicpb.ConversationServiceClient = (*fakeConversationClient)(nil)

func newTestServiceContext(t *testing.T, gwClient rpc.GatewayPusher) *svc.ServiceContext {
	mr := miniredis.NewMiniRedis()
	require.NoError(t, mr.Start())
	t.Cleanup(func() { mr.Close() })

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("close redis client: %v", err)
		}
	})

	return &svc.ServiceContext{
		RedisClient:   client,
		GatewayClient: gwClient,
	}
}
