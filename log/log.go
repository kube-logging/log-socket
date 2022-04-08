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
type Target = log.Target

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

func WithFields(target Target, fields log.FieldSet) Target {
	return TargetWithFields{
		Target: target,
		Fields: fields,
	}
}

type TargetWithFields struct {
	Target
	Fields log.FieldSet
}

func (t TargetWithFields) Record(message string, fields log.FieldSet) {
	t.Target.Record(message, log.CollapseFieldSets(t.Fields, fields))
}

func NewWriterTarget(w io.Writer) *WriterTarget {
	return &WriterTarget{
		writer: w,
	}
}

type WriterTarget struct {
	writer io.Writer
	mutex  sync.Mutex
}

func (t *WriterTarget) Record(message string, fields log.FieldSet) {
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
