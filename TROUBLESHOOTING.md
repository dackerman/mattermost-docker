# Troubleshooting Guide - Mattermost with Unlimited AI Agents

This guide covers common issues and solutions for the Mattermost Docker setup with the modified AI Agents plugin.

## 🚨 Common Issues

### 1. Plugin Issues

#### "Not Yet Configured" Message in Sidebar

**Symptoms**: Sparkle button shows "Agents is not yet configured for this workspace"

**Cause**: No AI agent has been properly configured with valid credentials.

**Solution**:
1. Go to **System Console > Plugins > Agents**
2. Ensure plugin is **enabled**
3. Click **Add an Agent**
4. Configure with valid API key and settings
5. **Important**: Use a unique username (e.g., "claude2" instead of "claude")

#### Plugin Settings Page is Blank

**Symptoms**: Settings page only shows "Enable Plugin" toggle, no configuration options

**Possible Causes**:
- Custom settings component not loading
- JavaScript errors in browser
- Plugin build issues

**Solutions**:
```bash
# 1. Check browser console for errors
# 2. Disable and re-enable the plugin
# 3. Rebuild plugin if using custom version
cd mattermost-plugin-agents
./build-plugin.sh

# 4. Check Mattermost logs
docker logs mattermost-app --tail 50
```

#### Plugin Won't Upload

**Symptoms**: Upload fails or plugin appears corrupted

**Solutions**:
1. **Check file size**: Should be ~20MB, not 90MB+
2. **Verify build**: Use the provided build script
3. **Check permissions**: Docker handles this automatically
4. **Clear cache**: Disable plugin, restart Mattermost, re-upload

### 2. AI Agent Configuration Issues

#### API Connection Failures

**Symptoms**: "Sorry! An error occurred while accessing the LLM"

**Debugging Steps**:
```bash
# Enable detailed logging
# Go to System Console > Plugins > Agents
# Enable "LLM Trace" and check logs

docker logs mattermost-app --tail 100 | grep "mattermost-ai"
```

**Common Fixes**:
- **Verify API Key**: Test key works outside Mattermost
- **Check Credits**: Ensure API account has sufficient credits  
- **Network Access**: Verify container can reach api.anthropic.com
- **Rate Limits**: Check if hitting API rate limits

#### Agent Not Responding

**Symptoms**: Messages sent but no responses

**Checklist**:
1. ✅ Agent is enabled in settings
2. ✅ API key is valid and has credits
3. ✅ Agent username is unique
4. ✅ Default model is supported
5. ✅ Network connectivity works

**Test Direct Message**:
1. Find the agent user in Direct Messages
2. Send a simple test message
3. Check if response appears

### 3. Docker and Infrastructure Issues

#### Container Won't Start

**Symptoms**: `docker-compose up` fails

**Common Solutions**:
```bash
# Check port conflicts
sudo netstat -tulpn | grep :8065

# Fix permission issues (if using bind mounts)
sudo chown -R 2000:2000 volumes/

# Check disk space
df -h

# View detailed errors
docker-compose logs
```

#### Database Connection Issues

**Symptoms**: Mattermost shows database errors

**Solutions**:
```bash
# Check PostgreSQL status
docker logs mattermost-postgres

# Recreate database (WARNING: loses data)
docker-compose down
docker volume rm mattermost-docker_postgres_data
docker-compose up -d

# Check connection from Mattermost
docker exec -it mattermost-app ping mattermost-postgres
```

#### Memory Issues

**Symptoms**: Containers crashing, slow performance

**Solutions**:
```bash
# Check memory usage
docker stats

# Increase Docker memory limits (Docker Desktop)
# Settings > Resources > Memory: 6GB+

# Monitor container resource usage
docker exec -it mattermost-app top
```

### 4. Network and Access Issues

#### Can't Access Mattermost (8065)

**Symptoms**: Browser can't connect to localhost:8065

**Debugging**:
```bash
# Check if port is open
curl http://localhost:8065

# Check container status
docker ps

# Check if port is bound correctly
docker port mattermost-app
```

**Solutions**:
- **Firewall**: Ensure port 8065 is open
- **Docker Network**: Verify port mapping in docker-compose.yml
- **Container Health**: Check if Mattermost container is healthy

#### Tailscale/Remote Access Issues

**Symptoms**: Can't access from remote devices

**Solutions**:
1. **Configure Mattermost Site URL**:
   ```bash
   # System Console > Environment > Web Server
   # Set Site URL to your Tailscale IP: http://100.x.x.x:8065
   ```

