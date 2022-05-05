package internal

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	authv1 "k8s.io/api/authentication/v1"

	"github.com/banzaicloud/log-socket/internal/metrics"
	"github.com/banzaicloud/log-socket/log"
)

func Listen(addr string, tlsConfig *tls.Config, reg ListenerRegistry, logs log.Sink,
	stopSignal Handleable, terminationSignal Handleable, authenticator Authenticator) {
	upgrader := websocket.Upgrader{}
	server := &http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Event(logs, "new listener connection request", log.V(2), log.Fields{"request": r})

			metrics.Listeners(metrics.MListenerTotal)

			authToken := r.Header.Get(AuthHeaderKey)
			if authToken == "" {
				metrics.Listeners(metrics.MListenerRejected)
				log.Event(logs, "no authentication token in request headers", log.Fields{"headers": r.Header})
				http.Error(w, "missing authentication token", http.StatusForbidden)
				return
			}

			usrInfo, err := authenticator.Authenticate(authToken)
			if err != nil {
				metrics.Listeners(metrics.MListenerRejected)
				log.Event(logs, "authentication failed", log.Error(err), log.Fields{"token": authToken})
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			flow, err := ExtractFlow(r)
			if err != nil {
				log.Event(logs, "failed to extract flow from request", log.Error(err), log.Fields{"request": r})
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			wsConn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Event(logs, "failed to upgrade connection", log.Error(err))
				// cannot reply with an error here since the connection has been "hijacked"
				return
			}

			log.Event(logs, "successful websocket upgrade", log.V(2), log.Fields{"request": r, "wsConn": wsConn})

			l := &listener{
				conn:    wsConn,
				reg:     reg,
				logs:    logs,
				flow:    flow,
				usrInfo: usrInfo,
			}
			metrics.Listeners(metrics.MListenerApproved)
			reg.Register(l)
			go l.readLoop()
			wsConn.SetCloseHandler(func(code int, text string) error {
				metrics.Listeners(metrics.MListenerRemoved)
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

type ListenerRegistry interface {
	Register(Listener)
	Unregister(Listener)
}

type Listener interface {
	Send(Record)
	Flow() FlowReference
}

type listener struct {
	conn    *websocket.Conn
	flow    FlowReference
	logs    log.Sink
	reg     ListenerRegistry
	usrInfo authv1.UserInfo
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

	allowListStr, ok := GetIn(r.Data, "kubernetes", "labels", RBACAllowList).(string)
	if !ok {
		metrics.Log(metrics.MLogFiltered)
		metrics.Bytes(metrics.MBytesFiltered, len(r.RawData))
		log.Event(l.logs, "RBAC list missing from log record", log.V(1), log.Fields{"record": r})
		return
	}
	allowList := strings.Split(allowListStr, ",")

	data := r.RawData
	if !hasItem(allowList, strings.ReplaceAll(l.usrInfo.Username, ":", "_")) {
		metrics.Log(metrics.MLogFiltered)
		metrics.Bytes(metrics.MBytesFiltered, len(r.RawData))
		log.Event(l.logs, "listener does not have permission to view log record", log.V(1), log.Fields{"listener": l, "record": r, "allowList": allowList})

		data = []byte(fmt.Sprintf(`{"error": "Permission denied to access %s logs for %s"}`, GetIn(r.Data, "kubernetes", "pod_name").(string), l.usrInfo.Username))
	} else {
		metrics.Log(metrics.MLogTransfered)
		metrics.Bytes(metrics.MBytesTransferred, len(r.RawData))
	}

	log.Event(l.logs, "sending log record to listener", log.V(1), log.Fields{"listener": l, "record": r})

	wc, err := l.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		log.Event(l.logs, "an error occurred while getting next writer for websocket connection", log.Error(err))
		goto unregister
	}

	if _, err := wc.Write(data); err != nil {
		log.Event(l.logs, "an error occurred while writing record data to websocket connection", log.Error(err))
		goto unregister
	}

	if err := wc.Close(); err != nil {
		log.Event(l.logs, "an error occurred while flushing frame to websocket connection", log.Error(err))
		goto unregister
	}

	return

unregister:
	metrics.Listeners(metrics.MListenerRemoved)
	go l.reg.Unregister(l)
}

// readLoop reads the websocket connection so we handle close messages
func (l *listener) readLoop() {
	for {
		typ, dat, err := l.conn.ReadMessage()
		log.Event(l.logs, "read message from listener", log.V(1), log.Fields{"type": typ, "data": dat, "error": err})
		if err != nil {
			log.Event(l.logs, "an error occurred while reading websocket connection", log.Error(err))
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
