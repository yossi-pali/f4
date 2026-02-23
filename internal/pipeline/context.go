package pipeline

import (
	"context"
	"sync"
	"time"
)

type ctxKey struct{}

// PipelineContext carries pipeline-specific data through the request.
type PipelineContext struct {
	StartTime  time.Time
	RequestID  string
	mu         sync.Mutex
	stageTimes map[string]time.Duration
}

// NewPipelineContext creates a new pipeline context.
func NewPipelineContext(requestID string) *PipelineContext {
	return &PipelineContext{
		StartTime:  time.Now(),
		RequestID:  requestID,
		stageTimes: make(map[string]time.Duration),
	}
}

// RecordStageTime records the duration of a stage.
func (pc *PipelineContext) RecordStageTime(stage string, d time.Duration) {
	pc.mu.Lock()
	pc.stageTimes[stage] = d
	pc.mu.Unlock()
}

// StageTimes returns a copy of recorded stage durations.
func (pc *PipelineContext) StageTimes() map[string]time.Duration {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	out := make(map[string]time.Duration, len(pc.stageTimes))
	for k, v := range pc.stageTimes {
		out[k] = v
	}
	return out
}

// WithPipelineContext attaches pipeline context to a context.Context.
func WithPipelineContext(ctx context.Context, pc *PipelineContext) context.Context {
	return context.WithValue(ctx, ctxKey{}, pc)
}

// FromContext retrieves the pipeline context.
func FromContext(ctx context.Context) *PipelineContext {
	pc, _ := ctx.Value(ctxKey{}).(*PipelineContext)
	return pc
}
