package contract

import "time"

const MaxFileEntryBatch = 64

type FileEntry struct {
	Name            string    `json:"name"`
	Path            string    `json:"path"`
	Kind            string    `json:"kind"`
	MIME            string    `json:"mime,omitempty"`
	SizeBytes       int64     `json:"sizeBytes"`
	Mode            string    `json:"mode"`
	Owner           string    `json:"owner"`
	Group           string    `json:"group"`
	ModifiedAt      time.Time `json:"modifiedAt"`
	ResourceVersion string    `json:"resourceVersion"`
	Editable        bool      `json:"editable"`
	Previewable     bool      `json:"previewable"`
}

type FileDirectory struct {
	Path          string      `json:"path"`
	Entries       []FileEntry `json:"entries"`
	Offset        int         `json:"offset"`
	NextOffset    int         `json:"nextOffset,omitempty"`
	Total         int         `json:"total,omitempty"`
	TotalKnown    bool        `json:"totalKnown,omitempty"`
	Truncated     bool        `json:"truncated"`
	ScanTruncated bool        `json:"scanTruncated,omitempty"`
	ReadAt        time.Time   `json:"readAt"`
}

type FileEntryBatchRequest struct {
	Paths []string `json:"paths"`
}

type FileEntryBatchResult struct {
	Entries     []FileEntry `json:"entries"`
	Unavailable []string    `json:"unavailable"`
}

type FileArchiveDownloadRequest struct {
	Sources                  []string          `json:"sources"`
	ExpectedResourceVersions map[string]string `json:"expectedResourceVersions,omitempty"`
}

type FileActionRequest struct {
	Action                   string            `json:"action"`
	Sources                  []string          `json:"sources,omitempty"`
	TrashIDs                 []string          `json:"trashIds,omitempty"`
	Target                   string            `json:"target,omitempty"`
	Name                     string            `json:"name,omitempty"`
	Mode                     string            `json:"mode,omitempty"`
	Format                   string            `json:"format,omitempty"`
	ExpectedResourceVersion  string            `json:"expectedResourceVersion,omitempty"`
	ExpectedResourceVersions map[string]string `json:"expectedResourceVersions,omitempty"`
}

type FileTrashEntry struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	OriginalPath    string    `json:"originalPath,omitempty"`
	Kind            string    `json:"kind"`
	SizeBytes       int64     `json:"sizeBytes"`
	Mode            string    `json:"mode"`
	Owner           string    `json:"owner"`
	Group           string    `json:"group"`
	DeletedAt       time.Time `json:"deletedAt"`
	ResourceVersion string    `json:"resourceVersion"`
	Restorable      bool      `json:"restorable"`
}

type FileTrashDirectory struct {
	Entries   []FileTrashEntry `json:"entries"`
	Total     int              `json:"total"`
	Truncated bool             `json:"truncated"`
	ReadAt    time.Time        `json:"readAt"`
}

type FileActionItem struct {
	Path            string `json:"path"`
	Destination     string `json:"destination,omitempty"`
	ResourceVersion string `json:"resourceVersion,omitempty"`
}

type FileActionFailure struct {
	Path   string `json:"path"`
	Detail string `json:"detail"`
}

type FileActionResult struct {
	Action    string              `json:"action"`
	Succeeded []FileActionItem    `json:"succeeded"`
	Failed    []FileActionFailure `json:"failed"`
}

type FileWriteRequest struct {
	Content                 string `json:"content"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

type FileWriteResult struct {
	Entry FileEntry `json:"entry"`
}

type FileTransferMetadata struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	SizeBytes       int64  `json:"sizeBytes"`
	ResourceVersion string `json:"resourceVersion"`
}

type FileTransferRequest struct {
	SourceNodeID    string `json:"sourceNodeId"`
	Path            string `json:"path"`
	ResourceVersion string `json:"resourceVersion"`
	TargetDirectory string `json:"targetDirectory"`
}

type FileTransferEvent struct {
	State       string     `json:"state"`
	LoadedBytes int64      `json:"loadedBytes,omitempty"`
	TotalBytes  int64      `json:"totalBytes,omitempty"`
	Entry       *FileEntry `json:"entry,omitempty"`
	Detail      string     `json:"detail,omitempty"`
}
