package onvif

import (
	"bytes"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"strings"

	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
)

type onvifUsernameToken struct {
	Username     string
	Password     string
	PasswordType string
	Nonce        string
	Created      string
}

func onvifAuthRequired(operation string, auth rtspAuthConfig) bool {
	if strings.TrimSpace(auth.Username) == "" {
		return false
	}

	// ONVIF clients commonly call this before building WS-Security timestamps.
	return operation != pkgonvif.DeviceGetSystemDateAndTime
}

func validateONVIFAuth(r *http.Request, request []byte, auth rtspAuthConfig) bool {
	username := strings.TrimSpace(auth.Username)
	password := strings.TrimSpace(auth.Password)
	if username == "" {
		return true
	}

	if user, pass, ok := r.BasicAuth(); ok {
		return constantTimeEqual(user, username) && constantTimeEqual(pass, password)
	}

	token, ok := parseONVIFUsernameToken(request)
	if !ok {
		return false
	}
	if !constantTimeEqual(strings.TrimSpace(token.Username), username) {
		return false
	}

	passwordType := token.PasswordType
	switch {
	case strings.Contains(passwordType, "PasswordDigest"):
		return validateONVIFPasswordDigest(token, password)
	case strings.Contains(passwordType, "PasswordText"), token.Nonce == "" && token.Created == "":
		return constantTimeEqual(token.Password, password)
	default:
		return false
	}
}

func parseONVIFUsernameToken(request []byte) (onvifUsernameToken, bool) {
	decoder := xml.NewDecoder(bytes.NewReader(request))
	var token onvifUsernameToken
	var inUsernameToken bool
	var current string

	for {
		item, err := decoder.Token()
		if err != nil {
			break
		}

		switch item := item.(type) {
		case xml.StartElement:
			if item.Name.Local == "UsernameToken" {
				inUsernameToken = true
				continue
			}
			if !inUsernameToken {
				continue
			}
			current = item.Name.Local
			if current == "Password" {
				token.PasswordType = ""
				for _, attr := range item.Attr {
					if attr.Name.Local == "Type" {
						token.PasswordType = attr.Value
						break
					}
				}
			}
		case xml.CharData:
			if !inUsernameToken || current == "" {
				continue
			}
			value := strings.TrimSpace(string(item))
			if value == "" {
				continue
			}
			switch current {
			case "Username":
				token.Username += value
			case "Password":
				token.Password += value
			case "Nonce":
				token.Nonce += value
			case "Created":
				token.Created += value
			}
		case xml.EndElement:
			if item.Name.Local == "UsernameToken" {
				return token, token.Username != "" && token.Password != ""
			}
			if inUsernameToken && item.Name.Local == current {
				current = ""
			}
		}
	}

	return token, false
}

func validateONVIFPasswordDigest(token onvifUsernameToken, password string) bool {
	if token.Nonce == "" || token.Created == "" || token.Password == "" {
		return false
	}

	nonce, err := base64.StdEncoding.DecodeString(token.Nonce)
	if err != nil {
		nonce = []byte(token.Nonce)
	}

	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(token.Created))
	h.Write([]byte(password))
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return constantTimeEqual(token.Password, expected)
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func writeONVIFAuthError(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="go2rtc-onvif"`)
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:ter="http://www.onvif.org/ver10/error"><s:Body><s:Fault><s:Code><s:Value>s:Sender</s:Value><s:Subcode><s:Value>ter:NotAuthorized</s:Value></s:Subcode></s:Code><s:Reason><s:Text xml:lang="en">Unauthorized</s:Text></s:Reason></s:Fault></s:Body></s:Envelope>`))
}
