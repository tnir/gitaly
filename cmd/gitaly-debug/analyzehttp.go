package main

import (
	"os"

	"gitlab.com/gitlab-org/gitaly/internal/git/stats"
)

func analyzeHTTPClone(cloneURL string, formatJSON bool) {
	st := &stats.Clone{
		URL:  cloneURL,
		JSON: formatJSON,
		Out:  os.Stdout,
	}

	noError(st.DoGet())
	noError(st.DoPost())
}
