package internal

import (
	"path"
	"sync"

	authv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	FKClusterFlow FlowKind = "clusterflow"
	FKFlow        FlowKind = "flow"

	RegisterListener listenerEventType = iota
	UnregisterListener

	AuthHeaderKey = "X-Authorization"
	RBACAllowList = "rbac/AllowList"
)

var (
	DefLabel          = map[string]string{"app.kubernetes.io/created-by": "log-socket"}
	FlowAnnotationKey = "flowRef"
)

type Strimap = map[string]interface{}

func GetIn(m interface{}, args ...interface{}) interface{} {
	var subSet = m
	for _, arg := range args {
		switch t := subSet.(type) {
		case []interface{}:
			elt, ok := arg.(int)
			if !ok {
				return nil
			}
			if elt < 0 || elt >= len(t) {
				return nil
			}
			subSet = t[elt]
		case map[string]interface{}:
			key, ok := arg.(string)
			if !ok {
				return nil
			}
			subSet = t[key]
		default:
			return nil
		}
	}
	return subSet
}

type Record struct {
	RawData []byte
	Data    Strimap
	Flow    FlowReference
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

type FlowReference struct {
	types.NamespacedName
	Kind FlowKind
}

func (f FlowReference) URL() string {
	return path.Join(string(f.Kind), f.Namespace, f.Name)
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

type Authenticator interface {
	Authenticate(token string) (authv1.UserInfo, error)
}
