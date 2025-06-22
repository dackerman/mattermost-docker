package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"agent-bot/types"
)

// BotAgent implements the Agent interface to handle incoming messages
type BotAgent struct {
	botUserID      string
	botUsername    string
	botDisplayName string
	llm            types.LLM
	decisionLLM    types.LLM
	chat           types.Chat
	activeThreads  map[string]bool
	lastCleanup    time.Time
}

// NewBotAgent creates a new agent that handles messages
func NewBotAgent(botUserID, botUsername, botDisplayName string, llm types.LLM, decisionLLM types.LLM, chat types.Chat) *BotAgent {
	return &BotAgent{
		botUserID:      botUserID,
		botUsername:    botUsername,
		botDisplayName: botDisplayName,
		llm:            llm,
		decisionLLM:    decisionLLM,
		chat:           chat,
		activeThreads:  make(map[string]bool),
		lastCleanup:    time.Now(),
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
	// Check for direct mentions and DMs first - always respond to these
	mention := "@" + a.botUsername
	isMentioned := strings.Contains(message.Message, mention) || strings.Contains(message.Message, a.botUserID)
	
	if isMentioned || message.IsDM {
		return true
	}

	// For active threads, use LLM to decide if we should respond
	isInActiveThread := a.activeThreads[message.ThreadId] && message.ThreadId != ""
	if isInActiveThread {
		return a.shouldRespondInThreadLLM(message)
	}

	return false
}

func (a *BotAgent) logResponseReason(message types.PostedMessage) {
	mention := "@" + a.botUsername
	isMentioned := strings.Contains(message.Message, mention) || strings.Contains(message.Message, a.botUserID)
	isInActiveThread := a.activeThreads[message.ThreadId] && message.ThreadId != ""

	if isMentioned {
		log.Printf("[%s] MENTION: Bot mentioned, preparing response", time.Now().Format("2006-01-02 15:04:05"))
	} else if message.IsDM {
		log.Printf("[%s] DM: Direct message received, preparing response", time.Now().Format("2006-01-02 15:04:05"))
	} else if isInActiveThread {
		log.Printf("[%s] THREAD: Responding in active thread", time.Now().Format("2006-01-02 15:04:05"))
	}
}

// shouldRespondInThreadLLM uses a fast LLM to decide if we should respond in an active thread
func (a *BotAgent) shouldRespondInThreadLLM(message types.PostedMessage) bool {
	// Get recent thread context for decision making
	context, err := a.getThreadContext(message)
	if err != nil {
		log.Printf("[%s] DECISION: Failed to get thread context, defaulting to simple heuristic: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		return a.shouldRespondInThreadFallback(message)
	}

	// Create a focused prompt for the decision LLM
	decisionPrompt := fmt.Sprintf(`You are a chat bot assistant. Based on this conversation context, should you respond to the latest message?

Context:
%s

Your bot username is "%s" and display name is "%s".

Respond with ONLY "YES" if you should respond (if the message is:
- A direct question to anyone
- Asking for help or information
- Continuing a conversation you're already part of
- Requesting an action or task

Respond with ONLY "NO" if you should not respond (if the message is:
- Casual conversation between others
- Off-topic chatter
- Simple acknowledgments like "ok", "thanks", "lol"
- Private conversation between specific people

Answer:`, context, a.botUsername, a.botDisplayName)

	// Use the fast decision LLM
	response, err := a.decisionLLM.Prompt(decisionPrompt)
	if err != nil {
		log.Printf("[%s] DECISION: LLM call failed, using fallback: %v", time.Now().Format("2006-01-02 15:04:05"), err)
		return a.shouldRespondInThreadFallback(message)
	}

	// Parse the response
	response = strings.TrimSpace(strings.ToUpper(response))
	shouldRespond := strings.Contains(response, "YES")
	
	log.Printf("[%s] DECISION: LLM response '%s' -> %v", time.Now().Format("2006-01-02 15:04:05"), response, shouldRespond)
	return shouldRespond
}

// shouldRespondInThreadFallback is the original simple heuristic as a fallback
func (a *BotAgent) shouldRespondInThreadFallback(message types.PostedMessage) bool {
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

	// Use streaming response
	a.respondWithStream(message, prompt)
}

// respondWithStream handles streaming LLM responses with periodic message updates
func (a *BotAgent) respondWithStream(message types.PostedMessage, prompt string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("[%s] STREAM: Starting streaming response", timestamp)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start the streaming request
	chunkChan, err := a.llm.PromptStream(ctx, prompt)
	if err != nil {
		log.Printf("[%s] ERROR: Failed to start streaming: %v", timestamp, err)
		// Fallback to non-streaming response
		a.respondWithFallback(message, prompt)
		return
	}

	// Create initial empty message
	initialMsg := types.ChatMessage{
		ChannelId: message.ChannelId,
		Message:   "_Thinking..._", // Markdown italic placeholder
	}

	// Handle thread creation and continuation
	if message.ThreadId != "" {
		// This is already part of a thread, continue in it
		initialMsg.ThreadId = message.ThreadId
		a.activeThreads[message.ThreadId] = true
		log.Printf("[%s] THREAD: Continuing in existing thread %s", timestamp, message.ThreadId)
	} else if strings.Contains(message.Message, "@"+a.botUsername) || strings.Contains(message.Message, a.botUserID) {
		// This is a new mention, create a thread
		if a.canCreateThread(message.PostId) {
			initialMsg.ThreadId = message.PostId
			a.activeThreads[message.PostId] = true
			log.Printf("[%s] THREAD: Created thread for post %s", timestamp, message.PostId)
		}
	}

	// Post initial message
	if err := a.chat.PostMessage(initialMsg); err != nil {
		log.Printf("[%s] ERROR: Failed to post initial message: %v", timestamp, err)
		return
	}

	// We need to get the message ID of the posted message
	// For now, we'll use a simple approach and get the latest message in the channel
	// This could be improved by having PostMessage return the message ID
	var messageID string
	time.Sleep(100 * time.Millisecond) // Small delay to ensure message is posted
	
	// Try to get recent messages to find our message ID
	// This is a simplified approach - in a real implementation, PostMessage should return the ID
	if threadMessages, err := a.chat.GetThreadMessages(initialMsg.ThreadId); err == nil && len(threadMessages) > 0 {
		// Get the last message (should be ours)
		messageID = threadMessages[len(threadMessages)-1].ID
	} else {
		// Fallback - we can't update without message ID
		log.Printf("[%s] WARNING: Could not get message ID for updates, proceeding without streaming updates", timestamp)
		a.respondWithFallback(message, prompt)
		return
	}

	log.Printf("[%s] STREAM: Got message ID %s for updates", timestamp, messageID)

	// Start streaming and updating
	a.processStream(ctx, chunkChan, messageID, timestamp)
}

// processStream handles the streaming response and periodic updates
func (a *BotAgent) processStream(ctx context.Context, chunkChan <-chan types.StreamChunk, messageID string, timestamp string) {
	var responseBuffer strings.Builder
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastUpdate := time.Now()
	updateInterval := 1 * time.Second

	log.Printf("[%s] STREAM: Starting to process chunks", timestamp)

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				// Channel closed, stream ended
				log.Printf("[%s] STREAM: Channel closed, finalizing", timestamp)
				a.finalizeStreamResponse(messageID, responseBuffer.String(), timestamp)
				return
			}

			if chunk.Error != nil {
				log.Printf("[%s] STREAM: Error received: %v", timestamp, chunk.Error)
				a.finalizeStreamResponse(messageID, responseBuffer.String()+"\n\n_Error: Failed to complete response_", timestamp)
				return
			}

			if chunk.Done {
				log.Printf("[%s] STREAM: Received completion signal", timestamp)
				a.finalizeStreamResponse(messageID, responseBuffer.String(), timestamp)
				return
			}

			// Append new content
			if chunk.Content != "" {
				responseBuffer.WriteString(chunk.Content)
				log.Printf("[%s] STREAM: Added chunk (%d chars), total: %d chars", timestamp, len(chunk.Content), responseBuffer.Len())
			}

		case <-ticker.C:
			// Periodic update
			if time.Since(lastUpdate) >= updateInterval && responseBuffer.Len() > 0 {
				currentResponse := responseBuffer.String()
				if err := a.chat.UpdateMessage(messageID, currentResponse); err != nil {
					log.Printf("[%s] STREAM: Failed to update message: %v", timestamp, err)
				} else {
					log.Printf("[%s] STREAM: Updated message (%d chars)", timestamp, len(currentResponse))
					lastUpdate = time.Now()
				}
			}

		case <-ctx.Done():
			log.Printf("[%s] STREAM: Context cancelled", timestamp)
			a.finalizeStreamResponse(messageID, responseBuffer.String()+"\n\n_Response cancelled_", timestamp)
			return
		}
	}
}

