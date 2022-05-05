package metrics

import (
	"crypto/sha256"
	"fmt"
)

func Key(name string, labels ...string) *TMetric {
	return &TMetric{Name: name, Labels: labels}
}

type TMetric struct {
	Name   string
	Labels []string
}

func (t *TMetric) Hash() string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%v", t)))
	return fmt.Sprintf("%x", h.Sum(nil))
}
