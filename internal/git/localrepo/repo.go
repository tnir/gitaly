package localrepo

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitaly/internal/command"
	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/git/repository"
	"gitlab.com/gitlab-org/gitaly/internal/gitaly/config"
)

// Repo represents a local Git repository.
type Repo struct {
	repository.GitRepo
	commandFactory *git.ExecCommandFactory
	cfg            config.Cfg
}

// New creates a new Repo from its protobuf representation.
func New(repo repository.GitRepo, cfg config.Cfg) *Repo {
	return &Repo{
		GitRepo:        repo,
		cfg:            cfg,
		commandFactory: git.NewExecCommandFactory(cfg),
	}
}

// Exec creates a git command with the given args and Repo, executed in the
// Repo. It validates the arguments in the command before executing.
func (repo *Repo) Exec(ctx context.Context, globals []git.GlobalOption, cmd git.Cmd, opts ...git.CmdOpt) (*command.Command, error) {
	return repo.commandFactory.New(ctx, repo, globals, cmd, opts...)
}

// ExecAndWait is similar to Exec, but waits for the command to exit before
// returning.
func (repo *Repo) ExecAndWait(ctx context.Context, globals []git.GlobalOption, cmd git.Cmd, opts ...git.CmdOpt) error {
	command, err := repo.Exec(ctx, globals, cmd, opts...)
	if err != nil {
		return err
	}

	return command.Wait()
}

// Config returns executor of the 'config' sub-command.
func (repo *Repo) Config() Config {
	return Config{repo: repo}
}

// Remote returns executor of the 'remote' sub-command.
func (repo *Repo) Remote() Remote {
	return Remote{repo: repo}
}

func errorWithStderr(err error, stderr []byte) error {
	return fmt.Errorf("%w, stderr: %q", err, stderr)
}