package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"
)

func main() {
	var listenAddr string
	pflag.StringVar(&listenAddr, "listen-addr", ":10001", "address where the service accepts WebSocket listeners")
	pflag.Parse()

	if !strings.Contains(listenAddr, "://") {
		listenAddr = "ws://" + listenAddr
	}
	wsConn, _, err := websocket.DefaultDialer.DialContext(context.Background(), listenAddr, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open websocket connection: %v\n", err)
		return
	}
	for {
		msgTyp, reader, err := wsConn.NextReader()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get next reader for websocket connection: %v\n", err)
			continue
		}
		switch msgTyp {
		case websocket.BinaryMessage:
			data, err := io.ReadAll(reader)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read record data: %v\n", err)
				continue
			}
			fmt.Fprintf(os.Stdout, "new record: %s\n", data)
		}
	}
}
