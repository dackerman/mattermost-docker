# Containerized Mattermost with Claude AI Agents on NixOS

This guide shows you how to run Mattermost with Claude AI agents in containers on NixOS using Docker Compose, providing a clean, isolated, and easily manageable deployment.

## Benefits of the Container Approach

- **Clean System**: Keep your NixOS host system clean without installing PostgreSQL, Mattermost, etc. directly
- **Easy Updates**: Update containers independently without affecting the host
- **Isolation**: All services run in isolated containers with defined resource limits
- **Portability**: Easy to move, backup, or replicate the entire setup
- **Development-Friendly**: Easy to spin up/down for testing

## Prerequisites

- NixOS system with admin access
- Domain name (optional, can use localhost for testing)
- Anthropic API key for Claude access

## Step 1: Enable Docker in NixOS

Add Docker support to your `/etc/nixos/configuration.nix`:

```nix
{ config, pkgs, ... }:

{
  # Enable Docker
  virtualisation.docker = {
    enable = true;
    enableOnBoot = true;
  };

  # Add your user to the docker group
  users.users.yourusername = {  # Replace with your username
    extraGroups = [ "docker" ];
  };

  # Install Docker Compose
  environment.systemPackages = with pkgs; [
    docker-compose
    curl
    wget
  ];

  # Open firewall for Mattermost
  networking.firewall.allowedTCPPorts = [ 8065 ];
}
```

Apply the configuration and reboot:

```bash
sudo nixos-rebuild switch
sudo reboot
```

## Step 2: Create Project Directory Structure

```bash
# Create project directory
mkdir -p ~/mattermost-docker
cd ~/mattermost-docker

# Create subdirectories
mkdir -p {config,data,logs,plugins}
mkdir -p data/{postgres,mattermost}
```

## Step 3: Create Docker Compose Configuration

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: mattermost-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: mattermost
      POSTGRES_PASSWORD: mattermost_password
      POSTGRES_DB: mattermost
    volumes:
      - ./data/postgres:/var/lib/postgresql/data
    networks:
      - mattermost-network

  mattermost:
    image: mattermost/mattermost-enterprise-edition:latest
    container_name: mattermost-app
    restart: unless-stopped
    depends_on:
      - postgres
    environment:
      # Database settings
      MM_SQLSETTINGS_DRIVERNAME: postgres
      MM_SQLSETTINGS_DATASOURCE: postgres://mattermost:mattermost_password@postgres:5432/mattermost?sslmode=disable&connect_timeout=10

      # Basic server settings
      MM_SERVICESETTINGS_SITEURL: http://localhost:8065
      MM_SERVICESETTINGS_LISTENADDRESS: ":8065"
      MM_SERVICESETTINGS_ENABLEBOTACCOUNTCREATION: true

      # File and plugin settings
      MM_FILESETTINGS_ENABLEFILEATTACHMENTS: true
      MM_FILESETTINGS_MAXFILESIZE: 52428800
      MM_PLUGINSETTINGS_ENABLE: true
      MM_PLUGINSETTINGS_ENABLEUPLOADS: true
      MM_PLUGINSETTINGS_ALLOWINSECUREDOWNLOADURL: true

      # Integration settings
      MM_SERVICESETTINGS_ENABLEINCOMINGWEBHOOKS: true
      MM_SERVICESETTINGS_ENABLEOUTGOINGWEBHOOKS: true
      MM_SERVICESETTINGS_ENABLECOMMANDS: true

      # Security (change these in production!)
      MM_SERVICESETTINGS_ENABLEDEVELOPER: false
    ports:
      - "8065:8065"
    volumes:
      - ./config:/mattermost/config
      - ./data/mattermost:/mattermost/data
      - ./logs:/mattermost/logs
      - ./plugins:/mattermost/plugins
    networks:
      - mattermost-network

networks:
  mattermost-network:
    driver: bridge

volumes:
  postgres-data:
  mattermost-data:
