package imports

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"go4.org/lock"
)

type cache struct {
	dirname string
}

func newCache(dirname string) (*cache, error) {
	if err := os.MkdirAll(dirname, 0755); err != nil {
		return nil, errors.Wrap(err, "creating cache directory")
	}
	return &cache{dirname}, nil
}

func (c *cache) dir(name string, f func(filepath string) error) error {
	target := filepath.Join(c.dirname, name)

	if _, err := os.Stat(target); err != nil {
		if !os.IsNotExist(err) {
			return errors.Wrap(err, "cache accessing directory")
		}

		if err := os.Mkdir(target, 755); err != nil {
			return errors.Wrap(err, "cache creating directory")
		}
	}

	closer, err := lock.Lock(target + ".lock")
	if err != nil {
		return errors.Wrap(err, "cache acquiring directory lock")
	}
	defer closer.Close()
	return f(target)
}

func (c *cache) file(name string, f func(filepath string) error) error {
	target := filepath.Join(c.dirname, name)

	closer, err := lock.Lock(target + ".lock")
	if err != nil {
		return errors.Wrap(err, "cache acquiring file lock")
	}
	defer closer.Close()

	return f(target)
}
