package rainbow

import (
	"errors"
	"image"
	"image/color"

	"gocv.io/x/gocv"
)

type HSVRange struct {
	HueMin, HueMax byte
	SatMin, SatMax byte
	ValMin, ValMax byte
}

var Balls = map[string]*HSVRange{
	"yellow": &HSVRange{22, 42, 100, 255, 130, 255},
	"green":  &HSVRange{45, 100, 10, 255, 0, 255},
	"blue":   &HSVRange{100, 120, 90, 255, 80, 255},
	"red":    &HSVRange{165, 10, 100, 255, 60, 255},
}

type BallPosition struct {
	X      int
	Y      int
	Radius int
}

func ScaleAndConvertToHSV(img gocv.Mat, desiredWidth int) (hsv gocv.Mat) {
	// Scale to desired width.
	width := img.Cols()
	scaleFactor := float64(desiredWidth) / float64(width)
	scaled := gocv.NewMat()
	gocv.Resize(img, &scaled, image.Point{}, scaleFactor, scaleFactor, gocv.InterpolationLinear)
	defer scaled.Close()

	// Convert to HSV.
	hsv = gocv.NewMat()
	gocv.CvtColor(scaled, &hsv, gocv.ColorBGRToHSV)

	return
}

func HSVMaskNoWrapAround(hsv gocv.Mat, hsvRange *HSVRange) gocv.Mat {
	lb, _ := gocv.NewMatFromBytes(1, 3, gocv.MatTypeCV8U, []byte{
		hsvRange.HueMin,
		hsvRange.SatMin,
		hsvRange.ValMin,
	})
	defer lb.Close()
	ub, _ := gocv.NewMatFromBytes(1, 3, gocv.MatTypeCV8U, []byte{
		hsvRange.HueMax,
		hsvRange.SatMax,
		hsvRange.ValMax,
	})
	defer ub.Close()
	mask := gocv.NewMatWithSize(hsv.Rows(), hsv.Cols(), gocv.MatTypeCV8U)
	gocv.InRange(hsv, lb, ub, &mask)
	return mask
}

func HSVMask(hsv gocv.Mat, hsvRange *HSVRange) gocv.Mat {
	if hsvRange.HueMax > hsvRange.HueMin {
		return HSVMaskNoWrapAround(hsv, hsvRange)
	} else {
		range1 := *hsvRange
		range1.HueMax = 180
		mask1 := HSVMaskNoWrapAround(hsv, &range1)
		defer mask1.Close()
		range2 := *hsvRange
		range2.HueMin = 0
		mask2 := HSVMaskNoWrapAround(hsv, &range2)
		defer mask2.Close()
		mask := gocv.NewMatWithSize(hsv.Rows(), hsv.Cols(), gocv.MatTypeCV8U)
		gocv.BitwiseOr(mask1, mask2, &mask)
		return mask
	}
}

func FindBallPosition(hsv gocv.Mat, hsvRange *HSVRange) (pos BallPosition, err error) {
	mask := HSVMask(hsv, hsvRange)
	defer mask.Close()

	// Apply two iterations each of erosion and dilation, to remove noise.
	nullMat := gocv.NewMat()
	defer nullMat.Close()
	gocv.Erode(mask, &mask, nullMat)
	gocv.Erode(mask, &mask, nullMat)
	gocv.Dilate(mask, &mask, nullMat)
	gocv.Dilate(mask, &mask, nullMat)

	// Find contours.
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple).ToPoints()
	if len(contours) > 0 {
		var (
			maxArea        float64
			largestContour []image.Point
		)
		maxArea = 0
		for _, c := range contours {
			area := gocv.ContourArea(gocv.NewPointVectorFromPoints(c))
			if area > maxArea {
				maxArea = area
				largestContour = c
			}
		}
		boundingRect := gocv.BoundingRect(gocv.NewPointVectorFromPoints(largestContour))
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
	gocv.Circle(&img, image.Point{pos.X, pos.Y}, pos.Radius, color.RGBA{0, 255, 255, 0}, 2)
}
