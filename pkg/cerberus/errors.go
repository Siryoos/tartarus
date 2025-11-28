package cerberus

import "fmt"

// AuthenticationError indicates that credentials are invalid or missing.
type AuthenticationError struct {
	Message string
	Cause   error
}

func (e *AuthenticationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("authentication failed: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("authentication failed: %s", e.Message)
}

func (e *AuthenticationError) Unwrap() error {
	return e.Cause
}

// NewAuthenticationError creates a new authentication error.
func NewAuthenticationError(message string, cause error) *AuthenticationError {
	return &AuthenticationError{
		Message: message,
		Cause:   cause,
	}
}

// AuthorizationError indicates that the identity lacks permission.
type AuthorizationError struct {
	Message  string
	Identity *Identity
	Action   Action
	Resource Resource
}

func (e *AuthorizationError) Error() string {
	return fmt.Sprintf("authorization denied: %s (identity=%s, action=%s, resource=%s/%s)",
		e.Message, e.Identity.ID, e.Action, e.Resource.Type, e.Resource.ID)
}

// NewAuthorizationError creates a new authorization error.
func NewAuthorizationError(message string, identity *Identity, action Action, resource Resource) *AuthorizationError {
	return &AuthorizationError{
		Message:  message,
		Identity: identity,
		Action:   action,
		Resource: resource,
	}
}

// AuditError indicates that audit logging failed.
// This should not block the request but should be logged.
type AuditError struct {
	Message string
	Cause   error
}

func (e *AuditError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("audit failed: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("audit failed: %s", e.Message)
}

func (e *AuditError) Unwrap() error {
	return e.Cause
}

// NewAuditError creates a new audit error.
func NewAuditError(message string, cause error) *AuditError {
	return &AuditError{
		Message: message,
		Cause:   cause,
	}
}
