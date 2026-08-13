package onvif

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	pkgonvif "github.com/AlexxIT/go2rtc/pkg/onvif"
	"gopkg.in/yaml.v3"
)

const (
	eventCreatePullPointSubscription = "CreatePullPointSubscription"
	eventGetEventProperties          = "GetEventProperties"
	eventGetServiceCapabilities      = "GetServiceCapabilities"
	eventGetStatus                   = "GetStatus"
	eventPullMessages                = "PullMessages"
	eventRenew                       = "Renew"
	eventSetSynchronizationPoint     = "SetSynchronizationPoint"
	eventSubscribe                   = "Subscribe"
	eventUnsubscribe                 = "Unsubscribe"
)

const (
	eventActionBase = "http://www.onvif.org/ver10/events/wsdl/"
	wsnActionBase   = "http://docs.oasis-open.org/wsn/bw-2/"
)

const (
	defaultEventInterval    = time.Minute
	defaultEventBurst       = 10
	defaultSubscriptionTTL  = time.Hour
	maxPullMessages         = 100
	maxPullTimeout          = 30 * time.Second
	maxSubscriptionQueueLen = 512
)

var errSubscriptionNotFound = errors.New("onvif event subscription not found")

type eventTemplate struct {
	Enabled        *bool  `yaml:"enabled"`
	Topic          string `yaml:"topic"`
	SourceData     string `yaml:"sourceData"`
	StartData      string `yaml:"startData"`
	EndData        string `yaml:"endData"`
	StartOperation string `yaml:"startOperation"`
	EndOperation   string `yaml:"endOperation"`
}

type eventConfig struct {
	Enabled   *bool           `yaml:"enabled"`
	Interval  string          `yaml:"interval"`
	Burst     int             `yaml:"burst"`
	Permanent bool            `yaml:"permanent"`
	Templates []eventTemplate `yaml:"templates"`
}

// UnmarshalYAML accepts both the documented mapping and a plain template list.
func (c *eventConfig) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		type plain eventConfig
		value := plain{Interval: defaultEventInterval.String(), Burst: defaultEventBurst}
		if err := node.Decode(&value); err != nil {
			return err
		}
		*c = eventConfig(value)
		return nil

	case yaml.SequenceNode:
		var templates []eventTemplate
		if err := node.Decode(&templates); err != nil {
			return err
		}
		*c = eventConfig{
			Interval:  defaultEventInterval.String(),
			Burst:     defaultEventBurst,
			Templates: templates,
		}
		return nil

	case yaml.ScalarNode:
		if node.Tag == "!!null" || node.Value == "" {
			*c = eventConfig{}
			return nil
		}
	}

	return fmt.Errorf("event config must be a mapping or template list")
}

type eventNotification struct {
	Topic      string
	Data       string
	Source     string
	SourceData string
	Operation  string
	Time       time.Time
}

type eventSubscription struct {
	ID              string
	ExpiresAt       time.Time
	Source          string
	ConsumerURL     string
	TopicFilter     string
	TemplateIndexes []int
	Queue           []eventNotification
	Active          map[int]bool
	Notify          chan struct{}
	Done            chan struct{}
	doneOnce        sync.Once
}

type eventManager struct {
	mu            sync.Mutex
	interval      time.Duration
	burst         int
	permanent     bool
	enabled       bool
	templates     []eventTemplate
	subscriptions map[string]*eventSubscription
	stop          chan struct{}
	stopOnce      sync.Once
}

var events = newEventManager(eventConfig{})

var permanentSubscriptionExpiration = time.Date(9999, time.December, 31, 23, 59, 59, 0, time.UTC)

func newEventManager(cfg eventConfig) *eventManager {
	interval, err := time.ParseDuration(cfg.Interval)
	if err != nil || interval <= 0 {
		interval = defaultEventInterval
	}

	burst := cfg.Burst
	if burst < 0 {
		burst = 0
	}

	return &eventManager{
		interval:      interval,
		burst:         burst,
		permanent:     cfg.Permanent,
		enabled:       cfg.Enabled == nil || *cfg.Enabled,
		templates:     append([]eventTemplate(nil), cfg.Templates...),
		subscriptions: make(map[string]*eventSubscription),
		stop:          make(chan struct{}),
	}
}

