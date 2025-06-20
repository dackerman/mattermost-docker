#!/bin/bash

# Setup script for Agent Bot
# This script helps create a bot account in Mattermost and configure the environment

echo "Agent Bot Setup"
echo "==============="

# Check if required environment variables are set
if [ -z "$MATTERMOST_SERVER_URL" ]; then
    echo "Enter your Mattermost server URL (e.g., http://localhost:8065):"
    read -r MATTERMOST_SERVER_URL
fi

if [ -z "$MATTERMOST_ADMIN_TOKEN" ]; then
    echo "Enter your Mattermost admin access token:"
    read -r MATTERMOST_ADMIN_TOKEN
fi

echo "Creating bot account..."

# Create bot account via API
BOT_RESPONSE=$(curl -s -X POST \
    -H "Authorization: Bearer $MATTERMOST_ADMIN_TOKEN" \
    -H "Content-Type: application/json" \
    -d '{
        "username": "agent-bot",
        "display_name": "Agent Bot",
        "description": "AI Agent Bot for Mattermost"
    }' \
    "$MATTERMOST_SERVER_URL/api/v4/bots")

# Extract bot user ID and access token
BOT_USER_ID=$(echo "$BOT_RESPONSE" | grep -o '"user_id":"[^"]*"' | cut -d'"' -f4)
ACCESS_TOKEN=$(echo "$BOT_RESPONSE" | grep -o '"access_token":"[^"]*"' | cut -d'"' -f4)

if [ -n "$BOT_USER_ID" ] && [ -n "$ACCESS_TOKEN" ]; then
    echo "Bot created successfully!"
    echo "Bot User ID: $BOT_USER_ID"
    echo "Access Token: $ACCESS_TOKEN"
    
    # Create .env file
    cat > .env << EOF
MATTERMOST_SERVER_URL=$MATTERMOST_SERVER_URL
MATTERMOST_ACCESS_TOKEN=$ACCESS_TOKEN
MATTERMOST_BOT_USER_ID=$BOT_USER_ID
PORT=8080
EOF
    
    echo ".env file created with bot configuration"
    echo "You can now run: go run main.go"
else
    echo "Failed to create bot. Response:"
    echo "$BOT_RESPONSE"
    echo ""
    echo "Make sure:"
    echo "1. Bot account creation is enabled in System Console > Integrations > Bot Accounts"
    echo "2. Your admin token has proper permissions"
    echo "3. The username 'agent-bot' is not already taken"
fi