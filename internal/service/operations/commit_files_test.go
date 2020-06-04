package operations_test
	"gitlab.com/gitlab-org/gitaly/internal/service/operations"
	user = &gitalypb.User{
		Name:  []byte("John Doe"),
		Email: []byte("johndoe@gitlab.com"),
		GlId:  "user-1",
	}
	cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
	defer cleanupSrv()

	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
			headerRequest := headerRequest(tc.repo, user, tc.branchName, commitFilesMessage)
			require.Equal(t, user.Name, headCommit.Committer.Name)
			require.Equal(t, user.Email, headCommit.Committer.Email)
	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
			cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
			defer cleanupSrv()

			headerRequest := headerRequest(testRepo, user, branchName, commitFilesMessage)
	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
	cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
	defer cleanupSrv()

	headerRequest := headerRequest(testRepo, user, targetBranchName, commitFilesMessage)
	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, user, targetBranchName, commitFilesMessage)
	cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
	defer cleanupSrv()

	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(newRepo, user, targetBranchName, commitFilesMessage)
	cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
	defer cleanupSrv()

	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
	glID := "key-123"

	cleanupSrv := operations.SetupAndStartGitlabServer(t, glID, testRepo.GlRepository)
	defer cleanupSrv()

			user:   &gitalypb.User{Name: []byte(".,:;<>\"'\nJane Doe.,:;<>'\"\n"), Email: []byte(".,:;<>'\"\njanedoe@gitlab.com.,:;<>'\"\n"), GlId: glID},
			user:   &gitalypb.User{Name: []byte("Ja<ne\n D>oe"), Email: []byte("ja<ne\ndoe>@gitlab.com"), GlId: glID},
	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, user, branchName, commitFilesMessage)
	cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
	defer cleanupSrv()

	for _, hookName := range operations.GitlabPreHooks {
			require.Contains(t, resp.PreReceiveError, "GL_ID="+user.GlId)
			require.Contains(t, resp.PreReceiveError, "GL_USERNAME="+user.GlUsername)
	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
	cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
	defer cleanupSrv()

				headerRequest(testRepo, user, "feature", commitFilesMessage),
				headerRequest(testRepo, user, "feature", commitFilesMessage),
				headerRequest(testRepo, user, "utf-dir", commitFilesMessage),
	serverSocketPath, stop := operations.RunOperationServiceServer(t)
	client, conn := operations.NewOperationClient(t, serverSocketPath)
	cleanupSrv := operations.SetupAndStartGitlabServer(t, user.GlId, testRepo.GlRepository)
	defer cleanupSrv()

			req:  headerRequest(nil, user, branchName, commitFilesMessage),
			req:  headerRequest(testRepo, user, "", commitFilesMessage),
			req:  headerRequest(testRepo, user, branchName, nil),
			req:  setStartSha(headerRequest(testRepo, user, branchName, commitFilesMessage), "foobar"),