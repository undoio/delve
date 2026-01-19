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

type CheckpointArgs struct {
	CheckpointId int `json:"id"`
}

type GotoCheckpointRequest struct {
	dap.Request

	Arguments CheckpointArgs
}

type ListCheckpointsRequest struct {
	dap.Request
}

type CreateCheckpointRequest struct {
	dap.Request

	Arguments struct {
		Label string `json:"label"`
	}
}

type DeleteCheckpointRequest struct {
	dap.Request

	Arguments CheckpointArgs
}

type LastValueRequest struct {
	dap.Request

	Arguments struct {
		Expression string `json:"expression"`
		FrameId    int    `json:"frameId"`
		ThreadId   int    `json:"threadId"`
	}
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

type GotoCheckpointResponse struct {
	dap.Response
}

type CreateCheckpointResponseBody struct {
	Id int `json:"id"`
}

type CreateCheckpointResponse struct {
	dap.Response

	Body CreateCheckpointResponseBody `json:"body"`
}

type DeleteCheckpointResponse struct {
	dap.Response
}

type Checkpoint struct {
	Id    int    `json:"id"`
	Label string `json:"label"`
	Time  string `json:"time"`
}

type ListCheckpointsBody struct {
	Checkpoints []Checkpoint `json:"checkpoints"`
}

type ListCheckpointsResponse struct {
	dap.Response

	Body ListCheckpointsBody `json:"body"`
}

type LastValueResult struct {
	Found bool
}

type LastValueResponse struct {
	dap.Response

	Body LastValueResult `json:"body"`
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

	codec.RegisterRequest("undo/gotoCheckpoint",
		func() dap.Message { return &GotoCheckpointRequest{} },
		func() dap.Message { return &GotoCheckpointResponse{} },
	)

	codec.RegisterRequest("undo/createCheckpoint",
		func() dap.Message { return &CreateCheckpointRequest{} },
		func() dap.Message { return &CreateCheckpointResponse{} },
	)

	codec.RegisterRequest("undo/deleteCheckpoint",
		func() dap.Message { return &DeleteCheckpointRequest{} },
		func() dap.Message { return &DeleteCheckpointResponse{} },
	)

	codec.RegisterRequest("undo/listCheckpoints",
		func() dap.Message { return &ListCheckpointsRequest{} },
		func() dap.Message { return &ListCheckpointsResponse{} },
	)

	codec.RegisterRequest("undo/lastValue",
		func() dap.Message { return &LastValueRequest{} },
		func() dap.Message { return &LastValueResponse{} },
	)

	return codec
}
