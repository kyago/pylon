package protocol

// FileTransport implements Transport using the file-based inbox/outbox protocol.
type FileTransport struct {
	InboxDir  string
	OutboxDir string
}

// NewFileTransport creates a new FileTransport.
func NewFileTransport(inboxDir, outboxDir string) *FileTransport {
	return &FileTransport{InboxDir: inboxDir, OutboxDir: outboxDir}
}

// WriteTask writes a task assignment to the agent's inbox directory.
func (ft *FileTransport) WriteTask(agentName string, msg *MessageEnvelope) error {
	return WriteTask(ft.InboxDir, agentName, msg)
}

// ReadResult reads a result from the given file path.
func (ft *FileTransport) ReadResult(path string) (*MessageEnvelope, error) {
	return ReadResult(path)
}

// WriteResult writes a result to the agent's outbox directory.
func (ft *FileTransport) WriteResult(agentName string, msg *MessageEnvelope) error {
	return WriteResult(ft.OutboxDir, agentName, msg)
}

// ScanResults returns paths of all result files in the outbox directory.
func (ft *FileTransport) ScanResults() ([]string, error) {
	return ScanOutbox(ft.OutboxDir)
}

// Verify interface compliance at compile time.
var _ Transport = (*FileTransport)(nil)
