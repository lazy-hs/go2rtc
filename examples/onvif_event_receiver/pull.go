package main

import (
	"context"
	"fmt"
	"time"
)

func (c *eventClient) runPull(ctx context.Context, cfg config, receiver *eventPrinter) error {
	sub, err := c.createPullPoint(ctx, cfg.topic, cfg.subscriptionTTL)
	if err != nil {
		return err
	}
	statusLog.Printf("PullPoint subscription created: manager=%s expires=%s", displayURL(sub.manager), formatExpiration(sub.expires))
	defer c.unsubscribeWithTimeout(sub)

	nextRenew := time.Now().Add(subscriptionRenewDelay(sub.expires, cfg.subscriptionTTL))
	for {
		if ctx.Err() != nil {
			statusLog.Printf("Stopping PullPoint receiver: %v", ctx.Err())
			return nil
		}
		if time.Now().After(nextRenew) {
			if err = c.renew(ctx, sub, cfg.subscriptionTTL); err != nil {
				return fmt.Errorf("renew PullPoint subscription: %w", err)
			}
			statusLog.Printf("PullPoint subscription renewed: expires=%s", formatExpiration(sub.expires))
			nextRenew = time.Now().Add(subscriptionRenewDelay(sub.expires, cfg.subscriptionTTL))
		}

		operation := `<tev:PullMessages xmlns:tev="` + eventNamespace + `"><tev:Timeout>` + xmlDuration(cfg.pullTimeout) +
			`</tev:Timeout><tev:MessageLimit>` + fmt.Sprint(cfg.messageLimit) + `</tev:MessageLimit></tev:PullMessages>`
		body, pullErr := c.soap(ctx, sub.manager, actionPullMessages, operation, sub.headers)
		if pullErr != nil {
			if ctx.Err() != nil {
				continue
			}
			return fmt.Errorf("PullMessages: %w", pullErr)
		}
		count, receiveErr := receiver.receive(body)
		if receiveErr != nil {
			return fmt.Errorf("parse PullMessages: %w", receiveErr)
		}
		statusLog.Printf("PullMessages completed: messages=%d", count)
	}
}

func (c *eventClient) createPullPoint(ctx context.Context, topic string, ttl time.Duration) (*subscription, error) {
	var filter string
	if topic != "" {
		filter = `<tev:Filter xmlns:wsnt="` + wsntNamespace + `" xmlns:tns1="http://www.onvif.org/ver10/topics"><wsnt:TopicExpression Dialect="http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet">` +
			escapeXML(topic) + `</wsnt:TopicExpression></tev:Filter>`
	}
	operation := `<tev:CreatePullPointSubscription xmlns:tev="` + eventNamespace + `">` + filter +
		`<tev:InitialTerminationTime>` + xmlDuration(ttl) + `</tev:InitialTerminationTime></tev:CreatePullPointSubscription>`
	body, err := c.soap(ctx, c.eventURL, actionCreatePullPoint, operation, nil)
	if err != nil {
		return nil, fmt.Errorf("CreatePullPointSubscription: %w", err)
	}
	return c.parseSubscription(body)
}

func (c *eventClient) renew(ctx context.Context, sub *subscription, ttl time.Duration) error {
	operation := `<wsnt:Renew xmlns:wsnt="` + wsntNamespace + `"><wsnt:TerminationTime>` + xmlDuration(ttl) + `</wsnt:TerminationTime></wsnt:Renew>`
	body, err := c.soap(ctx, sub.manager, actionRenew, operation, sub.headers)
	if err != nil {
		return err
	}
	if value := findNestedText(body, "RenewResponse", "TerminationTime"); value != "" {
		if expires, parseErr := time.Parse(time.RFC3339Nano, value); parseErr == nil {
			sub.expires = expires
		}
	}
	if sub.expires.IsZero() || !sub.expires.After(time.Now()) {
		sub.expires = time.Now().Add(ttl)
	}
	return nil
}

func (c *eventClient) unsubscribeWithTimeout(sub *subscription) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	operation := `<wsnt:Unsubscribe xmlns:wsnt="` + wsntNamespace + `"/>`
	if _, err := c.soap(ctx, sub.manager, actionUnsubscribe, operation, sub.headers); err != nil {
		statusLog.Printf("WARN Unsubscribe failed: %v", err)
		return
	}
	statusLog.Printf("Subscription removed")
}

func xmlDuration(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	if duration%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("PT%dS", seconds)
}

func subscriptionRenewDelay(expires time.Time, fallback time.Duration) time.Duration {
	remaining := time.Until(expires)
	if expires.IsZero() || remaining <= 0 {
		remaining = fallback
	}
	delay := remaining / 2
	if delay < time.Second {
		return time.Second
	}
	return delay
}

func formatExpiration(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.Local().Format(time.RFC3339)
}
