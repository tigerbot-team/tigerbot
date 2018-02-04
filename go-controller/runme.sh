#!/bin/bash
docker run --rm -ti \
    --name controller \
    --net=host \
    --privileged \
    --device=/dev/ttyAMA0 \
    -v /dev/input:/dev/input \
    -v /var/run/dbus:/var/run/dbus \
    tigerbot/go-controller
