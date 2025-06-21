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
	"github.com/joho/godotenv"
	"github.com/mattermost/mattermost-server/v6/model"
)

type Config struct {
	ServerURL     string
	AccessToken   string
	BotUserID     string
	AnthropicKey  string
}


type Bot struct {
	client          *model.Client4
	config          Config
	wsClient        *model.WebSocketClient
	reconnectTicker *time.Ticker
	stopChan        chan struct{}
	llmBackend      llms.LLMBackend
	activeThreads   map[string]bool // Track threads where bot is actively participating
	lastCleanup     time.Time       // Track when we last cleaned up stale threads
}

func NewBot(config Config, llmBackend llms.LLMBackend) *Bot {
	client := model.NewAPIv4Client(config.ServerURL)
	client.SetToken(config.AccessToken)
	
	return &Bot{
		client:        client,
		config:        config,
		stopChan:      make(chan struct{}),
		llmBackend:    llmBackend,
		activeThreads: make(map[string]bool),
		lastCleanup:   time.Now(),
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

func (b *Bot) cleanupStaleThreads() {
	// Clean up stale thread tracking every 10 minutes
	if time.Since(b.lastCleanup) < 10*time.Minute {
		return
	}
	
	log.Printf("[%s] CLEANUP: Cleaning up stale thread references", time.Now().Format("2006-01-02 15:04:05"))
	
	// Test a few thread IDs to see if they're still accessible
	staleThreads := make([]string, 0)
	count := 0
	for threadId := range b.activeThreads {
		if count >= 5 { // Only check first 5 to avoid too many API calls
			break
		}
		if _, _, err := b.client.GetPost(threadId, ""); err != nil {
			staleThreads = append(staleThreads, threadId)
		}
		count++
	}
	
	// Remove stale threads
	for _, threadId := range staleThreads {
		delete(b.activeThreads, threadId)
		log.Printf("[%s] CLEANUP: Removed stale thread %s", time.Now().Format("2006-01-02 15:04:05"), threadId)
	}
	
	b.lastCleanup = time.Now()
	log.Printf("[%s] CLEANUP: Completed, %d active threads remaining", time.Now().Format("2006-01-02 15:04:05"), len(b.activeThreads))
}

func (b *Bot) sendTypingIndicator(channelID, parentID string) error {
	// Use official Mattermost client method
	typingRequest := model.TypingRequest{
		ChannelId: channelID,
		ParentId:  parentID,
	}
	
	_, err := b.client.PublishUserTyping(b.config.BotUserID, typingRequest)
	if err != nil {
		return fmt.Errorf("typing indicator failed: %v", err)
	}
	
	return nil
}

func (b *Bot) handleWebSocketEvent(event *model.WebSocketEvent) {
	// Periodically clean up stale thread references
	b.cleanupStaleThreads()
	
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
	isMentioned := strings.Contains(post.Message, "@agent-bot") || strings.Contains(post.Message, b.config.BotUserID)
	isInActiveThread := b.activeThreads[post.RootId] && post.RootId != ""
	isDM := post.Type == "D" // Direct message
	
	shouldRespond := isMentioned || isDM
	
	// Also respond in active threads, but decide if we should based on content
	if !shouldRespond && isInActiveThread {
		shouldRespond = b.shouldRespondInThread(&post)
	}
	
	if shouldRespond {
		if isMentioned {
			log.Printf("[%s] MENTION: Bot mentioned, preparing response", time.Now().Format("2006-01-02 15:04:05"))
		} else if isDM {
			log.Printf("[%s] DM: Direct message received, preparing response", time.Now().Format("2006-01-02 15:04:05"))
		} else if isInActiveThread {
			log.Printf("[%s] THREAD: Responding in active thread", time.Now().Format("2006-01-02 15:04:05"))
		}
		b.respondToMessage(&post)
	} else {
		log.Printf("[%s] SKIP: No mention/DM/thread participation needed", time.Now().Format("2006-01-02 15:04:05"))
	}
}

func (b *Bot) shouldRespondInThread(post *model.Post) bool {
	// Simple heuristic: respond if the message seems to be asking a question or addressing the conversation
	message := strings.ToLower(post.Message)
	
	// Question indicators
	if strings.Contains(message, "?") {
		return true
	}
	
	// Direct conversation indicators
	questionWords := []string{"how", "what", "when", "where", "why", "who", "can you", "could you", "would you", "do you"}
	for _, word := range questionWords {
		if strings.Contains(message, word) {
			return true
		}
	}
	
	// Skip if it seems like casual conversation between others
	if len(message) < 10 || strings.Contains(message, "lol") || strings.Contains(message, "haha") {
		return false
	}
	
	return true // Default to participating in active threads
}

func (b *Bot) getThreadContext(post *model.Post) (string, error) {
	// If this is not a threaded message, just return the current message
	rootId := post.RootId
	if rootId == "" {
		rootId = post.Id // If this will become the root of a new thread
	}
	
	// Get all posts in the thread
	threadPosts, _, err := b.client.GetPostThread(rootId, "", true)
	if err != nil {
		log.Printf("[%s] THREAD: Failed to get thread context: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		return post.Message, nil // Fallback to just the current message
	}
	
	// Note: We identify the bot by UserID comparison instead of fetching user info
	
	// Build context string
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Previous conversation context:\n\n")
	
	// Sort posts by creation time
	posts := make([]*model.Post, 0, len(threadPosts.Posts))
	for _, p := range threadPosts.Posts {
		posts = append(posts, p)
	}
	
	// Sort by CreateAt timestamp
	for i := 0; i < len(posts); i++ {
		for j := i + 1; j < len(posts); j++ {
			if posts[i].CreateAt > posts[j].CreateAt {
				posts[i], posts[j] = posts[j], posts[i]
			}
		}
	}
	
	// Format each post with speaker identification
	for _, p := range posts {
		if p.Id == post.Id {
			continue // Skip the current message, we'll add it separately
		}
		
		// Get user info for this post
		user, _, err := b.client.GetUser(p.UserId, "")
		var speaker string
		if err != nil {
			speaker = "Unknown User"
		} else if p.UserId == b.config.BotUserID {
			speaker = "Assistant" // This is the bot
		} else {
			speaker = user.Username
		}
		
		contextBuilder.WriteString(fmt.Sprintf("%s: %s\n", speaker, p.Message))
	}
	
	// Add current message
	currentUser, _, err := b.client.GetUser(post.UserId, "")
	var currentSpeaker string
	if err != nil {
		currentSpeaker = "User"
	} else {
		currentSpeaker = currentUser.Username
	}
	
	contextBuilder.WriteString(fmt.Sprintf("\n%s: %s", currentSpeaker, post.Message))
	
	result := contextBuilder.String()
	log.Printf("[%s] THREAD: Built context with %d posts (%d chars)", time.Now().Format("2006-01-02 15:04:05"), len(posts), len(result))
	return result, nil
}

func (b *Bot) respondToMessage(post *model.Post) {
	ctx := context.Background()
	
	// Send typing indicator immediately
	parentID := ""
	if post.RootId != "" {
		parentID = post.RootId
	}
	
	if err := b.sendTypingIndicator(post.ChannelId, parentID); err != nil {
		log.Printf("[%s] WARNING: Failed to send typing indicator: %v", time.Now().Format("2006-01-02 15:04:05"), err)
	} else {
		log.Printf("[%s] TYPING: Sent typing indicator for channel %s", time.Now().Format("2006-01-02 15:04:05"), post.ChannelId)
	}
	
	// Get thread context for coherent responses
	prompt, err := b.getThreadContext(post)
	if err != nil {
		log.Printf("[%s] ERROR: Failed to get thread context: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		prompt = post.Message // Fallback to just the current message
	}
	
	// Get LLM response with full context
	response, err := b.llmBackend.Prompt(ctx, prompt)
	if err != nil {
		log.Printf("[%s] ERROR: LLM request failed: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		response = "I'm sorry, I'm having trouble processing your request right now. Please try again later."
	}
	
	log.Printf("[%s] OUTGOING: Sending response to channel %s: %s", 
		time.Now().Format("2006-01-02 15:04:05"), 
		post.ChannelId, 
		response[:min(100, len(response))]+"...")
	
	newPost := &model.Post{
		ChannelId: post.ChannelId,
		Message:   response,
	}
	
	// Handle thread creation and continuation
	if post.RootId != "" {
		// This is already part of a thread, continue in it
		newPost.RootId = post.RootId
		b.activeThreads[post.RootId] = true
		log.Printf("[%s] THREAD: Continuing in existing thread %s", time.Now().Format("2006-01-02 15:04:05"), post.RootId)
	} else if strings.Contains(post.Message, "@agent-bot") || strings.Contains(post.Message, b.config.BotUserID) {
		// This is a new mention, verify the post exists before creating thread
		if _, _, err := b.client.GetPost(post.Id, ""); err != nil {
			log.Printf("[%s] THREAD: Cannot create thread, post %s not accessible: %v", time.Now().Format("2006-01-02 15:04:05"), post.Id, err)
			// Don't set RootId, post as a regular message
		} else {
			newPost.RootId = post.Id
			b.activeThreads[post.Id] = true
			log.Printf("[%s] THREAD: Created thread for post %s", time.Now().Format("2006-01-02 15:04:05"), post.Id)
		}
	}

	if _, _, err := b.client.CreatePost(newPost); err != nil {
		log.Printf("[%s] ERROR: Failed to send message: %v", time.Now().Format("2006-01-02 15:04:05"), err)
	} else {
		log.Printf("[%s] SUCCESS: Message sent successfully", time.Now().Format("2006-01-02 15:04:05"))
	}
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
	}

	if config.ServerURL == "" || config.AccessToken == "" {
		log.Fatal("Missing required environment variables: MATTERMOST_SERVER_URL, MATTERMOST_ACCESS_TOKEN")
	}
	
	if config.AnthropicKey == "" {
		log.Fatal("Missing required environment variable: ANTHROPIC_API_KEY")
	}

	// Initialize LLM backend
	llmBackend := llms.NewAnthropicBackend(config.AnthropicKey)
	
	bot := NewBot(config, llmBackend)
	bot.start()
}