FROM golang:1-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/key-value-store .

FROM alpine:3

COPY --from=build /out/key-value-store /usr/local/bin/key-value-store
ENV LISTEN_ADDR=:8080
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/key-value-store"]
