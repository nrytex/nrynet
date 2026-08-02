#!/usr/bin/env sh
set -eu

VERSION="${VERSION:-dev}"
OUTPUT="${OUTPUT:-bin}"

for target in linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64; do
  os="${target%/*}"
  arch="${target#*/}"
  directory="$OUTPUT/$VERSION-$os-$arch"
  extension=""
  if [ "$os" = "windows" ]; then extension=".exe"; fi
  mkdir -p "$directory"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$directory/nat-link-server$extension" ./server
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$directory/nat-link-client$extension" ./client
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w" -o "$directory/nat-link-relay$extension" ./relay
done
