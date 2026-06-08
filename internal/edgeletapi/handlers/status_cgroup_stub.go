//go:build !linux || !cgo

package handlers

func augmentWithCgroupStatus(map[string]string) {}

func shouldAugmentCgroupStatus() bool { return false }
