package llms

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// LLMBackend interface for pluggable LLM providers
type LLMBackend interface {
	Prompt(ctx context.Context, text string) (string, error)
}

// AnthropicBackend implements LLMBackend using Anthropic's Claude
type AnthropicBackend struct {
	client *anthropic.Client
	model  string
}

func NewAnthropicBackend(apiKey string) *AnthropicBackend {
	// Set API key as environment variable for the client
	os.Setenv("ANTHROPIC_API_KEY", apiKey)
	client := anthropic.NewClient()
	return &AnthropicBackend{
		client: &client,
		model:  "claude-sonnet-4-20250514", // Claude 4 Sonnet latest
	}
}

func (a *AnthropicBackend) Prompt(ctx context.Context, text string) (string, error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("[%s] LLM: Starting Anthropic API call", timestamp)
	log.Printf("[%s] LLM: Model: %s", timestamp, a.model)
	log.Printf("[%s] LLM: Input prompt (%d chars): %s", timestamp, len(text), text)
	log.Printf("[%s] LLM: Max tokens: 4096", timestamp)
	log.Printf("[%s] LLM: Web search enabled (max 3 searches)", timestamp)
	
	startTime := time.Now()
	
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(a.model),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(text)),
		},
		Tools: []anthropic.ToolUnionParam{
			{
				OfWebSearchTool20250305: &anthropic.WebSearchTool20250305Param{
					MaxUses: anthropic.Int(3), // Limit to 3 searches per request
				},
			},
		},
	})
	
	duration := time.Since(startTime)
	
	if err != nil {
		log.Printf("[%s] LLM: API call failed after %v: %v", timestamp, duration, err)
		return "", fmt.Errorf("anthropic API error: %v", err)
	}
	
	log.Printf("[%s] LLM: API call completed in %v", timestamp, duration)
	log.Printf("[%s] LLM: Response ID: %s", timestamp, resp.ID)
	log.Printf("[%s] LLM: Model used: %s", timestamp, resp.Model)
	log.Printf("[%s] LLM: Stop reason: %s", timestamp, resp.StopReason)
	log.Printf("[%s] LLM: Usage - Input tokens: %d, Output tokens: %d", timestamp, resp.Usage.InputTokens, resp.Usage.OutputTokens)
	log.Printf("[%s] LLM: Content blocks received: %d", timestamp, len(resp.Content))
	
	if len(resp.Content) == 0 {
		log.Printf("[%s] LLM: ERROR: No content blocks in response", timestamp)
		return "", fmt.Errorf("no content in response")
	}
	
	// Extract text from content blocks using proper type assertion
	var result strings.Builder
	for i, block := range resp.Content {
		log.Printf("[%s] LLM: Processing content block %d", timestamp, i)
		
		// Use proper type assertion to extract text
		switch content := block.AsAny().(type) {
		case anthropic.TextBlock:
			text := content.Text
			log.Printf("[%s] LLM: Extracted text from block %d (%d chars): %s", timestamp, i, len(text), text)
			result.WriteString(text)
		default:
			log.Printf("[%s] LLM: Block %d is not a text block, type: %T", timestamp, i, content)
		}
	}
	
	finalResult := result.String()
	if finalResult == "" {
		// Fallback if no text blocks found
		finalResult = "I received your message and processed it with Claude, but no text content was returned."
		log.Printf("[%s] LLM: No text content extracted, using fallback", timestamp)
	} else {
		log.Printf("[%s] LLM: Successfully extracted response text (%d chars total)", timestamp, len(finalResult))
	}
	
	return finalResult, nil
}