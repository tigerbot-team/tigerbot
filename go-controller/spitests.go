package main

import (
	"fmt"
	"log"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/imu"

	"periph.io/x/periph/conn/physic"
	"periph.io/x/periph/conn/spi"
	"periph.io/x/periph/conn/spi/spireg"
	"periph.io/x/periph/host"
)

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
	err = m.Calibrate()
	if err != nil {
		fmt.Println("Failed to calibrate IMU", err)
		panic("Failed to open IMU")
	}
	m.ResetFIFO()

	for range time.NewTicker(time.Millisecond * 200).C {
		log.Println("FIFO:", m.ReadFIFO())
	}
}
