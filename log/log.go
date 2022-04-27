package log

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/siliconbrain/gologlite/log"
)

var Event = log.Event

type Fields = log.FieldMap
type Sink = log.Target

func Error(err error) log.FieldSet {
	return Fields{
		"error": err,
	}
}

func V(verbosity int) log.FieldSet {
	return Fields{
		"verbosity": verbosity,
	}
}

func WithFields(logs Sink, fields log.FieldSet) Sink {
	return SinkWithFields{
		Sink:   logs,
		Fields: fields,
	}
}

type SinkWithFields struct {
	Sink
	Fields log.FieldSet
}

func (t SinkWithFields) Record(message string, fields log.FieldSet) {
	t.Sink.Record(message, log.CollapseFieldSets(t.Fields, fields))
}

func NewWriterSink(w io.Writer) *WriterSink {
	return &WriterSink{
		writer: w,
	}
}

type WriterSink struct {
	writer io.Writer
	mutex  sync.Mutex
}

func (t *WriterSink) Record(message string, fields log.FieldSet) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	_, _ = io.WriteString(t.writer, "[ ")
	_, _ = io.WriteString(t.writer, time.Now().UTC().Format(time.RFC3339))
	_, _ = io.WriteString(t.writer, " ] ")
	_, _ = io.WriteString(t.writer, message)
	_, _ = io.WriteString(t.writer, " | ")
	_ = json.NewEncoder(t.writer).Encode(fields)
	_, _ = fmt.Fprintln(t.writer)
}
