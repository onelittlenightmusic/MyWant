package mywant

import "time"

// LogAPIOperation logs an API operation (POST, PUT, DELETE, etc.)
func (cb *ChainBuilder) LogAPIOperation(method, endpoint, resource, status string, statusCode int, errorMsg, details string) {
	entry := APILogEntry{
		Timestamp:  time.Now(),
		Method:     method,
		Endpoint:   endpoint,
		Resource:   resource,
		Status:     status,
		StatusCode: statusCode,
		ErrorMsg:   errorMsg,
		Details:    details,
	}

	cb.apiLogsMutex.Lock()
	defer cb.apiLogsMutex.Unlock()

	cb.apiLogs = append(cb.apiLogs, entry)

	// Keep only the most recent maxLogSize entries
	if len(cb.apiLogs) > cb.maxLogSize {
		cb.apiLogs = cb.apiLogs[len(cb.apiLogs)-cb.maxLogSize:]
	}
}

// GetAPILogs returns a copy of all API logs
func (cb *ChainBuilder) GetAPILogs() []APILogEntry {
	cb.apiLogsMutex.RLock()
	defer cb.apiLogsMutex.RUnlock()

	// Return a copy to prevent external modification
	logs := make([]APILogEntry, len(cb.apiLogs))
	copy(logs, cb.apiLogs)
	return logs
}

// ClearAPILogs clears all API logs
func (cb *ChainBuilder) ClearAPILogs() {
	cb.apiLogsMutex.Lock()
	defer cb.apiLogsMutex.Unlock()
	cb.apiLogs = make([]APILogEntry, 0)
}
