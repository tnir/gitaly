package repository

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly/internal/git/gittest"
	"gitlab.com/gitlab-org/gitaly/internal/gitaly/config"
	"gitlab.com/gitlab-org/gitaly/internal/tempdir"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/streamio"
	"google.golang.org/grpc/codes"
)

func TestSuccessfulCreateBundleRequest(t *testing.T) {
	cfg, repo, repoPath, client := setupRepositoryService(t)

	ctx, cancel := testhelper.Context()
	defer cancel()

	// Create a work tree with a HEAD pointing to a commit that is missing. CreateBundle should
	// clean this up before creating the bundle.
	sha := gittest.WriteCommit(t, cfg, repoPath, gittest.WithBranch("branch"))

	require.NoError(t, os.MkdirAll(filepath.Join(repoPath, "gitlab-worktree"), 0755))

	gittest.Exec(t, cfg, "-C", repoPath, "worktree", "add", "gitlab-worktree/worktree1", sha.String())
	require.NoError(t, os.Chtimes(filepath.Join(repoPath, "gitlab-worktree", "worktree1"), time.Now().Add(-7*time.Hour), time.Now().Add(-7*time.Hour)))

	gittest.Exec(t, cfg, "-C", repoPath, "branch", "-D", "branch")
	require.NoError(t, os.Remove(filepath.Join(repoPath, "objects", sha.String()[0:2], sha.String()[2:])))

	request := &gitalypb.CreateBundleRequest{Repository: repo}

	c, err := client.CreateBundle(ctx, request)
	require.NoError(t, err)

	reader := streamio.NewReader(func() ([]byte, error) {
		response, err := c.Recv()
		return response.GetData(), err
	})

	dstDir, err := tempdir.New(ctx, repo, config.NewLocator(cfg))
	require.NoError(t, err)
	dstFile, err := ioutil.TempFile(dstDir, "")
	require.NoError(t, err)
	defer dstFile.Close()
	defer os.RemoveAll(dstFile.Name())

	_, err = io.Copy(dstFile, reader)
	require.NoError(t, err)

	output := gittest.Exec(t, cfg, "-C", repoPath, "bundle", "verify", dstFile.Name())
	// Extra sanity; running verify should fail on bad bundles
	require.Contains(t, string(output), "The bundle records a complete history")
}

func TestFailedCreateBundleRequestDueToValidations(t *testing.T) {
	_, client := setupRepositoryServiceWithoutRepo(t)

	testCases := []struct {
		desc    string
		request *gitalypb.CreateBundleRequest
		code    codes.Code
	}{
		{
			desc:    "empty repository",
			request: &gitalypb.CreateBundleRequest{},
			code:    codes.InvalidArgument,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.desc, func(t *testing.T) {
			ctx, cancel := testhelper.Context()
			defer cancel()

			stream, err := client.CreateBundle(ctx, testCase.request)
			require.NoError(t, err)

			_, err = stream.Recv()
			require.NotEqual(t, io.EOF, err)
			testhelper.RequireGrpcError(t, err, testCase.code)
		})
	}
}
