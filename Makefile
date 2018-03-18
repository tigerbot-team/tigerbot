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
go-phase-1-image go-controller/controller: $(ARCH_DEPS) metabotspin/mb3.binary $(shell find go-controller -name '*.go') go-controller/phase-1.Dockerfile
	docker build . -f go-controller/phase-1.Dockerfile -t tigerbot/go-controller-phase-1:latest
	-docker rm -f tigerbot-build
	docker create --name=tigerbot-build tigerbot/go-controller-phase-1:latest
	docker cp tigerbot-build:/go/src/github.com/tigerbot-team/tigerbot/go-controller/controller go-controller/controller
	-docker rm -f tigerbot-build

PHONY: go-controller-image
go-controller-image go-controller-image.tar: go-phase-1-image
	docker build . -f go-controller/phase-2.Dockerfile -t tigerbot/go-controller:latest
	docker save tigerbot/go-controller:latest > go-controller-image.tar

go-install-to-pi: go-controller-image.tar
	rsync -zv --progress go-controller-image.tar pi@$(BOT_HOST):go-controller-image.tar
	ssh pi@$(BOT_HOST) docker load -i go-controller-image.tar

go-patch: go-controller/controller
	rsync -zv --progress go-controller/controller pi@$(BOT_HOST):controller
	@echo 'Now run the image with -v `pwd`/controller:/controller'

install-to-pi: controller-image.tar
	rsync -zv --progress controller-image.tar pi@$(BOT_HOST):controller-image.tar
	ssh pi@$(BOT_HOST) docker load -i controller-image.tar

metabotspin/mb3.binary: metabotspin/*.spin
	$(MAKE) build-image
	docker run --rm \
	           -v "$(shell pwd):/tigerbot" \
	           -w /tigerbot/metabotspin \
	           tigerbot/build:latest \
	           openspin mb3.spin

