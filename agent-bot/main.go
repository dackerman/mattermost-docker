package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/mattermost/mattermost-server/v6/model"
)

type Config struct {
	ServerURL   string
	AccessToken string
	BotUserID   string
}

type Bot struct {
	client          *model.Client4
	config          Config
	wsClient        *model.WebSocketClient
	reconnectTicker *time.Ticker
	stopChan        chan struct{}
}

func NewBot(config Config) *Bot {
	client := model.NewAPIv4Client(config.ServerURL)
	client.SetToken(config.AccessToken)
	
	return &Bot{
		client:   client,
		config:   config,
		stopChan: make(chan struct{}),
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
		log.Printf("[%s] ERROR: Failed to parse post: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		return
	}
	
	// Log all incoming messages
	log.Printf("[%s] INCOMING: User %s in channel %s: %s", 
		time.Now().Format("2006-01-02 15:04:05"), 
		post.UserId, 
		post.ChannelId, 
		post.Message)
	
	// Don't respond to our own messages
	if post.UserId == b.config.BotUserID {
		log.Printf("[%s] SKIP: Ignoring own message", time.Now().Format("2006-01-02 15:04:05"))
		return
	}
	
	// Check if we should respond
	shouldRespond := strings.Contains(post.Message, "@agent-bot") || strings.Contains(post.Message, b.config.BotUserID)
	if shouldRespond {
		log.Printf("[%s] MENTION: Bot mentioned, preparing response", time.Now().Format("2006-01-02 15:04:05"))
		b.respondToMessage(&post)
	} else {
		log.Printf("[%s] SKIP: No mention detected", time.Now().Format("2006-01-02 15:04:05"))
	}
}

func (b *Bot) respondToMessage(post *model.Post) {
	// Echo the message back with a prefix
	response := fmt.Sprintf("Echo: %s", post.Message)
	
	log.Printf("[%s] OUTGOING: Sending response to channel %s: %s", 
		time.Now().Format("2006-01-02 15:04:05"), 
		post.ChannelId, 
		response)
	
	newPost := &model.Post{
		ChannelId: post.ChannelId,
		Message:   response,
	}

	if _, _, err := b.client.CreatePost(newPost); err != nil {
		log.Printf("[%s] ERROR: Failed to send message: %v", time.Now().Format("2006-01-02 15:04:05"), err)
	} else {
		log.Printf("[%s] SUCCESS: Message sent successfully", time.Now().Format("2006-01-02 15:04:05"))
	}
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