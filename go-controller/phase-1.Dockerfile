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

RUN apt-get install libasound2-dev libasound2 libasound2-plugins


## Pre-build the ToF libraries
#
#COPY VL53L0X_1.0.2 $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L0X_1.0.2
#COPY VL53L0X_rasp $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L0X_rasp
#WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L0X_rasp
#RUN API_DIR=../VL53L0X_1.0.2 make all examples
#
#RUN mkdir -p $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
#WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
