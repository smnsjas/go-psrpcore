package serialization

// HostInfo represents the host information.
// It matches the structure expected by PowerShell deserializer (private fields).
type HostInfo struct {
	HostDefaultData *HostDefaultData `xml:"_hostDefaultData"`
	IsHostNull      bool             `xml:"_isHostNull"`
	IsHostUINull    bool             `xml:"_isHostUINull"`
	IsHostRawUINull bool             `xml:"_isHostRawUINull"`
	UseRunspaceHost bool             `xml:"_useRunspaceHost"`
}

// HostDefaultData represents the default host data.
type HostDefaultData struct {
	Data map[string]interface{} `xml:"data>entry"`
}
