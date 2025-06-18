package main

import (
	"fmt"
	"os"
	"wiredaudio/monitor"
	"wiredaudio/receiver"
	"wiredaudio/sender"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Printf("Usage: %s <command>\n\nCommands:\n", os.Args[0])
		fmt.Println("  receive")
		fmt.Println("  send")
		fmt.Println("  monitor")
		return
	}

	switch args[0] {
	case "receive":
		receiver.Start()
	case "send":
		sender.Start()
	case "monitor":
		monitor.Start()
	default:
		fmt.Printf("Unknown command: %s\n", args[0])
		fmt.Println("Available commands: receive, send")
		return
	}
}
