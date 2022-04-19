package main

import (
	"context"
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

	var logger log.Target = log.NewWriterTarget(os.Stdout)

	if !strings.Contains(listenAddr, "://") {
		listenAddr = "ws://" + listenAddr
	}
	wsConn, _, err := websocket.DefaultDialer.DialContext(context.Background(), listenAddr, nil)
	if err != nil {
		log.Event(logger, "failed to open websocket connection", log.Error(err))
		return
	}
	for {
		msgTyp, reader, err := wsConn.NextReader()
		if err != nil {
			log.Event(logger, "failed to get next reader for websocket connection", log.Error(err))
			continue
		}
		switch msgTyp {
		case websocket.BinaryMessage:
			data, err := io.ReadAll(reader)
			if err != nil {
				log.Event(logger, "failed to read record data", log.Error(err))
				continue
			}
			log.Event(logger, "new record", log.Fields{"data": data})
		}
	}
}
