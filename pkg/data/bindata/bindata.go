package bindata

import (
	"embed"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// Bindata wraps embed.FS with go-bindata-style Asset helpers.
type Bindata struct {
	FS     *embed.FS
	Prefix string
}

func (b Bindata) Asset(name string) ([]byte, error) {
	return b.FS.ReadFile(path.Join(b.Prefix, name))
}

func (b Bindata) AssetNames() []string {
	var assets []string
	_ = fs.WalkDir(b.FS, ".", func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry == nil {
			return err
		}
		n := entry.Name()
		if entry.Type().IsRegular() && !strings.HasPrefix(n, ".") && !strings.HasPrefix(n, "_") {
			assets = append(assets, strings.TrimPrefix(strings.TrimPrefix(p, b.Prefix), "/"))
		}
		return nil
	})
	slices.Sort(assets)
	return assets
}
