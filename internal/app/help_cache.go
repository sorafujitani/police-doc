package app

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sorafujitani/police-doc/internal/extract"
	"github.com/sorafujitani/police-doc/internal/spec"
)

// Bump when the cache layout or collector's interpretation changes.
const helpCacheVersion = 2

type cachedHelp struct {
	SchemaVersion int            `json:"schema_version"`
	Binary        string         `json:"binary"`
	Fingerprint   string         `json:"fingerprint"`
	Snapshot      *spec.Snapshot `json:"snapshot"`
	Warnings      []string       `json:"warnings,omitempty"`
}

func helpCachePath(directory string, example extract.Example) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	key := cwd + "\x00" + runtime.GOOS + "/" + runtime.GOARCH + "\x00" + example.Words[0].Value
	if example.LocalPackage {
		origin, err := filepath.Abs(filepath.Dir(example.Location.File))
		if err != nil {
			return "", err
		}
		key += "\x00npx\x00" + origin
	}
	return filepath.Join(directory, fmt.Sprintf("%x.json", sha256.Sum256([]byte(key)))), nil
}

func executableFingerprint(binary string) (string, error) {
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	identity := fmt.Sprintf("%s\x00%d\x00%d\x00%d", resolved, info.Size(), info.ModTime().UnixNano(), info.Mode())
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity))), nil
}

func loadHelpCache(path, tool string) (cachedHelp, error) {
	// Check before Open: opening a FIFO can block before file.Stat is reached.
	info, err := os.Lstat(path)
	if err != nil {
		return cachedHelp{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return cachedHelp{}, fmt.Errorf("help cache must be a regular file of at most 8 MiB")
	}
	file, err := os.Open(path)
	if err != nil {
		return cachedHelp{}, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return cachedHelp{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return cachedHelp{}, fmt.Errorf("help cache must be a regular file of at most 8 MiB")
	}
	var cached cachedHelp
	decoder := json.NewDecoder(io.LimitReader(file, (8<<20)+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cached); err != nil {
		return cachedHelp{}, err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return cachedHelp{}, fmt.Errorf("help cache must contain a single JSON document")
	}
	if cached.SchemaVersion != helpCacheVersion || cached.Binary == "" || cached.Fingerprint == "" || cached.Snapshot == nil || cached.Snapshot.Tool != tool || cached.Snapshot.OS != runtime.GOOS {
		return cachedHelp{}, fmt.Errorf("invalid help cache for %s", tool)
	}
	if err := cached.Snapshot.Validate(); err != nil {
		return cachedHelp{}, err
	}
	return cached, nil
}

func writeHelpCache(path string, cached cachedHelp) error {
	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return fmt.Errorf("help cache exceeds 8 MiB")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".help-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	// Readers see either the previous complete entry or the new complete entry.
	return os.Rename(file.Name(), path)
}
