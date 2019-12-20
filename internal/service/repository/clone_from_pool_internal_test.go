package repository_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"

	"gitlab.com/gitlab-org/gitaly/internal/git/objectpool"
	"gitlab.com/gitlab-org/gitaly/internal/helper/text"
	"gitlab.com/gitlab-org/gitaly/internal/service/repository"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

func NewTestObjectPool(t *testing.T) (*objectpool.ObjectPool, *gitalypb.Repository) {
	repo, _, relativePath := testhelper.CreateRepo(t, testhelper.GitlabTestStoragePath())

	pool, err := objectpool.NewObjectPool(repo.GetStorageName(), relativePath)
	require.NoError(t, err)

	return pool, repo
}

// getForkDestination creates a repo struct and path, but does not actually create the directory
func getForkDestination(t *testing.T) (*gitalypb.Repository, string, func()) {
	folder, err := text.RandomHex(20)
	require.NoError(t, err)
	forkRepoPath := filepath.Join(testhelper.GitlabTestStoragePath(), folder)
	forkedRepo := &gitalypb.Repository{StorageName: "default", RelativePath: folder, GlRepository: "project-1"}

	return forkedRepo, forkRepoPath, func() { os.RemoveAll(forkRepoPath) }
}

func TestCloneFromPoolInternal(t *testing.T) {
	server, serverSocketPath := runFullServer(t)
	defer server.Stop()

	ctxOuter, cancel := testhelper.Context()
	defer cancel()

	md := testhelper.GitalyServersMetadata(t, serverSocketPath)
	ctx := metadata.NewOutgoingContext(ctxOuter, md)

	client, conn := repository.NewRepositoryClient(t, serverSocketPath)
	defer conn.Close()

	testRepo, testRepoPath, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	pool, poolRepo := NewTestObjectPool(t)
	defer pool.Remove(ctx)

	require.NoError(t, pool.Create(ctx, testRepo))
	require.NoError(t, pool.Link(ctx, testRepo))

	fullRepack(t, testRepoPath)

	_, newBranch := testhelper.CreateCommitOnNewBranch(t, testRepoPath)

	forkedRepo, forkRepoPath, forkRepoCleanup := getForkDestination(t)
	defer forkRepoCleanup()

	req := &gitalypb.CloneFromPoolInternalRequest{
		Repository:       forkedRepo,
		SourceRepository: testRepo,
		Pool: &gitalypb.ObjectPool{
			Repository: poolRepo,
		},
	}

	_, err := client.CloneFromPoolInternal(ctx, req)
	require.NoError(t, err)

	assert.True(t, testhelper.GetGitObjectDirSize(t, forkRepoPath) < 100)

	isLinked, err := pool.LinkedToRepository(testRepo)
	require.NoError(t, err)
	require.True(t, isLinked)

	// feature is a branch known to exist in the source repository. By looking it up in the target
	// we establish that the target has branches, even though (as we saw above) it has no objects.
	testhelper.MustRunCommand(t, nil, "git", "-C", forkRepoPath, "show-ref", "--verify", "refs/heads/feature")
	testhelper.MustRunCommand(t, nil, "git", "-C", forkRepoPath, "show-ref", "--verify", fmt.Sprintf("refs/heads/%s", newBranch))
}

// fullRepack does a full repack on the repository, which means if it has a pool repository linked, it will get rid of redundant objects that are reachable in the pool
func fullRepack(t *testing.T, repoPath string) {
	testhelper.MustRunCommand(t, nil, "git", "-C", repoPath, "repack", "-A", "-l", "-d")
}
