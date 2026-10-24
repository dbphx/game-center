FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /gamehub .

FROM alpine:3.20
COPY --from=build /gamehub /gamehub
# Tài khoản và ví lưu ở đây; gắn volume để không mất khi tạo lại container.
ENV DB_PATH=/data/gamehub.db
VOLUME /data
EXPOSE 8080
CMD ["/gamehub"]
