package reconcile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath_NormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{`.\hello.txt`, "hello.txt"},
		{`/hello/world`, "hello/world"},
		{"a/b/../c", "a/c"},
		{`C:\test\path`, "C:/test/path"},
	}

	for _, tc := range tests {
		assert.Equal(t, tc.expected, normalizePath(tc.input))
	}
}

func TestPath_IsInternalMetadataPath(t *testing.T) {
	assert.True(t, isInternalMetadataPath(".wake/state.db"))
	assert.True(t, isInternalMetadataPath(".git/config"))
	assert.False(t, isInternalMetadataPath("src/main.go"))
}

func TestPath_MatchSinglePattern(t *testing.T) {
	assert.True(t, matchSinglePattern("src", "src/main.go"))
	assert.True(t, matchSinglePattern("src/", "src/main.go"))
	assert.True(t, matchSinglePattern("*.go", "src/main.go"))
	assert.False(t, matchSinglePattern("*.js", "src/main.go"))
	assert.True(t, matchSinglePattern("src/main.go", "src/main.go"))
}

func TestPath_IsSafeRelativePath(t *testing.T) {
	assert.True(t, isSafeRelativePath("src/main.go"))
	assert.False(t, isSafeRelativePath("../main.go"))
	assert.False(t, isSafeRelativePath("/etc/passwd"))
	assert.False(t, isSafeRelativePath(`C:\windows`))
	assert.False(t, isSafeRelativePath(`\\server\share`))
}

func TestPath_ExtractTokens(t *testing.T) {
	tokens := extractTokens("hello, world! (this is a test)")
	assert.Contains(t, tokens, "hello")
	assert.Contains(t, tokens, "world!")
	assert.Contains(t, tokens, "test")
}

func TestPath_LooksLikeFilePath(t *testing.T) {
	assert.True(t, looksLikeFilePath("src/main.go"))
	assert.True(t, looksLikeFilePath("Makefile"))
	assert.False(t, looksLikeFilePath("http://example.com"))
	assert.False(t, looksLikeFilePath("v1.0.0"))
	assert.False(t, looksLikeFilePath("e.g."))
	assert.False(t, looksLikeFilePath("1."))
}
