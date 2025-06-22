package llms

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/invopop/jsonschema"

	"agent-bot/asana"
)

// LLMBackend interface for pluggable LLM providers
type LLMBackend interface {
	Prompt(ctx context.Context, text string) (string, error)
}

// AnthropicBackend implements LLMBackend using Anthropic's Claude
type AnthropicBackend struct {
	client      *anthropic.Client
	model       string
	asanaClient *asana.Client
}

func NewAnthropicBackend(apiKey, asanaKey string) *AnthropicBackend {
	// Set API key as environment variable for the client
	os.Setenv("ANTHROPIC_API_KEY", apiKey)
	client := anthropic.NewClient()
	
	// Initialize Asana client with required API key
	asanaClient := asana.NewClient(asanaKey, &http.Client{})
	
	return &AnthropicBackend{
		client:      &client,
		model:       "claude-sonnet-4-20250514", // Claude 4 Sonnet latest
		asanaClient: asanaClient,
	}
}

func (a *AnthropicBackend) Prompt(ctx context.Context, text string) (string, error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("[%s] LLM: Starting Anthropic API call", timestamp)
	log.Printf("[%s] LLM: Model: %s", timestamp, a.model)
	log.Printf("[%s] LLM: Input prompt (%d chars): %s", timestamp, len(text), text)
	log.Printf("[%s] LLM: Max tokens: 4096", timestamp)
	log.Printf("[%s] LLM: Web search enabled (max 3 searches)", timestamp)

	// Build tools array
	tools := []anthropic.ToolUnionParam{
		{
			OfWebSearchTool20250305: &anthropic.WebSearchTool20250305Param{
				MaxUses: anthropic.Int(3), // Limit to 3 searches per request
			},
		},
	}

	// Add Asana tools
	log.Printf("[%s] LLM: Adding Asana tools", timestamp)
	asanaTools := []anthropic.ToolUnionParam{
		{
			OfTool: &anthropic.ToolParam{
				Name:        "list_asana_projects",
				Description: anthropic.String("List projects in an Asana workspace"),
				InputSchema: ListProjectsInputSchema,
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "list_asana_project_tasks",
				Description: anthropic.String("List tasks in an Asana project"),
				InputSchema: ListProjectTasksInputSchema,
			},
		},
		{
			OfTool: &anthropic.ToolParam{
				Name:        "list_asana_user_tasks",
				Description: anthropic.String("List tasks assigned to a user in Asana"),
				InputSchema: ListUserTasksInputSchema,
			},
		},
	}
	tools = append(tools, asanaTools...)

	// Initialize conversation
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(text)),
	}

	var finalResult strings.Builder

	// Tool use conversation loop
	for {
		startTime := time.Now()
		
		resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(a.model),
			MaxTokens: 4096,
			Messages:  messages,
			Tools:     tools,
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

		// Process response blocks
		for i, block := range resp.Content {
			log.Printf("[%s] LLM: Processing content block %d", timestamp, i)
			
			switch content := block.AsAny().(type) {
			case anthropic.TextBlock:
				text := content.Text
				log.Printf("[%s] LLM: Extracted text from block %d (%d chars): %s", timestamp, i, len(text), text)
				finalResult.WriteString(text)
			case anthropic.ToolUseBlock:
				log.Printf("[%s] LLM: Tool use block %d: %s", timestamp, i, content.Name)
				inputJSON, _ := json.Marshal(content.Input)
				log.Printf("[%s] LLM: Tool input: %s", timestamp, string(inputJSON))
			default:
				log.Printf("[%s] LLM: Block %d is not a text or tool use block, type: %T", timestamp, i, content)
			}
		}

		// Add the assistant's response to the conversation
		messages = append(messages, resp.ToParam())

		// Handle tool use
		toolResults := []anthropic.ContentBlockParamUnion{}
		
		for _, block := range resp.Content {
			switch content := block.AsAny().(type) {
			case anthropic.ToolUseBlock:
				log.Printf("[%s] LLM: Executing tool: %s", timestamp, content.Name)
				
				var response interface{}
				var err error
				
				switch content.Name {
				case "list_asana_projects":
					var input asana.ListProjectsArgs
					if err := json.Unmarshal(content.Input, &input); err == nil {
						projects, err := a.asanaClient.ListProjects(input.WorkspaceGID)
						if err != nil {
							response = fmt.Sprintf("Error listing projects: %v", err)
						} else {
							response = projects
						}
					} else {
						response = fmt.Sprintf("Invalid input: %v", err)
					}
					
				case "list_asana_project_tasks":
					var input asana.ListProjectTasksArgs
					if err := json.Unmarshal(content.Input, &input); err == nil {
						tasks, err := a.asanaClient.ListProjectTasks(input.ProjectGID)
						if err != nil {
							response = fmt.Sprintf("Error listing project tasks: %v", err)
						} else {
							response = tasks
						}
					} else {
						response = fmt.Sprintf("Invalid input: %v", err)
					}
					
				case "list_asana_user_tasks":
					var input asana.ListUserTasksArgs
					if err := json.Unmarshal(content.Input, &input); err == nil {
						tasks, err := a.asanaClient.ListUserTasks(input.AssigneeGID, input.WorkspaceGID)
						if err != nil {
							response = fmt.Sprintf("Error listing user tasks: %v", err)
						} else {
							response = tasks
						}
					} else {
						response = fmt.Sprintf("Invalid input: %v", err)
					}
				}
				
				// Convert response to JSON and add as tool result
				b, err := json.Marshal(response)
				if err != nil {
					b = []byte(fmt.Sprintf("Error marshalling response: %v", err))
				}
				
				log.Printf("[%s] LLM: Tool result: %s", timestamp, string(b))
				toolResults = append(toolResults, anthropic.NewToolResultBlock(content.ID, string(b), false))
			}
		}
		
		// If no tool results, break the loop
		if len(toolResults) == 0 {
			break
		}
		
		// Add tool results to conversation and continue
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}

	result := finalResult.String()
	if result == "" {
		// Fallback if no text blocks found
		result = "I received your message and processed it with Claude, but no text content was returned."
		log.Printf("[%s] LLM: No text content extracted, using fallback", timestamp)
	} else {
		log.Printf("[%s] LLM: Successfully extracted response text (%d chars total)", timestamp, len(result))
	}
	
	return result, nil
}

// GenerateSchema creates a schema for tool input validation
func GenerateSchema[T any]() anthropic.ToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return anthropic.ToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

// Asana tool schemas
var ListProjectsInputSchema = GenerateSchema[asana.ListProjectsArgs]()
var ListProjectTasksInputSchema = GenerateSchema[asana.ListProjectTasksArgs]()
var ListUserTasksInputSchema = GenerateSchema[asana.ListUserTasksArgs]()