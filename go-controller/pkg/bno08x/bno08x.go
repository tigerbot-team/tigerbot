package bno08x

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/headingholder/angle"
	"gonum.org/v1/gonum/spatial/r3"
	"io"
	"math"
	"sync"
	"time"

	"go.bug.st/serial"
)

const serialDevice = "/dev/ttyAMA0"

const ReportFrequency = 100
const ReportInterval = time.Second / ReportFrequency

type IMUReport struct {
	Time   time.Time
	Index  uint8
	Yaw    int16
	Pitch  int16
	Roll   int16
	XAccel int16
	YAccel int16
	ZAccel int16
}

var startTime = time.Now()

func (i IMUReport) String() string {
	return fmt.Sprintf("%s [%02x] Y:%7.2f P:%7.2f R:%7.2f X:%7.2f Y:%7.2f Z:%7.2f",
		time.Since(startTime).Round(time.Millisecond), i.Index, float64(i.Yaw)/100.0, float64(i.Pitch)/100.0, float64(i.Roll)/100.0,
		float64(i.XAccel)/100.0, float64(i.YAccel)/100.0, float64(i.ZAccel)/100.0)
}

func (i IMUReport) YawDegrees() float64 {
	return (float64(i.Yaw)) / 100.0
}
func (i IMUReport) PitchDegrees() float64 {
	return (float64(i.Pitch)) / 100.0
}
func (i IMUReport) RollDegrees() float64 {
	return (float64(i.Roll)) / 100.0
}

func (i IMUReport) YawRadians() float64 {
	return i.YawDegrees() * 2 * math.Pi / 360.0
}

func (i IMUReport) PitchRadians() float64 {
	return i.PitchDegrees() * 2 * math.Pi / 360.0
}

func (i IMUReport) RollRadians() float64 {
	return i.RollDegrees() * 2 * math.Pi / 360.0
}

func (i IMUReport) RobotYaw() angle.PlusMinus180 {
	return CalculateRobotYaw(i.YawRadians(), i.PitchRadians(), i.RollRadians())
}

type Interface interface {
	CurrentReport() IMUReport
	WaitForReportAfter(t time.Time) IMUReport
}

type BNO08X struct {
	lock       sync.Mutex
	cond       *sync.Cond
	lastReport IMUReport
}

var _ Interface = (*BNO08X)(nil)

func New() *BNO08X {
	b := &BNO08X{}
	b.cond = sync.NewCond(&b.lock)
	return b
}

func (b *BNO08X) CurrentReport() IMUReport {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.lastReport
}

func (b *BNO08X) WaitForReportAfter(t time.Time) IMUReport {
	b.lock.Lock()
	defer b.lock.Unlock()
	startTime := time.Now()
	for !b.lastReport.Time.After(t) {
		b.cond.Wait()
		if time.Since(startTime) > time.Second {
			panic("IMU hasn't responded for >1s")
		}
	}
	return b.lastReport
}

func (b *BNO08X) LoopReadingReports(ctx context.Context) {
	defer b.cond.Broadcast()
	for ctx.Err() == nil {
		err := b.openAndLoop(ctx)
		if ctx.Err() != nil {
			return
		}
		fmt.Println("BNO08X loop stopped; will retry", err)
		time.Sleep(100 * time.Millisecond)
		b.cond.Broadcast()
	}
}

func (b *BNO08X) openAndLoop(ctx context.Context) interface{} {
	mode := &serial.Mode{
		BaudRate: 115200,
	}
	s, err := serial.Open(serialDevice, mode)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %w", serialDevice, err)
	}
	defer func() {
		_ = s.Close()
		fmt.Println("Closed serial port.")
	}()

	br := bufio.NewReader(s)
resync:
	fmt.Println("BNO08X Resync...")
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		buf, err := br.Peek(2)
		if err != nil {
			return fmt.Errorf("failed to read from serial: %w", err)
		}
		if bytes.Equal(buf, []byte{0xaa, 0xaa}) {
			break
		}
		_, err = br.Discard(1)
		if err != nil {
			return fmt.Errorf("failed to read from serial: %w", err)
		}
	}
	fmt.Println("BNO08X: In sync with packet stream.")

	const packetLen = 19
	buf := make([]byte, packetLen)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		_, err := io.ReadAtLeast(br, buf, packetLen)
		if err != nil {
			return fmt.Errorf("failed to read from serial: %w", err)
		}
		if !bytes.Equal(buf[:2], []byte{0xaa, 0xaa}) {
			fmt.Println("BNO08X: Lost sync?!")
			goto resync
		}
		//fmt.Printf("Packet: %x\n", buf)
		var checksum uint8
		for _, b := range buf[2 : packetLen-1] {
			checksum += b
		}
		if buf[18] != checksum {
			fmt.Printf("BNO08X:  BAD CHECKSUM %x != %x\n", buf[18], checksum)
			goto resync
		}
		var report IMUReport
		report.Time = time.Now()
		report.Index = buf[2]
		report.Yaw = int16(binary.LittleEndian.Uint16(buf[3:5]))
		report.Pitch = int16(binary.LittleEndian.Uint16(buf[5:7]))
		report.Roll = int16(binary.LittleEndian.Uint16(buf[7:9]))
		report.XAccel = int16(binary.LittleEndian.Uint16(buf[9:11]))
		report.YAccel = int16(binary.LittleEndian.Uint16(buf[11:13]))
		report.ZAccel = int16(binary.LittleEndian.Uint16(buf[13:15]))
		b.setReport(report)
	}
}

func (b *BNO08X) setReport(report IMUReport) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.lastReport = report
	b.cond.Broadcast()
}

// CalculateRobotYaw calculates the robot's rotation around an axis perpendicular to the floor.  I.e.
// it does a coordinate transform from the IMU's yaw, pitch, roll according to how it is mounted on the robot.
func CalculateRobotYaw(yaw float64, pitch float64, roll float64) angle.PlusMinus180 {
	// Sensor gives us yaw, pitch, roll.  These need to be applied in that order.

	// Rotate the axes yaw radians around Z.
	x0 := r3.Vec{X: 1}
	z0 := r3.Vec{Z: 1}
	y0 := r3.Vec{Y: 1}

	x1 := r3.Rotate(x0, yaw, z0)
	y1 := r3.Rotate(y0, yaw, z0)
	z1 := z0

	// Rotate pitch radians around the *new* Y.
	x2 := r3.Rotate(x1, pitch, y1)
	//y2 := y1
	z2 := r3.Rotate(z1, pitch, y1)

	// Rotate roll radians around the *new* X.
	//x3 := x2
	//y3 := r3.Rotate(y2, roll,  x2)
	z3 := r3.Rotate(z2, roll, x2)

	// Take the x and y components of the final Z vector.  The Z vector points towards the
	// front of the robot.
	return angle.FromFloat(math.Atan2(z3.X, z3.Y) * 360 / (2 * math.Pi))
}
