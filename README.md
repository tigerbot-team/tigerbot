# tigerbot

- Building:
  - `make go-controller/bin/controller`
    - other targets: `make go-controller/bin/<target>`
   where targets in 
  `controller  ina219tests  joytests  screentests  servotests  spitests  toftests`
- Build image
  - `make go-controller-image`
- install
  - `make go-install-to-pi BOT_HOST=<pi IP>`
  - `make go-patch BOT_HOST=<pi IP>`
- run on Pi
  - ```
       docker run --rm -ti \
       -v `pwd`/controller:/controller \
       -v `pwd`/mb3.binary:/mb3.binary \
       -v /tmp:/tmp \
       -v /dev:/dev \
       --privileged \
       tigerbot/go-controller | tee controller.log
    ```

### Notes
- `toftests` binary needs to run inside container, so mount it in.

