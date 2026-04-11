package timeline

import (
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/external"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/internal"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/sorter"
	"github.com/wangliang139/NovaForge/server/pkg/strategy/infra/timeline/types"
	mdtypes "github.com/wangliang139/NovaForge/server/pkg/strategy/types"
)

type (
	ExternalMerger       = types.ExternalMerger
	Frame                = types.Frame
	SourceError          = mdtypes.SourceError
	ErrorPolicy          = types.ErrorPolicy
	SorterConfig         = sorter.SorterConfig
	ExternalMergerConfig = external.ExternalMergerConfig
	InternalQueue        = internal.InternalQueue
)

var (
	ErrorPolicyFailFast = types.ErrorPolicyFailFast
	ErrorPolicyDegrade  = types.ErrorPolicyDegrade

	ErrInvalidInternalEvent = mdtypes.ErrInvalidInternalEvent
	ErrInvalidInternalSeq   = mdtypes.ErrInvalidInternalSeq

	DefaultSorterConfig = sorter.DefaultSorterConfig

	NewInternalQueue  = internal.NewInternalQueue
	NewExternalMerger = external.NewExternalMerger
)
