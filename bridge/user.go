package bridge

import (
    "context"
    "fmt"
    "strings"
    "time"

    "maunium.net/go/mautrix/event"
    "maunium.net/go/mautrix/id"
    "go.uber.org/zap"
)

type User struct {
    bridge *Bridge
    MXID   id.UserID
}

func NewUser(bridge *Bridge, mxid id.UserID) *User {
    return &User{
        bridge: bridge,
        MXID:   mxid,
    }
}

func (u *User) HandleCommand(roomID id.RoomID, body string) {
    parts := strings.Fields(body)
    if len(parts) == 0 {
        return
    }

    command := strings.ToLower(parts[0])

    ctx := context.Background()

    switch command {
    case "!help":
        u.sendHelpMessage(ctx, roomID)
    case "!status":
        u.sendStatusMessage(ctx, roomID)
    case "!list":
        u.listConversations(ctx, roomID)
    case "!sync":
        u.forceSyncConversations(ctx, roomID)
    default:
        u.sendUnknownCommandMessage(ctx, roomID)
    }
}

func (u *User) sendHelpMessage(ctx context.Context, roomID id.RoomID) {
    content := &event.MessageEventContent{
        MsgType: event.MsgNotice,
        Body: `Available commands:
!help - Show this help message
!status - Show bridge status
!list - List active conversations
!sync - Force sync conversations from Hostex`,
    }
    _, err := u.bridge.MatrixClient.SendMessageEvent(ctx, roomID, event.EventMessage, content)
    if err != nil {
        u.bridge.Logger.Error("Failed to send help message", zap.Error(err))
    }
}

func (u *User) sendStatusMessage(ctx context.Context, roomID id.RoomID) {
    var bridgedRooms int
    lastPollTime := u.bridge.GetLastPollTime()

    for _, portal := range u.bridge.portalsByID {
        if portal.RoomID != "" {
            bridgedRooms++
        }
    }

    content := &event.MessageEventContent{
        MsgType: event.MsgNotice,
        Body: fmt.Sprintf(`Bridge Status:
Connected to Hostex: %v
Bridged conversations: %d
Last poll time: %s
Timezone: %s`,
            u.bridge.HostexClient != nil,
            bridgedRooms,
            lastPollTime.Format(time.RFC3339),
            u.bridge.Config.Timezone),
    }
    _, err := u.bridge.MatrixClient.SendMessageEvent(ctx, roomID, event.EventMessage, content)
    if err != nil {
        u.bridge.Logger.Error("Failed to send status message", zap.Error(err))
    }
}

func (u *User) listConversations(ctx context.Context, roomID id.RoomID) {
    var conversationList strings.Builder
    conversationList.WriteString("Active conversations:\n\n")

    for _, portal := range u.bridge.portalsByID {
        if portal.RoomID != "" {
            conversationList.WriteString(fmt.Sprintf("- %s (%s)\n  Room: %s\n  Last activity: %s\n\n",
                portal.Info.Guest.Name,
                portal.Info.ChannelType,
                portal.RoomID,
                portal.Info.LastMessageAt.Format(time.RFC3339)))
        }
    }

    content := &event.MessageEventContent{
        MsgType: event.MsgNotice,
        Body:    conversationList.String(),
    }
    _, err := u.bridge.MatrixClient.SendMessageEvent(ctx, roomID, event.EventMessage, content)
    if err != nil {
        u.bridge.Logger.Error("Failed to send conversation list", zap.Error(err))
    }
}

func (u *User) forceSyncConversations(ctx context.Context, roomID id.RoomID) {
    u.sendNotice(ctx, roomID, "Forcing sync of conversations from Hostex...")

    go func() {
        u.bridge.ForceSyncConversations()
        u.sendNotice(ctx, roomID, "Sync complete. Use !list to see updated conversations.")
    }()
}

func (u *User) sendUnknownCommandMessage(ctx context.Context, roomID id.RoomID) {
    content := &event.MessageEventContent{
        MsgType: event.MsgNotice,
        Body:    "Unknown command. Type !help for a list of available commands.",
    }
    _, err := u.bridge.MatrixClient.SendMessageEvent(ctx, roomID, event.EventMessage, content)
    if err != nil {
        u.bridge.Logger.Error("Failed to send unknown command message", zap.Error(err))
    }
}

func (u *User) sendNotice(ctx context.Context, roomID id.RoomID, message string) {
    content := &event.MessageEventContent{
        MsgType: event.MsgNotice,
        Body:    message,
    }
    _, err := u.bridge.MatrixClient.SendMessageEvent(ctx, roomID, event.EventMessage, content)
    if err != nil {
        u.bridge.Logger.Error("Failed to send notice", zap.Error(err))
    }
}
