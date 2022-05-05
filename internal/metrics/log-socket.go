package metrics

type MetricKey = string
type LogAction = MetricKey
type BytesAction = MetricKey
type ListenerAction = MetricKey

const (
	LogPrefix string = "log_socket_"

	MLog     MetricKey = LogPrefix + "event"
	MLogRecv MetricKey = LogPrefix + "event_received"

	MLogTransfered LogAction = "transferred"
	MLogFiltered   LogAction = "filtered"

	MTotalBytes MetricKey = LogPrefix + "bytes"
	MBytesRecv  MetricKey = LogPrefix + "bytes_received"

	MBytesTransferred BytesAction = "transferred"
	MBytesFiltered    BytesAction = "filtered"

	MListeners        MetricKey      = LogPrefix + "listeners"
	MListenerCurrent  ListenerAction = "current"
	MListenerTotal    ListenerAction = "total"
	MListenerApproved ListenerAction = "approved"
	MListenerRejected ListenerAction = "rejected"
	MListenerRemoved  ListenerAction = "removed"

	MListener MetricKey = LogPrefix + "listener"

	MHealthChecks MetricKey = LogPrefix + "healthchecks"

	MError MetricKey = "errors"

	MStatus    MetricKey = "status"
	MType      MetricKey = "type"
	MNamespace MetricKey = "namespace"
	MFlow      MetricKey = "flowname"
	MUser      MetricKey = "user"
)

var commonMetricKeys = []string{MStatus, MType, MNamespace, MFlow, MUser}

func LogReceived() {
	Record(Key(MLogRecv), Inc())
}

func Log(action LogAction, keys ...string) {
	Record(Key(MLog, commonMetricKeys...), Inc(), append([]string{action}, keys...)...)
}

func BytesReceived(val int) {
	Record(Key(MBytesRecv), Add(float64(val)))
}

func Bytes(action BytesAction, val int, keys ...string) {
	Record(Key(MTotalBytes, commonMetricKeys...), Add(float64(val)), append([]string{action}, keys...)...)
}

func ListenersTotal(action ListenerAction, v float64) {
	switch action {
	case MListenerRejected:
		Record(Key(MListeners, MStatus), Inc(), MListenerRejected)
	case MListenerApproved:
		Record(Key(MListeners, MStatus), Inc(), MListenerApproved)
	case MListenerRemoved:
		Record(Key(MListeners, MStatus), Inc(), MListenerRemoved)
	case MListenerTotal:
		Record(Key(MListeners, MStatus), Inc(), MListenerTotal)
	case MListenerCurrent:
		if v >= 0 {
			Record(Key(MListeners, MStatus), Set(v), MListenerCurrent)
		}

	}
}

func Listeners(action ListenerAction, val int, keys ...string) {
	switch action {
	case MListenerRejected:
		Record(Key(MListener, commonMetricKeys...), Inc(), append([]string{action}, keys...)...)
	case MListenerApproved:
		Record(Key(MListener, commonMetricKeys...), Inc(), append([]string{action}, keys...)...)
	case MListenerRemoved:
		Record(Key(MListener, commonMetricKeys...), Inc(), append([]string{action}, keys...)...)
	}
	ListenersTotal(action, float64(val))
}

func HealthCheck() {
	Record(Key(MHealthChecks), Inc())
}

func Error() {
	Record(Key(MError), Inc())
}
