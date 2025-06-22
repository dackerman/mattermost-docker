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
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/invopop/jsonschema"

	"agent-bot/asana"
	"agent-bot/types"
)

// LLMBackend interface for pluggable LLM providers
type LLMBackend interface {
	Prompt(ctx context.Context, text string) (string, error)
	PromptStream(ctx context.Context, text string) (<-chan types.StreamChunk, error)
}

// AnthropicBackend implements LLMBackend using Anthropic's Claude
type AnthropicBackend struct {
	client       *anthropic.Client
	model        string
	maxTokens    int
	maxWebSearch int
	enableTools  bool
	asanaClient  *asana.Client
}

func NewAnthropicBackend(apiKey, asanaKey, model string, maxTokens, maxWebSearch int, enableTools bool) *AnthropicBackend {
	// Set API key as environment variable for the client
	os.Setenv("ANTHROPIC_API_KEY", apiKey)
	
	// Initialize client with MCP beta support
	client := anthropic.NewClient(
		option.WithHeader("anthropic-beta", "mcp-client-2025-04-04"),
	)
	
	// Initialize Asana client with required API key
	asanaClient := asana.NewClient(asanaKey, &http.Client{})
	
	return &AnthropicBackend{
		client:       &client,
		model:        model,
		maxTokens:    maxTokens,
		maxWebSearch: maxWebSearch,
		enableTools:  enableTools,
		asanaClient:  asanaClient,
	}
}

