package sound

import (
	"fmt"
	"os"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/speaker"
	"github.com/faiface/beep/wav"
)

func InitSound() chan string {
	soundsToPlay := make(chan string)
	go func() {
		defer func() {
			recover()
			for s := range soundsToPlay {
				fmt.Println("Unable to play", s)
			}
		}()
		sampleRate := beep.SampleRate(44100)
		err := speaker.Init(sampleRate, sampleRate.N(time.Second/5))
		if err != nil {
			fmt.Println("Failed to open speaker", err)
			for s := range soundsToPlay {
				fmt.Println("Unable to play", s)
			}
		}
		var ctrl *beep.Ctrl
		var s beep.StreamSeekCloser
		for soundToPlay := range soundsToPlay {
			if ctrl != nil {
				speaker.Lock()
				ctrl.Paused = true
				ctrl.Streamer = nil
				speaker.Unlock()
				ctrl = nil
			}
			if s != nil {
				s.Close()
			}

			f, err := os.Open(soundToPlay)
			if err != nil {
				fmt.Println("Failed to open sound", err)
				continue
			}
			s, _, err = wav.Decode(f)
			if err != nil {
				fmt.Println("Failed to decode sound", err)
				continue
			}
			ctrl := &beep.Ctrl{Streamer: s}
			speaker.Play(ctrl)
		}
	}()
	return soundsToPlay
}
