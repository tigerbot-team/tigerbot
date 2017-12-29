FROM resin/rpi-raspbian:latest
ENTRYPOINT []

RUN apt-get update && \
  apt-get -qy install \
  libusb-dev \
  libdbus-1-dev \
  libglib2.0-dev \
  libudev-dev \
  libical-dev \
  libreadline-dev \
  libsdl1.2-dev \
  pi-bluetooth \
  bluez \
  build-essential \
  python-pip \
  python-dev \
  python-gi \
  python-gobject

RUN mkdir /controller
COPY requirements.txt joy.py simpleRobot.py /controller/
# COPY zerobot.apt-clone.tar.gz .
# RUN apt-clone restore zerobot.apt-clone.tar.gz
WORKDIR /controller
RUN pip install -r requirements.txt
