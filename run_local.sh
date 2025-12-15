#!/bin/bash

set -e

docker compose down

sleep 2

docker compose build

sleep 2

docker compose up -d

sleep 2

docker logs billing-fork-billing-1 -f