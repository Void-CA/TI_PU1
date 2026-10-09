# Build stage
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api

# Runtime stage
FROM alpine:3.22
RUN adduser -D -u 10001 app
USER app
COPY --from=build /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
