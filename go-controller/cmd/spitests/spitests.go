package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/fogleman/gg"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/imu"

	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"
)

var lock sync.Mutex

func main() {
	// Make sure periph is initialized.
	if _, err := host.Init(); err != nil {
		log.Fatal(err)
	}

	refs := spireg.All()
	for _, r := range refs {
		log.Printf("Port ref: %v", r)
	}

	// Use spireg SPI port registry to find the first available SPI bus.
	p, err := spireg.Open("/dev/spidev0.1")
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()

	// Convert the spi.Port into a spi.Conn so it can be used for communication.
	c, err := p.Connect(physic.KiloHertz*1000, spi.Mode3, 8)
	if err != nil {
		log.Fatal(err)
	}

	const W = 0x00
	const R = 0x80

	write := []byte{W | 0x6A, 0x10} // Disable I2C
	read := make([]byte, len(write))
	if err := c.Tx(write, read); err != nil {
		log.Fatal(err)
	}
	// Read it back
	write = []byte{R | 0x6A, 0x00}
	if err := c.Tx(write, read); err != nil {
		log.Fatal(err)
	}
	// Use read.
	log.Printf("Read back: %x\n", read[1:])

	m, err := imu.NewSPI("/dev/spidev0.1")
	if err != nil {
		fmt.Println("Failed to open IMU", err)
		panic("Failed to open IMU")
	}

	err = m.Configure()
	if err != nil {
		fmt.Println("Failed to configure IMU", err)
		panic("Failed to open IMU")
	}
	time.Sleep(1 * time.Second)
	err = m.Calibrate()
	if err != nil {
		fmt.Println("Failed to calibrate IMU", err)
		panic("Failed to open IMU")
	}
	m.ResetFIFO()

	go drawOnScreen()
	go readFIFO(m)

	for range time.NewTicker(time.Millisecond * 200).C {
		headingLock.Lock()
		fmt.Printf("Heading: %.4f Offset: %f Num: %d\n", headingEstimate, offset, numR)
		headingLock.Unlock()
	}
}

var headingLock sync.Mutex
var headingEstimate float64
var offset = 0.0
var numR int

func readFIFO(m imu.Interface) {
	const imuDT = 1 * time.Millisecond

	for range time.NewTicker(time.Millisecond * 20).C {
		lock.Lock()
		yawReadings := m.ReadFIFO()
		lock.Unlock()
		headingLock.Lock()
		numR = len(yawReadings)
		for _, yaw := range yawReadings {
			yawDegreesPerSec := float64(yaw) * m.DegreesPerLSB()
			if math.Abs(yawDegreesPerSec) < 0.1 {
				offset = offset*0.999 + 0.001*yawDegreesPerSec
			}
			headingEstimate -= imuDT.Seconds() * (yawDegreesPerSec - offset)
		}
		headingLock.Unlock()
	}
}

func drawOnScreen() {
	f, err := os.OpenFile("/dev/fb1", os.O_RDWR, 0666)
	if err != nil {
		panic(err)
	}

	charge := 0.0
	for range time.NewTicker(500 * time.Millisecond).C {
		const S = 128
		dc := gg.NewContext(S, S)
		dc.SetRGBA(1, 0.9, 0, 1)
		headingLock.Lock()
		j := headingEstimate
		headingLock.Unlock()
		//for i := 0; i < 360; i += 15 {
		//	dc.Push()
		//	dc.RotateAbout(gg.Radians(float64(i)+j), S/2, S/2)
		//	dc.DrawEllipse(S/2, S/2, S*7/16, S/8)
		//	dc.Fill()
		//	dc.Pop()
		//}

		dc.Push()
		dc.Translate(60, 5)
		dc.DrawString("CHARGE LVL", 0, 10)

		// Draw the larger power bar at the bottom. Colour depends on charge level.
		if charge < 0.1 {
			dc.SetRGBA(1, 0.2, 0, 1)
			dc.Push()
			dc.Translate(14, 80)
			DrawWarnign(dc)
			dc.Pop()
		}

		dc.DrawRectangle(36, 70, 30, 10)

		for n := 2; n < 13; n++ {
			if charge >= (float64(n) / 13) {
				dc.DrawRectangle(38, 75-float64(n)*5, 26, 3)
			}
		}

		dc.Fill()

		dc.DrawString(fmt.Sprintf("%.1fv", 11.4+charge), 33, 93)

		dc.SetRGBA(1, 0.9, 0, 1)

		dc.Translate(14, 30)
		dc.Rotate(gg.Radians(j))
		dc.Scale(0.5, 1.0)
		dc.DrawRegularPolygon(3, 0, 0, 14, 0)
		dc.Fill()

		dc.Pop()

		charge += 0.1
		if charge > 1 {
			charge = 0
		}

		var buf [128 * 128 * 2]byte
		for y := 0; y < S; y++ {
			for x := 0; x < S; x++ {
				c := dc.Image().At(x, y)
				r, g, b, _ := c.RGBA() // 16-bit pre-multiplied

				rb := byte(r >> (16 - 5))
				gb := byte(g >> (16 - 6)) // Green has 6 bits
				bb := byte(b >> (16 - 5))

				buf[(127-y)*2+(x)*128*2+1] = (rb << 3) | (gb >> 3)
				buf[(127-y)*2+(x)*128*2] = bb | (gb << 5)
			}
		}
		_, err = f.Seek(0, 0)
		if err != nil {
			panic(err)
		}

		lock.Lock()
		_, err = f.Write(buf[:])
		lock.Unlock()
		if err != nil {
			panic(err)
		}
	}
}

func DrawWarnign(dc *gg.Context) {
	dc.SetRGB(1, 0.2, 0)
	dc.DrawRegularPolygon(3, 0, 0, 14, 0)
	dc.Fill()
	dc.SetRGBA(0, 0, 0, 0.9)
	dc.DrawString("!", -3, 3)
}
