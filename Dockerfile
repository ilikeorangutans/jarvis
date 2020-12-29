FROM alpine

RUN apk --no-cache add tzdata

RUN mkdir /app && mkdir /data
VOLUME /data
WORKDIR /app
COPY target/linux-arm/bot /app/
COPY db /app/db

ENTRYPOINT ["/app/bot"]