func (a *AnthropicBackend) Prompt(ctx context.Context, text string) (string, error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("[%s] LLM: Starting Anthropic API call", timestamp)
	log.Printf("[%s] LLM: Model: %s", timestamp, a.model)
	log.Printf("[%s] LLM: Input prompt (%d chars): %s", timestamp, len(text), text)
	log.Printf("[%s] LLM: Max tokens: %d", timestamp, a.maxTokens)
	if a.enableTools {
		log.Printf("[%s] LLM: Web search enabled (max %d searches)", timestamp, a.maxWebSearch)
	} else {
		log.Printf("[%s] LLM: Tools disabled", timestamp)
	}

	// Build tools array conditionally
	var tools []anthropic.BetaToolUnionParam
	if a.enableTools {
		tools = []anthropic.BetaToolUnionParam{
			{
				OfWebSearchTool20250305: &anthropic.BetaWebSearchTool20250305Param{
					MaxUses: anthropic.Int(int64(a.maxWebSearch)), // Configurable max searches per request
				},
			},
		}

		// Add Asana tools
		log.Printf("[%s] LLM: Adding Asana tools", timestamp)
		asanaTools := []anthropic.BetaToolUnionParam{
			{
				OfTool: &anthropic.BetaToolParam{
					Name:        "list_asana_projects",
					Description: anthropic.String("List projects in an Asana workspace"),
					InputSchema: ListProjectsBetaInputSchema,
				},
			},
			{
				OfTool: &anthropic.BetaToolParam{
					Name:        "list_asana_project_tasks",
					Description: anthropic.String("List incomplete tasks in an Asana project"),
					InputSchema: ListProjectTasksBetaInputSchema,
				},
			},
			{
				OfTool: &anthropic.BetaToolParam{
					Name:        "list_asana_user_tasks",
					Description: anthropic.String("List incomplete tasks assigned to a user in Asana"),
					InputSchema: ListUserTasksBetaInputSchema,
				},
			},
			{
				OfTool: &anthropic.BetaToolParam{
					Name:        "list_asana_users",
					Description: anthropic.String("List users in an Asana workspace to get their GIDs for other operations"),
					InputSchema: ListUsersBetaInputSchema,
				},
			},
		}
		tools = append(tools, asanaTools...)
	}

	// Initialize conversation
	messages := []anthropic.BetaMessageParam{
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(text)),
	}

	var finalResult strings.Builder

	// Tool use conversation loop
	for {
		startTime := time.Now()
		
		// Configure MCP servers
		var mcpServers []anthropic.BetaRequestMCPServerURLDefinitionParam
		if a.enableTools {
			log.Printf("[%s] LLM: Adding MCP server: hello-world-mcp", timestamp)
			mcpServers = []anthropic.BetaRequestMCPServerURLDefinitionParam{
				{
					Type: "url",
					URL:  "http://mcp-server:3000/mcp",
					Name: "hello-world-mcp",
					ToolConfiguration: anthropic.BetaRequestMCPServerToolConfigurationParam{
						Enabled: anthropic.Bool(true),
					},
				},
			}
		}
		
		params := anthropic.BetaMessageNewParams{
			Model:     anthropic.Model(a.model),
			MaxTokens: int64(a.maxTokens),
			Messages:  messages,
			MCPServers: mcpServers,
		}
		if a.enableTools && len(tools) > 0 {
			params.Tools = tools
		}
		resp, err := a.client.Beta.Messages.New(ctx, params)
		
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
			case anthropic.BetaTextBlock:
				text := content.Text
				log.Printf("[%s] LLM: Extracted text from block %d (%d chars): %s", timestamp, i, len(text), text)
				finalResult.WriteString(text)
			case anthropic.BetaToolUseBlock:
				log.Printf("[%s] LLM: Tool use block %d: %s", timestamp, i, content.Name)
				inputJSON, _ := json.Marshal(content.Input)
				log.Printf("[%s] LLM: Tool input: %s", timestamp, string(inputJSON))
			case anthropic.BetaMCPToolUseBlock:
				log.Printf("[%s] LLM: MCP tool use block %d: %s from server %s", timestamp, i, content.Name, content.ServerName)
				inputJSON, _ := json.Marshal(content.Input)
				log.Printf("[%s] LLM: MCP tool input: %s", timestamp, string(inputJSON))
			default:
				log.Printf("[%s] LLM: Block %d is not a text or tool use block, type: %T", timestamp, i, content)
			}
		}

		// Add the assistant's response to the conversation
		messages = append(messages, resp.ToParam())

		// Handle tool use
		toolResults := []anthropic.BetaContentBlockParamUnion{}
		
		for _, block := range resp.Content {
			switch content := block.AsAny().(type) {
			case anthropic.BetaToolUseBlock:
				log.Printf("[%s] LLM: Executing tool: %s", timestamp, content.Name)
				
				var response interface{}
				var err error
				
				switch content.Name {
				case "list_asana_projects":
					var input asana.ListProjectsArgs
					inputBytes, _ := json.Marshal(content.Input)
					if err := json.Unmarshal(inputBytes, &input); err == nil {
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
					inputBytes, _ := json.Marshal(content.Input)
					if err := json.Unmarshal(inputBytes, &input); err == nil {
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
					inputBytes, _ := json.Marshal(content.Input)
					if err := json.Unmarshal(inputBytes, &input); err == nil {
						tasks, err := a.asanaClient.ListUserTasks(input.AssigneeGID, input.WorkspaceGID)
						if err != nil {
							response = fmt.Sprintf("Error listing user tasks: %v", err)
						} else {
							response = tasks
						}
					} else {
						response = fmt.Sprintf("Invalid input: %v", err)
					}
					
				case "list_asana_users":
					var input asana.ListUsersArgs
					inputBytes, _ := json.Marshal(content.Input)
					if err := json.Unmarshal(inputBytes, &input); err == nil {
						users, err := a.asanaClient.ListUsers(input.WorkspaceGID)
						if err != nil {
							response = fmt.Sprintf("Error listing users: %v", err)
						} else {
							response = users
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
				toolResults = append(toolResults, anthropic.NewBetaToolResultBlock(content.ID, string(b), false))
			case anthropic.BetaMCPToolUseBlock:
				log.Printf("[%s] LLM: Executing MCP tool: %s from server: %s", timestamp, content.Name, content.ServerName)
				
				// For MCP tools, the tool execution is handled by the Anthropic API
				// We just need to add the MCP tool result block
				log.Printf("[%s] LLM: MCP tool will be executed automatically by API", timestamp)
				// No explicit handling needed for MCP tools - they're executed by the API
			}
		}
		
		// If no tool results, break the loop
		if len(toolResults) == 0 {
			break
		}
		
		// Add tool results to conversation and continue
		messages = append(messages, anthropic.NewBetaUserMessage(toolResults...))
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

// PromptStream provides streaming responses from the LLM
// For now, this simulates streaming by chunking the regular API response
// TODO: Implement true streaming when the SDK documentation is clarified
func (a *AnthropicBackend) PromptStream(ctx context.Context, text string) (<-chan types.StreamChunk, error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	log.Printf("[%s] LLM_STREAM: Starting simulated streaming response", timestamp)
	log.Printf("[%s] LLM_STREAM: Model: %s", timestamp, a.model)
	log.Printf("[%s] LLM_STREAM: Input prompt (%d chars): %s", timestamp, len(text), text)
	log.Printf("[%s] LLM_STREAM: Max tokens: %d", timestamp, a.maxTokens)
	
	// For now, we'll simulate streaming by using the regular API and chunking the response
	// This provides the streaming user experience while we work on true streaming integration
	log.Printf("[%s] LLM_STREAM: Using simulated streaming (chunked response)", timestamp)

	// Create output channel
	chunkChan := make(chan types.StreamChunk, 10) // Buffered channel

	// Start simulated streaming in a goroutine
	go func() {
		defer close(chunkChan)

		startTime := time.Now()
		
		// Get the full response using the regular API
		response, err := a.Prompt(ctx, text)
		if err != nil {
			log.Printf("[%s] LLM_STREAM: API call failed: %v", timestamp, err)
			select {
			case chunkChan <- types.StreamChunk{
				Content: "",
				Done:    true,
				Error:   fmt.Errorf("API error: %v", err),
			}:
			case <-ctx.Done():
			}
			return
		}

		duration := time.Since(startTime)
		log.Printf("[%s] LLM_STREAM: Got response (%d chars) in %v, now chunking", timestamp, len(response), duration)

		// Simulate streaming by sending chunks of the response
		chunkSize := 10 // Characters per chunk
		chunkDelay := 50 * time.Millisecond // Delay between chunks

		for i := 0; i < len(response); i += chunkSize {
			select {
			case <-ctx.Done():
				log.Printf("[%s] LLM_STREAM: Context cancelled during chunking", timestamp)
				return
			default:
			}

			end := i + chunkSize
			if end > len(response) {
				end = len(response)
			}

			chunk := response[i:end]
			
			// Send chunk
			select {
			case chunkChan <- types.StreamChunk{
				Content: chunk,
				Done:    false,
				Error:   nil,
			}:
			case <-ctx.Done():
				log.Printf("[%s] LLM_STREAM: Context cancelled while sending chunk", timestamp)
				return
			}

			// Add delay between chunks for realistic streaming effect
			if i+chunkSize < len(response) {
				time.Sleep(chunkDelay)
			}
		}

		// Send completion signal
		log.Printf("[%s] LLM_STREAM: Finished streaming %d chars", timestamp, len(response))
		select {
		case chunkChan <- types.StreamChunk{
			Content: "",
			Done:    true,
			Error:   nil,
		}:
		case <-ctx.Done():
			return
		}
	}()

	return chunkChan, nil
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

// GenerateBetaSchema creates a beta schema for tool input validation
func GenerateBetaSchema[T any]() anthropic.BetaToolInputSchemaParam {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return anthropic.BetaToolInputSchemaParam{
		Properties: schema.Properties,
	}
}

// Asana tool schemas
var ListProjectsInputSchema = GenerateSchema[asana.ListProjectsArgs]()
var ListProjectTasksInputSchema = GenerateSchema[asana.ListProjectTasksArgs]()
var ListUserTasksInputSchema = GenerateSchema[asana.ListUserTasksArgs]()
var ListUsersInputSchema = GenerateSchema[asana.ListUsersArgs]()

// Beta Asana tool schemas
var ListProjectsBetaInputSchema = GenerateBetaSchema[asana.ListProjectsArgs]()
var ListProjectTasksBetaInputSchema = GenerateBetaSchema[asana.ListProjectTasksArgs]()
var ListUserTasksBetaInputSchema = GenerateBetaSchema[asana.ListUserTasksArgs]()
var ListUsersBetaInputSchema = GenerateBetaSchema[asana.ListUsersArgs]()