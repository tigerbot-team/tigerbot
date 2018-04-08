package main

import (
	"fmt"
	"image"
	"os"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/rainbow"
	"gocv.io/x/gocv"
)

func main() {
	// Get name of file to analyze.
	filename := os.Args[1]

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
