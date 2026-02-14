//go:build !govips || !cgo

package pipeline

func Startup() error {
	return nil
}

func Shutdown() {}

func newTransformer() (Transformer, error) {
	return stdlibTransformer{}, nil
}
