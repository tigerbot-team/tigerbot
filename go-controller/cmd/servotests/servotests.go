package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/mux"
	"github.com/tigerbot-team/tigerbot/go-controller/pkg/pca9685"
)

func main() {
	mx, err := mux.New("/dev/i2c-1")
	if err != nil {
		fmt.Println("Failed to open mux", err)
		return
	}

	err = mx.SelectSinglePort(6)
	if err != nil {
		fmt.Println("Failed to select mux port", err)
		return
	}

	pwmController, err := pca9685.New("/dev/i2c-1")
	if err != nil {
		fmt.Println("Failed to open PCA9685", err)
		return
	}

	err = pwmController.Configure()
	if err != nil {
		fmt.Println("Failed to configure PCA9685", err)
		return
	}

	fmt.Println(
		`Commands:
    s <n> <position>        # Configure port for servo
    p <n> <pwm-duty-cycle>  # Configure port for PWM

<n>               Port number 0-15
<position>        Servo postiion 0.0-1.0; 0.5=centre
<pwm-duty-cycle>  Raw PWM duty cycle 0.0-1.0; 0=fully off, 1.0=fully on\n`)

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("\nFailed to read stdin: ", err)
			return
		}

		parts := strings.Split(line, "")
		switch parts[0] {
		case "s", "p":
			if len(parts) < 3 {
				fmt.Println("Not enough parameters")
				continue
			}
			n, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("Expected int, not ", parts[1])
				continue
			}
			if n < 0 || n > 15 {
				fmt.Println("Expected 0 <= n < 16")
				continue
			}
			v, err := strconv.ParseFloat(parts[1], 64)
			if err != nil {
				fmt.Println("Expected float, not ", parts[2])
				continue
			}
			if v > 1 {
				fmt.Println("Clamping value to 1.0")
				v = 1
			}
			if v < 0 {
				fmt.Println("Clamping value to 0.0")
				v = 0
			}
			if parts[0] == "s" {
				err = pwmController.SetServo(n, v)
			} else {
				err = pwmController.SetPWM(n, v)
			}
			if err != nil {
				fmt.Println("Failed to write to PCA9685: ", err)
				return
			}
		}
	}
}
