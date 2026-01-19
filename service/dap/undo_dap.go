package dap

// Type definitions for Undo's DAP protocol extensions.

import "github.com/google/go-dap"

type StepOverBackRequest struct {
	dap.Request

	Arguments dap.StepBackArguments
}

type StepOutBackRequest struct {
	dap.Request

	Arguments dap.StepBackArguments
}

type StepOverBackResponse struct {
	dap.Response
}

type StepOutBackResponse struct {
	dap.Response
}

func makeUndoDapCodec() *dap.Codec {
	codec := dap.NewCodec()
	codec.RegisterRequest("undo/stepOverBack",
		func() dap.Message { return &StepOverBackRequest{} },
		func() dap.Message { return &StepOverBackResponse{} },
	)

	codec.RegisterRequest("undo/stepOutBack",
		func() dap.Message { return &StepOutBackRequest{} },
		func() dap.Message { return &StepOutBackResponse{} },
	)

	return codec
}
