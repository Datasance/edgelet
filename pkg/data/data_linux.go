//go:build linux && full && !cgo

package data

import (
	"embed"

	"github.com/datasance/edgelet/pkg/data/bindata"
)

//go:embed embed/*
var embedFS embed.FS

var (
	bd = bindata.Bindata{FS: &embedFS, Prefix: "embed"}

	// Asset returns embedded bundle bytes by filename (SHA256.tar.zst).
	Asset = bd.Asset
	// AssetNames lists embedded bundle filenames.
	AssetNames = bd.AssetNames
)
