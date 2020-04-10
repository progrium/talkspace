package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/speaker"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type gorillaWSAdapter struct {
	sync.Mutex
	*websocket.Conn
}

func (ws *gorillaWSAdapter) Read(p []byte) (int, error) {
	_, msg, err := ws.Conn.ReadMessage()
	return copy(p, msg), err
}

func (ws *gorillaWSAdapter) Write(p []byte) (int, error) {
	ws.Lock()
	defer ws.Unlock()
	return len(p), ws.Conn.WriteMessage(websocket.TextMessage, p)
}

func main() {
	sr := beep.SampleRate(48000)
	format := beep.Format{
		SampleRate:  sr,
		NumChannels: 1,
		Precision:   1,
	}
	chBufs := make(chan bufferStreamer, 1024)
	go func() {
		speaker.Init(sr, sr.N(time.Second/10))
		for buf := range chBufs {
			speaker.Play(&effects.Volume{
				Streamer: &buf,
				Base:     2,
				Volume:   -2,
			})
		}
	}()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Println(err)
				return
			}
			wc := &gorillaWSAdapter{Conn: conn}
			for {
				chunk := make([]byte, 1024)
				_, err := wc.Read(chunk)
				if err != nil {
					log.Println(err)
					return
				}
				chBufs <- bufferStreamer{
					f:    format,
					data: chunk,
				}
			}
		}
		http.ServeFile(w, r, "index.html")
	})
	log.Println("listening on 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

type bufferStreamer struct {
	f    beep.Format
	data []byte
	pos  int
}

func (bs *bufferStreamer) Stream(samples [][2]float64) (n int, ok bool) {
	if bs.pos >= len(bs.data) {
		return 0, false
	}
	for i := range samples {
		if bs.pos >= len(bs.data) {
			break
		}
		sample, advance := bs.f.DecodeSigned(bs.data[bs.pos:])
		samples[i] = sample
		bs.pos += advance
		n++
	}
	return n, true
}

func (bs *bufferStreamer) Err() error {
	return nil
}

func (bs *bufferStreamer) Len() int {
	return len(bs.data) / bs.f.Width()
}

func (bs *bufferStreamer) Position() int {
	return bs.pos / bs.f.Width()
}

func (bs *bufferStreamer) Seek(p int) error {
	if p < 0 || bs.Len() < p {
		return fmt.Errorf("buffer: seek position %v out of range [%v, %v]", p, 0, bs.Len())
	}
	bs.pos = p * bs.f.Width()
	return nil
}
