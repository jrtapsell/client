package chat

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/keybase/client/go/chat/types"
	"github.com/keybase/client/go/kbtest"
	"github.com/keybase/client/go/libkb"
	"github.com/keybase/client/go/protocol/chat1"
	"github.com/keybase/client/go/protocol/gregor1"
	"github.com/keybase/client/go/protocol/keybase1"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestGetThreadSupersedes(t *testing.T) {
	testGetThreadSupersedes(t, false)
	testGetThreadSupersedes(t, true)
}

func testGetThreadSupersedes(t *testing.T, deleteHistory bool) {
	ctx, world, ri, _, sender, _ := setupTest(t, 1)
	defer world.Cleanup()

	u := world.GetUsers()[0]
	tc := world.Tcs[u.Username]
	trip := newConvTriple(ctx, t, tc, u.Username)
	firstMessagePlaintext := chat1.MessagePlaintext{
		ClientHeader: chat1.MessageClientHeader{
			Conv:        trip,
			TlfName:     u.Username,
			TlfPublic:   false,
			MessageType: chat1.MessageType_TLFNAME,
		},
		MessageBody: chat1.MessageBody{},
	}
	firstMessageBoxed, _, _, _, _, err := sender.Prepare(ctx, firstMessagePlaintext,
		chat1.ConversationMembersType_KBFS, nil)
	require.NoError(t, err)
	res, err := ri.NewConversationRemote2(ctx, chat1.NewConversationRemote2Arg{
		IdTriple:   trip,
		TLFMessage: *firstMessageBoxed,
	})
	require.NoError(t, err)

	t.Logf("basic test")
	_, msgBoxed, _, err := sender.Send(ctx, res.ConvID, chat1.MessagePlaintext{
		ClientHeader: chat1.MessageClientHeader{
			Conv:        trip,
			Sender:      u.User.GetUID().ToBytes(),
			TlfName:     u.Username,
			TlfPublic:   false,
			MessageType: chat1.MessageType_TEXT,
		},
		MessageBody: chat1.NewMessageBodyWithText(chat1.MessageText{
			Body: "HIHI",
		}),
	}, 0, nil)
	require.NoError(t, err)
	msgID := msgBoxed.GetMessageID()
	thread, _, err := tc.ChatG.ConvSource.Pull(ctx, res.ConvID, u.User.GetUID().ToBytes(),
		&chat1.GetThreadQuery{
			MessageTypes: []chat1.MessageType{chat1.MessageType_TEXT},
		}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(thread.Messages), "wrong length")
	require.Equal(t, msgID, thread.Messages[0].GetMessageID(), "wrong msgID")

	_, editMsgBoxed, _, err := sender.Send(ctx, res.ConvID, chat1.MessagePlaintext{
		ClientHeader: chat1.MessageClientHeader{
			Conv:        trip,
			Sender:      u.User.GetUID().ToBytes(),
			TlfName:     u.Username,
			TlfPublic:   false,
			MessageType: chat1.MessageType_EDIT,
			Supersedes:  msgID,
		},
		MessageBody: chat1.NewMessageBodyWithEdit(chat1.MessageEdit{
			MessageID: msgID,
			Body:      "EDITED",
		}),
	}, 0, nil)
	require.NoError(t, err)
	editMsgID := editMsgBoxed.GetMessageID()

	t.Logf("testing an edit")
	thread, _, err = tc.ChatG.ConvSource.Pull(ctx, res.ConvID, u.User.GetUID().ToBytes(),
		&chat1.GetThreadQuery{
			MessageTypes: []chat1.MessageType{chat1.MessageType_TEXT},
		}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(thread.Messages), "wrong length")
	require.Equal(t, msgID, thread.Messages[0].GetMessageID(), "wrong msgID")
	require.Equal(t, editMsgID, thread.Messages[0].Valid().ServerHeader.SupersededBy, "wrong super")
	require.Equal(t, "EDITED", thread.Messages[0].Valid().MessageBody.Text().Body, "wrong body")

	t.Logf("testing a delete")
	delTyp := chat1.MessageType_DELETE
	delBody := chat1.NewMessageBodyWithDelete(chat1.MessageDelete{
		MessageIDs: []chat1.MessageID{msgID, editMsgID},
	})
	delSupersedes := msgID
	var delHeader *chat1.MessageDeleteHistory
	if deleteHistory {
		delTyp = chat1.MessageType_DELETEHISTORY
		delHeader = &chat1.MessageDeleteHistory{
			Upto: editMsgID + 1,
		}
		delBody = chat1.NewMessageBodyWithDeletehistory(*delHeader)
		delSupersedes = 0
	}
	_, deleteMsgBoxed, _, err := sender.Send(ctx, res.ConvID, chat1.MessagePlaintext{
		ClientHeader: chat1.MessageClientHeader{
			Conv:          trip,
			Sender:        u.User.GetUID().ToBytes(),
			TlfName:       u.Username,
			TlfPublic:     false,
			MessageType:   delTyp,
			Supersedes:    delSupersedes,
			DeleteHistory: delHeader,
		},
		MessageBody: delBody,
	}, 0, nil)
	require.NoError(t, err)
	deleteMsgID := deleteMsgBoxed.GetMessageID()
	thread, _, err = tc.ChatG.ConvSource.Pull(ctx, res.ConvID, u.User.GetUID().ToBytes(),
		&chat1.GetThreadQuery{
			MessageTypes: []chat1.MessageType{chat1.MessageType_TEXT},
		}, nil)
	require.NoError(t, err)
	require.Equal(t, 0, len(thread.Messages), "wrong length")

	t.Logf("testing disabling resolve")
	thread, _, err = tc.ChatG.ConvSource.Pull(ctx, res.ConvID, u.User.GetUID().ToBytes(),
		&chat1.GetThreadQuery{
			MessageTypes: []chat1.MessageType{
				chat1.MessageType_TEXT,
				chat1.MessageType_EDIT,
				chat1.MessageType_DELETE,
				chat1.MessageType_DELETEHISTORY,
			},
			DisableResolveSupersedes: true,
		}, nil)
	require.NoError(t, err)
	require.Equal(t, 3, len(thread.Messages), "wrong length")
	require.Equal(t, msgID, thread.Messages[2].GetMessageID(), "wrong msgID")
	require.Equal(t, deleteMsgID, thread.Messages[2].Valid().ServerHeader.SupersededBy, "wrong super")
}

