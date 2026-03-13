package telegram

type Update struct {
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
	Message       *Message       `json:"message,omitempty"`
	UpdateID      int64          `json:"update_id"`
}

type Message struct { //nolint:govet // readability over field packing
	Chat            Chat     `json:"chat"`
	From            *User    `json:"from,omitempty"`
	MessageID       int64    `json:"message_id"`
	ReplyToMessage  *Message `json:"reply_to_message,omitempty"`
	Text            string   `json:"text,omitempty"`
	MessageThreadID int64    `json:"message_thread_id,omitempty"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type User struct { //nolint:govet // readability over field packing
	ID       int64  `json:"id"`
	Username string `json:"username,omitempty"`
}

type CallbackQuery struct { //nolint:govet // readability over field packing
	Data    string   `json:"data,omitempty"`
	From    User     `json:"from"`
	ID      string   `json:"id"`
	Message *Message `json:"message,omitempty"`
}
