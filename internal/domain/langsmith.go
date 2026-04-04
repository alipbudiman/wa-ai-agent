package ports

type LangSmithRunReport struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	RunType string `json:"run_type"`
}

type LangSmithRunEvent struct {
	Name string  `json:"name"`
	Time *string `json:"time,omitempty"`
}

type LangSmithLogReport struct {
	ID                     string               `json:"id"`
	Name                   string               `json:"name"`
	StartTime              *string              `json:"start_time,omitempty"`
	RunType                string               `json:"run_type"`
	EndTime                *string              `json:"end_time,omitempty"`
	Extra                  map[string]any       `json:"extra,omitempty"`
	Error                  *string              `json:"error,omitempty"`
	Serialized             any                  `json:"serialized,omitempty"`
	Events                 []LangSmithRunEvent  `json:"events,omitempty"`
	Inputs                 map[string]any       `json:"inputs,omitempty"`
	Outputs                map[string]any       `json:"outputs,omitempty"`
	ReferenceExampleID     *string              `json:"reference_example_id,omitempty"`
	ParentRunID            *string              `json:"parent_run_id,omitempty"`
	Tags                   []string             `json:"tags,omitempty"`
	Attachments            map[string]any       `json:"attachments,omitempty"`
	SessionID              *string              `json:"session_id,omitempty"`
	ChildRunIDs            []string             `json:"child_run_ids,omitempty"`
	ChildRuns              []LangSmithRunReport `json:"child_runs,omitempty"`
	FeedbackStats          any                  `json:"feedback_stats,omitempty"`
	AppPath                *string              `json:"app_path,omitempty"`
	ManifestID             *string              `json:"manifest_id,omitempty"`
	Status                 *string              `json:"status,omitempty"`
	PromptTokens           *int                 `json:"prompt_tokens,omitempty"`
	CompletionTokens       *int                 `json:"completion_tokens,omitempty"`
	TotalTokens            *int                 `json:"total_tokens,omitempty"`
	PromptTokenDetails     map[string]any       `json:"prompt_token_details,omitempty"`
	CompletionTokenDetails map[string]any       `json:"completion_token_details,omitempty"`
	FirstTokenTime         *string              `json:"first_token_time,omitempty"`
	TotalCost              *float64             `json:"total_cost,omitempty"`
	PromptCost             *float64             `json:"prompt_cost,omitempty"`
	CompletionCost         *float64             `json:"completion_cost,omitempty"`
	PromptCostDetails      map[string]any       `json:"prompt_cost_details,omitempty"`
	CompletionCostDetails  map[string]any       `json:"completion_cost_details,omitempty"`
	ParentRunIDs           []string             `json:"parent_run_ids,omitempty"`
	TraceID                *string              `json:"trace_id,omitempty"`
	DottedOrder            *string              `json:"dotted_order,omitempty"`
	InDataset              *bool                `json:"in_dataset,omitempty"`
}
