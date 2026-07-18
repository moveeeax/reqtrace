package trace

import (
	"crypto/tls"
	"net/http/httptrace"
)

// NewClientTrace returns an *httptrace.ClientTrace whose callbacks stamp
// lifecycle events onto the given Recorder. The caller is responsible for
// marking EventStart before the request and EventDone afterwards, since those
// bracket the whole exchange rather than a single httptrace hook.
func NewClientTrace(r *Recorder) *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) {
			r.Mark(EventDNSStart)
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			r.Mark(EventDNSDone)
		},
		ConnectStart: func(network, addr string) {
			r.Mark(EventConnectStart)
		},
		ConnectDone: func(network, addr string, err error) {
			r.Mark(EventConnectDone)
		},
		TLSHandshakeStart: func() {
			r.Mark(EventTLSStart)
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			r.Mark(EventTLSDone)
		},
		GotFirstResponseByte: func() {
			r.Mark(EventFirstByte)
		},
	}
}
