package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"runtime"

	log "github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/backup"
	"gitlab.com/gitlab-org/gitaly/internal/storage"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

type serverRepository struct {
	storage.ServerInfo
	StorageName   string `json:"storage_name"`
	RelativePath  string `json:"relative_path"`
	GlProjectPath string `json:"gl_project_path"`
}

type createSubcommand struct {
	backupPath string
	parallel   int
}

func (cmd *createSubcommand) Flags(fs *flag.FlagSet) {
	fs.StringVar(&cmd.backupPath, "path", "", "repository backup path")
	fs.IntVar(&cmd.parallel, "parallel", runtime.NumCPU(), "maximum number of parallel backups")
}

func (cmd *createSubcommand) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	fsBackup := backup.NewFilesystem(cmd.backupPath)

	var pipeline backup.CreatePipeline
	pipeline = backup.NewPipeline(log.StandardLogger(), fsBackup)
	if cmd.parallel > 0 {
		pipeline = backup.NewParallelCreatePipeline(pipeline, cmd.parallel)
	}

	decoder := json.NewDecoder(stdin)
	for {
		var sr serverRepository
		if err := decoder.Decode(&sr); err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("create: %w", err)
		}
		repo := gitalypb.Repository{
			StorageName:   sr.StorageName,
			RelativePath:  sr.RelativePath,
			GlProjectPath: sr.GlProjectPath,
		}
		pipeline.Create(ctx, &backup.CreateRequest{
			Server:     sr.ServerInfo,
			Repository: &repo,
		})
	}

	if err := pipeline.Done(); err != nil {
		return fmt.Errorf("create: %w", err)
	}
	return nil
}
