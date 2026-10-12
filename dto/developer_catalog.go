package dto

// DeveloperCatalog is an independent public contract. Monetary values are
// decimal strings in Currency; optional prices distinguish absence from zero.
type DeveloperCatalog struct {
	Object         string                  `json:"object"`
	SchemaVersion  string                  `json:"schema_version"`
	Currency       string                  `json:"currency"`
	CurrencySymbol string                  `json:"currency_symbol,omitempty"`
	Models         []DeveloperCatalogModel `json:"models"`
}

type DeveloperCatalogModel struct {
	ID        string                     `json:"id"`
	Provider  string                     `json:"provider,omitempty"`
	Category  string                     `json:"category,omitempty"`
	Endpoints []DeveloperCatalogEndpoint `json:"endpoints"`
	Pricing   DeveloperCatalogPricing    `json:"pricing"`
	Billing   DeveloperCatalogBilling    `json:"billing"`
}

type DeveloperCatalogEndpoint struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

type DeveloperCatalogPricing struct {
	Variable               bool                             `json:"variable"`
	InputPer1MTokens       *string                          `json:"input_per_1m_tokens,omitempty"`
	OutputPer1MTokens      *string                          `json:"output_per_1m_tokens,omitempty"`
	CachedInputPer1MTokens *string                          `json:"cached_input_per_1m_tokens,omitempty"`
	PerRequest             *string                          `json:"per_request,omitempty"`
	PerImage               *string                          `json:"per_image,omitempty"`
	PerSecond              *string                          `json:"per_second,omitempty"`
	PerCharacter           *string                          `json:"per_character,omitempty"`
	Components             []DeveloperCatalogPriceComponent `json:"components,omitempty"`
}

// Components preserve independently priced usage subjects which do not have a
// dedicated field, including image/audio tokens and vendor resource units.
type DeveloperCatalogPriceComponent struct {
	Subject string `json:"subject"`
	Unit    string `json:"unit"`
	Price   string `json:"price"`
}

type DeveloperCatalogBilling struct {
	Mode     string   `json:"mode"`
	Features []string `json:"features,omitempty"`
}
