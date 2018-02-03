FROM resin/rpi-raspbian:latest

RUN apt-get update && \
  apt-get -qy install git wget less vim unzip gdebi-core

RUN wget https://github.com/parallaxinc/PropellerIDE/releases/download/0.38.5/propelleride-0.38.5-armhf.deb && \
    gdebi -n ./propelleride-0.38.5-armhf.deb

ENTRYPOINT []
CMD sh
