package praefect

import (
	"context"
	"crypto/sha1"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/transactions"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func runPraefectServerAndTxMgr(t testing.TB, opts ...transactions.ManagerOpt) (*grpc.ClientConn, *transactions.Manager, testhelper.Cleanup) {
	conf := testConfig(1)
	txMgr := transactions.NewManager(opts...)
	cc, _, cleanup := runPraefectServer(t, conf, buildOptions{
		withTxMgr:   txMgr,
		withNodeMgr: nullNodeMgr{}, // to suppress node address issues
	})
	return cc, txMgr, cleanup
}

func setupMetrics() (*prometheus.CounterVec, []transactions.ManagerOpt) {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{}, []string{"status"})
	return counter, []transactions.ManagerOpt{
		transactions.WithCounterMetric(counter),
	}
}

type counterMetrics struct {
	registered, started, invalid, committed int
}

func verifyCounterMetrics(t *testing.T, counter *prometheus.CounterVec, expected counterMetrics) {
	t.Helper()

	registered, err := counter.GetMetricWithLabelValues("registered")
	require.NoError(t, err)
	require.Equal(t, float64(expected.registered), testutil.ToFloat64(registered))

	started, err := counter.GetMetricWithLabelValues("started")
	require.NoError(t, err)
	require.Equal(t, float64(expected.started), testutil.ToFloat64(started))

	invalid, err := counter.GetMetricWithLabelValues("invalid")
	require.NoError(t, err)
	require.Equal(t, float64(expected.invalid), testutil.ToFloat64(invalid))

	committed, err := counter.GetMetricWithLabelValues("committed")
	require.NoError(t, err)
	require.Equal(t, float64(expected.committed), testutil.ToFloat64(committed))
}

func TestTransactionSucceeds(t *testing.T) {
	counter, opts := setupMetrics()
	cc, txMgr, cleanup := runPraefectServerAndTxMgr(t, opts...)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := gitalypb.NewRefTransactionClient(cc)

	transactionID, cancelTransaction, err := txMgr.RegisterTransaction(ctx, []string{"node1"})
	require.NoError(t, err)
	require.NotZero(t, transactionID)
	defer cancelTransaction()

	hash := sha1.Sum([]byte{})

	response, err := client.VoteTransaction(ctx, &gitalypb.VoteTransactionRequest{
		TransactionId:        transactionID,
		Node:                 "node1",
		ReferenceUpdatesHash: hash[:],
	})
	require.NoError(t, err)
	require.Equal(t, gitalypb.VoteTransactionResponse_COMMIT, response.State)

	verifyCounterMetrics(t, counter, counterMetrics{
		registered: 1,
		started:    1,
		committed:  1,
	})
}

func TestTransactionWithMultipleNodes(t *testing.T) {
	testcases := []struct {
		desc          string
		nodes         []string
		hashes        [][20]byte
		expectedState gitalypb.VoteTransactionResponse_TransactionState
	}{
		{
			desc: "Nodes with same hash",
			nodes: []string{
				"node1",
				"node2",
			},
			hashes: [][20]byte{
				sha1.Sum([]byte{}),
				sha1.Sum([]byte{}),
			},
			expectedState: gitalypb.VoteTransactionResponse_COMMIT,
		},
		{
			desc: "Nodes with different hashes",
			nodes: []string{
				"node1",
				"node2",
			},
			hashes: [][20]byte{
				sha1.Sum([]byte("foo")),
				sha1.Sum([]byte("bar")),
			},
			expectedState: gitalypb.VoteTransactionResponse_ABORT,
		},
		{
			desc: "More nodes with same hash",
			nodes: []string{
				"node1",
				"node2",
				"node3",
				"node4",
			},
			hashes: [][20]byte{
				sha1.Sum([]byte("foo")),
				sha1.Sum([]byte("foo")),
				sha1.Sum([]byte("foo")),
				sha1.Sum([]byte("foo")),
			},
			expectedState: gitalypb.VoteTransactionResponse_COMMIT,
		},
		{
			desc: "Majority with same hash",
			nodes: []string{
				"node1",
				"node2",
				"node3",
				"node4",
			},
			hashes: [][20]byte{
				sha1.Sum([]byte("foo")),
				sha1.Sum([]byte("foo")),
				sha1.Sum([]byte("bar")),
				sha1.Sum([]byte("foo")),
			},
			expectedState: gitalypb.VoteTransactionResponse_ABORT,
		},
	}

	cc, txMgr, cleanup := runPraefectServerAndTxMgr(t)
	defer cleanup()

	ctx, cleanup := testhelper.Context()
	defer cleanup()

	client := gitalypb.NewRefTransactionClient(cc)

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			transactionID, cancelTransaction, err := txMgr.RegisterTransaction(ctx, tc.nodes)
			require.NoError(t, err)
			defer cancelTransaction()

			var wg sync.WaitGroup
			for i := 0; i < len(tc.nodes); i++ {
				wg.Add(1)

				go func(idx int) {
					defer wg.Done()

					response, err := client.VoteTransaction(ctx, &gitalypb.VoteTransactionRequest{
						TransactionId:        transactionID,
						Node:                 tc.nodes[idx],
						ReferenceUpdatesHash: tc.hashes[idx][:],
					})
					require.NoError(t, err)
					require.Equal(t, tc.expectedState, response.State)
				}(i)
			}

			wg.Wait()
		})
	}
}