// finalizeStreamResponse sends the final update and logs completion
func (a *BotAgent) finalizeStreamResponse(messageID string, finalContent string, timestamp string) {
	if finalContent == "" {
		finalContent = "_No response generated_"
	}

	if err := a.chat.UpdateMessage(messageID, finalContent); err != nil {
		log.Printf("[%s] STREAM: Failed to finalize message: %v", timestamp, err)
	} else {
		log.Printf("[%s] STREAM: Response completed (%d chars total)", timestamp, len(finalContent))
	}
}

// respondWithFallback uses the original non-streaming approach
func (a *BotAgent) respondWithFallback(message types.PostedMessage, prompt string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("[%s] FALLBACK: Using non-streaming response", timestamp)

	// Get LLM response with full context
	response, err := a.llm.Prompt(prompt)
	if err != nil {
		log.Printf("[%s] ERROR: LLM request failed: %v", timestamp, err)
		response = "I'm sorry, I'm having trouble processing your request right now. Please try again later."
	}

	log.Printf("[%s] OUTGOING: Sending fallback response to channel %s: %s",
		timestamp,
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
		log.Printf("[%s] THREAD: Continuing in existing thread %s", timestamp, message.ThreadId)
	} else if strings.Contains(message.Message, "@"+a.botUsername) || strings.Contains(message.Message, a.botUserID) {
		// This is a new mention, create a thread
		if a.canCreateThread(message.PostId) {
			chatMsg.ThreadId = message.PostId
			a.activeThreads[message.PostId] = true
			log.Printf("[%s] THREAD: Created thread for post %s", timestamp, message.PostId)
		}
	}

	// Send the response
	if err := a.chat.PostMessage(chatMsg); err != nil {
		log.Printf("[%s] ERROR: Failed to send message: %v", timestamp, err)
	} else {
		log.Printf("[%s] SUCCESS: Message sent successfully", timestamp)
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
			speaker = a.botDisplayName // This is the bot
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