# Minimal starkite container image
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S kite && adduser -S -G kite kite
COPY bin/kite-linux /usr/local/bin/kite
RUN chmod 0755 /usr/local/bin/kite

USER kite
ENTRYPOINT ["/usr/local/bin/kite"]
CMD ["--help"]