type failingRemote struct {
	t *testing.T
}

func newFailingRemote(t *testing.T) failingRemote {
	return failingRemote{
		t: t,
	}
}

var _ chat1.RemoteInterface = (*failingRemote)(nil)

func (f failingRemote) GetInboxRemote(context.Context, chat1.GetInboxRemoteArg) (chat1.GetInboxRemoteRes, error) {
	require.Fail(f.t, "GetInboxRemote call")
	return chat1.GetInboxRemoteRes{}, nil
}
func (f failingRemote) GetThreadRemote(context.Context, chat1.GetThreadRemoteArg) (chat1.GetThreadRemoteRes, error) {

	require.Fail(f.t, "GetThreadRemote call")
	return chat1.GetThreadRemoteRes{}, nil
}
func (f failingRemote) GetPublicConversations(context.Context, chat1.GetPublicConversationsArg) (chat1.GetPublicConversationsRes, error) {
	require.Fail(f.t, "GetPublicConversations call")
	return chat1.GetPublicConversationsRes{}, nil
}
func (f failingRemote) PostRemote(context.Context, chat1.PostRemoteArg) (chat1.PostRemoteRes, error) {

	require.Fail(f.t, "PostRemote call")
	return chat1.PostRemoteRes{}, nil
}
func (f failingRemote) NewConversationRemote(context.Context, chat1.ConversationIDTriple) (chat1.NewConversationRemoteRes, error) {

	require.Fail(f.t, "NewConversationRemote call")
	return chat1.NewConversationRemoteRes{}, nil
}
func (f failingRemote) NewConversationRemote2(context.Context, chat1.NewConversationRemote2Arg) (chat1.NewConversationRemoteRes, error) {

	require.Fail(f.t, "NewConversationRemote2 call")
	return chat1.NewConversationRemoteRes{}, nil
}
func (f failingRemote) GetMessagesRemote(context.Context, chat1.GetMessagesRemoteArg) (chat1.GetMessagesRemoteRes, error) {

	require.Fail(f.t, "GetMessagesRemote call")
	return chat1.GetMessagesRemoteRes{}, nil
}
func (f failingRemote) MarkAsRead(context.Context, chat1.MarkAsReadArg) (chat1.MarkAsReadRes, error) {

	require.Fail(f.t, "MarkAsRead call")
	return chat1.MarkAsReadRes{}, nil
}
func (f failingRemote) SetConversationStatus(context.Context, chat1.SetConversationStatusArg) (chat1.SetConversationStatusRes, error) {

	require.Fail(f.t, "SetConversationStatus call")
	return chat1.SetConversationStatusRes{}, nil
}
func (f failingRemote) SetAppNotificationSettings(context.Context, chat1.SetAppNotificationSettingsArg) (chat1.SetAppNotificationSettingsRes, error) {
	require.Fail(f.t, "SetAppNotificationSettings call")
	return chat1.SetAppNotificationSettingsRes{}, nil
}
func (f failingRemote) GetUnreadUpdateFull(context.Context, chat1.InboxVers) (chat1.UnreadUpdateFull, error) {

	require.Fail(f.t, "GetUnreadUpdateFull call")
	return chat1.UnreadUpdateFull{}, nil
}
func (f failingRemote) GetS3Params(context.Context, chat1.ConversationID) (chat1.S3Params, error) {

	require.Fail(f.t, "GetS3Params call")
	return chat1.S3Params{}, nil
}
func (f failingRemote) S3Sign(context.Context, chat1.S3SignArg) ([]byte, error) {

	require.Fail(f.t, "S3Sign call")
	return nil, nil
}
func (f failingRemote) GetInboxVersion(context.Context, gregor1.UID) (chat1.InboxVers, error) {

	require.Fail(f.t, "GetInboxVersion call")
	return chat1.InboxVers(0), nil
}
func (f failingRemote) TlfFinalize(context.Context, chat1.TlfFinalizeArg) error {

	require.Fail(f.t, "TlfFinalize call")
	return nil
}
func (f failingRemote) TlfResolve(context.Context, chat1.TlfResolveArg) error {

	require.Fail(f.t, "TlfResolve call")
	return nil
}
func (f failingRemote) PublishReadMessage(context.Context, chat1.PublishReadMessageArg) error {

	require.Fail(f.t, "PublishReadMessage call")
	return nil
}
func (f failingRemote) PublishSetConversationStatus(context.Context, chat1.PublishSetConversationStatusArg) error {

	require.Fail(f.t, "PublicSetConversationStatus call")
	return nil
}
func (f failingRemote) SyncInbox(ctx context.Context, vers chat1.InboxVers) (chat1.SyncInboxRes, error) {
	require.Fail(f.t, "SyncInbox")
	return chat1.SyncInboxRes{}, nil
}

