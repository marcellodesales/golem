#!/usr/bin/env bash
set -e

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"

# Root directory of Distribution
DISTRIBUTION_ROOT=$(cd ../..; pwd -P)

# Image containing the integration tests environment.
INTEGRATION_IMAGE=${INTEGRATION_IMAGE:-distribution/docker-integration}

# Make sure we upgrade the integration environment.
# Not yet on hub, run `docker build -t distribution/docker-integration .`
#docker pull $INTEGRATION_IMAGE

# Start the integration tests in a Docker container.
ID=$(docker run -d -t --privileged \
	-v ${DISTRIBUTION_ROOT}:/go/src/github.com/docker/distribution \
	-e "DOCKER_IMAGE=$DOCKER_IMAGE" \
	-e "DOCKER_VERSION=$DOCKER_VERSION" \
	-e "STORAGE_DRIVER=$STORAGE_DRIVER" \
	-e "EXEC_DRIVER=$EXEC_DRIVER" \
	${INTEGRATION_IMAGE} \
	./test_runner.sh "$@")

# Clean it up when we exit.
trap "docker rm -f -v $ID > /dev/null" EXIT

docker logs -f $ID