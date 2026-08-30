package stash

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"wake/internal/git"
)

type Engine struct {
	repoRoot  string
	gitClient git.Client
	hashCache map[string]string // map[filepath]sha256hash
}

func NewEngine(repoRoot string, client git.Client) *Engine {
	return &Engine{
		repoRoot:  repoRoot,
		gitClient: client,
		hashCache: make(map[string]string),
	}
}

// StashModifiedFiles reads the git index for uncommitted changes and securely backs them up.
func (e *Engine) StashModifiedFiles(ctx context.Context) error {
	status, err := e.gitClient.GetStatus(ctx, e.repoRoot)
	if err != nil {
		return err
	}

	filesToCheck := git.ExtractModifiedFiles(status)
	if len(filesToCheck) == 0 {
		return nil
	}

	stashDir := filepath.Join(e.repoRoot, ".wake", "recovery_stash")

	for _, relPath := range filesToCheck {
		// Ignore changes inside the wake folder itself
		if strings.HasPrefix(relPath, ".wake/") || relPath == ".wake" || strings.HasPrefix(relPath, ".git/") || relPath == ".git" {
			continue
		}

		absPath := filepath.Join(e.repoRoot, relPath)
		fileInfo, err := os.Lstat(absPath)
		if err != nil || fileInfo.IsDir() || fileInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}

		// Calculate file hash
		hash, err := hashFile(absPath)
		if err != nil {
			continue
		}

		// If we already stashed this exact file content, skip
		if prevHash, ok := e.hashCache[relPath]; ok && prevHash == hash {
			continue
		}

		// New changes detected! Copy to stash folder.
		targetPath := filepath.Join(stashDir, relPath+fmt.Sprintf("_%d.bak", time.Now().Unix()))
		if err := copyFile(absPath, targetPath); err == nil {
			// Only update cache if copy succeeds
			e.hashCache[relPath] = hash
		}
	}

	_ = e.cleanupStash(stashDir)

	return nil
}

func (e *Engine) cleanupStash(stashDir string) error {
	const maxStashFiles = 50
	const maxStashAge = 24 * time.Hour

	entries, err := os.ReadDir(stashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var files []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry)
		}
	}

	sort.Slice(files, func(i, j int) bool {
		infoI, errI := files[i].Info()
		infoJ, errJ := files[j].Info()
		if errI != nil || errJ != nil {
			return false
		}
		// Sort newest first
		return infoI.ModTime().After(infoJ.ModTime())
	})

	for i, entry := range files {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		
		path := filepath.Join(stashDir, entry.Name())
		if i >= maxStashFiles || time.Since(info.ModTime()) > maxStashAge {
			_ = os.Remove(path)
		}
	}
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > 100*1024*1024 { // 100MB limit
		return "", fmt.Errorf("file too large to hash: %d bytes", info.Size())
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0750); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
