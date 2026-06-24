FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/stint ./cmd/server \
 && CGO_ENABLED=0 go build -o /out/stint-collect ./cmd/collect

FROM alpine:3.21
RUN adduser -D -H stint && mkdir -p /data/dumps && chown -R stint:stint /data
USER stint
COPY --from=build /out/stint /stint
COPY --from=build /out/stint-collect /stint-collect
VOLUME ["/data/dumps"]
EXPOSE 8080
ENTRYPOINT ["/stint"]
