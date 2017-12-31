FROM resin/rpi-raspbian:latest
ENTRYPOINT []

COPY mb3.binary /

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
  python-gobject \
  wget \
  && \
  wget https://github.com/parallaxinc/PropellerIDE/releases/download/0.38.5/propelleride-0.38.5-armhf.deb && \
  sh -c "dpkg -i propelleride-0.38.5-armhf.deb || true" && \
  apt-get install -y -f && \
  apt-get clean -y

RUN mkdir /controller
COPY requirements.txt tiny4wd.py joy.py simpleRobot.py /controller/
# COPY zerobot.apt-clone.tar.gz .
# RUN apt-clone restore zerobot.apt-clone.tar.gz
WORKDIR /controller
RUN pip install -r requirements.txt

CMD propman /mb3.binary && python tiny4wd.py
