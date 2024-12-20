FROM docker.io/library/golang:1.23 AS builder

WORKDIR /src

COPY go.mod go.sum .

RUN go mod download

COPY *.go Makefile .

RUN make build

FROM registry.access.redhat.com/ubi9/ubi-micro:9.4

RUN mkdir -p licenses

COPY LICENSE licenses/LICENSE

COPY --from=builder /src/osluc /usr/local/bin/

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/osluc"]
