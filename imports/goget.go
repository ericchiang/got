package imports

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/vcs"
	"github.com/pkg/errors"
)

// cacheKey replaces any non-filepath frendly characters with '-'. This could
// potentially create an ambiguous mapping, but practically we don't
// expect it.
func cacheKey(remote string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		default:
			return '-'
		}
		return r
	}, remote)
}

type repoDir struct {
	Name    string
	Package bool
	Files   []string
	Imports []string
}

func goGet(c *cache, meta *pkgMeta, to, version string) error {
	if version == "" {
		return errors.New("no version specified to checkout")
	}

	return c.dir(cacheKey(meta.Remote), func(path string) error {
		repo, err := newRepo(meta, path)
		if err != nil {
			return errors.Wrap(err, "creating repo")
		}

		if !repo.CheckLocal() {
			if err := repo.Get(); err != nil {
				if e, ok := err.(*vcs.RemoteError); ok {
					return errors.Errorf("%s: %s %v", e.Error(), e.Out(), e.Original())
				}
				return errors.Wrap(err, "cloning repo")
			}
		}

		if err := repo.UpdateVersion(version); err != nil {
			// Revision might just not exist locally.
			if err := repo.Update(); err != nil {
				return errors.Wrap(err, "updating repo")
			}
			if err := repo.UpdateVersion(version); err != nil {
				return errors.Wrapf(err, "updating repo to revision %s", version)
			}
		}
		if err := copyDir(to, path); err != nil {
			return errors.Wrap(err, "copying repo")
		}
		return nil
	})
}

func newRepo(meta *pkgMeta, local string) (vcs.Repo, error) {
	// Manually setting the VCS prevents another round trip to the
	// provider to determine what the VCS is.
	switch meta.VCS {
	case "git":
		return vcs.NewGitRepo(meta.Remote, local)
	case "svn":
		return vcs.NewSvnRepo(meta.Remote, local)
	case "bzr":
		return vcs.NewBzrRepo(meta.Remote, local)
	case "hg":
		return vcs.NewHgRepo(meta.Remote, local)
	default:
		return vcs.NewRepo(meta.Remote, local)
	}
}

func copyDir(to, from string) error {
	// TODO: speed this up.
	//
	// - Don't need to stat files if ignoreDir and ignoreFile tell us to ignore them.
	// - Don't need to sort results.
	// - Can use multiple goroutines.
	//
	return filepath.Walk(from, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if from == path {
			return nil
		}

		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)

		name := filepath.Base(path)

		if info.IsDir() {
			if ignoreDir(name) {
				return filepath.SkipDir
			}

			// Use Mkdir instead of MkdirAll because the parent directories
			// should already exist. If they don't, it's an indication that
			// there's an error in this method's logic.
			//
			// TODO: don't create empty directories.
			if err := os.Mkdir(target, info.Mode()); err != nil {
				return errors.Wrapf(err, "copying directory %s", path)
			}
			return nil
		}

		if ignoreFile(name) {
			return nil
		}

		from, err := os.OpenFile(path, os.O_RDONLY, info.Mode())
		if err != nil {
			return errors.Wrapf(err, "opening file for reading %s", path)
		}
		defer from.Close()

		to, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode())
		if err != nil {
			return errors.Wrapf(err, "creating copy of file %s", path)
		}
		defer to.Close()

		if _, err := io.Copy(to, from); err != nil {
			return errors.Wrapf(err, "copying file contents of %s", path)
		}
		return nil
	})
}

func ignoreDir(dirname string) bool {
	switch dirname {
	case "testdata", "vendor":
		return true
	}
	if strings.HasPrefix(dirname, ".") {
		return true
	}
	return strings.HasPrefix(dirname, "_")
}

var versionFiles = []string{
	"godeps.json",
	"glide.yaml",

	// "gopkg.toml", // Not understood yet.
}

func ignoreFile(filename string) bool {
	for _, name := range versionFiles {
		if strings.EqualFold(filename, name) {
			return false
		}
	}

	switch filepath.Ext(filename) {
	case ".s", ".c":
		// Go code can depend on .s and .c files, e.g.:
		// https://github.com/golang/sys/tree/master/unix
		return false
	case ".go":
		// Always ignore test files.
		return strings.HasSuffix(filename, "_test.go")
	}

	return !isLegalFile(filename)
}

// File lists and code taken from https://github.com/sgotti/glide-vc/blob/master/gvc.go
// which was in turn taken from https://github.com/client9/gosupplychain/blob/master/license.go

// licenseFilePrefix is a list of filename prefixes that indicate it
//  might contain a software license
var licenseFilePrefix = []string{
	"licence", // UK spelling
	"license", // US spelling
	"copying",
	"unlicense",
	"copyright",
	"copyleft",
}

// legalFileSubstring are substrings that indicate the file is likely
// to contain some type of legal declaration.  "legal" is often used
// that it might be moved to LicenseFilePrefix
var legalFileSubstring = []string{
	"legal",
	"notice",
	"disclaimer",
	"patent",
	"third-party",
	"thirdparty",
}

// isLegalFile returns true if the file is likely to contain some type
// of of legal declaration or licensing information
func isLegalFile(path string) bool {
	lowerfile := strings.ToLower(filepath.Base(path))
	for _, prefix := range licenseFilePrefix {
		if strings.HasPrefix(lowerfile, prefix) {
			return true
		}
	}
	for _, substring := range legalFileSubstring {
		if strings.Contains(lowerfile, substring) {
			return true
		}
	}
	return false
}