func (f failingRemote) SyncChat(ctx context.Context, vers chat1.InboxVers) (chat1.SyncChatRes, error) {
	require.Fail(f.t, "SyncChat")
	return chat1.SyncChatRes{}, nil
}

func (f failingRemote) SyncAll(ctx context.Context, arg chat1.SyncAllArg) (chat1.SyncAllResult, error) {
	require.Fail(f.t, "SyncAll")
	return chat1.SyncAllResult{}, nil
}

func (f failingRemote) UpdateTypingRemote(ctx context.Context, arg chat1.UpdateTypingRemoteArg) error {
	require.Fail(f.t, "UpdateTypingRemote")
	return nil
}

func (f failingRemote) GetTLFConversations(ctx context.Context, arg chat1.GetTLFConversationsArg) (chat1.GetTLFConversationsRes, error) {
	require.Fail(f.t, "GetTLFConversations")
	return chat1.GetTLFConversationsRes{}, nil
}

func (f failingRemote) JoinConversation(ctx context.Context, convID chat1.ConversationID) (chat1.JoinLeaveConversationRemoteRes, error) {
	require.Fail(f.t, "JoinConversation")
	return chat1.JoinLeaveConversationRemoteRes{}, nil
}

func (f failingRemote) LeaveConversation(ctx context.Context, convID chat1.ConversationID) (chat1.JoinLeaveConversationRemoteRes, error) {
	require.Fail(f.t, "LeaveConversation")
	return chat1.JoinLeaveConversationRemoteRes{}, nil
}

