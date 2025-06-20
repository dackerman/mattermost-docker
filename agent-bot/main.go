package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/mattermost/mattermost-server/v6/model"
)

type Config struct {
	ServerURL   string
	AccessToken string
	BotUserID   string
}

type Bot struct {
	client *model.Client4
	config Config
}

func NewBot(config Config) *Bot {
	client := model.NewAPIv4Client(config.ServerURL)
	client.SetToken(config.AccessToken)
	
	return &Bot{
		client: client,
		config: config,
	}
}

func (b *Bot) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var post model.Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Don't respond to our own messages
	if post.UserId == b.config.BotUserID {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only respond to mentions
	if strings.Contains(post.Message, "@"+b.config.BotUserID) {
		b.respondToMessage(&post)
	}

	w.WriteHeader(http.StatusOK)
}

func (b *Bot) handleWebSocketEvent(event *model.WebSocketEvent) {
	// Parse post data from event
	postData, ok := event.GetData()["post"].(string)
	if !ok {
		return
	}
	
	var post model.Post
	if err := json.Unmarshal([]byte(postData), &post); err != nil {
		log.Printf("Failed to parse post: %v", err)
		return
	}
	
	// Don't respond to our own messages
	if post.UserId == b.config.BotUserID {
		return
	}
	
	// Only respond to mentions
	if strings.Contains(post.Message, "@agent-bot") || strings.Contains(post.Message, b.config.BotUserID) {
		b.respondToMessage(&post)
	}
}

func (b *Bot) respondToMessage(post *model.Post) {
	// Echo the message back with a prefix
	response := fmt.Sprintf("Echo: %s", post.Message)
	
	newPost := &model.Post{
		ChannelId: post.ChannelId,
		Message:   response,
	}

	if _, _, err := b.client.CreatePost(newPost); err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

func (b *Bot) start() {
	// Start WebSocket connection to listen for events
	webSocketClient, err := model.NewWebSocketClient4("ws://localhost:8065", b.client.AuthToken)
	if err != nil {
		log.Fatal("Failed to connect to WebSocket:", err)
	}
	
	webSocketClient.Listen()
	
	// Listen for message events
	go func() {
		for event := range webSocketClient.EventChannel {
			if event.EventType() == model.WebsocketEventPosted {
				b.handleWebSocketEvent(event)
			}
		}
	}()
	
	// Keep HTTP server for health checks
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	
	log.Printf("Bot listening on port %s (WebSocket connected)", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	config := Config{
		ServerURL:   os.Getenv("MATTERMOST_SERVER_URL"),
		AccessToken: os.Getenv("MATTERMOST_ACCESS_TOKEN"),
		BotUserID:   os.Getenv("MATTERMOST_BOT_USER_ID"),
	}

	if config.ServerURL == "" || config.AccessToken == "" {
		log.Fatal("Missing required environment variables: MATTERMOST_SERVER_URL, MATTERMOST_ACCESS_TOKEN")
	}

	bot := NewBot(config)
	bot.start()
}