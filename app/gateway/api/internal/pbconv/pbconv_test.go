package pbconv

import (
	"encoding/json"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
	"github.com/stretchr/testify/require"
)

func TestConversationSerialisesEmptyMemberIdsAsList(t *testing.T) {
	item := Conversation(&pb.ConversationResponse{Id: 1})

	raw, err := json.Marshal(item)
	require.NoError(t, err)
	require.JSONEq(t, `{"conversation_id":1,"conversation_type":"","is_active":false,"created_at":0,"member_ids":[],"name":"","avatar":"","creator_id":0}`, string(raw))
}

func TestUserSerialisationUsesNumericIDAndRPCUnits(t *testing.T) {
	user := &pb.UserInfoResponse{
		Id:        42,
		Nickname:  "alice",
		CreatedAt: 1_700_000_000_123,
		UpdatedAt: 1_700_000_000_456,
	}

	info, err := json.Marshal(UserInfo(user))
	require.NoError(t, err)
	require.JSONEq(t, `{"id":42,"email":"","status":0,"nickname":"alice","avatar":"","created_at":1700000000123,"updated_at":1700000000456}`, string(info))

	list, err := json.Marshal(UserListItem(user))
	require.NoError(t, err)
	require.JSONEq(t, `{"id":42,"nickname":"alice","email":"","avatar":""}`, string(list))
}

func TestFriendshipSerialisesEmptyTagsAsList(t *testing.T) {
	item := Friendship(&pb.FriendshipResponse{UserId: 1, FriendId: 2, Status: "accepted"})

	raw, err := json.Marshal(item)
	require.NoError(t, err)
	require.JSONEq(t, `{"user_id":1,"friend_id":2,"status":"accepted","created_at":0,"updated_at":0,"name":"","email":"","avatar":"","tags":[]}`, string(raw))
}

func TestMessageReadDetailCarriesMemberName(t *testing.T) {
	detail := MessageReadDetail(&pb.MessageReadDetailItem{
		UserId:            7,
		IsRead:            true,
		LastReadMessageId: 99,
		UpdatedAt:         1_700_000_000_123,
		Email:             "bob@example.com",
		Avatar:            "b.png",
		Name:              "bob",
	})

	raw, err := json.Marshal(detail)
	require.NoError(t, err)
	require.JSONEq(t, `{"user_id":7,"is_read":true,"last_read_message_id":99,"updated_at":1700000000123,"email":"bob@example.com","avatar":"b.png","name":"bob"}`, string(raw))
}
