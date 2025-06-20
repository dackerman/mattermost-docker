# Agent Bot

A simple Mattermost bot that currently echoes messages. Built as a foundation for AI agent functionality.

## Setup

1. Copy environment file:
```bash
cp .env.example .env
```

2. Configure your bot in `.env`:
- `MATTERMOST_SERVER_URL`: Your Mattermost server URL
- `MATTERMOST_ACCESS_TOKEN`: Bot's access token from Mattermost
- `MATTERMOST_BOT_USER_ID`: Bot's user ID

3. Run the bot:
```bash
go run main.go
```

## Bot Creation

To create the bot account in Mattermost:
1. Go to System Console > Integrations > Bot Accounts
2. Enable "Bot Account Creation"
3. Create new bot account and copy the access token
4. Add bot to desired teams/channels

## Current Behavior

- Responds to mentions with "Echo: [original message]"
- Responds to direct messages
- Ignores its own messages

## Future Enhancements

This bot is designed to be enhanced with AI agent capabilities.