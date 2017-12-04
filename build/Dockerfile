FROM resin/rpi-raspbian:latest
ENTRYPOINT []

RUN apt-get update && \
  apt-get -qy install git wget && \
  wget https://github.com/parallaxinc/PropellerIDE/releases/download/0.38.5/propelleride-0.38.5-armhf.deb
  # && \
RUN dpkg -i propelleride-*-armhf.deb || true
RUN apt-get install -f

RUN git clone https://github.com/jpalombo/metabotspin.git && cd metabotspin && ls -1 *.spin | xargs -I {} openspin {}
