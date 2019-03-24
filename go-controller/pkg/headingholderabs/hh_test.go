package headingholderabs

import (
	"math"
	"testing"
)

func TestClampHeading(t *testing.T) {
	expectClampResult(t, 0, 0, 0)
	expectClampResult(t, 0, 179, 179)
	expectClampResult(t, 0, -179, -179)
	expectClampResult(t, 0, 360, 0)
	expectClampResult(t, 0, 361, 1)
	expectClampResult(t, 0, 359, -1)

	expectClampResult(t, 100, 0, 0)
	expectClampResult(t, 100, 179, 179)
	expectClampResult(t, 100, -179, 181)
	expectClampResult(t, 100, 360, 0)
	expectClampResult(t, 100, 361, 1)
	expectClampResult(t, 100, 359, -1)
	expectClampResult(t, 100, -79, -79)
	expectClampResult(t, 100, -81, 279)
	expectClampResult(t, 100, 720, 0)
	expectClampResult(t, 100, 720+180, 180)

	expectClampResult(t, 420, -79, 281)
	expectClampResult(t, 420, -81, 279)
	expectClampResult(t, 420, 720, 360)
	expectClampResult(t, 420, 900, 540)
}

func expectClampResult(t *testing.T, cH, tH, eH float64) {
	aH := clampHeading(cH, tH)
	if math.Abs(aH-cH) > 180 {
		t.Errorf(">180 value for %f + %f = %f, expected %f", cH, tH, aH, eH)
	}

	a := math.Mod(aH, 360)
	if a < 0 {
		a += 360
	}

	b := math.Mod(tH, 360)
	if b < 0 {
		b += 360
	}

	if a != b {
		t.Errorf("Not equal mod 360: %f + %f = %f, expected %f", cH, tH, aH, eH)
	}

	if aH != eH {
		t.Errorf("Not equal to expected value: %f + %f = %f, expected %f", cH, tH, aH, eH)
	}
}
