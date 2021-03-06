// +build postgres

package praefect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/config"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/datastore"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/datastore/glsql"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/nodes"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/protoregistry"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/transactions"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper/promtest"
	"gitlab.com/gitlab-org/gitaly/internal/transaction/txinfo"
	"gitlab.com/gitlab-org/gitaly/internal/transaction/voting"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"google.golang.org/grpc/peer"
)

func getDB(t *testing.T) glsql.DB {
	return glsql.GetDB(t, "praefect")
}

func TestStreamDirectorMutator_Transaction(t *testing.T) {
	type subtransactions []struct {
		vote          string
		shouldSucceed bool
	}

	type node struct {
		primary            bool
		subtransactions    subtransactions
		shouldGetRepl      bool
		shouldParticipate  bool
		generation         int
		expectedGeneration int
	}

	testcases := []struct {
		desc         string
		primaryFails bool
		nodes        []node
	}{
		{
			desc: "successful vote should not create replication jobs",
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
				{primary: false, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
				{primary: false, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
			},
		},
		{
			desc:         "successful vote should create replication jobs if the primary fails",
			primaryFails: true,
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
				{primary: false, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: true, shouldParticipate: true, expectedGeneration: 0},
				{primary: false, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: true, shouldParticipate: true, expectedGeneration: 0},
			},
		},
		{
			desc: "failing vote should not create replication jobs without committed subtransactions",
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foo", shouldSucceed: false}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 0},
				{primary: false, subtransactions: subtransactions{{vote: "qux", shouldSucceed: false}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 0},
				{primary: false, subtransactions: subtransactions{{vote: "bar", shouldSucceed: false}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 0},
			},
		},
		{
			desc: "failing vote should create replication jobs with committed subtransaction",
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foo", shouldSucceed: true}, {vote: "foo", shouldSucceed: false}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
				{primary: false, subtransactions: subtransactions{{vote: "foo", shouldSucceed: true}, {vote: "qux", shouldSucceed: false}}, shouldGetRepl: true, shouldParticipate: true, expectedGeneration: 0},
				{primary: false, subtransactions: subtransactions{{vote: "foo", shouldSucceed: true}, {vote: "bar", shouldSucceed: false}}, shouldGetRepl: true, shouldParticipate: true, expectedGeneration: 0},
			},
		},
		{
			desc: "primary should reach quorum with disagreeing secondary",
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
				{primary: false, subtransactions: subtransactions{{vote: "barfoo", shouldSucceed: false}}, shouldGetRepl: true, shouldParticipate: true, expectedGeneration: 0},
			},
		},
		{
			desc: "quorum should create replication jobs for disagreeing node",
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
				{primary: false, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldGetRepl: false, shouldParticipate: true, expectedGeneration: 1},
				{primary: false, subtransactions: subtransactions{{vote: "barfoo", shouldSucceed: false}}, shouldGetRepl: true, shouldParticipate: true, expectedGeneration: 0},
			},
		},
		{
			desc: "only consistent secondaries should participate",
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldParticipate: true, generation: 1, expectedGeneration: 2},
				{primary: false, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldParticipate: true, generation: 1, expectedGeneration: 2},
				{shouldParticipate: false, shouldGetRepl: true, generation: 0, expectedGeneration: 0},
				{shouldParticipate: false, shouldGetRepl: true, generation: datastore.GenerationUnknown, expectedGeneration: datastore.GenerationUnknown},
			},
		},
		{
			desc: "secondaries should not participate when primary's generation is unknown",
			nodes: []node{
				{primary: true, subtransactions: subtransactions{{vote: "foobar", shouldSucceed: true}}, shouldParticipate: true, generation: datastore.GenerationUnknown, expectedGeneration: 0},
				{shouldParticipate: false, shouldGetRepl: true, generation: datastore.GenerationUnknown, expectedGeneration: datastore.GenerationUnknown},
			},
		},
		{
			// All transactional RPCs are expected to cast vote if they are successful. If they don't, something is wrong
			// and we should replicate to the secondaries to be sure.
			desc: "unstarted transaction creates replication jobs if the primary is successful",
			nodes: []node{
				{primary: true, shouldGetRepl: false, expectedGeneration: 1},
				{primary: false, shouldGetRepl: true, expectedGeneration: 0},
			},
		},
		{
			// If the RPC fails without any subtransactions, the Gitalys would not have performed any changes yet.
			// We don't have to consider the secondaries outdated.
			desc:         "unstarted transaction doesn't create replication jobs if the primary fails",
			primaryFails: true,
			nodes: []node{
				{primary: true, expectedGeneration: 0},
				{primary: false, expectedGeneration: 0},
			},
		},
		{
			// If there were no subtransactions and the RPC failed, the primary should not have performed any changes.
			// We don't need to schedule replication jobs to replication targets either as they'd have jobs
			// already scheduled by the earlier RPC that made them outdated or by the reconciler.
			desc:         "unstarted transaction should not create replication jobs for outdated node if the primary fails",
			primaryFails: true,
			nodes: []node{
				{primary: true, shouldGetRepl: false, generation: 1, expectedGeneration: 1},
				{primary: false, shouldGetRepl: false, generation: 1, expectedGeneration: 1},
				{primary: false, shouldGetRepl: false, generation: 0, expectedGeneration: 0},
				{primary: false, shouldGetRepl: false, generation: datastore.GenerationUnknown, expectedGeneration: datastore.GenerationUnknown},
			},
		},
		{
			// If there were no subtransactions and the primary did not fail, we should schedule replication jobs to every secondary.
			// All transactional RPCs are expected to vote if they are successful.
			desc: "unstarted transaction should create replication jobs for outdated node if the primary succeeds",
			nodes: []node{
				{primary: true, shouldGetRepl: false, generation: 1, expectedGeneration: 2},
				{primary: false, shouldGetRepl: true, generation: 1, expectedGeneration: 1},
				{primary: false, shouldGetRepl: true, generation: 0, expectedGeneration: 0},
				{primary: false, shouldGetRepl: true, generation: datastore.GenerationUnknown, expectedGeneration: datastore.GenerationUnknown},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			storageNodes := make([]*config.Node, 0, len(tc.nodes))
			for i := range tc.nodes {
				socket := testhelper.GetTemporaryGitalySocketFileName(t)
				testhelper.NewServerWithHealth(t, socket)
				node := &config.Node{Address: "unix://" + socket, Storage: fmt.Sprintf("node-%d", i)}
				storageNodes = append(storageNodes, node)
			}

			conf := config.Config{
				VirtualStorages: []*config.VirtualStorage{
					&config.VirtualStorage{
						Name:  "praefect",
						Nodes: storageNodes,
					},
				},
			}

			var replicationWaitGroup sync.WaitGroup
			queueInterceptor := datastore.NewReplicationEventQueueInterceptor(datastore.NewMemoryReplicationEventQueue(conf))
			queueInterceptor.OnEnqueue(func(ctx context.Context, event datastore.ReplicationEvent, queue datastore.ReplicationEventQueue) (datastore.ReplicationEvent, error) {
				defer replicationWaitGroup.Done()
				return queue.Enqueue(ctx, event)
			})

			repo := gitalypb.Repository{
				StorageName:  "praefect",
				RelativePath: "/path/to/hashed/repository",
			}

			ctx, cancel := testhelper.Context()
			defer cancel()

			nodeMgr, err := nodes.NewManager(testhelper.DiscardTestEntry(t), conf, nil, nil, promtest.NewMockHistogramVec(), protoregistry.GitalyProtoPreregistered, nil, nil)
			require.NoError(t, err)
			nodeMgr.Start(0, time.Hour)

			shard, err := nodeMgr.GetShard(ctx, conf.VirtualStorages[0].Name)
			require.NoError(t, err)

			for i := range tc.nodes {
				node, err := shard.GetNode(fmt.Sprintf("node-%d", i))
				require.NoError(t, err)
				waitNodeToChangeHealthStatus(ctx, t, node, true)
			}

			txMgr := transactions.NewManager(conf)

			// set up the generations prior to transaction
			rs := datastore.NewPostgresRepositoryStore(getDB(t), conf.StorageNames())
			for i, n := range tc.nodes {
				if n.generation == datastore.GenerationUnknown {
					continue
				}

				require.NoError(t, rs.SetGeneration(ctx, repo.StorageName, repo.RelativePath, storageNodes[i].Storage, n.generation))
			}

			coordinator := NewCoordinator(
				queueInterceptor,
				rs,
				NewNodeManagerRouter(nodeMgr, rs),
				txMgr,
				conf,
				protoregistry.GitalyProtoPreregistered,
			)

			fullMethod := "/gitaly.SmartHTTPService/PostReceivePack"

			frame, err := proto.Marshal(&gitalypb.PostReceivePackRequest{
				Repository: &repo,
			})
			require.NoError(t, err)
			peeker := &mockPeeker{frame}

			streamParams, err := coordinator.StreamDirector(ctx, fullMethod, peeker)
			require.NoError(t, err)

			txCtx := peer.NewContext(streamParams.Primary().Ctx, &peer.Peer{})
			transaction, err := txinfo.TransactionFromContext(txCtx)
			require.NoError(t, err)

			var voterWaitGroup sync.WaitGroup
			for i, node := range tc.nodes {
				if node.shouldGetRepl {
					replicationWaitGroup.Add(1)
				}

				if !node.shouldParticipate {
					continue
				}

				i := i
				node := node

				voterWaitGroup.Add(1)
				go func() {
					defer voterWaitGroup.Done()

					for _, subtransaction := range node.subtransactions {
						vote := voting.VoteFromData([]byte(subtransaction.vote))
						err := txMgr.VoteTransaction(ctx, transaction.ID, fmt.Sprintf("node-%d", i), vote)
						if subtransaction.shouldSucceed {
							if !assert.NoError(t, err) {
								break
							}
						} else {
							if !assert.True(t, errors.Is(err, transactions.ErrTransactionFailed)) {
								break
							}
						}
					}
				}()
			}
			voterWaitGroup.Wait()

			if tc.primaryFails {
				streamParams.Primary().ErrHandler(errors.New("rpc failure"))
			}

			err = streamParams.RequestFinalizer()
			require.NoError(t, err)

			// Nodes that successfully committed should have their generations incremented.
			// Nodes that did not successfully commit or did not participate should remain on their
			// existing generation.
			for i, n := range tc.nodes {
				gen, err := rs.GetGeneration(ctx, repo.StorageName, repo.RelativePath, storageNodes[i].Storage)
				require.NoError(t, err)
				require.Equal(t, n.expectedGeneration, gen, "node %d has wrong generation", i)
			}

			replicationWaitGroup.Wait()

			for i, node := range tc.nodes {
				events, err := queueInterceptor.Dequeue(ctx, "praefect", fmt.Sprintf("node-%d", i), 10)
				require.NoError(t, err)
				if node.shouldGetRepl {
					require.Len(t, events, 1)
				} else {
					require.Empty(t, events)
				}
			}
		})
	}
}
