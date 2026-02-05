package main

// SMTPResponseCode represents an SMTP response code
type SMTPResponseCode int

// ValidationResult represents the result of email validation
type ValidationResult string

const (
	ResultValid           ValidationResult = "valid"
	ResultInvalid         ValidationResult = "invalid"
	ResultTemporaryFailure ValidationResult = "temporary_failure"
	ResultCatchall        ValidationResult = "catchall"
	ResultPendingRetry    ValidationResult = "pending_retry"
)

// ValidationCategory represents the category of validation result
type ValidationCategory string

const (
	CategorySuccess         ValidationCategory = "success"
	CategoryTemporaryFailure ValidationCategory = "temporary_failure"
	CategoryPermanentFailure ValidationCategory = "permanent_failure"
)

// ValidationAction represents the action to take
type ValidationAction string

const (
	ActionAccept ValidationAction = "accept"
	ActionReject ValidationAction = "reject"
	ActionRetry  ValidationAction = "retry"
	ActionLog    ValidationAction = "log"
)

// SMTPCodeInfo contains information about how to handle a specific SMTP code
type SMTPCodeInfo struct {
	Category          ValidationCategory `json:"category"`
	Type              string            `json:"type"`
	Action            ValidationAction  `json:"action"`
	Description       string            `json:"description"`
	ValidationResult  ValidationResult  `json:"validation_result"`
	RetryAfterSeconds int               `json:"retry_after_seconds,omitempty"`
	MaxRetries        int               `json:"max_retries,omitempty"`
	CatchallIndicator string            `json:"catchall_indicator,omitempty"`
	Notes             string            `json:"notes,omitempty"`
}

