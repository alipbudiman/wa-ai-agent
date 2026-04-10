FROM golang:1.25

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /usr/local/bin/wa-ai-agent ./main.go

ENV GIN_SERVER_HOST=0.0.0.0
ENV GIN_SERVER_PORT=8001

EXPOSE 8001

CMD ["/usr/local/bin/wa-ai-agent"]