package main

import (
	"context"
	"fmt"
	"github.com/quartercastle/vector"
	angle2 "github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
	"math"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/bno08x"
)

func main() {
	imu := bno08x.New()
	go imu.LoopReadingReports(context.Background())
	var once sync.Once
	var offset angle2.PlusMinus180
	imu.WaitForReportAfter(time.Now())
	for {
		rep := imu.CurrentReport()
		fmt.Printf("%v\n", rep)

		yaw := rep.YawRadians()
		pitch := rep.PitchRadians()
		roll := rep.RollRadians()
		fmt.Printf("Radians: yaw %0.3f pitch %0.3f roll  %0.3f\n", yaw, pitch, roll)

		rotOrth := func(vec, axis vector.Vector, theta float64) vector.Vector {
			axisCrossVec, err := axis.Cross(vec)
			if err != nil {
				fmt.Println("Cross prod err:", err)
			}
			vRot := vec.Scale(math.Cos(theta)).Add(axisCrossVec.Scale(math.Sin(theta)))
			return vRot
		}

		x1 := rotOrth(vector.X, vector.Z, yaw)
		y1 := rotOrth(vector.Y, vector.Z, yaw)
		z1 := vector.Z

		fmt.Printf("x1:    x %5f y %5f z %5f mag %f\n", x1[0], x1[1], x1[2], x1.Magnitude())
		fmt.Printf("y1:    x %5f y %5f z %5f mag %f\n", y1[0], y1[1], y1[2], x1.Magnitude())
		fmt.Printf("z1:    x %5f y %5f z %5f mag %f\n", z1[0], z1[1], z1[2], x1.Magnitude())

		x2 := rotOrth(x1, y1, pitch)
		y2 := y1
		z2 := rotOrth(z1, y1, pitch)

		fmt.Printf("x2:    x %5f y %5f z %5f mag %f\n", x2[0], x2[1], x2[2], x2.Magnitude())
		fmt.Printf("y2:    x %5f y %5f z %5f mag %f\n", y2[0], y2[1], y2[2], y2.Magnitude())
		fmt.Printf("z2:    x %5f y %5f z %5f mag %f\n", z2[0], z2[1], z2[2], z2.Magnitude())

		x3 := x2
		y3 := rotOrth(y2, x2, roll)
		z3 := rotOrth(z2, x2, roll)

		fmt.Printf("x3:    x %5f y %5f z %5f mag %f\n", x3[0], x3[1], x3[2], x3.Magnitude())
		fmt.Printf("y3:    x %5f y %5f z %5f mag %f\n", y3[0], y3[1], y3[2], y3.Magnitude())
		fmt.Printf("z3:    x %5f y %5f z %5f mag %f\n", z3[0], z3[1], z3[2], z3.Magnitude())

		z3[2] = 0
		angle := angle2.FromFloat(math.Atan2(z3[0], z3[1]) * 360 / (2 * math.Pi))
		once.Do(func() {
			offset = angle
		})

		fmt.Printf("Angle: %.2f\n", angle.Sub(offset).Float())

		time.Sleep(200 * time.Millisecond)
	}
}
