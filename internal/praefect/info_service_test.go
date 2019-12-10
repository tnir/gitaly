package praefect

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
)

type byStorage []*gitalypb.RepositoryReplicasResponse_RepositoryDetails

func (a byStorage) Len() int      { return len(a) }
func (a byStorage) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byStorage) Less(i, j int) bool {
	return a[i].Repository.StorageName < a[j].Repository.StorageName
}

func TestInfoService_RepositoryReplicas(t *testing.T) {
	testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	conf := testConfig(3)

	cleanup := CreateNodeStorages(t, conf.VirtualStorages[0].Nodes)
	defer cleanup()

	var primaryRepoPath string
	for _, node := range conf.VirtualStorages[0].Nodes {
		// we don't need to clean up the repos because the cleanup function from CreateNodeStorages
		// will clean up the entire temp dir
		_, destRepoPath, cleanup := cloneRepoAtStorage(t, testRepo, node.Storage)
		defer cleanup()
		if node.DefaultPrimary {
			primaryRepoPath = destRepoPath
		}
	}

	primaryRepo := *testRepo
	primaryRepo.StorageName = conf.VirtualStorages[0].Name

	ctx, cancel := testhelper.Context()
	defer cancel()

	cc, _, cleanup := runPraefectServerWithGitaly(t, conf)
	defer cleanup()

	repoClient := gitalypb.NewRepositoryServiceClient(cc)

	checksumResp, err := repoClient.CalculateChecksum(ctx, &gitalypb.CalculateChecksumRequest{Repository: &primaryRepo})
	require.NoError(t, err)
	primaryChecksum := checksumResp.GetChecksum()

	infoClient := gitalypb.NewInfoServiceClient(cc)
	resp, err := infoClient.RepositoryReplicas(ctx, &gitalypb.RepositoryReplicasRequest{
		Repository: testRepo,
	})
	require.NoError(t, err)

	require.Equal(t, &gitalypb.RepositoryReplicasResponse_RepositoryDetails{
		Repository: &gitalypb.Repository{
			StorageName:  conf.VirtualStorages[0].Nodes[0].Storage,
			RelativePath: primaryRepo.GetRelativePath(),
		},
		Checksum: primaryChecksum,
	}, resp.Primary)

	sort.Sort(byStorage(resp.Replicas))
	require.Equal(t, []*gitalypb.RepositoryReplicasResponse_RepositoryDetails{
		{
			Repository: &gitalypb.Repository{
				StorageName:  conf.VirtualStorages[0].Nodes[1].Storage,
				RelativePath: primaryRepo.GetRelativePath(),
			},
			Checksum: primaryChecksum,
		},
		{
			Repository: &gitalypb.Repository{
				StorageName:  conf.VirtualStorages[0].Nodes[2].Storage,
				RelativePath: primaryRepo.GetRelativePath(),
			},
			Checksum: primaryChecksum,
		},
	}, resp.Replicas)

	// create a commit manually on the primary repo
	testhelper.CreateCommitOnNewBranch(t, primaryRepoPath)
	newChecksumResp, err := repoClient.CalculateChecksum(ctx, &gitalypb.CalculateChecksumRequest{Repository: &primaryRepo})
	require.NoError(t, err)
	oldChecksum := primaryChecksum

	resp, err = infoClient.RepositoryReplicas(ctx, &gitalypb.RepositoryReplicasRequest{
		Repository: testRepo,
	})
	require.NoError(t, err)

	require.Equal(t, &gitalypb.RepositoryReplicasResponse_RepositoryDetails{
		Repository: &gitalypb.Repository{
			StorageName:  conf.VirtualStorages[0].Nodes[0].Storage,
			RelativePath: primaryRepo.GetRelativePath(),
		},
		Checksum: newChecksumResp.GetChecksum(),
	}, resp.Primary)

	sort.Sort(byStorage(resp.Replicas))
	require.Equal(t, []*gitalypb.RepositoryReplicasResponse_RepositoryDetails{
		{
			Repository: &gitalypb.Repository{
				StorageName:  conf.VirtualStorages[0].Nodes[1].Storage,
				RelativePath: primaryRepo.GetRelativePath(),
			},
			Checksum: oldChecksum,
		},
		{
			Repository: &gitalypb.Repository{
				StorageName:  conf.VirtualStorages[0].Nodes[2].Storage,
				RelativePath: primaryRepo.GetRelativePath(),
			},
			Checksum: oldChecksum,
		},
	}, resp.Replicas)
}
