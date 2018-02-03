ifeq ($(shell uname -m),x86_64)
	ARCH_DEPS:=/proc/sys/fs/binfmt_misc/arm
endif

/proc/sys/fs/binfmt_misc/arm:
	echo ':arm:M::\x7fELF\x01\x01\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x28\x00:\xff\xff\xff\xff\xff\xff\xff\x00\xff\xff\xff\xff\xff\xff\xff\xff\xfe\xff\xff\xff:/usr/bin/qemu-arm-static:' | sudo tee /proc/sys/fs/binfmt_misc/register

build-image: $(ARCH_DEPS)
	docker build ./build -f build/Dockerfile -t tigerbot/build:latest

controller-image: $(ARCH_DEPS) metabotspin/mb3.binary
	docker build . -f python-controller/Dockerfile -t tigerbot/controller:latest

metabotspin/mb3.binary: build-image
	docker run --rm \
	           -v "$(shell pwd):/tigerbot" \
	           -w /tigerbot/metabotspin \
	           tigerbot/build:latest \
	           openspin mb3.spin

