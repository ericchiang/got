package imports

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/pkg/errors"
)

func withCache(t *testing.T, test func(t *testing.T, c *cache)) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	c, err := newCache(filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	test(t, c)
}

func TestNewCache(t *testing.T) {
	withCache(t, func(_ *testing.T, _ *cache) {})
}

func TestFileCache(t *testing.T) {
	withCache(t, testFileCache)
}

func testFileCache(t *testing.T, c *cache) {
	const key = "foo"
	var data = []byte("bar")
	if err := c.file(key, func(p string) error {
		return ioutil.WriteFile(p, data, 0644)
	}); err != nil {
		t.Fatal(err)
	}

	if err := c.file(key, func(p string) error {
		got, err := ioutil.ReadFile(p)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, got) {
			return errors.Errorf("file didn't contain expected content wanted (%s), got (%s)", data, got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
