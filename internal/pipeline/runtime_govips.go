//go:build govips && cgo

package pipeline

import (
	"sync"

	"github.com/davidbyttow/govips/v2/vips"
)

var (
	startupOnce sync.Once
	shutdownMu  sync.Mutex
	started     bool
)

func Startup() error {
	startupOnce.Do(func() {
		vips.Startup(&vips.Config{
			MaxCacheFiles: 0,
			MaxCacheMem:   128 * 1024 * 1024,
			MaxCacheSize:  100,
		})

		shutdownMu.Lock()
		started = true
		shutdownMu.Unlock()
	})
	return nil
}

func Shutdown() {
	shutdownMu.Lock()
	defer shutdownMu.Unlock()
	if !started {
		return
	}
	vips.Shutdown()
	started = false
}

func newTransformer() (Transformer, error) {
	return govipsTransformer{}, nil
}
