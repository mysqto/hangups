package hangups

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/golang/protobuf/proto"
	hangouts "github.com/mysqto/hangups/proto"
	"github.com/tidwall/gjson"
)

const (
	imageUploadURL = "https://docs.google.com/upload/photos/resumable"
)

// Client is a hangouts client
type Client struct {
	Session   *Session
	ClientID  string
	UserAgent string
}

// getMessageContent creates a new MessageContent with content
func getMessageContent(content string) *hangouts.MessageContent {

	if len(content) == 0 {
		return nil
	}

	// TODO split it on TEXT, LINE_BREAK and LINK
	segmentType := hangouts.SegmentType_SEGMENT_TYPE_TEXT

	linkData := &hangouts.LinkData{}
	// check if it is a link
	if govalidator.IsURL(content) {
		segmentType = hangouts.SegmentType_SEGMENT_TYPE_LINK
		linkData.LinkTarget = proto.String(content)
	}

	return &hangouts.MessageContent{
		Segment: []*hangouts.Segment{
			{
				Type:       &segmentType,
				Text:       proto.String(content),
				Formatting: &hangouts.Formatting{},
				LinkData:   linkData,
			},
		},
		Attachment: nil,
	}
}

// getExistingMedia creates a new ExistingMedia with imageID
func getExistingMedia(imageID string) *hangouts.ExistingMedia {
	if len(imageID) > 0 {
		return &hangouts.ExistingMedia{
			Photo: &hangouts.Photo{
				PhotoId: proto.String(imageID),
			},
		}
	}
	return nil
}

// initialize random number generator needed for client id
func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func getLookupSpec(id string) *hangouts.EntityLookupSpec {
	if id[0] == '+' {
		return &hangouts.EntityLookupSpec{
			Phone:                &id,
			CreateOffnetworkGaia: proto.Bool(true),
		}
	} else if govalidator.IsEmail(id) {
		return &hangouts.EntityLookupSpec{
			Email:                &id,
			CreateOffnetworkGaia: proto.Bool(true),
		}
	}
	return &hangouts.EntityLookupSpec{GaiaId: &id}
}

// NewRequestHeaders creates basic request header
func (c *Client) NewRequestHeaders() *hangouts.RequestHeader {
	version := "hangups-0.0.1"
	language := "en"

	return &hangouts.RequestHeader{
		ClientVersion:    &hangouts.ClientVersion{MajorVersion: &version},
		ClientIdentifier: &hangouts.ClientIdentifier{Resource: nil}, //use request_header.client_identifier.resource
		LanguageCode:     &language,
	}
}

// NewEventRequestHeaders creates basic event request header
func (c *Client) NewEventRequestHeaders(conversationID string, offTheRecord bool,
	deliveryMedium hangouts.DeliveryMediumType) *hangouts.EventRequestHeader {

	expectedOtr := hangouts.OffTheRecordStatus_OFF_THE_RECORD_STATUS_ON_THE_RECORD
	if offTheRecord {
		expectedOtr = hangouts.OffTheRecordStatus_OFF_THE_RECORD_STATUS_OFF_THE_RECORD
	}

	// needs to be unique every time
	clientGeneratedID := uint64(rand.Uint32())
	eventType := hangouts.EventType_EVENT_TYPE_REGULAR_CHAT_MESSAGE
	return &hangouts.EventRequestHeader{
		ConversationId:    &hangouts.ConversationId{Id: &conversationID},
		ClientGeneratedId: &clientGeneratedID,
		ExpectedOtr:       &expectedOtr,
		DeliveryMedium:    &hangouts.DeliveryMedium{MediumType: &deliveryMedium},
		EventType:         &eventType,
	}
}

// ProtobufAPIRequest do a protobuf API request
func (c *Client) ProtobufAPIRequest(apiEndpoint string, requestStruct, responseStruct proto.Message) error {
	payload, err := proto.Marshal(requestStruct)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://clients6.google.com/chat/v1/%s", apiEndpoint)
	headers := map[string]string{
		"Content-Type": "application/x-protobuf",
	}
	output, err := c.APIRequest(url, "proto", headers, payload)
	if err != nil {
		return err
	}

	decodedOutput, err := base64.StdEncoding.DecodeString(string(output))
	if err != nil {
		return err
	}

	err = proto.Unmarshal(decodedOutput, responseStruct)
	if err != nil {
		return err
	}

	return nil
}