func (f failingRemote) PreviewConversation(ctx context.Context, convID chat1.ConversationID) (chat1.JoinLeaveConversationRemoteRes, error) {
	require.Fail(f.t, "PreviewConversation")
	return chat1.JoinLeaveConversationRemoteRes{}, nil
}

func (f failingRemote) DeleteConversation(ctx context.Context, convID chat1.ConversationID) (chat1.DeleteConversationRemoteRes, error) {
	require.Fail(f.t, "DeleteConversation")
	return chat1.DeleteConversationRemoteRes{}, nil
}

func (f failingRemote) GetMessageBefore(ctx context.Context, arg chat1.GetMessageBeforeArg) (chat1.GetMessageBeforeRes, error) {
	require.Fail(f.t, "GetMessageBefore")
	return chat1.GetMessageBeforeRes{}, nil
}

func (f failingRemote) RemoteNotificationSuccessful(ctx context.Context,
	arg chat1.RemoteNotificationSuccessfulArg) error {
	require.Fail(f.t, "RemoteNotificationSuccessful")
	return nil
}

func (f failingRemote) SetGlobalAppNotificationSettings(ctx context.Context, arg chat1.GlobalAppNotificationSettings) error {
	require.Fail(f.t, "SetGlobalAppNotificationSettings")
	return nil
}

func (f failingRemote) GetGlobalAppNotificationSettings(ctx context.Context) (chat1.GlobalAppNotificationSettings, error) {
	require.Fail(f.t, "GetGlobalAppNotificationSettings")
	return chat1.GlobalAppNotificationSettings{}, nil
}

func (f failingRemote) SetConvRetention(ctx context.Context, _ chat1.SetConvRetentionArg) (res chat1.SetRetentionRes, err error) {
	require.Fail(f.t, "SetConvRetention")
	return res, errors.New("SetConvRetention not mocked")
}

func (f failingRemote) SetTeamRetention(ctx context.Context, _ chat1.SetTeamRetentionArg) (res chat1.SetRetentionRes, err error) {
	require.Fail(f.t, "SetTeamRetention")
	return res, errors.New("SetTeamRetention not mocked")
}

func (f failingRemote) RetentionSweepConv(ctx context.Context, convID chat1.ConversationID) (res chat1.SweepRes, err error) {
	require.Fail(f.t, "UpgradeKBFSToImpteam")
	return res, nil
}

func (f failingRemote) UpgradeKBFSToImpteam(ctx context.Context, tlfID chat1.TLFID) error {
	require.Fail(f.t, "UpgradeKBFSToImpteam")
	return nil
}

type failingTlf struct {
	t *testing.T
}

func newFailingTlf(t *testing.T) failingTlf {
	return failingTlf{
		t: t,
	}
}

func (f failingTlf) CryptKeys(context.Context, string) (keybase1.GetTLFCryptKeysRes, error) {
	require.Fail(f.t, "CryptKeys call")
	return keybase1.GetTLFCryptKeysRes{}, nil
}

