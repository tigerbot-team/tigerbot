package hardware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/pca9685"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/screen"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/ina219"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/picobldc"
)

const (
	NumServoPorts = 16

	NotePico     = "PICO"
	NoteTOFs     = "DISTANCE"
	NoteServo    = "SERVO"
	NotePowerMon = "PWR MON"

	motorToMMScaleFactor = 7.384
)

type I2CController struct {
	lock sync.Mutex

	// Desired values.  Stored off in case we need to re-initialise the hardware.
	motorFL, motorFR, motorBL, motorBR int16
	pwmPorts                           map[int]pwmTypes // Either servoPosition or pwmValue
	pwmPortsWithUpdates                map[int]bool

	prop        picobldc.Interface
	tofsEnabled bool

	revisionUpdated               *sync.Cond
	nextRevision                  revision
	distanceReadings              DistanceReadings
	leftMotorDist, rightMotorDist float64
	accumulatedRotations          picobldc.PerMotorVal[float64]
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
	defer c.lock.Unlock()
	c.tofsEnabled = enabled
}

func (c *I2CController) SetMotorSpeeds(frontLeft, frontRight, backLeft, backRight int16) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.motorFR = frontRight
	c.motorFL = frontLeft
	c.motorBL = backLeft
	c.motorBR = backRight
	return nil
}

func (c *I2CController) SetServo(n int, value float64) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.pwmPorts[n] = servoPosition(value)
	c.pwmPortsWithUpdates[n] = true
}

func (c *I2CController) SetPWM(n int, value float64) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.pwmPorts[n] = pwmValue(value)
	c.pwmPortsWithUpdates[n] = true
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
func (c *I2CController) AccumulatedRotations() picobldc.PerMotorVal[float64] {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.accumulatedRotations
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

	pico, err := picobldc.New()
	if err != nil {
		fmt.Println("Failed to open Pico", err)
		screen.SetNotice(NotePico, screen.LevelErr)
		return
	}
	defer func() {
		_ = pico.Close()
	}()

	distanceTracker := picobldc.NewDistanceTracker(pico)

	// Only one sensor on the main bus, Pico also has one as a peripheral.
	var powerSensors []powerMonitor
	var busCells = []int{4, 4}
	for _, addr := range []int{ina219.Addr1} {
		pwrSen, err := ina219.NewI2C("/dev/i2c-1", addr)
		if err != nil {
			fmt.Println("Failed to open power sensor; ignoring! ", err)
			continue
		}
		const shuntOhms = 0.05
		const maxCurrentAmps = 5.0
		err = pwrSen.Configure(shuntOhms, maxCurrentAmps)
		if err != nil {
			fmt.Println("Failed to open power sensor; ignoring! ", err)
			continue
		}
		powerSensors = append(powerSensors, pwrSen)
	}
	powerSensors = append(powerSensors, pico)

	dummyServos := pca9685.Dummy()
	var servos = dummyServos
	defer func() {
		_ = servos.Close()
	}()

	var lastServoInitTime time.Time

	resetOrDummyOutServos := func() {
		fmt.Println("Resetting servos...")
		lastServoInitTime = time.Now()
		if servos != dummyServos {
			fmt.Println("Closing old servo controller.")
			_ = servos.Close()
			servos = dummyServos
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
	}
	resetOrDummyOutServos()

	ticker := time.NewTicker(25 * time.Millisecond)

	var lastFL, lastFR, lastBL, lastBR int16
	var lastPowerReadingTime time.Time
	var lastMotorUpdTime time.Time

	// Enable Pico watchdog just before we start the loop.
	const picoWatchdogTimeout = time.Second
	if err := pico.SetWatchdog(picoWatchdogTimeout); err != nil {
		fmt.Println("Failed to configure Pico watchdog", err)
		screen.SetNotice(NotePico, screen.LevelErr)
		return
	}
	fmt.Println("Pico watchdog enabled.")

	if initDone != nil {
		initDone.Done()
		initDone = nil
	}

	for ctx.Err() == nil {
		<-ticker.C

		c.lock.Lock()
		fl, fr, bl, br := c.motorFL, c.motorFR, c.motorBL, c.motorBR
		c.lock.Unlock()

		speedsChanged := fl != lastFL || fr != lastFR || bl != lastBL || br != lastBR
		needToPetWatchdog := time.Since(lastMotorUpdTime) > (picoWatchdogTimeout / 10)
		if speedsChanged || needToPetWatchdog {
			err = pico.SetMotorSpeeds(fl, fr, bl, br)
			if err != nil {
				fmt.Println("Failed to update motor speeds", err)
				screen.SetNotice(NotePico, screen.LevelErr)
				return
			}
			lastFL, lastFR, lastBL, lastBR = fl, fr, bl, br
			screen.ClearNotice(NotePico)
			lastMotorUpdTime = time.Now()
		}

		if err := distanceTracker.Poll(); err != nil {
			fmt.Println("Failed to poll motor distances, will retry on next loop", err)
			screen.SetNotice(NotePico, screen.LevelErr)
		} else {
			acc := distanceTracker.AccumulatedRotations()
			c.lock.Lock()
			c.accumulatedRotations = acc
			c.lock.Unlock()
		}

		if servos == dummyServos && time.Since(lastServoInitTime) > 1*time.Second {
			resetOrDummyOutServos()
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

		if time.Since(lastPowerReadingTime) > 5*time.Second {
			for i, ps := range powerSensors {
				bv, err := ps.BusVoltage()
				if err != nil {
					screen.SetNotice(NotePowerMon, screen.LevelErr)
					continue
				}
				bc, err := ps.CurrentAmps()
				if err != nil {
					screen.SetNotice(NotePowerMon, screen.LevelErr)
					continue
				}
				bp, err := ps.PowerWatts()
				if err != nil {
					screen.SetNotice(NotePowerMon, screen.LevelErr)
					continue
				}
				name := "Pi"
				if i > 0 {
					name = "Traction"
				}
				fmt.Printf("%v bus: %.2fV %.2fA %.2fW ", name, bv, bc, bp)
				screen.ClearNotice(NotePowerMon)
				screen.SetBusVoltage(i, bv, busCells[i])
			}
			fmt.Println()
			lastPowerReadingTime = time.Now()
		}
	}
}

type powerMonitor interface {
	BusVoltage() (float64, error)
	CurrentAmps() (float64, error)
	PowerWatts() (float64, error)
}
