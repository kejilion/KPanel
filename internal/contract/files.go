package contract

import "time"

const MaxFileEntryBatch = 64

// MaxFileShareBytes bounds the O(file size) proof for anonymous file responses.
const MaxFileShareBytes int64 = 10 << 30

// Legacy single-request uploads retain their independent compatibility limit.
const MaxLegacyFileUploadBytes int64 = 512 << 20

// A full content proof remains bounded independently of the download lifetime.
const FileShareFingerprintTimeout = 5 * time.Minute

// Managed transfers and public shares have independent budgets even when their
// maximum file sizes match. Directory extraction keeps its own budget.
const MaxFileTransferBytes int64 = 10 << 30
const FileTransferChunkBytes = 8 << 20

// File and directory choices deliberately have independent task scope.
type FileConflictPolicies struct {
	File      string `json:"file,omitempty"`
	Directory string `json:"directory,omitempty"`
}

type FileConflict struct {
	Source  FileEntry `json:"source"`
	Target  FileEntry `json:"target"`
	Changed bool      `json:"changed,omitempty"`
}

type FileConflictDecision struct {
	Revision    uint64 `json:"revision"`
	Policy      string `json:"policy"`
	ApplyToRest bool   `json:"applyToRest,omitempty"`
}

type FileConflictResolution struct {
	DirectoryIdentity string `json:"directoryIdentity,omitempty"`
	Name              string `json:"name"`
	Overwrite         bool   `json:"overwrite,omitempty"`
	Merge             bool   `json:"merge,omitempty"`
	Skipped           bool   `json:"skipped,omitempty"`
	Version           string `json:"version,omitempty"`
}

type FileReceiveInput struct {
	ExpectedDirectoryIdentity     string                `json:"expectedDirectoryIdentity,omitempty"`
	RequestedName                 string                `json:"-"`
	ConflictPolicies              *FileConflictPolicies `json:"conflictPolicies,omitempty"`
	Directory                     string                `json:"directory"`
	Name                          string                `json:"name"`
	Kind                          string                `json:"kind"`
	SizeBytes                     int64                 `json:"sizeBytes"`
	SourceKey                     string                `json:"sourceKey"`
	Overwrite                     bool                  `json:"overwrite,omitempty"`
	ExpectedTargetResourceVersion string                `json:"expectedTargetResourceVersion,omitempty"`
	Mode                          string                `json:"mode,omitempty"`
	ModifiedAt                    *time.Time            `json:"modifiedAt,omitempty"`
}

type FileReceiveRequest struct {
	Decision  *FileConflictDecision `json:"decision,omitempty"`
	Operation string                `json:"operation"`
	ID        string                `json:"id,omitempty"`
	SourceKey string                `json:"sourceKey,omitempty"`
	Input     *FileReceiveInput     `json:"input,omitempty"`
	SizeBytes int64                 `json:"sizeBytes,omitempty"`
	SHA256    string                `json:"sha256,omitempty"`
}

type FileReceiveSession struct {
	PublishJobID string            `json:"publishJobId,omitempty"`
	Conflict     *FileConflict     `json:"conflict,omitempty"`
	Revision     uint64            `json:"revision,omitempty"`
	Result       *FileActionResult `json:"result,omitempty"`
	ID           string            `json:"id"`
	State        string            `json:"state"`
	Offset       int64             `json:"offset"`
	SizeBytes    int64             `json:"sizeBytes"`
	PrefixSHA256 string            `json:"prefixSha256"`
	ChunkBytes   int               `json:"chunkBytes"`
	ExpiresAt    time.Time         `json:"expiresAt"`
	Entry        *FileEntry        `json:"entry,omitempty"`
}

