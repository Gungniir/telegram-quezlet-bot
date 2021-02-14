#!/bin/bash

docker stop osat
docker rm osat
docker rmi osat:release
docker load -i image.tar
docker run --restart always -d --network osat  --name osat osat:release