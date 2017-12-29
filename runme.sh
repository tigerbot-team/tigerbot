#!/bin/bash
docker run --rm -ti --privileged --device=/dev/ttyAMA0 -v /home/pi/tigerbot/metabotspin:/metabotspin tigerbot-build propman -t /metabotspin/mb3.binary
