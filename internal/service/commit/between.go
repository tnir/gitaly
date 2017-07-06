package commit

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"

	pb "gitlab.com/gitlab-org/gitaly-proto/go"
	"gitlab.com/gitlab-org/gitaly/internal/helper"
	"gitlab.com/gitlab-org/gitaly/internal/helper/lines"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var commitLogFormatFields = []string{
	"%H",  // commit hash
	"%s",  // subject
	"%b",  // body
	"%an", // author name
	"%ae", // author email
	"%aI", // author date, strict ISO 8601 format
	"%cn", // committer name
	"%ce", // committer email
	"%cI", // committer date, strict ISO 8601 format
}

func gitLog(writer lines.Sender, repo *pb.Repository, from string, to string) error {
	repoPath, err := helper.GetRepoPath(repo)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"RepoPath": repoPath,
		"From":     from,
		"To":       to,
	}).Debug("GitLog")

	revisionRange := string(from) + ".." + string(to)
	// Use \x1f (ASCII field separator) as the field delimiter
	formatFlag := "--pretty=format:" + strings.Join(commitLogFormatFields, "%x1f")

	args := []string{
		"--git-dir",
		repoPath,
		"log",
		"-z", // use 0x00 as the entry terminator (instead of \n)
		"--reverse",
		formatFlag,
		revisionRange,
	}
	cmd, err := helper.GitCommandReader(args...)
	if err != nil {
		return err
	}
	defer cmd.Kill()

	split := lines.ScanWithDelimiter([]byte("\x00"))
	if err := lines.Send(cmd, writer, split); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		// We expect this error to be caused by non-existing references. In that
		// case, we just log the error and send no commits to the `writer`.
		log.WithFields(log.Fields{"error": err}).Info("ignoring git-log error")
	}

	return nil
}

func validateCommitsBetweenRequest(in *pb.CommitsBetweenRequest) error {
	if len(in.GetFrom()) == 0 {
		return fmt.Errorf("empty From")
	}

	if len(in.GetTo()) == 0 {
		return fmt.Errorf("empty To")
	}

	return nil
}

func (s *server) CommitsBetween(in *pb.CommitsBetweenRequest, stream pb.CommitService_CommitsBetweenServer) error {
	if err := validateCommitsBetweenRequest(in); err != nil {
		return grpc.Errorf(codes.InvalidArgument, "CommitsBetween: %v", err)
	}

	writer := newCommitsBetweenWriter(stream)

	return gitLog(writer, in.GetRepository(), string(in.GetFrom()), string(in.GetTo()))
}