// GetSMTPCodeInfo returns the handling information for a specific SMTP response code
func GetSMTPCodeInfo(code int) *SMTPCodeInfo {
	switch code {
	// Success codes (2xx)
	case 200:
		return &SMTPCodeInfo{
			Category:         CategorySuccess,
			Type:             "valid",
			Action:           ActionAccept,
			Description:      "Non-standard success response",
			ValidationResult: ResultValid,
		}
	case 220:
		return &SMTPCodeInfo{
			Category:         CategorySuccess,
			Type:             "service_ready",
			Action:           ActionAccept,
			Description:      "Service ready",
			ValidationResult: ResultValid,
		}
	case 250:
		return &SMTPCodeInfo{
			Category:         CategorySuccess,
			Type:             "valid",
			Action:           ActionAccept,
			Description:      "Requested mail action okay, completed",
			ValidationResult: ResultValid,
			Notes:            "May indicate catch-all if accepts invalid addresses",
		}
	case 251:
		return &SMTPCodeInfo{
			Category:          CategorySuccess,
			Type:              "valid_forward",
			Action:            ActionAccept,
			Description:       "User not local; will forward to <forward-path>",
			ValidationResult:  ResultValid,
			CatchallIndicator: "medium",
			Notes:             "Indicates forwarding behavior, may suggest catch-all domain",
		}
	case 252:
		return &SMTPCodeInfo{
			Category:          CategorySuccess,
			Type:              "catchall_strong_indicator",
			Action:            ActionAccept,
			Description:       "Cannot VRFY user, but will accept message and attempt delivery",
			ValidationResult:  ResultCatchall,
			CatchallIndicator: "strong",
			Notes:             "Server accepts mail without verifying user - strong catch-all indicator",
		}

	// Temporary failure codes (4xx)
	case 421:
		return &SMTPCodeInfo{
			Category:          CategoryTemporaryFailure,
			Type:              "service_unavailable",
			Action:            ActionRetry,
			Description:       "Service not available, closing transmission channel",
			ValidationResult:  ResultTemporaryFailure,
			RetryAfterSeconds: 300,
			MaxRetries:        3,
		}
	case 450:
		return &SMTPCodeInfo{
			Category:          CategoryTemporaryFailure,
			Type:              "greylisting",
			Action:            ActionRetry,
			Description:       "Requested mail action not taken: mailbox unavailable (e.g., mailbox busy)",
			ValidationResult:  ResultTemporaryFailure,
			RetryAfterSeconds: 60,
			MaxRetries:        3,
			Notes:             "Commonly used for greylisting - retry after delay",
		}
	case 451:
		return &SMTPCodeInfo{
			Category:          CategoryTemporaryFailure,
			Type:              "greylisting",
			Action:            ActionRetry,
			Description:       "Requested action aborted: local error in processing",
			ValidationResult:  ResultTemporaryFailure,
			RetryAfterSeconds: 300,
			MaxRetries:        3,
			Notes:             "Often indicates greylisting or temporary server issues - MUST retry with exponential backoff",
		}
	case 452:
		return &SMTPCodeInfo{
			Category:          CategoryTemporaryFailure,
			Type:              "inbox_full",
			Action:            ActionRetry,
			Description:       "Requested action not taken: insufficient system storage",
			ValidationResult:  ResultTemporaryFailure,
			RetryAfterSeconds: 3600,
			MaxRetries:        2,
			Notes:             "Inbox full or server overloaded - retry after longer delay",
		}
	case 503:
		return &SMTPCodeInfo{
			Category:          CategoryTemporaryFailure,
			Type:              "bad_sequence",
			Action:            ActionRetry,
			Description:       "Bad sequence of commands",
			ValidationResult:  ResultTemporaryFailure,
			RetryAfterSeconds: 0,
			Notes:             "May require restarting SMTP conversation",
		}

	// Permanent failure codes (5xx)
	case 500, 501, 502, 504:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "syntax_error",
			Action:           ActionReject,
			Description:      "Syntax error or command not implemented",
			ValidationResult: ResultInvalid,
		}
	case 521:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "domain_not_accept_mail",
			Action:           ActionReject,
			Description:      "Domain does not accept mail",
			ValidationResult: ResultInvalid,
			Notes:            "Domain does not accept mail (RFC 7504)",
		}
	case 530:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "authentication_required",
			Action:           ActionReject,
			Description:      "Authentication required",
			ValidationResult: ResultInvalid,
		}
	case 550:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "user_unknown",
			Action:           ActionReject,
			Description:      "Requested action not taken: mailbox unavailable",
			ValidationResult: ResultInvalid,
			Notes:            "User does not exist - HARD BOUNCE - do not retry",
		}
	case 551:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "user_not_local",
			Action:           ActionReject,
			Description:      "User not local; please try <forward-path>",
			ValidationResult: ResultInvalid,
		}
	case 552:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "mailbox_full",
			Action:           ActionReject,
			Description:      "Requested mail action aborted: exceeded storage allocation",
			ValidationResult: ResultInvalid,
			Notes:            "Permanent mailbox full (different from 452 which is temporary)",
		}
	case 553:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "mailbox_name_not_allowed",
			Action:           ActionReject,
			Description:      "Requested action not taken: mailbox name not allowed",
			ValidationResult: ResultInvalid,
		}
	case 554:
		return &SMTPCodeInfo{
			Category:         CategoryPermanentFailure,
			Type:             "transaction_failed",
			Action:           ActionReject,
			Description:      "Transaction failed",
			ValidationResult: ResultInvalid,
		}

	default:
		// Handle by code range
		if code >= 200 && code < 300 {
			return &SMTPCodeInfo{
				Category:         CategorySuccess,
				Type:             "success",
				Action:           ActionAccept,
				Description:      "Success response",
				ValidationResult: ResultValid,
			}
		} else if code >= 400 && code < 500 {
			return &SMTPCodeInfo{
				Category:          CategoryTemporaryFailure,
				Type:              "temporary_failure",
				Action:            ActionRetry,
				Description:       "Temporary failure",
				ValidationResult:  ResultTemporaryFailure,
				RetryAfterSeconds: 300,
				MaxRetries:        3,
			}
		} else if code >= 500 && code < 600 {
			return &SMTPCodeInfo{
				Category:         CategoryPermanentFailure,
				Type:             "permanent_failure",
				Action:           ActionReject,
				Description:      "Permanent failure",
				ValidationResult: ResultInvalid,
			}
		}
		return nil
	}
}

// IsRetryable checks if a code should be retried
func (info *SMTPCodeInfo) IsRetryable() bool {
	return info.Category == CategoryTemporaryFailure && info.Action == ActionRetry
}

// IsCatchallIndicator checks if a code indicates catch-all behavior
func (info *SMTPCodeInfo) IsCatchallIndicator() bool {
	return info.CatchallIndicator == "strong" || info.CatchallIndicator == "medium"
}

// ShouldTestForCatchall determines if we should test for catch-all after this response
func ShouldTestForCatchall(code int) bool {
	return code == 250 || code == 251 || code == 252
}
