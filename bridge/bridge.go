package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
	"go.uber.org/zap"

	"github.com/keithah/hostex-bridge-go/config"
	"github.com/keithah/hostex-bridge-go/database"
	"github.com/keithah/hostex-bridge-go/hostexapi"
)

type Bridge struct {
	Config       *config.Config
	DB           *database.Database
	HostexClient *hostexapi.Client
	MatrixClient *mautrix.Client
	Logger       *zap.Logger

	usersByMXID    map[id.UserID]*User
	portalsByID    map[string]*Portal
	managementRoom id.RoomID
	spaceRoom      id.RoomID

	stop          chan struct{}
	wg            sync.WaitGroup
	lastPollTime  time.Time
}

func NewBridge(cfg *config.Config, db *database.Database, hostexClient *hostexapi.Client, matrixClient *mautrix.Client, logger *zap.Logger) *Bridge {
	return &Bridge{
		Config:       cfg,
		DB:           db,
		HostexClient: hostexClient,
		MatrixClient: matrixClient,
		Logger:       logger,
		usersByMXID:  make(map[id.UserID]*User),
		portalsByID:  make(map[string]*Portal),
		stop:         make(chan struct{}),
	}
}

func (b *Bridge) Start() error {
	b.Logger.Info("Starting Hostex bridge")

	// Login to Matrix
	_, err := b.MatrixClient.Login(&mautrix.ReqLogin{
		Type:             "m.login.password",
		Identifier:       mautrix.UserIdentifier{Type: "m.id.user", User: b.Config.User.UserID},
		Password:         b.Config.Appservice.ASToken,
		DeviceID:         "HostexBridge",
		StoreCredentials: true,
	})
	if err != nil {
		return fmt.Errorf("failed to log in to Matrix: %w", err)
	}

	// Create or find management room
	b.managementRoom, err = b.createOrFindManagementRoom()
	if err != nil {
		return fmt.Errorf("failed to create or find management room: %w", err)
	}

	// Create personal filtering space if enabled
	if b.Config.PersonalSpaceEnable {
		b.spaceRoom, err = b.createOrFindPersonalSpace()
		if err != nil {
			return fmt.Errorf("failed to create or find personal space: %w", err)
		}
	}

	// Start syncing
	b.wg.Add(1)
	go b.startSyncing()

	// Start polling
	b.wg.Add(1)
	go b.startPolling()

	// Send setup message
	b.sendSetupMessage()

	return nil
}

func (b *Bridge) Stop() {
	b.Logger.Info("Stopping Hostex bridge")
	close(b.stop)
	b.wg.Wait()
}

func (b *Bridge) createOrFindManagementRoom() (id.RoomID, error) {
	rooms, err := b.MatrixClient.JoinedRooms()
	if err != nil {
		return "", err
	}

	for _, roomID := range rooms.JoinedRooms {
		// Check if this is the management room
		state, err := b.MatrixClient.RoomState(roomID)
		if err != nil {
			continue
		}
		for _, evt := range state {
			if evt.Type == "m.room.name" && evt.Content.Get("name").(string) == "Hostex Bridge Management" {
				return roomID, nil
			}
		}
	}

	// If not found, create a new management room
	createRoom := &mautrix.ReqCreateRoom{
		Visibility: "private",
		Name:       "Hostex Bridge Management",
		Topic:      "Management room for Hostex bridge",
		Invite:     []id.UserID{id.UserID(b.Config.Admin.UserID)},
	}
	resp, err := b.MatrixClient.CreateRoom(createRoom)
	if err != nil {
		return "", err
	}

	return resp.RoomID, nil
}

