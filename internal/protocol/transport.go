package protocol

// Transport abstracts how tasks and results are exchanged between
// the orchestrator and agents. The default implementation is FileTransport
// (file-based inbox/outbox), but this interface enables future alternatives
// (e.g., in-memory, gRPC, message queue).
type Transport interface {
	// WriteTask sends a task assignment to an agent.
	WriteTask(agentName string, msg *MessageEnvelope) error

	// ReadResult reads a result from an agent.
	ReadResult(path string) (*MessageEnvelope, error)

	// WriteResult writes a result to an agent's outbox.
	WriteResult(agentName string, msg *MessageEnvelope) error

	// ScanResults returns paths of all result files in the outbox.
	ScanResults() ([]string, error)
}
