package transaction

import (
	"context"
	"errors"

	"gitlab.com/gitlab-org/gitaly/internal/helper"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/transactions"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

type Server struct {
	gitalypb.UnimplementedRefTransactionServer

	txMgr *transactions.Manager
}

func NewServer(txMgr *transactions.Manager) gitalypb.RefTransactionServer {
	return &Server{
		txMgr: txMgr,
	}
}

// VoteTransaction is called by a client who's casting a vote on a reference
// transaction, blocking until a vote across all participating nodes has been
// completed.
func (s *Server) VoteTransaction(ctx context.Context, in *gitalypb.VoteTransactionRequest) (*gitalypb.VoteTransactionResponse, error) {
	err := s.txMgr.VoteTransaction(ctx, in.TransactionId, in.Node, in.ReferenceUpdatesHash)
	if err != nil {
		if errors.Is(err, transactions.ErrNotFound) {
			return nil, helper.ErrNotFound(err)
		}
		return nil, helper.ErrInternal(err)
	}

	return &gitalypb.VoteTransactionResponse{
		State: gitalypb.VoteTransactionResponse_COMMIT,
	}, nil
}
