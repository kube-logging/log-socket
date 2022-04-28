package main

import (
	"context"
	"crypto/tls"
	"math/big"
	"net"
	"os"
	"sync"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// "k8s.io/apimachinery/pkg/runtime"

	"github.com/banzaicloud/log-socket/internal"
	"github.com/banzaicloud/log-socket/internal/reconciler"
	"github.com/banzaicloud/log-socket/log"
	"github.com/banzaicloud/log-socket/pkg/slice"
	"github.com/banzaicloud/log-socket/pkg/tlstools"
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/logging/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
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
	reconcileEventChannel := make(internal.ReconcileEventChannel)

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

	s := runtime.NewScheme()
	if err := loggingv1beta1.AddToScheme(s); err != nil {
		panic(err)
	}
	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: s})
	if err != nil {
		panic(err)
	}
	rec := reconciler.New(ingestAddr, c)
	authenticator := internal.TokenReviewAuthenticator{Client: c}

	go func() {
		for {
			select {
			case <-stopLatch.Chan():
				goto foo
			case evt := <-reconcileEventChannel:
				res, err := rec.Reconcile(context.Background(), evt)
				log.Event(logs, "reconcile", log.Fields{"res": res, "err": err})
			}
		}
	foo:
	}()

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

		internal.Listen(listenAddr, tlsConfig, listenerReg, logs, stopSignal, nil, authenticator)
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
				if len(listenersToAdd) > 0 || len(listenersToRemove) > 0 {
					reconcileEventChannel <- generateReconcileEvent(listeners)

				}
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

	reconcileEventChannel <- internal.ReconcileEvent{}

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

func generateReconcileEvent(listeners []internal.Listener) internal.ReconcileEvent {
	requestMap := map[internal.FlowReference]bool{}
	for _, l := range listeners {
		requestMap[l.Flow()] = true
	}

	requests := []internal.FlowReference{}

	for k := range requestMap {
		requests = append(requests, k)
	}

	return internal.ReconcileEvent{Requests: requests}
}
