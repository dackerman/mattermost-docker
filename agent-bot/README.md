# Agent Bot

A Mattermost bot powered by Anthropic's Claude AI with robust WebSocket connectivity and thread management.

## Features

- **AI-Powered Responses**: Uses Anthropic's Claude for intelligent conversation
- **Thread Management**: Creates and participates in threaded conversations
- **WebSocket Auto-Reconnection**: Survives server restarts automatically
- **Pluggable LLM Backend**: Easy to swap different AI providers
- **Smart Response Logic**: Decides when to participate in conversations

## Setup

1. Copy environment file:
```bash
cp .env.example .env
```

2. Configure your bot in `.env`:
- `MATTERMOST_SERVER_URL`: Your Mattermost server URL
- `MATTERMOST_ACCESS_TOKEN`: Bot's access token from Mattermost
- `MATTERMOST_BOT_USER_ID`: Bot's user ID
- `ANTHROPIC_API_KEY`: Your Anthropic API key

3. Run the bot:
```bash
go run main.go
```

## Bot Behavior

- **@mentions**: Responds to direct mentions and creates threads
- **Direct Messages**: Responds to all DMs
- **Thread Participation**: Continues conversations in active threads
- **Smart Filtering**: Uses heuristics to decide when to respond

## Architecture

- **LLMBackend Interface**: Pluggable design for different AI providers
- **AnthropicBackend**: Current implementation using Claude
- **Thread Tracking**: Maintains state of active conversations
- **Connection Recovery**: Automatic WebSocket reconnection

## Health Monitoring

Check bot status: `curl http://localhost:8081/health`
- Returns "OK" if WebSocket connected
- Returns "WebSocket Disconnected" if connection lost

## Future Enhancements

- Additional LLM backends (OpenAI, etc.)
- Conversation context management
- Enhanced thread logic
- Message history integration