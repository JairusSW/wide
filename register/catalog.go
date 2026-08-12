// Package register exposes Wide's explicit provider catalog to generated Wago
// runtimes. Importing this package has no registration side effects.
package register

import (
	"github.com/JairusSW/wide"
	wago "github.com/wago-org/wago"
)

// Providers returns a fresh catalog so callers cannot mutate shared metadata.
func Providers() []wago.PluginProvider {
	return []wago.PluginProvider{wide.Provider()}
}
