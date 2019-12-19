package hook

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"gitlab.com/gitlab-org/gitaly/streamio"

	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/gitlabshell"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

type hookRequest interface {
	GetKeyId() string
	GetRepository() *gitalypb.Repository
}

func hookRequestEnv(req hookRequest) ([]string, error) {
	return append(gitlabshell.Env(),
		fmt.Sprintf("GL_ID=%s", req.GetKeyId()),
		fmt.Sprintf("GL_REPOSITORY=%s", req.GetRepository().GetGlRepository())), nil
}

func gitlabShellHook(hookName string) string {
	return filepath.Join(config.Config.Ruby.Dir, "gitlab-shell", "hooks", hookName)
}

func (s *server) PreReceiveHook(stream gitalypb.HookService_PreReceiveHookServer) error {
	firstRequest, err := stream.Recv()
	if err != nil {
		return helper.ErrInternal(err)
	}

	if err := validatePreReceiveHookRequest(firstRequest); err != nil {
		return helper.ErrInvalidArgument(err)
	}

	hookEnv, err := hookRequestEnv(firstRequest)
	if err != nil {
		return helper.ErrInternal(err)
	}

	hookEnv = append(hookEnv, fmt.Sprintf("GL_PROTOCOL=%s", firstRequest.GetProtocol()))

	stdin := streamio.NewReader(func() ([]byte, error) {
		req, err := stream.Recv()
		return req.GetStdin(), err
	})
	stdout := streamio.NewWriter(func(p []byte) error { return stream.Send(&gitalypb.PreReceiveHookResponse{Stdout: p}) })
	stderr := streamio.NewWriter(func(p []byte) error { return stream.Send(&gitalypb.PreReceiveHookResponse{Stderr: p}) })

	repoPath, err := helper.GetRepoPath(firstRequest.GetRepository())
	if err != nil {
		return helper.ErrInternal(err)
	}

	c := exec.Command(gitlabShellHook("pre-receive"))
	c.Dir = repoPath

	success, err := streamCommandResponse(
		stream.Context(),
		stdin,
		stdout, stderr,
		c,
		hookEnv,
	)

	if err != nil {
		return helper.ErrInternal(err)
	}

	if err := stream.SendMsg(&gitalypb.PreReceiveHookResponse{
		Success: success,
	}); err != nil {
		return helper.ErrInternal(err)
	}

	return nil
}

func validatePreReceiveHookRequest(in *gitalypb.PreReceiveHookRequest) error {
	if in.GetRepository() == nil {
		return errors.New("repository is empty")
	}

	return nil
}
