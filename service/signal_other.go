//go:build !windows

package main

import "context"

func startSignalWatcher(ctx context.Context, a *Agent) {}

