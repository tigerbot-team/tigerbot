# Start with a container that's already set up with OpenCV
# and do the builds in there.

FROM golang:1.21-bullseye

RUN apt-get update -y && \
    apt-get install -y make gcc wget git sudo

# Download and install the Golang open CV library
RUN mkdir $GOPATH/src/hybridgroup/ && \
    cd $GOPATH/src/hybridgroup/ && \
    git clone https://github.com/hybridgroup/gocv.git

RUN cd $GOPATH/src/hybridgroup/gocv && \
    make install_raspi

RUN apt-get install  -y libasound2-dev libasound2 libasound2-plugins


## Pre-build the ToF libraries
#
COPY VL53L5CX_Linux_driver_1.3.11 $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L5CX_Linux_driver_1.3.11
WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L5CX_Linux_driver_1.3.11
RUN make -C user/test clean all

RUN mkdir -p $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
