package pbconv

import (
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	"github.com/hellopoisonx/aim/app/logic/rpc/pb"
)

// Conversation maps a stored conversation to its RPC representation.
func Conversation(conv orm.Conversation, memberIDs []int64, name string) *pb.ConversationResponse {
	return &pb.ConversationResponse{
		Id:               conv.ID,
		ConversationType: string(conv.ConversationType),
		IsActive:         conv.IsActive,
		CreatedAt:        conv.CreatedAt.UnixMilli(),
		MemberIds:        memberIDs,
		Name:             name,
		Avatar:           conv.Avatar,
		CreatorId:        conv.CreatorID,
	}
}

// User maps a stored user profile to its RPC representation.
func User(info orm.UserInfo) *pb.UserInfoResponse {
	return &pb.UserInfoResponse{
		Id:        info.ID,
		Email:     info.Email,
		Status:    int32(info.Status),
		Nickname:  info.Nickname,
		Avatar:    info.Avatar,
		CreatedAt: info.CreatedAt.Unix(),
		UpdatedAt: info.UpdatedAt.Unix(),
	}
}
