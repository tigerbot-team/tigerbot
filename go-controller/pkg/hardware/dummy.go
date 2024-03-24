package hardware

import (
	"context"
	"fmt"
)

type Dummy struct {}

func NewDummy() *Dummy {
	return &Dummy{}
}

func (d *Dummy) Start(ctx context.Context) {
	fmt.Println("DHW: Start")
}

func (d *Dummy) StartRawControlMode() RawControl {
	fmt.Println("DHW: StartRawControlMode")
	return nil
}

func (d *Dummy) StartHeadingHoldMode() HeadingAbsolute {
	fmt.Println("DHW: StartHeadingHoldMode")
	return nil
}

func (d *Dummy) StartYawAndThrottleMode() HeadingRelative {
	fmt.Println("DHW: StartYawAndThrottleMode")
	return nil
}

func (d *Dummy) StopMotorControl() {
	fmt.Println("DHW: StopMotorControl")
}

func (d *Dummy) CurrentHeading() float64 {
	fmt.Println("DHW: CurrentHeading")
	return 0
}

func (d *Dummy) CurrentDistanceReadings(revision revision) DistanceReadings {
	fmt.Println("DHW: CurrentDistanceReadings")
	return DistanceReadings{}
}

func (d *Dummy) CurrentMotorDistances() (l, r float64) {
	fmt.Println("DHW: CurrentMotorDistances")
	return 0, 0
}

func (d *Dummy) SetServo(port int, value float64) {
	fmt.Printf("DHW: SetServo port=%v value=%v\n", port, value)
}

func (d *Dummy) SetPWM(port int, value float64) {
	fmt.Printf("DHW: SetPWM port=%v value=%v\n", port, value)
}

func (d *Dummy) PlaySound(path string) {
	fmt.Printf("DHW: PlaySound path=%v\n", path)
}

func (d *Dummy) Shutdown() {
	fmt.Println("DHW: Shutdown")
}

var _ Interface = (*Dummy)(nil)
