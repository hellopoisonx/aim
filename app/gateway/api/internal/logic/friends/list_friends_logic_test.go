package friends

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/types"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	friendshippb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeFriendshipClient serves ListFriends and fails every other RPC.
type fakeFriendshipClient struct {
	resp *friendshippb.ListFriendsResp
	err  error
}

func (f *fakeFriendshipClient) ListFriends(_ context.Context, _ *friendshippb.ListFriendsReq, _ ...grpc.CallOption) (*friendshippb.ListFriendsResp, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.resp, nil
}

func (f *fakeFriendshipClient) AddFriend(context.Context, *friendshippb.AddFriendReq, ...grpc.CallOption) (*friendshippb.AddFriendResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) AcceptFriend(context.Context, *friendshippb.AcceptFriendReq, ...grpc.CallOption) (*friendshippb.AcceptFriendResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) RejectFriend(context.Context, *friendshippb.RejectFriendReq, ...grpc.CallOption) (*friendshippb.RejectFriendResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) ListFriendApplications(context.Context, *friendshippb.ListFriendApplicationsReq, ...grpc.CallOption) (*friendshippb.ListFriendApplicationsResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) CreateFriendTag(context.Context, *friendshippb.CreateFriendTagReq, ...grpc.CallOption) (*friendshippb.CreateFriendTagResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) RenameFriendTag(context.Context, *friendshippb.RenameFriendTagReq, ...grpc.CallOption) (*friendshippb.RenameFriendTagResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) DeleteFriendTag(context.Context, *friendshippb.DeleteFriendTagReq, ...grpc.CallOption) (*friendshippb.DeleteFriendTagResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) ListFriendTags(context.Context, *friendshippb.ListFriendTagsReq, ...grpc.CallOption) (*friendshippb.ListFriendTagsResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) SetFriendTags(context.Context, *friendshippb.SetFriendTagsReq, ...grpc.CallOption) (*friendshippb.SetFriendTagsResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) RemoveFriendTag(context.Context, *friendshippb.RemoveFriendTagReq, ...grpc.CallOption) (*friendshippb.RemoveFriendTagResp, error) {
	return nil, errNotImplemented
}

type errString string

func (e errString) Error() string { return string(e) }

const errNotImplemented = errString("not implemented in fake")

// fakeUserClient returns a profile per requested id and records the calls.
type fakeUserClient struct {
	calls atomic.Int32
	err   error
	delay time.Duration
}

func (f *fakeUserClient) GetUserInfo(_ context.Context, req *friendshippb.GetUserInfoReq, _ ...grpc.CallOption) (*friendshippb.GetUserInfoResp, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return nil, f.err
	}

	return &friendshippb.GetUserInfoResp{
		User: &friendshippb.UserInfoResponse{
			Id:       req.GetId(),
			Nickname: fmt.Sprintf("user-%d", req.GetId()),
			Email:    fmt.Sprintf("user-%d@example.com", req.GetId()),
		},
	}, nil
}

func (f *fakeUserClient) CreateUserInfo(context.Context, *friendshippb.CreateUserInfoReq, ...grpc.CallOption) (*friendshippb.CreateUserInfoResp, error) {
	return nil, errNotImplemented
}

func (f *fakeUserClient) GetUserInfoByEmail(context.Context, *friendshippb.GetUserInfoByEmailReq, ...grpc.CallOption) (*friendshippb.GetUserInfoResp, error) {
	return nil, errNotImplemented
}

func (f *fakeUserClient) GetUserInfoByNickname(context.Context, *friendshippb.GetUserInfoByNicknameReq, ...grpc.CallOption) (*friendshippb.GetUserInfoResp, error) {
	return nil, errNotImplemented
}

func (f *fakeUserClient) UpdateUserInfoProfile(context.Context, *friendshippb.UpdateUserInfoProfileReq, ...grpc.CallOption) (*friendshippb.UpdateUserInfoProfileResp, error) {
	return nil, errNotImplemented
}

