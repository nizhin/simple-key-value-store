FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/key-value-store .

FROM alpine:3

COPY --from=build /out/key-value-store /usr/local/bin/key-value-store
ENV LISTEN_ADDR=:8080
ENV DATA_FILE=/data/store.db
RUN mkdir /data && chown 65532:65532 /data
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/key-value-store"]
