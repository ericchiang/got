// Package imports implements code for inspecting and managing Go imports.
package imports

import (
	"context"
	"encoding/xml"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

// loadImports loads a file and parses its import declarations and package name
func loadImports(file string) (imports []string, err error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
	if err != nil {
		return nil, errors.Wrap(err, "parsing file")
	}
	for _, imp := range f.Imports {
		if imp.Path == nil || imp.Path.Value == "" || goStdPackages[imp.Path.Value] {
			continue
		}
		imports = append(imports, imp.Path.Value)
	}
	return imports, nil
}

// pkgMeta holds information about a package's remote repo.
type pkgMeta struct {
	// Root is the package that corresponds to the root of the remote repo.
	// For example the root package of "golang.org/x/net/context" is
	// "golang.org/x/net".
	Root string

	// Remote is the remote address of a package's repo.
	// For example "http://golang.org/x/net"
	Remote string

	// VCS is the version control system used by the remote repo. For example "git" or "svn"
	VCS string
}

func importMeta(pkg string) (*pkgMeta, bool) {
	for _, v := range vcsList {
		m := v.regex.FindStringSubmatch(pkg)
		if m == nil {
			continue
		}

		if m[1] != "" {
			root := m[1]
			return &pkgMeta{
				Root:   root,
				Remote: "https://" + root,
				VCS:    v.vcs,
			}, true
		}
	}
	return nil, false
}

var defaultResolver = new(resolver)

type resolver struct {
	mu sync.Mutex

	// inflight requests
	inflight []*resolverInflight
	// cached results
	results []*pkgMeta
}

type resolverInflight struct {
	// Name of the package that's being queried.
	pkg string

	// Channel that returns a value when the meta and err fields
	// are valid to read.
	done <-chan struct{}

	meta *pkgMeta
	err  error
}

func (r *resolver) fetchImportMeta(ctx context.Context, pkg string) (*pkgMeta, error) {
	r.mu.Lock()

	// First check the cache.
	for _, result := range r.results {
		if !strings.HasPrefix(pkg, result.Root) {
			continue
		}

		result := result
		r.mu.Unlock()
		return result, nil
	}

	// Then check if there's an inflight request that can satisfy the same query.
	for _, inflight := range r.inflight {
		if !strings.HasPrefix(pkg, inflight.pkg) && !strings.HasPrefix(inflight.pkg, pkg) {
			continue
		}
		// Found an inflight request, just wait on that.
		inflight := inflight
		r.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, errors.Wrap(ctx.Err(), "stopped waiting for inflight request")
		case <-inflight.done:
			return inflight.meta, inflight.err
		}
	}

	// No inflight request, have to set one up.
	done := make(chan struct{})
	inflight := &resolverInflight{pkg: pkg, done: done}
	r.inflight = append(r.inflight, inflight)
	r.mu.Unlock()

	// Fetch metadata.
	inflight.meta, inflight.err = fetchImportMeta(ctx, pkg)

	// Signal to other goroutines that the results can be checked.
	close(done)

	// Remove inflight from query. Record result if no errors were experienced.
	r.mu.Lock()
	if inflight.err == nil {
		r.results = append(r.results, inflight.meta)
	}

	n := 0
	for _, inf := range r.inflight {
		if inf.pkg == pkg {
			continue
		}
		r.inflight[n] = inf
		n++
	}
	r.inflight = r.inflight[:n]
	r.mu.Unlock()

	return inflight.meta, inflight.err
}

func fetchImportMeta(ctx context.Context, pkg string) (*pkgMeta, error) {
	u := "https://" + pkg
	if strings.ContainsRune(u, '?') {
		u = u + "&go-get=1"
	} else {
		u = u + "?go-get=1"
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, errors.Wrap(err, "create request")
	}
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "getting go-get url %s", u)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, errors.Errorf("getting go-get url %s: %s", u, resp.Status)
	}

	meta, err := parseImportMeta(resp.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing response from %s", u)
	}
	return meta, nil
}

func parseImportMeta(r io.Reader) (*pkgMeta, error) {
	d := xml.NewDecoder(r)
	d.CharsetReader = charsetReader
	d.Strict = false
	for {
		t, err := d.Token()
		if err != nil {
			if err == io.EOF {
				// If we hit the end of the markup and don't have anything
				// we return an error.
				return nil, errors.Errorf("no 'go-import' meta field found")
			}
			return nil, errors.Wrap(err, "parsing go-get response")
		}
		if e, ok := t.(xml.StartElement); ok && strings.EqualFold(e.Name.Local, "body") {
			return nil, errors.Errorf("no 'go-import' meta field found")
		}
		if e, ok := t.(xml.EndElement); ok && strings.EqualFold(e.Name.Local, "head") {
			return nil, errors.Errorf("no 'go-import' meta field found")
		}
		e, ok := t.(xml.StartElement)
		if !ok || !strings.EqualFold(e.Name.Local, "meta") {
			continue
		}
		if attrValue(e.Attr, "name") != "go-import" {
			continue
		}
		if f := strings.Fields(attrValue(e.Attr, "content")); len(f) == 3 {
			return &pkgMeta{
				Root:   f[0],
				VCS:    f[1],
				Remote: f[2],
			}, nil
		}
	}
}

