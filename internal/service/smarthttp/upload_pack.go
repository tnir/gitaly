package smarthttp

import (
	"crypto/sha1"
	"fmt"
	"io"

	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"gitlab.com/gitlab-org/gitaly/internal/command"
	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
	"gitlab.com/gitlab-org/gitaly/internal/metadata/featureflag"
	"gitlab.com/gitlab-org/gitaly/internal/service/inspect"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"gitlab.com/gitlab-org/gitaly/streamio"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	deepenCount = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gitaly_smarthttp_deepen_requests_total",
			Help: "Number of git-upload-pack requests processed that contained a 'deepen' message",
		},
	)
)

func init() {
	prometheus.MustRegister(deepenCount)
}

func (s *server) PostUploadPack(stream gitalypb.SmartHTTPService_PostUploadPackServer) error {
	ctx := stream.Context()

	req, err := stream.Recv() // First request contains Repository only
	if err != nil {
		return err
	}

	if err := validateUploadPackRequest(req); err != nil {
		return err
	}

	h := sha1.New()

	stdinReader := io.TeeReader(streamio.NewReader(func() ([]byte, error) {
		resp, err := stream.Recv()

		return resp.GetData(), err
	}), h)

	pr, pw := io.Pipe()
	defer pw.Close()
	stdin := io.TeeReader(stdinReader, pw)
	deepenCh := make(chan bool, 1)
	go func() {
		deepenCh <- scanDeepen(pr)
	}()

	var respBytes int64

	stdout := streamio.NewWriter(func(p []byte) error {
		respBytes += int64(len(p))
		return stream.Send(&gitalypb.PostUploadPackResponse{Data: p})
	})

	// TODO: it is first step of the https://gitlab.com/gitlab-org/gitaly/issues/1519
	// needs to be removed after we get some statistics on this
	stdout = inspect.Write(stdout, inspect.LogPackInfoStatistic(ctx))

	env := git.AddGitProtocolEnv(ctx, req, command.GitEnv)

	repoPath, err := helper.GetRepoPath(req.Repository)
	if err != nil {
		return err
	}

	git.WarnIfTooManyBitmaps(ctx, repoPath)

	var globalOpts []git.Option
	if featureflag.IsEnabled(ctx, featureflag.UploadPackFilter) {
		globalOpts = append(globalOpts,
			git.ValueFlag{"-c", "uploadpack.allowFilter=true"},
			git.ValueFlag{"-c", "uploadpack.allowAnySHA1InWant=true"},
		)
	}

	for _, o := range req.GitConfigOptions {
		globalOpts = append(globalOpts, git.ValueFlag{"-c", o})
	}

	cmd, err := git.SafeBareCmd(ctx, stdin, stdout, nil, env, globalOpts, git.SubCmd{
		Name:  "upload-pack",
		Flags: []git.Option{git.Flag{"--stateless-rpc"}},
		Args:  []string{repoPath},
	})

	if err != nil {
		return status.Errorf(codes.Unavailable, "PostUploadPack: cmd: %v", err)
	}

	if err := cmd.Wait(); err != nil {
		pw.Close() // ensure scanDeepen returns
		if _, ok := command.ExitStatus(err); ok && <-deepenCh {
			// We have seen a 'deepen' message in the request. It is expected that
			// git-upload-pack has a non-zero exit status: don't treat this as an
			// error.
			deepenCount.Inc()
			return nil
		}

		return status.Errorf(codes.Unavailable, "PostUploadPack: %v", err)
	}

	grpc_logrus.Extract(ctx).WithField("request_sha", fmt.Sprintf("%x", h.Sum(nil))).WithField("response_bytes", respBytes).Info("request details")

	return nil
}

func validateUploadPackRequest(req *gitalypb.PostUploadPackRequest) error {
	if req.Data != nil {
		return status.Errorf(codes.InvalidArgument, "PostUploadPack: non-empty Data")
	}

	return nil
}