```

## Step 4: Create Environment Configuration

Create `.env` file for secrets:

```bash
cat > .env << 'EOF'
# Database
POSTGRES_PASSWORD=your_secure_password_here

# Mattermost
MATTERMOST_SITE_URL=http://localhost:8065

# Security - Generate secure random strings for production
MM_SERVICESETTINGS_ENABLEDEVELOPER=false

# Change these in production!
MM_EMAILSETTINGS_SMTPSERVER=
MM_EMAILSETTINGS_SMTPPORT=587
MM_EMAILSETTINGS_SMTPUSERNAME=
MM_EMAILSETTINGS_SMTPPASSWORD=
EOF
```

**🔒 Security Note**: Change the default passwords before deploying to production!

## Step 5: Launch the Containers

```bash
# Start the containers
docker-compose up -d

# Check the logs
docker-compose logs -f

# Verify containers are running
docker-compose ps
```

Expected output:
```
Name                     Command               State           Ports
------------------------------------------------------------------------
mattermost-app          /entrypoint.sh mattermost       Up      0.0.0.0:8065->8065/tcp
mattermost-postgres     docker-entrypoint.sh postgres   Up      5432/tcp
```

## Step 6: Initial Mattermost Setup

1. **Access Mattermost**: Open `http://localhost:8065` in your browser

2. **Create Admin Account**: Follow the setup wizard to create your system administrator account

3. **Configure System Console**:
   - Go to **System Console** → **Plugins** → **Plugin Management**
   - Ensure **Enable Plugins** is set to `true`
   - Set **Enable Plugin Uploads** to `true`

## Step 7: Install AI Agents Plugin

### Download and Install the Plugin

```bash
# Download the latest AI Agents plugin
cd ~/mattermost-docker
curl -L -o mattermost-plugin-agents.tar.gz \
  "https://github.com/mattermost/mattermost-plugin-agents/releases/latest/download/mattermost-plugin-agents.tar.gz"

# Extract to plugins directory (the container will pick it up)
mkdir -p plugins/com.mattermost.plugin-agents
tar -xzf mattermost-plugin-agents.tar.gz -C plugins/

# Restart Mattermost to pick up the plugin
docker-compose restart mattermost
```

**Alternative: Upload via System Console**
1. Go to **System Console** → **Plugins** → **Plugin Management**
2. Upload the `mattermost-plugin-agents.tar.gz` file
3. Enable the plugin

## Step 8: Configure Claude 4 Agent

1. In Mattermost, go to **System Console** → **Plugins** → **Agents**
2. Click **Add an Agent**
3. Configure your Claude 4 agent:

```
Agent Configuration:
- Name: claude4-sonnet
- Display Name: Claude 4 Assistant
- Username: claude4
- Description: Claude 4 AI assistant for development tasks

LLM Configuration:
- Provider: Anthropic
- Model: claude-sonnet-4-20250514
- API Key: [Your Anthropic API Key]
- API URL: https://api.anthropic.com/v1/messages

Custom Instructions:
You are a helpful AI assistant in a Mattermost workspace for software development.
You help with:
- Code reviews and technical questions
- Architecture and design decisions
- Debugging and troubleshooting
- Project planning and task breakdown

Always provide working examples when helping with code.
Use markdown formatting for better readability.
```

4. Click **Save**

## Step 9: Container Management

### Useful Docker Commands

```bash
# View logs
docker-compose logs mattermost
docker-compose logs postgres

# Restart services
docker-compose restart mattermost
docker-compose restart postgres

# Stop everything
docker-compose down

# Start with fresh containers (removes old containers)
docker-compose down && docker-compose up -d

# Update to latest images
docker-compose pull
docker-compose down && docker-compose up -d

# Backup data
tar -czf mattermost-backup-$(date +%Y%m%d).tar.gz data/ config/

# View resource usage
docker stats
```

### Monitoring Container Health

```bash
# Check container status
docker-compose ps

# View resource usage in real-time
docker stats mattermost-app mattermost-postgres

# Check disk usage
du -sh data/
```

