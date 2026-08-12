//go:build !tinygo

package wide

import (
	"context"
	"encoding/json"
	"fmt"

	wago "github.com/wago-org/wago"
)

func loadWide(rt *wago.Runtime, cfg Config) error {
	definition := Definition()
	digest, err := wago.DefinitionDigest(definition)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return rt.LoadPlugins(context.Background(), wago.PluginSet{
		Providers: []wago.PluginProvider{Provider()},
		Selections: []wago.PluginSelection{{
			ID:               definition.ID,
			DefinitionDigest: digest,
			Direct:           true,
			Dependencies:     map[string]string{},
			Config:           raw,
			Grants: []wago.AuthorityGrant{
				{Name: wago.AuthorityCompilerTypeDefine, Scope: wago.AuthorityScope{Modules: []string{"wide"}}},
				{Name: wago.AuthorityCompilerInstructionDefine, Scope: wago.AuthorityScope{Modules: []string{InstructionModule}}},
			},
		}},
	})
}

func carrierName(carrier wago.WasmType) string {
	switch carrier {
	case wago.WasmI32:
		return "i32"
	case wago.WasmI64:
		return "i64"
	case wago.WasmF32:
		return "f32"
	case wago.WasmF64:
		return "f64"
	case wago.WasmV128:
		return "v128"
	case wago.WasmFuncRef:
		return "funcref"
	case wago.WasmExternRef:
		return "externref"
	default:
		panic(fmt.Sprintf("unsupported test carrier %d", carrier))
	}
}
