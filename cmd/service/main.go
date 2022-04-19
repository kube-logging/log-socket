package main

import (
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"

	"github.com/banzaicloud/log-socket/log"
	"github.com/banzaicloud/log-socket/pkg/concurrent"
)

func main() {
	var ingestAddr string
	var listenAddr string
	pflag.StringVar(&ingestAddr, "ingest-addr", ":10000", "address where the service ingests logs")
	pflag.StringVar(&listenAddr, "listen-addr", ":10001", "address where the service accepts WebSocket listeners")
	pflag.Parse()

	var logger log.Target = log.NewWriterTarget(os.Stdout)

	type record struct {
		Data []byte
	}
	recordCh := make(chan record)

	var listeners concurrent.Slice[listener]

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		defer close(recordCh)

		http.ListenAndServe(ingestAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusInternalServerError)
				return
			}
			if err := r.Body.Close(); err != nil {
				log.Event(logger, "failed to close request body")
			}
			rec := record{
				// TODO: extract auth data from request
				Data: data,
			}
			log.Event(logger, "got log record via HTTP", log.V(1), log.Fields{"record": rec})
			recordCh <- rec
			w.WriteHeader(http.StatusOK)
		}))
	}()
	go func() {
		defer wg.Done()

		upgrader := websocket.Upgrader{}

		http.ListenAndServe(listenAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wsConn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Event(logger, "failed to upgrade connection", log.Error(err))
				return
			}

			l := listener{
				Conn: wsConn,
				// TODO: add auth info
			}
			listeners.Append(l)
			wsConn.SetCloseHandler(func(code int, text string) error {
				log.Event(logger, "websocket connection closing", log.Fields{"code": code, "text": text})
				concurrent.RemoveItemsFromSlice(&listeners, l)
				return nil
			})
		}))
	}()
	go func() {
		defer wg.Done()

		for r := range recordCh {
			log.Event(logger, "forwarding record", log.V(1), log.Fields{"record": r})

			if listeners.Len() == 0 {
				log.Event(logger, "no listeners, discarding record", log.V(1), log.Fields{"record": r})
				continue
			}
			var listenersToRemove []listener
			listeners.ForEachWithIndex(func(i int, l listener) {
				logger := log.WithFields(logger, log.Fields{"listener": l})

				// TODO: check auth

				wc, err := l.Conn.NextWriter(websocket.BinaryMessage)
				if err != nil {
					log.Event(logger, "failed to get next writer for websocket connection", log.Error(err))
					listenersToRemove = append(listenersToRemove, l)
					return
				}
				if _, err := wc.Write(r.Data); err != nil {
					log.Event(logger, "failed to write record data to websocket connection", log.Error(err))
					listenersToRemove = append(listenersToRemove, l)
					return
				}
				if err := wc.Close(); err != nil {
					log.Event(logger, "failed to flush data to websocket connection", log.Error(err))
					listenersToRemove = append(listenersToRemove, l)
					return
				}
			})
			concurrent.RemoveItemsFromSlice(&listeners, listenersToRemove...)
		}
	}()

	wg.Wait()
}

type listener struct {
	Conn *websocket.Conn
}

func (l listener) Equals(o listener) bool {
	return l.Conn == o.Conn
}
