package nebulaphotomode

import (
	"context"
	"io/ioutil"
	"sync"

	"fmt"
	"sync/atomic"
	"time"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/hardware"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/joystick"
	"gocv.io/x/gocv"
	yaml "gopkg.in/yaml.v2"
)

type NebulaConfig struct {
	Sequence []string
}

type NebulaPhotoMode struct {
	hw hardware.Interface

	cancel         context.CancelFunc
	startWG        sync.WaitGroup
	stopWG         sync.WaitGroup
	joystickEvents chan *joystick.Event

	running        bool
	cancelSequence context.CancelFunc
	sequenceWG     sync.WaitGroup

	paused int32

	pictureIndex int
	savePicture  int32

	// Config
	config NebulaConfig
}

func New(hw hardware.Interface) *NebulaPhotoMode {
	m := &NebulaPhotoMode{
		hw:             hw,
		joystickEvents: make(chan *joystick.Event),
		config: NebulaConfig{
			Sequence: []string{"red", "blue", "yellow", "green"},
		},
	}
	cfg, err := ioutil.ReadFile("/cfg/nebulaphoto.yaml")
	if err != nil {
		fmt.Println(err)
	} else {
		err = yaml.Unmarshal(cfg, &m.config)
		if err != nil {
			fmt.Println(err)
		}
	}
	// Write out the config that we are using.
	fmt.Printf("NEBULAPHOTO: Using config: %#v\n", m.config)
	cfgBytes, err := yaml.Marshal(&m.config)
	//fmt.Printf("NEBULAPHOTO: Marshalled: %#v\n", cfgBytes)
	if err != nil {
		fmt.Println(err)
	} else {
		err = ioutil.WriteFile("/cfg/nebulaphoto-in-use.yaml", cfgBytes, 0666)
		if err != nil {
			fmt.Println(err)
		}
	}
	return m
}

func (m *NebulaPhotoMode) Name() string {
	return "NEBULAPHOTO MODE"
}

func (m *NebulaPhotoMode) StartupSound() string {
	return "/sounds/nebulaphotomode.wav"
}

func (m *NebulaPhotoMode) Start(ctx context.Context) {
	m.stopWG.Add(1)
	var loopCtx context.Context
	loopCtx, m.cancel = context.WithCancel(ctx)
	go m.loop(loopCtx)
}

func (m *NebulaPhotoMode) Stop() {
	m.cancel()
	m.stopWG.Wait()
}

func (m *NebulaPhotoMode) loop(ctx context.Context) {
	defer m.stopWG.Done()
	defer m.stopSequence()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-m.joystickEvents:
			switch event.Type {
			case joystick.EventTypeButton:
				if event.Value == 1 {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("NEBULAPHOTO: Getting ready!")
						m.startWG.Add(1)
						m.startSequence()
					case joystick.ButtonSquare:
						m.stopSequence()
					case joystick.ButtonTriangle:
						m.pauseOrResumeSequence()
					case joystick.ButtonCircle:
						atomic.StoreInt32(&m.savePicture, 1)
					}
				} else {
					switch event.Number {
					case joystick.ButtonR1:
						fmt.Println("NEBULAPHOTO: GO!")
						m.startWG.Done()
					}
				}
			}
		}
	}
}

func (m *NebulaPhotoMode) startSequence() {
	if m.running {
		fmt.Println("NEBULAPHOTO: Already running")
		return
	}

	fmt.Println("NEBULAPHOTO: Starting sequence...")
	m.running = true
	atomic.StoreInt32(&m.paused, 0)

	seqCtx, cancel := context.WithCancel(context.Background())
	m.cancelSequence = cancel
	m.sequenceWG.Add(1)
	go m.runSequence(seqCtx)
}

func (m *NebulaPhotoMode) takePicture() (err error) {
	webcam, werr := gocv.VideoCaptureDevice(0)
	if werr != nil {
		err = fmt.Errorf("error opening video capture device: %v", werr)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()
	if ok := webcam.Read(img); !ok {
		err = fmt.Errorf("cannot read picture from webcam device")
		return
	}
	if img.Empty() {
		err = fmt.Errorf("no image on device")
		return
	}
	fmt.Printf("NEBULAPHOTO: Read image %v x %v\n", img.Cols(), img.Rows())
	m.pictureIndex++
	saveFile := fmt.Sprintf("/tmp/nebula-image-%v.jpg", m.pictureIndex)
	success := gocv.IMWrite(saveFile, img)
	fmt.Printf("NEBULAPHOTO: wrote %v? %v\n", saveFile, success)
	return
}

func (m *NebulaPhotoMode) fatal(err error) {
	// Placeholder for what to do if we hit a fatal error.
	// Callers assume that this does not return normally.
	panic(err)
}

// runSequence is a goroutine that reads from the camera and controls the motors.
func (m *NebulaPhotoMode) runSequence(ctx context.Context) {
	defer m.sequenceWG.Done()
	defer fmt.Println("NEBULAPHOTO: Exiting sequence loop")
	defer m.hw.StopMotorControl()

	// Let the user know that we're ready, then wait for the "GO" signal.
	m.hw.PlaySound("/sounds/ready.wav")
	m.startWG.Wait()

	for ii := range m.config.Sequence {

		fmt.Println("NEBULAPHOTO: Next target ball: ", m.config.Sequence[ii])
		m.announceTargetBall(ii)

		for {
			time.Sleep(1 * time.Second)

			if atomic.LoadInt32(&m.savePicture) == 1 {
				err := m.takePicture()
				if err != nil {
					m.fatal(err)
				}
				atomic.StoreInt32(&m.savePicture, 0)
				break
			}
		}
	}
}

func (m *NebulaPhotoMode) announceTargetBall(ii int) {
	m.hw.PlaySound(
		fmt.Sprintf("/sounds/%vball.wav", m.config.Sequence[ii]),
	)
}

func (m *NebulaPhotoMode) stopSequence() {
	if !m.running {
		fmt.Println("NEBULAPHOTO: Not running")
		return
	}
	fmt.Println("NEBULAPHOTO: Stopping sequence...")

	m.cancelSequence()
	m.cancelSequence = nil
	m.sequenceWG.Wait()
	m.running = false
	atomic.StoreInt32(&m.paused, 0)

	m.hw.StopMotorControl()

	fmt.Println("NEBULAPHOTO: Stopped sequence...")
}

func (m *NebulaPhotoMode) pauseOrResumeSequence() {
	if atomic.LoadInt32(&m.paused) == 1 {
		fmt.Println("NEBULAPHOTO: Resuming sequence...")
		atomic.StoreInt32(&m.paused, 0)
	} else {
		fmt.Println("NEBULAPHOTO: Pausing sequence...")
		atomic.StoreInt32(&m.paused, 1)
	}
}

func (m *NebulaPhotoMode) OnJoystickEvent(event *joystick.Event) {
	m.joystickEvents <- event
}
