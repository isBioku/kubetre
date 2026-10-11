package airlock

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/smtp"
	"sort"
	"strings"
	"time"

	treV1 "github.com/isBioku/kubetre/api/v1alpha1"
)

// Message is one notification email.
type Message struct {
	To      []string
	Subject string
	Body    string
}

// Notifier sends notifications.
type Notifier interface {
	Send(ctx context.Context, m Message) error
}

// Messages decides who is told about a request reaching a status, following AzureTRE's
// notifier: airlock managers when a request needs review, researchers when it is approved.
// The requester also hears about every other outcome.
func Messages(req *treV1.AirlockRequest, ws *treV1.Workspace, uiURL string) []Message {
	link := strings.TrimSuffix(uiURL, "/") + "/workspaces/" + ws.Name + "/requests/" + req.Name
	subject := func(what string) string {
		return fmt.Sprintf("[Azure TRE] %s request %q %s", req.Spec.Type, req.Spec.Title, what)
	}
	body := func(lead string) string {
		var b bytes.Buffer
		fmt.Fprintf(&b, "%s\n\n", lead)
		fmt.Fprintf(&b, "Workspace: %s\nRequest: %s (%s)\nType: %s\nRequested by: %s\n",
			ws.Spec.DisplayName, req.Spec.Title, req.Name, req.Spec.Type, nameOf(req.Spec.CreatedBy))
		if req.Spec.BusinessJustification != "" {
			fmt.Fprintf(&b, "Justification: %s\n", req.Spec.BusinessJustification)
		}
		for _, f := range req.Status.Files {
			fmt.Fprintf(&b, "File: %s (%d bytes)\n", f.Name, f.Size)
		}
		if req.Status.StatusMessage != "" {
			fmt.Fprintf(&b, "Message: %s\n", req.Status.StatusMessage)
		}
		fmt.Fprintf(&b, "\nOpen the request: %s\n", link)
		return b.String()
	}
	requester := emails([]string{req.Spec.CreatedBy.Email})
	switch req.Status.Status {
	case treV1.AirlockInReview:
		return nonEmpty(Message{To: emails(ws.Spec.AirlockManagers), Subject: subject("needs your review"),
			Body: body("An airlock request is waiting for review.")})
	case treV1.AirlockApproved:
		return nonEmpty(Message{To: emails(append(append([]string{}, ws.Spec.Researchers...), req.Spec.CreatedBy.Email)),
			Subject: subject("was approved"), Body: body("An airlock request was approved. Its file can now be downloaded.")})
	case treV1.AirlockRejected:
		return nonEmpty(Message{To: requester, Subject: subject("was rejected"), Body: body("Your airlock request was rejected.")})
	case treV1.AirlockBlockedByScan:
		return nonEmpty(Message{To: requester, Subject: subject("was blocked by the malware scan"), Body: body("Your airlock request was blocked: the malware scan found a threat.")})
	case treV1.AirlockFailed:
		return nonEmpty(Message{To: requester, Subject: subject("failed"), Body: body("Your airlock request failed.")})
	case treV1.AirlockRevoked:
		return nonEmpty(Message{To: emails(append(append([]string{}, ws.Spec.Researchers...), req.Spec.CreatedBy.Email)),
			Subject: subject("was revoked"), Body: body("An approved airlock request was revoked. Its file can no longer be downloaded.")})
	}
	return nil
}

func nonEmpty(m Message) []Message {
	if len(m.To) == 0 {
		return nil
	}
	return []Message{m}
}

func nameOf(u treV1.AirlockUser) string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

// emails keeps entries that look like email addresses, lower-cased and de-duplicated.
func emails(list []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, e := range list {
		e = strings.ToLower(strings.TrimSpace(e))
		if strings.Count(e, "@") == 1 && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	sort.Strings(out)
	return out
}

// SMTPNotifier sends email through an SMTP server with STARTTLS, such as Azure Communication
// Services' SMTP endpoint.
type SMTPNotifier struct {
	Host, Username, Password, From string
	Port                           int
}

func (n *SMTPNotifier) Send(ctx context.Context, m Message) error {
	addr := net.JoinHostPort(n.Host, fmt.Sprint(n.Port))
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		n.From, strings.Join(m.To, ", "), m.Subject, time.Now().UTC().Format(time.RFC1123Z), m.Body)
	var auth smtp.Auth
	if n.Username != "" {
		auth = smtp.PlainAuth("", n.Username, n.Password, n.Host)
	}
	done := make(chan error, 1)
	go func() { done <- smtp.SendMail(addr, auth, n.From, m.To, msg.Bytes()) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RecordingNotifier keeps messages, for tests.
type RecordingNotifier struct{ Sent []Message }

func (r *RecordingNotifier) Send(_ context.Context, m Message) error {
	r.Sent = append(r.Sent, m)
	return nil
}
