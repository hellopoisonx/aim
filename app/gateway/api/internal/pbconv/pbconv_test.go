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

func TestFriendshipSerialisesEmptyTagsAsList(t *testing.T) {
	item := Friendship(&pb.FriendshipResponse{UserId: 1, FriendId: 2, Status: "accepted"})

	raw, err := json.Marshal(item)
	require.NoError(t, err)
	require.JSONEq(t, `{"user_id":1,"friend_id":2,"status":"accepted","created_at":0,"updated_at":0,"name":"","email":"","avatar":"","tags":[]}`, string(raw))
}