func TestTransactionWithContextCancellation(t *testing.T) {
	cc, txMgr, cleanup := runPraefectServerAndTxMgr(t)
	defer cleanup()

	client := gitalypb.NewRefTransactionClient(cc)

	ctx, cancel := testhelper.Context()

	transactionID, cancelTransaction, err := txMgr.RegisterTransaction(ctx, []string{"voter", "absent"})
	require.NoError(t, err)
	defer cancelTransaction()

	hash := sha1.Sum([]byte{})

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := client.VoteTransaction(ctx, &gitalypb.VoteTransactionRequest{
			TransactionId:        transactionID,
			Node:                 "voter",
			ReferenceUpdatesHash: hash[:],
		})
		require.Error(t, err)
		require.Equal(t, codes.Canceled, status.Code(err))
	}()

	cancel()
	wg.Wait()
}

func TestTransactionRegistrationWithInvalidNodesFails(t *testing.T) {
	ctx, cleanup := testhelper.Context()
	defer cleanup()

	txMgr := transactions.NewManager()

	_, _, err := txMgr.RegisterTransaction(ctx, []string{})
	require.Equal(t, transactions.ErrMissingNodes, err)

	_, _, err = txMgr.RegisterTransaction(ctx, []string{"node1", "node2", "node1"})
	require.Equal(t, transactions.ErrDuplicateNodes, err)
}

func TestTransactionRegistrationWithSameNodeFails(t *testing.T) {
	ctx, cleanup := testhelper.Context()
	defer cleanup()

	txMgr := transactions.NewManager()

	_, _, err := txMgr.RegisterTransaction(ctx, []string{"foo", "bar", "foo"})
	require.Error(t, err)
}

func TestTransactionFailures(t *testing.T) {
	counter, opts := setupMetrics()
	cc, _, cleanup := runPraefectServerAndTxMgr(t, opts...)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := gitalypb.NewRefTransactionClient(cc)

	hash := sha1.Sum([]byte{})
	_, err := client.VoteTransaction(ctx, &gitalypb.VoteTransactionRequest{
		TransactionId:        1,
		Node:                 "node1",
		ReferenceUpdatesHash: hash[:],
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))

	verifyCounterMetrics(t, counter, counterMetrics{
		started: 1,
		invalid: 1,
	})
}

func TestTransactionCancellation(t *testing.T) {
	counter, opts := setupMetrics()
	cc, txMgr, cleanup := runPraefectServerAndTxMgr(t, opts...)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := gitalypb.NewRefTransactionClient(cc)

	transactionID, cancelTransaction, err := txMgr.RegisterTransaction(ctx, []string{"node1"})
	require.NoError(t, err)
	require.NotZero(t, transactionID)

	cancelTransaction()

	hash := sha1.Sum([]byte{})
	_, err = client.VoteTransaction(ctx, &gitalypb.VoteTransactionRequest{
		TransactionId:        transactionID,
		Node:                 "node1",
		ReferenceUpdatesHash: hash[:],
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))

	verifyCounterMetrics(t, counter, counterMetrics{
		registered: 1,
		started:    1,
		invalid:    1,
	})
}