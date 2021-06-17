FROM golang:1.14 as BUILD

WORKDIR /build
COPY . .

# # run tests
# RUN go test -cover ./...

# build a static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o registry-creds .


# run in here
FROM alpine

RUN apk add --update ca-certificates && \
  rm -rf /var/cache/apk/*

COPY --from=BUILD /build/registry-creds /registry-creds

ENTRYPOINT ["/registry-creds"]
