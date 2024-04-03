package picobldc

type distanceProvider interface {
	RawDistancesTraveled() (PerMotorVal[int16], error)
}

type DistanceTracker struct {
	pico distanceProvider

	doneFirstPoll bool
	lastRawValues PerMotorVal[int16]

	accumulator PerMotorVal[int64]
}

func NewDistanceTracker(pico distanceProvider) *DistanceTracker {
	return &DistanceTracker{
		pico: pico,
	}
}

func (d *DistanceTracker) Poll() error {
	raw, err := d.pico.RawDistancesTraveled()
	if err != nil {
		return err
	}

	if d.doneFirstPoll {
		for m, newD := range raw {
			oldD := d.lastRawValues[m]
			delta := newD - oldD
			d.accumulator[m] += int64(delta)
		}
	}

	d.lastRawValues = raw
	d.doneFirstPoll = true
	return nil
}

func (d *DistanceTracker) AccumulatedRotations() (rotations PerMotorVal[float64]) {
	for m, v := range d.accumulator {
		rotations[m] = float64(v) / 256.0
	}
	return
}

func (d *DistanceTracker) Zero() {
	d.accumulator = PerMotorVal[int64]{}
}
