package tools

import (
	"github.com/heliohq/anycli/internal/tools/bitly"
	"github.com/heliohq/anycli/internal/tools/calendar"
	"github.com/heliohq/anycli/internal/tools/contacts"
	"github.com/heliohq/anycli/internal/tools/discord"
	"github.com/heliohq/anycli/internal/tools/docs"
	"github.com/heliohq/anycli/internal/tools/drive"
	"github.com/heliohq/anycli/internal/tools/figma"
	"github.com/heliohq/anycli/internal/tools/forms"
	"github.com/heliohq/anycli/internal/tools/freshbooks"
	"github.com/heliohq/anycli/internal/tools/gateprobe"
	"github.com/heliohq/anycli/internal/tools/gmail"
	"github.com/heliohq/anycli/internal/tools/linkedin"
	"github.com/heliohq/anycli/internal/tools/meet"
	"github.com/heliohq/anycli/internal/tools/microsoftcalendar"
	"github.com/heliohq/anycli/internal/tools/microsoftonedrive"
	"github.com/heliohq/anycli/internal/tools/microsoftoutlook"
	"github.com/heliohq/anycli/internal/tools/mongodb"
	"github.com/heliohq/anycli/internal/tools/notion"
	"github.com/heliohq/anycli/internal/tools/sheets"
	"github.com/heliohq/anycli/internal/tools/slack"
	"github.com/heliohq/anycli/internal/tools/slides"
	"github.com/heliohq/anycli/internal/tools/tasks"
	"github.com/heliohq/anycli/internal/tools/x"
)

// Built-in service registration. internal/exec imports this package (for
// GetService), so these init-time registrations are always live — no blank
// imports needed anywhere. Service packages implement the Service interface
// by duck typing and never import this registry, so registration cannot
// create an import cycle.
func init() {
	RegisterService("bitly", &bitly.Service{})
	RegisterService("slack", &slack.Service{})
	RegisterService("notion", &notion.Service{})
	RegisterService("sheets", &sheets.Service{})
	RegisterService("gmail", &gmail.Service{})
	RegisterService("slides", &slides.Service{})
	RegisterService("calendar", &calendar.Service{})
	RegisterService("contacts", &contacts.Service{})
	RegisterService("docs", &docs.Service{})
	RegisterService("drive", &drive.Service{})
	RegisterService("discord", &discord.Service{})
	RegisterService("figma", &figma.Service{})
	RegisterService("forms", &forms.Service{})
	RegisterService("linkedin", &linkedin.Service{})
	RegisterService("meet", &meet.Service{})
	RegisterService("tasks", &tasks.Service{})
	RegisterService("x", &x.Service{})
	RegisterService("microsoft-outlook", &microsoftoutlook.Service{})
	RegisterService("microsoft-calendar", &microsoftcalendar.Service{})
	RegisterService("microsoft-onedrive", &microsoftonedrive.Service{})
	RegisterService("mongodb", &mongodb.Service{})
	RegisterService("freshbooks", &freshbooks.Service{})
	// gate-probe is the approval-gate E2E harness (design 318): hidden,
	// credential-free, local-echo-only. Registered like every other service
	// so Inspect/lint/policy coverage traverse it; consumer-side visibility
	// is gated by the consumer, not here.
	RegisterService("gate-probe", &gateprobe.Service{})
}