func (f failingTlf) PublicCanonicalTLFNameAndID(context.Context, string) (keybase1.CanonicalTLFNameAndIDWithBreaks, error) {
	require.Fail(f.t, "PublicCanonicalTLFNameAndID call")
	return keybase1.CanonicalTLFNameAndIDWithBreaks{}, nil
}

func (f failingTlf) CompleteAndCanonicalizePrivateTlfName(context.Context, string) (keybase1.CanonicalTLFNameAndIDWithBreaks, error) {
	require.Fail(f.t, "CompleteAndCanonicalizePrivateTlfName call")
	return keybase1.CanonicalTLFNameAndIDWithBreaks{}, nil
}

func (f failingTlf) Lookup(context.Context, string, bool) (*types.NameInfo, error) {
	require.Fail(f.t, "Lookup call")
	return nil, nil
}

func (f failingTlf) EncryptionKeys(ctx context.Context, tlfName string, tlfID chat1.TLFID,
	membersType chat1.ConversationMembersType, public bool) (*types.NameInfo, error) {
	return f.Lookup(ctx, tlfName, public)
}

func (f failingTlf) DecryptionKeys(ctx context.Context, tlfName string, tlfID chat1.TLFID,
	membersType chat1.ConversationMembersType, public bool,
	keyGeneration int, kbfsEncrypted bool) (*types.NameInfo, error) {
	return f.Lookup(ctx, tlfName, public)
}

type failingUpak struct {
	t *testing.T
}

func newFailingUpak(t *testing.T) failingUpak {
	return failingUpak{
		t: t,
	}
}

func (f failingUpak) ClearMemory() {
	require.Fail(f.t, "ClearMemory call")
}
func (f failingUpak) Load(arg libkb.LoadUserArg) (ret *keybase1.UserPlusAllKeys, user *libkb.User, err error) {
	require.Fail(f.t, "Load call")
	return nil, nil, nil
}
func (f failingUpak) LoadV2(arg libkb.LoadUserArg) (ret *keybase1.UserPlusKeysV2AllIncarnations, user *libkb.User, err error) {
	require.Fail(f.t, "LoadV2 call")
	return nil, nil, nil
}
func (f failingUpak) LoadKeyV2(ctx context.Context, uid keybase1.UID, kid keybase1.KID) (*keybase1.UserPlusKeysV2, *keybase1.PublicKeyV2NaCl, map[keybase1.Seqno]keybase1.LinkID, error) {
	require.Fail(f.t, "LoadKeyV2")
	return nil, nil, nil, nil
}
func (f failingUpak) CheckKIDForUID(ctx context.Context, uid keybase1.UID, kid keybase1.KID) (found bool, revokedAt *keybase1.KeybaseTime, deleted bool, err error) {
	require.Fail(f.t, "ChceckKIDForUID call")
	return false, nil, false, nil
}
func (f failingUpak) LoadUserPlusKeys(ctx context.Context, uid keybase1.UID, pollForKID keybase1.KID) (keybase1.UserPlusKeys, error) {
	require.Fail(f.t, "LoadUserPlusKeys call")
	return keybase1.UserPlusKeys{}, nil
}
func (f failingUpak) Invalidate(ctx context.Context, uid keybase1.UID) {
	require.Fail(f.t, "Invalidate call")
}
func (f failingUpak) LoadDeviceKey(ctx context.Context, uid keybase1.UID, deviceID keybase1.DeviceID) (upk *keybase1.UserPlusAllKeys, deviceKey *keybase1.PublicKey, revoked *keybase1.RevokedKey, err error) {
	require.Fail(f.t, "LoadDeviceKey call")
	return nil, nil, nil, nil
}
func (f failingUpak) LookupUsername(ctx context.Context, uid keybase1.UID) (libkb.NormalizedUsername, error) {
	require.Fail(f.t, "LookupUsername call")
	return "", nil
}
func (f failingUpak) LookupUsernameUPAK(ctx context.Context, uid keybase1.UID) (libkb.NormalizedUsername, error) {
	require.Fail(f.t, "LookupUsernameUPAK call")
	return "", nil
}
func (f failingUpak) LookupUID(ctx context.Context, un libkb.NormalizedUsername) (keybase1.UID, error) {
	require.Fail(f.t, "LookupUID call")
	return keybase1.UID(""), nil
}
func (f failingUpak) LookupUsernameAndDevice(ctx context.Context, uid keybase1.UID, did keybase1.DeviceID) (username libkb.NormalizedUsername, deviceName string, deviceType string, err error) {
	require.Fail(f.t, "LookupUsernameAndDevice call")
	return "", "", "", nil
}
func (f failingUpak) ListFollowedUIDs(uid keybase1.UID) ([]keybase1.UID, error) {
	require.Fail(f.t, "ListFollowedUIDs call")
	return nil, nil
}
func (f failingUpak) PutUserToCache(ctx context.Context, user *libkb.User) error {
	require.Fail(f.t, "PutUserToCache call")
	return nil
}

