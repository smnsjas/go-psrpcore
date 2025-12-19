package messages

import "github.com/google/uuid"

// NewSignal creates a SIGNAL message.
// This is used to stop a running pipeline (Ctrl+C).
// MS-PSRP Section 2.2.2.10
func NewSignal(runspaceID, pipelineID uuid.UUID) *Message {
	return &Message{
		Destination: DestinationServer,
		Type:        MessageTypeSignal,
		RunspaceID:  runspaceID,
		PipelineID:  pipelineID,
		Data:        nil,
	}
}
