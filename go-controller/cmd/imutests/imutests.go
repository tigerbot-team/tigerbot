package main

import (
	"context"
	"fmt"
	"github.com/quartercastle/vector"
	angle2 "github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
	"gonum.org/v1/gonum/spatial/r3"
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

		angle := calculateRobotYaw(yaw, pitch, roll)
		once.Do(func() {
			offset = angle
		})

		fmt.Printf("Angle: %.2f\n", angle.Sub(offset).Float())

		angle = calculateRobotYawGonum(yaw, pitch, roll)
		fmt.Printf("Angle gonum: %.2f\n", angle.Sub(offset).Float())

		angle = rep.RobotYaw()
		fmt.Printf("Angle lib: %.2f\n", angle.Sub(offset).Float())

		time.Sleep(200 * time.Millisecond)
	}
}

// Prototype code, now moved to the bno08x library...

func calculateRobotYawGonum(yaw float64, pitch float64, roll float64) angle2.PlusMinus180 {
	rotOrth := func(vec, axis r3.Vec, theta float64) r3.Vec {
		return r3.Rotate(vec, theta, axis)
	}

	// Rotate the axes yaw radians around Z.
	x0 := r3.Vec{X: 1}
	z0 := r3.Vec{Z: 1}
	y0 := r3.Vec{Y: 1}

	x1 := rotOrth(x0, z0, yaw)
	y1 := rotOrth(y0, z0, yaw)
	z1 := z0

	fmt.Printf("x1:    x %5f y %5f z %5f mag %f\n", x1.X, x1.Y, x1.Z, r3.Norm(x1))
	fmt.Printf("y1:    x %5f y %5f z %5f mag %f\n", y1.X, y1.Y, y1.Z, r3.Norm(x1))
	fmt.Printf("z1:    x %5f y %5f z %5f mag %f\n", z1.X, z1.Y, z1.Z, r3.Norm(x1))

	// Rotate pitch radians around the *new* Y.
	x2 := rotOrth(x1, y1, pitch)
	y2 := y1
	z2 := rotOrth(z1, y1, pitch)

	fmt.Printf("x2:    x %5f y %5f z %5f mag %f\n", x2.X, x2.Y, x2.Z, r3.Norm(x2))
	fmt.Printf("y2:    x %5f y %5f z %5f mag %f\n", y2.X, y2.Y, y2.Z, r3.Norm(y2))
	fmt.Printf("z2:    x %5f y %5f z %5f mag %f\n", z2.X, z2.Y, z2.Z, r3.Norm(z2))

	// Rotate roll radians around the new X.
	x3 := x2
	y3 := rotOrth(y2, x2, roll)
	z3 := rotOrth(z2, x2, roll)

	fmt.Printf("x3:    x %5f y %5f z %5f mag %f\n", x3.X, x3.Y, x3.Z, r3.Norm(x3))
	fmt.Printf("y3:    x %5f y %5f z %5f mag %f\n", y3.X, y3.Y, y3.Z, r3.Norm(y3))
	fmt.Printf("z3:    x %5f y %5f z %5f mag %f\n", z3.X, z3.Y, z3.Z, r3.Norm(z3))

	// Take the x and y components of the final Z vector.  The Z vector points towards the
	// front of the robot.
	angle := angle2.FromFloat(math.Atan2(z3.X, z3.Y) * 360 / (2 * math.Pi))
	return angle
}

func calculateRobotYaw(yaw float64, pitch float64, roll float64) angle2.PlusMinus180 {
	rotOrth := func(vec, axis vector.Vector, theta float64) vector.Vector {
		// The vector library's Rotate method is bugged.  Had to implement our own
		// but hten switched to gonum.

		// This is Rodrigues' algorithm specialised for orthogonal unit vectors (i.e. omitting the
		// final term, which is always zero for orthogonal unit vectors).
		axisCrossVec, err := axis.Cross(vec)
		if err != nil {
			fmt.Println("Cross prod err:", err)
		}
		vRot := vec.Scale(math.Cos(theta)).Add(axisCrossVec.Scale(math.Sin(theta)))
		return vRot
	}

	// Rotate the axes yaw radians around Z.
	x1 := rotOrth(vector.X, vector.Z, yaw)
	y1 := rotOrth(vector.Y, vector.Z, yaw)
	z1 := vector.Z

	fmt.Printf("x1:    x %5f y %5f z %5f mag %f\n", x1[0], x1[1], x1[2], x1.Magnitude())
	fmt.Printf("y1:    x %5f y %5f z %5f mag %f\n", y1[0], y1[1], y1[2], x1.Magnitude())
	fmt.Printf("z1:    x %5f y %5f z %5f mag %f\n", z1[0], z1[1], z1[2], x1.Magnitude())

	// Rotate pitch radians around the *new* Y.
	x2 := rotOrth(x1, y1, pitch)
	y2 := y1
	z2 := rotOrth(z1, y1, pitch)

	fmt.Printf("x2:    x %5f y %5f z %5f mag %f\n", x2[0], x2[1], x2[2], x2.Magnitude())
	fmt.Printf("y2:    x %5f y %5f z %5f mag %f\n", y2[0], y2[1], y2[2], y2.Magnitude())
	fmt.Printf("z2:    x %5f y %5f z %5f mag %f\n", z2[0], z2[1], z2[2], z2.Magnitude())

	// Rotate roll radians around the new X.
	x3 := x2
	y3 := rotOrth(y2, x2, roll)
	z3 := rotOrth(z2, x2, roll)

	fmt.Printf("x3:    x %5f y %5f z %5f mag %f\n", x3[0], x3[1], x3[2], x3.Magnitude())
	fmt.Printf("y3:    x %5f y %5f z %5f mag %f\n", y3[0], y3[1], y3[2], y3.Magnitude())
	fmt.Printf("z3:    x %5f y %5f z %5f mag %f\n", z3[0], z3[1], z3[2], z3.Magnitude())

	// Take the x and y components of the final Z vector.  The Z vector points towards the
	// front of the robot.
	angle := angle2.FromFloat(math.Atan2(z3[0], z3[1]) * 360 / (2 * math.Pi))
	return angle
}
