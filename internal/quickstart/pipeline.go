package quickstart

import (
	"context"
	"fmt"

	"github.com/wujun4code/clashforge/internal/config"
	"github.com/wujun4code/clashforge/internal/core"
	"github.com/wujun4code/clashforge/internal/netfilter"
	"github.com/wujun4code/clashforge/internal/publish"
	"github.com/wujun4code/clashforge/internal/subscription"
	"github.com/wujun4code/clashforge/internal/workernode"
)

// EventWriter is the callback pipelines use to emit progress events.
type EventWriter func(e Event)

// Pipeline is the common interface for CF Workers and VPS deploy paths.
type Pipeline interface {
	// Run executes the full deploy sequence, emitting events via out.
	// Returns nil on success; the final "error" event is emitted before returning.
	Run(ctx context.Context, req *DeployRequest, out EventWriter) error
}

// Deps holds the external services injected into pipelines.
type Deps struct {
	DataDir      string
	ConfigPath   string
	Config       *config.MetaclashConfig
	Core         *core.CoreManager
	SubManager   *subscription.Manager
	WorkerStore  *workernode.Store
	PublishStore *publish.Store
	Netfilter    *netfilter.Manager // used to apply transparent-proxy rules after core starts
}

// NewPipeline returns the pipeline for the requested deploy type.
func NewPipeline(dt DeployType, deps Deps) (Pipeline, error) {
	switch dt {
	case DeployTypeCFWorkers:
		return &WorkersPipeline{deps: deps}, nil
	case DeployTypeVPS:
		return &VPSPipeline{deps: deps}, nil
	default:
		return nil, fmt.Errorf("unknown deploy_type: %q", dt)
	}
}

// emit is a helper that constructs and dispatches an Event.
func emit(out EventWriter, phase Phase, step string, status EventStatus, msg string, detail ...string) {
	d := ""
	if len(detail) > 0 {
		d = detail[0]
	}
	out(Event{Phase: phase, Step: step, Status: status, Message: msg, Detail: d})
}
