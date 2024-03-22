BOT_HOST ?= tigerbot

ifeq ($(shell uname -m),x86_64)
	ARCH_DEPS:=/proc/sys/fs/binfmt_misc/arm
endif

/proc/sys/fs/binfmt_misc/arm:
	echo ':arm:M::\x7fELF\x01\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x28\x00:\xff\xff\xff\xff\xff\xff\xff\x00\xff\xff\xff\xff\xff\xff\xff\xff\xfe\xff\xff\xff:/usr/bin/qemu-arm-static:' | sudo tee /proc/sys/fs/binfmt_misc/register

build-image: $(ARCH_DEPS)
	docker build ./build -f build/Dockerfile -t tigerbot/build:latest

python-controller-image: $(ARCH_DEPS) metabotspin/mb3.binary
	docker build . -f python-controller/Dockerfile -t tigerbot/controller:latest

PHONY: go-phase-1-image
go-phase-1-image: $(ARCH_DEPS) go-controller/phase-1.Dockerfile
	docker build -f go-controller/phase-1.Dockerfile -t tigerbot/go-controller-phase-1:latest .

go-controller/bin/%: go-controller/cmd/%/*.go $(ARCH_DEPS) $(shell find go-controller -name '*.go') go-controller/phase-1.Dockerfile
	$(MAKE) go-phase-1-image
	-mkdir -p .go-cache
	-mkdir -p go-controller/bin
	docker run --rm -v `pwd`/go-controller:/go/src/github.com/tigerbot-team/tigerbot/go-controller \
	                -v `pwd`/.go-cache:/go-cache \
	                -e GOCACHE=/go-cache \
	                -w /go/src/github.com/tigerbot-team/tigerbot/go-controller \
	                tigerbot/go-controller-phase-1:latest \
	                bash -c "GOMAXPROCS=1 go build -p 1 -v -o ../$@ ../$(<D)"

PHONY: go-controller-image
go-controller-image go-controller-image.tar: go-controller/bin/controller go-controller/sounds/* go-controller/*.Dockerfile go-controller/copy-libs
	docker build . -f go-controller/phase-2.Dockerfile -t tigerbot/go-controller:latest
	#docker save tigerbot/go-controller:latest > go-controller-image.tar

go-install-to-pi: go-controller-image.tar
	rsync -zv --progress go-controller-image.tar pi@$(BOT_HOST):go-controller-image.tar
	ssh pi@$(BOT_HOST) docker load -i go-controller-image.tar

go-patch: go-controller/bin/controller metabotspin/mb3.binary
	rsync -zv --progress go-controller/bin/controller pi@$(BOT_HOST):controller
	@echo 'Now run the image with -v `pwd`/controller:/controller'

build-on-pi:
	rsync -zv --progress -r ./ pi@$(BOT_HOST):tigerbot-build
	ssh pi@$(BOT_HOST) make --directory tigerbot-build go-controller-image

# Building and using a container image with Go, OpenCV and GOCV.

GOCV_DEV_IMAGE = tigerbot/go-dev
TIGERBOT = /go/src/github.com/tigerbot-team/tigerbot
GOCV = /go/src/gocv.io/x/gocv

gocv-dev-image: $(ARCH_DEPS)
	docker build -f go-controller/Dockerfile-dev -t $(GOCV_DEV_IMAGE) .

go-controller/cvtest: go-controller/cvtest.go
	sudo docker run --rm \
	    -v /root/.cache:/root/.cache \
	    -v `pwd`:$(TIGERBOT) \
	    -w $(TIGERBOT)/go-controller \
	    $(GOCV_DEV_IMAGE) \
	    bash -c "source $(GOCV)/env.sh && GOMAXPROCS=1 go build -p 1 -v cvtest.go"

run-cvtest: go-controller/cvtest
	sudo modprobe bcm2835-v4l2
	docker run --rm -it \
	    --net=host \
	    -v /dev:/dev --privileged \
	    -v /home/pi/.Xauthority:/.Xauthority -e XAUTHORITY=/.Xauthority \
	    -e DISPLAY=127.0.0.1:10.0 -v /tmp/.X11-unix:/tmp/.X11-unix \
	    -v `pwd`:$(TIGERBOT) \
	    -w $(TIGERBOT)/go-controller \
	    $(GOCV_DEV_IMAGE) \
	    ./cvtest $(CVTEST_ARGS)

run-cvtest2: go-controller/bin/cvtest
	docker run --rm -it \
	    --net=host \
	    -v /dev:/dev --privileged \
	    -v /home/pi/.Xauthority:/.Xauthority -e XAUTHORITY=/.Xauthority \
	    -e DISPLAY=127.0.0.1:10.0 -v /tmp/.X11-unix:/tmp/.X11-unix \
	    -v `pwd`:$(TIGERBOT) \
	    -w $(TIGERBOT)/go-controller \
	    -v /usr/bin/libcamerify:/libcamerify \
	    -v /usr/lib/arm-linux-gnueabihf/libcamera:/usr/lib/arm-linux-gnueabihf/libcamera \
	    -v /usr/lib/arm-linux-gnueabihf/libcamera.so.0.2:/usr/lib/arm-linux-gnueabihf/libcamera.so.0.2 \
	    -v /usr/lib/arm-linux-gnueabihf/libcamera.so.0.2.0:/usr/lib/arm-linux-gnueabihf/libcamera.so.0.2.0 \
	    -v /usr/lib/arm-linux-gnueabihf/libcamera-base.so.0.2:/usr/lib/arm-linux-gnueabihf/libcamera-base.so.0.2 \
	    -v /usr/lib/arm-linux-gnueabihf/libcamera-base.so.0.2.0:/usr/lib/arm-linux-gnueabihf/libcamera-base.so.0.2.0 \
	    -v /usr/lib/arm-linux-gnueabihf/libpisp.so.1:/usr/lib/arm-linux-gnueabihf/libpisp.so.1 \
	    -v /usr/lib/arm-linux-gnueabihf/libpisp.so.1.0.4:/usr/lib/arm-linux-gnueabihf/libpisp.so.1.0.4 \
	    -v /usr/lib/arm-linux-gnueabihf/liblttng-ust.so.1:/usr/lib/arm-linux-gnueabihf/liblttng-ust.so.1 \
	    -v /usr/lib/arm-linux-gnueabihf/liblttng-ust.so.1.0.0:/usr/lib/arm-linux-gnueabihf/liblttng-ust.so.1.0.0 \
	    -v /usr/lib/arm-linux-gnueabihf/libyaml-0.so.2:/usr/lib/arm-linux-gnueabihf/libyaml-0.so.2 \
	    -v /usr/lib/arm-linux-gnueabihf/libyaml-0.so.2.0.9:/usr/lib/arm-linux-gnueabihf/libyaml-0.so.2.0.9 \
	    -v /usr/lib/arm-linux-gnueabihf/libdw.so.1:/usr/lib/arm-linux-gnueabihf/libdw.so.1 \
	    -v /usr/lib/arm-linux-gnueabihf/libdw-0.188.so:/usr/lib/arm-linux-gnueabihf/libdw-0.188.so \
	    -v /usr/lib/arm-linux-gnueabihf/libunwind.so.8:/usr/lib/arm-linux-gnueabihf/libunwind.so.8 \
	    -v /usr/lib/arm-linux-gnueabihf/libunwind.so.8.0.1:/usr/lib/arm-linux-gnueabihf/libunwind.so.8.0.1 \
	    -v /usr/lib/arm-linux-gnueabihf/libboost_log.so.1.74.0:/usr/lib/arm-linux-gnueabihf/libboost_log.so.1.74.0 \
	    -v /usr/lib/arm-linux-gnueabihf/libboost_thread.so.1.74.0:/usr/lib/arm-linux-gnueabihf/libboost_thread.so.1.74.0 \
	    -v /usr/lib/arm-linux-gnueabihf/liblttng-ust-common.so.1:/usr/lib/arm-linux-gnueabihf/liblttng-ust-common.so.1 \
	    -v /usr/lib/arm-linux-gnueabihf/liblttng-ust-common.so.1.0.0:/usr/lib/arm-linux-gnueabihf/liblttng-ust-common.so.1.0.0 \
	    -v /usr/lib/arm-linux-gnueabihf/liblttng-ust-tracepoint.so.1:/usr/lib/arm-linux-gnueabihf/liblttng-ust-tracepoint.so.1 \
	    -v /usr/lib/arm-linux-gnueabihf/liblttng-ust-tracepoint.so.1.0.0:/usr/lib/arm-linux-gnueabihf/liblttng-ust-tracepoint.so.1.0.0 \
	    -v /usr/lib/arm-linux-gnueabihf/libboost_filesystem.so.1.74.0:/usr/lib/arm-linux-gnueabihf/libboost_filesystem.so.1.74.0 \
	    -v /usr/lib/arm-linux-gnueabihf/libffi.so.8:/usr/lib/arm-linux-gnueabihf/libffi.so.8 \
	    -v /usr/lib/arm-linux-gnueabihf/libc.so.6:/lib/arm-linux-gnueabihf/libc.so.6 \
	    tigerbot/go-controller-phase-1:latest \
	    /libcamerify /bin/bash -c "echo $(CVTEST_ARGS)"
	    #/libcamerify ./bin/cvtest $(CVTEST_ARGS) \

enter-dev-image:
	docker run --rm -it \
	    --net=host \
	    -v /home/pi/.Xauthority:/.Xauthority -e XAUTHORITY=/.Xauthority \
	    -e DISPLAY=127.0.0.1:10.0 -v /tmp/.X11-unix:/tmp/.X11-unix \
	    -v `pwd`:$(TIGERBOT) \
	    -w $(TIGERBOT)/go-controller \
	    $(GOCV_DEV_IMAGE) \
	    bash
