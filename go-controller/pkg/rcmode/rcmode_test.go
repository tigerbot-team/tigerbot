package rcmode

import (
	"math"
	"testing"
)

func TestMix(t *testing.T) {
	fl, fr, bl, br := Mix(0, 0)
	if fl != 0 || fr != 0 || bl != 0 || br != 0 {
		t.Fatalf("Input of 0s should return 0s, not %v, %v, %v, %v", fl, fr, bl, br)
	}

	fl, fr, bl, br = Mix(math.MinInt16, 0)
	if fl != -63 || fr != 63 || bl != -63 || br != 63 {
		t.Fatalf("Input of full-left returned %v, %v, %v, %v", fl, fr, bl, br)
	}

	fl, fr, bl, br = Mix(math.MaxInt16, 0)
	if fl != 63 || fr != -63 || bl != 63 || br != -63 {
		t.Fatalf("Input of full-left returned %v, %v, %v, %v", fl, fr, bl, br)
	}
}
