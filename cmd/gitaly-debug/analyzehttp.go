package main

import (
	"context"
	"fmt"

	"gitlab.com/gitlab-org/gitaly/internal/git/stats"
)

func analyzeHTTPClone(cloneURL string) {
	st := &stats.Clone{
		URL:         cloneURL,
		Interactive: true,
		Record:      func(key string, value float64) { fmt.Printf("%-40s %15.5g\n", key, value) },
	}

	noError(st.Perform(context.Background()))
}