func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "ascii":
		return input, nil
	default:
		return nil, fmt.Errorf("can't decode XML document using charset %q", charset)
	}
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}

type vcsInfo struct {
	host    string
	pattern string
	vcs     string
	regex   *regexp.Regexp
}

func init() {
	// Precompile the regular expressions used to check VCS locations.
	for _, v := range vcsList {
		v.regex = regexp.MustCompile(v.pattern)
	}
}

var vcsList = []*vcsInfo{
	{
		host:    "github.com",
		pattern: `^(?P<rootpkg>github\.com/[A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+)(/[A-Za-z0-9_.\-]+)*$`,
		vcs:     "git",
	},
	{
		host:    "bitbucket.org",
		pattern: `^(?P<rootpkg>bitbucket\.org/([A-Za-z0-9_.\-]+/[A-Za-z0-9_.\-]+))(/[A-Za-z0-9_.\-]+)*$`,
		// Bitbucket can host multiple kind of repos.
	},
	{
		host:    "launchpad.net",
		pattern: `^(?P<rootpkg>launchpad\.net/(([A-Za-z0-9_.\-]+)(/[A-Za-z0-9_.\-]+)?|~[A-Za-z0-9_.\-]+/(\+junk|[A-Za-z0-9_.\-]+)/[A-Za-z0-9_.\-]+))(/[A-Za-z0-9_.\-]+)*$`,
		vcs:     "bzr",
	},
	{
		host:    "git.launchpad.net",
		pattern: `^(?P<rootpkg>git\.launchpad\.net/(([A-Za-z0-9_.\-]+)|~[A-Za-z0-9_.\-]+/(\+git|[A-Za-z0-9_.\-]+)/[A-Za-z0-9_.\-]+))$`,
		vcs:     "git",
	},
	{
		host:    "hub.jazz.net",
		pattern: `^(?P<rootpkg>hub\.jazz\.net/git/[a-z0-9]+/[A-Za-z0-9_.\-]+)(/[A-Za-z0-9_.\-]+)*$`,
		vcs:     "git",
	},
	{
		host:    "go.googlesource.com",
		pattern: `^(?P<rootpkg>go\.googlesource\.com/[A-Za-z0-9_.\-]+/?)$`,
	},
	// TODO: Once Google Code becomes fully deprecated this can be removed.
	{
		host:    "code.google.com",
		pattern: `^(?P<rootpkg>code\.google\.com/[pr]/([a-z0-9\-]+)(\.([a-z0-9\-]+))?)(/[A-Za-z0-9_.\-]+)*$`,
		vcs:     "git",
	},
	// Alternative Google setup for SVN. This is the previous structure but it still works... until Google Code goes away.
	{
		pattern: `^(?P<rootpkg>[a-z0-9_\-.]+\.googlecode\.com/svn(/.*)?)$`,
		vcs:     "svc",
	},
	// Alternative Google setup. This is the previous structure but it still works... until Google Code goes away.
	{
		pattern: `^(?P<rootpkg>[a-z0-9_\-.]+\.googlecode\.com/(git|hg))(/.*)?$`,
	},
	// If none of the previous detect the type they will fall to this looking for the type in a generic sense
	// by the extension to the path.
	{
		pattern: `^(?P<rootpkg>(?P<repo>([a-z0-9.\-]+\.)+[a-z0-9.\-]+(:[0-9]+)?/[A-Za-z0-9_.\-/]*?)\.(bzr|git|hg|svn))(/[A-Za-z0-9_.\-]+)*$`,
	},
}