//AddUser Invite users to join an existing group conversation.
func (c *Client) AddUser(inviteesGaiaIds []string, conversationID string) (*hangouts.AddUserResponse, error) {
	inviteeIds := make([]*hangouts.InviteeID, len(inviteesGaiaIds))
	for ind, gaiaID := range inviteesGaiaIds {
		inviteeIds[ind] = &hangouts.InviteeID{GaiaId: &gaiaID}
	}

	request := &hangouts.AddUserRequest{
		RequestHeader:      c.NewRequestHeaders(),
		InviteeId:          inviteeIds,
		EventRequestHeader: c.NewEventRequestHeaders(conversationID, false, hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL),
	}
	response := &hangouts.AddUserResponse{}
	err := c.ProtobufAPIRequest("conversations/adduser", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

//CreateConversation Create a new conversation.
func (c *Client) CreateConversation(inviteesGaiaIds []string, name string, oneOnOne bool) (*hangouts.CreateConversationResponse, error) {
	inviteeIds := make([]*hangouts.InviteeID, len(inviteesGaiaIds))
	for ind, gaiaID := range inviteesGaiaIds {
		inviteeIds[ind] = &hangouts.InviteeID{GaiaId: &gaiaID}
	}

	conversationType := hangouts.ConversationType_CONVERSATION_TYPE_GROUP
	if oneOnOne {
		conversationType = hangouts.ConversationType_CONVERSATION_TYPE_ONE_TO_ONE
	}
	clientGeneratedID := uint64(rand.Uint32())
	request := &hangouts.CreateConversationRequest{
		RequestHeader:     c.NewRequestHeaders(),
		InviteeId:         inviteeIds,
		ClientGeneratedId: &clientGeneratedID,
		Name:              &name,
		Type:              &conversationType,
	}
	response := &hangouts.CreateConversationResponse{}
	err := c.ProtobufAPIRequest("conversations/createconversation", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

/*
DeleteConversation Leave a one-to-one conversation.
One-to-one conversations are "sticky"; they can't actually be deleted.
This API clears the event history of the specified conversation up to
delete_upper_bound_timestamp, hiding it if no events remain.
*/
func (c *Client) DeleteConversation(conversationID string, deleteUpperBoundTimestamp uint64) (*hangouts.DeleteConversationResponse, error) {
	request := &hangouts.DeleteConversationRequest{
		RequestHeader:             c.NewRequestHeaders(),
		ConversationId:            &hangouts.ConversationId{Id: &conversationID},
		DeleteUpperBoundTimestamp: &deleteUpperBoundTimestamp,
	}
	response := &hangouts.DeleteConversationResponse{}
	err := c.ProtobufAPIRequest("conversations/deleteconversation", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// EasterEgg send an easter egg event to a conversation.
func (c *Client) EasterEgg(conversationID string, message string) (*hangouts.EasterEggResponse, error) {
	request := &hangouts.EasterEggRequest{
		RequestHeader:  c.NewRequestHeaders(),
		ConversationId: &hangouts.ConversationId{Id: &conversationID},
		EasterEgg:      &hangouts.EasterEgg{Message: &message},
	}
	response := &hangouts.EasterEggResponse{}
	err := c.ProtobufAPIRequest("conversations/easteregg", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetConversation return conversation info and recent events.
func (c *Client) GetConversation(conversationID string, includeEvent bool, maxEventsPerConversation uint64) (*hangouts.GetConversationResponse, error) {
	request := &hangouts.GetConversationRequest{
		RequestHeader:            c.NewRequestHeaders(),
		ConversationSpec:         &hangouts.ConversationSpec{ConversationId: &hangouts.ConversationId{Id: &conversationID}},
		IncludeEvent:             &includeEvent,
		MaxEventsPerConversation: &maxEventsPerConversation,
	}
	response := &hangouts.GetConversationResponse{}
	err := c.ProtobufAPIRequest("conversations/getconversation", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetEntities returns entities of a specific id
func (c *Client) GetEntities(id string) (*hangouts.Entity, error) {
	response, err := c.GetEntityByID([]string{id})
	if err != nil {
		return nil, err
	}

	return response.EntityResult[0].Entity[0], nil
}

// GetEntityByID return info about a list of users.
func (c *Client) GetEntityByID(Ids []string) (*hangouts.GetEntityByIdResponse, error) {
	batchLookupSpec := make([]*hangouts.EntityLookupSpec, len(Ids))
	for ind, id := range Ids {
		batchLookupSpec[ind] = getLookupSpec(id)
	}
	request := &hangouts.GetEntityByIdRequest{
		RequestHeader:   c.NewRequestHeaders(),
		BatchLookupSpec: batchLookupSpec,
	}
	response := &hangouts.GetEntityByIdResponse{}
	err := c.ProtobufAPIRequest("contacts/getentitybyid", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetSelfInfo return info about the current user.
func (c *Client) GetSelfInfo() (*hangouts.GetSelfInfoResponse, error) {
	request := &hangouts.GetSelfInfoRequest{
		RequestHeader: c.NewRequestHeaders(),
	}
	response := &hangouts.GetSelfInfoResponse{}
	err := c.ProtobufAPIRequest("contacts/getselfinfo", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetSuggestedEntities return suggested contacts
func (c *Client) GetSuggestedEntities(maxCount uint64) (*hangouts.GetSuggestedEntitiesResponse, error) {
	request := &hangouts.GetSuggestedEntitiesRequest{
		RequestHeader: c.NewRequestHeaders(),
		MaxCount:      proto.Uint64(maxCount),
	}
	response := &hangouts.GetSuggestedEntitiesResponse{}
	err := c.ProtobufAPIRequest("contacts/getsuggestedentities", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// QueryPresence return presence status for a list of users.
// doesnt support passing an array of gaiaIds.
// fails with:
// {"status":4,"error_description":"Duplicate ParticipantIds in request"}
func (c *Client) QueryPresence(gaiaID string) (*hangouts.QueryPresenceResponse, error) {
	request := &hangouts.QueryPresenceRequest{
		RequestHeader: c.NewRequestHeaders(),
		ParticipantId: []*hangouts.ParticipantId{&hangouts.ParticipantId{GaiaId: &gaiaID, ChatId: &gaiaID}},
		FieldMask:     []hangouts.FieldMask{hangouts.FieldMask_FIELD_MASK_REACHABLE, hangouts.FieldMask_FIELD_MASK_AVAILABLE, hangouts.FieldMask_FIELD_MASK_DEVICE},
	}
	response := &hangouts.QueryPresenceResponse{}
	err := c.ProtobufAPIRequest("presence/querypresence", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// RemoveUser leave a group conversation.
func (c *Client) RemoveUser(conversationID string) (*hangouts.RemoveUserResponse, error) {
	request := &hangouts.RemoveUserRequest{
		RequestHeader:      c.NewRequestHeaders(),
		EventRequestHeader: c.NewEventRequestHeaders(conversationID, false, hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL),
	}
	response := &hangouts.RemoveUserResponse{}
	err := c.ProtobufAPIRequest("conversations/removeuser", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetGroupConversationURL get the group conversation join URL
func (c *Client) GetGroupConversationURL(conversationID string) (string, error) {
	request := &hangouts.GetGroupConversationUrlRequest{
		RequestHeader:  c.NewRequestHeaders(),
		ConversationId: &hangouts.ConversationId{Id: proto.String(conversationID)},
	}
	response := &hangouts.GetGroupConversationUrlResponse{}
	err := c.ProtobufAPIRequest("conversations/getgroupconversationurl", request, response)
	if err != nil {
		return "", err
	}
	return response.GetGroupConversationUrl(), nil
}

// ModifyOTRStatus modify the otrstatus of current conversation
func (c *Client) ModifyOTRStatus(conversationID string, otrStatus hangouts.OffTheRecordStatus) (*hangouts.ModifyOTRStatusResponse, error) {
	offTheRecord := otrStatus == hangouts.OffTheRecordStatus_OFF_THE_RECORD_STATUS_OFF_THE_RECORD
	request := &hangouts.ModifyOTRStatusRequest{
		RequestHeader:      c.NewRequestHeaders(),
		EventRequestHeader: c.NewEventRequestHeaders(conversationID, offTheRecord, hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL),
		OtrStatus:          &otrStatus,
	}
	response := &hangouts.ModifyOTRStatusResponse{}
	err := c.ProtobufAPIRequest("conversations/modifyotrstatus", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// RenameConversation rename a conversation.
// Both group and one-to-one conversations may be renamed, but the
// official Hangouts clients have mixed support for one-to-one
// conversations with custom names.
func (c *Client) RenameConversation(conversationID, newName string) (*hangouts.RenameConversationResponse, error) {
	request := &hangouts.RenameConversationRequest{
		RequestHeader:      c.NewRequestHeaders(),
		EventRequestHeader: c.NewEventRequestHeaders(conversationID, false, hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL),
		NewName:            &newName,
	}
	response := &hangouts.RenameConversationResponse{}
	err := c.ProtobufAPIRequest("conversations/renameconversation", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SearchEntities return info for users based on a query.
func (c *Client) SearchEntities(query string, maxCount uint64) (*hangouts.SearchEntitiesResponse, error) {
	request := &hangouts.SearchEntitiesRequest{
		RequestHeader: c.NewRequestHeaders(),
		Query:         &query,
		MaxCount:      &maxCount,
	}
	response := &hangouts.SearchEntitiesResponse{}
	err := c.ProtobufAPIRequest("contacts/searchentities", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SendMessage sends a message to a user/group
// to can be phoneNumber, email chatID/GaiaID or conversation ID
func (c *Client) SendMessage(to, content string) error {
	return c.sendMessage(to, content, "")
}

// sendMessage the actual send routine
func (c *Client) sendMessage(to, content, imageID string) error {
	conv, err := c.Create1On1Conversation(to)
	if err != nil {
		return errors.New("cannot determine one on one conversation for " + to)
	}
	resp, err := c.sendChatMessage(conv, content, imageID)

	if err != nil {
		return fmt.Errorf("fail to send message to %s, error : %v ", to, err)
	}

	if resp.GetResponseHeader().GetStatus() != hangouts.ResponseStatus_RESPONSE_STATUS_OK {
		return fmt.Errorf("fail to send message to %s, error : %v ", to, resp.GetResponseHeader().GetErrorDescription())
	}
	return nil
}

// Create1On1Conversation create an 1 on 1 private conversation
func (c *Client) Create1On1Conversation(id string) (*hangouts.Conversation, error) {
	chatID := id
	// email/phone/GaiaID
	if govalidator.IsEmail(id) || id[0] == '+' || govalidator.IsNumeric(id) {
		entity, err := c.GetEntities(id)
		if err != nil {
			return nil, fmt.Errorf("error getting entity for %s: %v", id, err)
		}
		chatID = *entity.Id.ChatId

		resp, err := c.CreateConversation([]string{chatID}, id, true)

		if err != nil {
			return nil, err
		}
		return resp.GetConversation(), nil
	}

	// try to search as a conversationID
	resp, err := c.GetConversation(id, false, 0)
	if err != nil {
		return nil, err
	}
	return resp.GetConversationState().GetConversation(), nil
}

// SendChatMessage send a chat message to a conversation, keep this for backward compatibility
// Deprecated: SendChatMessage is deprecated since it has some hardcoded parameters, use SendMessage instead
func (c *Client) SendChatMessage(conversationID, message string) (*hangouts.SendChatMessageResponse, error) {
	request := &hangouts.SendChatMessageRequest{
		RequestHeader:      c.NewRequestHeaders(),
		EventRequestHeader: c.NewEventRequestHeaders(conversationID, false, hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL),
		MessageContent:     getMessageContent(message),
	}
	return c.sendChatMessageRequest(request)

}

// SendChatImage send a image to a conversation, keep this for backward compatibility
// Deprecated: SendChatImage is deprecated since it has some hardcoded parameters, use SendMessage  instead
func (c *Client) SendChatImage(conversationID, imageID string) (*hangouts.SendChatMessageResponse, error) {
	request := &hangouts.SendChatMessageRequest{
		RequestHeader:      c.NewRequestHeaders(),
		EventRequestHeader: c.NewEventRequestHeaders(conversationID, false, hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL),
		ExistingMedia:      getExistingMedia(imageID),
	}
	return c.sendChatMessageRequest(request)
}

// sendChatMessage send a chat message to a conversation.
func (c *Client) sendChatMessage(conversation *hangouts.Conversation, content, imageID string) (*hangouts.SendChatMessageResponse, error) {
	offTheRecord := conversation.GetOtrStatus() == hangouts.OffTheRecordStatus_OFF_THE_RECORD_STATUS_OFF_THE_RECORD
	deliveryMediumOption := conversation.SelfConversationState.DeliveryMediumOption
	deliveryMediumType := hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL

	if len(deliveryMediumOption) > 0 {
		deliveryMediumType = *deliveryMediumOption[0].DeliveryMedium.MediumType
	}

	request := &hangouts.SendChatMessageRequest{
		RequestHeader:      c.NewRequestHeaders(),
		EventRequestHeader: c.NewEventRequestHeaders(*conversation.ConversationId.Id, offTheRecord, deliveryMediumType),
		MessageContent:     getMessageContent(content),
		ExistingMedia:      getExistingMedia(imageID),
	}
	return c.sendChatMessageRequest(request)
}

// sendChatMessageRequest actual send routine
func (c *Client) sendChatMessageRequest(request *hangouts.SendChatMessageRequest) (*hangouts.SendChatMessageResponse, error) {
	response := &hangouts.SendChatMessageResponse{}
	err := c.ProtobufAPIRequest("conversations/sendchatmessage", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SendOffNetworkInvitation send an invitation to a non-contact.
func (c *Client) SendOffNetworkInvitation(email string) (*hangouts.SendOffnetworkInvitationResponse, error) {
	addressType := hangouts.OffnetworkAddressType_OFFNETWORK_ADDRESS_TYPE_EMAIL
	request := &hangouts.SendOffnetworkInvitationRequest{
		RequestHeader: c.NewRequestHeaders(),
		InviteeAddress: &hangouts.OffnetworkAddress{
			Type:  &addressType,
			Email: &email,
		},
	}
	response := &hangouts.SendOffnetworkInvitationResponse{}
	err := c.ProtobufAPIRequest("devices/sendoffnetworkinvitation", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SetActiveClient set the active client. timeout is 120 secs in hangups
func (c *Client) SetActiveClient(email string, isActive bool, timeoutSecs uint64) (*hangouts.SetActiveClientResponse, error) {
	if c.ClientID == "" {
		return nil, errors.New("can't set active client without a ClientID")
	}
	emailAndResource := fmt.Sprintf("%s/%s", email, c.ClientID)

	request := &hangouts.SetActiveClientRequest{
		RequestHeader: c.NewRequestHeaders(),
		IsActive:      &isActive,
		FullJid:       &emailAndResource,
		TimeoutSecs:   &timeoutSecs,
	}
	response := &hangouts.SetActiveClientResponse{}
	err := c.ProtobufAPIRequest("clients/setactiveclient", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SetConversationNotificationLevel set the notification level of a conversation.
func (c *Client) SetConversationNotificationLevel(conversationID string,
	setQuiet bool) (*hangouts.SetConversationNotificationLevelResponse, error) {
	notificationLevel := hangouts.NotificationLevel_NOTIFICATION_LEVEL_RING
	if setQuiet {
		notificationLevel = hangouts.NotificationLevel_NOTIFICATION_LEVEL_QUIET
	}
	request := &hangouts.SetConversationNotificationLevelRequest{
		RequestHeader:  c.NewRequestHeaders(),
		Level:          &notificationLevel,
		ConversationId: &hangouts.ConversationId{Id: &conversationID},
	}
	response := &hangouts.SetConversationNotificationLevelResponse{}
	err := c.ProtobufAPIRequest("conversations/setconversationnotificationlevel", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SetFocus set focus to a conversation.
func (c *Client) SetFocus(conversationID string, unfocus bool, timeoutSecs uint32) (*hangouts.SetFocusResponse, error) {
	isFocused := hangouts.FocusType_FOCUS_TYPE_FOCUSED
	if unfocus {
		isFocused = hangouts.FocusType_FOCUS_TYPE_UNFOCUSED
	}
	request := &hangouts.SetFocusRequest{
		RequestHeader:  c.NewRequestHeaders(),
		ConversationId: &hangouts.ConversationId{Id: &conversationID},
		Type:           &isFocused,
		TimeoutSecs:    &timeoutSecs,
	}
	response := &hangouts.SetFocusResponse{}
	err := c.ProtobufAPIRequest("conversations/setfocus", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SetGroupLinkSharingEnabled enable/disable group link sharing
func (c *Client) SetGroupLinkSharingEnabled(conversationID string,
	status hangouts.GroupLinkSharingStatus) (*hangouts.SetGroupLinkSharingEnabledResponse, error) {

	request := &hangouts.SetGroupLinkSharingEnabledRequest{
		RequestHeader: c.NewRequestHeaders(),
		EventRequestHeader: c.NewEventRequestHeaders(conversationID, false,
			hangouts.DeliveryMediumType_DELIVERY_MEDIUM_BABEL),
		GroupLinkSharingStatus: &status,
	}
	response := &hangouts.SetGroupLinkSharingEnabledResponse{}
	err := c.ProtobufAPIRequest("conversations/setgrouplinksharingenabled", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SetPresence set the presence status.
// presenceState: 1=NONE 30=IDLE 40=ACTIVE
func (c *Client) SetPresence(presenceState int32, timeoutSecs uint64) (*hangouts.SetPresenceResponse, error) {
	request := &hangouts.SetPresenceRequest{
		RequestHeader: c.NewRequestHeaders(),
		PresenceStateSetting: &hangouts.PresenceStateSetting{
			TimeoutSecs: &timeoutSecs,
			Type:        (*hangouts.ClientPresenceStateType)(&presenceState),
		},
	}
	response := &hangouts.SetPresenceResponse{}
	err := c.ProtobufAPIRequest("presence/setpresence", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SetTyping set the typing status of a conversation.
// typingState: 1=Started 2=Paused 3=Stopped
func (c *Client) SetTyping(conversationID string, typingState int32) (*hangouts.SetTypingResponse, error) {
	request := &hangouts.SetTypingRequest{
		RequestHeader:  c.NewRequestHeaders(),
		ConversationId: &hangouts.ConversationId{Id: &conversationID},
		Type:           (*hangouts.TypingType)(&typingState),
	}
	response := &hangouts.SetTypingResponse{}
	err := c.ProtobufAPIRequest("conversations/settyping", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SyncAllNewEvents list all events occurring at or after a timestamp.
func (c *Client) SyncAllNewEvents(lastSyncTimestamp, maxResponseSizeBytes uint64) (*hangouts.SyncAllNewEventsResponse, error) {
	request := &hangouts.SyncAllNewEventsRequest{
		RequestHeader:        c.NewRequestHeaders(),
		LastSyncTimestamp:    &lastSyncTimestamp,
		MaxResponseSizeBytes: &maxResponseSizeBytes,
	}
	response := &hangouts.SyncAllNewEventsResponse{}
	err := c.ProtobufAPIRequest("conversations/syncallnewevents", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// SyncRecentConversations return info on recent conversations and their events.
func (c *Client) SyncRecentConversations(maxConversations, maxEventsPerConversation uint64) (*hangouts.SyncRecentConversationsResponse, error) {
	request := &hangouts.SyncRecentConversationsRequest{
		RequestHeader:            c.NewRequestHeaders(),
		MaxConversations:         &maxConversations,
		MaxEventsPerConversation: &maxEventsPerConversation,
		SyncFilter:               []hangouts.SyncFilter{hangouts.SyncFilter_SYNC_FILTER_INBOX},
	}
	response := &hangouts.SyncRecentConversationsResponse{}
	err := c.ProtobufAPIRequest("conversations/syncrecentconversations", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// UpdateWatermark update the watermark (read timestamp) of a conversation.
func (c *Client) UpdateWatermark(conversationID string, lastReadTimestamp uint64) (*hangouts.UpdateWatermarkResponse, error) {
	request := &hangouts.UpdateWatermarkRequest{
		RequestHeader:     c.NewRequestHeaders(),
		ConversationId:    &hangouts.ConversationId{Id: &conversationID},
		LastReadTimestamp: &lastReadTimestamp,
	}
	response := &hangouts.UpdateWatermarkResponse{}
	err := c.ProtobufAPIRequest("conversations/updatewatermark", request, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// getImageUploadURL request the image upload url for uploading
func (c *Client) getImageUploadURL(image string) (*UploadFile, error) {

	uploadFile, err := readImage(image)

	if err != nil {
		return nil, err
	}

	now := time.Now()
	nowMsec := now.UnixNano() / int64(time.Millisecond)
	param := fmt.Sprintf(`    
	{
        "protocolVersion": "0.8",
        "createSessionRequest": {
            "fields": [
                {
                    "external": {
                        "name": "file",
                        "filename": "%s",
                        "put": {},
                        "size": %v
                    }
                },
                {
                    "inlined": {
                        "name": "use_upload_size_pref",
                        "content": "true",
                        "contentType": "text/plain"
                    }
                },
                {
                    "inlined": {
                        "name": "album_mode",
                        "content": "temporary",
                        "contentType": "text/plain"
                    }
                },
                {
                    "inlined": {
                        "name": "title",
                        "content": "%s",
                        "contentType": "text/plain"
                    }
                },
                {
                    "inlined": {
                        "name": "addtime",
                        "content": "%v",
                        "contentType": "text/plain"
                    }
                },
                {
                    "inlined": {
                        "name": "batchid",
                        "content": "%v",
                        "contentType": "text/plain"
                    }
                },
                {
                    "inlined": {
                        "name": "album_name",
                        "content": "%v",
                        "contentType": "text/plain"
                    }
                },
                {
                    "inlined": {
                        "name": "album_abs_position",
                        "content": "0",
                        "contentType": "text/plain"
                    }
                },
                {
                    "inlined": {
                        "name": "client",
                        "content": "hangouts",
                        "contentType": "text/plain"
                    }
                }
            ]
        }
    }`, uploadFile.name, uploadFile.size, uploadFile.name, nowMsec, nowMsec, now.Format("2006-01-02"))

	params := json.RawMessage(param)

	payload, _ := params.MarshalJSON()

	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
	}

	resp, err := c.APIRequest(imageUploadURL, "json", headers, payload)

	if err != nil {
		return nil, err
	}
	if !gjson.ValidBytes(resp) {
		return nil, errors.New("cannot find upload url to upload, raw response : " + string(resp))
	}

	result := gjson.GetBytes(resp, "sessionStatus.externalFieldTransfers.0.putInfo.url")
	if !result.Exists() {
		return nil, errors.New("cannot find upload url to upload, raw response : " + string(resp))
	}
	uploadFile.uploadURL = result.String()
	return uploadFile, nil
}

// UploadImage uploads an image to google and returns the imageID for chat message sending
// image can be a file path or base64 encoded image
func (c *Client) UploadImage(image string) (*Photo, error) {
	uploadFile, err := c.getImageUploadURL(image)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Content-Type": "application/octet-stream",
	}

	resp, err := c.APIRequest(uploadFile.uploadURL, "json", headers, uploadFile.data)

	if err != nil {
		return nil, err
	}

	if !gjson.ValidBytes(resp) {
		return nil, errors.New("cannot upload image, raw response : " + string(resp))
	}

	result := gjson.GetBytes(resp, "sessionStatus.additionalInfo.uploader_service\\.GoogleRupioAdditionalInfo.completionInfo.customerSpecificInfo")
	if !result.Exists() {
		return nil, errors.New("cannot get image data from response, raw response : " + string(resp))
	}

	var photo Photo

	err = json.Unmarshal([]byte(result.Raw), &photo)

	if err != nil {
		return nil, err
	}

	return &photo, nil
}

// SendImage send image to a user or group
// to can be phoneNumber, email chatID/GaiaID or conversation ID
// image can be a file path or base64 encoded image
func (c *Client) SendImage(to, image string) error {
	photo, err := c.UploadImage(image)
	if err != nil {
		return fmt.Errorf("error uploading media %v : %v", image, err)
	}

	return c.sendMessage(to, "", photo.ImageID)
}

// SendImage send image to a user or group
// to can be phoneNumber, email chatID/GaiaID or conversation ID
// photoID is a photo id from hangouts message
func (c *Client) SendPhotoID(to, photoID string) error {
	return c.sendMessage(to, "", photoID)
}
