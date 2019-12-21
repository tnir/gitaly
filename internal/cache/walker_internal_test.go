package cache

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanWalkEmptyDirs(t *testing.T) {
	tmp, err := ioutil.TempDir("", t.Name())
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(tmp)) }()

	for _, tt := range []struct {
		path  string
		stale bool
	}{
		{path: "a/b/c/"},
		{path: "a/b/c/1", stale: true},
		{path: "a/b/c/2", stale: true},
		{path: "a/b/d/"},
		{path: "e/"},
		{path: "e/1"},
		{path: "f/"},
	} {
		p := filepath.Join(tmp, tt.path)
		if strings.HasSuffix(tt.path, "/") {
			require.NoError(t, os.MkdirAll(p, 0755))
		} else {
			f, err := os.Create(p)
			require.NoError(t, err)
			require.NoError(t, f.Close())
		}

		if tt.stale {
			require.NoError(t, os.Chtimes(p, time.Now(), time.Now().Add(-1*time.Hour)))
		}
	}

	require.NoError(t, cleanWalk(tmp))

	actual := findFiles(t, tmp)
	expect := `.
./e
./e/1
`
	require.Equal(t, expect, actual)
}

func findFiles(t testing.TB, path string) string {
	cmd := exec.Command("find", ".")
	cmd.Dir = path
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(out)
}
