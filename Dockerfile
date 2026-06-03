FROM ubuntu:22.04

# Prevent interactive prompts during apt-get
ENV DEBIAN_FRONTEND=noninteractive

# Install dependencies and Go 1.25
RUN apt-get update && apt-get install -y \
    wget \
    tar \
    git \
    curl \
    gnupg \
    build-essential \
    ca-certificates \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_20.x nodistro main" > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update && apt-get install -y \
    nodejs \
    && npm install -g skills@latest \
    && rm -rf /var/lib/apt/lists/*

RUN wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz \
    && rm go1.25.0.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"
ENV DISABLE_TELEMETRY=1
ENV DOCUMENT_STORAGE_DIR=/app/uploads/documents

WORKDIR /app

RUN npx --yes skills add https://github.com/vercel-labs/skills --skill find-skills -g -a universal -y \
    && npx --yes skills list -g --json > /dev/null \
    && skills --version > /dev/null

RUN npx --yes skills add https://github.com/juliusbrussee/caveman --skill caveman -g -a universal -y \
    && npx --yes skills list -g --json > /dev/null \
    && skills --version > /dev/null

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /usr/local/bin/wa-ai-agent ./main.go

ENV GIN_SERVER_HOST=0.0.0.0
ENV GIN_SERVER_PORT=8001

EXPOSE 8001

CMD ["/usr/local/bin/wa-ai-agent"]
