FROM sgtwilko/rpi-raspbian-opencv:stretch-latest

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
RUN bash -c "cd $GOPATH/src/gocv.io/x/gocv && \
             source ./env.sh && \
             go run -v ./cmd/version/main.go"

COPY metabotspin/mb3.binary /mb3.binary
COPY go-controller/controller /controller

ENTRYPOINT []
CMD /controller
