FROM golang:1.22-alpine3.19 AS builder
ARG VERSION

RUN apk add --no-cache git gcc musl-dev make

WORKDIR /go/src/github.com/infobloxopen/migrate

ENV GO111MODULE=on

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

RUN make build-docker

FROM alpine:3.19

COPY --from=builder /go/src/github.com/infobloxopen/migrate/cmd/migrate/config /cli/config/
COPY --from=builder /go/src/github.com/infobloxopen/migrate/build/migrate.linux-386 /usr/local/bin/migrate
RUN ln -s /usr/local/bin/migrate /migrate

ENTRYPOINT ["migrate"]
CMD ["--help"]
