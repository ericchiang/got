package imports

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestCacheKey(t *testing.T) {
	tests := []struct {
		remote string
		want   string
	}{
		{"https://github.com/camlistore/go4", "https---github-com-camlistore-go4"},
		{"git@github.com:foo/bar", "git-github-com-foo-bar"},
	}
	for _, test := range tests {
		got := cacheKey(test.remote)
		if got != test.want {
			t.Errorf("cacheKey(%q), wanted=%q, got=%q", test.remote, test.want, got)
		}
	}
}

func TestIgnoreFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"asm_darwin_386.s", false},
		{"gccgo_c.c", false},
		{"errors.go", false},
		{"errors.py", true},
		{"errors_test.go", true},
		{"LICENSE", false},
		{"LICENSE.txt", false},
	}
	for _, test := range tests {
		got := ignoreFile(test.name)
		if got != test.want {
			t.Errorf("ignoreFile(%q), wanted=%t, got=%t", test.name, test.want, got)
		}
	}
}

type file struct {
	path string
	// If data is empty, the filepath is assumed to be a directory
	data string
}

func (f file) isDir() bool {
	return f.data == ""
}

func writeFiles(t *testing.T, dir string, files []file) {
	for _, f := range files {
		target := filepath.Join(dir, f.path)
		if f.isDir() {
			if err := os.Mkdir(target, 0755); err != nil {
				t.Fatalf("creating directory %s: %v", target, err)
			}
		} else {
			if err := ioutil.WriteFile(target, []byte(f.data), 0644); err != nil {
				t.Fatalf("writing file %s: %v", target, err)
			}
		}
	}
}

func compareFiles(t *testing.T, dir string, files []file) {
	toMatch := make(map[string]file, len(files))
	for _, f := range files {
		toMatch[f.path] = f
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			f, ok := toMatch[rel]
			if !ok {
				t.Errorf("found unexpected directory %s", rel)
				return nil
			}
			delete(toMatch, rel)
			if !f.isDir() {
				t.Errorf("found directory %s but expected a file", rel)
			}
			return nil
		}

		f, ok := toMatch[rel]
		if !ok {
			t.Errorf("found unexpected file %s", rel)
			return nil
		}
		delete(toMatch, rel)
		if f.isDir() {
			t.Errorf("found file %s but expected a directory", rel)
			return nil
		}

		data, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		got := string(data)
		if got != f.data {
			t.Errorf("expected file %s to contain data:\n%s\ngot:\n%s\n", f.data, got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range toMatch {
		if f.isDir() {
			t.Errorf("didn't find directory %s", name)
		} else {
			t.Errorf("didn't find file %s", name)
		}
	}
}

func TestCopyDir(t *testing.T) {
	tests := []struct {
		files []file
		want  []file
	}{
		{
			files: []file{
				{"a", ""},
				{"a/b", ""},
				{"a/b/hi.go", `package b`},
				{"a/c", ""},
				{"a/.foo", ""},
				{"a/.foo/hi.go", "package foo"},
			},
			want: []file{
				{"a", ""},
				{"a/b", ""},
				{"a/b/hi.go", `package b`},
				{"a/c", ""},
			},
		},
	}

	for _, test := range tests {
		func() {
			src, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(src)

			dest, err := ioutil.TempDir("", "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dest)

			writeFiles(t, src, test.files)

			if err := copyDir(dest, src); err != nil {
				t.Error(err)
			}

			compareFiles(t, dest, test.want)
		}()
	}
}
