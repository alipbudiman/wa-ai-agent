FROM ubuntu:22.04

# Prevent interactive prompts during apt-get
ENV DEBIAN_FRONTEND=noninteractive

# Install dependencies and Go 1.25
RUN apt-get update && apt-get install -y \
    wget \
    tar \
    git \
    build-essential \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz \
    && tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz \
    && rm go1.25.0.linux-amd64.tar.gz

ENV PATH="/usr/local/go/bin:${PATH}"

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /usr/local/bin/wa-ai-agent ./main.go

ENV GIN_SERVER_HOST=0.0.0.0
ENV GIN_SERVER_PORT=8001

EXPOSE 8001

CMD ["/usr/local/bin/wa-ai-agent"]