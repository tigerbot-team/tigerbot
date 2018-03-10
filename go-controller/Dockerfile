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

# Pre-build gocv to cache the package.
RUN bash -c "cd $GOPATH/src/gocv.io/x/gocv && \
             source ./env.sh && \
             go build -v gocv.io/x/gocv"

RUN bash -c "cd $GOPATH/src/gocv.io/x/gocv && \
             source ./env.sh && \
             go build -v ./cmd/saveimage/main.go"

COPY go-controller/ $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller
RUN bash -c "source $GOPATH/src/gocv.io/x/gocv/env.sh && \
             go build -v controller.go"

COPY metabotspin/mb3.binary /mb3.binary

FROM resin/raspberry-pi-alpine:latest

COPY --from=build /lib/ld-linux-armhf.so* /lib
RUN mkdir -p /usr/local/lib

COPY --from=build /usr/lib/arm-linux-gnueabihf/* /usr/local/lib/
COPY --from=build /lib/arm-linux-gnueabihf/* /usr/local/lib/
COPY --from=build /usr/local/lib /usr/local/lib
COPY --from=build /go/src/github.com/tigerbot-team/tigerbot/go-controller/controller /controller
COPY --from=build /opt/vc/lib/* /usr/local/lib/
ENV LD_LIBRARY_PATH=/usr/local/lib

ENTRYPOINT []
CMD /controller