## Step 10: Production Hardening

### SSL/TLS with Nginx Reverse Proxy

Update your NixOS configuration to add nginx:

```nix
# Add to configuration.nix
{
  services.nginx = {
    enable = true;
    recommendedTlsSettings = true;
    recommendedOptimisation = true;
    recommendedGzipSettings = true;
    recommendedProxySettings = true;

    virtualHosts."yourdomain.com" = {
      enableACME = true;
      forceSSL = true;
      locations."/" = {
        proxyPass = "http://127.0.0.1:8065";
        proxyWebsockets = true;
        extraConfig = ''
          proxy_set_header Host $host;
          proxy_set_header X-Real-IP $remote_addr;
          proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
          proxy_set_header X-Forwarded-Proto $scheme;
          proxy_set_header X-Frame-Options SAMEORIGIN;
          proxy_buffers 256 16k;
          proxy_buffer_size 16k;
          client_max_body_size 50M;
          proxy_read_timeout 600s;
          proxy_cache_revalidate on;
          proxy_cache_min_uses 2;
          proxy_cache_use_stale timeout;
          proxy_cache_lock on;
        '';
      };
    };
  };

  # Enable ACME for Let's Encrypt certificates
  security.acme = {
    acceptTerms = true;
    defaults.email = "your-email@example.com";
  };

  # Open HTTPS port
  networking.firewall.allowedTCPPorts = [ 80 443 8065 ];
}
```

### Security-Hardened Docker Compose

Create `docker-compose.prod.yml`:

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: mattermost-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: mattermost
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
      POSTGRES_DB: mattermost
    volumes:
      - postgres-data:/var/lib/postgresql/data
    networks:
      - mattermost-network
    secrets:
      - postgres_password
    security_opt:
      - no-new-privileges:true
    user: postgres

  mattermost:
    image: mattermost/mattermost-enterprise-edition:latest
    container_name: mattermost-app
    restart: unless-stopped
    depends_on:
      - postgres
    environment:
      MM_SQLSETTINGS_DRIVERNAME: postgres
      MM_SQLSETTINGS_DATASOURCE: postgres://mattermost:$(cat /run/secrets/postgres_password)@postgres:5432/mattermost?sslmode=disable&connect_timeout=10
      MM_SERVICESETTINGS_SITEURL: https://yourdomain.com
      MM_SERVICESETTINGS_LISTENADDRESS: ":8065"
      MM_SERVICESETTINGS_ENABLEBOTACCOUNTCREATION: true
      MM_FILESETTINGS_ENABLEFILEATTACHMENTS: true
      MM_FILESETTINGS_MAXFILESIZE: 52428800
      MM_PLUGINSETTINGS_ENABLE: true
      MM_PLUGINSETTINGS_ENABLEUPLOADS: true
    ports:
      - "127.0.0.1:8065:8065"  # Only bind to localhost
    volumes:
      - mattermost-config:/mattermost/config
      - mattermost-data:/mattermost/data
      - mattermost-logs:/mattermost/logs
      - mattermost-plugins:/mattermost/plugins
    networks:
      - mattermost-network
    secrets:
      - postgres_password
    security_opt:
      - no-new-privileges:true
    user: "2000:2000"

networks:
  mattermost-network:
    driver: bridge
    internal: false

volumes:
  postgres-data:
  mattermost-config:
  mattermost-data:
  mattermost-logs:
  mattermost-plugins:

secrets:
  postgres_password:
    file: ./secrets/postgres_password.txt
```

## Step 11: Backup and Restore

### Automated Backup Script

Create `backup.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

BACKUP_DIR="/home/yourusername/mattermost-backups"
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_NAME="mattermost_backup_${DATE}"

mkdir -p "${BACKUP_DIR}"

echo "Starting Mattermost backup..."

# Stop Mattermost (keep database running)
cd ~/mattermost-docker
docker-compose stop mattermost

