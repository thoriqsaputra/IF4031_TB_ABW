FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.work ./
COPY agarthan/user_service/go.mod agarthan/user_service/go.sum ./agarthan/user_service/
COPY pkg/models/go.mod ./pkg/models/

RUN cd agarthan/user_service && go mod download

COPY agarthan/user_service ./agarthan/user_service
COPY pkg/models ./pkg/models

RUN cd agarthan/user_service && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/user_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/user_service /usr/local/bin/user_service

USER app

EXPOSE 3000

ENTRYPOINT ["/usr/local/bin/user_service"]
