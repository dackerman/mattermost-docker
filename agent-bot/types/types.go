package types

// Message represents a generic chat message
type Message struct {
	ID        string
	UserID    string
	ChannelID string
	ThreadID  string
	Content   string
	Timestamp int64
}

// User represents a generic chat user
type User struct {
	ID       string
	Username string
	IsBot    bool
}

// PostedMessage represents an incoming message event
type PostedMessage struct {
	PostId    string
	UserId    string
	ThreadId  string
	ChannelId string
	Message   string
}

// Agent handles incoming messages
type Agent interface {
	MessagePosted(message PostedMessage)
}

// ChatMessage represents an outgoing message
type ChatMessage struct {
	ThreadId  string
	ChannelId string
	Message   string
}

// Chat provides generic chat platform operations
type Chat interface {
	// Send a message
	PostMessage(message ChatMessage) error

	// Send typing indicator
	SendTypingIndicator(channelID, threadID string) error

	// Retrieve a specific message
	GetMessage(messageID string) (*Message, error)

	// Retrieve all messages in a thread
	GetThreadMessages(threadID string) ([]*Message, error)

	// Get user information
	GetUser(userID string) (*User, error)
}

// LLM provides language model operations
type LLM interface {
	Prompt(message string) (string, error)
}
