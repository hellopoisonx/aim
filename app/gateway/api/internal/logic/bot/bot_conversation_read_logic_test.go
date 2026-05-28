package bot

import (
	"context"
	"errors"
	"testing"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/conversationservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/botperm"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeBotConversationClient struct {
	membersReq     *conversationservice.GetConversationMembersReq
	historyReq     *conversationservice.GetConversationHistoryReq
	detailReq      *conversationservice.GetConversationMembersDetailReq
	updateReadReq  *conversationservice.UpdateReadReceiptReq
	listStatesReq  *conversationservice.ListConversationReadStatesReq
	membersResp    *conversationservice.GetConversationMembersResp
	historyResp    *conversationservice.GetConversationHistoryResp
	detailResp     *conversationservice.GetConversationMembersDetailResp
	updateReadResp *conversationservice.UpdateReadReceiptResp
	listStatesResp *conversationservice.ListConversationReadStatesResp
	membersErr     error
	historyErr     error
	detailErr      error
	updateReadErr  error
	listStatesErr  error
}

var _ conversationservice.ConversationService = (*fakeBotConversationClient)(nil)

func (f *fakeBotConversationClient) CreateConversation(context.Context, *conversationservice.CreateConversationReq, ...grpc.CallOption) (*conversationservice.CreateConversationResp, error) {
	return nil, errors.New("CreateConversation not implemented")
}

func (f *fakeBotConversationClient) GetConversationHistory(_ context.Context, in *conversationservice.GetConversationHistoryReq, _ ...grpc.CallOption) (*conversationservice.GetConversationHistoryResp, error) {
	f.historyReq = in
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	if f.historyResp != nil {
		return f.historyResp, nil
	}
	return &conversationservice.GetConversationHistoryResp{}, nil
}

func (f *fakeBotConversationClient) GetConversationMembers(_ context.Context, in *conversationservice.GetConversationMembersReq, _ ...grpc.CallOption) (*conversationservice.GetConversationMembersResp, error) {
	f.membersReq = in
	if f.membersErr != nil {
		return nil, f.membersErr
	}
	if f.membersResp != nil {
		return f.membersResp, nil
	}
	return &conversationservice.GetConversationMembersResp{MemberIds: []int64{1001, 2002}}, nil
}

func (f *fakeBotConversationClient) GetUserConversations(context.Context, *conversationservice.GetUserConversationsReq, ...grpc.CallOption) (*conversationservice.GetUserConversationsResp, error) {
	return nil, errors.New("GetUserConversations not implemented")
}

func (f *fakeBotConversationClient) AddGroupMembers(context.Context, *conversationservice.AddGroupMembersReq, ...grpc.CallOption) (*conversationservice.AddGroupMembersResp, error) {
	return nil, errors.New("AddGroupMembers not implemented")
}

func (f *fakeBotConversationClient) RemoveGroupMembers(context.Context, *conversationservice.RemoveGroupMembersReq, ...grpc.CallOption) (*conversationservice.RemoveGroupMembersResp, error) {
	return nil, errors.New("RemoveGroupMembers not implemented")
}

func (f *fakeBotConversationClient) LeaveGroup(context.Context, *conversationservice.LeaveGroupReq, ...grpc.CallOption) (*conversationservice.LeaveGroupResp, error) {
	return nil, errors.New("LeaveGroup not implemented")
}

func (f *fakeBotConversationClient) DismissGroup(context.Context, *conversationservice.DismissGroupReq, ...grpc.CallOption) (*conversationservice.DismissGroupResp, error) {
	return nil, errors.New("DismissGroup not implemented")
}

func (f *fakeBotConversationClient) UpdateGroupInfo(context.Context, *conversationservice.UpdateGroupInfoReq, ...grpc.CallOption) (*conversationservice.UpdateGroupInfoResp, error) {
	return nil, errors.New("UpdateGroupInfo not implemented")
}

func (f *fakeBotConversationClient) GrantGroupAdmin(context.Context, *conversationservice.GrantGroupAdminReq, ...grpc.CallOption) (*conversationservice.GrantGroupAdminResp, error) {
	return nil, errors.New("GrantGroupAdmin not implemented")
}

func (f *fakeBotConversationClient) RevokeGroupAdmin(context.Context, *conversationservice.RevokeGroupAdminReq, ...grpc.CallOption) (*conversationservice.RevokeGroupAdminResp, error) {
	return nil, errors.New("RevokeGroupAdmin not implemented")
}

