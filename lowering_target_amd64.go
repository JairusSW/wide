//go:build amd64

package wide

import (
	"github.com/wago-org/wago/codegen"
	x86 "github.com/wago-org/wago/codegen/amd64"
	a64 "github.com/wago-org/wago/codegen/arm64"
)

func selectTargetLowering(amd64 *x86.Lowering, _ *a64.Lowering) codegen.Lowering {
	if amd64 == nil {
		return nil
	}
	return amd64
}
