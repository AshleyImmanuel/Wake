package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScanDirectory walks the given directory and returns a map of file paths to their size and mod time.
// It skips .git, .wake, and other internal metadata directories.
func ScanDirectory(root string) (map[string]string, error) {
	files := make(map[string]string)
	
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors (e.g. permission denied)
		}
		
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		
		rel = strings.ReplaceAll(rel, "\\", "/")
		
		if info.IsDir() {
			if rel == ".git" || rel == ".wake" || strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, ".wake/") {
				return filepath.SkipDir
			}
			return nil
		}
		
		if rel == ".git" || rel == ".wake" {
			return nil
		}
		
		files[rel] = fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
		return nil
	})
	
	return files, err
}