func (f *fakeBotConversationClient) TransferGroupOwner(context.Context, *conversationservice.TransferGroupOwnerReq, ...grpc.CallOption) (*conversationservice.TransferGroupOwnerResp, error) {
	return nil, errors.New("TransferGroupOwner not implemented")
}

func (f *fakeBotConversationClient) GetConversationMembersDetail(_ context.Context, in *conversationservice.GetConversationMembersDetailReq, _ ...grpc.CallOption) (*conversationservice.GetConversationMembersDetailResp, error) {
	f.detailReq = in
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	if f.detailResp != nil {
		return f.detailResp, nil
	}
	return &conversationservice.GetConversationMembersDetailResp{}, nil
}

func (f *fakeBotConversationClient) UpdateReadReceipt(_ context.Context, in *conversationservice.UpdateReadReceiptReq, _ ...grpc.CallOption) (*conversationservice.UpdateReadReceiptResp, error) {
	f.updateReadReq = in
	if f.updateReadErr != nil {
		return nil, f.updateReadErr
	}
	if f.updateReadResp != nil {
		return f.updateReadResp, nil
	}
	return &conversationservice.UpdateReadReceiptResp{ReadState: &pb.ReadStateItem{UserId: in.UserId, LastReadMessageId: in.LastReadMessageId, UpdatedAt: 1700000000000}}, nil
}

func (f *fakeBotConversationClient) ListConversationReadStates(_ context.Context, in *conversationservice.ListConversationReadStatesReq, _ ...grpc.CallOption) (*conversationservice.ListConversationReadStatesResp, error) {
	f.listStatesReq = in
	if f.listStatesErr != nil {
		return nil, f.listStatesErr
	}
	if f.listStatesResp != nil {
		return f.listStatesResp, nil
	}
	return &conversationservice.ListConversationReadStatesResp{}, nil
}

type fakeReadReceiptPublisher struct {
	called            bool
	fromUserID        int64
	conversationID    int64
	lastReadMessageID int64
	updatedAt         int64
	err               error
}

func (f *fakeReadReceiptPublisher) PublishReadReceipt(_ context.Context, fromUserID, conversationID, lastReadMessageID, updatedAt int64) error {
	f.called = true
	f.fromUserID = fromUserID
	f.conversationID = conversationID
	f.lastReadMessageID = lastReadMessageID
	f.updatedAt = updatedAt
	return f.err
}

func TestBotGetConversationHistory(t *testing.T) {
	client := &fakeBotConversationClient{
		historyResp: &conversationservice.GetConversationHistoryResp{
			Messages: []*pb.MessageItem{{
				Id:             99,
				ConversationId: 7,
				SenderId:       2002,
				SenderInfo:     &pb.SenderInfo{Name: "alice", Email: "a@example.com"},
				MessageType:    "text",
				Content:        "hi",
				ClientMsgId:    "c1",
				CreatedAt:      1700000000000,
				ReadDetails: []*pb.MessageReadDetailItem{{
					UserId:            1001,
					IsRead:            true,
					LastReadMessageId: 99,
					UpdatedAt:         1700000000001,
					Email:             "bot@example.com",
				}},
			}},
			NextCursorId:        88,
			NextCursorCreatedAt: 1699999999999,
			HasMore:             true,
			ReadStates:          []*pb.ReadStateItem{{UserId: 1001, LastReadMessageId: 99, UpdatedAt: 1700000000001}},
		},
	}
	logic := NewBotGetConversationHistoryLogic(ctxWithBot(botperm.ActionConversationHistory), &svc.ServiceContext{LogicConversationClient: client})

	resp, err := logic.BotGetConversationHistory(&types.BotGetConversationHistoryRequest{Id: "7", CursorId: "100", Limit: 200})

	require.NoError(t, err)
	require.Equal(t, int64(7), client.membersReq.ConversationId)
	require.Equal(t, int64(100), client.historyReq.CursorId)
	require.Equal(t, int32(100), client.historyReq.Limit)
	require.Equal(t, "99", resp.Messages[0].Id)
	require.Equal(t, "2002", resp.Messages[0].SenderId)
	require.Equal(t, "99", resp.Messages[0].ReadDetails[0].LastReadMessageId)
	require.Equal(t, "88", resp.NextCursorId)
	require.Equal(t, "1001", resp.ReadStates[0].UserId)
}

