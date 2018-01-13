#!/bin/bash
docker run --rm -ti --privileged --device=/dev/ttyAMA0 -v /home/pi/tigerbot:/tigerbot controller:latest /bin/bash
