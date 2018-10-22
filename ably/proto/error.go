package proto

import (
	"bytes"
	"fmt"
)

// ErrorInfo describes an error returned via ProtocolMessage.
type ErrorInfo struct {
	StatusCode int    `json:"statusCode,omitempty" codec:"statusCode,omitempty"`
	Code       int    `json:"code,omitempty" codec:"code,omitempty"`
	HRef       string `json:"href,omitempty" codec:"href,omitempty"` //spec TI4
	Message    string `json:"message,omitempty" codec:"message,omitempty"`
	Server     string `json:"serverId,omitempty" codec:"serverId,omitempty"`
}

func (e *ErrorInfo) FromMap(ctx map[string]interface{}) {
	if v, ok := ctx["statusCode"]; ok {
		e.StatusCode = coerceInt(v)
	}
	if v, ok := ctx["code"]; ok {
		e.Code = coerceInt(v)
	}
	if v, ok := ctx["href"]; ok {
		e.HRef = v.(string)
	}
	if v, ok := ctx["message"]; ok {
		e.Message = v.(string)
	}
	if v, ok := ctx["serverId"]; ok {
		e.Server = v.(string)
	}
}

// Error implements the builtin error interface.
func (e *ErrorInfo) Error() string {
	var buf bytes.Buffer
	buf.WriteString("[ErrorInfo")
	if e.Message != "" {
		fmt.Fprintf(&buf, ":%s", e.Message)
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&buf, ": statusCode=%d", e.StatusCode)
	}
	if e.Code != 0 {
		// spec TI5
		fmt.Fprintf(&buf, ": See https://help.ably.io/error/%d", e.Code)
	}
	buf.WriteString("]")
	return buf.String()
}
