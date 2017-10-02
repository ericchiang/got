package imports

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestParseGodeps(t *testing.T) {
	data := `{
	"ImportPath": "k8s.io/kubernetes",
	"GoVersion": "go1.8",
	"GodepVersion": "v79",
	"Packages": [
		"github.com/onsi/ginkgo/ginkgo",
		"github.com/jteeuwen/go-bindata/go-bindata",
		"github.com/tools/godep",
		"./..."
	],
	"Deps": [
		{
			"ImportPath": "github.com/coreos/go-oidc/jose",
			"Rev": "a4973d9a4225417aecf5d450a9522f00c1f7130f"
		},
		{
			"ImportPath": "github.com/coreos/go-oidc/key",
			"Rev": "a4973d9a4225417aecf5d450a9522f00c1f7130f"
		},
		{
			"ImportPath": "github.com/coreos/go-oidc/oauth2",
			"Rev": "a4973d9a4225417aecf5d450a9522f00c1f7130f"
		},
		{
			"ImportPath": "github.com/coreos/go-oidc/oidc",
			"Rev": "a4973d9a4225417aecf5d450a9522f00c1f7130f"
		},
		{
			"ImportPath": "github.com/docker/engine-api/types/time",
			"Comment": "v0.3.1-78-gdea108d",
			"Rev": "dea108d3aa0c67d7162a3fd8aa65f38a430019fd"
		},
		{
			"ImportPath": "github.com/docker/engine-api/types/versions",
			"Comment": "v0.3.1-78-gdea108d",
			"Rev": "dea108d3aa0c67d7162a3fd8aa65f38a430019fd"
		},
		{
			"ImportPath": "github.com/docker/go-connections/nat",
			"Comment": "v0.2.1-30-g3ede32e",
			"Rev": "3ede32e2033de7505e6500d6c868c2b9ed9f169d"
		}
	]
}`

	lookup := func(ctx context.Context, name string) (*pkgMeta, error) {
		meta, ok := importMeta(name)
		if !ok {
			return nil, fmt.Errorf("lookup failed for package %s", name)
		}
		return meta, nil
	}

	want := []pinnedPackage{
		{
			meta: &pkgMeta{
				Root:   "github.com/coreos/go-oidc",
				Remote: "https://github.com/coreos/go-oidc",
				VCS:    "git",
			},
			version: "a4973d9a4225417aecf5d450a9522f00c1f7130f",
		},
		{
			meta: &pkgMeta{
				Root:   "github.com/docker/engine-api",
				Remote: "https://github.com/docker/engine-api",
				VCS:    "git",
			},
			version: "dea108d3aa0c67d7162a3fd8aa65f38a430019fd",
		},
		{
			meta: &pkgMeta{
				Root:   "github.com/docker/go-connections",
				Remote: "https://github.com/docker/go-connections",
				VCS:    "git",
			},
			version: "3ede32e2033de7505e6500d6c868c2b9ed9f169d",
		},
	}

	pkgs, err := parseGodeps(lookup, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		return pkgs[i].meta.Root < pkgs[j].meta.Root
	})
	if !reflect.DeepEqual(pkgs, want) {
		t.Errorf("wanted %#v, got #%v", want, pkgs)
	}
}
