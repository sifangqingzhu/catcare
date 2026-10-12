FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./... && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /catcare ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S catcare && adduser -S -G catcare catcare
WORKDIR /app
COPY --from=build /catcare /app/catcare
USER catcare
EXPOSE 8080
ENTRYPOINT ["/app/catcare"]
