FROM docker.io/library/golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /agent-memory ./cmd/agent-memory

FROM docker.io/library/alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /agent-memory /usr/local/bin/agent-memory

ENTRYPOINT ["agent-memory"]
