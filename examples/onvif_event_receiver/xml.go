package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"time"
)

func findNestedText(body []byte, parent, target string) string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var stack []string
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		switch token := token.(type) {
		case xml.StartElement:
			if token.Name.Local == target && containsName(stack, parent) {
				var value string
				if decoder.DecodeElement(&value, &token) == nil {
					return strings.TrimSpace(value)
				}
				return ""
			}
			stack = append(stack, token.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}

func hasElement(body []byte, name string) bool {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == name {
			return true
		}
	}
}

func findServiceXAddr(body []byte, namespace string) string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Service" {
			continue
		}
		var service struct {
			Namespace string `xml:"Namespace"`
			XAddr     string `xml:"XAddr"`
		}
		if decoder.DecodeElement(&service, &start) == nil && strings.TrimSpace(service.Namespace) == namespace {
			return strings.TrimSpace(service.XAddr)
		}
	}
}

func parseSubscriptionResponse(body []byte) (address string, headers []string, expires time.Time, err error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var stack []string
	for {
		token, tokenErr := decoder.Token()
		if tokenErr == io.EOF {
			break
		}
		if tokenErr != nil {
			return "", nil, time.Time{}, tokenErr
		}

		switch token := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			switch {
			case token.Name.Local == "Address" && containsName(stack, "SubscriptionReference"):
				if decodeErr := decoder.DecodeElement(&address, &token); decodeErr != nil {
					return "", nil, time.Time{}, decodeErr
				}
				address = strings.TrimSpace(address)
				continue
			case token.Name.Local == "TerminationTime":
				var value string
				if decodeErr := decoder.DecodeElement(&value, &token); decodeErr != nil {
					return "", nil, time.Time{}, decodeErr
				}
				expires, _ = time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
				continue
			case parent == "ReferenceParameters":
				header, encodeErr := encodeReferenceParameter(decoder, token)
				if encodeErr != nil {
					return "", nil, time.Time{}, encodeErr
				}
				headers = append(headers, header)
				continue
			}
			stack = append(stack, token.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if address == "" {
		return "", nil, time.Time{}, errors.New("subscription response has no manager address")
	}
	return address, headers, expires, nil
}

func encodeReferenceParameter(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	for _, attr := range start.Attr {
		if attr.Name.Space == wsaNamespace && attr.Name.Local == "IsReferenceParameter" {
			return encodeElement(decoder, start)
		}
	}
	start.Attr = append(start.Attr, xml.Attr{
		Name:  xml.Name{Space: wsaNamespace, Local: "IsReferenceParameter"},
		Value: "true",
	})
	return encodeElement(decoder, start)
}

func encodeElement(decoder *xml.Decoder, start xml.StartElement) (string, error) {
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	if err := encoder.EncodeToken(start); err != nil {
		return "", err
	}
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		switch token.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
		if err = encoder.EncodeToken(token); err != nil {
			return "", err
		}
	}
	if err := encoder.Flush(); err != nil {
		return "", err
	}
	return output.String(), nil
}

func containsName(stack []string, name string) bool {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == name {
			return true
		}
	}
	return false
}
