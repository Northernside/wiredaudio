package receiver

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"os"
	"os/exec"
	"sync"
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

	monitorClients []net.Conn
	monitorLock    sync.Mutex
)

func Start() {
	go discoveryServer()
	go monitorServer()

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

			msg := fmt.Sprintf("%.2f\n", dbfs)
			monitorLock.Lock()
			for _, conn := range monitorClients {
				conn.Write([]byte(msg))
			}
			monitorLock.Unlock()

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

func monitorServer() {
	socketPath := "/tmp/wiredaudio.sock"
	os.Remove(socketPath) //recreate

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatal("monitor socket listen failed:", err)
	}

	log.Println("monitor server on", socketPath)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Println("monitor accept error:", err)
				continue
			}

			monitorLock.Lock()
			monitorClients = append(monitorClients, conn)
			monitorLock.Unlock()

			go func(c net.Conn) {
				defer c.Close()
				log.Println("monitor client connected")

				// keep until broken
				_, _ = io.Copy(io.Discard, c)

				monitorLock.Lock()
				for i, cc := range monitorClients {
					if cc == c {
						monitorClients = append(monitorClients[:i], monitorClients[i+1:]...)
						break
					}
				}
				monitorLock.Unlock()

				log.Println("monitor client disconnected")
			}(conn)
		}
	}()
}
