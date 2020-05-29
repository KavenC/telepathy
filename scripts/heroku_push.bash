#!/usr/bin/env bash
# Usage heroku_push.bash <app-name>
set -e
APP=$1
./scripts/build_image.bash
docker tag telepathy:latest registry.heroku.com/${APP}/web
docker push registry.heroku.com/${APP}/web
heroku container:release web