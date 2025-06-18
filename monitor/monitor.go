package monitor

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const maxBlocks = 13

// ansi 256-color codes for volume meter
var colors = []int{
	22, 34, 76, 82, 148,
	184, 220, 214, 208, 202,
	196, 88, 52,
}

func Start() {
	socket := "/tmp/wiredaudio.sock"
	conn, err := net.Dial("unix", socket)
	if err != nil {
		fmt.Println("connection error:", err)
		os.Exit(1)
	}
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		dbfs, err := strconv.ParseFloat(strings.TrimSpace(scanner.Text()), 64)
		if err != nil {
			continue
		}

		fmt.Fprintf(os.Stdout, "\rVolume: %6.2f dBFS %s\033[K", dbfs, levelBar(dbfs))
	}
}

func levelBar(dbfs float64) string {
	totalBlocks := (dbfs + 60) / 5
	if totalBlocks < 0 {
		totalBlocks = 0
	} else if totalBlocks > float64(maxBlocks) {
		totalBlocks = float64(maxBlocks)
	}

	fullBlocks := int(totalBlocks)
	frac := totalBlocks - float64(fullBlocks)

	var fracBlock string
	switch {
	case frac >= 7.0/8.0:
		fracBlock = "▉"
	case frac >= 3.0/4.0:
		fracBlock = "▊"
	case frac >= 5.0/8.0:
		fracBlock = "▋"
	case frac >= 1.0/2.0:
		fracBlock = "▌"
	case frac >= 3.0/8.0:
		fracBlock = "▍"
	case frac >= 1.0/4.0:
		fracBlock = "▎"
	case frac >= 1.0/8.0:
		fracBlock = "▏"
	default:
		fracBlock = ""
	}

	var sb strings.Builder
	sb.WriteString("[")

	for i := range fullBlocks {
		colorCode := colors[i]
		sb.WriteString(fmt.Sprintf("\x1b[38;5;%dm█", colorCode))
	}

	if fracBlock != "" && fullBlocks < maxBlocks {
		colorCode := colors[fullBlocks]
		sb.WriteString(fmt.Sprintf("\x1b[38;5;%dm%s", colorCode, fracBlock))
		fullBlocks++
	}

	for i := fullBlocks; i < maxBlocks; i++ {
		sb.WriteString(" ")
	}

	sb.WriteString("\x1b[0m]")
	return sb.String()
}
