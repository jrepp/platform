package store

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// cleanName reduces a caller-supplied name to a canonical store key, or returns
// empty when the name cannot be one.
//
// Rejecting rather than repairing is the point. A name containing traversal is
// not a name with a typo in it; silently rewriting `../../etc/passwd` into
// something inside the root would turn an attack into a confusing success.
func cleanName(raw string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}
	// Reject traversal in the caller's own name before path.Clean can absorb
	// it. Clean("/../escape.txt") is "/escape.txt", so checking only the
	// cleaned result would accept the request and quietly store it under a
	// different name than was asked for -- inside the root, so not an escape,
	// but a rewrite that hides the caller's bug and makes the audit record
	// disagree with the request.
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == ".." {
			return ""
		}
	}
	clean := path.Clean("/" + trimmed)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "." || clean == "" {
		return ""
	}
	return clean
}

// resolve maps a clean name to an absolute path that is provably inside root.
//
// The containment check compares cleaned absolute paths and requires a
// separator boundary, so a sibling directory whose name merely starts with the
// root's name cannot pass: /srv/assets-old is not inside /srv/assets.
func resolve(root, name string) (string, error) {
	if root == "" {
		return "", ErrInvalidName
	}
	clean := cleanName(name)
	if clean == "" {
		return "", ErrInvalidName
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", ErrInvalidName
	}
	rootAbs = filepath.Clean(rootAbs)
	full := filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(clean)))
	if full == rootAbs || !strings.HasPrefix(full, rootAbs+string(os.PathSeparator)) {
		return "", ErrInvalidName
	}
	return full, nil
}
