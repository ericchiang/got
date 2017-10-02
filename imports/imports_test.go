package imports

import (
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadInfo(t *testing.T) {
	tests := []struct {
		file    string
		imports []string
	}{
		{
			file: `package foo

import (
	"net"

	"golang.org/x/net"
	"golang.org/x/net/context"
)
`,
			imports: []string{
				"golang.org/x/net",
				"golang.org/x/net/context",
			},
		},
	}

	for _, test := range tests {
		dir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, "foo.go")
		if err := ioutil.WriteFile(target, []byte(test.file), 0644); err != nil {
			t.Fatal(err)
		}
		imports, err := loadImports(target)
		if err != nil {
			t.Fatalf("loading file %s: %v", target, err)
		}
		if reflect.DeepEqual(imports, test.imports) {
			t.Errorf("expected package imports %q got %q", test.imports, imports)
		}
	}
}

func TestImportMeta(t *testing.T) {
	tests := []struct {
		name   string
		root   string
		remote string
		vcs    string
	}{
		{
			name:   "github.com/spf13/cobra",
			root:   "github.com/spf13/cobra",
			remote: "https://github.com/spf13/cobra",
			vcs:    "git",
		},
		{
			name:   "github.com/miekg/dns/dnsutil",
			root:   "github.com/miekg/dns",
			remote: "https://github.com/miekg/dns",
			vcs:    "git",
		},
		{
			name:   "bitbucket.org/bertimus9/systemstat",
			root:   "bitbucket.org/bertimus9/systemstat",
			remote: "https://bitbucket.org/bertimus9/systemstat",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			meta, ok := importMeta(test.name)
			if !ok {
				t.Fatalf("couldn't look up package %s statically", test.name)
			}
			want := pkgMeta{
				Root:   test.root,
				Remote: test.remote,
				VCS:    test.vcs,
			}
			got := *meta

			if !reflect.DeepEqual(want, got) {
				t.Errorf("wanted=%#v, got=%#v", want, got)
			}
		})
	}
}

func TestPartImportMeta(t *testing.T) {
	tests := []struct {
		name string
		resp string
		want pkgMeta
	}{
		{
			name: "go4.org/lock",
			resp: `
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
<meta name="go-import" content="go4.org git https://github.com/camlistore/go4">
<meta name="go-source" content="go4.org https://github.com/camlistore/go4/ https://github.com/camlistore/go4/tree/master{/dir} https://github.com/camlistore/go4/blob/master{/dir}/{file}#L{line}">
<meta http-equiv="refresh" content="0; url=https://godoc.org/go4.org/lock">
</head>
<body>
See <a href="https://godoc.org/go4.org/lock">docs</a>.
</body>
</html>
		`,
			want: pkgMeta{
				Root:   "go4.org",
				Remote: "https://github.com/camlistore/go4",
				VCS:    "git",
			},
		},
		{
			name: "golang.org/x/net/context",
			resp: `
<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
<meta name="go-import" content="golang.org/x/net git https://go.googlesource.com/net">
<meta name="go-source" content="golang.org/x/net https://github.com/golang/net/ https://github.com/golang/net/tree/master{/dir} https://github.com/golang/net/blob/master{/dir}/{file}#L{line}">
<meta http-equiv="refresh" content="0; url=https://godoc.org/golang.org/x/net/context">
</head>
<body>
Nothing to see here; <a href="https://godoc.org/golang.org/x/net/context">move along</a>.
</body>
</html>
			`,
			want: pkgMeta{
				Root:   "golang.org/x/net",
				Remote: "https://go.googlesource.com/net",
				VCS:    "git",
			},
		},
		{
			name: "cloud.google.com/go/compute/metadata",
			resp: `
<!DOCTYPE html>
<html>
  <head>
    
    <meta name="go-import" content="cloud.google.com/go git https://code.googlesource.com/gocloud">
    <meta name="go-source" content="cloud.google.com/go https://github.com/GoogleCloudPlatform/gcloud-golang https://github.com/GoogleCloudPlatform/gcloud-golang/tree/master{/dir} https://github.com/GoogleCloudPlatform/gcloud-golang/tree/master{/dir}/{file}#L{line}">
    <meta http-equiv="refresh" content="0; url=https://godoc.org/cloud.google.com/go/compute/metadata">
  </head>
  <body>
    Nothing to see here. Please <a href="https://godoc.org/cloud.google.com/go/compute/metadata">move along</a>.
  </body>
</html>
			`,
			want: pkgMeta{
				Root:   "cloud.google.com/go",
				Remote: "https://code.googlesource.com/gocloud",
				VCS:    "git",
			},
		},
		{
			name: "gopkg.in/gcfg.v1/scanner",
			resp: `
<html>
<head>
<meta name="go-import" content="gopkg.in/gcfg.v1 git https://gopkg.in/gcfg.v1">
<meta name="go-source" content="gopkg.in/gcfg.v1 _ https://github.com/go-gcfg/gcfg/tree/v1.2.0{/dir} https://github.com/go-gcfg/gcfg/blob/v1.2.0{/dir}/{file}#L{line}">
</head>
<body>
go get gopkg.in/gcfg.v1/scanner
</body>
</html>
			`,
			want: pkgMeta{
				Root:   "gopkg.in/gcfg.v1",
				Remote: "https://gopkg.in/gcfg.v1",
				VCS:    "git",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp := strings.NewReader(strings.TrimSpace(test.resp))

			got, err := parseImportMeta(resp)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(*got, test.want) {
				t.Errorf("wanted=%#v, got=%#v", test.want, *got)
			}
		})
	}
}
