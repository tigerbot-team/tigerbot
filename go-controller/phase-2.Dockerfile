# Start with a container that's already set up with OpenCV
# and do the builds in there.

FROM tigerbot/go-controller-phase-1:latest as build

COPY go-controller/bin/controller /go/src/github.com/tigerbot-team/tigerbot/go-controller/bin/controller
COPY go-controller/copy-libs /go/src/github.com/tigerbot-team/tigerbot/go-controller/copy-libs

WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller

CMD bin/controller
