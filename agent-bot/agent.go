package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"agent-bot/types"
)

// BotAgent implements the Agent interface to handle incoming messages
type BotAgent struct {
	botUserID     string
	llm           types.LLM
	chat          types.Chat
	activeThreads map[string]bool
	lastCleanup   time.Time
}

// NewBotAgent creates a new agent that handles messages
func NewBotAgent(botUserID string, llm types.LLM, chat types.Chat) *BotAgent {
	return &BotAgent{
		botUserID:     botUserID,
		llm:           llm,
		chat:          chat,
		activeThreads: make(map[string]bool),
		lastCleanup:   time.Now(),
	}
}

// MessagePosted handles incoming messages from the websocket
func (a *BotAgent) MessagePosted(message types.PostedMessage) {
	// Periodically clean up stale thread references
	a.cleanupStaleThreads()

	// Log all incoming messages
	log.Printf("[%s] INCOMING: Message in channel %s: %s",
		time.Now().Format("2006-01-02 15:04:05"),
		message.ChannelId,
		message.Message)

	// Check if we should respond
	shouldRespond := a.shouldRespond(message)

	if shouldRespond {
		a.logResponseReason(message)
		a.respondToMessage(message)
	} else {
		log.Printf("[%s] SKIP: No mention/DM/thread participation needed", time.Now().Format("2006-01-02 15:04:05"))
	}
}

func (a *BotAgent) shouldRespond(message types.PostedMessage) bool {
	// Check various conditions for responding
	isMentioned := strings.Contains(message.Message, "@agent-bot") || strings.Contains(message.Message, a.botUserID)
	isInActiveThread := a.activeThreads[message.ThreadId] && message.ThreadId != ""

	shouldRespond := isMentioned || message.IsDM

	// Also respond in active threads, but decide if we should based on content
	if !shouldRespond && isInActiveThread {
		shouldRespond = a.shouldRespondInThread(message)
	}

	return shouldRespond
}

func (a *BotAgent) logResponseReason(message types.PostedMessage) {
	isMentioned := strings.Contains(message.Message, "@agent-bot") || strings.Contains(message.Message, a.botUserID)
	isInActiveThread := a.activeThreads[message.ThreadId] && message.ThreadId != ""

	if isMentioned {
		log.Printf("[%s] MENTION: Bot mentioned, preparing response", time.Now().Format("2006-01-02 15:04:05"))
	} else if message.IsDM {
		log.Printf("[%s] DM: Direct message received, preparing response", time.Now().Format("2006-01-02 15:04:05"))
	} else if isInActiveThread {
		log.Printf("[%s] THREAD: Responding in active thread", time.Now().Format("2006-01-02 15:04:05"))
	}
}

func (a *BotAgent) shouldRespondInThread(message types.PostedMessage) bool {
	// Simple heuristic: respond if the message seems to be asking a question or addressing the conversation
	msg := strings.ToLower(message.Message)

	// Question indicators
	if strings.Contains(msg, "?") {
		return true
	}

	// Direct conversation indicators
	questionWords := []string{"how", "what", "when", "where", "why", "who", "can you", "could you", "would you", "do you"}
	for _, word := range questionWords {
		if strings.Contains(msg, word) {
			return true
		}
	}

	// Skip if it seems like casual conversation between others
	if len(msg) < 10 || strings.Contains(msg, "lol") || strings.Contains(msg, "haha") {
		return false
	}

	return true // Default to participating in active threads
}

