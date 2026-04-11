// Package profiling provides Pyroscope continuous profiling integration.
package profiling

import (
	"fmt"
	"os"
	"runtime"

	"github.com/grafana/pyroscope-go"

	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
)

// BuildTags reads pod-level metadata from environment variables and returns
// a tag map for Pyroscope labels. Empty values are omitted.
func BuildTags() map[string]string {
	tags := make(map[string]string)

	if v := os.Getenv("POD_NAME"); v != "" {
		tags["pod"] = v
	}
	if v := os.Getenv("POD_NAMESPACE"); v != "" {
		tags["namespace"] = v
	}

	return tags
}

// Start initializes the Pyroscope profiling SDK if PyroscopeServerAddress is set.
// Returns a stop function to shut down the profiler. When the address is empty,
// returns a no-op stop function.
func Start(cfg *config.Config) (func(), error) {
	if cfg.PyroscopeServerAddress == "" {
		return func() {}, nil
	}

	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)

	profiler, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: cfg.ServiceName,
		ServerAddress:   cfg.PyroscopeServerAddress,
		Tags:            BuildTags(),
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("starting Pyroscope profiler: %w", err)
	}

	return func() {
		_ = profiler.Stop()
	}, nil
}
