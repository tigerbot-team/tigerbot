package main

import (
	"fmt"
	"image"
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

	// Resize to a width of 600.
	fmt.Printf("Input size = %v x %v\n", img.Cols(), img.Rows())
	width := img.Cols()
	scaleFactor := float64(600) / float64(width)
	fmt.Printf("Scaling by %v\n", scaleFactor)
	smaller := gocv.NewMat()
	gocv.Resize(img, smaller, image.Point{}, scaleFactor, scaleFactor, gocv.InterpolationLinear)
	fmt.Printf("Scaled size = %v x %v\n", smaller.Cols(), smaller.Rows())

	window := gocv.NewWindow("Hello")
	window.ResizeWindow(smaller.Cols(), smaller.Rows())
	for {
		window.IMShow(smaller)
		key := window.WaitKey(0)
		fmt.Printf("Key = %v\n", key)
		if key == 110 {
			break
		}
	}
}
