package ref

import (
	"bytes"
	"net"
	"os"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"
	log "github.com/sirupsen/logrus"

	"github.com/stretchr/testify/require"
	pb "gitlab.com/gitlab-org/gitaly-proto/go"
	"gitlab.com/gitlab-org/gitaly/internal/helper/lines"
	"gitlab.com/gitlab-org/gitaly/internal/rubyserver"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	localBranches = map[string]*pb.GitCommit{
		"refs/heads/100%branch": {
			Id:        "1b12f15a11fc6e62177bef08f47bc7b5ce50b141",
			Body:      []byte("Merge branch 'add-directory-with-space' into 'master'\r\n\r\nAdd a directory containing a space in its name\r\n\r\nneeded for verifying the fix of `https://gitlab.com/gitlab-com/support-forum/issues/952` \r\n\r\nSee merge request !11"),
			BodySize:  221,
			ParentIds: []string{"6907208d755b60ebeacb2e9dfea74c92c3449a1f", "38008cb17ce1466d8fec2dfa6f6ab8dcfe5cf49e"},
			Subject:   []byte("Merge branch 'add-directory-with-space' into 'master'"),
			Author: &pb.CommitAuthor{
				Name:  []byte("Stan Hu"),
				Email: []byte("stanhu@gmail.com"),
				Date:  &timestamp.Timestamp{Seconds: 1471558878},
			},
			Committer: &pb.CommitAuthor{
				Name:  []byte("Stan Hu"),
				Email: []byte("stanhu@gmail.com"),
				Date:  &timestamp.Timestamp{Seconds: 1471558878},
			},
		},
		"refs/heads/improve/awesome": {
			Id:        "5937ac0a7beb003549fc5fd26fc247adbce4a52e",
			Subject:   []byte("Add submodule from gitlab.com"),
			Body:      []byte("Add submodule from gitlab.com\n\nSigned-off-by: Dmitriy Zaporozhets <dmitriy.zaporozhets@gmail.com>\n"),
			BodySize:  98,
			ParentIds: []string{"570e7b2abdd848b95f2f578043fc23bd6f6fd24d"},
			Author: &pb.CommitAuthor{
				Name:  []byte("Dmitriy Zaporozhets"),
				Email: []byte("dmitriy.zaporozhets@gmail.com"),
				Date:  &timestamp.Timestamp{Seconds: 1393491698},
			},
			Committer: &pb.CommitAuthor{
				Name:  []byte("Dmitriy Zaporozhets"),
				Email: []byte("dmitriy.zaporozhets@gmail.com"),
				Date:  &timestamp.Timestamp{Seconds: 1393491698},
			},
		},
		"refs/heads/'test'": {
			Id:        "e56497bb5f03a90a51293fc6d516788730953899",
			Subject:   []byte("Merge branch 'tree_helper_spec' into 'master'"),
			Body:      []byte("Merge branch 'tree_helper_spec' into 'master'\n\nAdd directory structure for tree_helper spec\n\nThis directory structure is needed for a testing the method flatten_tree(tree) in the TreeHelper module\n\nSee [merge request #275](https://gitlab.com/gitlab-org/gitlab-ce/merge_requests/275#note_732774)\n\nSee merge request !2\n"),
			BodySize:  317,
			ParentIds: []string{"5937ac0a7beb003549fc5fd26fc247adbce4a52e", "4cd80ccab63c82b4bad16faa5193fbd2aa06df40"},
			Author: &pb.CommitAuthor{
				Name:  []byte("Sytse Sijbrandij"),
				Email: []byte("sytse@gitlab.com"),
				Date:  &timestamp.Timestamp{Seconds: 1420925009},
			},
			Committer: &pb.CommitAuthor{
				Name:  []byte("Sytse Sijbrandij"),
				Email: []byte("sytse@gitlab.com"),
				Date:  &timestamp.Timestamp{Seconds: 1420925009},
			},
		},
	}
)

func TestMain(m *testing.M) {
	os.Exit(testMain(m))
}

var rubyServer *rubyserver.Server

func testMain(m *testing.M) int {
	defer testhelper.MustHaveNoChildProcess()

	var err error

	testhelper.ConfigureRuby()
	rubyServer, err = rubyserver.Start()
	if err != nil {
		log.Fatal(err)
	}
	defer rubyServer.Stop()

	// Use 100 bytes as the maximum message size to test that fragmenting the
	// ref list works correctly
	lines.MaxMsgSize = 100

	return m.Run()
}

func runRefServiceServer(t *testing.T) (*grpc.Server, string) {
	serverSocketPath := testhelper.GetTemporaryGitalySocketFileName()
	grpcServer := testhelper.NewTestGrpcServer(t, nil, nil)

	listener, err := net.Listen("unix", serverSocketPath)
	if err != nil {
		t.Fatal(err)
	}

	pb.RegisterRefServiceServer(grpcServer, &server{rubyServer})
	reflection.Register(grpcServer)

	go grpcServer.Serve(listener)

	return grpcServer, serverSocketPath
}

func newRefServiceClient(t *testing.T, serverSocketPath string) (pb.RefServiceClient, *grpc.ClientConn) {
	connOpts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	}
	conn, err := grpc.Dial(serverSocketPath, connOpts...)
	if err != nil {
		t.Fatal(err)
	}

	return pb.NewRefServiceClient(conn), conn
}

func assertContainsLocalBranch(t *testing.T, branches []*pb.FindLocalBranchResponse, branch *pb.FindLocalBranchResponse) {
	for _, b := range branches {
		if bytes.Equal(branch.Name, b.Name) {
			if !testhelper.FindLocalBranchResponsesEqual(branch, b) {
				t.Errorf("Expected branch\n%v\ngot\n%v", branch, b)
			}
			return // Found the branch and it maches. Success!
		}
	}
	t.Errorf("Expected to find branch %q in local branches", branch.Name)
}

func assertContainsBranch(t *testing.T, branches []*pb.FindAllBranchesResponse_Branch, branch *pb.FindAllBranchesResponse_Branch) {
	var branchNames [][]byte

	for _, b := range branches {
		if bytes.Equal(branch.Name, b.Name) {
			require.Equal(t, b.Target, branch.Target, "mismatched targets")
			return // Found the branch and it maches. Success!
		}
		branchNames = append(branchNames, b.Name)
	}

	t.Errorf("Expected to find branch %q in branches %s", branch.Name, branchNames)
}