func TestGetThreadCaching(t *testing.T) {
	ctx, world, ri, _, sender, _ := setupTest(t, 1)
	defer world.Cleanup()

	u := world.GetUsers()[0]
	tc := world.Tcs[u.Username]
	trip := newConvTriple(ctx, t, tc, u.Username)
	firstMessagePlaintext := chat1.MessagePlaintext{
		ClientHeader: chat1.MessageClientHeader{
			Conv:        trip,
			TlfName:     u.Username,
			TlfPublic:   false,
			MessageType: chat1.MessageType_TLFNAME,
		},
		MessageBody: chat1.MessageBody{},
	}
	firstMessageBoxed, _, _, _, _, err := sender.Prepare(ctx, firstMessagePlaintext,
		chat1.ConversationMembersType_KBFS, nil)
	require.NoError(t, err)
	res, err := ri.NewConversationRemote2(ctx, chat1.NewConversationRemote2Arg{
		IdTriple:   trip,
		TLFMessage: *firstMessageBoxed,
	})
	require.NoError(t, err)

	_, msgBoxed, _, err := sender.Send(ctx, res.ConvID, chat1.MessagePlaintext{
		ClientHeader: chat1.MessageClientHeader{
			Conv:        trip,
			Sender:      u.User.GetUID().ToBytes(),
			TlfName:     u.Username,
			TlfPublic:   false,
			MessageType: chat1.MessageType_TEXT,
		},
		MessageBody: chat1.NewMessageBodyWithText(chat1.MessageText{
			Body: "HIHI",
		}),
	}, 0, nil)
	require.NoError(t, err)
	msgID := msgBoxed.GetMessageID()

	tc.ChatG.ConvSource.Clear(res.ConvID, u.User.GetUID().ToBytes())
	tc.ChatG.ConvSource.Disconnected(ctx)
	tc.ChatG.InboxSource.Disconnected(ctx)
	t.Logf("make sure we get offline error")
	thread, _, err := tc.ChatG.ConvSource.Pull(ctx, res.ConvID, u.User.GetUID().ToBytes(),
		&chat1.GetThreadQuery{
			MessageTypes: []chat1.MessageType{chat1.MessageType_TEXT},
		}, nil)
	require.Error(t, err)
	require.IsType(t, OfflineError{}, err, "wrong error type")

	t.Logf("read to populate caches")
	tc.ChatG.ConvSource.Connected(ctx)
	tc.ChatG.InboxSource.Connected(ctx)
	thread, _, err = tc.ChatG.ConvSource.Pull(ctx, res.ConvID, u.User.GetUID().ToBytes(),
		&chat1.GetThreadQuery{
			MessageTypes: []chat1.MessageType{chat1.MessageType_TEXT},
		}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(thread.Messages), "wrong length")
	require.Equal(t, msgID, thread.Messages[0].GetMessageID(), "wrong msgID")

	t.Logf("reading thread again for total cache hit")
	failingRI := newFailingRemote(t)
	failingTI := newFailingTlf(t)
	tc.ChatG.ConvSource.Disconnected(ctx)
	tc.ChatG.InboxSource.Disconnected(ctx)
	tc.ChatG.ConvSource.SetRemoteInterface(func() chat1.RemoteInterface { return failingRI })
	tc.ChatG.InboxSource.SetRemoteInterface(func() chat1.RemoteInterface { return failingRI })

	tc.G.OverrideUPAKLoader(newFailingUpak(t))

	ctx = newTestContextWithTlfMock(tc, failingTI)
	thread, _, err = tc.ChatG.ConvSource.Pull(ctx, res.ConvID, u.User.GetUID().ToBytes(),
		&chat1.GetThreadQuery{
			MessageTypes: []chat1.MessageType{chat1.MessageType_TEXT},
		}, nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(thread.Messages), "wrong length")
	require.Equal(t, msgID, thread.Messages[0].GetMessageID(), "wrong msgID")
}

