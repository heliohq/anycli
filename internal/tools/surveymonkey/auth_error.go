package surveymonkey

import (
	"net/http"

	"github.com/heliohq/anycli/internal/tools/execution"
)

// authErrorCodes are the SurveyMonkey error ids that signal an invalid/expired/
// revoked/missing authorization token (all HTTP 401). They mean the resolved
// credential itself is bad and should be invalidated, unlike the 403 permission/
// plan codes (1014/1015/1016/1017/1018), which leave the credential valid.
var authErrorCodes = map[string]bool{
	"1010": true, // token not provided
	"1011": true, // token invalid
	"1012": true, // token expired
	"1013": true, // token revoked
}

// classifyCredentialError marks a SurveyMonkey error as an explicit credential
// rejection when the HTTP status is 401 or the error code is one of the auth
// codes, so the engine can invalidate the stored token. Permission (1014),
// plan (1015), and region (1018) failures are NOT credential rejections — the
// token is valid; the account simply lacks a scope, plan, or regional host.
func classifyCredentialError(status int, code string, err error) error {
	if status == http.StatusUnauthorized || authErrorCodes[code] {
		return execution.RejectCredential(err)
	}
	return err
}
