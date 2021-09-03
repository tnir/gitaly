package repocleaner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/v14/internal/helper"
	"gitlab.com/gitlab-org/gitaly/v14/internal/praefect"
	"gitlab.com/gitlab-org/gitaly/v14/internal/praefect/datastore"
	"gitlab.com/gitlab-org/gitaly/v14/proto/go/gitalypb"
)

// StateOwner performs check for the existence of the repositories.
type StateOwner interface {
	// DoesntExist returns RepositoryClusterPath for each repository that doesn't exist in the database
	// by querying repositories and storage_repositories tables.
	DoesntExist(ctx context.Context, virtualStorage, storage string, replicaPaths []string) ([]datastore.RepositoryClusterPath, error)
}

// Acquirer acquires storage for processing and no any other Acquirer can acquire it again until it is released.
type Acquirer interface {
	// Populate adds provided storage into the pool of entries to acquire.
	Populate(ctx context.Context, virtualStorage, storage string) error
	// AcquireNextStorage acquires next storage based on the inactive time.
	AcquireNextStorage(ctx context.Context, inactive, updatePeriod time.Duration) (*datastore.ClusterPath, func() error, error)
}

// Action is a procedure to be executed on the repositories that doesn't exist in praefect database.
type Action interface {
	// Perform runs actual action for non-existing repositories.
	Perform(ctx context.Context, notExisting []datastore.RepositoryClusterPath) error
}

// Runner scans healthy gitaly nodes for the repositories, verifies if
// found repositories are known by praefect and runs a special action.
type Runner struct {
	cfg           Cfg
	logger        logrus.FieldLogger
	healthChecker praefect.HealthChecker
	conns         praefect.Connections
	stateOwner    StateOwner
	acquirer      Acquirer
	action        Action
}

// Cfg contains set of configuration parameters to run Runner.
type Cfg struct {
	// RunInterval: the check runs if the previous operation was done at least RunInterval before.
	RunInterval time.Duration
	// LivenessInterval: an update runs on the locked entity with provided period to signal that entity is in use.
	LivenessInterval time.Duration
	// RepositoriesInBatch is the number of repositories to pass as a batch for processing.
	RepositoriesInBatch int
}

// NewRunner returns instance of the Runner.
func NewRunner(cfg Cfg, logger logrus.FieldLogger, healthChecker praefect.HealthChecker, conns praefect.Connections, stateOwner StateOwner, acquirer Acquirer, action Action) *Runner {
	return &Runner{
		cfg:           cfg,
		logger:        logger.WithField("component", "repocleaner.repository_existence"),
		healthChecker: healthChecker,
		conns:         conns,
		stateOwner:    stateOwner,
		acquirer:      acquirer,
		action:        action,
	}
}

// Run scans healthy gitaly nodes for the repositories, verifies if
// found repositories are known by praefect and runs a special action.
// It runs on each tick of the provided ticker and finishes with context cancellation.
func (gs *Runner) Run(ctx context.Context, ticker helper.Ticker) error {
	gs.logger.Info("started")
	defer gs.logger.Info("completed")

	defer ticker.Stop()

	for virtualStorage, connByStorage := range gs.conns {
		for storage := range connByStorage {
			if err := gs.acquirer.Populate(ctx, virtualStorage, storage); err != nil {
				return fmt.Errorf("populate database: %w", err)
			}
		}
	}

	var tick helper.Ticker
	for {
		// We use a local tick variable to run the first cycle
		// without wait. All the other iterations are waiting
		// for the next tick or context cancellation.
		if tick != nil {
			tick.Reset()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-tick.C():
			}
		} else {
			tick = ticker
		}

		gs.run(ctx)
	}
}

func (gs *Runner) run(ctx context.Context) {
	clusterPath, release, err := gs.acquirer.AcquireNextStorage(ctx, gs.cfg.RunInterval, gs.cfg.LivenessInterval)
	if err != nil {
		gs.logger.WithError(err).Error("unable to acquire next storage to verify")
		return
	}

	logger := gs.logger
	defer func() {
		if err := release(); err != nil {
			logger.WithError(err).Error("failed to release storage acquired to verify")
		}
	}()

	if clusterPath == nil {
		gs.logger.Debug("no storages to verify")
		return
	}

	logger = gs.loggerWith(clusterPath.VirtualStorage, clusterPath.Storage)
	err = gs.execOnRepositories(ctx, clusterPath.VirtualStorage, clusterPath.Storage, func(paths []datastore.RepositoryClusterPath) {
		relativePaths := make([]string, len(paths))
		for i, path := range paths {
			relativePaths[i] = path.RelativeReplicaPath
		}
		notExisting, err := gs.stateOwner.DoesntExist(ctx, clusterPath.VirtualStorage, clusterPath.Storage, relativePaths)
		if err != nil {
			logger.WithError(err).WithField("repositories", paths).Error("failed to check existence")
			return
		}

		if err := gs.action.Perform(ctx, notExisting); err != nil {
			logger.WithError(err).WithField("existence", notExisting).Error("perform action")
			return
		}
	})
	if err != nil {
		logger.WithError(err).Error("failed to exec action on repositories")
		return
	}
}

func (gs *Runner) loggerWith(virtualStorage, storage string) logrus.FieldLogger {
	return gs.logger.WithFields(logrus.Fields{"virtual_storage": virtualStorage, "storage": storage})
}

func (gs *Runner) execOnRepositories(ctx context.Context, virtualStorage, storage string, action func([]datastore.RepositoryClusterPath)) error {
	gclient, err := gs.getInternalGitalyClient(virtualStorage, storage)
	if err != nil {
		return fmt.Errorf("setup gitaly client: %w", err)
	}

	resp, err := gclient.WalkRepos(ctx, &gitalypb.WalkReposRequest{StorageName: storage})
	if err != nil {
		return fmt.Errorf("unable to walk repos: %w", err)
	}

	batch := make([]datastore.RepositoryClusterPath, 0, gs.cfg.RepositoriesInBatch)
	for {
		res, err := resp.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return fmt.Errorf("failure on walking repos: %w", err)
			}
			break
		}

		batch = append(batch, datastore.RepositoryClusterPath{
			ClusterPath: datastore.ClusterPath{
				VirtualStorage: virtualStorage,
				Storage:        storage,
			},
			RelativeReplicaPath: res.RelativePath,
		})

		if len(batch) == cap(batch) {
			action(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		action(batch)
	}
	return nil
}

func (gs *Runner) getInternalGitalyClient(virtualStorage, storage string) (gitalypb.InternalGitalyClient, error) {
	conn, found := gs.conns[virtualStorage][storage]
	if !found {
		return nil, fmt.Errorf("no connection to the gitaly node %q/%q", virtualStorage, storage)
	}
	return gitalypb.NewInternalGitalyClient(conn), nil
}
