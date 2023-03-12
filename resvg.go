package resvg

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"errors"
	"io"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

//go:generate go run src/gen.go

// wasmzip was compiled using `cargo build --release --target wasm32-unknown-unknown`
// and packed into zip
//
//go:embed target/wasm32-unknown-unknown/release/resvg_go.wasm.zip
var wasmzip []byte
var wasmzr, _ = zip.NewReader(bytes.NewReader(wasmzip), int64(len(wasmzip)))

// instance instance
type instance struct {
	ctx context.Context
	mod api.Module
}

var (
	// ErrOutOfRange wasm memory out of range
	ErrOutOfRange = errors.New("wasm memory out of range")
	// ErrNullWasmPointer null wasm pointer
	ErrNullWasmPointer = errors.New("null wasm pointer")
)

// DefaultResvg DefaultResvg
// instance
func DefaultResvg() (*instance, error) {
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	f, err := wasmzr.Open("resvg_go.wasm")
	if err != nil {
		return nil, err
	}
	wasm, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	inst, err := r.Instantiate(ctx, wasm)
	if err != nil {
		return nil, err
	}
	return &instance{ctx, inst}, nil
}

func (inst *instance) ResvgRender(tree *UsvgTree, ft *UsvgFitTo, tf *TinySkiaTransform, pixmap *TinySkiaPixmap) error {
	if tree.free || ft.free || tf.free || pixmap.free {
		return ErrNullWasmPointer
	}
	fn := inst.mod.ExportedFunction("__resvg_render")
	_, err := fn.Call(
		inst.ctx,
		api.EncodeI32(tree.ptr),
		api.EncodeI32(ft.ptr),
		api.EncodeI32(tf.ptr),
		api.EncodeI32(pixmap.ptr),
	)
	if err != nil {
		return err
	}
	ft.free = true
	tf.free = true
	return nil
}

func (inst *instance) DefaultResvgRenderToPNG(svg []byte, font ...[]byte) ([]byte, error) {
	inst, err := DefaultResvg()
	if err != nil {
		return nil, err
	}
	opt, err := inst.UsvgOptionsDefault()
	if err != nil {
		return nil, err
	}
	defer opt.Free()
	tree, err := inst.UsvgTreeFromData(svg, opt)
	if err != nil {
		return nil, err
	}
	defer tree.Free()
	db, err := inst.NewFontdbDatabase()
	if err != nil {
		return nil, err
	}
	defer db.Free()
	for i := range font {
		err = db.LoadFontData(font[i])
		if err != nil {
			return nil, err
		}
	}
	err = tree.ConvertText(db, true)
	if err != nil {
		return nil, err
	}
	ft, err := inst.UsvgFitToZoom(1.0)
	if err != nil {
		return nil, err
	}
	tf, err := inst.TinySkiaTransformDefault()
	if err != nil {
		return nil, err
	}
	size, err := tree.GetSizeClone()
	if err != nil {
		return nil, err
	}
	defer size.Free()
	screenSize, err := size.ToScreenSize()
	if err != nil {
		return nil, err
	}
	defer screenSize.Free()
	width, err := screenSize.Width()
	if err != nil {
		return nil, err
	}
	height, err := screenSize.Height()
	if err != nil {
		return nil, err
	}
	pixmap, err := inst.NewTinySkiaPixmap(width, height)
	if err != nil {
		return nil, err
	}
	defer pixmap.Free()
	err = inst.ResvgRender(tree, ft, tf, pixmap)
	if err != nil {
		return nil, err
	}
	out, err := pixmap.EncodePNG()
	if err != nil {
		return nil, err
	}
	return out, nil
}
