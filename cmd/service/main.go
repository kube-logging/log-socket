package main

import (
	"crypto/tls"
	"math/big"
	"net"
	"os"
	"sync"

	"github.com/spf13/pflag"

	"github.com/banzaicloud/log-socket/internal"
	"github.com/banzaicloud/log-socket/log"
	"github.com/banzaicloud/log-socket/pkg/slice"
	"github.com/banzaicloud/log-socket/pkg/tlstools"
)

func main() {
	var ingestAddr string
	var listenAddr string
	pflag.StringVar(&ingestAddr, "ingest-addr", ":10000", "address where the service ingests logs")
	pflag.StringVar(&listenAddr, "listen-addr", ":10001", "address where the service accepts WebSocket listeners")
	pflag.Parse()

	var logs log.Sink = log.NewWriterSink(os.Stdout)

	records := make(internal.RecordsChannel)
	listenerReg := make(internal.ListenerEventChannel)

	caCert, caKey, err := tlstools.GenerateSelfSignedCA()
	if err != nil {
		log.Event(logs, "failed to generate self-signed CA", log.Error(err))
		return
	}

	tlsCert, err := tlstools.GenerateTLSCert(caCert, caKey, big.NewInt(1), []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::")})
	if err != nil {
		log.Event(logs, "failed to generate TLS certificate with self-signed CA", log.Error(err))
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{
			tlsCert,
		},
	}

	stopLatch := internal.NewWaitableLatch()
	stopSignal := internal.NewHandleableLatch(stopLatch.Chan())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stopLatch.Close()

		internal.Ingest(ingestAddr, records, logs, stopSignal, nil)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stopLatch.Close()

		internal.Listen(listenAddr, tlsConfig, listenerReg, logs, stopSignal, nil)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stopLatch.Close()

		var listeners []internal.Listener

	loop:
		for {
			select {
			case <-stopLatch.Chan():
				break loop
			case ev := <-listenerReg:
				listenersToAdd, listenersToRemove := gatherListenerEvents(ev, listenerReg)
				if len(listenersToRemove) > 0 {
					slice.RemoveFunc(&listeners, func(item internal.Listener) bool {
						for _, l := range listenersToRemove {
							if l == item {
								return true
							}
						}
						return false
					})
				}
				listeners = append(listeners, listenersToAdd...)
			case r, ok := <-records:
				if !ok {
					log.Event(logs, "records channel closed", log.V(1))
					break loop
				}

				log.Event(logs, "forwarding record", log.V(1), log.Fields{"record": r})

				if len(listeners) == 0 {
					log.Event(logs, "no listeners, discarding record", log.V(1), log.Fields{"record": r})
					continue loop
				}

				for _, l := range listeners {
					l.Send(r)
				}
			}
		}
	}()

	wg.Wait()
}

func gatherListenerEvents(ev internal.ListenerEvent, ch <-chan internal.ListenerEvent) (listenersToAdd []internal.Listener, listenersToRemove []internal.Listener) {
start:
	switch ev.EventType {
	case internal.RegisterListener:
		listenersToAdd = append(listenersToAdd, ev.Listener)
	case internal.UnregisterListener:
		listenersToRemove = append(listenersToRemove, ev.Listener)
	}
	select {
	case ev = <-ch:
		goto start
	default:
		return
	}
}
