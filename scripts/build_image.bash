#!/usr/bin/env bash
docker build --rm -t telepathy --build-arg version=$(git rev-parse --short HEAD) .