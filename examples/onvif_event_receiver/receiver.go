package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

type simpleItem struct {
	Name  string `xml:"Name,attr" json:"name"`
	Value string `xml:"Value,attr" json:"value"`
}

type itemGroup struct {
	SimpleItems  []simpleItem  `xml:"SimpleItem"`
	ElementItems []elementItem `xml:"ElementItem"`
}

type elementItem struct {
	Name string `xml:"Name,attr"`
	XML  string `xml:",innerxml"`
}

type wireNotification struct {
	Topic   string `xml:"Topic"`
	Message struct {
		Event struct {
			UTCTime           string    `xml:"UtcTime,attr"`
			PropertyOperation string    `xml:"PropertyOperation,attr"`
			Source            itemGroup `xml:"Source"`
			Key               itemGroup `xml:"Key"`
			Data              itemGroup `xml:"Data"`
		} `xml:"Message"`
	} `xml:"Message"`
}

type receivedEvent struct {
	Type              string       `json:"type"`
	Mode              string       `json:"mode"`
	ReceivedAt        time.Time    `json:"received_at"`
	Topic             string       `json:"topic"`
	UTCTime           string       `json:"utc_time,omitempty"`
	PropertyOperation string       `json:"property_operation,omitempty"`
	Source            []simpleItem `json:"source,omitempty"`
	Key               []simpleItem `json:"key,omitempty"`
	Data              []simpleItem `json:"data,omitempty"`
	ElementData       []string     `json:"element_data,omitempty"`
}

type eventPrinter struct {
	mode string
	raw  bool
	mu   sync.Mutex
}

func (p *eventPrinter) receive(body []byte) (int, error) {
	if p.raw {
		p.mu.Lock()
		_, _ = fmt.Fprintf(os.Stderr, "\n--- RAW ONVIF %s ---\n%s\n--- END RAW ---\n", strings.ToUpper(p.mode), body)
		p.mu.Unlock()
	}

	events, err := parseNotifications(body, p.mode)
	if err != nil {
		return 0, err
	}
	for _, event := range events {
		line, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return 0, marshalErr
		}
		p.mu.Lock()
		_, _ = fmt.Fprintln(os.Stdout, string(line))
		p.mu.Unlock()
	}
	return len(events), nil
}

func parseNotifications(body []byte, mode string) ([]receivedEvent, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var events []receivedEvent
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "NotificationMessage" {
			continue
		}

		var wire wireNotification
		if err = decoder.DecodeElement(&wire, &start); err != nil {
			return nil, err
		}
		event := receivedEvent{
			Type:              "onvif_event",
			Mode:              mode,
			ReceivedAt:        time.Now().UTC(),
			Topic:             strings.TrimSpace(wire.Topic),
			UTCTime:           strings.TrimSpace(wire.Message.Event.UTCTime),
			PropertyOperation: strings.TrimSpace(wire.Message.Event.PropertyOperation),
			Source:            wire.Message.Event.Source.SimpleItems,
			Key:               wire.Message.Event.Key.SimpleItems,
			Data:              wire.Message.Event.Data.SimpleItems,
		}
		for _, item := range wire.Message.Event.Data.ElementItems {
			event.ElementData = append(event.ElementData, strings.TrimSpace(item.XML))
		}
		events = append(events, event)
	}
}
