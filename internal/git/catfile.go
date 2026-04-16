package git

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

const nullHash = "0000000000000000000000000000000000000000"

type BlobSizeResolver struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Scanner
	cancel context.CancelFunc
}

func NewBlobSizeResolver(ctx context.Context, repo string) (*BlobSizeResolver, error) {
	cmdCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cmdCtx, "git", "-C", repo, "cat-file", "--batch-check")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("cat-file stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("cat-file stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("cat-file start: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	return &BlobSizeResolver{
		cmd:    cmd,
		stdin:  stdin,
		reader: scanner,
		cancel: cancel,
	}, nil
}

func (r *BlobSizeResolver) Resolve(entries []RawEntry) (map[string]int64, error) {
	needed := make(map[string]struct{})
	for _, e := range entries {
		if e.OldHash != nullHash && e.OldHash != "" {
			needed[e.OldHash] = struct{}{}
		}
		if e.NewHash != nullHash && e.NewHash != "" {
			needed[e.NewHash] = struct{}{}
		}
	}

	if len(needed) == 0 {
		return map[string]int64{}, nil
	}

	for h := range needed {
		if _, err := io.WriteString(r.stdin, h+"\n"); err != nil {
			return nil, fmt.Errorf("cat-file write: %w", err)
		}
	}

	sizes := make(map[string]int64, len(needed))
	for i := 0; i < len(needed); i++ {
		if !r.reader.Scan() {
			if err := r.reader.Err(); err != nil {
				return nil, fmt.Errorf("cat-file read: %w", err)
			}
			return nil, fmt.Errorf("cat-file: unexpected EOF after %d/%d responses", i, len(needed))
		}

		parts := strings.Fields(r.reader.Text())
		if len(parts) < 3 || parts[1] != "blob" {
			continue
		}

		size, err := parseInt64(parts[2])
		if err != nil {
			continue
		}
		sizes[parts[0]] = size
	}

	return sizes, nil
}

func (r *BlobSizeResolver) Close() error {
	r.stdin.Close()
	r.cancel()
	_ = r.cmd.Wait()
	return nil
}
