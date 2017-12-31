FROM resin/rpi-raspbian:latest
ENTRYPOINT []

RUN apt-get update && \
  wget https://github.com/parallaxinc/PropellerIDE/releases/download/0.38.5/propelleride-0.38.5-armhf.deb && \
  sh -c "dpkg -i propelleride-0.38.5-armhf.deb || true" && \
  apt-get install -y -f && \
  apt-get clean -y
COPY ./metabotspin/mb3.binary /
CMD propman mb3.binary
