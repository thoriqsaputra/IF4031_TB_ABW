FROM golang:1.25-alpine AS build

WORKDIR /src

COPY agarthan/auth_service/go.mod agarthan/auth_service/go.sum ./agarthan/auth_service/
COPY pkg/models/go.mod ./pkg/models/
COPY pkg/middleware/go.mod ./pkg/middleware/

RUN cd agarthan/auth_service && go mod download

COPY agarthan/auth_service ./agarthan/auth_service
COPY pkg/models ./pkg/models
COPY pkg/middleware ./pkg/middleware

RUN cd agarthan/auth_service && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/auth_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/auth_service /usr/local/bin/auth_service

USER app

EXPOSE 3001

ENTRYPOINT ["/usr/local/bin/auth_service"]