func (b *Bridge) createOrFindPersonalSpace() (id.RoomID, error) {
	rooms, err := b.MatrixClient.JoinedRooms()
	if err != nil {
		return "", err
	}

	for _, roomID := range rooms.JoinedRooms {
		// Check if this is the personal space
		state, err := b.MatrixClient.RoomState(roomID)
		if err != nil {
			continue
		}
		for _, evt := range state {
			if evt.Type == "m.room.type" && evt.Content.Get("type").(string) == "m.space" &&
				evt.Type == "m.room.name" && evt.Content.Get("name").(string) == "Hostex Conversations" {
				return roomID, nil
			}
		}
	}

	// If not found, create a new personal space
	createRoom := &mautrix.ReqCreateRoom{
		Visibility: "private",
		Name:       "Hostex Conversations",
		Topic:      "Personal space for Hostex conversations",
		CreationContent: map[string]interface{}{
			"type": "m.space",
		},
		InitialState: []*event.Event{
			{
				Type: "m.room.type",
				Content: event.Content{
					"type": "m.space",
				},
			},
		},
	}
	resp, err := b.MatrixClient.CreateRoom(createRoom)
	if err != nil {
		return "", err
	}

	return resp.RoomID, nil
}

func (b *Bridge) startSyncing() {
	defer b.wg.Done()

	syncer := b.MatrixClient.Syncer.(*mautrix.DefaultSyncer)
	syncer.OnEventType(event.EventMessage, b.handleMatrixMessage)

	for {
		select {
		case <-b.stop:
			return
		default:
			err := b.MatrixClient.Sync()
			if err != nil {
				b.Logger.Error("Sync error", zap.Error(err))
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func (b *Bridge) startPolling() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.Config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
			b.pollHostex()
		}
	}
}

func (b *Bridge) pollHostex() {
	b.lastPollTime = time.Now()
	conversations, err := b.HostexClient.GetConversations()
	if err != nil {
		b.Logger.Error("Failed to get conversations", zap.Error(err))
		return
	}

	for _, conv := range conversations {
		b.handleHostexConversation(conv)
	}
}

func (b *Bridge) handleHostexConversation(conv hostexapi.Conversation) {
	portal, ok := b.portalsByID[conv.ID]
	if !ok {
		portal = NewPortal(b, conv.ID)
		b.portalsByID[conv.ID] = portal
	}

	portal.UpdateInfo(conv)
	err := portal.CreateMatrixRoom()
	if err != nil {
		b.Logger.Error("Failed to create Matrix room", zap.Error(err))
		return
	}

	err = portal.BackfillMessages()
	if err != nil {
		b.Logger.Error("Failed to backfill messages", zap.Error(err))
	}
}

func (b *Bridge) handleMatrixMessage(source mautrix.EventSource, evt *event.Event) {
	if evt.RoomID == b.managementRoom {
		b.handleManagementCommand(evt)
		return
	}

	portal, ok := b.portalsByID[evt.RoomID.String()]
	if !ok {
		b.Logger.Warn("Received message for unknown portal", zap.String("room_id", evt.RoomID.String()))
		return
	}

	portal.HandleMatrixMessage(evt)
}

func (b *Bridge) handleManagementCommand(evt *event.Event) {
	if evt.Sender != id.UserID(b.Config.Admin.UserID) {
		b.Logger.Warn("Unauthorized management command", zap.String("sender", evt.Sender.String()))
		return
	}

	content, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}

	user, ok := b.usersByMXID[evt.Sender]
	if !ok {
		user = NewUser(b, evt.Sender)
		b.usersByMXID[evt.Sender] = user
	}

	user.HandleCommand(evt.RoomID, content.Body)
}

func (b *Bridge) sendSetupMessage() {
	content := &event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    "Hostex bridge has been set up and is now running.",
	}
	_, err := b.MatrixClient.SendMessageEvent(b.managementRoom, event.EventMessage, content)
	if err != nil {
		b.Logger.Error("Failed to send setup message", zap.Error(err))
	}
}

func (b *Bridge) GetLastPollTime() time.Time {
	return b.lastPollTime
}

func (b *Bridge) ForceSyncConversations() {
	b.pollHostex()
}
