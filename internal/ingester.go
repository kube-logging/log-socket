package internal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/banzaicloud/log-socket/log"
)

const HealthCheckEndpoint = "/healthz"

func Ingest(addr string, records RecordSink, logs log.Sink, stopSignal Handleable, terminateSignal Handleable) {
	logs = log.WithFields(logs, log.Fields{"task": "log ingestion"})

	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Event(logs, "HTTP server received request", log.V(2), log.Fields{"request": r})

			if r.URL.Path == HealthCheckEndpoint {
				log.Event(logs, "health check", log.V(1))
				w.WriteHeader(http.StatusOK)
				return
			}

			elts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(elts) < 1 {
				http.Error(w, "invalid URL path", http.StatusNotFound)
				return
			}

			var flow FlowReference
			flow.Kind, flow.Namespace, flow.Name = FlowKind(elts[0]), elts[1], elts[2]
			switch flow.Kind {
			case FKClusterFlow, FKFlow:
			default:
				http.Error(w, "invalid flow kind", http.StatusBadRequest)
				return
			}

			data, err := io.ReadAll(r.Body)
			if err != nil {
				log.Event(logs, "failed to read request body", log.Error(err))
				http.Error(w, "failed to read request body", http.StatusInternalServerError)
				return
			}
			if err := r.Body.Close(); err != nil {
				log.Event(logs, "failed to close request body", log.Error(err))
				http.Error(w, "failed to close request body", http.StatusInternalServerError)
				return
			}

			rec := Record{
				RawData: data,
				Flow:    flow,
			}

			if err := json.Unmarshal(data, &rec.Data); err != nil {
				log.Event(logs, "failed to parse log data", log.Error(err))
				http.Error(w, "failed to parse log data", http.StatusBadRequest)
				return
			}

			log.Event(logs, "got log record via HTTP", log.V(1), log.Fields{"record": rec})
			records.Push(rec)
			w.WriteHeader(http.StatusOK)
		}),
	}

	var shutdownWG sync.WaitGroup
	if stopSignal != nil {
		ctx := context.Background()

		if terminateSignal != nil {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(context.Background())
			terminateSignal.HandleWith(cancel)
		}

		stopSignal.HandleWith(func() {
			shutdownWG.Add(1)
			defer shutdownWG.Done()
			if err := server.Shutdown(ctx); err != nil {
				log.Event(logs, "error during HTTP server shutdown", log.Error(err))
			}
		})
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Event(logs, "HTTP server ListenAndServe returned an error", log.Error(err))
	}
	shutdownWG.Wait()
}