func (m *eventManager) start() {
	if !m.enabled || len(m.templates) == 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case now := <-ticker.C:
				m.generate(now)
			case <-m.stop:
				return
			}
		}
	}()
}

func (m *eventManager) close() {
	m.stopOnce.Do(func() {
		m.mu.Lock()
		for _, sub := range m.subscriptions {
			sub.close()
		}
		m.mu.Unlock()
		close(m.stop)
	})
}

func (m *eventManager) create(source string, ttl time.Duration) (*eventSubscription, string) {
	return m.createSubscription(source, "", "", ttl)
}

func (m *eventManager) createPull(source, filter string, ttl time.Duration) (*eventSubscription, string) {
	return m.createSubscription(source, filter, "", ttl)
}

func (m *eventManager) createPush(source, filter, consumerURL string, ttl time.Duration) (*eventSubscription, string) {
	sub, id := m.createSubscription(source, filter, consumerURL, ttl)
	time.AfterFunc(pushStartDelay, func() { m.pushLoop(id) })
	return sub, id
}

func (m *eventManager) createSubscription(source, filter, consumerURL string, ttl time.Duration) (*eventSubscription, string) {
	if ttl <= 0 {
		ttl = defaultSubscriptionTTL
	}

	now := time.Now().UTC()
	expiresAt := now.Add(ttl)
	if m.permanent {
		expiresAt = permanentSubscriptionExpiration
	}
	sub := &eventSubscription{
		ID:              pkgonvif.UUID(),
		ExpiresAt:       expiresAt,
		Source:          source,
		ConsumerURL:     consumerURL,
		TopicFilter:     filter,
		TemplateIndexes: m.matchingTemplateIndexes(filter),
		Active:          make(map[int]bool),
		Notify:          make(chan struct{}, 1),
		Done:            make(chan struct{}),
	}

	m.mu.Lock()
	m.cleanupLocked(now)
	m.subscriptions[sub.ID] = sub
	m.enqueueLocked(sub, m.burst, now, "")
	m.mu.Unlock()

	return sub, sub.ID
}

func (m *eventManager) pull(id string, limit int, timeout time.Duration) ([]eventNotification, time.Time, error) {
	if limit <= 0 {
		limit = 10
	} else if limit > maxPullMessages {
		limit = maxPullMessages
	}
	if timeout < 0 {
		timeout = 0
	} else if timeout > maxPullTimeout {
		timeout = maxPullTimeout
	}

	m.mu.Lock()
	sub, ok := m.subscriptionLocked(id, time.Now().UTC())
	if !ok {
		m.mu.Unlock()
		return nil, time.Time{}, errSubscriptionNotFound
	}

	if len(sub.Queue) == 0 && timeout > 0 {
		notify := sub.Notify
		done := sub.Done
		if untilExpiration := time.Until(sub.ExpiresAt); untilExpiration < timeout {
			timeout = max(untilExpiration, time.Nanosecond)
		}
		m.mu.Unlock()

		timer := time.NewTimer(timeout)
		select {
		case <-notify:
			if !timer.Stop() {
				<-timer.C
			}
		case <-done:
			if !timer.Stop() {
				<-timer.C
			}
		case <-m.stop:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}

		m.mu.Lock()
		sub, ok = m.subscriptionLocked(id, time.Now().UTC())
		if !ok {
			m.mu.Unlock()
			return nil, time.Time{}, errSubscriptionNotFound
		}
	}

	if len(sub.Queue) < limit {
		limit = len(sub.Queue)
	}
	notifications := append([]eventNotification(nil), sub.Queue[:limit]...)
	sub.Queue = sub.Queue[limit:]
	expiresAt := sub.ExpiresAt
	if len(sub.Queue) == 0 {
		select {
		case <-sub.Notify:
		default:
		}
	}
	m.mu.Unlock()

	return notifications, expiresAt, nil
}

