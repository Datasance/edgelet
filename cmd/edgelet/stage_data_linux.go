//go:build linux && !cgo

package main

import "github.com/eclipse-iofog/edgelet/pkg/data"

func dataEnsureExtracted() error { return data.EnsureExtracted() }

func dataRuntimeBinary() (string, error) { return data.RuntimeBinary() }
