package llm

import (
	"context"

	"ai_agent/internal/types"
)

type PatchRequest struct {
	Finding           types.Finding
	FilePath          string
	FunctionName      string
	PackageName       string
	FunctionSource    string
	SurroundingSource string
}

type PatchResponse struct {
	UpdatedFunction string  `json:"updated_function"`
	Confidence      float64 `json:"confidence"`
	Rationale       string  `json:"rationale"`
}

type Client interface {
	GeneratePatch(ctx context.Context, req PatchRequest) (PatchResponse, error)
}
