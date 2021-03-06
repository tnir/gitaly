package backup

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

// Strategy used to create/restore backups
type Strategy interface {
	Create(context.Context, *CreateRequest) error
	Restore(context.Context, *RestoreRequest) error
}

// CreatePipeline is a pipeline that only handles creating backups
type CreatePipeline interface {
	Create(context.Context, *CreateRequest)
	Done() error
}

// Pipeline handles a series of requests to create/restore backups. Pipeline
// encapsulates error handling for the caller.
type Pipeline struct {
	log      logrus.FieldLogger
	strategy Strategy
	failed   int64
}

// NewPipeline creates a new pipeline
func NewPipeline(log logrus.FieldLogger, strategy Strategy) *Pipeline {
	return &Pipeline{
		log:      log,
		strategy: strategy,
	}
}

// Create requests that a repository backup be created
func (p *Pipeline) Create(ctx context.Context, req *CreateRequest) {
	repoLog := p.repoLogger(req.Repository)
	repoLog.Info("started backup")

	if err := p.strategy.Create(ctx, req); err != nil {
		if errors.Is(err, ErrSkipped) {
			repoLog.WithError(err).Warn("skipped backup")
		} else {
			repoLog.WithError(err).Error("backup failed")
			atomic.AddInt64(&p.failed, 1)
		}
		return
	}

	repoLog.Info("completed backup")
}

// Restore requests that a repository be restored from backup
func (p *Pipeline) Restore(ctx context.Context, req *RestoreRequest) {
	repoLog := p.repoLogger(req.Repository)
	repoLog.Info("started restore")

	if err := p.strategy.Restore(ctx, req); err != nil {
		if errors.Is(err, ErrSkipped) {
			repoLog.WithError(err).Warn("skipped restore")
		} else {
			repoLog.WithError(err).Error("restore failed")
			atomic.AddInt64(&p.failed, 1)
		}
		return
	}

	repoLog.Info("completed restore")
}

// Done indicates that the pipeline is complete and returns any accumulated errors
func (p *Pipeline) Done() error {
	if p.failed > 0 {
		return fmt.Errorf("pipeline: %d failures encountered", p.failed)
	}
	return nil
}

func (p *Pipeline) repoLogger(repo *gitalypb.Repository) logrus.FieldLogger {
	return p.log.WithFields(logrus.Fields{
		"storage_name":    repo.StorageName,
		"relative_path":   repo.RelativePath,
		"gl_project_path": repo.GlProjectPath,
	})
}

// ParallelCreatePipeline is a pipeline that creates backups in parallel
type ParallelCreatePipeline struct {
	next CreatePipeline
	n    int

	workersOnce sync.Once
	wg          sync.WaitGroup
	done        chan struct{}
	requests    chan *CreateRequest

	mu  sync.Mutex
	err error
}

// NewParallelCreatePipeline creates a new ParallelCreatePipeline where `next`
// is the pipeline called to create the backups and `n` is the number of
// parallel backups that will run.
func NewParallelCreatePipeline(next CreatePipeline, n int) *ParallelCreatePipeline {
	return &ParallelCreatePipeline{
		next:     next,
		n:        n,
		done:     make(chan struct{}),
		requests: make(chan *CreateRequest),
	}
}

// Create queues a call to `next.Create` which will be run in parallel
func (p *ParallelCreatePipeline) Create(ctx context.Context, req *CreateRequest) {
	p.workersOnce.Do(p.startWorkers)

	select {
	case <-ctx.Done():
		p.setErr(ctx.Err())
	case p.requests <- req:
	}
}

// Done waits for any in progress calls to `Create` to complete then reports any accumulated errors
func (p *ParallelCreatePipeline) Done() error {
	close(p.done)
	p.wg.Wait()
	if err := p.next.Done(); err != nil {
		return err
	}
	return p.err
}

func (p *ParallelCreatePipeline) startWorkers() {
	for i := 0; i < p.n; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

func (p *ParallelCreatePipeline) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.done:
			return
		case req := <-p.requests:
			p.next.Create(context.TODO(), req)
		}
	}
}

func (p *ParallelCreatePipeline) setErr(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return
	}
	p.err = err
}
