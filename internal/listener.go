package internal

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"go.uber.org/multierr"
	authv1 "k8s.io/api/authentication/v1"

	"github.com/banzaicloud/log-socket/log"
)

func Listen(addr string, tlsConfig *tls.Config, reg ListenerRegistry, logs log.Sink, metrics ListenMetrics,
	stopSignal Handleable, terminationSignal Handleable, authenticator Authenticator) {
	upgrader := websocket.Upgrader{}
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Event(logs, "new listener connection request", log.V(2), log.Fields{"request": r})

			flow, err := ExtractFlow(r)
			if err != nil {
				log.Event(logs, "failed to extract flow from request", log.V(1), log.Error(err), log.Fields{"request": r})
				metrics.ListenerRejected(flow, authv1.UserInfo{})
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			authToken := r.Header.Get(AuthHeaderKey)
			if authToken == "" {
				log.Event(logs, "no authentication token in request headers", log.V(1), log.Fields{"headers": r.Header})
				metrics.ListenerRejected(flow, authv1.UserInfo{})
				http.Error(w, "missing authentication token", http.StatusForbidden)
				return
			}

			usrInfo, err := authenticator.Authenticate(authToken)
			if err != nil {
				log.Event(logs, "authentication failed", log.V(1), log.Error(err), log.Fields{"token": authToken})
				metrics.ListenerRejected(flow, usrInfo)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			wsConn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Event(logs, "failed to upgrade connection", log.V(1), log.Error(err))
				metrics.ListenerRejected(flow, usrInfo)
				// cannot reply with an error here since the connection has been "hijacked"
				return
			}

			log.Event(logs, "successful websocket upgrade", log.V(2), log.Fields{"request": r, "wsConn": wsConn})

			metrics.ListenerAccepted(flow, usrInfo)

			l := &listener{
				conn:    wsConn,
				flow:    flow,
				logs:    logs,
				metrics: metrics,
				reg:     reg,
				usrInfo: usrInfo,
			}
			reg.Register(l)
			go l.readLoop()
			wsConn.SetCloseHandler(func(code int, text string) error {
				log.Event(logs, "websocket connection closed", log.V(1), log.Fields{"code": code, "text": text, "listener": l})
				reg.Unregister(l)
				return nil
			})

			log.Event(logs, "listener connected", log.Fields{"listener": l})
		}),
		TLSConfig: tlsConfig,
	}

	if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		log.Event(logs, "websocket listener server returned an error", log.Error(err))
	}
}

type ListenMetrics interface {
	ListenerAccepted(flow FlowReference, user authv1.UserInfo)
	ListenerRejected(flow FlowReference, user authv1.UserInfo)
	listenerMetrics
}

type ListenerRegistry interface {
	Register(Listener)
	Unregister(Listener)
}

type Listener interface {
	Send(Record)
	Flow() FlowReference
	User() authv1.UserInfo
}

type listener struct {
	conn    *websocket.Conn
	flow    FlowReference
	logs    log.Sink
	metrics listenerMetrics
	reg     ListenerRegistry
	usrInfo authv1.UserInfo
}

type listenerMetrics interface {
	LogRecordRedacted(l Listener, r Record)
	LogRecordTransmitted(l Listener, r Record)
}

func (l listener) Equals(o listener) bool {
	return l.conn == o.conn
}

func (l listener) Flow() FlowReference {
	return l.flow
}

func (l listener) Format(f fmt.State, c rune) {
	type listener struct {
		Conn *websocket.Conn
		Flow FlowReference
		User authv1.UserInfo
	}
	flag := ""
	switch {
	case f.Flag('#'):
		flag = "#"
	case f.Flag('+'):
		flag = "+"
	}
	fmt.Fprintf(f, fmt.Sprintf("%%%s%c", flag, c), listener{
		Conn: l.conn,
		Flow: l.flow,
		User: l.usrInfo,
	})
}