// Values generated using the following command.
//
//		go list std | grep -v 'vendor' | awk '{ printf "\"%s\": true,\n", $1 }'
//
var goStdPackages = map[string]bool{
	"C": true, // cgo

	"archive/tar":                       true,
	"archive/zip":                       true,
	"bufio":                             true,
	"bytes":                             true,
	"compress/bzip2":                    true,
	"compress/flate":                    true,
	"compress/gzip":                     true,
	"compress/lzw":                      true,
	"compress/zlib":                     true,
	"container/heap":                    true,
	"container/list":                    true,
	"container/ring":                    true,
	"context":                           true,
	"crypto":                            true,
	"crypto/aes":                        true,
	"crypto/cipher":                     true,
	"crypto/des":                        true,
	"crypto/dsa":                        true,
	"crypto/ecdsa":                      true,
	"crypto/elliptic":                   true,
	"crypto/hmac":                       true,
	"crypto/internal/cipherhw":          true,
	"crypto/md5":                        true,
	"crypto/rand":                       true,
	"crypto/rc4":                        true,
	"crypto/rsa":                        true,
	"crypto/sha1":                       true,
	"crypto/sha256":                     true,
	"crypto/sha512":                     true,
	"crypto/subtle":                     true,
	"crypto/tls":                        true,
	"crypto/x509":                       true,
	"crypto/x509/pkix":                  true,
	"database/sql":                      true,
	"database/sql/driver":               true,
	"debug/dwarf":                       true,
	"debug/elf":                         true,
	"debug/gosym":                       true,
	"debug/macho":                       true,
	"debug/pe":                          true,
	"debug/plan9obj":                    true,
	"encoding":                          true,
	"encoding/ascii85":                  true,
	"encoding/asn1":                     true,
	"encoding/base32":                   true,
	"encoding/base64":                   true,
	"encoding/binary":                   true,
	"encoding/csv":                      true,
	"encoding/gob":                      true,
	"encoding/hex":                      true,
	"encoding/json":                     true,
	"encoding/pem":                      true,
	"encoding/xml":                      true,
	"errors":                            true,
	"expvar":                            true,
	"flag":                              true,
	"fmt":                               true,
	"go/ast":                            true,
	"go/build":                          true,
	"go/constant":                       true,
	"go/doc":                            true,
	"go/format":                         true,
	"go/importer":                       true,
	"go/internal/gccgoimporter":         true,
	"go/internal/gcimporter":            true,
	"go/internal/srcimporter":           true,
	"go/parser":                         true,
	"go/printer":                        true,
	"go/scanner":                        true,
	"go/token":                          true,
	"go/types":                          true,
	"hash":                              true,
	"hash/adler32":                      true,
	"hash/crc32":                        true,
	"hash/crc64":                        true,
	"hash/fnv":                          true,
	"html":                              true,
	"html/template":                     true,
	"image":                             true,
	"image/color":                       true,
	"image/color/palette":               true,
	"image/draw":                        true,
	"image/gif":                         true,
	"image/internal/imageutil":          true,
	"image/jpeg":                        true,
	"image/png":                         true,
	"index/suffixarray":                 true,
	"internal/cpu":                      true,
	"internal/nettrace":                 true,
	"internal/poll":                     true,
	"internal/race":                     true,
	"internal/singleflight":             true,
	"internal/syscall/unix":             true,
	"internal/syscall/windows":          true,
	"internal/syscall/windows/registry": true,
	"internal/syscall/windows/sysdll":   true,
	"internal/testenv":                  true,
	"internal/trace":                    true,
	"io":                                true,
	"io/ioutil":                         true,
	"log":                               true,
	"log/syslog":                        true,
	"math":                              true,
	"math/big":                          true,
	"math/bits":                         true,
	"math/cmplx":                        true,
	"math/rand":                         true,
	"mime":                              true,
	"mime/multipart":                    true,
	"mime/quotedprintable":              true,
	"net":                            true,
	"net/http":                       true,
	"net/http/cgi":                   true,
	"net/http/cookiejar":             true,
	"net/http/fcgi":                  true,
	"net/http/httptest":              true,
	"net/http/httptrace":             true,
	"net/http/httputil":              true,
	"net/http/internal":              true,
	"net/http/pprof":                 true,
	"net/internal/socktest":          true,
	"net/mail":                       true,
	"net/rpc":                        true,
	"net/rpc/jsonrpc":                true,
	"net/smtp":                       true,
	"net/textproto":                  true,
	"net/url":                        true,
	"os":                             true,
	"os/exec":                        true,
	"os/signal":                      true,
	"os/user":                        true,
	"path":                           true,
	"path/filepath":                  true,
	"plugin":                         true,
	"reflect":                        true,
	"regexp":                         true,
	"regexp/syntax":                  true,
	"runtime":                        true,
	"runtime/cgo":                    true,
	"runtime/debug":                  true,
	"runtime/internal/atomic":        true,
	"runtime/internal/sys":           true,
	"runtime/pprof":                  true,
	"runtime/pprof/internal/profile": true,
	"runtime/race":                   true,
	"runtime/trace":                  true,
	"sort":                           true,
	"strconv":                        true,
	"strings":                        true,
	"sync":                           true,
	"sync/atomic":                    true,
	"syscall":                        true,
	"testing":                        true,
	"testing/internal/testdeps":      true,
	"testing/iotest":                 true,
	"testing/quick":                  true,
	"text/scanner":                   true,
	"text/tabwriter":                 true,
	"text/template":                  true,
	"text/template/parse":            true,
	"time":                           true,
	"unicode":                        true,
	"unicode/utf16":                  true,
	"unicode/utf8":                   true,
	"unsafe":                         true,
}
