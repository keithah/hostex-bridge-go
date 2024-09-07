package bridge

import (
	"fmt"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"go.uber.org/zap"

	"github.com/keithah/hostex-bridge-go/hostexapi"
)

type Portal struct {
	bridge *Bridge
	ID     string
	RoomID id.RoomID

	Info hostexapi.Conversation
}

func NewPortal(bridge *Bridge, id string) *Portal {
	return &Portal{
		bridge: bridge,
		ID:     id,
	}
}

func (p *Portal) UpdateInfo(info hostexapi.Conversation) {
	p.Info = info
}

func (p *Portal) CreateMatrixRoom() error {
	if p.RoomID != "" {
		return nil
	}

	existingRoomID, err := p.bridge.DB.GetPortal(p.ID)
	if err != nil {
		return fmt.Errorf("failed to check existing portal: %w", err)
	}

	if existingRoomID != "" {
		p.RoomID = existingRoomID
		return nil
	}

	createRoom := &mautrix.ReqCreateRoom{
		Visibility: "private",
		Name:       fmt.Sprintf("%s - %s", p.Info.ChannelType, p.Info.Guest.Name),
		Topic:      fmt.Sprintf("Hostex conversation for %s", p.Info.PropertyTitle),
	}

	resp, err := p.bridge.MatrixClient.CreateRoom(createRoom)
	if err != nil {
		return fmt.Errorf("failed to create Matrix room: %w", err)
	}

	p.RoomID = resp.RoomID
	p.bridge.Logger.Info("Created Matrix room", zap.String("room_id", p.RoomID.String()))

	err = p.bridge.DB.StorePortal(p.ID, p.RoomID, createRoom.Name, createRoom.Topic, "", false)
	if err != nil {
		return fmt.Errorf("failed to store portal in database: %w", err)
	}

	if p.bridge.Config.PersonalSpaceEnable {
		err = p.addToPersonalSpace()
		if err != nil {
			p.bridge.Logger.Error("Failed to add room to personal space", zap.Error(err))
		}
	}

	return nil
}

func (p *Portal) addToPersonalSpace() error {
	_, err := p.bridge.MatrixClient.SendStateEvent(p.bridge.spaceRoom, event.StateSpaceChild, p.RoomID.String(), &event.SpaceChildEventContent{
		Via: []string{p.bridge.Config.Homeserver.Domain},
	})
	if err != nil {
		return fmt.Errorf("failed to add room to personal space: %w", err)
	}
	return nil
}

func (p *Portal) HandleMatrixMessage(evt *event.Event) {
	if evt.Type != event.EventMessage {
		return
	}

	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		p.bridge.Logger.Warn("Received non-message event")
		return
	}

	// Send message to Hostex
	err := p.bridge.HostexClient.SendMessage(p.ID, content.Body)
	if err != nil {
		p.bridge.Logger.Error("Failed to send message to Hostex", zap.Error(err))
		return
	}

	// Store message in database
	err = p.bridge.DB.StoreMessage(p.ID, evt.ID, time.Now(), evt.Sender.String(), content.Body)
	if err != nil {
		p.bridge.Logger.Error("Failed to store message in database", zap.Error(err))
	}
}

func (p *Portal) BackfillMessages() error {
	lastTimestamp, err := p.bridge.DB.GetLastMessageTimestamp(p.ID)
	if err != nil {
		return fmt.Errorf("failed to get last message timestamp: %w", err)
	}

	messages, err := p.bridge.HostexClient.GetMessages(p.ID, lastTimestamp, 10)
	if err != nil {
		return fmt.Errorf("failed to get messages from Hostex: %w", err)
	}

	for _, msg := range messages {
		err = p.SendMessage(msg)
		if err != nil {
			p.bridge.Logger.Error("Failed to send backfilled message", zap.Error(err))
		}
	}

	return nil
}

func (p *Portal) SendMessage(msg hostexapi.Message) error {
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    msg.Content,
	}

	// Convert timestamp to configured timezone
	loc, err := time.LoadLocation(p.bridge.Config.Timezone)
	if err != nil {
		p.bridge.Logger.Error("Failed to load timezone", zap.Error(err))
		loc = time.UTC
	}
	timestamp := msg.Timestamp.In(loc)

	resp, err := p.bridge.MatrixClient.SendMessageEvent(p.RoomID, event.EventMessage, content, mautrix.ReqSendEvent{Timestamp: timestamp.UnixNano() / 1e6})
	if err != nil {
		return fmt.Errorf("failed to send Matrix message: %w", err)
	}

	// Store message in database
	err = p.bridge.DB.StoreMessage(p.ID, resp.EventID, timestamp, msg.Sender, msg.Content)
	if err != nil {
		p.bridge.Logger.Error("Failed to store message in database", zap.Error(err))
	}

	return nil
}
