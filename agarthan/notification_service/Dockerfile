FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY agarthan/notification_service/go.mod agarthan/notification_service/go.work ./agarthan/notification_service/
COPY pkg/models/go.mod ./pkg/models/

WORKDIR /app/agarthan/notification_service
ENV GOPROXY=direct
RUN go mod download

WORKDIR /app
COPY agarthan/notification_service/ ./agarthan/notification_service/
COPY pkg/models/ ./pkg/models/

WORKDIR /app/agarthan/notification_service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/notification_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/notification_service /usr/local/bin/notification_service

USER app

EXPOSE 3003

ENTRYPOINT ["/usr/local/bin/notification_service"]
