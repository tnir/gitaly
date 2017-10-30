package wiki

import (
	"gitlab.com/gitlab-org/gitaly/internal/rubyserver"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func (s *server) WikiGetPageVersions(request *pb.WikiGetPageVersionsRequest, stream pb.WikiService_WikiGetPageVersionsServer) error {
	ctx := stream.Context()

	if len(request.GetPagePath()) == 0 {
		return grpc.Errorf(codes.InvalidArgument, "WikiGetPageVersions: Empty Path")
	}

	client, err := s.WikiServiceClient(ctx)
	if err != nil {
		return err
	}

	clientCtx, err := rubyserver.SetHeaders(ctx, request.GetRepository())
	if err != nil {
		return err
	}

	rubyStream, err := client.WikiGetPageVersions(clientCtx, request)
	if err != nil {
		return err
	}

	return rubyserver.Proxy(func() error {
		resp, err := rubyStream.Recv()
		if err != nil {
			md := rubyStream.Trailer()
			stream.SetTrailer(md)
			return err
		}
		return stream.Send(resp)
	})
}
