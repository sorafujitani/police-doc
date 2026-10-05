package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestHelpCacheRejectsFIFOWithoutBlocking(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("requires mkfifo")
	}
	path := filepath.Join(t.TempDir(), "cache.json")
	if output, err := exec.CommandContext(t.Context(), mkfifo, path).CombinedOutput(); err != nil {
		t.Fatalf("mkfifo: %v: %s", err, output)
	}
	done := make(chan error, 1)
	go func() {
		_, err := loadHelpCache(path, "acme")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("accepted a FIFO as a cache entry")
		}
	case <-time.After(time.Second):
		// Release a blocked reader before failing, without leaving a goroutine.
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Error(err)
		}
		<-done
		t.Fatal("opening a non-regular cache blocked before validating its type")
	}
}
