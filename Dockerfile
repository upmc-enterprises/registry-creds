FROM golang:1.20-alpine as builder

RUN apk add make

WORKDIR /mnt
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o registry-creds main.go



FROM alpine:3.17 
COPY --from=builder /mnt/registry-creds registry-creds

RUN apk add --update ca-certificates && \
  rm -rf /var/cache/apk/*

ENTRYPOINT ["/registry-creds"]
