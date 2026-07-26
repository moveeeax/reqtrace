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
			// Dual-stack hosts can fire ConnectStart/ConnectDone more than once:
			// Go dials each resolved address in turn (or races them) until one
			// succeeds, so an unreachable address attempted first completes with
			// an error before the address that is actually used. Recorder.Mark
			// keeps only the first instant it sees per event, so marking on a
			// failed attempt here would pin EventConnectDone to that doomed
			// attempt and understate (or otherwise misreport) TCPConnect for the
			// connection the request actually ends up using. Only the attempt
			// that succeeds should complete the phase.
			if err != nil {
				return
			}
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
