package main

import (
	"fmt"
	"image"
	"os"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rainbow"
	"gocv.io/x/gocv"
)

func main() {
	// Get name of file to analyze.
	filename := os.Args[1]
	if filename == "camera" {
		loopReadingCamera()
	} else {
		analyzeFile(filename)
	}
}

func loopReadingCamera() {
	webcam, err := gocv.VideoCaptureDevice(0)
	if err != nil {
		fmt.Printf("error opening video capture device: %v\n", 0)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	for {
		// This blocks until the next frame is ready.
		if ok := webcam.Read(img); !ok {
			fmt.Printf("cannot read device\n")
			return
			time.Sleep(1 * time.Millisecond)
			continue
		}
		if img.Empty() {
			fmt.Printf("no image on device\n")
			time.Sleep(1 * time.Millisecond)
			continue
		}

		// Convert to HSV and Resize to a width of 600.
		hsv := rainbow.ScaleAndConvertToHSV(img, 600)

		if pos, err := rainbow.FindBallPosition(hsv, rainbow.Balls["orange"]); err == nil {
			fmt.Printf("Found at %#v\n", pos)
		} else {
			fmt.Printf("Not found: %v\n", err)
		}
	}
}

func analyzeFile(filename string) {
	// Read that file (as BGR).
	img := gocv.IMRead(filename, gocv.IMReadColor)

	// Convert to HSV and Resize to a width of 600.
	hsv := rainbow.ScaleAndConvertToHSV(img, 600)

	// Also scale the original image, for easier and nicer display
	// of the result.
	fmt.Printf("Input size = %v x %v\n", img.Cols(), img.Rows())
	width := img.Cols()
	scaleFactor := float64(600) / float64(width)
	fmt.Printf("Scaling by %v\n", scaleFactor)
	gocv.Resize(img, img, image.Point{}, scaleFactor, scaleFactor, gocv.InterpolationLinear)
	fmt.Printf("Scaled size = %v x %v\n", img.Cols(), img.Rows())

	// Try to find balls.
	for color, hsvRange := range rainbow.Balls {
		fmt.Printf("Looking for %v ball...\n", color)
		if pos, err := rainbow.FindBallPosition(hsv, hsvRange); err == nil {
			fmt.Printf("Found at %#v\n", pos)
			rainbow.MarkBallPosition(img, pos)
		} else {
			fmt.Printf("Not found: %v\n", err)
		}
	}

	window := gocv.NewWindow("Hello")
	window.ResizeWindow(img.Cols(), img.Rows())
	for {
		window.IMShow(img)
		key := window.WaitKey(0)
		fmt.Printf("Key = %v\n", key)
		if key == 110 {
			break
		}
	}
}
