# Start with a container that's already set up with OpenCV
# and do the builds in there.

FROM tigerbot/go-controller-phase-1:latest as build

COPY go-controller/controller /go/src/github.com/tigerbot-team/tigerbot/go-controller/controller
COPY go-controller/copy-libs /go/src/github.com/tigerbot-team/tigerbot/go-controller/copy-libs

WORKDIR $GOPATH/src/github.com/tigerbot-team/tigerbot/go-controller

# Copy the shared libraries that the controller uses to a designated
# directory so that they're easy to find in the next phase.
RUN bash -c "source /go/src/gocv.io/x/gocv/env.sh && \
             ./copy-libs"

# Now build the container image that we actually ship by copying
# across only the relevant files.  We start with alpins since it's
# nice and small to start with but we'll be throwing in a lot
# of glibc-linked binaries so the resulting system will be a bit
# of a hybrid.

FROM resin/raspberry-pi-alpine:latest

RUN apk --no-cache add util-linux strace

RUN mkdir -p /usr/local/lib
COPY metabotspin/mb3.binary /mb3.binary
COPY --from=build /usr/bin/propman /usr/bin/propman
COPY --from=build /lib/ld-linux-armhf.so* /lib
COPY --from=build /controller-libs/* /usr/local/lib/
COPY --from=build /go/src/github.com/tigerbot-team/tigerbot/go-controller/controller /controller
COPY --from=build /go/src/github.com/tigerbot-team/tigerbot/VL53L0X_rasp/bin/* /usr/local/bin/
ENV LD_LIBRARY_PATH=/usr/local/lib

ENTRYPOINT []
CMD /controller
