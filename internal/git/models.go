package git

// StatusCode represents single-character Git porcelain status codes.
type StatusCode string

const (
	StatusUnmodified StatusCode = " "
	StatusModified   StatusCode = "M"
	StatusAdded      StatusCode = "A"
	StatusDeleted    StatusCode = "D"
	StatusRenamed    StatusCode = "R"
	StatusCopied     StatusCode = "C"
	StatusUntracked  StatusCode = "?"
	StatusIgnored    StatusCode = "!"
	StatusUnmerged   StatusCode = "U"
)

// FileStatus represents the status of an individual file in index and working tree.
type FileStatus struct {
	Path           string     `json:"path"`
	OrigPath       string     `json:"orig_path,omitempty"` // For renames and copies
	StagingStatus  StatusCode `json:"staging_status"`      // Status in index ('M', 'A', 'D', 'R', etc.)
	WorkTreeStatus StatusCode `json:"worktree_status"`     // Status in working tree ('M', 'D', '?', etc.)
}

// StatusResult contains parsed output from git status.
type StatusResult struct {
	StagedFiles    []FileStatus `json:"staged_files"`
	UnstagedFiles  []FileStatus `json:"unstaged_files"`
	UntrackedFiles []string     `json:"untracked_files"`
	UnmergedFiles  []string     `json:"unmerged_files"`
	IsClean        bool         `json:"is_clean"`
}

// RepositoryState represents a complete live snapshot of the git repository.
type RepositoryState struct {
	RootPath          string       `json:"root_path"`
	Branch            string       `json:"branch"`
	CommitHash        string       `json:"commit_hash"`
	IsDetached        bool         `json:"is_detached"`
	HasCommits        bool         `json:"has_commits"`
	IsClean           bool         `json:"is_clean"`
	HasMergeConflicts bool         `json:"has_merge_conflicts"`
	StagedFiles       []FileStatus `json:"staged_files"`
	UnstagedFiles     []FileStatus `json:"unstaged_files"`
	UntrackedFiles    []string     `json:"untracked_files"`
	UnmergedFiles     []string     `json:"unmerged_files"`
	ModifiedFiles     []string     `json:"modified_files"` // Consolidated list of all altered paths
}

// FileChange represents a file modified between two commits.
type FileChange struct {
	Path     string     `json:"path"`
	OrigPath string     `json:"orig_path,omitempty"`
	Status   StatusCode `json:"status"` // 'M', 'A', 'D', 'R', etc.
}
