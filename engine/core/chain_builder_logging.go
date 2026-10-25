package mywant

import "time"

// LogAPIOperation logs an API operation (POST, PUT, DELETE, etc.)
func (cb *ChainBuilder) LogAPIOperation(method, endpoint, resource, status string, statusCode int, errorMsg, details string) {
	cb.LogAPIRequest("", method, endpoint, resource, status, statusCode, errorMsg, details)
}

// LogAPIRequest is LogAPIOperation for a request with an ID (request_id.go).
func (cb *ChainBuilder) LogAPIRequest(requestID, method, endpoint, resource, status string, statusCode int, errorMsg, details string) {
	entry := APILogEntry{
		RequestID:  requestID,
		Timestamp:  time.Now(),
		Method:     method,
		Endpoint:   endpoint,
		Resource:   resource,
		Status:     status,
		StatusCode: statusCode,
		ErrorMsg:   errorMsg,
		Details:    details,
	}
	cb.apiLogs.Append(entry)
}

// GetAPILogs returns a snapshot of all API logs.
func (cb *ChainBuilder) GetAPILogs() []APILogEntry {
	return cb.apiLogs.Snapshot(0)
}

// ClearAPILogs clears all API logs.
func (cb *ChainBuilder) ClearAPILogs() {
	cb.apiLogs.Clear()
}
