## 🐳 Docker Deployment

For users who prefer containerized deployment, we now provide Docker deployment options. This is particularly useful for:

- Running the service on your own server instead of GitHub Actions
- Better resource control (CPU/RAM allocation)
- Easier environment management
- Persistent logging and model caching
- **Prerequisites**:
  - Docker installed ([Installation Guide](https://docs.docker.com/engine/install/))
  - Docker Compose (usually included with Docker Desktop)
  - Configured Docker image registry mirror (for faster builds in some regions)

### Quick Start with Docker

1. Clone the repository:
```bash
git clone https://github.com/TideDra/zotero-arxiv-daily.git
cd zotero-arxiv-daily
```

2. Build the Docker image (recommended for customization):
```bash
docker build . -t local/zotero-arxiv-daily:latest
```

3. Create necessary directories:
```bash
mkdir -p logs models
```

4. Edit the `docker-compose.yml` file to configure your environment variables:
```yaml
    environment:
      # Required parameters
      - ZOTERO_ID=12345678
      - ZOTERO_KEY=AbCdEfGhIjKlMnOpQrStUvWx
      - SMTP_SERVER=smtp.example.com
      - SMTP_PORT=465
      - SMTP_USER=your_email@example.com
      - SMTP_PASSWORD=your_email_password
      - SMTP_SENDER=your_email@example.com
      - SMTP_RECEIVER=receiver_email@example.com
      - LLM_API_KEY=your_google_gemini_api_key

      # Optional parameters
      - LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai/
      - LLM_MODEL=gemini-2.5-flash
      - LLM_LANGUAGE=English
      - ARXIV_CATEGORIES=["cs.AI","cs.CV","cs.LG","cs.CL"]
      - MAX_PAPER_NUM=10
      - DEBUG=false
      
      # 新增配置
      - HF_ENDPOINT=https://hf-mirror.com
      # - TZ=Asia/Shanghai  # 时区设置
      # - http_proxy=http://proxy.example.com:8080  # HTTP代理（可选）
      # - https_proxy=http://proxy.example.com:8080 # HTTPS代理（可选）
      # - no_proxy=localhost,127.0.0.1,.internal  # 代理排除项
```

5. Start the service:
```bash
docker compose up -d
```

### Key Features of Docker Deployment

- **Scheduled Execution**: By default runs daily at 8:00 AM (configurable in `command` section)
- **Log Persistence**: All logs are saved in the `logs/` directory
- **Model Caching**: Local LLM models can be cached in `models/` directory
- **Resource Isolation**: Runs in a contained environment with all dependencies included
- **Easy Updates**: Simply rebuild the image when updating the service

### Configuration Options

You can customize the deployment by:

1. **Changing schedule time**: Edit the cron expression in `command` section (default: `0 8 * * *` means 8:00 AM daily)
2. **Using local LLM**: Set `USE_LLM_API=0` and uncomment the models volume
3. **Proxy settings**: Uncomment and configure proxy environment variables if needed
4. **Timezone**: Uncomment `TZ` variable to set specific timezone (you may also need to comment `- /etc/localtime:/etc/localtime:ro`)

### Monitoring and Maintenance

- View logs:
```bash
docker logs zotero-arxiv-daily
```

- Stop the service:
```bash
docker compose down
```

- Update the service:
```bash
git pull origin main
docker compose down
docker compose up -d --build
```

### Why Choose Docker Deployment?

1. **Consistent Environment**: Eliminates "works on my machine" problems
2. **Resource Control**: Allocate specific CPU/RAM resources as needed
3. **Isolation**: Runs separately from your host system
4. **Portability**: Easy to move between different servers
5. **Persistent Storage**: Logs and models persist between container restarts