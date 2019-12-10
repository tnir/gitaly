package info

import (
	"context"

	grpc_logrus "github.com/grpc-ecosystem/go-grpc-middleware/logging/logrus"

	"gitlab.com/gitlab-org/gitaly/internal/praefect/models"

	"gitlab.com/gitlab-org/gitaly/internal/helper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// RepositoryReplicas returns a list of repositories that includes the checksum of the primary as well as the replicas
func (s *Server) RepositoryReplicas(ctx context.Context, in *gitalypb.RepositoryReplicasRequest) (*gitalypb.RepositoryReplicasResponse, error) {
	relativePath := in.GetRepository().GetRelativePath()

	repository, err := s.datastore.GetRepository(relativePath)
	if err != nil {
		return nil, helper.ErrInternal(err)
	}

	resp, err := s.getRepositoryDetails(ctx, repository)
	if err != nil {
		return nil, helper.ErrInternal(err)
	}
	return resp, nil
}

func (s *Server) getRepositoryDetails(ctx context.Context, repository *models.Repository) (*gitalypb.RepositoryReplicasResponse, error) {
	var listRepositoriesResp gitalypb.RepositoryReplicasResponse
	g, ctx := errgroup.WithContext(ctx)
	cc, err := s.connections.GetConnection(repository.Primary.Storage)
	if err != nil {
		return nil, err
	}

	// primary
	g.Go(func() error {
		listRepositoriesResp.Primary, err = getChecksum(
			ctx,
			&gitalypb.Repository{
				StorageName:  repository.Primary.Storage,
				RelativePath: repository.RelativePath,
			}, cc)

		return err
	})

	// replicas
	listRepositoriesResp.Replicas = make([]*gitalypb.RepositoryReplicasResponse_RepositoryDetails, len(repository.Replicas))

	for i, replica := range repository.Replicas {
		i := i             // rescoping
		replica := replica // rescoping
		cc, err := s.connections.GetConnection(replica.Storage)
		if err != nil {
			return nil, err
		}

		g.Go(func() error {
			listRepositoriesResp.Replicas[i], err = getChecksum(ctx, &gitalypb.Repository{
				StorageName:  replica.Storage,
				RelativePath: repository.RelativePath,
			}, cc)

			return err
		})
	}

	if err := g.Wait(); err != nil {
		grpc_logrus.Extract(ctx).WithError(err).Error()
		return nil, err
	}

	return &listRepositoriesResp, nil
}

func getChecksum(ctx context.Context, repo *gitalypb.Repository, cc *grpc.ClientConn) (*gitalypb.RepositoryReplicasResponse_RepositoryDetails, error) {
	client := gitalypb.NewRepositoryServiceClient(cc)

	resp, err := client.CalculateChecksum(ctx,
		&gitalypb.CalculateChecksumRequest{
			Repository: repo,
		})
	if err != nil {
		return nil, err
	}

	return &gitalypb.RepositoryReplicasResponse_RepositoryDetails{
		Repository: repo,
		Checksum:   resp.GetChecksum(),
	}, nil
}
