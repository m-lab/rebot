FROM golang:1.12 as build
ENV CGO_ENABLED 0
ADD . /go/src/github.com/m-lab/rebot
RUN go get \
    -v \
    -ldflags "-X github.com/m-lab/go/prometheusx.GitShortCommit=$(git log -1 --format=%h)" \
    github.com/m-lab/rebot

# Now copy the built image into the minimal base image
FROM alpine:3.7
RUN apk add ca-certificates
COPY --from=build /go/bin/rebot /
WORKDIR /
ENTRYPOINT ["/rebot"]
