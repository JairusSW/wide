//go:build arm64

package wide

import (
	"github.com/wago-org/wago/codegen"
	x86 "github.com/wago-org/wago/codegen/amd64"
	a64 "github.com/wago-org/wago/codegen/arm64"
)

func selectTargetLowering(_ *x86.Lowering, arm64 *a64.Lowering) codegen.Lowering {
	if arm64 == nil {
		return nil
	}
	return arm64
}
