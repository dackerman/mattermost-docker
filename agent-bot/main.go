package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"agent-bot/llms"
	"agent-bot/types"

	"github.com/joho/godotenv"
	"github.com/mattermost/mattermost-server/v6/model"
)

type Config struct {
	ServerURL    string
	AccessToken  string
	BotUserID    string
	AnthropicKey string
	AsanaKey     string
}

type Bot struct {
	client          *model.Client4
	config          Config
	wsClient        *model.WebSocketClient
	reconnectTicker *time.Ticker
	stopChan        chan struct{}
	llmBackend      llms.LLMBackend
	agent           types.Agent
}

func NewBot(config Config, llmBackend llms.LLMBackend) *Bot {
	client := model.NewAPIv4Client(config.ServerURL)
	client.SetToken(config.AccessToken)

	bot := &Bot{
		client:     client,
		config:     config,
		stopChan:   make(chan struct{}),
		llmBackend: llmBackend,
	}

	// Create the agent with proper dependencies
	llmAdapter := &LLMAdapter{backend: llmBackend}
	chatAdapter := &ChatAdapter{bot: bot}
	bot.agent = NewBotAgent(config.BotUserID, llmAdapter, chatAdapter)

	return bot
}

func (b *Bot) handleWebSocketEvent(event *model.WebSocketEvent) {
	// Parse post data from event
	postData, ok := event.GetData()["post"].(string)
	if !ok {
		return
	}

	var post model.Post
	if err := json.Unmarshal([]byte(postData), &post); err != nil {
		log.Printf("[%s] ERROR: Failed to parse post: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}

	// Don't respond to our own messages
	if post.UserId == b.config.BotUserID {
		log.Printf("[%s] SKIP: Ignoring own message", time.Now().Format("2006-01-02 15:04:05"))
		return
	}

	// Extract channel type from event data
	channelType, _ := event.GetData()["channel_type"].(string)
	isDM := channelType == "D"

	// Convert to PostedMessage and delegate to agent
	message := types.PostedMessage{
		PostId:    post.Id,
		UserId:    post.UserId,
		ThreadId:  post.RootId,
		ChannelId: post.ChannelId,
		Message:   post.Message,
		IsDM:      isDM,
	}

	b.agent.MessagePosted(message)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (b *Bot) connectWebSocket() error {
	wsURL := strings.Replace(b.config.ServerURL, "http://", "ws://", 1)
	log.Printf("[%s] WEBSOCKET: Connecting to %s", time.Now().Format("2006-01-02 15:04:05"), wsURL)

	wsClient, err := model.NewWebSocketClient4(wsURL, b.client.AuthToken)
	if err != nil {
		return fmt.Errorf("failed to create WebSocket client: %v", err)
	}

	wsClient.Listen()
	b.wsClient = wsClient
	log.Printf("[%s] WEBSOCKET: Connection established, listening for events", time.Now().Format("2006-01-02 15:04:05"))

	return nil
}

func (b *Bot) isWebSocketConnected() bool {
	return b.wsClient != nil && b.wsClient.EventChannel != nil
}

func (b *Bot) handleWebSocketReconnection() {
	b.reconnectTicker = time.NewTicker(10 * time.Second)

	go func() {
		for {
			select {
			case <-b.reconnectTicker.C:
				if !b.isWebSocketConnected() {
					log.Printf("[%s] WEBSOCKET: Connection lost, attempting to reconnect...", time.Now().Format("2006-01-02 15:04:05"))

					if err := b.connectWebSocket(); err != nil {
						log.Printf("[%s] WEBSOCKET: Reconnection failed: %v", time.Now().Format("2006-01-02 15:04:05"), err)
					} else {
						log.Printf("[%s] WEBSOCKET: Reconnected successfully", time.Now().Format("2006-01-02 15:04:05"))
						b.startEventListener()
					}
				}
			case <-b.stopChan:
				b.reconnectTicker.Stop()
				return
			}
		}
	}()
}

func (b *Bot) startEventListener() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s] WEBSOCKET: Event listener panicked: %v", time.Now().Format("2006-01-02 15:04:05"), r)
				b.wsClient = nil
			}
		}()

		if b.wsClient == nil || b.wsClient.EventChannel == nil {
			log.Printf("[%s] WEBSOCKET: No WebSocket client or event channel", time.Now().Format("2006-01-02 15:04:05"))
			return
		}

		for {
			select {
			case event, ok := <-b.wsClient.EventChannel:
				if !ok {
					log.Printf("[%s] WEBSOCKET: Event channel closed, connection lost", time.Now().Format("2006-01-02 15:04:05"))
					b.wsClient = nil
					return
				}

				if event.EventType() == model.WebsocketEventPosted {
					log.Printf("[%s] EVENT: Received post event", time.Now().Format("2006-01-02 15:04:05"))
					b.handleWebSocketEvent(event)
				} else {
					log.Printf("[%s] EVENT: Received event type: %s", time.Now().Format("2006-01-02 15:04:05"), event.EventType())
				}
			case <-b.stopChan:
				return
			}
		}
	}()
}

