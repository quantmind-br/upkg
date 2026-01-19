package db

import (
	"encoding/json"

	"github.com/quantmind-br/upkg/internal/core"
)

// ToInstallRecord converts a db.Install to core.InstallRecord
//
//nolint:gocyclo // metadata decoding handles several legacy shapes.
func ToInstallRecord(dbRecord *Install) *core.InstallRecord {
	record := &core.InstallRecord{
		InstallID:    dbRecord.InstallID,
		PackageType:  core.PackageType(dbRecord.PackageType),
		Name:         dbRecord.Name,
		Version:      dbRecord.Version,
		InstallDate:  dbRecord.InstallDate,
		OriginalFile: dbRecord.OriginalFile,
		InstallPath:  dbRecord.InstallPath,
		DesktopFile:  dbRecord.DesktopFile,
		Metadata:     core.Metadata{},
	}

	// Convert metadata map to JSON and unmarshal into typed Metadata struct
	// This leverages the custom UnmarshalJSON method on core.Metadata
	if dbRecord.Metadata != nil {
		metadataJSON, err := json.Marshal(dbRecord.Metadata)
		if err == nil {
			// Ignore unmarshal errors - we'll fall back to empty metadata
			_ = json.Unmarshal(metadataJSON, &record.Metadata)
		}
	}

	// Set default install method if not specified
	if record.Metadata.InstallMethod == "" {
		record.Metadata.InstallMethod = core.InstallMethodLocal
	}

	return record
}
