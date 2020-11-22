FROM alpine

RUN mkdir /app
RUN mkdir /data
VOLUME /data
WORKDIR /app
COPY target/linux-arm/bot /app/

ENTRYPOINT ["/app/bot"]
