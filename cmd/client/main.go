package main

import (
	"context"
	"crypto/tls"
	"io"
	"os"
	"strings"

	"github.com/banzaicloud/log-socket/log"
	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"
)

func main() {
	var listenAddr string
	pflag.StringVar(&listenAddr, "listen-addr", ":10001", "address where the service accepts WebSocket listeners")
	pflag.Parse()

	var logs log.Sink = log.NewWriterSink(os.Stdout)

	if !strings.Contains(listenAddr, "://") {
		listenAddr = "wss://" + listenAddr
	}

	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	wsConn, _, err := dialer.DialContext(context.Background(), listenAddr, nil)
	if err != nil {
		log.Event(logs, "failed to open websocket connection", log.Error(err))
		return
	}
	for {
		msgTyp, reader, err := wsConn.NextReader()
		if err != nil {
			log.Event(logs, "failed to get next reader for websocket connection", log.Error(err))
			continue
		}
		switch msgTyp {
		case websocket.BinaryMessage:
			data, err := io.ReadAll(reader)
			if err != nil {
				log.Event(logs, "failed to read record data", log.Error(err))
				continue
			}
			log.Event(logs, "new record", log.Fields{"data": data})
		}
	}
}
