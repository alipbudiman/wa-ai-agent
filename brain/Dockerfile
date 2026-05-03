FROM python:3.13-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    curl \
    gnupg \
    git \
    tmux \
    ca-certificates \
    apt-transport-https \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_20.x nodistro main" > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update && apt-get install -y \
    nodejs \
    && npm install -g skills@latest \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app/brain
ENV DISABLE_TELEMETRY=1
ENV DOCUMENT_STORAGE_DIR=/app/uploads/documents

# Symlink global skills dir to volume-backed path so skills persist across restarts
RUN mkdir -p /app/.agents && ln -sf /app/.agents /root/.agents

RUN npx --yes skills add https://github.com/vercel-labs/skills --skill find-skills -g -a universal -y \
    && npx --yes skills list -g --json > /dev/null \
    && skills --version > /dev/null \
    && curl -L -o /usr/local/bin/cloudflared https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 \
    && chmod +x /usr/local/bin/cloudflared \
    && cloudflared --version

# Create and activate virtual environment
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:$PATH"
RUN echo 'export PATH="/opt/venv/bin:$PATH"' >> ~/.bashrc

COPY requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir -r /tmp/requirements.txt

COPY .claude /app/.claude
COPY brain /app/brain

EXPOSE 8000

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8000"]
