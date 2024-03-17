package angle

import "math"

// PlusMinus180 is an angle in degrees, stored as a value in range (180, 180].
// All operations clamp their output into range.
type PlusMinus180 struct {
	float64
}

func (a PlusMinus180) Add(b PlusMinus180) PlusMinus180 {
	return FromFloat(a.float64 + b.float64)
}

func (a PlusMinus180) Sub(b PlusMinus180) PlusMinus180 {
	return FromFloat(a.float64 - b.float64)
}

func (a PlusMinus180) AddFloat(f float64) PlusMinus180 {
	return FromFloat(a.float64 + f)
}

func (a PlusMinus180) SubFloat(f float64) PlusMinus180 {
	return FromFloat(a.float64 - f)
}

// Float returns the angle in degrees, range (-180, 180].
func (a PlusMinus180) Float() float64 {
	return a.float64
}

// FromFloat converts a float of any magnitude to a PlusMinus180 by calculating
// f mod 360 and shifting into range.
func FromFloat(f float64) PlusMinus180 {
	d := math.Mod(f, 360)
	if d <= -180 {
		d += 360
	} else if d > 180 {
		d -= 360
	}
	return PlusMinus180{d}
}