func (l *listener) Send(r Record) {
	log.Event(l.logs, "processing log record", log.V(2), log.Fields{"listener": l, "record": r})

	rules, err := loadRBACRules(r)
	if err != nil {
		log.Event(l.logs, "an error occurred while loading RBAC rules from record", log.V(1), log.Fields{"record": r})
	}

	data := r.RawData
	if !rules.canView(l.usrInfo) {
		log.Event(l.logs, "listener does not have permission to view log record", log.V(1), log.Fields{"listener": l, "record": r, "rules": rules})
		l.metrics.LogRecordRedacted(l, r)

		data = []byte(fmt.Sprintf(`{"error": "Permission denied to access %s logs for %s"}`, r.Data.Kubernetes.PodName, l.usrInfo.Username))
	} else {
		l.metrics.LogRecordTransmitted(l, r)
	}

	log.Event(l.logs, "sending log record to listener", log.V(1), log.Fields{"listener": l, "record": r})

	wc, err := l.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		log.Event(l.logs, "an error occurred while getting next writer for websocket connection", log.V(1), log.Error(err))
		goto unregister
	}

	if _, err := wc.Write(data); err != nil {
		log.Event(l.logs, "an error occurred while writing record data to websocket connection", log.V(1), log.Error(err))
		goto unregister
	}

	if err := wc.Close(); err != nil {
		log.Event(l.logs, "an error occurred while flushing frame to websocket connection", log.V(1), log.Error(err))
		goto unregister
	}

	return

unregister:
	go l.reg.Unregister(l)
}

func (l *listener) User() authv1.UserInfo {
	return l.usrInfo
}

// readLoop reads the websocket connection so we handle close messages
func (l *listener) readLoop() {
	for {
		typ, dat, err := l.conn.ReadMessage()
		log.Event(l.logs, "read message from listener", log.V(2), log.Fields{"type": typ, "data": dat, "error": err})
		if err != nil {
			log.Event(l.logs, "an error occurred while reading websocket connection", log.V(1), log.Error(err))
			return
		}
		if typ == websocket.CloseMessage {
			return
		}
	}
}

func ExtractFlow(req *http.Request) (res FlowReference, err error) {
	if elts := strings.Split(strings.Trim(req.URL.Path, "/"), "/"); len(elts) == 3 {
		res.Kind, res.Namespace, res.Name = FlowKind(elts[0]), elts[1], elts[2]
		return
	}
	return res, errors.New("URL path is not a valid flow reference")
}

func loadRBACRules(r Record) (res rbacRules, err error) {
	res = make(rbacRules)
loop:
	for k, v := range r.Data.Kubernetes.Labels {
		const keyPrefix = "rbac/"
		if strings.HasPrefix(k, keyPrefix) {
			p := policy(v)
			switch p {
			case policyAllow, policyDeny:
			default:
				err = multierr.Append(err, invalidRBACRule{k, v})
				continue loop
			}
			res[k[len(keyPrefix):]] = p
		}
	}
	return
}

type rbacRules map[string]policy

func (rs rbacRules) canView(userInfo authv1.UserInfo) bool {
	key := userInfo.Username
	// skip first 2 segments
	key = key[strings.IndexRune(key, ':')+1:]
	key = key[strings.IndexRune(key, ':')+1:]
	key = strings.ReplaceAll(key, ":", "_")
	if p, ok := rs[key]; ok { // user has custom policy
		return p == policyAllow
	}
	if p, ok := rs["policy"]; ok { // user has no custom policy, try using default policy
		return p == policyAllow
	}
	return false // default policy is deny
}

type policy string

const policyAllow policy = "allow"
const policyDeny policy = "deny"

type invalidRBACRule struct {
	key   string
	value string
}

func (e invalidRBACRule) Error() string {
	return fmt.Sprintf(`invalid RBAC rule "%s: %s"`, e.key, e.value)
}

func firstIndexOf[T comparable](slice []T, item T) int {
	for idx, itm := range slice {
		if itm == item {
			return idx
		}
	}
	return -1
}

func hasItem[T comparable](slice []T, item T) bool {
	return firstIndexOf(slice, item) != -1
}
