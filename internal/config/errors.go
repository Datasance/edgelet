package config

import "errors"

var (
	ErrConfigNotLoaded    = errors.New("configuration not loaded")
	ErrProfileNotFound    = errors.New("profile not found")
	ErrInvalidConfig      = errors.New("invalid configuration")
	ErrConfigFileNotFound = errors.New("config file not found")
)
