//go:build tools
// +build tools

package build

import (
	// Redis client
	_ "github.com/redis/go-redis/v9"
	// Mini Redis server for testing
	_ "github.com/alicebob/miniredis/v2"
)