func (a *BotAgent) respondToMessage(message types.PostedMessage) {
	// Send typing indicator
	a.sendTypingIndicator(message.ChannelId, message.ThreadId)

	// Get thread context for coherent responses
	prompt, err := a.getThreadContext(message)
	if err != nil {
		log.Printf("[%s] ERROR: Failed to get thread context: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		prompt = message.Message // Fallback to just the current message
	}

	// Get LLM response with full context
	response, err := a.llm.Prompt(prompt)
	if err != nil {
		log.Printf("[%s] ERROR: LLM request failed: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		response = "I'm sorry, I'm having trouble processing your request right now. Please try again later."
	}

	log.Printf("[%s] OUTGOING: Sending response to channel %s: %s",
		time.Now().Format("2006-01-02 15:04:05"),
		message.ChannelId,
		response[:min(100, len(response))]+"...")

	// Prepare chat message
	chatMsg := types.ChatMessage{
		ChannelId: message.ChannelId,
		Message:   response,
	}

	// Handle thread creation and continuation
	if message.ThreadId != "" {
		// This is already part of a thread, continue in it
		chatMsg.ThreadId = message.ThreadId
		a.activeThreads[message.ThreadId] = true
		log.Printf("[%s] THREAD: Continuing in existing thread %s", time.Now().Format("2006-01-02 15:04:05"), message.ThreadId)
	} else if strings.Contains(message.Message, "@agent-bot") || strings.Contains(message.Message, a.botUserID) {
		// This is a new mention, create a thread
		if a.canCreateThread(message.PostId) {
			chatMsg.ThreadId = message.PostId
			a.activeThreads[message.PostId] = true
			log.Printf("[%s] THREAD: Created thread for post %s", time.Now().Format("2006-01-02 15:04:05"), message.PostId)
		}
	}

	// Send the response
	if err := a.chat.PostMessage(chatMsg); err != nil {
		log.Printf("[%s] ERROR: Failed to send message: %v", time.Now().Format("2006-01-02 15:04:05"), err)
	} else {
		log.Printf("[%s] SUCCESS: Message sent successfully", time.Now().Format("2006-01-02 15:04:05"))
	}
}

func (a *BotAgent) sendTypingIndicator(channelID, threadID string) {
	if err := a.chat.SendTypingIndicator(channelID, threadID); err != nil {
		log.Printf("[%s] WARNING: Failed to send typing indicator: %v", time.Now().Format("2006-01-02 15:04:05"), err)
	} else {
		log.Printf("[%s] TYPING: Sent typing indicator for channel %s", time.Now().Format("2006-01-02 15:04:05"), channelID)
	}
}

func (a *BotAgent) canCreateThread(postID string) bool {
	// Verify the post exists before creating thread
	if _, err := a.chat.GetMessage(postID); err != nil {
		log.Printf("[%s] THREAD: Cannot create thread, post %s not accessible: %v", time.Now().Format("2006-01-02 15:04:05"), postID, err)
		return false
	}
	return true
}

func (a *BotAgent) getThreadContext(message types.PostedMessage) (string, error) {
	// If this is not a threaded message, just return the current message
	rootId := message.ThreadId
	if rootId == "" {
		rootId = message.PostId // If this will become the root of a new thread
	}

	// Get all posts in the thread
	posts, err := a.chat.GetThreadMessages(rootId)
	if err != nil {
		log.Printf("[%s] THREAD: Failed to get thread context: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		return message.Message, nil // Fallback to just the current message
	}

	// Build context string
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Previous conversation context:\n\n")

	// Sort posts by timestamp
	for i := 0; i < len(posts); i++ {
		for j := i + 1; j < len(posts); j++ {
			if posts[i].Timestamp > posts[j].Timestamp {
				posts[i], posts[j] = posts[j], posts[i]
			}
		}
	}

	// Format each post with speaker identification
	for _, p := range posts {
		if p.ID == message.PostId {
			continue // Skip the current message, we'll add it separately
		}

		// Get user info for this post
		user, err := a.chat.GetUser(p.UserID)
		var speaker string
		if err != nil {
			speaker = "Unknown User"
		} else if p.UserID == a.botUserID {
			speaker = "Assistant" // This is the bot
		} else {
			speaker = user.Username
		}

		contextBuilder.WriteString(fmt.Sprintf("%s: %s\n", speaker, p.Content))
	}

	// Add current message with speaker info
	if message.UserId != "" {
		user, err := a.chat.GetUser(message.UserId)
		var speaker string
		if err != nil {
			speaker = "User"
		} else {
			speaker = user.Username
		}
		contextBuilder.WriteString(fmt.Sprintf("\n%s: %s", speaker, message.Message))
	} else {
		contextBuilder.WriteString(fmt.Sprintf("\nUser: %s", message.Message))
	}

	result := contextBuilder.String()
	log.Printf("[%s] THREAD: Built context with %d posts (%d chars)", time.Now().Format("2006-01-02 15:04:05"), len(posts), len(result))
	return result, nil
}

func (a *BotAgent) cleanupStaleThreads() {
	// Clean up stale thread tracking every 10 minutes
	if time.Since(a.lastCleanup) < 10*time.Minute {
		return
	}

	log.Printf("[%s] CLEANUP: Cleaning up stale thread references", time.Now().Format("2006-01-02 15:04:05"))

	// Test a few thread IDs to see if they're still accessible
	staleThreads := make([]string, 0)
	count := 0
	for threadId := range a.activeThreads {
		if count >= 5 { // Only check first 5 to avoid too many API calls
			break
		}
		if _, err := a.chat.GetMessage(threadId); err != nil {
			staleThreads = append(staleThreads, threadId)
		}
		count++
	}

	// Remove stale threads
	for _, threadId := range staleThreads {
		delete(a.activeThreads, threadId)
		log.Printf("[%s] CLEANUP: Removed stale thread %s", time.Now().Format("2006-01-02 15:04:05"), threadId)
	}

	a.lastCleanup = time.Now()
	log.Printf("[%s] CLEANUP: Completed, %d active threads remaining", time.Now().Format("2006-01-02 15:04:05"), len(a.activeThreads))
}