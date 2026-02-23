package pipeline

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
)

// Run executes a single stage, recording its timing.
func Run[In any, Out any](ctx context.Context, s Stage[In, Out], in In) (Out, error) {
	start := time.Now()
	out, err := s.Execute(ctx, in)
	if pc := FromContext(ctx); pc != nil {
		pc.RecordStageTime(s.Name(), time.Since(start))
	}
	return out, err
}

// RunParallelMerge runs two stages in parallel with a merge function.
func RunParallelMerge[In any, OutA any, OutB any, Merged any](
	ctx context.Context,
	input In,
	stageA Stage[In, OutA],
	stageB Stage[In, OutB],
	merge func(OutA, OutB) Merged,
) (Merged, error) {
	var (
		outA OutA
		outB OutB
		zero Merged
	)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		outA, err = Run(ctx, stageA, input)
		return err
	})

	g.Go(func() error {
		var err error
		outB, err = Run(ctx, stageB, input)
		return err
	})

	if err := g.Wait(); err != nil {
		return zero, err
	}

	return merge(outA, outB), nil
}
