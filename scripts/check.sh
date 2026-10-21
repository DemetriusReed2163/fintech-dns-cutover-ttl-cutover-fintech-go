#!/bin/sh
set -eu
go test ./...
go build -o /tmp/fintech-dns-cutover ./cmd/cutover
