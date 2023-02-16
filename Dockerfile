FROM alpine:3.17
MAINTAINER Steve Sloka <steve@stevesloka.com>

RUN apk add --update ca-certificates && \
  rm -rf /var/cache/apk/*

COPY registry-creds registry-creds

ENTRYPOINT ["/registry-creds"]
