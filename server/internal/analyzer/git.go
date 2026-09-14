// Package analyzer 负责克隆公开 Git 仓库、识别技术栈并采样核心代码文件。
package analyzer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RepoInfo 是克隆结果元信息。
type RepoInfo struct {
	URL           string
	Dir           string
	RepoName      string
	DefaultBranch string
	HeadCommit    string
	HeadSubject   string
}

var errClauses = []struct {
	match []string
	msg   string
}{
	{[]string{"not found", "Repository not found", "does not appear to be a git repository", "Could not read from remote repository"}, "仓库不存在或不可访问，请确认地址是否为公开仓库"},
	{[]string{"Authentication failed", "could not read Username", "terminal prompts disabled", "interactive authentication"}, "仓库需要认证，暂不支持私有仓库"},
}

// Clone 对公开仓库执行 shallow clone，返回元信息。调用方负责清理返回的 Dir。
func Clone(ctx context.Context, gitURL, workDir string, timeout time.Duration) (*RepoInfo, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(workDir, "repo-*")
	if err != nil {
		return nil, err
	}

	cloneCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--single-branch", gitURL, dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return nil, classifyCloneErr(cloneCtx, err, stderr.String())
	}

	info := &RepoInfo{URL: gitURL, Dir: dir, RepoName: repoNameFromURL(gitURL)}
	if info.DefaultBranch, err = gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD"); err != nil {
		return nil, err
	}
	subject, err := gitOut(dir, "log", "-1", "--format=%H%n%s")
	if err == nil {
		parts := strings.SplitN(subject, "\n", 2)
		info.HeadCommit = parts[0]
		if len(parts) == 2 {
			info.HeadSubject = parts[1]
		}
	}
	return info, nil
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(out.String()), nil
}

func classifyCloneErr(ctx context.Context, err error, output string) error {
	if ctx.Err() != nil {
		return errors.New("克隆仓库超时")
	}
	for _, clause := range errClauses {
		for _, m := range clause.match {
			if strings.Contains(output, m) {
				return errors.New(clause.msg)
			}
		}
	}
	if strings.Contains(output, "timed out") || strings.Contains(output, "Connection refused") {
		return fmt.Errorf("网络异常，无法访问仓库: %s", firstLine(output))
	}
	return fmt.Errorf("克隆失败: %s", firstLine(output))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func repoNameFromURL(u string) string {
	u = strings.TrimSuffix(strings.TrimSpace(u), ".git")
	if i := strings.LastIndexAny(u, "/:"); i >= 0 {
		u = u[i+1:]
	}
	return u
}

// Cleanup 删除克隆目录。
func Cleanup(dir string) {
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}
