#!/usr/bin/env bash
set -euo pipefail

go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.28.0 generate