func (b *Bot) start() {
	log.Printf("[%s] STARTUP: Starting agent bot...", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("[%s] CONFIG: Server URL: %s", time.Now().Format("2006-01-02 15:04:05"), b.config.ServerURL)
	log.Printf("[%s] CONFIG: Bot User ID: %s", time.Now().Format("2006-01-02 15:04:05"), b.config.BotUserID)

	// Initial WebSocket connection
	if err := b.connectWebSocket(); err != nil {
		log.Fatalf("[%s] FATAL: Failed to connect to WebSocket: %v", time.Now().Format("2006-01-02 15:04:05"), err)
	}

	// Start event listener
	b.startEventListener()

	// Start reconnection handler
	b.handleWebSocketReconnection()

	// Keep HTTP server for health checks
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] HEALTH: Health check requested", time.Now().Format("2006-01-02 15:04:05"))
		status := "OK"
		if !b.isWebSocketConnected() {
			status = "WebSocket Disconnected"
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(status))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("[%s] SERVER: Bot listening on port %s (WebSocket connected)", time.Now().Format("2006-01-02 15:04:05"), port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// LLMAdapter adapts llms.LLMBackend to types.LLM interface
type LLMAdapter struct {
	backend llms.LLMBackend
}

func (l *LLMAdapter) Prompt(message string) (string, error) {
	return l.backend.Prompt(context.Background(), message)
}

// ChatAdapter adapts Bot to types.Chat interface
type ChatAdapter struct {
	bot *Bot
}

func (c *ChatAdapter) PostMessage(message types.ChatMessage) error {
	post := &model.Post{
		ChannelId: message.ChannelId,
		Message:   message.Message,
		RootId:    message.ThreadId,
	}

	if _, _, err := c.bot.client.CreatePost(post); err != nil {
		return fmt.Errorf("failed to post message: %v", err)
	}

	return nil
}

func (c *ChatAdapter) SendTypingIndicator(channelID, threadID string) error {
	typingRequest := model.TypingRequest{
		ChannelId: channelID,
		ParentId:  threadID,
	}
	
	_, err := c.bot.client.PublishUserTyping(c.bot.config.BotUserID, typingRequest)
	return err
}

func (c *ChatAdapter) GetMessage(messageID string) (*types.Message, error) {
	post, _, err := c.bot.client.GetPost(messageID, "")
	if err != nil {
		return nil, err
	}
	
	return &types.Message{
		ID:        post.Id,
		UserID:    post.UserId,
		ChannelID: post.ChannelId,
		ThreadID:  post.RootId,
		Content:   post.Message,
		Timestamp: post.CreateAt,
	}, nil
}

func (c *ChatAdapter) GetThreadMessages(threadID string) ([]*types.Message, error) {
	threadPosts, _, err := c.bot.client.GetPostThread(threadID, "", true)
	if err != nil {
		return nil, err
	}
	
	messages := make([]*types.Message, 0, len(threadPosts.Posts))
	for _, post := range threadPosts.Posts {
		messages = append(messages, &types.Message{
			ID:        post.Id,
			UserID:    post.UserId,
			ChannelID: post.ChannelId,
			ThreadID:  post.RootId,
			Content:   post.Message,
			Timestamp: post.CreateAt,
		})
	}
	
	return messages, nil
}

func (c *ChatAdapter) GetUser(userID string) (*types.User, error) {
	user, _, err := c.bot.client.GetUser(userID, "")
	if err != nil {
		return nil, err
	}
	
	return &types.User{
		ID:       user.Id,
		Username: user.Username,
		IsBot:    user.IsBot,
	}, nil
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	config := Config{
		ServerURL:    os.Getenv("MATTERMOST_SERVER_URL"),
		AccessToken:  os.Getenv("MATTERMOST_ACCESS_TOKEN"),
		BotUserID:    os.Getenv("MATTERMOST_BOT_USER_ID"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		AsanaKey:     os.Getenv("ASANA_API_KEY"),
	}

	if config.ServerURL == "" || config.AccessToken == "" {
		log.Fatal("Missing required environment variables: MATTERMOST_SERVER_URL, MATTERMOST_ACCESS_TOKEN")
	}

	if config.AnthropicKey == "" {
		log.Fatal("Missing required environment variable: ANTHROPIC_API_KEY")
	}

	if config.AsanaKey == "" {
		log.Fatal("Missing required environment variable: ASANA_API_KEY")
	}

	// Initialize LLM backend
	llmBackend := llms.NewAnthropicBackend(config.AnthropicKey, config.AsanaKey)

	bot := NewBot(config, llmBackend)
	bot.start()
}
