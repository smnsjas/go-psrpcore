package runspace

// SetSecurityEventCallback sets the callback for security events.
// This allows the consumer (e.g., go-psrp client) to receive and log
// security-relevant events from the protocol layer.
func (p *Pool) SetSecurityEventCallback(callback SecurityEventCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.securityCallback = callback
}

// emitSecurityEventLocked invokes the security callback if set.
// Caller MUST hold p.mu.
func (p *Pool) emitSecurityEventLocked(event string, details map[string]any) {
	cb := p.securityCallback
	if cb != nil {
		cb(event, details)
	}
}
