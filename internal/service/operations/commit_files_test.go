package operations
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
			headerRequest := headerRequest(tc.repo, testhelper.TestUser, tc.branchName, commitFilesMessage)
			require.Equal(t, testhelper.TestUser.Name, headCommit.Committer.Name)
			require.Equal(t, testhelper.TestUser.Email, headCommit.Committer.Email)
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
			headerRequest := headerRequest(testRepo, testhelper.TestUser, branchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, testhelper.TestUser, targetBranchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, testhelper.TestUser, targetBranchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(newRepo, testhelper.TestUser, targetBranchName, commitFilesMessage)
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
			user:   &gitalypb.User{Name: []byte(".,:;<>\"'\nJane Doe.,:;<>'\"\n"), Email: []byte(".,:;<>'\"\njanedoe@gitlab.com.,:;<>'\"\n"), GlId: testhelper.GlID},
			user:   &gitalypb.User{Name: []byte("Ja<ne\n D>oe"), Email: []byte("ja<ne\ndoe>@gitlab.com"), GlId: testhelper.GlID},
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
	headerRequest := headerRequest(testRepo, testhelper.TestUser, branchName, commitFilesMessage)
	for _, hookName := range GitlabPreHooks {
			require.Contains(t, resp.PreReceiveError, "GL_ID="+testhelper.TestUser.GlId)
			require.Contains(t, resp.PreReceiveError, "GL_USERNAME="+testhelper.TestUser.GlUsername)
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
				headerRequest(testRepo, testhelper.TestUser, "feature", commitFilesMessage),
				headerRequest(testRepo, testhelper.TestUser, "feature", commitFilesMessage),
				headerRequest(testRepo, testhelper.TestUser, "utf-dir", commitFilesMessage),
	serverSocketPath, stop := runOperationServiceServer(t)
	client, conn := newOperationClient(t, serverSocketPath)
			req:  headerRequest(nil, testhelper.TestUser, branchName, commitFilesMessage),
			req:  headerRequest(testRepo, testhelper.TestUser, "", commitFilesMessage),
			req:  headerRequest(testRepo, testhelper.TestUser, branchName, nil),
			req:  setStartSha(headerRequest(testRepo, testhelper.TestUser, branchName, commitFilesMessage), "foobar"),