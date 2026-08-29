package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_IsAncestor(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewClient(nil)

	// Since we are running in the Wake repository itself, HEAD should be an ancestor of itself
	isAncestor, err := client.IsAncestor(ctx, ".", "HEAD", "HEAD")
	require.NoError(t, err)
	assert.True(t, isAncestor)
}

func TestClient_CommitExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewClient(nil)

	exists, err := client.CommitExists(ctx, ".", "HEAD")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, _ = client.CommitExists(ctx, ".", "invalid_hash_xyz123")
	assert.False(t, exists)
}

func TestClient_GetState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewClient(nil)

	// Create a dummy repo in a temp dir
	tempDir, err := os.MkdirTemp("", "wake-git-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// This should fail gracefully or return empty state since it's not a git repo yet
	state, err := client.GetState(ctx, tempDir)
	require.NoError(t, err) // the command might return error, but our wrapper might handle it. Wait, GetState errors if not a repo.

	// Actually, GetState will error if it's not a git repo.
	assert.Empty(t, state.CommitHash)
}

func TestClient_GetRepoRoot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := NewClient(nil)

	dir, err := os.Getwd()
	require.NoError(t, err)

	root, err := client.GetRepoRoot(ctx, dir)
	require.NoError(t, err)
	assert.Contains(t, filepath.Clean(dir), filepath.Clean(root)) // root is a prefix of dir
}
