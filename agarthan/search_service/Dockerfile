FROM golang:1.25-alpine AS build

WORKDIR /src

COPY agarthan/search_service/go.work ./agarthan/search_service/
COPY agarthan/search_service/go.mod agarthan/search_service/go.sum ./agarthan/search_service/
COPY pkg/middleware/go.mod ./pkg/middleware/
COPY pkg/models/go.mod ./pkg/models/

RUN cd agarthan/search_service && go mod download

COPY agarthan/search_service ./agarthan/search_service
COPY pkg/middleware ./pkg/middleware
COPY pkg/models ./pkg/models

RUN cd agarthan/search_service && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/search_service .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/search_service /usr/local/bin/search_service

USER app

EXPOSE 3003

ENTRYPOINT ["/usr/local/bin/search_service"]
