package screen

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fogleman/gg"
)

var (
	Lock sync.Mutex

	BusVoltages = make([]float64, 2)
)

func LoopUpdatingScreen(ctx context.Context) {
	f, err := os.OpenFile("/dev/fb1", os.O_RDWR, 0666)
	if err != nil {
		fmt.Println("Failed to open screen, ignoring")
		return
	}

	for range time.NewTicker(500 * time.Millisecond).C {
		if ctx.Err() != nil {
			var buf [128 * 128 * 2]byte
			_, _ = f.Seek(0, 0)
			_, _ = f.Write(buf[:])
			return
		}
		const S = 128
		dc := gg.NewContext(S, S)
		dc.SetRGBA(1, 0.9, 0, 1)
		//headingLock.Lock()
		//j := headingEstimate
		//headingLock.Unlock()
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

		Lock.Lock()
		voltage := BusVoltages[0]
		Lock.Unlock()
		drawPowerBar(dc, voltage)
		Lock.Lock()
		voltage = BusVoltages[1]
		Lock.Unlock()
		dc.Translate(34, 0)
		drawPowerBar(dc, voltage)
		dc.Translate(-34, 0)

		//dc.SetRGBA(1, 0.9, 0, 1)
		//
		//dc.Translate(14, 30)
		//dc.Rotate(gg.Radians(1))
		//dc.Scale(0.5, 1.0)
		//dc.DrawRegularPolygon(3, 0, 0, 14, 0)
		//dc.Fill()
		//
		//dc.Pop()

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
			fmt.Println("Screen failure: ", err)
			return
		}

		for i := 0; i < 128; i++ {
			_, err = f.Write(buf[i*256 : i*256+256])
			if err != nil {
				fmt.Println("Screen failure: ", err)
				return
			}
			time.Sleep(10 * time.Microsecond)
		}
	}
}

const (
	minCellVoltage = 3
	maxCellVoltage = 4.2
)

func drawPowerBar(dc *gg.Context, voltage float64) {
	var cellVoltage float64
	if voltage > 9 {
		// assume the 4-cell pack
		cellVoltage = voltage / 4
	} else {
		// assume the 2-cell pack
		cellVoltage = voltage / 2
	}
	charge := (cellVoltage - minCellVoltage) / (maxCellVoltage - minCellVoltage)

	// Draw the larger power bar at the bottom. Colour depends on charge level.
	if charge < 0.1 {
		dc.SetRGBA(1, 0.2, 0, 1)
	}
	dc.DrawRectangle(0, 70, 30, 10)
	for n := 2; n < 13; n++ {
		if charge >= (float64(n) / 13) {
			dc.DrawRectangle(2, 75-float64(n)*5, 26, 3)
		}
	}
	dc.Fill()
	dc.DrawString(fmt.Sprintf("%.1fv", voltage), -2, 93)
}

func DrawWarning(dc *gg.Context) {
	dc.SetRGB(1, 0.2, 0)
	dc.DrawRegularPolygon(3, 0, 0, 14, 0)
	dc.Fill()
	dc.SetRGBA(0, 0, 0, 0.9)
	dc.DrawString("!", -3, 3)
}
