docker build -t osat:release .
docker save -o dist/docker/image.tar osat:release