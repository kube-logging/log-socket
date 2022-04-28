package internal

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"
)

var (
	DefLabel          = map[string]string{"app.kubernetes.io/created-by": "log-socket"}
	FlowNameHeaderKey = "flowName"
	FlowAnnotationKey = "flowRef"
)

type Record struct {
	Data []byte
}

type RecordSink interface {
	Push(Record)
}

type RecordsChannel chan Record

func (rs RecordsChannel) Push(r Record) {
	rs <- r
}

type Handleable interface {
	HandleWith(func())
}

func NewWaitableLatch() *WaitableLatch {
	return &WaitableLatch{
		ch: make(chan struct{}),
	}
}

type WaitableLatch struct {
	ch   chan struct{}
	once sync.Once
}

func (l *WaitableLatch) Chan() <-chan struct{} {
	return l.ch
}

func (l *WaitableLatch) Close() {
	l.once.Do(l.close)
}

func (l *WaitableLatch) close() {
	close(l.ch)
}

func (l *WaitableLatch) Wait() {
	<-l.ch
}

func NewHandleableLatch(ch <-chan struct{}) *HandleableLatch {
	l := &HandleableLatch{
		ch: ch,
	}
	go l.watch()
	return l
}

type HandleableLatch struct {
	ch       <-chan struct{}
	handlers []func()
	mutex    sync.Mutex
}

func (l *HandleableLatch) HandleWith(handler func()) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	select {
	case <-l.ch:
		handler()
	default:
		l.handlers = append(l.handlers, handler)
	}
}

func (l *HandleableLatch) watch() {
	<-l.ch
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, handler := range l.handlers {
		handler()
	}
}

type ReconcileEventChannel chan ReconcileEvent

type FlowKind string

const (
	FKClusterFlow FlowKind = "clusterflow"
	FKFlow        FlowKind = "flow"
)

type FlowReference struct {
	types.NamespacedName
	Kind FlowKind
}

type ReconcileEvent struct {
	Requests []FlowReference
}

type ListenerEventChannel chan ListenerEvent

type ListenerEvent struct {
	Listener  Listener
	EventType listenerEventType
}

type listenerEventType uint

const (
	RegisterListener listenerEventType = iota
	UnregisterListener
)

func (r ListenerEventChannel) Register(l Listener) {
	r <- ListenerEvent{
		Listener:  l,
		EventType: RegisterListener,
	}
}

func (r ListenerEventChannel) Unregister(l Listener) {
	r <- ListenerEvent{
		Listener:  l,
		EventType: UnregisterListener,
	}
}
