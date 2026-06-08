//go:build linux && cgo

package data

// EmbeddedBundleHash is empty on the fat runtime (embed lives in the thin binary).
func EmbeddedBundleHash() string {
	return ""
}
