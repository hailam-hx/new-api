package dto

type DeveloperTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DeveloperTask is an explicit metadata projection, never a provider payload.
type DeveloperTask struct {
	Object      string              `json:"object"`
	ID          string              `json:"id"`
	Type        string              `json:"type"`
	Model       string              `json:"model"`
	Status      string              `json:"status"`
	Progress    string              `json:"progress"`
	CreatedAt   int64               `json:"created_at"`
	StartedAt   *int64              `json:"started_at,omitempty"`
	CompletedAt *int64              `json:"completed_at,omitempty"`
	RequestID   string              `json:"request_id,omitempty"`
	Error       *DeveloperTaskError `json:"error,omitempty"`
}

type DeveloperTaskList struct {
	Object     string          `json:"object"`
	Data       []DeveloperTask `json:"data"`
	HasMore    bool            `json:"has_more"`
	NextCursor string          `json:"next_cursor,omitempty"`
}
