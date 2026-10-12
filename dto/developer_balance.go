package dto

// DeveloperKeyBalance exposes key allowance in the configured display unit.
type DeveloperKeyBalance struct {
	Object     string  `json:"object"`
	Unit       string  `json:"unit"`
	UnitSymbol string  `json:"unit_symbol,omitempty"`
	Total      *string `json:"total"`
	Used       string  `json:"used"`
	Remaining  *string `json:"remaining"`
	Unlimited  bool    `json:"unlimited"`
	ExpiresAt  *int64  `json:"expires_at"`
}
