FROM golang:1.25.13 AS builder

WORKDIR /src
COPY go.mod go.sum ./
COPY sdk ./sdk
COPY examples/external-integration-demo/main.go ./main.go
COPY examples/external-integration-demo/public ./public
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /integration-demo ./main.go

FROM alpine:3.20

COPY --from=builder /integration-demo /integration-demo
EXPOSE 8090 9000

ENTRYPOINT ["/integration-demo"]