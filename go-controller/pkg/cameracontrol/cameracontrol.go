package cameracontrol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
)

type CameraControl struct {
	Command []string

	// Internals.
	subProcess *exec.Cmd
	subInput   io.WriteCloser
	subOutput  *bufio.Scanner
}

func New() *CameraControl {
	return &CameraControl{
		Command: []string{"python", "pkg/cameracontrol/sub.py"},
	}
}

func (cc *CameraControl) Start() (err error) {
	cc.subProcess = exec.Command(cc.Command[0], cc.Command[1:]...)
	cc.subInput, err = cc.subProcess.StdinPipe()
	if err != nil {
		return fmt.Errorf("couldn't get StdinPipe for subprocess: %w", err)
	}
	subOutput, err := cc.subProcess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("couldn't get StdoutPipe for subprocess: %w", err)
	}
	cc.subOutput = bufio.NewScanner(subOutput)
	cc.subProcess.Stderr = cc.subProcess.Stdout

	err = cc.subProcess.Start()
	if err != nil {
		return fmt.Errorf("couldn't Start subprocess: %w", err)
	}

	return nil
}

const RESULT_PREFIX = "RESULT: "

func (cc *CameraControl) Execute(req string) (rsp string, err error) {
	log.Println("Send request to subprocess:", req)
	io.WriteString(cc.subInput, req+"\n")

	for {
		if cc.subOutput.Scan() {
			rsp = cc.subOutput.Text()
			log.Println(">>", rsp)
			if strings.HasPrefix(rsp, RESULT_PREFIX) {
				rsp = rsp[len(RESULT_PREFIX):]
				return
			}
		} else {
			rsp = ""
			err = cc.subOutput.Err()
			if err == nil {
				err = errors.New("Subprocess terminated")
			} else {
				err = fmt.Errorf("error from subprocess: %w", err)
			}
			return
		}
	}
}

func (cc *CameraControl) TakePicture() (fileName string, err error) {
	fileName, err = cc.Execute("take-picture")
	return
}
