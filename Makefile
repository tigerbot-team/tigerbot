BOT_HOST ?= tigerbot

ifeq ($(shell uname -m),x86_64)
	ARCH_DEPS:=/proc/sys/fs/binfmt_misc/arm
endif

/proc/sys/fs/binfmt_misc/arm:
	echo ':arm:M::\x7fELF\x01\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x28\x00:\xff\xff\xff\xff\xff\xff\xff\x00\xff\xff\xff\xff\xff\xff\xff\xff\xfe\xff\xff\xff:/usr/bin/qemu-arm-static:' | sudo tee /proc/sys/fs/binfmt_misc/register

build-image: $(ARCH_DEPS)
	docker build ./build -f build/Dockerfile -t tigerbot/build:latest

controller-image: $(ARCH_DEPS) metabotspin/mb3.binary
	docker build . -f python-controller/Dockerfile -t tigerbot/controller:latest

controller-image.tar: metabotspin/mb3.binary python-controller/*
	$(MAKE) controller-image
	docker save tigerbot/controller:latest > controller-image.tar

PHONY: go-phase-1-image
go-phase-1-image: $(ARCH_DEPS) go-controller/phase-1.Dockerfile
	docker build -f go-controller/phase-1.Dockerfile -t tigerbot/go-controller-phase-1:latest .

go-controller/controller: $(ARCH_DEPS) $(shell find go-controller -name '*.go') go-controller/phase-1.Dockerfile
	$(MAKE) go-phase-1-image
	-mkdir .go-cache
	docker run --rm -v `pwd`/go-controller:/go/src/github.com/tigerbot-team/tigerbot/go-controller \
	                -v `pwd`/.go-cache:/go-cache \
	                -e GOCACHE=/go-cache \
	                -w /go/src/github.com/tigerbot-team/tigerbot/go-controller \
	                tigerbot/go-controller-phase-1:latest \
	                bash -c "source /go/src/gocv.io/x/gocv/env.sh && \
	                         GOMAXPROCS=1 go build -p 1 -v controller.go"

go-controller/spitests: $(ARCH_DEPS) $(shell find go-controller -name '*.go') go-controller/phase-1.Dockerfile
	$(MAKE) go-phase-1-image
	-mkdir .go-cache
	docker run --rm -v `pwd`/go-controller:/go/src/github.com/tigerbot-team/tigerbot/go-controller \
	                -v `pwd`/.go-cache:/go-cache \
	                -e GOCACHE=/go-cache \
	                -w /go/src/github.com/tigerbot-team/tigerbot/go-controller \
	                tigerbot/go-controller-phase-1:latest \
	                bash -c "source /go/src/gocv.io/x/gocv/env.sh && \
	                         GOMAXPROCS=1 go build -p 1 -v spitests.go"

PHONY: go-controller-image
go-controller-image go-controller-image.tar: go-controller/controller metabotspin/mb3.binary go-controller/sounds/* go-controller/*.Dockerfile go-controller/copy-libs
	docker build . -f go-controller/phase-2.Dockerfile -t tigerbot/go-controller:latest
	docker save tigerbot/go-controller:latest > go-controller-image.tar

go-install-to-pi: go-controller-image.tar
	rsync -zv --progress go-controller-image.tar pi@$(BOT_HOST):go-controller-image.tar
	ssh pi@$(BOT_HOST) docker load -i go-controller-image.tar

go-patch: go-controller/controller metabotspin/mb3.binary
	rsync -zv --progress go-controller/controller pi@$(BOT_HOST):controller
	rsync -zv --progress metabotspin/mb3.binary pi@$(BOT_HOST):mb3.binary
	@echo 'Now run the image with -v `pwd`/controller:/controller -v `pwd`/mb3.binary:/mb3.binary'

install-to-pi: controller-image.tar
	rsync -zv --progress controller-image.tar pi@$(BOT_HOST):controller-image.tar
	ssh pi@$(BOT_HOST) docker load -i controller-image.tar

metabotspin/mb3.binary: metabotspin/*.spin
	$(MAKE) build-image
	rm -f metabotspin/mb3.binary
	docker run --rm \
	           -v "$(shell pwd):/tigerbot" \
	           -w /tigerbot/metabotspin \
	           tigerbot/build:latest \
	           openspin mb3.spin

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

enter-dev-image:
	docker run --rm -it \
	    --net=host \
	    -v /home/pi/.Xauthority:/.Xauthority -e XAUTHORITY=/.Xauthority \
	    -e DISPLAY=127.0.0.1:10.0 -v /tmp/.X11-unix:/tmp/.X11-unix \
	    -v `pwd`:$(TIGERBOT) \
	    -w $(TIGERBOT)/go-controller \
	    $(GOCV_DEV_IMAGE) \
	    bash