func TestBotGetConversationHistory_RequiresMembership(t *testing.T) {
	client := &fakeBotConversationClient{membersResp: &conversationservice.GetConversationMembersResp{MemberIds: []int64{2002}}}
	logic := NewBotGetConversationHistoryLogic(ctxWithBot(botperm.ActionConversationHistory), &svc.ServiceContext{LogicConversationClient: client})

	_, err := logic.BotGetConversationHistory(&types.BotGetConversationHistoryRequest{Id: "7"})

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeForbidden, ce.Code)
	require.Nil(t, client.historyReq)
}

func TestBotGetConversationMembers(t *testing.T) {
	client := &fakeBotConversationClient{detailResp: &conversationservice.GetConversationMembersDetailResp{Members: []*pb.MemberDetailItem{{UserId: 1001, Email: "bot@example.com", Avatar: "avatar", Role: "member", JoinedAt: 1, Name: "bot"}}}}
	logic := NewBotGetConversationMembersLogic(ctxWithBot(botperm.ActionConversationMembersRead), &svc.ServiceContext{LogicConversationClient: client})

	resp, err := logic.BotGetConversationMembers(&types.BotGetConversationMembersRequest{Id: "7"})

	require.NoError(t, err)
	require.Equal(t, int64(7), client.detailReq.ConversationId)
	require.Equal(t, "1001", resp.Members[0].UserId)
	require.Equal(t, "bot", resp.Members[0].Name)
}

func TestBotListReadStates(t *testing.T) {
	client := &fakeBotConversationClient{listStatesResp: &conversationservice.ListConversationReadStatesResp{ReadStates: []*pb.ReadStateItem{{UserId: 2002, LastReadMessageId: 42, UpdatedAt: 2}}}}
	logic := NewBotListReadStatesLogic(ctxWithBot(botperm.ActionReadReceiptRead), &svc.ServiceContext{LogicConversationClient: client})

	resp, err := logic.BotListReadStates(&types.BotListReadStatesRequest{Id: "7"})

	require.NoError(t, err)
	require.Equal(t, int64(7), client.listStatesReq.ConversationId)
	require.Equal(t, "2002", resp.ReadStates[0].UserId)
	require.Equal(t, "42", resp.ReadStates[0].LastReadMessageId)
}

func TestBotMarkReadPublishesBestEffort(t *testing.T) {
	client := &fakeBotConversationClient{}
	publisher := &fakeReadReceiptPublisher{err: errors.New("kafka down")}
	logic := NewBotMarkReadLogic(ctxWithBot(botperm.ActionReadReceiptWrite), &svc.ServiceContext{
		LogicConversationClient: client,
		ReadReceiptPub:          publisher,
	})

	resp, err := logic.BotMarkRead(&types.BotMarkReadRequest{Id: "7", LastReadMessageId: "42"})

	require.NoError(t, err)
	require.Equal(t, int64(7), client.updateReadReq.ConversationId)
	require.Equal(t, int64(1001), client.updateReadReq.UserId)
	require.Equal(t, int64(42), client.updateReadReq.LastReadMessageId)
	require.True(t, publisher.called)
	require.Equal(t, int64(1001), publisher.fromUserID)
	require.Equal(t, "42", resp.ReadState.LastReadMessageId)
}

func TestBotMarkReadValidatesIDs(t *testing.T) {
	logic := NewBotMarkReadLogic(ctxWithBot(botperm.ActionReadReceiptWrite), &svc.ServiceContext{LogicConversationClient: &fakeBotConversationClient{}})

	_, err := logic.BotMarkRead(&types.BotMarkReadRequest{Id: "bad", LastReadMessageId: "42"})

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBadInput, ce.Code)
}

func TestBotConversationReadRequiresAction(t *testing.T) {
	logic := NewBotListReadStatesLogic(ctxWithBot(botperm.ActionConversationList), &svc.ServiceContext{LogicConversationClient: &fakeBotConversationClient{}})

	_, err := logic.BotListReadStates(&types.BotListReadStatesRequest{Id: "7"})

	var ce *errorx.CodeError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, errorx.CodeBotScopeDenied, ce.Code)
}
