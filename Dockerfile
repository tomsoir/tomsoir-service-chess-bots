FROM golang:1.23.2-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/chess-bots .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/chess-bots /app/chess-bots
EXPOSE 9600
ENV APP_PORT=9600
ENTRYPOINT ["/app/chess-bots"]
