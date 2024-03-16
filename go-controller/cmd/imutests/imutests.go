package main

import (
	"context"
	"fmt"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/bno08x"
)

func main() {
	imu := bno08x.New()
	go imu.LoopReadingReports(context.Background())
	for {
		r := imu.CurrentReport()
		fmt.Printf("%v\n", r)
		time.Sleep(200 * time.Millisecond)
	}
}
