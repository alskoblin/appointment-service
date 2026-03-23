FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE_CMD=./cmd/app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/service ${SERVICE_CMD}

FROM gcr.io/distroless/static-debian12

WORKDIR /app
COPY --from=builder /bin/service /app/service

ENTRYPOINT ["/app/service"]
