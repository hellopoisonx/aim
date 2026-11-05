package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	"github.com/hellopoisonx/aim/app/logic/rpc/internal/servicefakes"
	"github.com/hellopoisonx/aim/app/logic/rpc/model/orm"
	sharedconversation "github.com/hellopoisonx/aim/app/shared/conversation"
	"github.com/hellopoisonx/aim/app/shared/events"
	"github.com/hellopoisonx/aim/app/shared/rocketmq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEventTxRunner is a ConversationTxRunner that can also publish
// transactionally, recording every event it was asked to commit.
type fakeEventTxRunner struct {
	store    *servicefakes.ConversationStore
	messages []rocketmq.TransactionalMessage
	err      error
}

func (r *fakeEventTxRunner) WithTx(_ context.Context, fn func(tx ConversationStore) error) error {
	if r.err != nil {
		return r.err
	}
	return fn(r.store)
}

func (r *fakeEventTxRunner) WithEventTx(_ context.Context, message rocketmq.TransactionalMessage, fn func(tx ConversationStore) error) error {
	if r.err != nil {
		return r.err
	}
	r.messages = append(r.messages, message)
	return fn(r.store)
}

// plainTxRunner only supports plain transactions, i.e. transactional publishing
// is not configured.
type plainTxRunner struct {
	store *servicefakes.ConversationStore
}

func (r plainTxRunner) WithTx(_ context.Context, fn func(tx ConversationStore) error) error {
	return fn(r.store)
}

func newGroupConversation(t *testing.T, store *servicefakes.ConversationStore, runner ConversationTxRunner) *ConversationService {
	t.Helper()
	svc := newTestConversationService(t, store, runner)
	_, err := svc.CreateConversation(context.Background(), sharedconversation.TypeGroup, 1, []int64{1, 2, 3}, "group", "")
	require.NoError(t, err)
	return svc
}

// formatConversationKey mirrors the key the service sends with each event.
func formatConversationKey(conversationID int64) string {
	return strconv.FormatInt(conversationID, 10)
}

func firstConversationID(t *testing.T, store *servicefakes.ConversationStore) int64 {
	t.Helper()
	require.Len(t, store.Conversations, 1)
	for id := range store.Conversations {
		return id
	}
	return 0
}

func TestAddGroupMembers_PublishesTransactionalEvent(t *testing.T) {
	store := servicefakes.NewConversationStore()
	runner := &fakeEventTxRunner{store: store}
	svc := newGroupConversation(t, store, runner)
	svc.eventTopic = "aim-conversation-events"

	conversationID := firstConversationID(t, store)
	_, err := svc.AddGroupMembers(context.Background(), conversationID, 1, []int64{4})
	require.NoError(t, err)

	require.Len(t, runner.messages, 1)
	message := runner.messages[0]
	assert.Equal(t, "aim-conversation-events", message.Topic)
	assert.Contains(t, message.EventUID, "conversation.event:")
	assert.Equal(t, formatConversationKey(conversationID), message.Key)

	var payload events.ConversationEvent
	require.NoError(t, json.Unmarshal(message.Body, &payload))
	assert.Equal(t, conversationID, payload.ConversationID)
	assert.ElementsMatch(t, []int64{1, 2, 3, 4}, payload.TargetUserIDs, "recipients include existing and new members")
	assert.Contains(t, payload.Content, "member_joined")

	// The system message must be part of the same transaction.
	assert.Len(t, store.Messages[conversationID], 1)
	assert.Equal(t, payload.MessageID, store.Messages[conversationID][0].ID)
}

func TestUpdateGroupInfo_PublishesAvatarEvent(t *testing.T) {
	store := servicefakes.NewConversationStore()
	runner := &fakeEventTxRunner{store: store}
	svc := newGroupConversation(t, store, runner)
	svc.eventTopic = "aim-conversation-events"

	conversationID := firstConversationID(t, store)
	avatar := "https://example.test/a.png"
	_, err := svc.UpdateGroupInfo(context.Background(), conversationID, 1, nil, &avatar)
	require.NoError(t, err)

	require.Len(t, runner.messages, 1)
	var payload events.ConversationEvent
	require.NoError(t, json.Unmarshal(runner.messages[0].Body, &payload))
	assert.Contains(t, payload.Content, "group_avatar_changed")
	assert.ElementsMatch(t, []int64{1, 2, 3}, payload.TargetUserIDs)
}

