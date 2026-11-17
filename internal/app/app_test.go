package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"radiobuenavia/internal/dropbox"
)

func TestPipelineConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const jobs = 3
	entered := make(chan struct{}, jobs)
	release := make(chan struct{})
	var active, peak, uploaded atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- runPipeline(ctx, jobs, make([]dropbox.FileMetadata, 8),
			func(context.Context, dropbox.FileMetadata) (downloadResult, error) { return downloadResult{}, nil },
			func(ctx context.Context, result downloadResult) error {
				n := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); n > old; old = peak.Load() {
					if peak.CompareAndSwap(old, n) {
						break
					}
				}
				select {
				case entered <- struct{}{}:
				default:
				}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
			func(context.Context, downloadResult) error { uploaded.Add(1); return nil })
	}()
	for i := 0; i < jobs; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("processing workers did not overlap")
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != jobs || uploaded.Load() != 8 || active.Load() != 0 {
		t.Fatalf("peak=%d uploaded=%d active=%d", peak.Load(), uploaded.Load(), active.Load())
	}
}

func TestPipelineStopsOnStageFailure(t *testing.T) {
	for _, stage := range []string{"download", "process", "upload"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			failure := errors.New(stage + " failed")
			var active atomic.Int32
			files := []dropbox.FileMetadata{{Name: "first"}, {Name: "second"}, {Name: "third"}}
			err := runPipeline(ctx, 2, files,
				func(ctx context.Context, file dropbox.FileMetadata) (downloadResult, error) {
					if stage == "download" {
						return downloadResult{}, failure
					}
					return downloadResult{name: file.Name}, nil
				},
				func(ctx context.Context, result downloadResult) error {
					active.Add(1)
					defer active.Add(-1)
					if result.name == "first" {
						if stage == "process" {
							return failure
						}
						return nil
					}
					<-ctx.Done()
					return ctx.Err()
				},
				func(context.Context, downloadResult) error { return failure })
			if !errors.Is(err, failure) || active.Load() != 0 {
				t.Fatalf("error=%v active=%d", err, active.Load())
			}
		})
	}
}
