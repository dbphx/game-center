# DCity: binary Go build sẵn cho linux/amd64 (giao diện nhúng trong binary).
FROM alpine:3.20
RUN apk add --no-cache tzdata ca-certificates && adduser -D -u 10001 dcity && mkdir -p /data && chown dcity /data
COPY dcity /app/dcity
USER dcity
ENV TZ=Asia/Ho_Chi_Minh
EXPOSE 8090
CMD ["/app/dcity", "-addr", ":8090", "-db", "/data/gamehub.db"]
