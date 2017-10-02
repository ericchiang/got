package imports

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

type pinnedPackage struct {
	meta    *pkgMeta
	version string
}

type resolverFunc func(ctx context.Context, name string) (*pkgMeta, error)

func parseGodeps(lookupPkgMeta resolverFunc, b []byte) ([]pinnedPackage, error) {
	var deps struct {
		Deps []struct {
			ImportPath string
			Rev        string
			// Comment can be a tag, but for now we'll ignore it.
			Comment string
		}
	}

	if err := json.Unmarshal(b, &deps); err != nil {
		return nil, errors.Wrap(err, "parsing godep file")
	}

	// We need to actually resolve the repo these package come from. While doing
	// this, if two packages have the same rev, assume they originate from the
	// same repo. For example if we see dependencies like:
	//
	//		{
	//			"ImportPath": "github.com/coreos/go-oidc/jose",
	//			"Rev": "a4973d9a4225417aecf5d450a9522f00c1f7130f"
	//		},
	//		{
	//			"ImportPath": "github.com/coreos/go-oidc/key",
	//			"Rev": "a4973d9a4225417aecf5d450a9522f00c1f7130f"
	//		},
	//
	// assume they're from the same repo and only look up the repo of one of them.
	toLookup := map[string]string{} // rev -> importPath

	for _, dep := range deps.Deps {
		if dep.ImportPath == "" {
			continue
		}
		if dep.Rev == "" {
			return nil, errors.Errorf("import %s didn't have an associated ref", dep.ImportPath)
		}
		toLookup[dep.Rev] = dep.ImportPath
	}

	var (
		mu       sync.Mutex
		packages []pinnedPackage
	)

	group, ctx := errgroup.WithContext(context.Background())

	for rev, importPath := range toLookup {
		rev, importPath := rev, importPath

		group.Go(func() error {
			meta, err := lookupPkgMeta(ctx, importPath)
			if err != nil {
				return errors.Wrapf(err, "lookup metatags for package %s", importPath)
			}

			mu.Lock()
			packages = append(packages, pinnedPackage{meta, rev})
			mu.Unlock()

			return nil
		})
	}

	return packages, group.Wait()
}
