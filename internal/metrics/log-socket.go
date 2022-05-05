package metrics

type MetricKey = string
type LogAction = MetricKey
type BytesAction = MetricKey
type ListenerAction = MetricKey

const (
	MTotalLog      MetricKey = "total_log"
	MLogReceived   LogAction = "received"
	MLogTransfered LogAction = "transferred"
	MLogFiltered   LogAction = "filtered"

	MTotalBytes       MetricKey   = "total_bytes"
	MBytesReceived    BytesAction = "received"
	MBytesTransferred BytesAction = "transferred"
	MBytesFiltered    BytesAction = "filtered"

	MListeners        MetricKey      = "listeners"
	MListenerCurrent  MetricKey      = "current"
	MListenerTotal    ListenerAction = "total"
	MListenerApproved ListenerAction = "approved"
	MListenerRejected ListenerAction = "rejected"
	MListenerRemoved  ListenerAction = "removed"

	MHealthChecks MetricKey = "total_healthchecks"

	MError MetricKey = "errors"

	KStatus MetricKey = "status"
)

func Log(action LogAction) {
	Record(Key(MTotalLog, KStatus), Inc(), action)
}

func Bytes(action BytesAction, val int) {
	Record(Key(MTotalBytes, KStatus), Add(float64(val)), action)
}

func Listeners(action ListenerAction) {
	switch action {
	case MListenerRejected:
		Record(Key(MListeners, KStatus), Inc(), MListenerRejected)
	case MListenerApproved:
		Record(Key(MListeners, KStatus), Inc(), MListenerApproved)
		Record(Key(MListeners, KStatus), Inc(), MListenerCurrent)
	case MListenerRemoved:
		Record(Key(MListeners, KStatus), Dec(), MListenerCurrent)
	case MListenerTotal:
		Record(Key(MListeners, KStatus), Inc(), MListenerTotal)
	}
}

func HealthCheck() {
	Record(Key(MHealthChecks), Inc())
}

func Error() {
	Record(Key(MError), Inc())
}
