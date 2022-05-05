package metrics

type MetricKey = string
type LogAction = MetricKey
type BytesAction = MetricKey
type ListenerAction = MetricKey

const (
	LogPrefix string = "log_socket_"

	MTotalLog      MetricKey = LogPrefix + "total_log"
	MLogReceived   LogAction = "received"
	MLogTransfered LogAction = "transferred"
	MLogFiltered   LogAction = "filtered"

	MTotalBytes       MetricKey   = LogPrefix + "total_bytes"
	MBytesReceived    BytesAction = "received"
	MBytesTransferred BytesAction = "transferred"
	MBytesFiltered    BytesAction = "filtered"

	MListeners        MetricKey      = LogPrefix + "listeners"
	MListenerCurrent  ListenerAction = "current"
	MListenerTotal    ListenerAction = "total"
	MListenerApproved ListenerAction = "approved"
	MListenerRejected ListenerAction = "rejected"
	MListenerRemoved  ListenerAction = "removed"

	MHealthChecks MetricKey = LogPrefix + "total_healthchecks"

	MError MetricKey = "errors"

	MStatus MetricKey = "status"
)

func Log(action LogAction) {
	Record(Key(MTotalLog, MStatus), Inc(), action)
}

func Bytes(action BytesAction, val int) {
	Record(Key(MTotalBytes, MStatus), Add(float64(val)), action)
}

func Listeners(action ListenerAction, v ...float64) {
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
		if len(v) > 0 {
			Record(Key(MListeners, MStatus), Set(v[0]), MListenerCurrent)
		} else {
			Record(Key(MListeners, MStatus), Set(0), MListenerCurrent)
		}

	}
}

func HealthCheck() {
	Record(Key(MHealthChecks), Inc())
}

func Error() {
	Record(Key(MError), Inc())
}
