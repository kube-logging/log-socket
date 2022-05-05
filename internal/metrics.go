package internal

import (
	"github.com/banzaicloud/log-socket/log"
	"github.com/prometheus/client_golang/prometheus"
	authv1 "k8s.io/api/authentication/v1"
)

const (
	metricNamespace         = "log_socket"
	flowKindLabelName       = "kind"
	flowNamespaceLabelName  = "namespace"
	flowNameLabelName       = "name"
	listenerStatusLabelName = "status"
	listenerUserLabelName   = "user"
	recordStatusLabelName   = "status"
)

func NewMetrics(logs log.Sink) *Metrics {
	return &Metrics{
		logs: logs,

		bytesReceived: registered(prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "bytes_received",
		}, []string{flowKindLabelName, flowNamespaceLabelName, flowNameLabelName})),
		currentListeners: registered(prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "current_listeners",
		})),
		errors: registered(prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "errors",
		})),
		healthChecks: registered(prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "healthchecks",
		})),
		listenerBytes: registered(prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "bytes",
		}, []string{recordStatusLabelName, flowKindLabelName, flowNamespaceLabelName, flowNameLabelName, listenerUserLabelName})),
		listenerRecords: registered(prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "records",
		}, []string{recordStatusLabelName, flowKindLabelName, flowNamespaceLabelName, flowNameLabelName, listenerUserLabelName})),
		listeners: registered(prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "listeners",
		}, []string{listenerStatusLabelName, flowKindLabelName, flowNamespaceLabelName, flowNameLabelName, listenerUserLabelName})),
		recordsReceived: registered(prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "records_received",
		}, []string{flowKindLabelName, flowNamespaceLabelName, flowNameLabelName})),
	}
}

type Metrics struct {
	logs log.Sink

	bytesReceived    *prometheus.CounterVec
	currentListeners prometheus.Gauge
	errors           prometheus.Counter
	healthChecks     prometheus.Counter
	listenerBytes    *prometheus.CounterVec
	listenerRecords  *prometheus.CounterVec
	listeners        *prometheus.CounterVec
	recordsReceived  *prometheus.CounterVec
}

func (ms *Metrics) CurrentListeners(cnt int) {
	ms.currentListeners.Set(float64(cnt))
}

func (ms *Metrics) Error() {
	ms.errors.Inc()
}

func (ms *Metrics) HealthCheck() {
	ms.healthChecks.Inc()
}

func (ms *Metrics) ListenerAccepted(flow FlowReference, user authv1.UserInfo) {
	ms.listeners.With(assembleLabels(prometheus.Labels{listenerStatusLabelName: "accepted"}, flowLabels(flow), userLabels(user))).Inc()
}

func (ms *Metrics) ListenerRejected(flow FlowReference, user authv1.UserInfo) {
	ms.listeners.With(assembleLabels(prometheus.Labels{listenerStatusLabelName: "rejected"}, flowLabels(flow), userLabels(user))).Inc()
}

func (ms *Metrics) ListenerRemoved(l Listener) {
	ms.listeners.With(assembleLabels(prometheus.Labels{listenerStatusLabelName: "removed"}, flowLabels(l.Flow()), userLabels(l.User()))).Inc()
}

func (ms *Metrics) LogRecordReceived(r Record) {
	labels := assembleLabels(prometheus.Labels{}, flowLabels(r.Flow))
	ms.bytesReceived.With(labels).Add(float64(len(r.RawData)))
	ms.recordsReceived.With(labels).Inc()
}

func (ms *Metrics) LogRecordRedacted(l Listener, r Record) {
	labels := assembleLabels(prometheus.Labels{recordStatusLabelName: "redacted"}, flowLabels(l.Flow()), userLabels(l.User()))
	ms.listenerBytes.With(labels).Add(float64(len(r.RawData)))
	ms.listenerRecords.With(labels).Inc()
}

func (ms *Metrics) LogRecordTransmitted(l Listener, r Record) {
	labels := assembleLabels(prometheus.Labels{recordStatusLabelName: "transmitted"}, flowLabels(l.Flow()), userLabels(l.User()))
	ms.listenerBytes.With(labels).Add(float64(len(r.RawData)))
	ms.listenerRecords.With(labels).Inc()
}

func registered[T prometheus.Collector](metric T) T {
	prometheus.MustRegister(metric)
	return metric
}

func assembleLabels(labels prometheus.Labels, srcs ...labelSource) prometheus.Labels {
	for _, src := range srcs {
		src.AddToLabels(labels)
	}
	return labels
}

type labelSource interface {
	AddToLabels(prometheus.Labels)
}

type flowLabels FlowReference

func (f flowLabels) AddToLabels(labels prometheus.Labels) {
	labels[flowKindLabelName] = string(f.Kind)
	labels[flowNamespaceLabelName] = f.Namespace
	labels[flowNameLabelName] = f.Name
}

type userLabels authv1.UserInfo

func (u userLabels) AddToLabels(labels prometheus.Labels) {
	labels[listenerUserLabelName] = u.Username
}
