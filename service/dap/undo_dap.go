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

type GotoStartRequest struct {
	dap.Request
}

type GotoEndRequest struct {
	dap.Request
}

type StepOverBackResponse struct {
	dap.Response
}

type StepOutBackResponse struct {
	dap.Response
}

type GotoStartResponse struct {
	dap.Response
}

type GotoEndResponse struct {
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

	codec.RegisterRequest("undo/gotoStart",
		func() dap.Message { return &GotoStartRequest{} },
		func() dap.Message { return &GotoStartResponse{} },
	)

	codec.RegisterRequest("undo/gotoEnd",
		func() dap.Message { return &GotoEndRequest{} },
		func() dap.Message { return &GotoEndResponse{} },
	)

	return codec
}
