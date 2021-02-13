#!/bin/bash

docker load -i image.tar
docker run -d --network osat run --name osat -e token=[insert your token here] osat