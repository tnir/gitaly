package operations
	"context"
	"gitlab.com/gitlab-org/gitaly/internal/metadata/featureflag"
func testSuccessfulUserCommitFilesRequest(t *testing.T, ctxWithFeatureFlags context.Context) {
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
			ctx := metadata.NewOutgoingContext(ctxWithFeatureFlags, md)
			headerRequest := headerRequest(tc.repo, testhelper.TestUser, tc.branchName, commitFilesMessage)
			headCommit, err := log.GetCommit(ctxWithFeatureFlags, tc.repo, tc.branchName)
			require.Equal(t, testhelper.TestUser.Name, headCommit.Committer.Name)
			require.Equal(t, testhelper.TestUser.Email, headCommit.Committer.Email)
func TestSuccessfulUserCommitFilesRequest(t *testing.T) {
	featureSet, err := testhelper.NewFeatureSets(nil, featureflag.GitalyRubyCallHookRPC, featureflag.GoUpdateHook)
	require.NoError(t, err)
	ctx, cancel := testhelper.Context()
	defer cancel()

	for _, features := range featureSet {
		t.Run(features.String(), func(t *testing.T) {
			ctx = features.WithParent(ctx)
			testSuccessfulUserCommitFilesRequest(t, ctx)
		})
	}
}

	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
			headerRequest := headerRequest(testRepo, testhelper.TestUser, branchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, testhelper.TestUser, targetBranchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, testhelper.TestUser, targetBranchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(newRepo, testhelper.TestUser, targetBranchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
			user:   &gitalypb.User{Name: []byte(".,:;<>\"'\nJane Doe.,:;<>'\"\n"), Email: []byte(".,:;<>'\"\njanedoe@gitlab.com.,:;<>'\"\n"), GlId: testhelper.GlID},
			user:   &gitalypb.User{Name: []byte("Ja<ne\n D>oe"), Email: []byte("ja<ne\ndoe>@gitlab.com"), GlId: testhelper.GlID},
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, testhelper.TestUser, branchName, commitFilesMessage)
	for _, hookName := range GitlabPreHooks {
			require.Contains(t, resp.PreReceiveError, "GL_ID="+testhelper.TestUser.GlId)
			require.Contains(t, resp.PreReceiveError, "GL_USERNAME="+testhelper.TestUser.GlUsername)
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
				headerRequest(testRepo, testhelper.TestUser, "feature", commitFilesMessage),
				headerRequest(testRepo, testhelper.TestUser, "feature", commitFilesMessage),
				headerRequest(testRepo, testhelper.TestUser, "utf-dir", commitFilesMessage),
	serverSocketPath, stop := runOperationServiceServer(t)
	defer stop()
	client, conn := newOperationClient(t, serverSocketPath)
			req:  headerRequest(nil, testhelper.TestUser, branchName, commitFilesMessage),
			req:  headerRequest(testRepo, testhelper.TestUser, "", commitFilesMessage),
			req:  headerRequest(testRepo, testhelper.TestUser, branchName, nil),
			req:  setStartSha(headerRequest(testRepo, testhelper.TestUser, branchName, commitFilesMessage), "foobar"),