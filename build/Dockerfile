FROM resin/rpi-raspbian:latest
ENTRYPOINT []

RUN apt-get update && \
  apt-get -qy install git wget less vi unzip && \
  wget https://github.com/parallaxinc/PropellerIDE/releases/download/0.38.5/propelleride-0.38.5-armhf.deb
  # && \
RUN dpkg -i propelleride-0.38.5-armhf.deb || true
RUN apt-get install -f
