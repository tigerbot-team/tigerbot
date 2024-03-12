package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"go.bug.st/serial"
)

type IMUReport struct {
	Index  uint8
	Yaw    int16
	Pitch  int16
	Roll   int16
	XAccel int16
	YAccel int16
	ZAccel int16
}

func main() {
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	s, err := serial.Open("/dev/ttyAMA0", mode)
	if err != nil {
		panic(err)
	}

	br := bufio.NewReader(s)
resync:
	fmt.Printf("Resync...")
	for {
		fmt.Printf(".")
		buf, err := br.Peek(2)
		if err != nil {
			panic(err)
		}
		if bytes.Equal(buf, []byte{0xaa, 0xaa}) {
			break
		}
		_, _ = br.Discard(1)
	}
	fmt.Printf("\nGot packet start.")

	const packetLen = 19
	buf := make([]byte, packetLen)
	for {
		_, err := io.ReadAtLeast(br, buf, packetLen)
		if err != nil {
			panic(err)
		}
		if !bytes.Equal(buf[:2], []byte{0xaa, 0xaa}) {
			fmt.Println("Lost sync.")
			goto resync
		}
		fmt.Printf("Packet: %x\n", buf)
		var checksum uint8
		for _, b := range buf[2 : packetLen-1] {
			checksum += b
		}
		if buf[18] != checksum {
			fmt.Printf("  BAD CHECKSUM %x != %x\n", buf[18], checksum)
		}
		var report IMUReport
		report.Index = buf[2]
		report.Yaw = int16(binary.LittleEndian.Uint16(buf[3:5]))
		report.Pitch = int16(binary.LittleEndian.Uint16(buf[5:7]))
		report.Roll = int16(binary.LittleEndian.Uint16(buf[7:9]))
		report.XAccel = int16(binary.LittleEndian.Uint16(buf[9:11]))
		report.YAccel = int16(binary.LittleEndian.Uint16(buf[11:13]))
		report.ZAccel = int16(binary.LittleEndian.Uint16(buf[13:15]))
		fmt.Printf("Report: %+v\n", report)
	}
}
