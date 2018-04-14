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

	var supportedProps = map[int]string{
		0:  "PosMsec",
		3:  "FrameWidth",
		4:  "FrameHeight",
		5:  "FPS",
		6:  "FOURCC",
		8:  "Format",
		9:  "Mode",
		10: "Brightness",
		11: "Contrast",
		12: "Saturation",
		15: "Exposure",
		21: "AutoExposure",
	}

	for i := 0; i <= 39; i++ {
		desc, ok := supportedProps[i]
		if ok {
			prop := gocv.VideoCaptureProperties(i)
			param := webcam.Get(prop)
			fmt.Printf("Video prop %v (%v) = %v\n", desc, prop, param)
		}
	}

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

		if pos, err := rainbow.FindBallPosition(hsv, rainbow.Balls["red"]); err == nil {
			fmt.Printf("Found at %#v\n", pos)
		} else {
			fmt.Printf("Not found: %v\n", err)
		}

		time.Sleep(500 * time.Millisecond)
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
