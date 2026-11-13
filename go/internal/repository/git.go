// Package repository owns workspace source repositories. The only executed program
// is git with fixed plumbing subcommands; repository content and hooks never run.
package repository

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const DefaultQuota int64 = 256 << 20

// Git's config is supplied by us, not by incoming commits or an imported .git.
func gitEnv() []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=/nonexistent", "LANG=C.UTF-8", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_COUNT=6", "GIT_CONFIG_KEY_0=core.hooksPath", "GIT_CONFIG_VALUE_0=/dev/null", "GIT_CONFIG_KEY_1=core.attributesFile", "GIT_CONFIG_VALUE_1=/dev/null", "GIT_CONFIG_KEY_2=protocol.allow", "GIT_CONFIG_VALUE_2=never", "GIT_CONFIG_KEY_3=receive.fsckObjects", "GIT_CONFIG_VALUE_3=true", "GIT_CONFIG_KEY_4=gc.auto", "GIT_CONFIG_VALUE_4=0", "GIT_CONFIG_KEY_5=maintenance.auto", "GIT_CONFIG_VALUE_5=false"}
}
func Git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--git-dir=" + dir}, args...)...)
	cmd.Env = gitEnv()
	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return output.data, nil
}

type limitedBuffer struct{ data []byte }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > 4<<20 {
		return 0, fmt.Errorf("git output exceeds 4 MiB")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func Init(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	_, err := Git(ctx, dir, "init", "--bare", "--initial-branch=main", dir)
	return err
}
func Bytes(dir string) (int64, error) {
	var size int64
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("repository contains a symbolic link")
		}
		if !d.IsDir() {
			st, err := d.Info()
			if err != nil {
				return err
			}
			size += st.Size()
		}
		return nil
	})
	return size, err
}
func copyRepo(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are forbidden")
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
func safeRef(raw string) bool {
	return len(raw) > 0 && len(raw) <= 200 && !strings.HasPrefix(raw, "-") && !strings.ContainsAny(raw, "\x00\r\n :\\?*[~^") && !strings.Contains(raw, "..") && !strings.Contains(raw, "@{")
}
func safePath(raw string) bool {
	return len(raw) <= 2048 && !strings.HasPrefix(raw, "/") && !strings.ContainsAny(raw, "\x00\r\n\\") && (raw == "" || filepath.ToSlash(filepath.Clean(raw)) == raw) && raw != ".." && !strings.HasPrefix(raw, "../")
}

// serveGitHTTP runs the fixed CGI program with a real process deadline. No
// inherited CGI/auth headers or user-selected executable enter its environment.
func serveGitHTTP(w http.ResponseWriter, r *http.Request, root, name, path string, env []string) error {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	env = append(env, "REQUEST_METHOD="+r.Method, "QUERY_STRING="+r.URL.RawQuery, "CONTENT_TYPE="+r.Header.Get("Content-Type"), "SERVER_PROTOCOL=HTTP/1.1", "REMOTE_ADDR=127.0.0.1")
	if r.ContentLength >= 0 {
		env = append(env, "CONTENT_LENGTH="+strconv.FormatInt(r.ContentLength, 10))
	}
	if encoding := r.Header.Get("Content-Encoding"); encoding == "gzip" || encoding == "identity" {
		env = append(env, "HTTP_CONTENT_ENCODING="+encoding)
	}
	if protocol := r.Header.Get("Git-Protocol"); len(protocol) <= 128 && !strings.ContainsAny(protocol, "\x00\r\n") {
		env = append(env, "HTTP_GIT_PROTOCOL="+protocol)
	}
	cmd := exec.CommandContext(ctx, "git", "http-backend")
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdin = r.Body
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	reader := bufio.NewReader(stdout)
	headers := http.Header{}
	status := 200
	total := 0
	for {
		line, e := reader.ReadString('\n')
		total += len(line)
		if e != nil || total > 32768 {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return fmt.Errorf("git HTTP header failed")
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.EqualFold(key, "Status") {
			fields := strings.Fields(value)
			if len(fields) > 0 {
				if code, e := strconv.Atoi(fields[0]); e == nil && code >= 100 && code <= 599 {
					status = code
				}
			}
		} else {
			headers.Add(key, value)
		}
	}
	for key, values := range headers {
		w.Header()[key] = values
	}
	w.WriteHeader(status)
	_, copyErr := io.Copy(w, reader)
	if copyErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	return waitErr
}
