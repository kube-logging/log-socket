package metrics

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var mutex sync.Mutex

var DefaultMetrics = map[string]*prometheus.GaugeVec{}

func register(metric *TMetric) (*prometheus.GaugeVec, error) {
	x := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: metric.Name,
	},
		metric.Labels,
	)

	if err := prometheus.Register(x); err != nil {
		return nil, err
	}
	DefaultMetrics[metric.Name] = x
	return x, nil
}

type FUpdater func(prometheus.Gauge)

func Record(metric *TMetric, updater func(prometheus.Gauge), keys ...string) {
	mutex.Lock()
	defer mutex.Unlock()
	if updater == nil {
		return
	}
	m, ok := DefaultMetrics[metric.Name]
	if !ok {
		var err error
		if m, err = register(metric); err != nil {
			fmt.Println(err)
			return
		}
	}

	g := m.WithLabelValues(keys...)
	updater(g)

}

func Inc() FUpdater {
	return func(g prometheus.Gauge) {
		g.Inc()
	}
}

func Dec() FUpdater {
	return func(g prometheus.Gauge) {
		g.Dec()
	}
}

func Add(val float64) FUpdater {
	return func(g prometheus.Gauge) {
		g.Add(val)
	}
}

func Sub(val float64) FUpdater {
	return func(g prometheus.Gauge) {
		g.Sub(val)
	}
}

func Set(val float64) FUpdater {
	return func(g prometheus.Gauge) {
		g.Set(val)
	}
}
