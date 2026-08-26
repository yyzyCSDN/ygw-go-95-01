#!/bin/sh
set -e
cd "$(dirname "$0")"
docker build -t ygw-go-95-01:latest .
