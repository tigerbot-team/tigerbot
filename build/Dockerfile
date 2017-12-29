FROM resin/rpi-raspbian:latest
ENTRYPOINT []

RUN apt-get update && \
  apt-get -qy install git wget less vi unzip && \
  wget https://github.com/parallaxinc/PropellerIDE/releases/download/0.38.5/propelleride-0.38.5-armhf.deb
  # && \
RUN dpkg -i propelleride-0.38.5-armhf.deb || true
RUN apt-get install -f

# Expects metabotspin repo to have been mounted at /metabotspin
# Compile and upload metabot code to propellor
RUN cd /metabotspin && openspin mb3.spin && propman mb3.binary
