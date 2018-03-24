# Start with a container that's already set up with OpenCV
# and do the builds in there.

FROM sgtwilko/rpi-raspbian-opencv:stretch-latest as build

RUN apt update
RUN apt install make gcc
RUN apt install wget
RUN wget https://dl.google.com/go/go1.10.linux-armv6l.tar.gz
RUN tar -C /usr/local -xzf go*.tar.gz

ENV PATH=$PATH:/usr/local/go/bin
ENV GOROOT=/usr/local/go/
ENV GOPATH=/go/
RUN apt install git

RUN mkdir -p $GOPATH/src/gocv.io/x/ && \
    cd $GOPATH/src/gocv.io/x/ && \
    git clone https://github.com/fasaxc/gocv.git

# Pre-build gocv to cache the package in this layer. That
# stops expensive gocv builds when we're compiling the controller.
RUN bash -c "cd $GOPATH/src/gocv.io/x/gocv && \
             source ./env.sh && \
             go build -v gocv.io/x/gocv"

RUN bash -c "cd $GOPATH/src/gocv.io/x/gocv && \
             source ./env.sh && \
             go build -v ./cmd/saveimage/main.go"

# Add the propeller IDE tools so we can extract the propman tool.
RUN wget https://github.com/parallaxinc/PropellerIDE/releases/download/0.38.5/propelleride-0.38.5-armhf.deb
RUN sh -c "dpkg -i propelleride-0.38.5-armhf.deb || true" && \
    apt-get install -y -f && \
    apt-get clean -y

# Now add the controller code and build.

COPY VL53L0X_1.0.2 $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L0X_1.0.2
COPY VL53L0X_rasp $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L0X_rasp
WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/VL53L0X_rasp
RUN API_DIR=../VL53L0X_1.0.2 make all examples

COPY go-controller/ $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
RUN bash -c "source $GOPATH/src/gocv.io/x/gocv/env.sh && \
             GOMAXPROCS=1 go build -p 1 -v controller.go"
