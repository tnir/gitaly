package repository

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"

	"gitlab.com/gitlab-org/gitaly/internal/git"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *server) CreateRepositoryFromURL(ctx context.Context, req *gitalypb.CreateRepositoryFromURLRequest) (*gitalypb.CreateRepositoryFromURLResponse, error) {
	if err := validateCreateRepositoryFromURLRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CreateRepositoryFromURL: %v", err)
	}

	repository := req.Repository

	repositoryFullPath, err := helper.GetPath(repository)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(repositoryFullPath); !os.IsNotExist(err) {
		return nil, status.Errorf(codes.InvalidArgument, "CreateRepositoryFromURL: dest dir exists")
	}

	u, err := url.Parse(req.GetUrl())
	if err != nil {
		return nil, helper.ErrInternal(err)
	}

	flags := []git.Option{git.Flag{Name: "--bare"}, git.ValueFlag{Name: "-c", Value: "http.followRedirects=false"}}
	if u.User != nil {
		userInfo := *u.User
		u.User = nil
		authHeader := fmt.Sprintf("Authorization: Basic %s", base64.StdEncoding.EncodeToString([]byte(userInfo.String())))
		flags = append(flags, git.ValueFlag{Name: "-c", Value: fmt.Sprintf("http.%s.extraHeader=%s", u.String(), authHeader)})
	}

	var stderr, stdout bytes.Buffer

	cmd, err := git.SafeBareCmd(ctx, nil, &stdout, &stderr, nil, nil, git.SubCmd{
		Name:        "clone",
		Flags:       flags,
		PostSepArgs: []string{u.String(), repositoryFullPath},
	})

	if err != nil {
		return nil, helper.ErrInternal(err)
	}

	if err := cmd.Wait(); err != nil {
		os.RemoveAll(repositoryFullPath)
		return nil, status.Errorf(codes.Internal, "CreateRepositoryFromURL: clone cmd wait: %v", err)
	}

	// CreateRepository is harmless on existing repositories with the side effect that it creates the hook symlink.
	if _, err := s.CreateRepository(ctx, &gitalypb.CreateRepositoryRequest{Repository: repository}); err != nil {
		return nil, status.Errorf(codes.Internal, "CreateRepositoryFromURL: create hooks failed: %v", err)
	}

	if err := removeOriginInRepo(ctx, repository); err != nil {
		return nil, status.Errorf(codes.Internal, "CreateRepositoryFromURL: %v", err)
	}

	return &gitalypb.CreateRepositoryFromURLResponse{}, nil
}

func validateCreateRepositoryFromURLRequest(req *gitalypb.CreateRepositoryFromURLRequest) error {
	if req.GetRepository() == nil {
		return fmt.Errorf("empty Repository")
	}

	if req.GetUrl() == "" {
		return fmt.Errorf("empty Url")
	}

	return nil
}