2. **Firewall Rules**:
   ```bash
   # Allow Tailscale subnet
   sudo ufw allow from 100.64.0.0/10 to any port 8065
   ```

3. **Docker Network**:
   ```yaml
   # In docker-compose.yml
   ports:
     - "0.0.0.0:8065:8065"  # Bind to all interfaces
   ```

### 5. Performance Optimization

#### Slow Response Times

**Symptoms**: AI agents take long time to respond

**Optimization**:
1. **Model Selection**: Use faster models (claude-3-5-haiku vs sonnet)
2. **Token Limits**: Reduce max_tokens for faster responses
3. **Temperature**: Lower temperature (0.3) for more deterministic responses
4. **Custom Instructions**: Keep them concise

#### High Resource Usage

**Symptoms**: High CPU/memory usage

**Solutions**:
```bash
# Monitor specific containers
docker stats mattermost-app mattermost-postgres

# Optimize Mattermost config
# System Console > Environment > Performance Monitoring
# Enable performance monitoring and optimize based on metrics

# Database optimization
# Increase PostgreSQL shared_buffers and work_mem if needed
```

## 🔧 Diagnostic Commands

### Quick Health Check

```bash
# Check all containers
docker ps

# Check Mattermost logs
docker logs mattermost-app --tail 50

# Check plugin status
docker exec -it mattermost-app cat /mattermost/logs/mattermost.log | grep "mattermost-ai"

# Test API connectivity (from inside container)
docker exec -it mattermost-app curl -s https://api.anthropic.com/
```

### Plugin Debugging

```bash
# Enable debug logging
# System Console > Plugins > Agents
# Enable "LLM Trace" and "Debug Logging"

# Check plugin directory
docker exec -it mattermost-app ls -la /mattermost/plugins/

# Check plugin config
docker exec -it mattermost-app cat /mattermost/config/config.json | grep -A 20 "PluginSettings"
```

### Network Debugging

```bash
# Test external connectivity
docker exec -it mattermost-app nslookup api.anthropic.com
docker exec -it mattermost-app curl -I https://api.anthropic.com/v1/messages

# Check internal Docker networking
docker network ls
docker network inspect mattermost-docker_default
```

## 🆘 Emergency Recovery

### Reset Plugin Configuration

```bash
# 1. Disable plugin via API (if UI is broken)
curl -X PUT http://localhost:8065/api/v4/plugins/mattermost-ai/disable \
  -H "Authorization: Bearer YOUR_TOKEN"

# 2. Remove plugin files
docker exec -it mattermost-app rm -rf /mattermost/plugins/mattermost-ai*

# 3. Restart Mattermost
docker restart mattermost-app

# 4. Re-upload and configure plugin
```

### Full Reset (Nuclear Option)

**⚠️ WARNING: This will delete all data**

```bash
# Stop all containers
docker-compose down

# Remove all data
docker volume prune
sudo rm -rf volumes/

# Start fresh
docker-compose up -d
```

### Backup and Restore

```bash
# Backup database
docker exec mattermost-postgres pg_dump -U mmuser mattermost > backup.sql

# Backup Mattermost data
sudo tar -czf mattermost-backup.tar.gz volumes/

# Restore database (after fresh install)
docker exec -i mattermost-postgres psql -U mmuser mattermost < backup.sql

# Restore data
sudo tar -xzf mattermost-backup.tar.gz
```

## 📞 Getting Help

### Log Analysis

Before asking for help, gather these logs:
```bash
# Mattermost server logs
docker logs mattermost-app > mattermost.log

# PostgreSQL logs  
docker logs mattermost-postgres > postgres.log

# Plugin-specific logs
docker logs mattermost-app | grep "mattermost-ai" > plugin.log

# System info
docker info > docker-info.txt
docker-compose version > compose-version.txt
```

### Community Resources

- **Mattermost Forums**: https://forum.mattermost.com/
- **Plugin Repository**: https://github.com/mattermost/mattermost-plugin-ai
- **Docker Issues**: https://github.com/mattermost/docker
- **Documentation**: https://docs.mattermost.com/

### Creating Bug Reports

Include:
1. **Environment**: OS, Docker version, Mattermost version
2. **Steps to Reproduce**: Exact steps that cause the issue
3. **Expected vs Actual**: What should happen vs what happens
4. **Logs**: Relevant log excerpts (sanitize sensitive data)
5. **Screenshots**: If UI-related issues

---

**Remember**: Most issues are configuration-related. Double-check API keys, network connectivity, and basic settings before diving into complex debugging! 🐛→✅