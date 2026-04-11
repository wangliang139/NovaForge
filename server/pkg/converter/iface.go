package converter

import (
	"time"

	"github.com/wangliang139/NovaForge/server/pkg/repos/calendar"
	"github.com/wangliang139/NovaForge/server/pkg/repos/document"
	"github.com/wangliang139/NovaForge/server/pkg/types"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// goverter:converter
// goverter:output:format function
// goverter:output:file ./generated.go
// goverter:matchIgnoreCase
// goverter:skipCopySameType
// goverter:ignoreUnexported
// goverter:enum:unknown @error
// goverter:extend ConvertTimeToPbTimestamp
// goverter:extend ConvertTimePtrToPbTimestamp
// goverter:extend ConvertTimeToMilliSecondInt64
// goverter:extend DocumentCatalogPb2Repo
// goverter:extend DocumentStatusPb2Repo
// goverter:extend DocumentCatalogRepo2Pb
// goverter:extend DocumentFormatRepo2Pb
// goverter:extend DocumentStatusRepo2Pb
// goverter:extend CalendarSourcePb2Repo
// goverter:extend CalendarSourceRepo2Pb
// goverter:extend CalendarTypePb2Repo
// goverter:extend CalendarTypeRepo2Pb
// goverter:extend AccountStatusTypes2Pb
// goverter:extend AccountStatusPb2Types
// goverter:extend AuthAlgorithmTypes2Pb
type Converter interface {
	DocumentRepo2Types(repo *document.Document) *types.Document
	DocumentGetByIdRowRepo2Types(repo *document.GetByIdRow) *types.Document
	DocumentSaveDraftToPendingRowRepo2Types(repo *document.SaveDraftToPendingRow) *types.Document
	DocumentSaveAiSummaryRowRepo2Types(repo *document.SaveAiSummaryRow) *types.Document
	DocumentArchiveDocumentRowRepo2Types(repo *document.ArchiveDocumentRow) *types.Document
	DocumentQueryDocumentsRowRepo2Types(repo *document.QueryDocumentsRow) *types.Document
	CalendarRepo2Types(repo *calendar.Calendar) *types.Calendar
	// goverter:map . Ext | CalendarExtentionRepo2Pb
}

func ConvertTimeToPbTimestamp(time time.Time) *timestamppb.Timestamp {
	return &timestamppb.Timestamp{
		Seconds: time.Unix(),
		Nanos:   int32(time.Nanosecond()),
	}
}

func ConvertTimePtrToPbTimestamp(time *time.Time) *timestamppb.Timestamp {
	if time == nil {
		return nil
	}
	return &timestamppb.Timestamp{
		Seconds: time.Unix(),
		Nanos:   int32(time.Nanosecond()),
	}
}

func ConvertTimeToSecondInt64(time time.Time) int64 {
	return time.Unix()
}

func ConvertTimeToMilliSecondInt64(time time.Time) int64 {
	return time.UnixMilli()
}
