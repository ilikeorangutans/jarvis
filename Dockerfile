FROM alpine

RUN apk --no-cache add tzdata

RUN mkdir /app
RUN mkdir /data
VOLUME /data
WORKDIR /app
COPY target/linux-arm/bot /app/

ENTRYPOINT ["/app/bot"]
