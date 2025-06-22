package types

type PostedMessage struct {
	PostId    string
	ThreadId  string
	ChannelId string
	Message   string
}

type Agent interface {
	MessagePosted(message PostedMessage)
}

type ChatMessage struct {
	ThreadId  string
	ChannelId string
	Message   string
}

type Chat interface {
	PostMessage(message ChatMessage) error
}

type LLM interface {
	Prompt(message string) (string, error)
}
