package client

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gravitino/terraform-provider-gravitino/internal/models"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// HTTPError is returned for every response with a status >= 400.
//
// It carries the real HTTP status code, because the `code` field of a Gravitino
// error payload is an application code in the 1000-1100 range (see ErrorModel in
// the Gravitino OpenAPI spec) and therefore can never be used to detect a 404.
type HTTPError struct {
	StatusCode int
	Status     string
	// Response is the decoded Gravitino error payload, or nil when the body was
	// not a Gravitino error payload (proxies, plain-text gateways, ...).
	Response *models.ErrorResponse
}

func (e *HTTPError) Error() string {
	if e.Response != nil {
		return fmt.Sprintf("%s (HTTP %d %s, type %s)", e.Response.Message, e.StatusCode, e.Status, e.Response.Type)
	}
	return fmt.Sprintf("request failed with status %d %s", e.StatusCode, e.Status)
}

// AsNotFound reports whether err is a 404 from the Gravitino API.
func (e *HTTPError) AsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

func NewResourceError(operation, resource string, err error) diag.Diagnostics {
	var diags diag.Diagnostics

	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		summary := fmt.Sprintf("Failed %s %q", operation, resource)

		if httpErr.Response != nil {
			diags = append(diags, diag.NewErrorDiagnostic(
				summary,
				fmt.Sprintf("Server returned [%d %s] (%s): %s",
					httpErr.StatusCode, httpErr.Status, httpErr.Response.Type, httpErr.Response.Message),
			))
			if len(httpErr.Response.Stack) > 0 {
				diags = append(diags, diag.NewErrorDiagnostic(
					"Server Stack Trace",
					strings.Join(httpErr.Response.Stack, "\n"),
				))
			}
			return diags
		}

		diags = append(diags, diag.NewErrorDiagnostic(summary, httpErr.Error()))
		if hint := errorHint(err); hint != "" {
			diags = append(diags, diag.NewErrorDiagnostic("Hint", hint))
		}
		return diags
	}

	var errResp *models.ErrorResponse
	if errors.As(err, &errResp) {
		diags = append(diags, diag.NewErrorDiagnostic(
			fmt.Sprintf("Failed %s %q", operation, resource),
			fmt.Sprintf("Server returned [%d %s]: %s", errResp.Code, errResp.Type, errResp.Message),
		))
		if len(errResp.Stack) > 0 {
			diags = append(diags, diag.NewErrorDiagnostic(
				"Server Stack Trace",
				strings.Join(errResp.Stack, "\n"),
			))
		}
		return diags
	}

	diags = append(diags, diag.NewErrorDiagnostic(
		fmt.Sprintf("Failed %s %q", operation, resource),
		err.Error(),
	))

	if hint := errorHint(err); hint != "" {
		diags = append(diags, diag.NewErrorDiagnostic("Hint", hint))
	}

	return diags
}

// NewDropError renders a failed drop. When force_destroy is false and
// Gravitino refused because the object still contains children or is in use,
// it appends an actionable diagnostic naming the force_destroy attribute and
// the command that retries the destroy after it is enabled.
func NewDropError(operation, resource string, forceDestroy bool, err error) diag.Diagnostics {
	diags := NewResourceError(operation, resource, err)
	if forceDestroy || !isDropRefused(err) {
		return diags
	}

	diags = append(diags, diag.NewErrorDiagnostic(
		"force_destroy is false",
		fmt.Sprintf(
			"Gravitino refused to drop %q because it still contains objects or is in use. "+
				"Set force_destroy = true on this resource and run terraform destroy again to drop it "+
				"together with everything underneath it, or delete the nested objects first.",
			resource,
		),
	))
	return diags
}

// isDropRefused reports whether err is Gravitino's refusal to drop a
// non-empty or in-use entity. The server reports these as InUseException or
// NonEmpty*Exception types. A typed payload is authoritative: NotInUseException
// (a disabled parent metalake or catalog) is also an HTTP 409, but force_destroy
// cannot resolve it. Only an untyped 409 falls back to the status.
func isDropRefused(err error) bool {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	if httpErr.Response != nil && httpErr.Response.Type != "" {
		switch httpErr.Response.Type {
		case "InUseException", "MetalakeInUseException", "CatalogInUseException",
			"NonEmptyMetalakeException", "NonEmptyCatalogException":
			return true
		}
		return false
	}
	return httpErr.StatusCode == http.StatusConflict
}

// IsNotFoundError reports whether err is a Gravitino 404.
//
// Callers use it to drop a resource from state (Read) or to treat an already
// deleted object as success (Delete).
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.AsNotFound()
	}
	return false
}

func errorHint(err error) string {
	if err == nil {
		return ""
	}
	errStr := err.Error()

	if strings.Contains(errStr, "connection refused") {
		return "The Gravitino server is unreachable. Verify the URI and that the server is running."
	}
	if strings.Contains(errStr, "no such host") {
		return "Host not found. Verify the Gravitino server hostname is correct."
	}
	if strings.Contains(errStr, "certificate") || strings.Contains(errStr, "x509") {
		return "TLS certificate error. If using self-signed certificates, configure your HTTP client accordingly."
	}
	if strings.Contains(errStr, "401") || strings.Contains(errStr, "Unauthorized") {
		return "Authentication failed. Check your credentials (username/password or OAuth token)."
	}
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		return "Request timed out. The Gravitino server may be overloaded or unreachable; the provider aborts HTTP requests after 30s."
	}
	if strings.Contains(errStr, "invalid character") {
		return "Invalid response from server. The URI may be incorrect (e.g., pointing to a web page instead of the API)."
	}

	return ""
}
