package main

import (
	"fmt"
	"os"
	"runtime"

	"gocv.io/x/gocv"
)

func main() {
	fmt.Print("---- Hello World! ----\n\n")
	fmt.Println("GOMAXPROCS", runtime.GOMAXPROCS(0))

	// Get name of file to analyze.
	filename := os.Args[1]

	// Read that file, as BGR.
	img := gocv.IMRead(filename, gocv.IMReadColor)

	//	// Resize to a width of 600.
	//	width := img.Cols()
	//	scaleFactor := float64(600) / width
	//	gocv.Resize(img, img, image.Point{}, scaleFactor, scaleFactor, gocv.InterpolationLinear)

	window := gocv.NewWindow("Hello")
	for {
		window.IMShow(img)
		if window.WaitKey(1) >= 0 {
			break
		}
	}
}