# Backup database
echo "Backing up database..."
docker-compose exec -T postgres pg_dump -U mattermost mattermost > "${BACKUP_DIR}/${BACKUP_NAME}_db.sql"

# Backup data and config
echo "Backing up data and config..."
tar -czf "${BACKUP_DIR}/${BACKUP_NAME}_data.tar.gz" -C ~/mattermost-docker data/ config/ plugins/

# Restart Mattermost
docker-compose start mattermost

echo "Backup completed: ${BACKUP_DIR}/${BACKUP_NAME}*"

# Cleanup old backups (keep last 7 days)
find "${BACKUP_DIR}" -name "mattermost_backup_*" -mtime +7 -delete

echo "Backup process finished successfully!"
```

Make it executable and add to cron:

```bash
chmod +x backup.sh

# Add to crontab for daily backups at 2 AM
(crontab -l ; echo "0 2 * * * /home/yourusername/backup.sh") | crontab -
```

### Restore Process

```bash
#!/usr/bin/env bash
# restore.sh
BACKUP_DATE=$1  # e.g., 20240120_020000

if [ -z "$BACKUP_DATE" ]; then
    echo "Usage: $0 <backup_date>"
    echo "Example: $0 20240120_020000"
    exit 1
fi

BACKUP_DIR="/home/yourusername/mattermost-backups"
BACKUP_NAME="mattermost_backup_${BACKUP_DATE}"

cd ~/mattermost-docker

# Stop services
docker-compose down

# Restore data
echo "Restoring data..."
rm -rf data/ config/ plugins/
tar -xzf "${BACKUP_DIR}/${BACKUP_NAME}_data.tar.gz"

# Start database
docker-compose up -d postgres
sleep 10

# Restore database
echo "Restoring database..."
docker-compose exec -T postgres psql -U mattermost -d mattermost < "${BACKUP_DIR}/${BACKUP_NAME}_db.sql"

# Start Mattermost
docker-compose up -d mattermost

echo "Restore completed!"
```

## Troubleshooting

### Common Issues

**Container Won't Start**
```bash
# Check logs
docker-compose logs mattermost
docker-compose logs postgres

# Check disk space
df -h

# Verify permissions
ls -la data/
```

**Database Connection Issues**
```bash
# Check if PostgreSQL is ready
docker-compose exec postgres pg_isready -U mattermost

# Check database connection
docker-compose exec postgres psql -U mattermost -d mattermost -c "SELECT version();"
```

**Plugin Installation Problems**
```bash
# Check plugin directory permissions
ls -la plugins/

# Manual plugin installation
docker-compose exec mattermost ls /mattermost/plugins/

# Restart to reload plugins
docker-compose restart mattermost
```

**Performance Issues**
```bash
# Monitor resource usage
docker stats

# Check available disk space
du -sh data/

# Optimize PostgreSQL
docker-compose exec postgres psql -U mattermost -d mattermost -c "VACUUM ANALYZE;"
```

## Next Steps

With your containerized Mattermost + Claude 4 setup complete, you can:

1. **Scale Agents**: Add multiple specialized Claude 4 agents for different domains
2. **Integrate Claude Code**: Use bot accounts to orchestrate Claude Code instances
3. **Monitor Performance**: Set up container monitoring with Prometheus/Grafana
4. **Automate Deployments**: Create CI/CD pipelines for updates
5. **Enhance Security**: Implement additional security layers and monitoring

## Benefits Achieved

✅ **Clean NixOS System**: No database or application services installed directly
✅ **Easy Management**: Simple Docker commands for all operations
✅ **Scalable**: Easy to add more containers or move to container orchestration
✅ **Backup-Friendly**: Clear data separation and automated backups
✅ **Development-Ready**: Perfect for your AI agent team MVP development
✅ **Production-Capable**: Can be hardened for production use

This containerized approach gives you the perfect foundation for building your "Slack-Orchestrated Claude Code Architecture" while keeping your NixOS system clean and manageable!
