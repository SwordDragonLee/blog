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
// proxy 非空时克隆走 HTTP 代理（github 直连不可达的场景）。
func Clone(ctx context.Context, gitURL, workDir string, timeout time.Duration, proxy string) (*RepoInfo, error) {
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(workDir, "repo-*")
	if err != nil {
		return nil, err
	}

	cloneCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// -c http.version=HTTP/1.1：大仓库浅克隆在丢包网络下 HTTP/2 易中途断流
	//（表现为 error: RPC failed / fatal: early EOF），强制 HTTP/1.1 更稳
	cmd := exec.CommandContext(cloneCtx, "git", "-c", "http.version=HTTP/1.1",
		"clone", "--depth", "1", "--single-branch", gitURL, dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if proxy != "" {
		// git 认 http_proxy/https_proxy 环境变量，仅注入到本次克隆的子进程
		cmd.Env = append(cmd.Env, "HTTP_PROXY="+proxy, "HTTPS_PROXY="+proxy)
	}
	// Windows 下超时杀掉 git.exe 后，其子进程（git-remote-https）仍握着 stderr 管道，
	// Wait 会永久阻塞导致任务卡在「克隆仓库」；WaitDelay 强制 Wait 在进程退出后限时返回
	cmd.WaitDelay = 10 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		return nil, classifyCloneErr(cloneCtx, timeout, err, stderr.String())
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

func classifyCloneErr(ctx context.Context, timeout time.Duration, err error, output string) error {
	// 仅在克隆自身的超时触发时才报「超时」；任务被主动取消（Canceled）不在此误报
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("克隆仓库超时（限时 %s，仓库过大或网络过慢，可调大 task.clone_timeout_seconds）", timeout)
	}
	for _, clause := range errClauses {
		for _, m := range clause.match {
			if strings.Contains(output, m) {
				return errors.New(clause.msg)
			}
		}
	}
	if strings.Contains(output, "timed out") || strings.Contains(output, "Connection refused") {
		return fmt.Errorf("网络异常，无法访问仓库: %s", errLine(output))
	}
	return fmt.Errorf("克隆失败: %s", errLine(output))
}

// errLine 从 git 的 stderr 中挑出真正有信息量的错误行。
// 首行往往是「Cloning into 'xxx'...」这类进度提示，真正的报错（error:/fatal: 等）
// 在后面，因此按关键词匹配优先；都没命中时退回最后一个非空行。
func errLine(output string) string {
	lines := strings.Split(output, "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	for _, line := range nonEmpty {
		low := strings.ToLower(line)
		for _, m := range []string{"error:", "fatal:", "rpc failed", "early eof",
			"transfer closed", "timed out", "connection", "ssl", "unable to access"} {
			if strings.Contains(low, m) {
				return truncateLine(line)
			}
		}
	}
	for i := len(nonEmpty) - 1; i >= 0; i-- {
		if !strings.HasPrefix(nonEmpty[i], "Cloning into") {
			return truncateLine(nonEmpty[i])
		}
	}
	return "git 未输出错误信息"
}

func truncateLine(s string) string {
	if len(s) > 200 {
		return s[:200]
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
