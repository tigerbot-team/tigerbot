#!/bin/bash

trap "reset-prop; exit 0" SIGINT SIGTERM EXIT

function reset-prop() {
  echo "Resetting propeller"
  [ -e /sys/class/gpio/gpio17/direction ] || echo 17 > /sys/class/gpio/export
  echo "low" > /sys/class/gpio/gpio17/direction
  sleep 0.1
}

while true; do

  # Set up the camera.
  echo "Configuring camera..."
  modprobe bcm2835-v4l2
  v4l2-ctl -c auto_exposure=0 \
       -c auto_exposure_bias=20 \
       -c white_balance_auto_preset=2 \
       -c iso_sensitivity_auto=0 \
       -c iso_sensitivity=4 \
       -c exposure_metering_mode=1

  # Start the controller.
  docker run --rm -ti \
    --name controller \
    --net=host \
    --privileged \
    -v /dev:/dev \
    -v /tmp:/tmp \
    tigerbot/go-controller

  echo "Controller exited!!!"
  reset-prop
done
