package presence

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/authctx"
	"github.com/hellopoisonx/aim/app/gateway/api/internal/svc"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/friendshipservice"
	"github.com/hellopoisonx/aim/app/logic/rpc/client/userservice"
	friendshippb "github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/hellopoisonx/aim/app/shared/errorx"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type errString string

func (e errString) Error() string { return string(e) }

const errNotImplemented = errString("not implemented in fake")

// fakeFriendshipClient serves ListFriends and fails every other RPC.
type fakeFriendshipClient struct {
	friendIDs []int64
	err       error
}

func (f *fakeFriendshipClient) ListFriends(_ context.Context, _ *friendshippb.ListFriendsReq, _ ...grpc.CallOption) (*friendshippb.ListFriendsResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	resp := &friendshippb.ListFriendsResp{}
	for _, id := range f.friendIDs {
		resp.Friends = append(resp.Friends, &friendshippb.FriendshipResponse{UserId: 1, FriendId: id, Status: "accepted"})
	}

	return resp, nil
}

func (f *fakeFriendshipClient) AddFriend(context.Context, *friendshippb.AddFriendReq, ...grpc.CallOption) (*friendshippb.AddFriendResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) ListFriendApplications(context.Context, *friendshippb.ListFriendApplicationsReq, ...grpc.CallOption) (*friendshippb.ListFriendApplicationsResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) AcceptFriend(context.Context, *friendshippb.AcceptFriendReq, ...grpc.CallOption) (*friendshippb.AcceptFriendResp, error) {
	return nil, errNotImplemented
}

func (f *fakeFriendshipClient) RejectFriend(context.Context, *friendshippb.RejectFriendReq, ...grpc.CallOption) (*friendshippb.RejectFriendResp, error) {
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

// fakeUserClient returns a nickname per id, or err when set.
type fakeUserClient struct {
	err error
}

func (f *fakeUserClient) GetUserInfo(_ context.Context, req *friendshippb.GetUserInfoReq, _ ...grpc.CallOption) (*friendshippb.GetUserInfoResp, error) {
	if f.err != nil {
		return nil, f.err
	}

	return &friendshippb.GetUserInfoResp{
		User: &friendshippb.UserInfoResponse{Nickname: fmt.Sprintf("user-%d", req.GetId())},
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

func authedCtx() context.Context {
	return authctx.WithIdentity(context.Background(), authctx.Identity{UserID: 1, DeviceID: "test"})
}

// presenceRedis starts miniredis and returns a client for it.
func presenceRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return client, mr
}

// addPresence registers live connections for userID the way the ws manager does.
func addPresence(t *testing.T, mr *miniredis.Miniredis, userID int64, nodes ...string) {
	t.Helper()

	for _, node := range nodes {
		_, err := mr.SAdd(fmt.Sprintf("aim:presence:%d", userID), node)
		require.NoError(t, err)
	}
}

func TestGetFriendsPresenceRequiresIdentity(t *testing.T) {
	_, err := NewGetFriendsPresenceLogic(context.Background(), &svc.ServiceContext{}).GetFriendsPresence()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, errorx.CodeAuth, codeErr.Code)
}

func TestGetFriendsPresenceWithoutFriendsReturnsEmptySlice(t *testing.T) {
	client, _ := presenceRedis(t)
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{},
		RedisClient:           client,
	}

	resp, err := NewGetFriendsPresenceLogic(authedCtx(), svcCtx).GetFriendsPresence()
	require.NoError(t, err)
	require.NotNil(t, resp.Presences)
	require.Empty(t, resp.Presences)
}

func TestGetFriendsPresenceMapsConnectionCountToStatus(t *testing.T) {
	client, mr := presenceRedis(t)
	// Two live connections for friend 10, none for friend 11.
	addPresence(t, mr, 10, "node-a", "node-b")
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{friendIDs: []int64{10, 11}},
		LogicUserClient:       &fakeUserClient{},
		RedisClient:           client,
	}

	resp, err := NewGetFriendsPresenceLogic(authedCtx(), svcCtx).GetFriendsPresence()
	require.NoError(t, err)
	require.Equal(t, []string{"online", "offline"}, []string{resp.Presences[0].Status, resp.Presences[1].Status})

	require.EqualValues(t, 10, resp.Presences[0].UserId)
	require.Equal(t, "user-10", resp.Presences[0].Name)
	require.Equal(t, "user-11", resp.Presences[1].Name)
}

func TestGetFriendsPresenceWithoutRedisReturnsEmptySlice(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{friendIDs: []int64{10}},
		LogicUserClient:       &fakeUserClient{},
	}

	resp, err := NewGetFriendsPresenceLogic(authedCtx(), svcCtx).GetFriendsPresence()
	require.NoError(t, err)
	require.NotNil(t, resp.Presences)
	require.Empty(t, resp.Presences)
}

func TestGetFriendsPresenceToleratesRedisFailure(t *testing.T) {
	client, mr := presenceRedis(t)
	mr.Close()
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{friendIDs: []int64{10}},
		RedisClient:           client,
	}

	resp, err := NewGetFriendsPresenceLogic(authedCtx(), svcCtx).GetFriendsPresence()
	require.NoError(t, err)
	require.NotNil(t, resp.Presences)
	require.Empty(t, resp.Presences)
}

func TestGetFriendsPresenceToleratesUserLookupFailure(t *testing.T) {
	client, mr := presenceRedis(t)
	addPresence(t, mr, 10, "node-a")
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{friendIDs: []int64{10}},
		LogicUserClient:       &fakeUserClient{err: errNotImplemented},
		RedisClient:           client,
	}

	resp, err := NewGetFriendsPresenceLogic(authedCtx(), svcCtx).GetFriendsPresence()
	require.NoError(t, err)
	require.Len(t, resp.Presences, 1)
	require.Equal(t, "online", resp.Presences[0].Status)
	require.Empty(t, resp.Presences[0].Name, "profile lookups stay best effort")
}

func TestGetFriendsPresenceSanitizesRPCError(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		LogicFriendshipClient: &fakeFriendshipClient{err: errString("redis password leaked")},
	}

	_, err := NewGetFriendsPresenceLogic(authedCtx(), svcCtx).GetFriendsPresence()

	var codeErr *errorx.CodeError
	require.ErrorAs(t, err, &codeErr)
	require.Equal(t, errorx.CodeInternal, codeErr.Code)
	require.Equal(t, "internal error", codeErr.Message)
}
