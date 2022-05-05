package metrics

func Key(name string, labels ...string) *TMetric {
	return &TMetric{Name: name, Labels: labels}
}

type TMetric struct {
	Name   string
	Labels []string
}
