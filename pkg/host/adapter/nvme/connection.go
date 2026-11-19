// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

// ConnectOption configures a single Connect call.
type ConnectOption func(*connectConfig)

// connectConfig is the resolved set of options for a Connect call.
// It is adapter-internal; callers compose it through ConnectOption funcs.
type connectConfig struct {
	transport Transport // defaults to TCP; an explicit empty value is invalid
	port      string    // trsvcid; "" = the transport default (4420 for tcp, none for any other transport)
}

// newConnectConfig applies the given options to a connectConfig holding Trident's defaults.
func newConnectConfig(opts []ConnectOption) connectConfig {
	c := connectConfig{transport: TransportTCP}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// WithTransport selects the transport. The default is TCP.
func WithTransport(t Transport) ConnectOption {
	return func(c *connectConfig) { c.transport = t }
}

// WithPort selects the target port (trsvcid) for IP transports. The default is 4420.
func WithPort(port string) ConnectOption {
	return func(c *connectConfig) { c.port = port }
}
