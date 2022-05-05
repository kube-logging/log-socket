package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/banzaicloud/log-socket/log"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const HealthCheckEndpoint = "/healthz"
const MetricsEndpoint = "/metrics"

func Ingest(addr string, records RecordSink, logs log.Sink, metrics IngestMetrics, stopSignal Handleable, terminateSignal Handleable) {
	logs = log.WithFields(logs, log.Fields{"task": "log ingestion"})

	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Event(logs, "HTTP server received request", log.V(2), log.Fields{"request": r.Body})

			if r.URL.Path == HealthCheckEndpoint {
				log.Event(logs, "health check", log.V(1))
				metrics.HealthCheck()
				w.WriteHeader(http.StatusOK)
				return
			}

			if r.URL.Path == MetricsEndpoint {
				log.Event(logs, "metrics", log.V(1))
				promhttp.Handler().ServeHTTP(w, r)
				return
			}

			elts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(elts) != 3 {
				log.Event(logs, "URL path is not a valid flow reference", log.Fields{"url": r.URL})
				http.Error(w, "invalid URL path", http.StatusBadRequest)
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

			dataSet := bytes.Split(data, []byte{'\n'})
			for _, data := range dataSet {

				if len(data) == 0 {
					continue
				}

				rec := Record{
					RawData: data,
					Flow:    flow,
				}

				metrics.LogRecordReceived(rec)

				if err := json.Unmarshal(data, &rec.Data); err != nil {
					log.Event(logs, "failed to parse log data", log.Error(err), log.Fields{"data": string(data)})
					http.Error(w, "failed to parse log data", http.StatusBadRequest)
					return
				}

				log.Event(logs, "got log record via HTTP", log.V(1), log.Fields{"record": rec})
				records.Push(rec)
			}
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

type IngestMetrics interface {
	HealthCheck()
	LogRecordReceived(r Record)
}
