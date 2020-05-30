#!/usr/bin/env bash
DATE=$(LC_TIME=utf8 date -u +"%Y-%b-%d %H:%M:%m UTC")
docker build -t telepathy --build-arg version="dev ${DATE}" .