package challengemode

import (
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/cameracontrol"
)

var cameraControl *cameracontrol.CameraControl

func init() {
	cameraControl = cameracontrol.New()
	err := cameraControl.Start()
	if err != nil {
		panic(err)
	}
}

func CameraExecute(log Log, req string) (string, error) {
	startTime := time.Now()
	rsp, err := cameraControl.Execute(req)
	log("CameraExecute: '%v', duration %v, rsp '%v'", req, time.Now().Sub(startTime), rsp)
	return rsp, err
}
