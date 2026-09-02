# Estágio de Build usando Go 1.26
FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/finances-app main.go

# Estágio final
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copia o binário para o PATH global do sistema (/usr/local/bin)
COPY --from=builder /app/finances-app /usr/local/bin/finances-app

CMD ["finances-app"]