FROM golang:1 AS builder

ARG SHA
ARG GOOS
ARG GOARCH

#RUN apt-get update && apt-get upgrade -y dpkg && apt-get -y install build-essential

RUN mkdir /app && mkdir /data
WORKDIR /app
COPY . /app/

RUN make target/$GOOS-$GOARCH/bot





FROM alpine

ARG GOOS
ARG GOARCH

RUN apk --no-cache add tzdata ca-certificates

RUN mkdir /app && mkdir /data
VOLUME /data
WORKDIR /app

COPY --from=builder /app/target/$GOOS-$GOARCH/bot /app/bot
COPY db /app/db

ENTRYPOINT ["/app/bot"]
