package dto

// DeveloperKeyUsage reports retained events, not a complete wallet ledger.
type DeveloperKeyUsage struct {
	Object              string                    `json:"object"`
	Unit                string                    `json:"unit"`
	UnitSymbol          string                    `json:"unit_symbol,omitempty"`
	StartTime           int64                     `json:"start_time"`
	EndTime             int64                     `json:"end_time"`
	RecordedConsumption string                    `json:"recorded_consumption"`
	RecordedRefunds     string                    `json:"recorded_refunds"`
	RecordedNet         string                    `json:"recorded_net"`
	BillingEvents       int64                     `json:"billing_events"`
	ErrorEvents         int64                     `json:"error_events"`
	Components          []DeveloperUsageComponent `json:"components"`
	ComponentStatus     string                    `json:"component_status"`
}

type DeveloperUsageComponent struct {
	Subject  string `json:"subject"`
	Unit     string `json:"unit"`
	Quantity string `json:"quantity"`
}
