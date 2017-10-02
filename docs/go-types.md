```
type Manager struct {
}

type pinnedRepos struct {
	name    string
	version string
}

type package struct {
	importPath string
	subpackages []package
}

func (m *Manager) download(repo imports.Repo, version string) {

}
```
