package discord

import "encoding/json"

const (
	interactionTypePing               = 1
	interactionTypeApplicationCommand = 2
	interactionTypeMessageComponent   = 3

	interactionResponsePong     = 1
	interactionResponseDeferred = 5
)

type Interaction struct { //nolint:govet // readability over field packing
	ChannelID string          `json:"channel_id,omitempty"`
	GuildID   string          `json:"guild_id,omitempty"`
	ID        string          `json:"id"`
	Token     string          `json:"token"`
	Type      int             `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	Member    *Member         `json:"member,omitempty"`
	User      *User           `json:"user,omitempty"`
}

type Member struct {
	User User `json:"user"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
}

type ApplicationCommandData struct {
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name"`
	Options []CommandOption `json:"options,omitempty"`
}

type CommandOption struct { //nolint:govet // readability over field packing
	Name  string `json:"name"`
	Type  int    `json:"type"`
	Value string `json:"value,omitempty"`
}

type MessageComponentData struct {
	CustomID string `json:"custom_id"`
	Value    string `json:"value,omitempty"`
}

type interactionResponse struct {
	Type int `json:"type"`
}
