FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/nat-link-server ./server

FROM alpine:3.22
RUN addgroup -S -g 10001 natlink && adduser -S -D -H -u 10001 -G natlink natlink \
    && mkdir -p /opt/nat-link/data /opt/nat-link/logs \
    && chown -R natlink:natlink /opt/nat-link
WORKDIR /opt/nat-link
COPY --from=build /out/nat-link-server ./nat-link-server
COPY config.example.yaml ./config.yaml
RUN chown natlink:natlink ./nat-link-server ./config.yaml
USER natlink
VOLUME ["/opt/nat-link/data", "/opt/nat-link/logs"]
EXPOSE 7000 7001 8080
EXPOSE 7002/udp 7003/udp
ENTRYPOINT ["/opt/nat-link/nat-link-server", "-config", "/opt/nat-link/config.yaml"]
