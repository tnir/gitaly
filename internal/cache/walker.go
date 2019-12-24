// Package cache supplies background workers for periodically cleaning the
// cache folder on all storages listed in the config file. Upon configuration
// validation, one worker will be started for each storage. The worker will
// walk the cache directory tree and remove any files older than one hour. The
// worker will walk the cache directory every ten minutes.
package cache

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.com/gitlab-org/gitaly/internal/config"
	"gitlab.com/gitlab-org/gitaly/internal/dontpanic"
	"gitlab.com/gitlab-org/gitaly/internal/log"
	"gitlab.com/gitlab-org/gitaly/internal/tempdir"
)

type walkFunc func(path string, info os.FileInfo, err error, dirEmpty bool) error

func walker(path string, info os.FileInfo, err error, dirEmpty bool) error {
	// To reduce pressure, sleep after each walk
	defer time.Sleep(100 * time.Microsecond)

	if err != nil {
		return err
	}

	countWalkCheck()

	if info.IsDir() {
		if !dirEmpty {
			return nil
		}

		if err := os.Remove(path); err != nil {
			// this is a potential race condition where
			// another walker may have already removed this
			// directory or added a file to it
			log.Default().
				WithField("path", path).
				WithError(err).
				Warn("unable to remove empty dir")
			return nil
		}

		countWalkRemoval()

		return nil
	}

	threshold := time.Now().Add(-1 * staleAge)
	if info.ModTime().After(threshold) {
		return nil
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			// race condition: another file walker on the
			// same storage may have deleted the file already
			return nil
		}

		return err
	}

	countWalkRemoval()

	return nil
}

func cleanWalk(walkPath string) error {
	walkErr := walkRoot(walkPath, walker)

	if os.IsNotExist(walkErr) {
		return nil
	}

	return walkErr
}

// walkRoot is a modified version of https://golang.org/pkg/path/filepath/#Walk
func walkRoot(root string, walkFn walkFunc) error {
	info, err := os.Lstat(root)
	if err != nil {
		err = walkFn(root, nil, err, false)
	} else {
		err = walk(root, info, walkFn)
	}
	if err == filepath.SkipDir {
		return nil
	}
	return err
}

// walk recursively descends path, calling walkFn.
func walk(path string, info os.FileInfo, walkFn walkFunc) error {
	if !info.IsDir() {
		return walkFn(path, info, nil, false)
	}

	names, err0 := readDirNames(path)

	for _, name := range names {
		filename := filepath.Join(path, name)
		fileInfo, err := os.Lstat(filename)
		if err != nil {
			if err := walkFn(filename, fileInfo, err, false); err != nil && err != filepath.SkipDir {
				return err
			}
		} else {
			err = walk(filename, fileInfo, walkFn)
			if err != nil {
				if !fileInfo.IsDir() || err != filepath.SkipDir {
					return err
				}
			}
		}
	}

	// re-read the directory contents after all children have been walked
	dirEmpty := false
	if names, err := readDirNames(path); err == nil && len(names) == 0 {
		dirEmpty = true
	}

	err1 := walkFn(path, info, err0, dirEmpty)
	// If err0 != nil, walk can't walk into this directory.
	// err1 != nil means walkFn want walk to skip this directory or stop walking.
	// Therefore, if one of err and err1 isn't nil, walk will return.
	if err0 != nil || err1 != nil {
		// The caller's behavior is controlled by the return value, which is decided
		// by walkFn. walkFn may ignore err and return nil.
		// If walkFn returns SkipDir, it will be handled by the caller.
		// So walk should return whatever walkFn returns.
		return err1
	}

	return nil
}

// readDirNames reads the directory named by dirname and returns
// an unsorted list of directory entries.
func readDirNames(dirname string) ([]string, error) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil, err
	}
	names, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		return nil, err
	}

	return names, nil
}

const cleanWalkFrequency = 10 * time.Minute

func walkLoop(storageName, walkPath string) {
	logrus.WithField("storage", storageName).Infof("Starting file walker for %s", walkPath)
	walkTick := time.NewTicker(cleanWalkFrequency)
	dontpanic.GoForever(time.Minute, func() {
		for {
			if err := cleanWalk(walkPath); err != nil {
				logrus.WithField("storage", storageName).Error(err)
			}

			<-walkTick.C
		}
	})
}

func startCleanWalker(storage config.Storage) {
	if disableWalker {
		return
	}

	walkLoop(storage.Name, tempdir.CacheDir(storage))
	walkLoop(storage.Name, tempdir.StateDir(storage))
}

var (
	disableMoveAndClear bool // only used to disable move and clear in tests
	disableWalker       bool // only used to disable object walker in tests
)

// moveAndClear will move the cache to the storage location's
// temporary folder, and then remove its contents asynchronously
func moveAndClear(storage config.Storage) error {
	if disableMoveAndClear {
		return nil
	}

	logger := logrus.WithField("storage", storage.Name)
	logger.Info("clearing disk cache object folder")

	tempPath := tempdir.TempDir(storage)
	if err := os.MkdirAll(tempPath, 0755); err != nil {
		return err
	}

	tmpDir, err := ioutil.TempDir(tempPath, "diskcache")
	if err != nil {
		return err
	}

	logger.Infof("moving disk cache object folder to %s", tmpDir)
	cachePath := tempdir.CacheDir(storage)
	if err := os.Rename(cachePath, filepath.Join(tmpDir, "moved")); err != nil {
		if os.IsNotExist(err) {
			logger.Info("disk cache object folder doesn't exist, no need to remove")
			return nil
		}

		return err
	}

	dontpanic.Go(func() {
		start := time.Now()
		if err := os.RemoveAll(tmpDir); err != nil {
			logger.Errorf("unable to remove disk cache objects: %q", err)
		}

		logger.Infof("cleared all cache object files in %s after %s", tmpDir, time.Since(start))
	})

	return nil
}

func init() {
	config.RegisterHook(func(cfg config.Cfg) error {
		for _, storage := range cfg.Storages {
			if err := moveAndClear(storage); err != nil {
				return err
			}

			startCleanWalker(storage)
		}
		return nil
	})
}
