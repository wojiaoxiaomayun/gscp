package deploy

import (
	"errors"
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

// UploadPair maps a single local path to a single remote destination.
type UploadPair struct {
	From string
	To   string
}

type Plan struct {
	EnvKey      string
	LocalPaths  []string
	ToPath      string
	UploadPairs []UploadPair // takes precedence over LocalPaths+ToPath when non-empty
	Ignore      []string
	Commands    []string
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

type commandStreamer struct {
	notify  func(Event)
	command string
	buffer  strings.Builder
	mu      sync.Mutex
}

func (r Runner) Run(server config.Server, workingDir string, plan Plan) error {
	// Build the full list of upload items depending on the mode.
	var allItems []uploadItem
	var pairItemCounts []int // only populated in pairs mode

	if len(plan.UploadPairs) > 0 {
		// Pairs mode: each pair has its own local→remote mapping.
		// Track how many items belong to each pair for per-pair progress reporting.
		pairItemCounts = make([]int, 0, len(plan.UploadPairs))
		for _, pair := range plan.UploadPairs {
			if strings.TrimSpace(pair.From) == "" || strings.TrimSpace(pair.To) == "" {
				return fmt.Errorf("env %q: upload_pairs entry has empty from or to", plan.EnvKey)
			}
			localPath := pair.From
			if !filepath.IsAbs(localPath) {
				localPath = filepath.Join(workingDir, localPath)
			}
			info, err := os.Stat(localPath)
			if err != nil {
				return fmt.Errorf("stat upload_pairs from %q: %w", pair.From, err)
			}
			items, err := buildUploadPlan(localPath, pair.To, info, plan.Ignore)
			if err != nil {
				return err
			}
			pairItemCounts = append(pairItemCounts, len(items))
			allItems = append(allItems, items...)
		}
	} else {
		// Single / multi-path mode (legacy): all paths go to the same to_path.
		if len(plan.LocalPaths) == 0 {
			return fmt.Errorf("env %q missing local_path", plan.EnvKey)
		}
		if strings.TrimSpace(plan.ToPath) == "" {
			return fmt.Errorf("env %q missing to_path", plan.EnvKey)
		}

		for _, localPath := range plan.LocalPaths {
			if strings.TrimSpace(localPath) == "" {
				continue
			}
			if !filepath.IsAbs(localPath) {
				localPath = filepath.Join(workingDir, localPath)
			}
			info, err := os.Stat(localPath)
			if err != nil {
				return fmt.Errorf("stat local_path %q: %w", localPath, err)
			}
			items, err := buildUploadPlan(localPath, plan.ToPath, info, plan.Ignore)
			if err != nil {
				return err
			}
			allItems = append(allItems, items...)
		}
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

	if len(allItems) == 0 {
		r.emit(Event{Type: EventStatus, Message: "no files found to upload"})
	} else if len(pairItemCounts) > 0 {
		// Pairs mode: report per-pair progress so the user can see each one finish.
		r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("uploading %d pair(s)", len(plan.UploadPairs))})

		globalProgress := newProgressReporter(r.Notify, totalSize(allItems), len(allItems))
		cursor := 0
		fileOffset := 0
		for i, pair := range plan.UploadPairs {
			count := pairItemCounts[i]
			pairItems := allItems[cursor : cursor+count]
			cursor += count
			isLast := i == len(plan.UploadPairs)-1

			r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("[%d/%d] uploading %s → %s", i+1, len(plan.UploadPairs), pair.From, pair.To)})
			if err := uploadItemsWithOffset(sftpClient, pairItems, globalProgress, fileOffset, isLast); err != nil {
				return err
			}
			fileOffset += count
			r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("[%d/%d] done %s → %s", i+1, len(plan.UploadPairs), pair.From, pair.To)})
		}
		r.emit(Event{Type: EventUploadDone, Message: "upload complete"})
	} else {
		r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("uploading to %s", plan.ToPath)})
		progress := newProgressReporter(r.Notify, totalSize(allItems), len(allItems))
		if err := uploadItems(sftpClient, allItems, progress); err != nil {
			return err
		}
		r.emit(Event{Type: EventUploadDone, Message: "upload complete"})
	}

	commands := normalizeCommands(plan.Commands)
	if len(commands) > 0 {
		r.emit(Event{Type: EventStatus, Message: fmt.Sprintf("running %d remote commands", len(commands))})
		if err := runRemoteCommands(client, commands, server.Password, r.Notify); err != nil {
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

	authMethods, keyErr := buildAuthMethods(server)
	if len(authMethods) == 0 {
		if keyErr != nil {
			return nil, keyErr
		}
		return nil, errors.New("no authentication method configured (provide password or key_path)")
	}

	client, err := ssh.Dial("tcp", host, &ssh.ClientConfig{
		User:            server.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		if keyErr != nil {
			return nil, fmt.Errorf("ssh dial %s: %w (private key was unavailable: %v)", host, err, keyErr)
		}
		return nil, fmt.Errorf("ssh dial %s: %w", host, err)
	}

	return client, nil
}

func buildAuthMethods(server config.Server) ([]ssh.AuthMethod, error) {
	authMethods := make([]ssh.AuthMethod, 0, 2)
	var keyErr error

	if server.KeyPath != "" {
		signer, err := loadPrivateKey(server.KeyPath, server.KeyPass)
		if err != nil {
			keyErr = err
		} else {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}
	if server.Password != "" {
		authMethods = append(authMethods, ssh.Password(server.Password))
	}

	return authMethods, keyErr
}

func loadPrivateKey(keyPath, passphrase string) (ssh.Signer, error) {
	keyPath = expandHome(keyPath)

	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key %q: %w", keyPath, err)
	}

	signer, err := ssh.ParsePrivateKey(data)
	if err == nil {
		return signer, nil
	}
	var missingPassphrase *ssh.PassphraseMissingError
	if !errors.As(err, &missingPassphrase) {
		return nil, fmt.Errorf("parse private key %q: %w", keyPath, err)
	}
	if passphrase == "" {
		return nil, fmt.Errorf("parse private key %q: key is encrypted but key_pass is empty", keyPath)
	}

	signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("parse encrypted private key %q: %w", keyPath, err)
	}
	return signer, nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// buildUploadPlan walks a local file or directory and lists every file to be
// uploaded. ignore holds glob patterns (see ignoreMatcher) for entries that
// must be excluded, matched against paths relative to the local root.
func buildUploadPlan(localPath, remoteBase string, info fs.FileInfo, ignore []string) ([]uploadItem, error) {
	remoteBase = path.Clean(filepath.ToSlash(remoteBase))
	matcher := newIgnoreMatcher(ignore)

	if !info.IsDir() {
		if matcher.match(filepath.Base(localPath)) {
			return nil, nil
		}
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
		relative, err := filepath.Rel(localPath, current)
		if err != nil {
			return fmt.Errorf("build relative path: %w", err)
		}
		relSlash := filepath.ToSlash(relative)

		if matcher.match(relSlash) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}

		fileInfo, err := d.Info()
		if err != nil {
			return fmt.Errorf("read file info: %w", err)
		}

		items = append(items, uploadItem{
			LocalPath:  current,
			RemotePath: path.Join(remoteBase, relSlash),
			Size:       fileInfo.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	return items, nil
}

// ignoreMatcher reports whether a path relative to a local upload root should
// be excluded from the upload, using gitignore-style glob patterns.
//
// Supported syntax (relative to the source root, "/" separators):
//
//   - A pattern without "/" matches a file or directory of that name at any
//     depth (e.g. "static" or "*.map"). Matching a directory excludes its
//     whole subtree.
//   - A pattern containing "/" is anchored at the source root and matches the
//     full relative path (e.g. "dist/static" or "assets/**/cache"). A
//     trailing "/" is ignored (it merely marks the pattern as directory-only).
//   - "*" matches any run of non-separator characters within one path
//     segment; "?" matches a single character; "[...]" character classes work
//     too (delegated to path.Match).
//   - "**" as a whole segment matches zero or more path segments.
type ignoreMatcher struct {
	names    []string   // patterns without "/"
	patterns [][]string // patterns with "/", split into segments
}

func newIgnoreMatcher(patterns []string) *ignoreMatcher {
	m := &ignoreMatcher{}
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.ToSlash(p)
		p = strings.TrimPrefix(p, "./")
		if strings.Contains(p, "/") {
			p = strings.TrimSuffix(p, "/")
			if p != "" {
				m.patterns = append(m.patterns, strings.Split(p, "/"))
			}
		} else {
			m.names = append(m.names, p)
		}
	}
	return m
}

// match reports whether rel (slash-separated relative path) is excluded.
func (m *ignoreMatcher) match(rel string) bool {
	if len(m.names) > 0 {
		base := path.Base(rel)
		for _, name := range m.names {
			if ok, err := path.Match(name, base); err == nil && ok {
				return true
			}
		}
	}
	if len(m.patterns) == 0 {
		return false
	}
	segs := strings.Split(rel, "/")
	for _, pat := range m.patterns {
		if matchSegments(pat, segs) {
			return true
		}
	}
	return false
}

// matchSegments matches a pattern split on "/" against path segments,
// supporting "**" as a segment matching zero or more segments.
func matchSegments(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if matchSegments(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	ok, err := path.Match(pat[0], segs[0])
	if err != nil || !ok {
		return false
	}
	return matchSegments(pat[1:], segs[1:])
}

func uploadItems(client *sftp.Client, items []uploadItem, progress *progressReporter) error {
	return uploadItemsWithOffset(client, items, progress, 0, true)
}

// uploadItemsWithOffset uploads items using a pre-existing progress reporter.
// indexOffset shifts the displayed file index (for multi-pair mode).
// finishOnDone controls whether progress.finish() is called after the last file.
func uploadItemsWithOffset(client *sftp.Client, items []uploadItem, progress *progressReporter, indexOffset int, finishOnDone bool) error {
	for i, item := range items {
		if err := client.MkdirAll(path.Dir(item.RemotePath)); err != nil {
			return fmt.Errorf("create remote dir %s: %w", path.Dir(item.RemotePath), err)
		}
		if err := uploadFile(client, item, indexOffset+i+1, progress); err != nil {
			return err
		}
	}
	if finishOnDone {
		progress.finish()
	}
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

func runRemoteCommands(client *ssh.Client, commands []string, sudoPassword string, notify func(Event)) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 80, 40, modes); err != nil {
		return fmt.Errorf("request pty: %w", err)
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	script := buildCommandScript(commands, sudoPassword)
	remoteCommand := "sh -lc " + shellSingleQuote(script)
	streamer := &commandStreamer{notify: notify, command: strings.Join(commands, " && ")}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamer.read(stdout)
	}()
	go func() {
		defer wg.Done()
		streamer.read(stderr)
	}()

	if err := session.Start(remoteCommand); err != nil {
		return fmt.Errorf("start remote command: %w", err)
	}

	waitErr := session.Wait()
	wg.Wait()
	streamer.flush()
	if waitErr != nil {
		return waitErr
	}
	return nil
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

func (s *commandStreamer) read(reader io.Reader) {
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			s.write(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			s.emitLine("ERROR: " + err.Error())
			return
		}
	}
}

func (s *commandStreamer) write(data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer.Write(data)
	for {
		content := s.buffer.String()
		index := strings.IndexByte(content, '\n')
		if index < 0 {
			return
		}
		line := strings.TrimRight(content[:index], "\r")
		remaining := content[index+1:]
		s.buffer.Reset()
		s.buffer.WriteString(remaining)
		s.emitLineLocked(line)
	}
}

func (s *commandStreamer) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()

	remaining := strings.TrimSpace(s.buffer.String())
	if remaining == "" {
		return
	}
	s.buffer.Reset()
	s.emitLineLocked(remaining)
}

func (s *commandStreamer) emitLine(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emitLineLocked(line)
}

func (s *commandStreamer) emitLineLocked(line string) {
	line = strings.TrimRight(line, "\r")
	if line == "" || s.notify == nil {
		return
	}

	s.notify(Event{Type: EventCommandOutput, Command: s.command, Output: line})
	if strings.HasPrefix(line, ">>> ") {
		s.notify(Event{Type: EventCommandStart, Command: strings.TrimPrefix(extractMarkerCommand(line), ""), Message: line})
		return
	}
	if strings.HasPrefix(line, "<<< done ") {
		s.notify(Event{Type: EventCommandDone, Command: extractMarkerCommand(strings.TrimPrefix(line, "<<< done ")), Message: line})
	}
}

func extractMarkerCommand(line string) string {
	clean := strings.TrimSpace(strings.TrimPrefix(line, ">>>"))
	clean = strings.TrimSpace(strings.TrimPrefix(clean, "<<< done"))
	if !strings.HasPrefix(clean, "[") {
		return clean
	}
	if idx := strings.Index(clean, "] "); idx >= 0 && idx+2 <= len(clean) {
		return clean[idx+2:]
	}
	return clean
}