type FileEntry struct {
	DirectoryIdentity string    `json:"directoryIdentity,omitempty"`
	Name              string    `json:"name"`
	Path              string    `json:"path"`
	Kind              string    `json:"kind"`
	MIME              string    `json:"mime,omitempty"`
	SizeBytes         int64     `json:"sizeBytes"`
	Mode              string    `json:"mode"`
	Owner             string    `json:"owner"`
	Group             string    `json:"group"`
	ModifiedAt        time.Time `json:"modifiedAt"`
	ResourceVersion   string    `json:"resourceVersion"`
	Editable          bool      `json:"editable"`
	Previewable       bool      `json:"previewable"`
	OfficeEditable    bool      `json:"officeEditable,omitempty"`
	// ShareVersion is an Agent-local, content-bound proof used by public file shares.
	// It is deliberately excluded from ordinary file API responses.
	ShareVersion string `json:"-"`
}

type FileShareEntry struct {
	FileEntry
	ShareVersion string `json:"shareVersion"`
}

const FileShareVersionHeader = "X-KPanel-File-Share-Version"

type FileDirectory struct {
	FileConflictVersion            int         `json:"fileConflictVersion,omitempty"`
	Path                           string      `json:"path"`
	Entries                        []FileEntry `json:"entries"`
	Offset                         int         `json:"offset"`
	NextOffset                     int         `json:"nextOffset,omitempty"`
	Total                          int         `json:"total,omitempty"`
	TotalKnown                     bool        `json:"totalKnown,omitempty"`
	Truncated                      bool        `json:"truncated"`
	ScanTruncated                  bool        `json:"scanTruncated,omitempty"`
	ArchiveManagementAvailable     bool        `json:"archiveManagementAvailable,omitempty"`
	ArchiveMemberDownloadAvailable bool        `json:"archiveMemberDownloadAvailable,omitempty"`
	FileReceiveVersion             int         `json:"fileReceiveVersion,omitempty"`
	PanelFileReceiveVersion        int         `json:"panelFileReceiveVersion,omitempty"`
	ReadAt                         time.Time   `json:"readAt"`
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

type FileArchiveMemberDownload struct {
	Entries []string `json:"entries"`
	Raw     bool     `json:"raw,omitempty"`
}

type FileActionRequest struct {
	ExpectedDirectoryIdentity     string                `json:"expectedDirectoryIdentity,omitempty"`
	ConflictPolicies              *FileConflictPolicies `json:"conflictPolicies,omitempty"`
	SourceKind                    string                `json:"sourceKind,omitempty"`
	DestinationName               string                `json:"destinationName,omitempty"`
	Overwrite                     bool                  `json:"overwrite,omitempty"`
	ExpectedTargetResourceVersion string                `json:"expectedTargetResourceVersion,omitempty"`
	Action                        string                `json:"action"`
	Sources                       []string              `json:"sources,omitempty"`
	TrashIDs                      []string              `json:"trashIds,omitempty"`
	Target                        string                `json:"target,omitempty"`
	Name                          string                `json:"name,omitempty"`
	Mode                          string                `json:"mode,omitempty"`
	Format                        string                `json:"format,omitempty"`
	ExpectedResourceVersion       string                `json:"expectedResourceVersion,omitempty"`
	ExpectedResourceVersions      map[string]string     `json:"expectedResourceVersions,omitempty"`
	ArchiveEntries                []string              `json:"archiveEntries,omitempty"`
}

// Archive paths are relative member names, never host filesystem paths.
type FileArchiveEntry struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type FileArchiveQuery struct {
	Path            string `json:"path"`
	ResourceVersion string `json:"resourceVersion"`
	Directory       string `json:"directory,omitempty"`
	Search          string `json:"search,omitempty"`
	Offset          int    `json:"offset,omitempty"`
}

type FileArchiveDirectory struct {
	Path            string             `json:"path"`
	ResourceVersion string             `json:"resourceVersion"`
	Directory       string             `json:"directory"`
	Entries         []FileArchiveEntry `json:"entries"`
	Total           int                `json:"total"`
	NextOffset      int                `json:"nextOffset,omitempty"`
	Truncated       bool               `json:"truncated"`
}

type FileArchiveJob struct {
	Entry            *FileEntry            `json:"entry,omitempty"`
	ConflictPolicies *FileConflictPolicies `json:"conflictPolicies,omitempty"`
	Conflict         *FileConflict         `json:"conflict,omitempty"`
	Revision         uint64                `json:"revision,omitempty"`
	ID               string                `json:"id"`
	Action           string                `json:"action"`
	Name             string                `json:"name"`
	Target           string                `json:"target"`
	State            string                `json:"state"`
	Entries          int                   `json:"entries"`
	ProcessedBytes   int64                 `json:"processedBytes"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
	Result           FileActionResult      `json:"result"`
	Detail           string                `json:"detail,omitempty"`
	Sources          []string              `json:"sources"`
	ArchiveEntries   []string              `json:"archiveEntries,omitempty"`
	Format           string                `json:"format,omitempty"`
}

type FileArchiveJobRequest struct {
	Decision  *FileConflictDecision `json:"decision,omitempty"`
	Operation string                `json:"operation"`
	ID        string                `json:"id,omitempty"`
	Input     *FileActionRequest    `json:"input,omitempty"`
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
	Code   string `json:"code,omitempty"`
	Path   string `json:"path"`
	Detail string `json:"detail"`
}

type FileActionResult struct {
	CompletedCount int                     `json:"completedCount,omitempty"`
	SkippedCount   int                     `json:"skippedCount,omitempty"`
	Skipped        []string                `json:"skipped,omitempty"`
	Resolution     *FileConflictResolution `json:"resolution,omitempty"`
	Action         string                  `json:"action"`
	Succeeded      []FileActionItem        `json:"succeeded"`
	Failed         []FileActionFailure     `json:"failed"`
}

type FileWriteRequest struct {
	Content                 string       `json:"content"`
	ExpectedResourceVersion string       `json:"expectedResourceVersion"`
	OfficeEdits             []OfficeEdit `json:"officeEdits,omitempty"`
	ExpectedContentVersion  string       `json:"expectedContentVersion,omitempty"`
}

// Office documents expose a bounded content view, never executable HTML or file URLs.
type OfficeDocument struct {
	Entry          FileEntry       `json:"entry"`
	Kind           string          `json:"kind"`
	ContentVersion string          `json:"contentVersion"`
	Sections       []OfficeSection `json:"sections"`
	Notes          []string        `json:"notes"`
}

type OfficeSection struct {
	Name    string       `json:"name"`
	Items   []OfficeItem `json:"items"`
	Width   float64      `json:"width,omitempty"`
	Height  float64      `json:"height,omitempty"`
	Rows    int          `json:"rows,omitempty"`
	Columns int          `json:"columns,omitempty"`
}

type OfficeItem struct {
	ID       string         `json:"id,omitempty"`
	Kind     string         `json:"kind"`
	Text     string         `json:"text"`
	Editable bool           `json:"editable"`
	Row      int            `json:"row,omitempty"`
	Column   int            `json:"column,omitempty"`
	X        float64        `json:"x,omitempty"`
	Y        float64        `json:"y,omitempty"`
	Width    float64        `json:"width,omitempty"`
	Height   float64        `json:"height,omitempty"`
	Bold     bool           `json:"bold,omitempty"`
	Italic   bool           `json:"italic,omitempty"`
	FontSize float64        `json:"fontSize,omitempty"`
	Align    string         `json:"align,omitempty"`
	Image    string         `json:"image,omitempty"`
	Formula  string         `json:"formula,omitempty"`
	Table    [][]OfficeItem `json:"table,omitempty"`
}

type OfficeEdit struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type FileWriteResult struct {
	Entry FileEntry `json:"entry"`
}

type FileTransferMetadata struct {
	Name            string     `json:"name"`
	Kind            string     `json:"kind"`
	SizeBytes       int64      `json:"sizeBytes"`
	ResourceVersion string     `json:"resourceVersion"`
	Offset          int64      `json:"offset,omitempty"`
	TransferVersion int        `json:"transferVersion,omitempty"`
	Mode            string     `json:"mode,omitempty"`
	ModifiedAt      *time.Time `json:"modifiedAt,omitempty"`
}

type FileTransferRequest struct {
	ConflictPolicies *FileConflictPolicies `json:"conflictPolicies,omitempty"`
	BatchID          string                `json:"batchId,omitempty"`
	SourceNodeID     string                `json:"sourceNodeId"`
	Path             string                `json:"path"`
	ResourceVersion  string                `json:"resourceVersion"`
	TargetDirectory  string                `json:"targetDirectory"`
	Background       bool                  `json:"background,omitempty"`
}

type FileRemoteDownloadRequest struct {
	ConflictPolicies *FileConflictPolicies `json:"conflictPolicies,omitempty"`
	URL              string                `json:"url"`
	TargetDirectory  string                `json:"targetDirectory"`
	Name             string                `json:"name,omitempty"`
	Background       bool                  `json:"background,omitempty"`
	Acceleration     string                `json:"acceleration,omitempty"`
	Torrent          []byte                `json:"torrent,omitempty"`
	SourceKind       string                `json:"sourceKind,omitempty"`
}

type FileRemoteDownloadJob struct {
	Result           *FileActionResult     `json:"result,omitempty"`
	ConflictPolicies *FileConflictPolicies `json:"conflictPolicies,omitempty"`
	Conflict         *FileConflict         `json:"conflict,omitempty"`
	Decision         *FileConflictDecision `json:"decision,omitempty"`
	Revision         uint64                `json:"revision,omitempty"`
	BatchID          string                `json:"batchId,omitempty"`
	ID               string                `json:"id"`
	State            string                `json:"state"`
	Source           string                `json:"source"`
	SourceKind       string                `json:"sourceKind,omitempty"`
	TargetHostID     string                `json:"targetHostId,omitempty"`
	TargetDirectory  string                `json:"targetDirectory"`
	Name             string                `json:"name,omitempty"`
	LoadedBytes      int64                 `json:"loadedBytes,omitempty"`
	TotalBytes       int64                 `json:"totalBytes,omitempty"`
	Entry            *FileEntry            `json:"entry,omitempty"`
	Code             string                `json:"code,omitempty"`
	TransferMode     string                `json:"transferMode,omitempty"`
	Peers            int                   `json:"peers,omitempty"`
	SpeedBytes       int64                 `json:"speedBytes,omitempty"`
	SourceBytes      int64                 `json:"sourceBytes,omitempty"`
	CreatedAt        time.Time             `json:"createdAt"`
	UpdatedAt        time.Time             `json:"updatedAt"`
	FinishedAt       *time.Time            `json:"finishedAt,omitempty"`
}

type FileRemoteDownloadJobList struct {
	Items []FileRemoteDownloadJob `json:"items"`
}

type FileTransferEvent struct {
	Result       *FileActionResult `json:"result,omitempty"`
	Conflict     *FileConflict     `json:"conflict,omitempty"`
	State        string            `json:"state"`
	LoadedBytes  int64             `json:"loadedBytes,omitempty"`
	TotalBytes   int64             `json:"totalBytes,omitempty"`
	Name         string            `json:"name,omitempty"`
	Entry        *FileEntry        `json:"entry,omitempty"`
	Code         string            `json:"code,omitempty"`
	Detail       string            `json:"detail,omitempty"`
	TransferMode string            `json:"transferMode,omitempty"`
	Peers        int               `json:"peers,omitempty"`
	SpeedBytes   int64             `json:"speedBytes,omitempty"`
	SourceBytes  int64             `json:"sourceBytes,omitempty"`
}
