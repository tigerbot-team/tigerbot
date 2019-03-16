package screen

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"sync"
	"time"

	"github.com/fogleman/gg"
)

var (
	lock sync.Mutex

	busVoltages = make([]float64, 2)
	leds        = make([]color.RGBA, 2)
	mode        string
)

func SetBusVoltage(n int, bv float64) {
	lock.Lock()
	defer lock.Unlock()
	if n < 0 || n >= len(busVoltages) {
		return
	}
	busVoltages[n] = bv
}
func SetBusVoltages(bvs []float64) {
	lock.Lock()
	defer lock.Unlock()
	busVoltages = bvs
}

func SetMode(m string) {
	lock.Lock()
	defer lock.Unlock()
	mode = m
}

func SetLED(n int, r, g, b float64) {
	lock.Lock()
	defer lock.Unlock()
	if n < 0 || n >= len(leds) {
		return
	}
	leds[n] = color.RGBA{
		A: 1,
		R: uint8(r * 255),
		G: uint8(g * 255),
		B: uint8(b * 255),
	}
}

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

		ledsCopy := make([]color.RGBA, 2)
		lock.Lock()
		for i, c := range leds {
			ledsCopy[i] = c
		}
		lock.Unlock()

		for i, c := range ledsCopy {
			dc.SetRGB(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
			dc.DrawRectangle(0, float64(i*(128/len(ledsCopy))), 50, float64(128/len(ledsCopy)))
			dc.Fill()
		}

		yellow(dc)
		dc.Push()
		dc.Translate(60, 5)
		dc.DrawString("CHARGE LVL", 0, 10)

		lock.Lock()
		voltage := busVoltages[0]
		lock.Unlock()
		dc.Translate(0, lineHeight)
		drawPowerBar(dc, voltage)
		lock.Lock()
		voltage = busVoltages[1]
		lock.Unlock()
		dc.Translate(34, 0)
		drawPowerBar(dc, voltage)
		dc.Translate(-34, 0)

		lock.Lock()
		m := mode
		lock.Unlock()
		dc.SetRGBA(0.1, 0.1, 0, 0.2)
		dc.Fill()
		yellow(dc)
		dc.DrawString(m, 0, overallBarHeight+lineHeight*2)

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

func yellow(dc *gg.Context) {
	dc.SetRGBA(1, 0.9, 0, 1)
}

const (
	minCellVoltage = 3
	maxCellVoltage = 4.2
)

const (
	lineHeight       = 12
	gap              = 2
	bottomBarHeight  = 10
	bottomBarWidth   = 30
	upperBarInset    = gap
	upperBarWidth    = bottomBarWidth - (upperBarInset * 2)
	upperBarHeight   = 3
	gapBetweenBars   = gap
	upperBarInterval = gapBetweenBars + upperBarHeight
	numUpperBars     = 10
	overallBarHeight = bottomBarHeight + numUpperBars*upperBarInterval
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
	dc.DrawRectangle(0, overallBarHeight-bottomBarHeight, bottomBarWidth, bottomBarHeight)

	for n := 1; n < numUpperBars; n++ {
		if charge >= (float64(n) / numUpperBars) {
			dc.DrawRectangle(upperBarInset,
				float64(numUpperBars-n)*upperBarInterval, upperBarWidth, upperBarHeight)
		}
	}
	dc.Fill()
	vString := fmt.Sprintf("%.1fv", voltage)
	var offset float64 = 0
	if len(vString) > 4 {
		offset = -2
	}
	dc.DrawString(vString, offset, overallBarHeight+lineHeight)
}

func DrawWarning(dc *gg.Context) {
	dc.SetRGB(1, 0.2, 0)
	dc.DrawRegularPolygon(3, 0, 0, 14, 0)
	dc.Fill()
	dc.SetRGBA(0, 0, 0, 0.9)
	dc.DrawString("!", -3, 3)
}