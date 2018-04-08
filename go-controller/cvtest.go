package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"

	"gocv.io/x/gocv"
)

type HSVRange struct {
	HueMin, HueMax byte
	SatMin, SatMax byte
	ValMin, ValMax byte
}

var Balls = map[string]*HSVRange{
	"yellow": &HSVRange{21, 42, 0, 255, 0, 255},
	"green":  &HSVRange{60, 100, 60, 255, 0, 255},
	"blue":   &HSVRange{100, 120, 90, 255, 80, 255},
	"orange": &HSVRange{165, 10, 120, 255, 60, 255},
}

type BallPosition struct {
	X      int
	Y      int
	Radius int
}

func HSVMaskNoWrapAround(hsv gocv.Mat, hsvRange *HSVRange) gocv.Mat {
	lb := gocv.NewMatFromBytes(1, 3, gocv.MatTypeCV8U, []byte{
		hsvRange.HueMin,
		hsvRange.SatMin,
		hsvRange.ValMin,
	})
	ub := gocv.NewMatFromBytes(1, 3, gocv.MatTypeCV8U, []byte{
		hsvRange.HueMax,
		hsvRange.SatMax,
		hsvRange.ValMax,
	})
	mask := gocv.NewMatWithSize(hsv.Rows(), hsv.Cols(), gocv.MatTypeCV8U)
	gocv.InRange(hsv, lb, ub, mask)
	return mask
}

func HSVMask(hsv gocv.Mat, hsvRange *HSVRange) gocv.Mat {
	if hsvRange.HueMax > hsvRange.HueMin {
		return HSVMaskNoWrapAround(hsv, hsvRange)
	} else {
		range1 := *hsvRange
		range1.HueMax = 180
		mask1 := HSVMaskNoWrapAround(hsv, &range1)
		range2 := *hsvRange
		range2.HueMin = 0
		mask2 := HSVMaskNoWrapAround(hsv, &range2)
		mask := gocv.NewMatWithSize(hsv.Rows(), hsv.Cols(), gocv.MatTypeCV8U)
		gocv.BitwiseOr(mask1, mask2, mask)
		return mask
	}
}

func FindBallPosition(hsv gocv.Mat, hsvRange *HSVRange) (pos BallPosition, err error) {
	mask := HSVMask(hsv, hsvRange)

	// Apply two iterations each of erosion and dilation, to remove noise.
	nullMat := gocv.NewMat()
	gocv.Erode(mask, mask, nullMat)
	gocv.Erode(mask, mask, nullMat)
	gocv.Dilate(mask, mask, nullMat)
	gocv.Dilate(mask, mask, nullMat)

	// Find contours.
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	if len(contours) > 0 {
		var (
			maxArea        float64
			largestContour []image.Point
		)
		maxArea = 0
		for _, c := range contours {
			area := gocv.ContourArea(c)
			if area > maxArea {
				maxArea = area
				largestContour = c
			}
		}
		boundingRect := gocv.BoundingRect(largestContour)
		rWidth := boundingRect.Max.X - boundingRect.Min.X
		rHeight := boundingRect.Max.Y - boundingRect.Min.Y
		if rWidth > 18 {
			if rHeight > 18 {
				pos.X = (boundingRect.Max.X + boundingRect.Min.X) / 2
				pos.Y = (boundingRect.Max.Y + boundingRect.Min.Y) / 2
				pos.Radius = (rWidth + rHeight) / 4
			} else {
				err = errors.New("largest contour not tall enough")
			}
		} else {
			err = errors.New("largest contour not wide enough")
		}
	} else {
		err = errors.New("didn't find any contours")
	}
	return
}

func MarkBallPosition(img gocv.Mat, pos BallPosition) {
	// Draw the circle and centroid on the image.
	gocv.Circle(img, image.Point{pos.X, pos.Y}, pos.Radius, color.RGBA{0, 255, 255, 0}, 2)
}

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

	// Try to find balls.
	hsv := gocv.NewMat()
	gocv.CvtColor(smaller, hsv, gocv.ColorBGRToHSV)
	for color, hsvRange := range Balls {
		fmt.Printf("Looking for %v ball...\n", color)
		if pos, err := FindBallPosition(hsv, hsvRange); err == nil {
			fmt.Printf("Found at %#v\n", pos)
			MarkBallPosition(smaller, pos)
		} else {
			fmt.Printf("Not found: %v\n", err)
		}
	}

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
