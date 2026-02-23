package pipeline

import "context"

// Stage represents a single processing step in the pipeline.
type Stage[In any, Out any] interface {
	// Name returns a human-readable name for logging and metrics.
	Name() string

	// Execute runs the stage logic.
	Execute(ctx context.Context, in In) (Out, error)
}
