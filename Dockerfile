# Minimal starkite container image
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata
COPY bin/kite-linux /usr/local/bin/kite

CMD ["kite", "--help"]
