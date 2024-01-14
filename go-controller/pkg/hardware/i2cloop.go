package hardware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/pca9685"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/screen"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/ina219"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/tofsensor"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/propeller"
)

const (
	NumServoPorts = 16

	NoteMux      = "MUX"
	NoteProp     = "PROP"
	NoteTOFs     = "DISTANCE"
	NoteServo    = "SERVO"
	NotePowerMon = "PWR MON"

	motorToMMScaleFactor = 7.384
)

type I2CController struct {
	lock sync.Mutex

	// Desired values.  Stored off in case we need to re-initialise the hardware.
	motorL, motorR      int8
	pwmPorts            map[int]pwmTypes // Either servoPosition or pwmValue
	pwmPortsWithUpdates map[int]bool

	prop        propeller.Interface
	tofsEnabled bool

	revisionUpdated               *sync.Cond
	nextRevision                  revision
	distanceReadings              DistanceReadings
	leftMotorDist, rightMotorDist float64
}

type pwmTypes interface {
	pwmsOnly()
}

type servoPosition float64

func (servoPosition) pwmsOnly() {}

type pwmValue float64

func (pwmValue) pwmsOnly() {}

func NewI2CController() *I2CController {
	c := &I2CController{
		pwmPorts:            map[int]pwmTypes{},
		pwmPortsWithUpdates: map[int]bool{},

		tofsEnabled: true,

		nextRevision: 1,
	}
	c.revisionUpdated = sync.NewCond(&c.lock)
	return c
}

func (c *I2CController) SetToFsEnabled(enabled bool) {
	c.lock.Lock()
	c.tofsEnabled = enabled
	c.lock.Unlock()
}

func (c *I2CController) SetMotorSpeeds(left, right int8) {
	c.lock.Lock()
	c.motorL = left
	c.motorR = right
	c.lock.Unlock()
}

func (c *I2CController) SetServo(n int, value float64) {
	c.lock.Lock()
	c.pwmPorts[n] = servoPosition(value)
	c.pwmPortsWithUpdates[n] = true
	c.lock.Unlock()
}

func (c *I2CController) SetPWM(n int, value float64) {
	c.lock.Lock()
	c.pwmPorts[n] = pwmValue(value)
	c.pwmPortsWithUpdates[n] = true
	c.lock.Unlock()
}

func (c *I2CController) CurrentDistanceReadings(rev revision) DistanceReadings {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Wait for a new revision.
	for c.distanceReadings.Revision <= rev {
		c.revisionUpdated.Wait()
	}

	return c.distanceReadings
}
func (c *I2CController) CurrentMotorDistances() (l, r float64) {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.leftMotorDist, c.rightMotorDist
}

func (c *I2CController) Loop(ctx context.Context, initDone *sync.WaitGroup) {
	fmt.Println("I2C loop started")
	for {
		c.loopUntilSomethingBadHappens(ctx, initDone)
		if ctx.Err() != nil {
			return
		}
		fmt.Println("===== !!! WARNING !!! I2C FAILURE; TRYING TO RECOVER =====")
		initDone = nil
	}
}

