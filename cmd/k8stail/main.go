package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	pathpkg "path"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/pflag"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/banzaicloud/log-socket/internal"
	"github.com/banzaicloud/log-socket/log"
)

func main() {
	var authToken string
	var clusterFlow bool
	var listenAddr string
	var svcName string
	var svcNamespace string
	var svcPort string
	var verbosity int
	pflag.StringVarP(&authToken, "token", "t", "", "token used for authentication")
	pflag.BoolVarP(&clusterFlow, "clusterflow", "c", false, "stream logs from a cluster flow instead of a regular flow")
	pflag.StringVar(&listenAddr, "listen-addr", "", "address where the service accepts WebSocket listeners")
	pflag.StringVarP(&svcNamespace, "namespace", "n", "default", "log socket service namespace")
	pflag.StringVarP(&svcPort, "port", "p", "10001", "log socket service listening port")
	pflag.StringVarP(&svcName, "service", "s", "log-socket", "name of the service that accepts WebSocket listeners")
	pflag.IntVarP(&verbosity, "verbosity", "v", verbosity, "log verbosity level")
	pflag.Parse()

	var logs log.Sink = log.WithVerbosityFilter(log.NewWriterSink(os.Stderr), verbosity)

	if pflag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "no flow reference specified")
		pflag.Usage()
		os.Exit(1)
	}

	flowRef := pflag.Arg(0)
	elts := strings.SplitN(flowRef, "/", 2)
	if len(elts) != 2 {
		fmt.Fprintf(os.Stderr, "invalid flow reference %q\n", flowRef)
		pflag.Usage()
		os.Exit(1)
	}
	flowNamespace, flowName := elts[0], elts[1]

	flowKind := "flow"
	if clusterFlow {
		flowKind = "clusterflow"
	}

	dialer := *websocket.DefaultDialer

	path := pathpkg.Join("/", flowKind, flowNamespace, flowName)

	var listenURL *url.URL
	if listenAddr == "" {
		cfg, err := ctrl.GetConfig()
		if err != nil {
			log.Event(logs, "failed to get kubeconfig", log.Error(err))
			os.Exit(2)
		}
		tlsCfg, err := rest.TLSConfigFor(cfg)
		if err != nil {
			log.Event(logs, "failed to get TLS config for kubeconfig")
			os.Exit(2)
		}

		dialer.TLSClientConfig = tlsCfg

		listenURL, err = proxyURL(cfg, svcNamespace, "services", svcName, true, svcPort, path)
		if err != nil {
			log.Event(logs, "failed to generate K8s API server proxy URL for service", log.Error(err), log.Fields{
				"kubeconfig": cfg,
				"namespace":  svcNamespace,
				"name":       svcName,
				"port":       svcPort,
				"path":       path,
			})
			os.Exit(2)
		}
	} else {
		var err error
		if !strings.Contains(listenAddr, "://") {
			listenAddr = "wss://" + listenAddr
		}
		listenURL, err = url.Parse(listenAddr)
		if err != nil {
			log.Event(logs, "failed to parse listen address", log.Error(err))
			os.Exit(2)
		}
		listenURL.Path = pathpkg.Join(listenURL.Path, path)
	}

	listenURL.Scheme = "wss"

	if dialer.TLSClientConfig == nil {
		dialer.TLSClientConfig = &tls.Config{}
	}
	dialer.TLSClientConfig.InsecureSkipVerify = true

	wsConn, _, err := dialer.DialContext(context.Background(), listenURL.String(), http.Header{internal.AuthHeaderKey: []string{authToken}})
	if err != nil {
		log.Event(logs, "failed to open websocket connection", log.Error(err), log.Fields{"url": listenURL})
		os.Exit(2)
	}

	log.Event(logs, "successfully connected to service", log.V(1), log.Fields{"addr": wsConn.UnderlyingConn().RemoteAddr()})

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	go func() {
		// TODO: defer exit
		for {
			msgTyp, reader, err := wsConn.NextReader()
			if err != nil {
				log.Event(logs, "failed to get next reader for websocket connection", log.Error(err))
				return
			}
			switch msgTyp {
			case websocket.BinaryMessage:
				data, err := io.ReadAll(reader)
				if err != nil {
					log.Event(logs, "failed to read record data", log.V(1), log.Error(err))
					continue
				}
				log.Event(logs, "new record", log.V(2), log.Fields{"data": data})
				fmt.Println(string(data))
			}
		}
	}()

	signal := <-signals
	log.Event(logs, "received signal", log.V(1), log.Fields{"signal": signal})
	deadline := time.Now().Add(5 * time.Second)
	if err := wsConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, signal.String()), deadline); err != nil {
		log.Event(logs, "an error occurred while writing close message to websocket", log.Error(err))
		os.Exit(2)
	}
}

func proxyURL(cfg *rest.Config, namespace, resourceType, name string, tls bool, port string, path string) (uri *url.URL, err error) {
	switch resourceType {
	case "pods", "services":
	default:
		return nil, errors.New("invalid resource type")
	}

	uri, err = url.Parse(cfg.Host)
	if err != nil {
		return
	}

	apiPath := cfg.APIPath
	if apiPath == "" {
		apiPath = "/api/v1"
	}

	resource := name
	if tls {
		resource = "https:" + resource
	}
	if port != "" {
		resource = resource + ":" + port
	}

	uri.Path = pathpkg.Join(apiPath, "namespaces", namespace, resourceType, resource, "proxy", path)

	return
}
