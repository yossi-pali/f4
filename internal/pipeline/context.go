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

// RecordSubStageTime records the duration of a sub-operation within a stage.
// Uses dotted key format: "stage_name.operation".
func (pc *PipelineContext) RecordSubStageTime(stage, operation string, d time.Duration) {
	key := stage + "." + operation
	pc.mu.Lock()
	pc.stageTimes[key] = d
	pc.mu.Unlock()
}

// Timer is a convenience helper for recording sub-stage durations.
type Timer struct {
	pc        *PipelineContext
	stage     string
	operation string
	start     time.Time
}

// StartTimer begins timing a sub-stage operation.
// Safe to call on nil receiver — returns a no-op timer.
func (pc *PipelineContext) StartTimer(stage, operation string) *Timer {
	return &Timer{
		pc:        pc,
		stage:     stage,
		operation: operation,
		start:     time.Now(),
	}
}

// Stop records the elapsed time and returns the duration.
// Safe to call when the Timer's PipelineContext is nil (no-op).
func (t *Timer) Stop() time.Duration {
	d := time.Since(t.start)
	if t.pc != nil {
		t.pc.RecordSubStageTime(t.stage, t.operation, d)
	}
	return d
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
