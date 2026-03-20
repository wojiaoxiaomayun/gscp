package deploy

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gscp/internal/config"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Plan struct {
	EnvKey    string
	LocalPath string
	ToPath    string
	Commands  []string
}

type EventType string

const (
	EventStatus         EventType = "status"
	EventUploadProgress EventType = "upload_progress"
	EventUploadDone     EventType = "upload_done"
	EventCommandStart   EventType = "command_start"
	EventCommandOutput  EventType = "command_output"
	EventCommandDone    EventType = "command_done"
)

type Event struct {
	Type             EventType
	Message          string
	Command          string
	Output           string
	TotalBytes       int64
	WrittenBytes     int64
	TotalFiles       int
	CurrentFile      string
	CurrentFileIndex int
	CurrentFileSize  int64
	CurrentFileDone  int64
	SpeedBytes       float64
	ETA              time.Duration
}

type Runner struct {
	Notify func(Event)
}

type uploadItem struct {
	LocalPath  string
	RemotePath string
	Size       int64
}

type progressReporter struct {
	mu               sync.Mutex
	notify           func(Event)
	startedAt        time.Time
	lastRenderedAt   time.Time
	totalBytes       int64
	totalFiles       int
	written          int64
	currentFile      string
	currentFileSize  int64
	currentFileDone  int64
	currentFileIndex int
}

func (r Runner) Run(server config.Server, workingDir string, plan Plan) error {
	if strings.TrimSpace(plan.LocalPath) == "" {
		return fmt.Errorf("env %q missing local_path", plan.EnvKey)
	}
	if strings.TrimSpace(plan.ToPath) == "" {
		return fmt.Errorf("env %q missing to_path", plan.EnvKey)
	}

	localPath := plan.LocalPath
	if !filepath.IsAbs(localPath) {
		localPath = filepath.Join(workingDir, localPath)
	}
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("stat local_path: %w", err)
	}

	items, err := buildUploadPlan(localPath, plan.ToPath, info)
	if err != nil {
		return err
	}

	r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("connecting to %s", server.Host)})
	client, err := dialSSH(server)
	if err != nil {
		return err
	}
	defer client.Close()

	r.emit(Event{Type: EventStatus, Message: "creating sftp session"})
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("create sftp client: %w", err)
	}
	defer sftpClient.Close()

	if len(items) == 0 {
		r.emit(Event{Type: EventStatus, Message: "no files found to upload"})
	} else {
		r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("uploading to %s", plan.ToPath)})
		progress := newProgressReporter(r.Notify, totalSize(items), len(items))
		if err := uploadItems(sftpClient, items, progress); err != nil {
			return err
		}
		r.emit(Event{Type: EventUploadDone, Message: "upload complete"})
	}

	commands := normalizeCommands(plan.Commands)
	if len(commands) > 0 {
		r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("running %d remote commands", len(commands))})
		for index, command := range commands {
			r.emit(Event{Type: EventCommandStart, Command: command, Message: fmt.Sprintf("command %d/%d: %s", index+1, len(commands), command)})
		}

		output, err := runRemoteCommands(client, commands, server.Password)
		if output != "" {
			r.emit(Event{Type: EventCommandOutput, Command: strings.Join(commands, " && "), Output: output})
		}
		if err != nil {
			return fmt.Errorf("run remote commands: %w", err)
		}

		r.emit(Event{Type: EventCommandDone, Command: strings.Join(commands, " && "), Message: fmt.Sprintf("remote commands finished (%d)", len(commands))})
	}

	r.emit(Event{Type: EventStatus, Message: "deployment finished"})
	return nil
}

func (r Runner) emit(event Event) {
	if r.Notify != nil {
		r.Notify(event)
	}
}

