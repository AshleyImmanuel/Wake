package hashfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"wake/internal/git"
)

// Client provides a Git-less file hashing fallback for environments
// where Git is not installed or the directory is not a Git repository.
// It uses SHA-256 hashing to track file state changes.
type Client struct{}

// NewClient creates a new hashfs Client that satisfies the git.Client interface.
func NewClient() git.Client {
	return &Client{}
}

func (c *Client) GetRepoRoot(ctx context.Context, dir string) (string, error) {
	return dir, nil
}

func (c *Client) GetCurrentCommit(ctx context.Context, repoPath string) (string, error) {
	return "hashfs-commit", nil
}

func (c *Client) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	return "main", nil
}

func (c *Client) GetStatus(ctx context.Context, repoPath string) (*git.StatusResult, error) {
	indexPath := filepath.Join(repoPath, ".wake", "hash_index.json")

	// Read old index
	oldHashes := make(map[string]string)
	if data, err := os.ReadFile(indexPath); err == nil {
		json.Unmarshal(data, &oldHashes)
	}

	newHashes := make(map[string]string)
	var modified []string
	var untracked []string

	filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				name := d.Name()
				if name == ".wake" || name == ".git" || name == "node_modules" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			return nil // skip symlinks
		}

		rel, err := filepath.Rel(repoPath, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		hash, err := hashFile(path)
		if err != nil {
			return nil
		}
		newHashes[rel] = hash

		if oldHash, exists := oldHashes[rel]; exists {
			if oldHash != hash {
				modified = append(modified, rel)
			}
		} else {
			untracked = append(untracked, rel)
		}
		return nil
	})

	for oldPath := range oldHashes {
		if _, exists := newHashes[oldPath]; !exists {
			modified = append(modified, oldPath)
		}
	}

	// Save new index
	if data, err := json.Marshal(newHashes); err == nil {
		os.MkdirAll(filepath.Dir(indexPath), 0700)
		os.WriteFile(indexPath, data, 0600)
	}

	result := &git.StatusResult{
		IsClean:        len(modified) == 0 && len(untracked) == 0,
		UntrackedFiles: untracked,
	}

	for _, m := range modified {
		result.UnstagedFiles = append(result.UnstagedFiles, git.FileStatus{Path: m})
	}

	return result, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Stubs for remaining interface methods
func (c *Client) GetState(ctx context.Context, repoPath string) (*git.RepositoryState, error) {
	status, _ := c.GetStatus(ctx, repoPath)
	return &git.RepositoryState{
		RootPath:       repoPath,
		Branch:         "main",
		CommitHash:     "hashfs-commit",
		HasCommits:     true,
		IsClean:        status.IsClean,
		UnstagedFiles:  status.UnstagedFiles,
		UntrackedFiles: status.UntrackedFiles,
		ModifiedFiles:  git.ExtractModifiedFiles(status),
	}, nil
}

func (c *Client) GetDiff(ctx context.Context, repoPath string, staged bool) (string, error) {
	return "", nil
}
func (c *Client) GetFileDiff(ctx context.Context, repoPath string, filePath string) (string, error) {
	return "", nil
}
func (c *Client) GetFileAtCommit(ctx context.Context, repoPath, filePath, commitHash string) (string, error) {
	return "", nil
}
func (c *Client) GetDiffBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) (string, error) {
	return "", nil
}
func (c *Client) GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error) {
	return nil, nil
}
func (c *Client) IsClean(ctx context.Context, repoPath string) (bool, error) {
	status, _ := c.GetStatus(ctx, repoPath)
	return status.IsClean, nil
}
func (c *Client) CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error) {
	return true, nil
}
func (c *Client) IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error) {
	return true, nil
}
