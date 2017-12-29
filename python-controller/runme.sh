#!/bin/bash
docker run --rm -ti --name controller --net=host --privileged --device=/dev/ttyAMA0 controller /bin/bash