func dialSSH(server config.Server) (*ssh.Client, error) {
	host := server.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "22")
	}

	client, err := ssh.Dial("tcp", host, &ssh.ClientConfig{
		User:            server.Username,
		Auth:            []ssh.AuthMethod{ssh.Password(server.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", host, err)
	}

	return client, nil
}

func buildUploadPlan(localPath, remoteBase string, info fs.FileInfo) ([]uploadItem, error) {
	remoteBase = path.Clean(filepath.ToSlash(remoteBase))

	if !info.IsDir() {
		return []uploadItem{{
			LocalPath:  localPath,
			RemotePath: path.Join(remoteBase, filepath.Base(localPath)),
			Size:       info.Size(),
		}}, nil
	}

	items := make([]uploadItem, 0)
	err := filepath.WalkDir(localPath, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			return fmt.Errorf("read file info: %w", err)
		}
		relative, err := filepath.Rel(localPath, current)
		if err != nil {
			return fmt.Errorf("build relative path: %w", err)
		}

		items = append(items, uploadItem{
			LocalPath:  current,
			RemotePath: path.Join(remoteBase, filepath.ToSlash(relative)),
			Size:       fileInfo.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return items, nil
}

func uploadItems(client *sftp.Client, items []uploadItem, progress *progressReporter) error {
	for index, item := range items {
		if err := client.MkdirAll(path.Dir(item.RemotePath)); err != nil {
			return fmt.Errorf("create remote dir %s: %w", path.Dir(item.RemotePath), err)
		}
		if err := uploadFile(client, item, index+1, progress); err != nil {
			return err
		}
	}
	progress.finish()
	return nil
}

func uploadFile(client *sftp.Client, item uploadItem, index int, progress *progressReporter) error {
	localFile, err := os.Open(item.LocalPath)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", item.LocalPath, err)
	}
	defer localFile.Close()

	remoteFile, err := client.Create(item.RemotePath)
	if err != nil {
		return fmt.Errorf("create remote file %s: %w", item.RemotePath, err)
	}
	defer remoteFile.Close()

	progress.setCurrent(item.LocalPath, item.Size, index)
	writer := io.MultiWriter(remoteFile, progress)
	if _, err := io.Copy(writer, localFile); err != nil {
		return fmt.Errorf("upload file %s: %w", item.LocalPath, err)
	}

	return nil
}

func runRemoteCommands(client *ssh.Client, commands []string, sudoPassword string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		return "", fmt.Errorf("request pty: %w", err)
	}

	script := buildCommandScript(commands, sudoPassword)
	remoteCommand := "sh -lc " + shellSingleQuote(script)
	output, err := session.CombinedOutput(remoteCommand)
	return string(output), err
}

func buildCommandScript(commands []string, sudoPassword string) string {
	lines := []string{"set -e"}
	for index, command := range commands {
		marker := fmt.Sprintf("[%d/%d] %s", index+1, len(commands), command)
		lines = append(lines, "printf '%s\\n' "+shellSingleQuote(">>> "+marker))
		lines = append(lines, prepareCommand(command, sudoPassword))
		lines = append(lines, "printf '%s\\n' "+shellSingleQuote("<<< done "+marker))
	}
	return strings.Join(lines, "\n")
}

func prepareCommand(command string, sudoPassword string) string {
	trimmed := strings.TrimSpace(command)
	if sudoPassword == "" || !strings.HasPrefix(trimmed, "sudo ") {
		return command
	}

	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "sudo "))
	if strings.HasPrefix(rest, "-S ") || strings.HasPrefix(rest, "--stdin ") {
		return command
	}

	return "printf '%s\\n' " + shellSingleQuote(sudoPassword) + " | sudo -S -p '' " + rest
}

func normalizeCommands(commands []string) []string {
	result := make([]string, 0, len(commands))
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		result = append(result, command)
	}
	return result
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func totalSize(items []uploadItem) int64 {
	var total int64
	for _, item := range items {
		total += item.Size
	}
	return total
}

func newProgressReporter(notify func(Event), total int64, totalFiles int) *progressReporter {
	now := time.Now()
	return &progressReporter{
		notify:         notify,
		startedAt:      now,
		lastRenderedAt: now,
		totalBytes:     total,
		totalFiles:     totalFiles,
	}
}

func (p *progressReporter) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	written := int64(len(data))
	p.written += written
	p.currentFileDone += written
	p.emitLocked(false)
	return len(data), nil
}

func (p *progressReporter) setCurrent(file string, size int64, index int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.currentFile = file
	p.currentFileSize = size
	p.currentFileDone = 0
	p.currentFileIndex = index
	p.emitLocked(true)
}

func (p *progressReporter) finish() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.written = p.totalBytes
	p.currentFileDone = p.currentFileSize
	p.emitLocked(true)
}

func (p *progressReporter) emitLocked(force bool) {
	if p.notify == nil {
		return
	}

	now := time.Now()
	if !force && now.Sub(p.lastRenderedAt) < 120*time.Millisecond {
		return
	}
	p.lastRenderedAt = now

	elapsed := now.Sub(p.startedAt)
	speed := 0.0
	if elapsed > 0 {
		speed = float64(p.written) / elapsed.Seconds()
	}

	eta := time.Duration(0)
	if speed > 0 && p.totalBytes >= p.written {
		eta = time.Duration(float64(time.Second) * (float64(p.totalBytes-p.written) / speed))
	}

	p.notify(Event{
		Type:             EventUploadProgress,
		TotalBytes:       p.totalBytes,
		WrittenBytes:     p.written,
		TotalFiles:       p.totalFiles,
		CurrentFile:      p.currentFile,
		CurrentFileIndex: p.currentFileIndex,
		CurrentFileSize:  p.currentFileSize,
		CurrentFileDone:  p.currentFileDone,
		SpeedBytes:       speed,
		ETA:              eta,
	})
}
