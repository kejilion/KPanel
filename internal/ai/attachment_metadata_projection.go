package ai

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"modernc.org/sqlite"
)

// Register during package initialization, before runtime connections can open:
// the driver's function registry is process-wide and has no registration lock.
var attachmentMetadataProjectionErr = sqlite.RegisterFunction("kpanel_ai_attachment_metadata_v1", &sqlite.FunctionImpl{
	NArgs: 1, Deterministic: true, VolatileArgs: true,
	Scalar: projectAttachmentMetadata,
})

func projectAttachmentMetadata(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if len(args) != 1 {
		return nil, errors.New("attachment metadata projection requires one BLOB")
	}
	var data []byte
	if args[0] != nil {
		var ok bool
		data, ok = args[0].([]byte)
		if !ok {
			return nil, errors.New("attachment metadata projection requires a BLOB")
		}
	}
	// The argument borrows SQLite memory only for this callback. Decode and
	// serialize synchronously; only owned metadata leaves the call, never Data.
	// Do not reenter SQLite or hand any borrowed value to another goroutine.
	items, err := decodeAttachmentMetadata(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(items)
}

func attachmentMetadataProjectionSQL() string {
	// CAST preserves historical TEXT rows while ensuring a borrowed BLOB input.
	// Keep the original byte guard before evaluating the projection; callers
	// still read the original length to preserve their domain-specific errors.
	return fmt.Sprintf("CASE WHEN length(CAST(attachments_json AS BLOB))<=%d THEN kpanel_ai_attachment_metadata_v1(CAST(attachments_json AS BLOB)) ELSE NULL END", maxAttachmentReadBytes)
}

func decodeAttachmentMetadataProjection(data []byte) ([]Attachment, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var items []Attachment
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("decode projected attachment metadata: %w", err)
	}
	return items, nil
}