func (m *eventManager) renew(id string, ttl time.Duration) (time.Time, error) {
	if ttl <= 0 {
		ttl = defaultSubscriptionTTL
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscriptionLocked(id, time.Now().UTC())
	if !ok {
		return time.Time{}, errSubscriptionNotFound
	}
	if m.permanent {
		sub.ExpiresAt = permanentSubscriptionExpiration
	} else {
		sub.ExpiresAt = time.Now().UTC().Add(ttl)
	}
	return sub.ExpiresAt, nil
}

func (m *eventManager) unsubscribe(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if id != "" {
		if sub, ok := m.subscriptions[id]; ok {
			sub.close()
			delete(m.subscriptions, id)
		}
	}
	return nil
}

func (m *eventManager) status(id string) (time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscriptionLocked(id, time.Now().UTC())
	if !ok {
		return time.Time{}, errSubscriptionNotFound
	}
	return sub.ExpiresAt, nil
}

func (m *eventManager) synchronize(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, ok := m.subscriptionLocked(id, time.Now().UTC())
	if !ok {
		return errSubscriptionNotFound
	}
	m.enqueueLocked(sub, len(sub.TemplateIndexes), time.Now().UTC(), "Initialized")
	return nil
}

func (m *eventManager) generate(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupLocked(now)
	generated := 0
	for _, sub := range m.subscriptions {
		generated += m.enqueueLocked(sub, 1, now, "")
	}
	if generated > 0 {
		log.Debug().Int("subscriptions", len(m.subscriptions)).Int("messages", generated).
			Msg("[onvif] events generated")
	}
}

func (m *eventManager) enqueueLocked(sub *eventSubscription, count int, now time.Time, operationOverride string) int {
	if !m.enabled || len(sub.TemplateIndexes) == 0 || count <= 0 {
		return 0
	}

	generated := 0
	for range count {
		if len(sub.Queue) >= maxSubscriptionQueueLen {
			sub.Queue = sub.Queue[1:]
		}

		i := sub.TemplateIndexes[rand.IntN(len(sub.TemplateIndexes))]
		template := m.templates[i]
		active := sub.Active[i]
		data := template.StartData
		operation := template.StartOperation
		if active {
			data = template.EndData
			operation = template.EndOperation
		}
		if operationOverride != "" {
			operation = operationOverride
		} else if operation == "" {
			operation = "Changed"
		}
		sub.Active[i] = !active

		sub.Queue = append(sub.Queue, eventNotification{
			Topic:      template.Topic,
			Data:       data,
			Source:     sub.Source,
			SourceData: template.SourceData,
			Operation:  operation,
			Time:       now,
		})
		generated++
	}

	select {
	case sub.Notify <- struct{}{}:
	default:
	}
	return generated
}

func (m *eventManager) subscriptionLocked(id string, now time.Time) (*eventSubscription, bool) {
	sub, ok := m.subscriptions[id]
	if !ok {
		return nil, false
	}
	if !sub.ExpiresAt.After(now) {
		sub.close()
		delete(m.subscriptions, id)
		return nil, false
	}
	return sub, true
}

func (m *eventManager) cleanupLocked(now time.Time) {
	for id, sub := range m.subscriptions {
		if !sub.ExpiresAt.After(now) {
			sub.close()
			delete(m.subscriptions, id)
		}
	}
}

func (m *eventManager) matchingTemplateIndexes(filter string) []int {
	indexes := make([]int, 0, len(m.templates))
	for i, template := range m.templates {
		if template.Enabled != nil && !*template.Enabled {
			continue
		}
		if eventTopicMatches(filter, template.Topic) {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

func (sub *eventSubscription) close() {
	sub.doneOnce.Do(func() { close(sub.Done) })
}

func eventTopicMatches(filter, topic string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == "*" {
		return true
	}
	for expression := range strings.SplitSeq(filter, "|") {
		expression = strings.TrimSpace(expression)
		if expression == topic || strings.TrimSuffix(expression, "/*") == strings.TrimSuffix(topic, "/") {
			return true
		}
		if strings.HasSuffix(expression, "/*") && strings.HasPrefix(topic, strings.TrimSuffix(expression, "*")) {
			return true
		}
	}
	return false
}

func eventResponse(r *http.Request, request []byte, operation string) ([]byte, error) {
	switch operation {
	case eventGetServiceCapabilities:
		return eventCapabilitiesResponse(), nil

	case eventGetEventProperties:
		return eventPropertiesResponse(), nil

	case eventCreatePullPointSubscription:
		ttl := parseEventDuration(pkgonvif.FindTagValue(request, "InitialTerminationTime"), defaultSubscriptionTTL)
		sub, id := events.createPull(eventSource(r), eventTopicFilter(request), ttl)
		log.Debug().Str("subscription", id).Str("source", sub.Source).Int("queued", len(sub.Queue)).
			Msg("[onvif] event subscription created")
		return createSubscriptionResponse(eventSubscriptionURL(r, id), id, sub.ExpiresAt), nil

	case eventSubscribe:
		consumerURL, err := eventConsumerURL(request, r.RemoteAddr)
		if err != nil {
			return nil, err
		}
		ttl := parseEventDuration(pkgonvif.FindTagValue(request, "InitialTerminationTime"), defaultSubscriptionTTL)
		sub, id := events.createPush(eventSource(r), eventTopicFilter(request), consumerURL, ttl)
		log.Info().Str("subscription", id).Str("source", sub.Source).Str("consumer", consumerURL).
			Int("queued", len(sub.Queue)).Msg("[onvif] push subscription created")
		return subscribeResponse(eventSubscriptionURL(r, id), id, sub.ExpiresAt), nil

	case eventPullMessages:
		id := eventSubscriptionID(r, request)
		limit, _ := strconv.Atoi(pkgonvif.FindTagValue(request, "MessageLimit"))
		timeout := parseEventDuration(pkgonvif.FindTagValue(request, "Timeout"), 0)
		notifications, expiresAt, err := events.pull(id, limit, timeout)
		if err != nil {
			return nil, err
		}
		log.Debug().Str("subscription", id).Dur("timeout", timeout).Int("messages", len(notifications)).
			Msg("[onvif] pull messages")
		logDeliveredEvents("pull", id, notifications)
		return pullMessagesResponse(notifications, expiresAt, eventSubscriptionURL(r, id)), nil

	case eventRenew:
		id := eventSubscriptionID(r, request)
		ttl := parseEventDuration(pkgonvif.FindTagValue(request, "TerminationTime"), defaultSubscriptionTTL)
		expiresAt, err := events.renew(id, ttl)
		if err != nil {
			return nil, err
		}
		return renewResponse(expiresAt), nil

	case eventGetStatus:
		expiresAt, err := events.status(eventSubscriptionID(r, request))
		if err != nil {
			return nil, err
		}
		return statusResponse(expiresAt), nil

	case eventSetSynchronizationPoint:
		if err := events.synchronize(eventSubscriptionID(r, request)); err != nil {
			return nil, err
		}
		return eventEnvelope(`<tev:SetSynchronizationPointResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl"/>`), nil

	case eventUnsubscribe:
		if err := events.unsubscribe(eventSubscriptionID(r, request)); err != nil {
			return nil, err
		}
		return eventEnvelope(`<wsnt:UnsubscribeResponse xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2"/>`), nil
	}

	return nil, nil
}

func logDeliveredEvents(delivery, subscription string, notifications []eventNotification) {
	for _, notification := range notifications {
		log.Debug().
			Str("delivery", delivery).
			Str("subscription", subscription).
			Str("topic", notification.Topic).
			Str("source", notification.Source).
			Str("operation", notification.Operation).
			Str("data", notification.Data).
			Msg("[onvif] event delivered")
	}
}

func isEventRequest(r *http.Request, operation string) bool {
	switch operation {
	case eventCreatePullPointSubscription,
		eventGetEventProperties,
		eventGetStatus,
		eventPullMessages,
		eventRenew,
		eventSetSynchronizationPoint,
		eventSubscribe,
		eventUnsubscribe:
		return true
	case eventGetServiceCapabilities:
		return strings.Contains(r.URL.Path, "event") || r.URL.Query().Has("subscription")
	default:
		return false
	}
}

func eventResponseAction(operation string) string {
	switch operation {
	case eventGetServiceCapabilities:
		return eventActionBase + "EventPortType/GetServiceCapabilitiesResponse"
	case eventGetEventProperties:
		return eventActionBase + "EventPortType/GetEventPropertiesResponse"
	case eventCreatePullPointSubscription:
		return eventActionBase + "EventPortType/CreatePullPointSubscriptionResponse"
	case eventPullMessages:
		return eventActionBase + "PullPointSubscription/PullMessagesResponse"
	case eventSetSynchronizationPoint:
		return eventActionBase + "PullPointSubscription/SetSynchronizationPointResponse"
	case eventSubscribe:
		return wsnActionBase + "NotificationProducer/SubscribeResponse"
	case eventRenew:
		return wsnActionBase + "SubscriptionManager/RenewResponse"
	case eventGetStatus:
		return wsnActionBase + "SubscriptionManager/GetStatusResponse"
	case eventUnsubscribe:
		return wsnActionBase + "SubscriptionManager/UnsubscribeResponse"
	default:
		return ""
	}
}

func eventAddressedResponse(r *http.Request, request, response []byte, action string) []byte {
	if action == "" {
		return response
	}

	bodyAt := bytes.Index(response, []byte(`<s:Body>`))
	if bodyAt < 0 {
		return response
	}

	requestURL := eventRequestURL(r)
	var header strings.Builder
	header.Grow(512)
	header.WriteString(`<s:Header><wsa:Action xmlns:wsa="http://www.w3.org/2005/08/addressing" s:mustUnderstand="1">`)
	header.WriteString(escapeXML(action))
	header.WriteString(`</wsa:Action><wsa:MessageID xmlns:wsa="http://www.w3.org/2005/08/addressing">urn:uuid:`)
	header.WriteString(pkgonvif.UUID())
	header.WriteString(`</wsa:MessageID>`)
	if messageID := strings.TrimSpace(pkgonvif.FindTagValue(request, "MessageID")); messageID != "" {
		header.WriteString(`<wsa:RelatesTo xmlns:wsa="http://www.w3.org/2005/08/addressing">`)
		header.WriteString(escapeXML(messageID))
		header.WriteString(`</wsa:RelatesTo>`)
	}
	header.WriteString(`<wsa:ReplyTo xmlns:wsa="http://www.w3.org/2005/08/addressing"><wsa:Address>http://www.w3.org/2005/08/addressing/anonymous</wsa:Address></wsa:ReplyTo>`)
	header.WriteString(`<wsa:To xmlns:wsa="http://www.w3.org/2005/08/addressing" s:mustUnderstand="1">`)
	header.WriteString(escapeXML(requestURL))
	header.WriteString(`</wsa:To></s:Header>`)

	addressed := make([]byte, 0, len(response)+header.Len())
	addressed = append(addressed, response[:bodyAt]...)
	addressed = append(addressed, header.String()...)
	addressed = append(addressed, response[bodyAt:]...)
	return addressed
}

func eventRequestURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

func eventCapabilitiesResponse() []byte {
	return eventEnvelope(`<tev:GetServiceCapabilitiesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl">
	<tev:Capabilities WSSubscriptionPolicySupport="true" WSPullPointSupport="true" WSPausableSubscriptionManagerInterfaceSupport="false" MaxNotificationProducers="64" MaxPullPoints="64" PersistentNotificationStorage="false"/>
</tev:GetServiceCapabilitiesResponse>`)
}

func eventPropertiesResponse() []byte {
	return eventEnvelope(`<tev:GetEventPropertiesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:wstop="http://docs.oasis-open.org/wsn/t-1" xmlns:tns1="http://www.onvif.org/ver10/topics">
	<tev:TopicNamespaceLocation>http://www.onvif.org/onvif/ver10/topics/topicns.xml</tev:TopicNamespaceLocation>
	<wsnt:FixedTopicSet>true</wsnt:FixedTopicSet>
	<wstop:TopicSet>
		<tns1:VideoSource wstop:topic="true"/>
		<tns1:VideoAnalytics wstop:topic="true"/>
		<tns1:RuleEngine wstop:topic="true"/>
		<tns1:Device wstop:topic="true"/>
	</wstop:TopicSet>
	<tev:TopicExpressionDialect>http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet</tev:TopicExpressionDialect>
	<tev:TopicExpressionDialect>http://docs.oasis-open.org/wsn/t-1/TopicExpression/Concrete</tev:TopicExpressionDialect>
	<tev:MessageContentFilterDialect>http://www.onvif.org/ver10/tev/messageContentFilter/ItemFilter</tev:MessageContentFilterDialect>
	<tev:MessageContentSchemaLocation>http://www.onvif.org/onvif/ver10/schema/onvif.xsd</tev:MessageContentSchemaLocation>
</tev:GetEventPropertiesResponse>`)
}

func createSubscriptionResponse(address, id string, expiresAt time.Time) []byte {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return eventEnvelope(fmt.Sprintf(`<tev:CreatePullPointSubscriptionResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:wsa="http://www.w3.org/2005/08/addressing">
	<tev:SubscriptionReference>
		<wsa:Address>%s</wsa:Address>
		<wsa:ReferenceParameters><tev:SubscriptionId>%s</tev:SubscriptionId></wsa:ReferenceParameters>
	</tev:SubscriptionReference>
	<tev:CurrentTime>%s</tev:CurrentTime>
	<tev:TerminationTime>%s</tev:TerminationTime>
</tev:CreatePullPointSubscriptionResponse>`, escapeXML(address), escapeXML(id), now, expiresAt.UTC().Format(time.RFC3339Nano)))
}

func pullMessagesResponse(notifications []eventNotification, expiresAt time.Time, subscriptionURL string) []byte {
	e := pkgonvif.NewEnvelope()
	e.Append(`<tev:PullMessagesResponse xmlns:tev="http://www.onvif.org/ver10/events/wsdl" xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2" xmlns:wsa="http://www.w3.org/2005/08/addressing" xmlns:tns1="http://www.onvif.org/ver10/topics">`)
	e.Appendf("<tev:CurrentTime>%s</tev:CurrentTime>", time.Now().UTC().Format(time.RFC3339Nano))
	e.Appendf("<tev:TerminationTime>%s</tev:TerminationTime>", expiresAt.UTC().Format(time.RFC3339Nano))

	for _, notification := range notifications {
		e.Append(notificationMessageXML(notification, subscriptionURL))
	}

	e.Append(`</tev:PullMessagesResponse>`)
	return e.Bytes()
}

func notificationMessageXML(notification eventNotification, subscriptionURL string) string {
	operation := notification.Operation
	if operation == "" {
		operation = "Changed"
	}

	var body strings.Builder
	body.Grow(384 + len(notification.Data))
	body.WriteString(`<wsnt:NotificationMessage>`)
	if subscriptionURL != "" {
		address := escapeXML(subscriptionURL)
		body.WriteString(`<wsnt:SubscriptionReference><wsa:Address>`)
		body.WriteString(address)
		body.WriteString(`</wsa:Address></wsnt:SubscriptionReference>`)
		body.WriteString(`<wsnt:ProducerReference><wsa:Address>`)
		body.WriteString(address)
		body.WriteString(`</wsa:Address></wsnt:ProducerReference>`)
	}
	body.WriteString(`<wsnt:Topic Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">`)
	body.WriteString(escapeXML(notification.Topic))
	body.WriteString(`</wsnt:Topic><wsnt:Message><tt:Message UtcTime="`)
	body.WriteString(notification.Time.UTC().Format(time.RFC3339Nano))
	body.WriteString(`" PropertyOperation="`)
	body.WriteString(escapeXML(operation))
	body.WriteString(`"><tt:Source><tt:SimpleItem Name="VideoSourceConfigurationToken" Value="`)
	body.WriteString(escapeXML(notification.Source))
	body.WriteString(`"/>`)
	body.WriteString(notification.SourceData)
	body.WriteString(`</tt:Source><tt:Data>`)
	body.WriteString(notification.Data)
	body.WriteString(`</tt:Data></tt:Message></wsnt:Message></wsnt:NotificationMessage>`)
	return body.String()
}

func renewResponse(expiresAt time.Time) []byte {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return eventEnvelope(fmt.Sprintf(`<wsnt:RenewResponse xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
	<wsnt:TerminationTime>%s</wsnt:TerminationTime>
	<wsnt:CurrentTime>%s</wsnt:CurrentTime>
</wsnt:RenewResponse>`, expiresAt.UTC().Format(time.RFC3339Nano), now))
}

func statusResponse(expiresAt time.Time) []byte {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return eventEnvelope(fmt.Sprintf(`<wsnt:GetStatusResponse xmlns:wsnt="http://docs.oasis-open.org/wsn/b-2">
	<wsnt:CurrentTime>%s</wsnt:CurrentTime>
	<wsnt:TerminationTime>%s</wsnt:TerminationTime>
</wsnt:GetStatusResponse>`, now, expiresAt.UTC().Format(time.RFC3339Nano)))
}

func eventEnvelope(body string) []byte {
	e := pkgonvif.NewEnvelope()
	e.Append(body)
	return e.Bytes()
}

func eventSubscriptionURL(r *http.Request, id string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	u := &url.URL{Scheme: scheme, Host: r.Host, Path: "/onvif/Subscription"}
	query := u.Query()
	query.Set("Idx", id)
	if channel := r.URL.Query().Get("channel"); channel != "" {
		query.Set("channel", channel)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func eventSubscriptionID(r *http.Request, request []byte) string {
	if id := r.URL.Query().Get("Idx"); id != "" {
		return id
	}
	if id := r.URL.Query().Get("subscription"); id != "" {
		return id
	}
	if id := strings.TrimSpace(pkgonvif.FindTagValue(request, "SubscriptionId")); id != "" {
		return id
	}
	if address := pkgonvif.FindTagValue(request, "To"); address != "" {
		u, err := url.Parse(html.UnescapeString(address))
		if err == nil {
			if id := u.Query().Get("Idx"); id != "" {
				return id
			}
			return u.Query().Get("subscription")
		}
	}
	return ""
}

func onvifServiceQuery(r *http.Request) string {
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		return ""
	}
	return "?channel=" + url.QueryEscape(channel)
}

func eventSource(r *http.Request) string {
	names := streams.GetAllNames()
	channel, err := strconv.Atoi(r.URL.Query().Get("channel"))
	if err == nil && channel > 0 {
		if channel <= len(names) {
			return names[channel-1]
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return "go2rtc"
}

func parseEventDuration(value string, fallback time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}

	// ONVIF uses the XML duration form PT#H#M#S.
	if !strings.HasPrefix(value, "PT") {
		return fallback
	}
	value = strings.TrimPrefix(value, "PT")
	var duration time.Duration
	var number strings.Builder
	for _, ch := range value {
		if ch >= '0' && ch <= '9' || ch == '.' {
			number.WriteRune(ch)
			continue
		}
		if number.Len() == 0 {
			return fallback
		}
		part, err := strconv.ParseFloat(number.String(), 64)
		if err != nil {
			return fallback
		}
		number.Reset()
		switch ch {
		case 'H':
			duration += time.Duration(part * float64(time.Hour))
		case 'M':
			duration += time.Duration(part * float64(time.Minute))
		case 'S':
			duration += time.Duration(part * float64(time.Second))
		default:
			return fallback
		}
	}
	if number.Len() != 0 || duration <= 0 {
		return fallback
	}
	return duration
}

func escapeXML(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}