func (f *fakeUserClient) UpdateUserInfoStatus(context.Context, *friendshippb.UpdateUserInfoStatusReq, ...grpc.CallOption) (*friendshippb.UpdateUserInfoStatusResp, error) {
	return nil, errNotImplemented
}

func (f *fakeUserClient) SearchUserInfoByNickname(context.Context, *friendshippb.SearchUserInfoByNicknameReq, ...grpc.CallOption) (*friendshippb.SearchUserInfoByNicknameResp, error) {
	return nil, errNotImplemented
}

var (
	_ friendshipservice.FriendshipService = (*fakeFriendshipClient)(nil)
	_ userservice.UserService             = (*fakeUserClient)(nil)
)

func friendship(id, friendID int64) *friendshippb.FriendshipResponse {
	return &friendshippb.FriendshipResponse{UserId: id, FriendId: friendID, Status: "accepted"}
}

func TestListFriendsRequiresIdentity(t *testing.T) {
	logic := NewListFriendsLogic(context.Background(), &svc.ServiceContext{})

	_, err := logic.ListFriends()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, errorx.CodeAuth, codeErr.Code)
}

func TestListFriendsEnrichesEveryPeer(t *testing.T) {
	friendClient := &fakeFriendshipClient{resp: &friendshippb.ListFriendsResp{
		Friends: []*friendshippb.FriendshipResponse{
			friendship(1, 10), friendship(1, 11), friendship(10, 1),
		},
	}}
	userClient := &fakeUserClient{}
	svcCtx := &svc.ServiceContext{LogicFriendshipClient: friendClient, LogicUserClient: userClient}

	ctx := authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 1, DeviceID: "test"})
	resp, err := NewListFriendsLogic(ctx, svcCtx).ListFriends()
	require.NoError(t, err)
	require.Len(t, resp.Friends, 3)

	require.Equal(t, "user-10", resp.Friends[0].Name)
	require.Equal(t, "user-11", resp.Friends[1].Name)
	// For an incoming friendship the peer is UserId, not FriendId.
	require.Equal(t, "user-10", resp.Friends[2].Name)
	require.EqualValues(t, 3, userClient.calls.Load())
}

func TestListFriendsToleratesUserLookupFailure(t *testing.T) {
	friendClient := &fakeFriendshipClient{resp: &friendshippb.ListFriendsResp{
		Friends: []*friendshippb.FriendshipResponse{friendship(1, 10)},
	}}
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: friendClient,
		LogicUserClient:       &fakeUserClient{err: errNotImplemented},
	}

	ctx := authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 1})
	resp, err := NewListFriendsLogic(ctx, svcCtx).ListFriends()
	require.NoError(t, err)
	require.Len(t, resp.Friends, 1)
	require.Empty(t, resp.Friends[0].Name, "profile lookups stay best effort")
}

func TestListFriendsRunsLookupsConcurrently(t *testing.T) {
	const friends = 16
	items := make([]*friendshippb.FriendshipResponse, 0, friends)
	for i := range friends {
		items = append(items, friendship(1, int64(100+i)))
	}

	userClient := &fakeUserClient{delay: 40 * time.Millisecond}
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{resp: &friendshippb.ListFriendsResp{Friends: items}},
		LogicUserClient:       userClient,
	}

	ctx := authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 1})
	start := time.Now()
	resp, err := NewListFriendsLogic(ctx, svcCtx).ListFriends()
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, resp.Friends, friends)
	// Serial lookups would take friends*delay = 640ms.
	require.Less(t, elapsed, time.Duration(friends)*userClient.delay/2, "lookups should run in parallel")
}

func TestListFriendsSanitizesRPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{err: errString("redis password leaked")},
	}

	ctx := authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 1})
	_, err := NewListFriendsLogic(ctx, svcCtx).ListFriends()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, errorx.CodeInternal, codeErr.Code)
	require.Equal(t, "internal error", codeErr.Message)
}

var _ = types.ListFriendsResponse{}
