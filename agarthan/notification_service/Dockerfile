FROM golang:1.25-alpine AS build

WORKDIR /src

COPY agarthan/notification_service/go.work ./agarthan/notification_service/
COPY agarthan/notification_service/go.mod agarthan/notification_service/go.sum ./agarthan/notification_service/

RUN cd agarthan/notification_service && go mod download

COPY agarthan/notification_service ./agarthan/notification_service

RUN cd agarthan/notification_service && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/notification_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/notification_service /usr/local/bin/notification_service

USER app

EXPOSE 3002

ENTRYPOINT ["/usr/local/bin/notification_service"]