type noGetThreadRemote struct {
	*kbtest.ChatRemoteMock
}

func newNoGetThreadRemote(mock *kbtest.ChatRemoteMock) *noGetThreadRemote {
	return &noGetThreadRemote{
		ChatRemoteMock: mock,
	}
}

func (n *noGetThreadRemote) GetThreadRemote(ctx context.Context, arg chat1.GetThreadRemoteArg) (chat1.GetThreadRemoteRes, error) {
	return chat1.GetThreadRemoteRes{}, errors.New("GetThreadRemote banned")
}

func TestGetThreadHoleResolution(t *testing.T) {
	ctx, world, ri2, _, sender, _ := setupTest(t, 1)
	defer world.Cleanup()

	ri := ri2.(*kbtest.ChatRemoteMock)
	u := world.GetUsers()[0]
	uid := u.User.GetUID().ToBytes()
	tc := world.Tcs[u.Username]
	syncer := NewSyncer(tc.Context())
	syncer.isConnected = true

	conv := newConv(ctx, t, tc, uid, ri, sender, u.Username)
	convID := conv.GetConvID()
	pt := chat1.MessagePlaintext{
		ClientHeader: chat1.MessageClientHeader{
			Conv:        conv.Metadata.IdTriple,
			Sender:      u.User.GetUID().ToBytes(),
			TlfName:     u.Username,
			TlfPublic:   false,
			MessageType: chat1.MessageType_TEXT,
		},
		MessageBody: chat1.NewMessageBodyWithText(chat1.MessageText{
			Body: "HIHI",
		}),
	}

	var msg *chat1.MessageBoxed
	var err error
	holes := 3
	for i := 0; i < holes; i++ {
		pt.MessageBody = chat1.NewMessageBodyWithText(chat1.MessageText{
			Body: fmt.Sprintf("MIKE: %d", i),
		})
		msg, _, _, _, _, err = sender.Prepare(ctx, pt, chat1.ConversationMembersType_KBFS, &conv)
		require.NoError(t, err)
		require.NotNil(t, msg)

		res, err := ri.PostRemote(ctx, chat1.PostRemoteArg{
			ConversationID: conv.GetConvID(),
			MessageBoxed:   *msg,
		})
		require.NoError(t, err)
		msg.ServerHeader = &res.MsgHeader
	}

	conv.MaxMsgs = []chat1.MessageBoxed{*msg}
	conv.MaxMsgSummaries = []chat1.MessageSummary{msg.Summary()}
	conv.ReaderInfo.MaxMsgid = msg.GetMessageID()
	ri.SyncInboxFunc = func(m *kbtest.ChatRemoteMock, ctx context.Context, vers chat1.InboxVers) (chat1.SyncInboxRes, error) {
		return chat1.NewSyncInboxResWithIncremental(chat1.SyncIncrementalRes{
			Vers:  vers + 1,
			Convs: []chat1.Conversation{conv},
		}), nil
	}
	doSync(t, syncer, ri, uid)

	localThread, err := tc.Context().ConvSource.PullLocalOnly(ctx, convID, uid, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 2, len(localThread.Messages))

	tc.Context().ConvSource.SetRemoteInterface(func() chat1.RemoteInterface {
		return newNoGetThreadRemote(ri)
	})
	thread, _, err := tc.Context().ConvSource.Pull(ctx, convID, uid, nil, nil)
	require.NoError(t, err)
	require.Equal(t, holes+2, len(thread.Messages))
	require.Equal(t, msg.GetMessageID(), thread.Messages[0].GetMessageID())
	require.Equal(t, "MIKE: 2", thread.Messages[0].Valid().MessageBody.Text().Body)

	// Make sure we don't consider it a hit if we end the fetch with a hole
	require.NoError(t, tc.Context().ConvSource.Clear(convID, uid))
	_, _, err = tc.Context().ConvSource.Pull(ctx, convID, uid, nil, nil)
	require.Error(t, err)
}