func (c *I2CController) loopUntilSomethingBadHappens(ctx context.Context, initDone *sync.WaitGroup) {
	defer func() {
		if initDone != nil {
			initDone.Done()
		}
	}()

	mx, err := mux.New("/dev/i2c-1")
	if err != nil {
		screen.SetNotice(NoteMux, screen.LevelErr)
		fmt.Println("Failed to open mux", err)
		return
	}
	defer mx.Close()

	err = mx.SelectSinglePort(mux.BusPropeller)
	if err != nil {
		fmt.Println("Failed to select mux port", err)
		return
	}
	screen.ClearNotice(NoteMux)

	prop, err := propeller.New()
	if err != nil {
		fmt.Println("Failed to open prop", err)
		screen.SetNotice(NoteProp, screen.LevelErr)
		return
	}
	defer prop.Close()

	var tofs []tofsensor.Interface
	defer func() {
		for _, tof := range tofs {
			_ = tof.Close()
		}
	}()
	for _, port := range []int{
		mux.BusTOFLeftRear, mux.BusTOFLeftFront,
		mux.BusTOFForwardLeft, mux.BusTOFForwardRight,
		mux.BusTOFRightFront, mux.BusTOFRightRear,
	} {
		fmt.Println("Initialising ToF ", port)

		err := mx.SelectSinglePort(port)
		if err != nil {
			fmt.Println("Failed to select mux port", err)
			return
		}

		tof, err := tofsensor.New("/dev/i2c-1", 0x29, byte(0x30+port))
		if err != nil {
			tof, err = tofsensor.New("/dev/i2c-1", byte(0x30+port))
			if err != nil {
				fmt.Println("Failed to open sensor", err)
				return
			}
		}

		err = tof.StartContinuousMeasurements()
		if err != nil {
			fmt.Println("Failed to start continuous measurements", err)
			return
		}
		tofs = append(tofs, tof)
	}

	readTofs := func() (DistanceReadings, error) {
		err := mx.SelectMultiplePorts(0x3f)
		readings := DistanceReadings{
			CaptureTime: time.Now(),
			Readings:    make([]Reading, len(tofs)),
			Revision:    c.nextRevision,
		}
		c.nextRevision++
		if err != nil {
			screen.SetNotice(NoteMux, screen.LevelErr)
			fmt.Println("Failed to select mux port", err)
			return readings, err
		}
		someErrors := false
		for j, tof := range tofs {
			readingInMM, err := tof.GetNextContinuousMeasurement()
			readings.Readings[j] = Reading{
				DistanceMM: readingInMM,
				Error:      err,
			}
			if err != nil {
				someErrors = true
			}
			if someErrors {
				screen.SetNotice(NoteTOFs, screen.LevelErr)
			} else {
				screen.ClearNotice(NoteTOFs)
			}
		}
		return readings, nil
	}

	err = mx.SelectSinglePort(mux.BusOthers)
	if err != nil {
		screen.SetNotice(NoteMux, screen.LevelErr)
		fmt.Println("Failed to select mux port", err)
		return
	}
	var powerSensors []ina219.Interface
	for _, addr := range []int{0x41, 0x44} {
		pwrSen, err := ina219.NewI2C("/dev/i2c-1", addr)
		if err != nil {
			fmt.Println("Failed to open power sensor; ignoring! ", err)
			continue
		}
		shuntOhms := 0.1
		if addr == ina219.Addr2 {
			// Motor sensor has a smaller shunt.
			shuntOhms = 0.05
		}
		err = pwrSen.Configure(shuntOhms, 10)
		if err != nil {
			fmt.Println("Failed to open power sensor; ignoring! ", err)
			continue
		}
		powerSensors = append(powerSensors, pwrSen)
	}

	dummyServos := pca9685.Dummy()
	var servos = dummyServos
	defer func() {
		err = mx.SelectSinglePort(mux.BusOthers)
		if err != nil {
			fmt.Println("Failed to select port when shutting down servos: ", err)
			screen.SetNotice(NoteMux, screen.LevelErr)
			return
		}
		_ = servos.Close()
	}()

	var lastServoInitTime time.Time

	resetOrDummyOutServos := func() error {
		fmt.Println("Resetting servos...")
		lastServoInitTime = time.Now()
		if servos != dummyServos {
			fmt.Println("Closing old servo controller.")
			_ = servos.Close()
			servos = dummyServos
		}
		err = mx.SelectSinglePort(mux.BusOthers)
		if err != nil {
			screen.SetNotice(NoteMux, screen.LevelErr)
			fmt.Println("Failed to select mux port", err)
			return err
		}
		servos, err = pca9685.New("/dev/i2c-1")
		if err != nil {
			fmt.Println("Failed open PCA9685 ", err)
			screen.SetNotice(NoteServo, screen.LevelErr)
			servos = dummyServos
		}
		fmt.Println("Opened PCA9685.")
		err = servos.Configure()
		if err != nil {
			fmt.Println("Failed to configure PCA9685", err)
			screen.SetNotice(NoteServo, screen.LevelErr)
			servos = dummyServos
		}
		fmt.Println("Configured PCA9685.")
		if servos != dummyServos {
			screen.ClearNotice(NoteServo)
		}

		// We may have been reset, queue servo updates for all the ports.
		c.lock.Lock()
		for n := range c.pwmPorts {
			c.pwmPortsWithUpdates[n] = true
		}
		c.lock.Unlock()

		return nil
	}
	err = resetOrDummyOutServos()
	if err != nil {
		fmt.Println("Failed to select port when initialising servos: ", err)
		screen.SetNotice(NoteMux, screen.LevelErr)
		return
	}

	ticker := time.NewTicker(25 * time.Millisecond)

	var lastL, lastR int8
	var lastPowerReadingTime time.Time

	if initDone != nil {
		initDone.Done()
		initDone = nil
	}

	for ctx.Err() == nil {
		<-ticker.C

		c.lock.Lock()
		tofsEnabled := c.tofsEnabled
		c.lock.Unlock()

		if tofsEnabled {
			readings, err := readTofs()
			if err != nil {
				fmt.Println("Failed to read tofs", err)
				return
			}
			fmt.Println("ToF readings:", readings)
			c.lock.Lock()
			c.distanceReadings = readings
			c.revisionUpdated.Broadcast()
			c.lock.Unlock()
		}

		c.lock.Lock()
		l, r := c.motorL, c.motorR
		c.lock.Unlock()
		// Speeds have changed
		err = mx.SelectSinglePort(mux.BusPropeller)
		if err != nil {
			screen.SetNotice(NoteMux, screen.LevelErr)
			fmt.Println("Failed to update mux port", err)
			return
		}
		if lastL != l || lastR != r {
			err = prop.SetMotorSpeeds(l, r)
			if err != nil {
				fmt.Println("Failed to update motor speeds", err)
				screen.SetNotice(NoteProp, screen.LevelErr)
				return
			}
			lastL, lastR = l, r
			screen.ClearNotice(NoteProp)
		}

		m1, m2, err := prop.GetEncoderPositions()
		if err == nil {
			rightMM := float64(-m2) / motorToMMScaleFactor
			leftMM := float64(-m1) / motorToMMScaleFactor
			fmt.Println("Motor positions: ", m1, "=", leftMM, "mm ", m2, "=", rightMM, "mm")
			c.lock.Lock()
			c.leftMotorDist = leftMM
			c.rightMotorDist = rightMM
			c.lock.Unlock()
			screen.ClearNotice(NoteProp)
			err = prop.StartEncoderRead()
			if err != nil {
				fmt.Println("Failed to start encoder read", err)
				screen.SetNotice(NoteProp, screen.LevelErr)
				return
			}
		} else if err != propeller.ErrNotReady {
			fmt.Println("Failed to read encoders", err)
		}

		if servos == dummyServos && time.Since(lastServoInitTime) > 1*time.Second {
			err = resetOrDummyOutServos()
			if err != nil {
				screen.SetNotice(NoteMux, screen.LevelErr)
				fmt.Println("Failed to select mux port while initialising servos", err)
				return
			}
		}

		err = mx.SelectSinglePort(mux.BusOthers)
		if err != nil {
			screen.SetNotice(NoteMux, screen.LevelErr)
			fmt.Println("Failed to update mux port", err)
			return
		}
		c.lock.Lock()
		pwmUpdatesCopy := c.pwmPortsWithUpdates
		c.pwmPortsWithUpdates = make(map[int]bool)
		c.lock.Unlock()
		for n := range pwmUpdatesCopy {
			c.lock.Lock()
			value := c.pwmPorts[n]
			c.lock.Unlock()

			switch v := value.(type) {
			case servoPosition:
				err = servos.SetServo(n, float64(v))
			case pwmValue:
				err = servos.SetPWM(n, float64(v))
			}
			if err != nil {
				fmt.Println("Failed to update servo/PWM port ", n, ": ", err)
				screen.SetNotice(NoteServo, screen.LevelErr)
				servos = dummyServos
				break
			}
			if servos != dummyServos {
				screen.ClearNotice(NoteServo)
			}
		}

		if time.Since(lastPowerReadingTime) > 1*time.Second {
			for i, ps := range powerSensors {
				bv, err := ps.ReadBusVoltage()
				if err != nil {
					screen.SetNotice(NotePowerMon, screen.LevelErr)
					continue
				}
				bc, err := ps.ReadCurrent()
				if err != nil {
					screen.SetNotice(NotePowerMon, screen.LevelErr)
					continue
				}
				bp, err := ps.ReadPower()
				if err != nil {
					screen.SetNotice(NotePowerMon, screen.LevelErr)
					continue
				}
				fmt.Printf("Bus %v: %.2fV %.2fA %.2fW\n", i, bv, bc, bp)
				screen.ClearNotice(NotePowerMon)
				screen.SetBusVoltage(i, bv)
			}
			lastPowerReadingTime = time.Now()
		}
	}
}
