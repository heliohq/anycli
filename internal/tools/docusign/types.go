package docusign

// Raw DocuSign eSignature response shapes (camelCase, as the API returns them)
// and their projection to the provider-neutral views in shape.go. Only the
// fields the neutral views need are decoded.

// rawEnvelopeSummary is the create/send and void response envelope.
type rawEnvelopeSummary struct {
	EnvelopeID string `json:"envelopeId"`
	Status     string `json:"status"`
	URI        string `json:"uri"`
}

// rawEnvelope is one full envelope record (list element or `get`).
type rawEnvelope struct {
	EnvelopeID        string `json:"envelopeId"`
	Status            string `json:"status"`
	EmailSubject      string `json:"emailSubject"`
	CreatedDateTime   string `json:"createdDateTime"`
	SentDateTime      string `json:"sentDateTime"`
	CompletedDateTime string `json:"completedDateTime"`
}

func (e rawEnvelope) view() envelopeView {
	return envelopeView{
		ID:          e.EnvelopeID,
		Status:      e.Status,
		Subject:     e.EmailSubject,
		CreatedAt:   e.CreatedDateTime,
		SentAt:      e.SentDateTime,
		CompletedAt: e.CompletedDateTime,
	}
}

// rawEnvelopeList is the GET /envelopes list response.
type rawEnvelopeList struct {
	Envelopes []rawEnvelope `json:"envelopes"`
}

// rawRecipient is one signer / carbon-copy / agent recipient.
type rawRecipient struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	Status         string `json:"status"`
	RoutingOrder   string `json:"routingOrder"`
	RecipientID    string `json:"recipientId"`
	SignedDateTime string `json:"signedDateTime"`
}

func (r rawRecipient) view(kind string) recipientView {
	return recipientView{
		Name:         r.Name,
		Email:        r.Email,
		Status:       r.Status,
		Type:         kind,
		RoutingOrder: r.RoutingOrder,
		RecipientID:  r.RecipientID,
		SignedAt:     r.SignedDateTime,
	}
}

// rawRecipients is the GET /envelopes/{id}/recipients response. DocuSign splits
// recipients by role; the neutral view flattens them with a type tag.
type rawRecipients struct {
	Signers      []rawRecipient `json:"signers"`
	CarbonCopies []rawRecipient `json:"carbonCopies"`
	Agents       []rawRecipient `json:"agents"`
	Editors      []rawRecipient `json:"editors"`
}

func (r rawRecipients) views() []recipientView {
	views := make([]recipientView, 0, len(r.Signers)+len(r.CarbonCopies)+len(r.Agents)+len(r.Editors))
	for _, s := range r.Signers {
		views = append(views, s.view("signer"))
	}
	for _, s := range r.CarbonCopies {
		views = append(views, s.view("carbon_copy"))
	}
	for _, s := range r.Agents {
		views = append(views, s.view("agent"))
	}
	for _, s := range r.Editors {
		views = append(views, s.view("editor"))
	}
	return views
}

// rawTemplate is one reusable template (list element or `get`).
type rawTemplate struct {
	TemplateID  string `json:"templateId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Shared      string `json:"shared"`
	Created     string `json:"created"`
}

func (t rawTemplate) view() templateView {
	return templateView{
		ID:          t.TemplateID,
		Name:        t.Name,
		Description: t.Description,
		Shared:      t.Shared,
		CreatedAt:   t.Created,
	}
}

// rawTemplateList is the GET /templates response.
type rawTemplateList struct {
	EnvelopeTemplates []rawTemplate `json:"envelopeTemplates"`
}
