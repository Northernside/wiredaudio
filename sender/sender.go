package sender

import (
	"encoding/binary"
	"log"
	"net"
	"time"

	"github.com/gordonklaus/portaudio"
)

const (
	discoveryPort   = 37146
	discoveryPrompt = "DISCOVER_RECEIVER"
	discoveryReply  = "RECEIVER_HERE"
	audioPort       = 37145
	sampleRate      = 44100
	framesPerBuffer = 512
)

func Start() {
	receiverAddr, err := discoverReceiver()
	if err != nil {
		log.Fatal("Discovery failed: ", err)
	}

	log.Println("Discovered receiver at:", receiverAddr)

	// initialize audio capture
	portaudio.Initialize()
	defer portaudio.Terminate()

	buffer := make([]int16, framesPerBuffer)
	stream, err := portaudio.OpenDefaultStream(1, 0, sampleRate, len(buffer), buffer)
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	err = stream.Start()
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Stop()

	// set up UDP connection to receiver
	conn, err := net.Dial("udp", receiverAddr.String())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	for {
		// capture a buffer of audio samples
		if err := stream.Read(); err != nil {
			log.Println("Read error:", err)
			continue
		}

		// encode and send audio buffer over UDP
		err = binary.Write(conn, binary.LittleEndian, buffer)
		if err != nil {
			log.Println("UDP send error:", err)
		}
	}
}

func discoverReceiver() (*net.UDPAddr, error) {
	broadcastAddr := &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: discoveryPort,
	}

	// use an ephemeral UDP socket for discovery
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// send discovery prompt to broadcast address
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.WriteToUDP([]byte(discoveryPrompt), broadcastAddr)
	if err != nil {
		return nil, err
	}

	// wait for the first valid discovery reply
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}
		msg := string(buf[:n])
		if msg == discoveryReply {
			// use the sender of the discovery reply as the audio receiver
			return &net.UDPAddr{IP: addr.IP, Port: audioPort}, nil
		}
	}
}
