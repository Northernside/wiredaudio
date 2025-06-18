package receiver

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	maxBlocks       = 13
	discoveryPort   = 37146
	audioPort       = 37145
	discoveryPrompt = "DISCOVER_RECEIVER"
	discoveryReply  = "RECEIVER_HERE"
)

var (
	sampleSquares float64
	sampleCount   int

	// ansi 256-color codes for volume meter
	colors = []int{
		22, 34, 76, 82, 148,
		184, 220, 214, 208, 202,
		196, 88, 52,
	}
)

func Start() {
	go discoveryServer()

	// listen for incoming audio udp packets
	addr := net.UDPAddr{
		Port: audioPort,
		IP:   net.ParseIP("[::]"), // support ipv6 and ipv4-mapped
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// set up pulseaudio raw stream input
	cmd := exec.Command("paplay", "--raw", "--format=s16le", "--rate=44100", "--channels=1", "--device=virtual_mic")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 4096)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			rms := 0.0
			if sampleCount > 0 {
				rms = math.Sqrt(sampleSquares / float64(sampleCount))
			}

			// convert to dbfs for perceptual volume
			maxInt16 := float64(32768)
			dbfs := 20 * math.Log10(rms/maxInt16)
			if dbfs < -100 {
				dbfs = -100 // clamp minimum value
			}

			fmt.Fprintf(os.Stdout, "\rVolume: %6.2f dBFS %s\033[K", dbfs, levelBar(dbfs))

			sampleSquares = 0
			sampleCount = 0
		}
	}()

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Read error:", err)
			continue
		}

		// pipe into audio output
		_, err = stdin.Write(buf[:n])
		if err != nil {
			log.Println("Write to paplay error:", err)
		}

		// accumulating rms = "root mean square" for volume calculation
		// each audio sample is a signed 16-bit value
		// square it to get its power then accumulate the squares and count
		for i := 0; i+1 < n; i += 2 {
			sample := int16(binary.LittleEndian.Uint16(buf[i : i+2]))
			val := float64(sample)
			sampleSquares += val * val
			sampleCount++
		}
	}
}

func discoveryServer() {
	addr := net.UDPAddr{
		Port: discoveryPort,
		IP:   net.ParseIP("[::]"),
	}

	log.Println("starting discovery server on", addr.String())
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatal("discovery listener error:", err)
	}
	defer conn.Close()

	// respond to discovery requests
	buf := make([]byte, 1024)
	for {
		n, senderAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Discovery read error:", err)
			continue
		}

		msg := string(buf[:n])
		if msg == discoveryPrompt {
			log.Printf("Discovery request from %v. accepting.\n", senderAddr)
			_, err = conn.WriteToUDP([]byte(discoveryReply), senderAddr)
			if err != nil {
				log.Println("Discovery reply error:", err)
			}
		}
	}
}

func levelBar(dbfs float64) string {
	// normalize to 0–13 blocks, start at -60 dBFS bc we want to ignore super quiet noise
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

	// fractional block handling (if any)
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