func TestConversationLocking(t *testing.T) {
	ctx, world, ri2, _, sender, _ := setupTest(t, 1)
	defer world.Cleanup()

	ri := ri2.(*kbtest.ChatRemoteMock)
	u := world.GetUsers()[0]
	uid := u.User.GetUID().ToBytes()
	tc := world.Tcs[u.Username]
	syncer := NewSyncer(tc.Context())
	syncer.isConnected = true
	hcs := tc.Context().ConvSource.(*HybridConversationSource)
	if hcs == nil {
		t.Skip()
	}

	timedAcquire := func(ctx context.Context, uid gregor1.UID, convID chat1.ConversationID) (ret bool) {
		cb := make(chan struct{})
		go func() {
			ret = hcs.lockTab.Acquire(ctx, uid, convID)
			close(cb)
		}()
		select {
		case <-cb:
		case <-time.After(20 * time.Second):
			require.Fail(t, "acquire timeout")
		}
		return ret
	}
	conv := newConv(ctx, t, tc, uid, ri, sender, u.Username)

	t.Logf("Trace 1 can get multiple locks")
	var breaks []keybase1.TLFIdentifyFailure
	ctx = Context(context.TODO(), tc.Context(), keybase1.TLFIdentifyBehavior_CHAT_CLI, &breaks,
		NewCachingIdentifyNotifier(tc.Context()))
	acquires := 5
	for i := 0; i < acquires; i++ {
		timedAcquire(ctx, uid, conv.GetConvID())
	}
	for i := 0; i < acquires; i++ {
		hcs.lockTab.Release(ctx, uid, conv.GetConvID())
	}
	require.Zero(t, len(hcs.lockTab.convLocks))

	t.Logf("Trace 2 properly blocked by Trace 1")
	ctx2 := Context(context.TODO(), tc.Context(), keybase1.TLFIdentifyBehavior_CHAT_CLI,
		&breaks, NewCachingIdentifyNotifier(tc.Context()))
	blockCb := make(chan struct{})
	hcs.lockTab.blockCb = &blockCb
	cb := make(chan struct{})
	require.False(t, timedAcquire(ctx, uid, conv.GetConvID()))
	go func() {
		require.True(t, timedAcquire(ctx2, uid, conv.GetConvID()))
		close(cb)
	}()
	select {
	case <-cb:
		require.Fail(t, "should have blocked")
	default:
	}
	// Wait for the thread to get blocked
	select {
	case <-blockCb:
	case <-time.After(20 * time.Second):
		require.Fail(t, "not blocked")
	}
	require.True(t, hcs.lockTab.Release(ctx, uid, conv.GetConvID()))
	select {
	case <-cb:
	case <-time.After(20 * time.Second):
		require.Fail(t, "no cb")
	}
	require.True(t, hcs.lockTab.Release(ctx2, uid, conv.GetConvID()))
	require.Zero(t, len(hcs.lockTab.convLocks))

	t.Logf("No trace")
	require.False(t, timedAcquire(context.TODO(), uid, conv.GetConvID()))
	require.False(t, timedAcquire(context.TODO(), uid, conv.GetConvID()))
	require.Zero(t, len(hcs.lockTab.convLocks))
}
