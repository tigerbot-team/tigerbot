package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/screen"
)

func main() {
	ctx := context.Background()

	go screen.LoopUpdatingScreen(ctx)

	screen.SetBusVoltages([]float64{8.4, 16.8})
	screen.SetLED(0, 1, 0, 0)
	screen.SetLED(1, 0, 1, 0)

	reader := bufio.NewReader(os.Stdin)
	errors := make(map[string]bool)
	level := screen.LevelInfo
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("\nFailed to read stdin: ", err)
			return
		}

		if errors[line] {
			screen.ClearNotice(strings.TrimSpace(line))
			delete(errors, line)
		} else {
			screen.SetNotice(strings.TrimSpace(line), level)
			errors[line] = true
		}
		if level == screen.LevelInfo {
			level = screen.LevelErr
		} else {
			level = screen.LevelInfo
		}
	}
}
