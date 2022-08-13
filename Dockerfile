# Start by building the application.
FROM golang:1.19 as build

WORKDIR /go/src/app

ADD . /go/src/app/registry-creds

WORKDIR /go/src/app/registry-creds

RUN go get -d -v ./...
RUN go test
RUN GOOS=linux GOARCH=amd64 go build -v -ldflags="-w -s" -o /go/bin/app/registry-creds

# Now copy it into our base image.
# FROM gcr.io/distroless/base
FROM scratch
COPY --from=build /go/bin/app/registry-creds /
CMD ["/registry-creds"]
