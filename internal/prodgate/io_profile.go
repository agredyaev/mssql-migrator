package prodgate

// DbIoProfile holds driver-boundary SQL I/O counters for perf debugging.
type DbIoProfile struct {
	ConnectMS      int64 `json:"connect_ms,omitempty"`
	QueryMS        int64 `json:"query_ms"`
	QueryCalls     int64 `json:"query_calls"`
	FetchMS        int64 `json:"fetch_ms,omitempty"`
	FetchCalls     int64 `json:"fetch_calls,omitempty"`
	ExecMS         int64 `json:"exec_ms"`
	ExecCalls      int64 `json:"exec_calls"`
	ExtraConnects  int64 `json:"extra_connects,omitempty"`
	ExtraConnectMS int64 `json:"extra_connect_ms,omitempty"`
}

func (p DbIoProfile) DbBoundaryMS() int64 {
	return p.QueryMS + p.FetchMS + p.ExecMS
}
