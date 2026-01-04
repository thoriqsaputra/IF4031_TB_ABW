FROM golang:1.25-alpine AS build

WORKDIR /src

COPY agarthan/workforce_rotation/go.work ./agarthan/workforce_rotation/
COPY agarthan/workforce_rotation/go.mod agarthan/workforce_rotation/go.sum ./agarthan/workforce_rotation/
COPY pkg/models/go.mod ./pkg/models/

RUN cd agarthan/workforce_rotation && go mod download

COPY agarthan/workforce_rotation ./agarthan/workforce_rotation
COPY pkg/models ./pkg/models

RUN cd agarthan/workforce_rotation && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/workforce_rotation .

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app

COPY --from=build /out/workforce_rotation /usr/local/bin/workforce_rotation

USER app

ENTRYPOINT ["/usr/local/bin/workforce_rotation"]