func TestUpdateGroupInfo_RenameAndAvatarAnnounceRename(t *testing.T) {
	store := servicefakes.NewConversationStore()
	runner := &fakeEventTxRunner{store: store}
	svc := newGroupConversation(t, store, runner)
	svc.eventTopic = "aim-conversation-events"

	conversationID := firstConversationID(t, store)
	name := "renamed"
	avatar := "https://example.test/b.png"
	_, err := svc.UpdateGroupInfo(context.Background(), conversationID, 1, &name, &avatar)
	require.NoError(t, err)

	require.Len(t, runner.messages, 1)
	var payload events.ConversationEvent
	require.NoError(t, json.Unmarshal(runner.messages[0].Body, &payload))
	assert.Contains(t, payload.Content, "group_renamed")
	assert.NotContains(t, payload.Content, "group_avatar_changed")
	assert.Equal(t, name, store.Conversations[conversationID].Name)
	assert.Equal(t, avatar, store.Conversations[conversationID].Avatar)
}

// refetchFailStore fails the post-commit conversation lookup, which is the
// third read of a group mutation (first check, member lookup, re-fetch).
type refetchFailStore struct {
	*servicefakes.ConversationStore
	failAt int
	gets   int
}

func (s *refetchFailStore) GetConversation(ctx context.Context, id int64) (orm.Conversation, error) {
	s.gets++
	if s.gets >= s.failAt {
		return orm.Conversation{}, errors.New("refetch failed")
	}
	return s.ConversationStore.GetConversation(ctx, id)
}

func TestGroupMutation_RefetchErrorIsPropagated(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := newTestConversationService(t, store, plainTxRunner{store: store})
	_, err := svc.CreateConversation(context.Background(), sharedconversation.TypeGroup, 1, []int64{1, 2, 3}, "group", "")
	require.NoError(t, err)
	conversationID := firstConversationID(t, store)

	refetchFail := &refetchFailStore{ConversationStore: store, failAt: 3}
	svc.store = refetchFail

	name := "renamed"
	_, err = svc.UpdateGroupInfo(context.Background(), conversationID, 1, &name, nil)
	require.ErrorContains(t, err, "refetch failed")

	refetchFail.gets = 0
	_, err = svc.AddGroupMembers(context.Background(), conversationID, 1, []int64{4})
	require.ErrorContains(t, err, "refetch failed")
	require.Equal(t, 3, refetchFail.gets)
}

func TestDismissGroup_PublishesBeforeDeactivation(t *testing.T) {
	store := servicefakes.NewConversationStore()
	runner := &fakeEventTxRunner{store: store}
	svc := newGroupConversation(t, store, runner)
	svc.eventTopic = "aim-conversation-events"

	conversationID := firstConversationID(t, store)
	require.NoError(t, svc.DismissGroup(context.Background(), conversationID, 1))

	require.Len(t, runner.messages, 1)
	var payload events.ConversationEvent
	require.NoError(t, json.Unmarshal(runner.messages[0].Body, &payload))
	assert.ElementsMatch(t, []int64{1, 2, 3}, payload.TargetUserIDs)
	assert.False(t, store.Conversations[conversationID].IsActive)
}

func TestGroupMutation_WithoutTransactionalRunnerPublishesNothing(t *testing.T) {
	store := servicefakes.NewConversationStore()
	svc := newGroupConversation(t, store, plainTxRunner{store: store})
	svc.eventTopic = "aim-conversation-events"

	conversationID := firstConversationID(t, store)
	_, err := svc.AddGroupMembers(context.Background(), conversationID, 1, []int64{4})
	require.NoError(t, err)

	// The write still commits even though no event can be published.
	members, err := store.GetConversationMembers(context.Background(), conversationID)
	require.NoError(t, err)
	assert.Len(t, members, 4)
	assert.Len(t, store.Messages[conversationID], 1)
}
