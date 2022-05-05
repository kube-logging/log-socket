package main

import (
	"context"
	"crypto/tls"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/spf13/pflag"
	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	var serviceAddr string
	var verbosity int
	pflag.StringVar(&ingestAddr, "ingest-addr", ":10000", "local address where the service ingests logs")
	pflag.StringVar(&serviceAddr, "service-addr", "log-socket.default.svc:10000", "remote address where the service ingests logs")
	pflag.StringVar(&listenAddr, "listen-addr", ":10001", "address where the service accepts WebSocket listeners")
	pflag.IntVarP(&verbosity, "verbosity", "v", verbosity, "log verbosity level")
	pflag.Parse()

	var logs log.Sink = log.WithVerbosityFilter(log.NewWriterSink(os.Stdout), verbosity)

	metrics := internal.NewMetrics(logs)

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
		log.Event(logs, "an error occurred while adding API group to scheme", log.Error(err), log.Fields{"group": loggingv1beta1.GroupVersion, "scheme": s})
		return
	}
	if err := authv1.AddToScheme(s); err != nil {
		log.Event(logs, "an error occurred while adding API group to scheme", log.Error(err), log.Fields{"group": authv1.SchemeGroupVersion, "scheme": s})
		return
	}
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Event(logs, "an error occurred while loading kubeconfig", log.Error(err))
		return
	}
	c, err := client.New(cfg, client.Options{Scheme: s})
	if err != nil {
		log.Event(logs, "an error occurred while creating kubernetes client", log.Error(err))
		return
	}

	if !strings.Contains(serviceAddr, "://") {
		serviceAddr = "http://" + serviceAddr
	}

	rec := reconciler.New(serviceAddr, c)
	authenticator := internal.TokenReviewAuthenticator{Client: c}

	go func() {
		for {
			select {
			case <-stopLatch.Chan():
				return
			case evt := <-reconcileEventChannel:
				res, err := rec.Reconcile(context.Background(), evt)
				log.Event(logs, "reconcile", log.V(1), log.Fields{"res": res, "err": err})
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stopLatch.Close()

		internal.Ingest(ingestAddr, records, logs, metrics, stopSignal, nil)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stopLatch.Close()

		internal.Listen(listenAddr, tlsConfig, listenerReg, logs, metrics, stopSignal, nil, authenticator)
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
								metrics.ListenerRemoved(l)
								return true
							}
						}
						return false
					})
				}
				listeners = append(listeners, listenersToAdd...)
				metrics.CurrentListeners(len(listeners))
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
