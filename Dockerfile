FROM golang as builder
WORKDIR /go/src/github.com/upmc-enterprises/registry-creds
COPY . .
RUN make build

FROM alpine:3.4
MAINTAINER Steve Sloka <slokas@upmc.edu>

RUN apk add --update ca-certificates && \
  rm -rf /var/cache/apk/*

COPY --from=builder /go/src/github.com/upmc-enterprises/registry-creds/registry-creds registry-creds

ENTRYPOINT ["/registry-creds"]
