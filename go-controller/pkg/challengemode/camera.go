package challengemode

import "github.com/tigerbot-team/tigerbot/go-controller/pkg/cameracontrol"

var cameraControl *cameracontrol.CameraControl

func init() {
	cameraControl = cameracontrol.New()
	err := cameraControl.Start()
	if err != nil {
		panic(err)
	}
}

func CameraExecute(req string) (string, error) {
	return cameraControl.Execute(req)
}
