// file: pkg/profiling/profiling.go
package profiling

import (
	"fmt"
	"runtime"

	"github.com/grafana/pyroscope-go"
	"github.com/kaio6fellipe/event-driven-bookinfo/pkg/config"
)

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
		profiler.Stop()
	}, nil
}
