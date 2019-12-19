package hook

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"gitlab.com/gitlab-org/gitaly/streamio"

	"github.com/stretchr/testify/require"

	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/testhelper"
	"gitlab.com/gitlab-org/gitaly/proto/go/gitalypb"
	"google.golang.org/grpc/codes"
)

func TestPreReceiveInvalidArgument(t *testing.T) {
	server, serverSocketPath := runHooksServer(t)
	defer server.Stop()

	client, conn := newHooksClient(t, serverSocketPath)
	defer conn.Close()

	ctx, cancel := testhelper.Context()
	defer cancel()

	stream, err := client.PreReceiveHook(ctx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&gitalypb.PreReceiveHookRequest{}))
	_, err = stream.Recv()

	testhelper.RequireGrpcError(t, err, codes.InvalidArgument)
}

func TestPreReceive(t *testing.T) {
	rubyDir := config.Config.Ruby.Dir
	defer func(rubyDir string) {
		config.Config.Ruby.Dir = rubyDir
	}(rubyDir)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	config.Config.Ruby.Dir = filepath.Join(cwd, "testdata")

	server, serverSocketPath := runHooksServer(t)
	defer server.Stop()

	testRepo, _, cleanupFn := testhelper.NewTestRepo(t)
	defer cleanupFn()

	client, conn := newHooksClient(t, serverSocketPath)
	defer conn.Close()

	ctx, cancel := testhelper.Context()
	defer cancel()

	testCases := []struct {
		desc           string
		stdin          io.Reader
		req            gitalypb.PreReceiveHookRequest
		success        bool
		stdout, stderr string
	}{
		{
			desc:    "valid stdin",
			stdin:   bytes.NewBufferString("a\nb\nc\nd\ne\nf\ng"),
			req:     gitalypb.PreReceiveHookRequest{Repository: testRepo, KeyId: "key_id", Protocol: "protocol"},
			success: true,
			stdout:  "OK",
			stderr:  "",
		},
		{
			desc:    "missing stdin",
			stdin:   bytes.NewBuffer(nil),
			req:     gitalypb.PreReceiveHookRequest{Repository: testRepo, KeyId: "key_id", Protocol: "protocol"},
			success: false,
			stdout:  "",
			stderr:  "FAIL",
		},
		{
			desc:    "missing protocol",
			stdin:   bytes.NewBufferString("a\nb\nc\nd\ne\nf\ng"),
			req:     gitalypb.PreReceiveHookRequest{Repository: testRepo, KeyId: "key_id"},
			success: false,
			stdout:  "",
			stderr:  "FAIL",
		},
		{
			desc:    "missing key_id",
			stdin:   bytes.NewBufferString("a\nb\nc\nd\ne\nf\ng"),
			req:     gitalypb.PreReceiveHookRequest{Repository: testRepo, Protocol: "protocol"},
			success: false,
			stdout:  "",
			stderr:  "FAIL",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			stream, err := client.PreReceiveHook(ctx)
			require.NoError(t, err)
			require.NoError(t, stream.Send(&tc.req))

			go func() {
				writer := streamio.NewWriter(func(p []byte) error {
					return stream.Send(&gitalypb.PreReceiveHookRequest{Stdin: p})
				})
				_, err := io.Copy(writer, tc.stdin)
				require.NoError(t, err)
				require.NoError(t, stream.CloseSend(), "close send")
			}()

			var success bool
			var stdout, stderr bytes.Buffer
			for {
				resp, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Errorf("error when receiving stream: %v", err)
				}

				_, err = stdout.Write(resp.GetStdout())
				require.NoError(t, err)
				_, err = stderr.Write(resp.GetStderr())
				require.NoError(t, err)

				success = resp.GetSuccess()
				require.NoError(t, err)
			}

			require.Equal(t, tc.success, success)
			assert.Equal(t, tc.stderr, string(bytes.TrimRight(stderr.Bytes(), "\n")))
			assert.Equal(t, tc.stdout, string(bytes.TrimRight(stdout.Bytes(), "\n")))
		})
	}
}
