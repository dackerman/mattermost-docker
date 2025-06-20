# Mattermost Docker Setup with Unlimited AI Agents

This repository contains a complete Mattermost installation with Docker and a modified AI Agents plugin that removes enterprise licensing restrictions, allowing unlimited AI agents on the free plan.

## 🚀 Quick Start

1. **Start Mattermost**:
   ```bash
   docker-compose up -d
   ```

2. **Access Mattermost**: http://localhost:8065

3. **Install AI Agents Plugin**: Upload `mattermost-ai-unlimited.tar.gz` through System Console

4. **Configure AI Agents**: Add your API keys and start using unlimited AI agents!

## 📋 Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [AI Agents Plugin Setup](#ai-agents-plugin-setup)
- [Configuration](#configuration)
- [Usage](#usage)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [License](#license)

## 🔧 Prerequisites

- Docker and Docker Compose
- 4GB+ RAM
- 10GB+ disk space
- Internet connection for AI API calls

## 🛠 Installation

### 1. Clone and Start Mattermost

```bash
git clone <this-repo>
cd mattermost-docker
docker-compose up -d
```

### 2. Initial Mattermost Setup

1. Navigate to http://localhost:8065
2. Create your admin account
3. Complete the initial setup wizard
4. Skip team creation if desired

### 3. Install Modified AI Agents Plugin

1. Go to **System Console > Plugin Management**
2. Click **Upload Plugin**
3. Upload `mattermost-ai-unlimited.tar.gz`
4. Enable the plugin

## 🤖 AI Agents Plugin Setup

### What's Modified

This version removes enterprise licensing restrictions:
- ✅ **Unlimited AI agents** (vs 1 on free plan)
- ✅ **Multiple LLM providers** simultaneously
- ✅ **Full feature access** without enterprise license

### Supported AI Providers

- **Anthropic Claude** (claude-3-5-sonnet, claude-3-5-haiku)
- **OpenAI** (GPT-4, GPT-3.5-turbo)
- **Azure OpenAI**
- **Local LLMs** (Ollama, etc.)

## ⚙️ Configuration

### 1. Enable AI Agents

1. Go to **System Console > Plugins > Agents**
2. Set **Enable Plugin** to **True**
3. Click **Save**

### 2. Add Your First AI Agent

1. Click **Add an Agent**
2. Configure basic settings:
   ```
   Display Name: Claude Assistant
   Agent Username: claude
   LLM Provider: Anthropic
   API Key: [your-anthropic-api-key]
   Default Model: claude-3-5-sonnet-20241022
   ```
3. Add custom instructions:
   ```
   You are a helpful AI assistant. Provide clear, accurate, and concise responses.
   When writing code, use proper formatting and include explanations.
   ```
4. Click **Save**

### 3. Add Additional Agents (Unlimited!)

Repeat the process to add more agents:
- **Coding Assistant**: Specialized for programming help
- **Writing Assistant**: Focused on content creation  
- **Data Analyst**: For data analysis and insights
- **Creative Assistant**: For brainstorming and creative tasks

### 4. Configure Default Settings

1. Set **Default Bot** to your preferred agent
2. Configure **Debug Settings** if needed:
   - Enable **LLM Trace** for detailed logging
   - Set **Allowed Upstream Hostnames** for tool access

## 📱 Usage

### Direct Messages
1. Click the sparkle ✨ button in any channel
2. Or send a direct message to any configured agent
3. Use natural language to interact

### Slash Commands
```bash
/ai <your question>                    # Ask the default bot
/ask-channel <question>               # Ask about channel content
/summarize-channel                    # Summarize recent messages
```

### Channel Integration
- Use the sparkle button in channels for context-aware responses
- Agents can access channel history for better answers
- Thread summarization available in post dropdown menus

### Features Available
- ✅ **Thread Summarization**: Summarize long discussions
- ✅ **Channel Summaries**: Get caught up quickly
- ✅ **Meeting Summaries**: Process call transcripts
- ✅ **Code Assistance**: Get programming help
- ✅ **Document Search**: Find information across channels
- ✅ **Custom Instructions**: Tailor agent behavior

## 🔍 Troubleshooting

### Plugin Won't Load
```bash
# Check logs
docker logs mattermost-app

# Common fixes:
1. Ensure plugin is enabled in System Console
2. Verify file permissions (Docker handles this automatically)
3. Check for conflicting plugins
```

### "Not Yet Configured" Message
This means no AI agent has been properly configured:
1. Go to System Console > Plugins > Agents
2. Add at least one agent with valid API key
3. Ensure the agent username is unique

### API Connection Issues
1. Verify API key is correct and has credits
2. Check network connectivity from container
3. Enable LLM Trace for detailed error logs
4. Ensure API endpoint URLs are reachable

### Agent Not Responding
1. Check server logs for errors
2. Verify agent is enabled and configured
3. Test with direct message first
4. Check API rate limits and quotas

### Settings Page Blank
If the plugin settings page shows only "Enable Plugin" toggle:
1. This is usually a build or registration issue
2. Try disabling and re-enabling the plugin
3. Check browser console for JavaScript errors
4. Verify the plugin uploaded completely

## 🛠 Development

### Building the Plugin

```bash
cd mattermost-plugin-agents
./build-plugin.sh
```

### Key Modifications Made

1. **License Check Removal** (`bots/bots.go:71-74`):
   ```go
   // Commented out enterprise license restriction
   // if len(cfgBots) > 1 && !b.licenseChecker.IsMultiLLMLicensed() { ... }
   ```

2. **Frontend Restrictions** (`webapp/src/components/system_console/bots.tsx`):
   ```javascript
   // Always allow adding bots
   const licenceAddDisabled = false;
   ```

3. **Plugin Configuration** (`plugin.json`):
   - Maintained original plugin ID for compatibility
   - Version set to "1.3.0-unlimited"
   - Reduced build size by including only linux-amd64

### File Structure
```
mattermost-docker/
├── docker-compose.yml              # Mattermost + PostgreSQL setup
├── mattermost-ai-unlimited.tar.gz  # Modified plugin package
├── mattermost-plugin-agents/       # Plugin source code
│   ├── bots/bots.go                # License restrictions removed
│   ├── webapp/src/                 # Frontend modifications
│   ├── build-plugin.sh             # Automated build script
│   └── plugin.json                 # Plugin configuration
└── volumes/                        # Docker persistent data
```

### Testing Changes

1. Make code modifications
2. Run `./build-plugin.sh`
3. Upload new plugin via System Console
4. Test functionality

## 📄 License

- **Mattermost**: [Mattermost License](https://github.com/mattermost/mattermost-server/blob/master/LICENSE.txt)
- **AI Agents Plugin**: [Apache 2.0](https://github.com/mattermost/mattermost-plugin-ai/blob/main/LICENSE.txt)
- **Modifications**: Open source, use at your own risk

## ⚠️ Disclaimer

This modified plugin removes enterprise licensing restrictions for educational and personal use. Use responsibly and ensure compliance with Mattermost's terms of service in production environments.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## 📞 Support

For issues with:
- **Mattermost Setup**: Check official [Mattermost documentation](https://docs.mattermost.com/)
- **AI Agents Plugin**: See [plugin documentation](https://github.com/mattermost/mattermost-plugin-ai)
- **This Modified Version**: Create an issue in this repository

---

🎉 **Enjoy unlimited AI agents in your Mattermost workspace!**