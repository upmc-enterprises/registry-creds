FROM alpine:3.4
MAINTAINER Steve Sloka <slokas@upmc.edu>

RUN apk add --update ca-certificates && \
  rm -rf /var/cache/apk/*

COPY registry-creds registry-creds

ENTRYPOINT ["/registry-creds"]
